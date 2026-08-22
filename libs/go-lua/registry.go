package lua

/* registry {{{ */

type registryHandler interface {
	registryOverflow()
}
type registry struct {
	array   []LValue
	top     int
	growBy  int
	maxSize int
	handler registryHandler
}

func newRegistry(handler registryHandler, initialSize int, growBy int, maxSize int) *registry {
	return &registry{make([]LValue, initialSize), 0, growBy, maxSize, handler}
}

func (rg *registry) resize(requiredSize int) bool { // +inline-start
	newSize := requiredSize + rg.growBy // give some padding
	if newSize > rg.maxSize {
		newSize = rg.maxSize
	}
	if newSize < requiredSize {
		rg.handler.registryOverflow()
		return false
	}
	rg.forceResize(newSize)
	return true
} // +inline-end

func (rg *registry) forceResize(newSize int) {
	newSlice := make([]LValue, newSize)
	copy(newSlice, rg.array[:rg.top]) // should we copy the area beyond top? there shouldn't be any valid values there so it shouldn't be necessary.
	rg.array = newSlice
}

func (rg *registry) SetTop(topi int) { // +inline-start
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := topi
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	oldtopi := rg.top
	rg.top = topi
	for i := oldtopi; i < rg.top; i++ {
		rg.array[i] = LNil
	}
	// values beyond top don't need to be valid LValues, so setting them to nil is fine
	// setting them to nil rather than LNil lets us invoke the golang memclr opto
	if rg.top < oldtopi {
		nilRange := rg.array[rg.top:oldtopi]
		for i := range nilRange {
			nilRange[i] = nil
		}
	}
	//for i := rg.top; i < oldtop; i++ {
	//	rg.Array[i] = LNil
	//}
} // +inline-end

func (rg *registry) Top() int {
	return rg.top
}

func (rg *registry) Push(v LValue) {
	newSize := rg.top + 1
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := newSize
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	rg.array[rg.top] = v
	rg.top++
}

func (rg *registry) Pop() LValue {
	v := rg.array[rg.top-1]
	rg.array[rg.top-1] = LNil
	rg.top--
	if v == nil {
		return LNil
	}
	return v
}

func (rg *registry) Get(reg int) LValue {
	v := rg.array[reg]
	if v == nil {
		return LNil
	}
	return v
}

// CopyRange will move a section of values from index `start` to index `regv`
// It will move `n` values.
// `limit` specifies the maximum end range that can be copied from. If it's set to -1, then it defaults to stopping at
// the top of the registry (values beyond the top are not initialized, so if specifying an alternative `limit` you should
// pass a value <= rg.top.
// If start+n is beyond the limit, then nil values will be copied to the destination slots.
// After the copy, the registry is truncated to be at the end of the copied range, ie the original of the copied values
// are nilled out. (So top will be regv+n)
// CopyRange should ideally be renamed to MoveRange.
func (rg *registry) CopyRange(regv, start, limit, n int) { // +inline-start
	newSize := regv + n
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := newSize
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	if limit == -1 || limit > rg.top {
		limit = rg.top
	}
	for i := 0; i < n; i++ {
		srcIdx := start + i
		if srcIdx >= limit || srcIdx < 0 {
			rg.array[regv+i] = LNil
		} else {
			v := rg.array[srcIdx]
			if v == nil {
				v = LNil
			}
			rg.array[regv+i] = v
		}
	}

	// values beyond top don't need to be valid LValues, so setting them to nil is fine
	// setting them to nil rather than LNil lets us invoke the golang memclr opto
	oldtop := rg.top
	rg.top = regv + n
	if rg.top < oldtop {
		nilRange := rg.array[rg.top:oldtop]
		for i := range nilRange {
			nilRange[i] = nil
		}
	}
} // +inline-end

// FillNil fills the registry with nil values from regm to regm+n and then sets the registry top to regm+n
func (rg *registry) FillNil(regm, n int) { // +inline-start
	newSize := regm + n
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := newSize
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	for i := 0; i < n; i++ {
		rg.array[regm+i] = LNil
	}
	// values beyond top don't need to be valid LValues, so setting them to nil is fine
	// setting them to nil rather than LNil lets us invoke the golang memclr opto
	oldtop := rg.top
	rg.top = regm + n
	if rg.top < oldtop {
		nilRange := rg.array[rg.top:oldtop]
		for i := range nilRange {
			nilRange[i] = nil
		}
	}
} // +inline-end

func (rg *registry) Insert(value LValue, reg int) {
	top := rg.Top()
	if reg >= top {
		// this section is inlined by go-inline
		// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
		{
			regi := reg
			vali := value
			newSize := regi + 1
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
			{
				requiredSize := newSize
				if requiredSize > cap(rg.array) {
					rg.resize(requiredSize)
				}
			}
			rg.array[regi] = vali
			if regi >= rg.top {
				rg.top = regi + 1
			}
		}
		return
	}
	top--
	for ; top >= reg; top-- {
		// this section is inlined by go-inline
		// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
		{
			regi := top + 1
			vali := rg.Get(top)
			newSize := regi + 1
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
			{
				requiredSize := newSize
				if requiredSize > cap(rg.array) {
					rg.resize(requiredSize)
				}
			}
			rg.array[regi] = vali
			if regi >= rg.top {
				rg.top = regi + 1
			}
		}
	}
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
	{
		regi := reg
		vali := value
		newSize := regi + 1
		// this section is inlined by go-inline
		// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
		{
			requiredSize := newSize
			if requiredSize > cap(rg.array) {
				rg.resize(requiredSize)
			}
		}
		rg.array[regi] = vali
		if regi >= rg.top {
			rg.top = regi + 1
		}
	}
}

func (rg *registry) Set(regi int, vali LValue) { // +inline-start
	newSize := regi + 1
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := newSize
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	rg.array[regi] = vali
	if regi >= rg.top {
		rg.top = regi + 1
	}
} // +inline-end

func (rg *registry) SetNumber(regi int, vali LNumber) { // +inline-start
	newSize := regi + 1
	// this section is inlined by go-inline
	// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
	{
		requiredSize := newSize
		if requiredSize > cap(rg.array) {
			rg.resize(requiredSize)
		}
	}
	rg.array[regi] = lnumberToValue(vali)
	if regi >= rg.top {
		rg.top = regi + 1
	}
} // +inline-end

func (rg *registry) IsFull() bool {
	return rg.top >= cap(rg.array)
}

/* }}} */
