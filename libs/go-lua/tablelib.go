package lua

import "sort"

var tableFuncs = map[string]LGoFunc{
	"getn":     tableGetN,
	"concat":   tableConcat,
	"insert":   tableInsert,
	"maxn":     tableMaxN,
	"remove":   tableRemove,
	"sort":     tableSort,
	"create":   tableCreate,
	"freeze":   tableMakeImmutable,
	"isfrozen": tableIsImmutable,
	"unpack":   baseUnpack,
}

func OpenTable(L *LState) int {
	mod := L.RegisterGoModule(TabLibName, tableFuncs).(*LTable)
	L.Push(mod)
	return 1
}

func tableSort(L *LState) int {
	tbl := L.CheckTable(1)
	if tbl.Immutable {
		L.RaiseError("attempt to sort Immutable table")
		return 0
	}
	sorter := lValueArraySorter{L, nil, tbl.Array}
	if L.GetTop() != 1 {
		sorter.Fn = L.CheckFunction(2)
	}
	sort.Sort(sorter)
	return 0
}

func tableCreate(L *LState) int {
	acap := L.CheckInt(1)
	hcap := L.CheckInt(2)
	tbl := CreateTable(acap, hcap)
	L.Push(tbl)
	return 1
}

func tableMakeImmutable(L *LState) int {
	tbl := L.CheckTable(1)
	tbl.Immutable = true
	L.Push(tbl)
	return 1
}

func tableIsImmutable(L *LState) int {
	tbl := L.CheckTable(1)
	L.Push(LBool(tbl.Immutable))
	return 1
}

func tableGetN(L *LState) int {
	L.Push(LNumber(L.CheckTable(1).Len()))
	return 1
}

func tableMaxN(L *LState) int {
	L.Push(LNumber(L.CheckTable(1).MaxN()))
	return 1
}

func tableRemove(L *LState) int {
	tbl := L.CheckTable(1)
	var removed LValue
	var success bool
	if L.GetTop() == 1 {
		removed, success = tbl.Remove(-1)
	} else {
		removed, success = tbl.Remove(L.CheckInt(2))
	}
	if !success {
		L.RaiseError("attempt to remove from Immutable table")
		return 0
	}
	L.Push(removed)
	return 1
}

func tableConcat(L *LState) int {
	tbl := L.CheckTable(1)
	sep := LString(L.OptString(2, ""))
	i := L.OptInt(3, 1)
	j := L.OptInt(4, tbl.Len())
	if L.GetTop() == 3 {
		if i > tbl.Len() || i < 1 {
			L.Push(emptyLString)
			return 1
		}
	}
	i = intMax(intMin(i, tbl.Len()), 1)
	j = intMin(intMin(j, tbl.Len()), tbl.Len())
	if i > j {
		L.Push(emptyLString)
		return 1
	}
	retbottom := L.GetTop()
	for ; i <= j; i++ {
		v := tbl.RawGetInt(i)
		if !LVCanConvToString(v) {
			L.RaiseError("invalid value (%s) at index %d in table for concat", v.Type().String(), i)
		}
		L.Push(v)
		if i != j {
			L.Push(sep)
		}
	}
	L.Push(stringConcat(L, L.GetTop()-retbottom, L.reg.Top()-1))
	return 1
}

func tableInsert(L *LState) int {
	tbl := L.CheckTable(1)
	nargs := L.GetTop()
	if nargs == 1 {
		L.RaiseError("wrong number of arguments")
	}

	var success bool
	if L.GetTop() == 2 {
		success = tbl.Append(L.Get(2))
	} else {
		success = tbl.Insert(L.CheckInt(2), L.CheckAny(3))
	}
	if !success {
		L.RaiseError("attempt to insert into Immutable table")
	}
	return 0
}
