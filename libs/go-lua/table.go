package lua

const defaultArrayCap = 32
const defaultHashCap = 32

type lValueArraySorter struct {
	L      *LState
	Fn     *LFunction
	Values []LValue
}

func (lv lValueArraySorter) Len() int {
	return len(lv.Values)
}

func (lv lValueArraySorter) Swap(i, j int) {
	lv.Values[i], lv.Values[j] = lv.Values[j], lv.Values[i]
}

func (lv lValueArraySorter) Less(i, j int) bool {
	if lv.Fn != nil {
		lv.L.Push(lv.Fn)
		lv.L.Push(lv.Values[i])
		lv.L.Push(lv.Values[j])
		lv.L.Call(2, 1)
		return LVAsBool(lv.L.reg.Pop())
	}
	return lessThan(lv.L, lv.Values[i], lv.Values[j])
}

func newLTable(acap int, hcap int) *LTable {
	if acap < 0 {
		acap = 0
	}
	if hcap < 0 {
		hcap = 0
	}
	tb := &LTable{}
	tb.Metatable = LNil
	tb.Immutable = false
	if acap != 0 {
		tb.Array = make([]LValue, 0, acap)
	}
	if hcap != 0 {
		tb.Strdict = make(map[string]LValue, hcap)
	}
	// LAZY OPTIMIZATION: Keys, K2i, and Dict are nil until actually needed
	// Keys/K2i only created when Next() is called
	// Dict only created when non-string keys are used
	return tb
}

func CreateTable(acap, hcap int) *LTable {
	return newLTable(acap, hcap)
}

// ensureIterationOrder creates Keys/K2i only when needed (Next() called)
func (tb *LTable) ensureIterationOrder() {
	if tb.Keys != nil {
		return // Already initialized
	}

	// Initialize iteration order tracking
	totalKeys := len(tb.Strdict)
	if tb.Dict != nil {
		totalKeys += len(tb.Dict)
	}

	tb.Keys = make([]LValue, 0, totalKeys)
	tb.K2i = make(map[LValue]int)

	// Add all existing string keys in arbitrary order (Lua compliant)
	for k := range tb.Strdict {
		lkey := LString(k)
		tb.K2i[lkey] = len(tb.Keys)
		tb.Keys = append(tb.Keys, lkey)
	}

	// Add all existing non-string keys (if Dict exists)
	if tb.Dict != nil {
		for k := range tb.Dict {
			tb.K2i[k] = len(tb.Keys)
			tb.Keys = append(tb.Keys, k)
		}
	}
}

// Len returns length of this LTable without using __len.
func (tb *LTable) Len() int {
	if tb.Array == nil {
		return 0
	}
	var prev = LNil
	for i := len(tb.Array) - 1; i >= 0; i-- {
		v := tb.Array[i]
		if prev == LNil && v != LNil {
			return i + 1
		}
		prev = v
	}
	return 0
}

// Append appends a given LValue to this LTable.
func (tb *LTable) Append(value LValue) bool {
	if tb.Immutable {
		return false
	}
	if value == LNil {
		return true
	}
	if tb.Array == nil {
		tb.Array = make([]LValue, 0, defaultArrayCap)
	}
	if len(tb.Array) == 0 || tb.Array[len(tb.Array)-1] != LNil {
		tb.Array = append(tb.Array, value)
	} else {
		i := len(tb.Array) - 2
		for ; i >= 0; i-- {
			if tb.Array[i] != LNil {
				break
			}
		}
		tb.Array[i+1] = value
	}
	return true
}

// Insert inserts a given LValue at position `i` in this table.
func (tb *LTable) Insert(i int, value LValue) bool {
	if tb.Immutable {
		return false
	}
	if tb.Array == nil {
		tb.Array = make([]LValue, 0, defaultArrayCap)
	}
	if i > len(tb.Array) {
		return tb.RawSetInt(i, value)
	}
	if i <= 0 {
		return tb.RawSet(LNumber(i), value)
	}
	i -= 1
	tb.Array = append(tb.Array, LNil)
	copy(tb.Array[i+1:], tb.Array[i:])
	tb.Array[i] = value
	return true
}

// MaxN returns a maximum number key that nil value does not exist before it.
func (tb *LTable) MaxN() int {
	if tb.Array == nil {
		return 0
	}
	for i := len(tb.Array) - 1; i >= 0; i-- {
		if tb.Array[i] != LNil {
			return i + 1
		}
	}
	return 0
}

// Remove removes from this table the element at a given position.
func (tb *LTable) Remove(pos int) (LValue, bool) {
	if tb.Immutable {
		return LNil, false
	}
	if tb.Array == nil {
		return LNil, true
	}
	larray := len(tb.Array)
	if larray == 0 {
		return LNil, true
	}
	i := pos - 1
	oldval := LNil
	switch {
	case i >= larray:
		// nothing to do
	case i == larray-1 || i < 0:
		oldval = tb.Array[larray-1]
		tb.Array = tb.Array[:larray-1]
	default:
		oldval = tb.Array[i]
		copy(tb.Array[i:], tb.Array[i+1:])
		tb.Array[larray-1] = nil
		tb.Array = tb.Array[:larray-1]
	}
	return oldval, true
}

// RawSet sets a given LValue to a given index without the __newindex metamethod.
// It is recommended to use `RawSetString` or `RawSetInt` for performance
// if you already know the given LValue is a string or number.
func (tb *LTable) RawSet(key LValue, value LValue) bool {
	if tb.Immutable {
		return false
	}
	switch v := key.(type) {
	case LNumber:
		if isArrayKey(v) {
			if tb.Array == nil {
				tb.Array = make([]LValue, 0, defaultArrayCap)
			}
			index := int(v) - 1
			alen := len(tb.Array)
			switch {
			case index == alen:
				tb.Array = append(tb.Array, value)
			case index > alen:
				for i := 0; i < (index - alen); i++ {
					tb.Array = append(tb.Array, LNil)
				}
				tb.Array = append(tb.Array, value)
			case index < alen:
				tb.Array[index] = value
			}
			return true
		}
	case LInteger:
		iv := int(v)
		if iv > 0 && iv < MaxArrayIndex {
			if tb.Array == nil {
				tb.Array = make([]LValue, 0, defaultArrayCap)
			}
			index := iv - 1
			alen := len(tb.Array)
			switch {
			case index == alen:
				tb.Array = append(tb.Array, value)
			case index > alen:
				for i := 0; i < (index - alen); i++ {
					tb.Array = append(tb.Array, LNil)
				}
				tb.Array = append(tb.Array, value)
			case index < alen:
				tb.Array[index] = value
			}
			return true
		}
	case LString:
		return tb.RawSetString(string(v), value)
	}

	return tb.RawSetH(key, value)
}

// RawSetInt sets a given LValue at a position `key` without the __newindex metamethod.
func (tb *LTable) RawSetInt(key int, value LValue) bool {
	if tb.Immutable {
		return false
	}
	if key < 1 || key >= MaxArrayIndex {
		return tb.RawSetH(LNumber(key), value)
	}
	if tb.Array == nil {
		tb.Array = make([]LValue, 0, defaultArrayCap)
	}
	index := key - 1
	alen := len(tb.Array)
	switch {
	case index == alen:
		tb.Array = append(tb.Array, value)
	case index > alen:
		for i := 0; i < (index - alen); i++ {
			tb.Array = append(tb.Array, LNil)
		}
		tb.Array = append(tb.Array, value)
	case index < alen:
		tb.Array[index] = value
	}
	return true
}

// RawSetString sets a given LValue to a given string index without the __newindex metamethod.
func (tb *LTable) RawSetString(key string, value LValue) bool {
	if tb.Immutable {
		return false
	}

	if tb.Strdict == nil {
		tb.Strdict = make(map[string]LValue, defaultHashCap)
	}

	lkey := LString(key)
	if value == LNil {
		delete(tb.Strdict, key)
		// Only update Keys/K2i if they exist (lazy)
		if tb.Keys != nil {
			if idx, ok := tb.K2i[lkey]; ok {
				lastIdx := len(tb.Keys) - 1
				lastKey := tb.Keys[lastIdx]

				if idx < lastIdx {
					tb.Keys[idx] = lastKey
					tb.K2i[lastKey] = idx
				}

				tb.Keys = tb.Keys[:lastIdx]
				delete(tb.K2i, lkey)
			}
		}
	} else {
		tb.Strdict[key] = value
		// Only update Keys/K2i if they exist (lazy)
		if tb.Keys != nil {
			if _, ok := tb.K2i[lkey]; !ok {
				tb.K2i[lkey] = len(tb.Keys)
				tb.Keys = append(tb.Keys, lkey)
			}
		}
	}
	return true
}

// RawSetH sets a given LValue to a given index without the __newindex metamethod.
// OPTIMIZED: No longer creates Keys/K2i or Dict until actually needed
func (tb *LTable) RawSetH(key LValue, value LValue) bool {
	if tb.Immutable {
		return false
	}
	if s, ok := key.(LString); ok {
		return tb.RawSetString(string(s), value)
	}

	if value == LNil {
		// LAZY: Only delete if Dict exists
		if tb.Dict != nil {
			delete(tb.Dict, key)
		}
		// Only update Keys/K2i if they exist (lazy)
		if tb.Keys != nil {
			if idx, ok := tb.K2i[key]; ok {
				lastIdx := len(tb.Keys) - 1
				lastKey := tb.Keys[lastIdx]

				if idx < lastIdx {
					tb.Keys[idx] = lastKey
					tb.K2i[lastKey] = idx
				}

				tb.Keys = tb.Keys[:lastIdx]
				delete(tb.K2i, key)
			}
		}
	} else {
		// LAZY: Only create Dict when storing non-string, non-int key
		if tb.Dict == nil {
			tb.Dict = make(map[LValue]LValue, 8) // Start small for rare use case
		}
		tb.Dict[key] = value
		// Only update Keys/K2i if they exist (lazy)
		if tb.Keys != nil {
			if _, ok := tb.K2i[key]; !ok {
				tb.K2i[key] = len(tb.Keys)
				tb.Keys = append(tb.Keys, key)
			}
		}
	}
	return true
}

// RawGet returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGet(key LValue) LValue {
	switch v := key.(type) {
	case LNumber:
		if isArrayKey(v) {
			if tb.Array == nil {
				return LNil
			}
			index := int(v) - 1
			if index >= len(tb.Array) {
				return LNil
			}
			return tb.Array[index]
		}
	case LInteger:
		iv := int(v)
		if iv > 0 && iv < MaxArrayIndex {
			if tb.Array == nil {
				return LNil
			}
			index := iv - 1
			if index >= len(tb.Array) {
				return LNil
			}
			return tb.Array[index]
		}
	case LString:
		if tb.Strdict == nil {
			return LNil
		}
		if ret, ok := tb.Strdict[string(v)]; ok {
			return ret
		}
		return LNil
	}
	if tb.Dict == nil {
		return LNil
	}
	if v, ok := tb.Dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetInt returns an LValue at position `key` without __index metamethod.
func (tb *LTable) RawGetInt(key int) LValue {
	if tb.Array == nil {
		return LNil
	}
	index := int(key) - 1
	if index >= len(tb.Array) || index < 0 {
		return LNil
	}
	return tb.Array[index]
}

// RawGetH returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetH(key LValue) LValue {
	if s, sok := key.(LString); sok {
		if tb.Strdict == nil {
			return LNil
		}
		if v, vok := tb.Strdict[string(s)]; vok {
			return v
		}
		return LNil
	}
	if tb.Dict == nil {
		return LNil
	}
	if v, ok := tb.Dict[key]; ok {
		return v
	}
	return LNil
}

// RawGetString returns an LValue associated with a given key without __index metamethod.
func (tb *LTable) RawGetString(key string) LValue {
	if tb.Strdict == nil {
		return LNil
	}
	if v, vok := tb.Strdict[key]; vok {
		return v
	}
	return LNil
}

// ForEach iterates over this table of elements, yielding each in turn to a given function.
func (tb *LTable) ForEach(cb func(LValue, LValue)) {
	if tb.Array != nil {
		for i, v := range tb.Array {
			if v != LNil {
				cb(LNumber(i+1), v)
			}
		}
	}
	if tb.Strdict != nil {
		for k, v := range tb.Strdict {
			if v != LNil {
				cb(LString(k), v)
			}
		}
	}
	if tb.Dict != nil {
		for k, v := range tb.Dict {
			if v != LNil {
				cb(k, v)
			}
		}
	}
}

// Next is equivalent to lua_next ( http://www.lua.org/manual/5.1/manual.html#lua_next ).
func (tb *LTable) Next(key LValue) (LValue, LValue) {
	// Lazy initialization - only create ordering when Next() is actually used
	tb.ensureIterationOrder()

	init := false
	if key == LNil {
		key = LNumber(0)
		init = true
	}

	if init || key != LNumber(0) {
		if kv, ok := key.(LNumber); ok && IsIntegerValue(kv) && int(kv) >= 0 && kv < LNumber(MaxArrayIndex) {
			index := int(kv)
			if tb.Array != nil {
				for ; index < len(tb.Array); index++ {
					if v := tb.Array[index]; v != LNil {
						return LNumber(index + 1), v
					}
				}
			}
			if tb.Array == nil || index == len(tb.Array) {
				if len(tb.Keys) == 0 {
					return LNil, LNil
				}
				key = tb.Keys[0]
				if v := tb.RawGetH(key); v != LNil {
					return key, v
				}
			}
		}
	}

	for i := tb.K2i[key] + 1; i < len(tb.Keys); i++ {
		key := tb.Keys[i]
		if v := tb.RawGetH(key); v != LNil {
			return key, v
		}
	}
	return LNil, LNil
}
