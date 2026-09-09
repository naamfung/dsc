package lua

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
)

func mainLoop(L *LState, baseframe *callFrame) {
	// Set background context and nil done channel for fast path
	L.ctx = context.Background()
	L.ctxDone = nil
	mainLoopWithContext(L, baseframe)
}

func mainLoopWithContext(L *LState, baseframe *callFrame) {
	var inst uint32
	var cf *callFrame

	if L.stack.IsEmpty() {
		return
	}

	L.currentFrame = L.stack.Last()
	if L.currentFrame.GoFunc != nil || (L.currentFrame.Fn != nil && L.currentFrame.Fn.IsG) {
		if callGFunction(L, false) {
			return
		}
		if baseframe != nil {
			return
		}
		if L.stack.IsEmpty() {
			return
		}
		L.currentFrame = L.stack.Last()
		if L.currentFrame.GoFunc != nil || (L.currentFrame.Fn != nil && L.currentFrame.Fn.IsG) {
			mainLoopWithContext(L, baseframe)
			return
		}
	}

	ctxDone := L.ctxDone
	if ctxDone == nil && L.ctx != nil {
		ctxDone = L.ctx.Done()
	}

	// checkCtx is inlined at strategic points: backward jumps, loops, and calls.
	// This reduces overhead vs checking every opcode while still catching
	// infinite loops and long-running code.
	checkCtx := func() bool {
		if ctxDone != nil {
			select {
			case <-ctxDone:
				L.RaiseError(L.ctx.Err().Error())
				return true
			default:
			}
		}
		return false
	}

	for {
		cf = L.currentFrame
		inst = cf.Fn.Proto.Code[cf.Pc]
		cf.Pc++

		// Handle yield continuation: when an opcode's inner call yielded and has
		// now completed, finish the originating opcode's post-call work. Only fires
		// when the current frame is the one that owns the continuation.
		if L.yieldCont != 0 && cf.Idx == L.yieldContIdx {
			handleYieldContinuation(L, cf, inst)
			continue
		}

		// Note: Some opcodes (CALL, TAILCALL, RETURN) may need to `return` from mainLoop
		// Others just `continue` to next instruction
		switch int(inst >> 26) {
		case OP_MOVE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			v := reg.Get(int(lbase) + B)
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_MOVEN:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			v := reg.Get(int(lbase) + B)
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := int(lbase) + A
				vali := v
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
			code := cf.Fn.Proto.Code
			pc := cf.Pc
			for i := 0; i < C; i++ {
				inst = code[pc]
				pc++
				A = int(inst>>18) & 0xff //GETA
				B = int(inst & 0x1ff)    //GETB
				v := reg.Get(int(lbase) + B)
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := int(lbase) + A
					vali := v
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
			cf.Pc = pc

		case OP_LOADK:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Bx := int(inst & 0x3ffff) //GETBX
			v := cf.Fn.Proto.Constants[Bx]
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_LOADBOOL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			if B != 0 {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := LTrue
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
			} else {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := LFalse
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
			if C != 0 {
				cf.Pc++
			}

		case OP_LOADNIL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			for i := RA; i <= int(lbase)+B; i++ {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := i
					vali := LNil
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

		case OP_GETUPVAL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			v := cf.Fn.Upvalues[B].Value()
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_GETGLOBAL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Bx := int(inst & 0x3ffff) //GETBX
			//reg.Set(RA, L.getField(cf.Fn.Env, cf.Fn.Proto.Constants[Bx]))
			v := L.getFieldString(cf.Fn.Env, cf.Fn.Proto.stringConstants[Bx])
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContGetField
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_LOADTYPE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Bx := int(inst & 0x3ffff) //GETBX
			name := cf.Fn.Proto.stringConstants[Bx]
			v := cf.Fn.Proto.runtimeTypeValueByName(name)
			if v == nil {
				L.RaiseError("unknown type %s", name)
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_GETTABLE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			v := L.getField(reg.Get(int(lbase)+B), L.rkValue(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContGetField
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_GETTABLEKS:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			v := L.getFieldString(reg.Get(int(lbase)+B), L.rkString(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContGetField
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_SETGLOBAL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Bx := int(inst & 0x3ffff) //GETBX
			value := reg.Get(RA)
			L.setFieldString(cf.Fn.Env, cf.Fn.Proto.stringConstants[Bx], value)
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContSetField
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}

		case OP_SETUPVAL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			value := reg.Get(RA)
			cf.Fn.Upvalues[B].SetValue(value)

		case OP_SETTABLE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			L.setField(reg.Get(RA), L.rkValue(B), L.rkValue(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContSetField
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}

		case OP_SETTABLEKS:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			L.setFieldString(reg.Get(RA), L.rkString(B), L.rkValue(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContSetField
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}

		case OP_NEWTABLE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			v := newLTable(B, C)
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_SELF:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			selfobj := reg.Get(int(lbase) + B)
			v := L.getFieldString(selfobj, L.rkString(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContSelf
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA + 1
				vali := selfobj
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

		case OP_ADD:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			// Inline rkValue(B)
			var lhs LValue
			if (B & opBitRk) != 0 {
				lhs = cf.Fn.Proto.Constants[B & ^opBitRk]
			} else {
				lhs = reg.array[int(lbase)+B]
			}
			// Inline rkValue(C)
			var rhs LValue
			if (C & opBitRk) != 0 {
				rhs = cf.Fn.Proto.Constants[C & ^opBitRk]
			} else {
				rhs = reg.array[int(lbase)+C]
			}
			// Fast path: both integers
			if lhsI, ok1 := lhs.(LInteger); ok1 {
				if rhsI, ok2 := rhs.(LInteger); ok2 {
					v := lintegerToValue(lhsI + rhsI)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Fast path: both numbers
			if lhsN, ok1 := lhs.(LNumber); ok1 {
				if rhsN, ok2 := rhs.(LNumber); ok2 {
					v := lnumberToValue(lhsN + rhsN)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Mixed or slow path
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := lnumberToValue(v1 + v2)
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			} else {
				v := objectArith(L, OP_ADD, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			}

		case OP_SUB:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			// Inline rkValue(B)
			var lhs LValue
			if (B & opBitRk) != 0 {
				lhs = cf.Fn.Proto.Constants[B & ^opBitRk]
			} else {
				lhs = reg.array[int(lbase)+B]
			}
			// Inline rkValue(C)
			var rhs LValue
			if (C & opBitRk) != 0 {
				rhs = cf.Fn.Proto.Constants[C & ^opBitRk]
			} else {
				rhs = reg.array[int(lbase)+C]
			}
			// Fast path: both integers
			if lhsI, ok1 := lhs.(LInteger); ok1 {
				if rhsI, ok2 := rhs.(LInteger); ok2 {
					v := lintegerToValue(lhsI - rhsI)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Fast path: both numbers
			if lhsN, ok1 := lhs.(LNumber); ok1 {
				if rhsN, ok2 := rhs.(LNumber); ok2 {
					v := lnumberToValue(lhsN - rhsN)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Mixed or slow path
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := lnumberToValue(v1 - v2)
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			} else {
				v := objectArith(L, OP_SUB, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			}

		case OP_MUL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			// Inline rkValue(B)
			var lhs LValue
			if (B & opBitRk) != 0 {
				lhs = cf.Fn.Proto.Constants[B & ^opBitRk]
			} else {
				lhs = reg.array[int(lbase)+B]
			}
			// Inline rkValue(C)
			var rhs LValue
			if (C & opBitRk) != 0 {
				rhs = cf.Fn.Proto.Constants[C & ^opBitRk]
			} else {
				rhs = reg.array[int(lbase)+C]
			}
			// Fast path: both integers
			if lhsI, ok1 := lhs.(LInteger); ok1 {
				if rhsI, ok2 := rhs.(LInteger); ok2 {
					v := lintegerToValue(lhsI * rhsI)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Fast path: both numbers
			if lhsN, ok1 := lhs.(LNumber); ok1 {
				if rhsN, ok2 := rhs.(LNumber); ok2 {
					v := lnumberToValue(lhsN * rhsN)
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					continue
				}
			}
			// Mixed or slow path
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := lnumberToValue(v1 * v2)
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			} else {
				v := objectArith(L, OP_MUL, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				newSize := RA + 1
				if newSize > cap(reg.array) {
					reg.resize(newSize)
				}
				reg.array[RA] = v
				if RA >= reg.top {
					reg.top = RA + 1
				}
			}

		case OP_DIV:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := numberArith(L, OP_DIV, v1, v2)
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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
				}
			} else {
				v := objectArith(L, OP_DIV, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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

		case OP_MOD:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := numberArith(L, OP_MOD, v1, v2)
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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
				}
			} else {
				v := objectArith(L, OP_MOD, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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

		case OP_POW:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			v1, ok1 := toNumber(lhs)
			v2, ok2 := toNumber(rhs)
			if ok1 && ok2 {
				v := numberArith(L, OP_POW, v1, v2)
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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
				}
			} else {
				v := objectArith(L, OP_POW, lhs, rhs)
				if L.yieldState != yieldNone {
					L.yieldCont = yieldContArith
					L.yieldContRA = int32(RA)
					L.yieldContIdx = cf.Idx
					cf.Pc--
					return
				}
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := v
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

		case OP_IDIV:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform arithmetic on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform arithmetic on a %s value", rhs.Type().String())
				continue
			}
			if r == 0 {
				L.RaiseError("attempt to divide by zero")
				continue
			}
			{
				rg := reg
				regi := RA
				// Floor division: rounds toward negative infinity (Lua 5.3 semantics)
				q := l / r
				if (l^r) < 0 && l%r != 0 {
					q--
				}
				vali := lintegerToValue(LInteger(q))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_BAND:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", rhs.Type().String())
				continue
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(l & r))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_BOR:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", rhs.Type().String())
				continue
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(l | r))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_BXOR:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", rhs.Type().String())
				continue
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(l ^ r))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_SHL:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", rhs.Type().String())
				continue
			}
			var result int64
			if r >= 64 || r < -63 {
				result = 0
			} else if r >= 0 {
				result = l << uint(r)
			} else {
				result = l >> uint(-r)
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(result))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_SHR:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			C := int(inst>>9) & 0x1ff
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			var l, r int64
			switch lv := lhs.(type) {
			case LInteger:
				l = int64(lv)
			case LNumber:
				l = int64(lv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", lhs.Type().String())
				continue
			}
			switch rv := rhs.(type) {
			case LInteger:
				r = int64(rv)
			case LNumber:
				r = int64(rv)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", rhs.Type().String())
				continue
			}
			var result int64
			if r >= 64 || r < -63 {
				result = 0
			} else if r >= 0 {
				result = l >> uint(r)
			} else {
				result = l << uint(-r)
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(result))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_UNM:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			unaryv := L.rkValue(B)
			if nm, ok := toNumber(unaryv); ok {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := lnumberToValue(-nm)
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
			} else {
				op := L.metaOp1(unaryv, "__unm")
				if op.Type() == LTFunction {
					reg.Push(op)
					reg.Push(unaryv)
					L.Call(1, 1)
					if L.yieldState != yieldNone {
						L.yieldCont = yieldContUnm
						L.yieldContRA = int32(RA)
						L.yieldContIdx = cf.Idx
						cf.Pc--
						return
					}
					// this section is inlined by go-inline
					// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
					{
						rg := reg
						regi := RA
						vali := reg.Pop()
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
				} else if str, ok1 := unaryv.(LString); ok1 {
					if num, err := parseNumber(string(str)); err == nil {
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
						{
							rg := reg
							regi := RA
							vali := -num
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
					} else {
						L.RaiseError("__unm undefined")
					}
				} else {
					L.RaiseError("__unm undefined")
				}
			}

		case OP_BNOT:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff
			RA := int(lbase) + A
			B := int(inst & 0x1ff)
			unaryv := L.rkValue(B)
			var n int64
			switch v := unaryv.(type) {
			case LInteger:
				n = int64(v)
			case LNumber:
				n = int64(v)
			default:
				L.RaiseError("attempt to perform bitwise operation on a %s value", unaryv.Type().String())
				continue
			}
			{
				rg := reg
				regi := RA
				vali := lintegerToValue(LInteger(^n))
				newSize := regi + 1
				if newSize > cap(rg.array) {
					rg.resize(newSize)
				}
				rg.array[regi] = vali
				if regi >= rg.top {
					rg.top = regi + 1
				}
			}

		case OP_NOT:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			if LVIsFalse(reg.Get(int(lbase) + B)) {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := LTrue
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
			} else {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := LFalse
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

		case OP_LEN:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			switch lv := L.rkValue(B).(type) {
			case LString:
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
				{
					rg := reg
					regi := RA
					vali := LNumber(len(lv))
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
				}
			default:
				op := L.metaOp1(lv, "__len")
				if op.Type() == LTFunction {
					reg.Push(op)
					reg.Push(lv)
					L.Call(1, 1)
					if L.yieldState != yieldNone {
						L.yieldCont = yieldContLen
						L.yieldContRA = int32(RA)
						L.yieldContIdx = cf.Idx
						cf.Pc--
						return
					}
					ret := reg.Pop()
					if ret.Type() == LTNumber {
						v, _ := ret.(LNumber)
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
						{
							rg := reg
							regi := RA
							vali := v
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
						}
					} else {
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
						{
							rg := reg
							regi := RA
							vali := ret
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
				} else if lv.Type() == LTTable {
					// this section is inlined by go-inline
					// source function is 'func (rg *registry) SetNumber(regi int, vali LNumber) ' in '_state.go'
					{
						rg := reg
						regi := RA
						vali := LNumber(lv.(*LTable).Len())
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
					}
				} else {
					L.RaiseError("__len undefined")
				}
			}

		case OP_CONCAT:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			RC := int(lbase) + C
			RB := int(lbase) + B
			v := stringConcat(L, RC-RB+1, RC)
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContConcat
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := v
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

		case OP_JMP:
			Sbx := int(inst&0x3ffff) - opMaxArgSbx //GETSBX
			cf.Pc += int32(Sbx)
			if Sbx < 0 && checkCtx() {
				return
			}

		case OP_EQ:
			A := int(inst>>18) & 0xff //GETA
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			ret := equals(L, L.rkValue(B), L.rkValue(C), false)
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContCompare
				L.yieldContRA = int32(A)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			v := 1
			if ret {
				v = 0
			}
			if v == A {
				cf.Pc++
			}

		case OP_LT:
			A := int(inst>>18) & 0xff //GETA
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			ret := lessThan(L, L.rkValue(B), L.rkValue(C))
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContCompare
				L.yieldContRA = int32(A)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			v := 1
			if ret {
				v = 0
			}
			if v == A {
				cf.Pc++
			}

		case OP_LE:
			A := int(inst>>18) & 0xff //GETA
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			lhs := L.rkValue(B)
			rhs := L.rkValue(C)
			ret := false

			if v1, ok1 := toNumber(lhs); ok1 {
				if v2, ok2 := toNumber(rhs); ok2 {
					ret = v1 <= v2
				} else {
					L.RaiseError("attempt to compare %v with %v", lhs.Type().String(), rhs.Type().String())
				}
			} else {
				if lhs.Type() != rhs.Type() {
					L.RaiseError("attempt to compare %v with %v", lhs.Type().String(), rhs.Type().String())
				}
				switch lhs.Type() {
				case LTString:
					ret = strCmp(string(lhs.(LString)), string(rhs.(LString))) <= 0
				case LTType:
					ret = TypeIsSubtype(lhs.(*LType), rhs.(*LType))
				default:
					switch objectRational(L, lhs, rhs, "__le") {
					case 1:
						ret = true
					case 0:
						ret = false
					case -2:
						L.yieldCont = yieldContCompare
						L.yieldContRA = int32(A)
						L.yieldContIdx = cf.Idx
						cf.Pc--
						return
					default:
						ret = !objectRationalWithError(L, rhs, lhs, "__lt")
						if L.yieldState != yieldNone {
							L.yieldCont = yieldContCompare
							L.yieldContRA = int32(A)
							L.yieldContIdx = cf.Idx
							cf.Pc--
							return
						}
					}
				}
			}

			v := 1
			if ret {
				v = 0
			}
			if v == A {
				cf.Pc++
			}

		case OP_TEST:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			C := int(inst>>9) & 0x1ff //GETC
			if LVAsBool(reg.Get(RA)) == (C == 0) {
				cf.Pc++
			}

		case OP_TESTSET:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			if value := reg.Get(int(lbase) + B); LVAsBool(value) != (C == 0) {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA
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
			} else {
				cf.Pc++
			}

		case OP_CALL:
			if checkCtx() {
				return
			}
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			nargs := B - 1
			if B == 0 {
				nargs = reg.Top() - (RA + 1)
			}
			lv := reg.Get(RA)
			nret := C - 1

			// Fast path: LType call - native dispatch, no call frame
			if lt, ok := lv.(*LType); ok {
				L.typeCall(lt, RA, nargs, nret)
				continue
			}

			var callable *LFunction
			var goFunc LGoFunc
			var meta bool
			switch fn := lv.(type) {
			case *LFunction:
				callable = fn
			case LGoFunc:
				goFunc = fn
			default:
				callable, meta = L.metaCall(lv)
			}
			// this section is inlined by go-inline
			// source function is 'func (ls *LState) pushCallFrame(cf callFrame, fn LValue, meta bool) ' in '_state.go'
			{
				ls := L
				cf := callFrame{Fn: callable, GoFunc: goFunc, Pc: 0, Base: int32(RA), LocalBase: int32(RA + 1), ReturnBase: int32(RA), NArgs: int16(nargs), NRet: int16(nret), TailCall: 0}
				fn := lv
				if meta {
					cf.NArgs++
					ls.reg.Insert(fn, int(cf.LocalBase))
				}
				if cf.Fn == nil && cf.GoFunc == nil {
					ls.RaiseError("attempt to call a non-function object")
				}
				if ls.stack.IsFull() {
					ls.RaiseError("stack overflow")
				}
				ls.stack.Push(cf)
				newcf := ls.stack.Last()
				// this section is inlined by go-inline
				// source function is 'func (ls *LState) initCallFrame(cf *callFrame) ' in '_state.go'
				{
					cf := newcf
					if cf.GoFunc != nil || (cf.Fn != nil && cf.Fn.IsG) {
						ls.reg.SetTop(int(cf.LocalBase) + int(cf.NArgs))
					} else {
						proto := cf.Fn.Proto
						nargs := cf.NArgs
						np := int(proto.NumParameters)
						if int(nargs) < np {
							// default any missing arguments to nil
							newSize := int(cf.LocalBase) + np
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								rg := ls.reg
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := int(nargs); i < np; i++ {
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}
							nargs = int16(np)
							ls.reg.top = newSize
						}

						if (proto.IsVarArg & VarArgIsVarArg) == 0 {
							if int(nargs) < int(proto.NumUsedRegisters) {
								nargs = int16(int(proto.NumUsedRegisters))
							}
							newSize := int(cf.LocalBase) + int(nargs)
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								rg := ls.reg
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := np; i < int(nargs); i++ {
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}
							ls.reg.top = int(cf.LocalBase) + int(proto.NumUsedRegisters)
						} else {
							/* swap vararg positions:
									   closure
									   namedparam1 <- lbase
									   namedparam2
									   vararg1
									   vararg2

							           TO

									   closure
									   nil
									   nil
									   vararg1
									   vararg2
									   namedparam1 <- lbase
									   namedparam2
							*/
							ls.reg.SetTop(int(cf.LocalBase) + int(nargs) + np)
							for i := 0; i < np; i++ {
								//ls.reg.Set(cf.LocalBase+nargs+i, ls.reg.Get(cf.LocalBase+i))
								ls.reg.array[int(cf.LocalBase)+int(nargs)+i] = ls.reg.array[int(cf.LocalBase)+i]
								//ls.reg.Set(cf.LocalBase+i, LNil)
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}

							cf.LocalBase += int32(nargs)
							maxreg := int(cf.LocalBase) + int(proto.NumUsedRegisters)
							ls.reg.SetTop(maxreg)
						}
					}
				}
				ls.currentFrame = newcf
			}
			if (goFunc != nil || (callable != nil && callable.IsG)) && callGFunction(L, false) {
				return
			}

		case OP_TAILCALL:
			if checkCtx() {
				return
			}
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			nargs := B - 1
			if B == 0 {
				nargs = reg.Top() - (RA + 1)
			}
			lv := reg.Get(RA)
			if lt, ok := lv.(*LType); ok {
				L.typeCall(lt, RA, nargs, int(cf.NRet))
				b := int(cf.NRet) + 1
				if cf.NRet == MultRet {
					b = 0
				}
				if returnFromTailcall(L, baseframe, cf, RA, b) {
					return
				}
				continue
			}
			var callable *LFunction
			var goFunc LGoFunc
			var meta bool
			switch fn := lv.(type) {
			case *LFunction:
				callable = fn
			case LGoFunc:
				goFunc = fn
			default:
				callable, meta = L.metaCall(lv)
			}
			if callable == nil && goFunc == nil {
				L.RaiseError("attempt to call a non-function object")
			}
			// this section is inlined by go-inline
			// source function is 'func (ls *LState) closeUpvalues(idx int) ' in '_state.go'
			{
				ls := L
				idx := lbase
				if ls.uvcache != nil {
					var prev *Upvalue
					for uv := ls.uvcache; uv != nil; uv = uv.next {
						if uv.index >= int(idx) {
							if prev != nil {
								prev.next = nil
							} else {
								ls.uvcache = nil
							}
							uv.Close()
						}
						prev = uv
					}
				}
			}
			if goFunc != nil || (callable != nil && callable.IsG) {
				luaframe := cf
				L.pushCallFrame(callFrame{
					Fn:         callable,
					GoFunc:     goFunc,
					Pc:         0,
					Base:       int32(RA),
					LocalBase:  int32(RA + 1),
					ReturnBase: cf.ReturnBase,
					NArgs:      int16(nargs),
					NRet:       cf.NRet,
					TailCall:   0,
				}, lv, meta)
				if callGFunction(L, true) {
					return
				}
				if L.currentFrame == nil || luaframe == baseframe {
					return
				}
				// If tail call returned to a Go frame, check for continuation (e.g. pcall)
				if L.currentFrame.GoFunc != nil || (L.currentFrame.Fn != nil && L.currentFrame.Fn.IsG) {
					ext := L.getFrameExt(L.currentFrame)
					if ext != nil && ext.Continuation != nil {
						if callGFunction(L, false) {
							return
						}
					} else {
						return
					}
				}
			} else {
				base := cf.Base
				cf.Fn = callable
				cf.Pc = 0
				cf.Base = int32(RA)
				cf.LocalBase = int32(RA + 1)
				cf.NArgs = int16(nargs)
				cf.TailCall++
				lbase := cf.LocalBase
				if meta {
					cf.NArgs++
					L.reg.Insert(lv, int(cf.LocalBase))
				}
				// this section is inlined by go-inline
				// source function is 'func (ls *LState) initCallFrame(cf *callFrame) ' in '_state.go'
				{
					ls := L
					if cf.GoFunc != nil || (cf.Fn != nil && cf.Fn.IsG) {
						ls.reg.SetTop(int(cf.LocalBase) + int(cf.NArgs))
					} else {
						proto := cf.Fn.Proto
						nargs := cf.NArgs
						np := int(proto.NumParameters)
						if int(nargs) < np {
							// default any missing arguments to nil
							newSize := int(cf.LocalBase) + np
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								rg := ls.reg
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := int(nargs); i < np; i++ {
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}
							nargs = int16(np)
							ls.reg.top = newSize
						}

						if (proto.IsVarArg & VarArgIsVarArg) == 0 {
							if int(nargs) < int(proto.NumUsedRegisters) {
								nargs = int16(int(proto.NumUsedRegisters))
							}
							newSize := int(cf.LocalBase) + int(nargs)
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								rg := ls.reg
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := np; i < int(nargs); i++ {
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}
							ls.reg.top = int(cf.LocalBase) + int(proto.NumUsedRegisters)
						} else {
							/* swap vararg positions:
									   closure
									   namedparam1 <- lbase
									   namedparam2
									   vararg1
									   vararg2

							           TO

									   closure
									   nil
									   nil
									   vararg1
									   vararg2
									   namedparam1 <- lbase
									   namedparam2
							*/
							ls.reg.SetTop(int(cf.LocalBase) + int(nargs) + np)
							for i := 0; i < np; i++ {
								//ls.reg.Set(cf.LocalBase+nargs+i, ls.reg.Get(cf.LocalBase+i))
								ls.reg.array[int(cf.LocalBase)+int(nargs)+i] = ls.reg.array[int(cf.LocalBase)+i]
								//ls.reg.Set(cf.LocalBase+i, LNil)
								ls.reg.array[int(cf.LocalBase)+i] = LNil
							}

							cf.LocalBase += int32(nargs)
							maxreg := int(cf.LocalBase) + int(proto.NumUsedRegisters)
							ls.reg.SetTop(maxreg)
						}
					}
				}
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) CopyRange(regv, start, limit, n int) ' in '_state.go'
				{
					rg := L.reg
					regv := base
					start := RA
					n := reg.Top() - RA - 1
					newSize := int(regv) + n
					if newSize > cap(rg.array) {
						rg.resize(newSize)
					}
					limit := rg.top
					for i := 0; i < int(n); i++ {
						srcIdx := start + i
						if srcIdx >= limit || srcIdx < 0 {
							rg.array[int(regv)+i] = LNil
						} else {
							rg.array[int(regv)+i] = rg.array[srcIdx]
						}
					}

					// values beyond top don't need to be valid LValues, so setting them to nil is fine
					// setting them to nil rather than LNil lets us invoke the golang memclr opto
					oldtop := rg.top
					rg.top = int(regv) + int(n)
					if rg.top < oldtop {
						nilRange := rg.array[rg.top:oldtop]
						for i := range nilRange {
							nilRange[i] = nil
						}
					}
				}
				cf.Base = base
				cf.LocalBase = base + (cf.LocalBase - lbase + 1)
			}

		case OP_RETURN:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			// this section is inlined by go-inline
			// source function is 'func (ls *LState) closeUpvalues(idx int) ' in '_state.go'
			{
				ls := L
				idx := lbase
				if ls.uvcache != nil {
					var prev *Upvalue
					for uv := ls.uvcache; uv != nil; uv = uv.next {
						if uv.index >= int(idx) {
							if prev != nil {
								prev.next = nil
							} else {
								ls.uvcache = nil
							}
							uv.Close()
						}
						prev = uv
					}
				}
			}
			nret := B - 1
			if B == 0 {
				nret = reg.Top() - RA
			}
			n := cf.NRet
			if cf.NRet == MultRet {
				n = int16(nret)
			}

			if L.Parent != nil && L.stack.Sp() == 1 {
				// this section is inlined by go-inline
				// source function is 'func copyReturnValues(L *LState, regv, start, n, b int) ' in '_vm.go'
				{
					regv := reg.Top()
					start := RA
					b := B
					if b == 1 {
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) FillNil(regm, n int) ' in '_state.go'
						{
							rg := L.reg
							regm := regv
							newSize := regm + int(n)
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := 0; i < int(n); i++ {
								rg.array[int(regm)+i] = LNil
							}
							// values beyond top don't need to be valid LValues, so setting them to nil is fine
							// setting them to nil rather than LNil lets us invoke the golang memclr opto
							oldtop := rg.top
							rg.top = int(regm) + int(n)
							if rg.top < oldtop {
								nilRange := rg.array[rg.top:oldtop]
								for i := range nilRange {
									nilRange[i] = nil
								}
							}
						}
					} else {
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) CopyRange(regv, start, limit, n int) ' in '_state.go'
						{
							rg := L.reg
							newSize := regv + int(n)
							if newSize > cap(rg.array) {
								rg.resize(newSize)
							}
							limit := rg.top
							for i := 0; i < int(n); i++ {
								srcIdx := start + i
								if srcIdx >= limit || srcIdx < 0 {
									rg.array[int(regv)+i] = LNil
								} else {
									rg.array[int(regv)+i] = rg.array[srcIdx]
								}
							}

							// values beyond top don't need to be valid LValues, so setting them to nil is fine
							// setting them to nil rather than LNil lets us invoke the golang memclr opto
							oldtop := rg.top
							rg.top = int(regv) + int(n)
							if rg.top < oldtop {
								nilRange := rg.array[rg.top:oldtop]
								for i := range nilRange {
									nilRange[i] = nil
								}
							}
						}
						if b > 1 && int(n) > (b-1) {
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) FillNil(regm, n int) ' in '_state.go'
							{
								rg := L.reg
								regm := regv + b - 1
								n := int(n) - (b - 1)
								newSize := regm + n
								// this section is inlined by go-inline
								// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
								{
									requiredSize := newSize
									if requiredSize > cap(rg.array) {
										rg.resize(requiredSize)
									}
								}
								for i := 0; i < int(n); i++ {
									rg.array[int(regm)+i] = LNil
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
							}
						}
					}
				}
				switchToParentThread(L, int(n), false, true)
				return
			}
			islast := baseframe == L.stack.Pop() || L.stack.IsEmpty()
			// this section is inlined by go-inline
			// source function is 'func copyReturnValues(L *LState, regv, start, n, b int) ' in '_vm.go'
			{
				regv := cf.ReturnBase
				start := RA
				b := B
				if b == 1 {
					// this section is inlined by go-inline
					// source function is 'func (rg *registry) FillNil(regm, n int) ' in '_state.go'
					{
						rg := L.reg
						regm := regv
						newSize := int(regm) + int(n)
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
						{
							requiredSize := newSize
							if requiredSize > cap(rg.array) {
								rg.resize(requiredSize)
							}
						}
						for i := 0; i < int(n); i++ {
							rg.array[int(regm)+i] = LNil
						}
						// values beyond top don't need to be valid LValues, so setting them to nil is fine
						// setting them to nil rather than LNil lets us invoke the golang memclr opto
						oldtop := rg.top
						rg.top = int(regm) + int(n)
						if rg.top < oldtop {
							nilRange := rg.array[rg.top:oldtop]
							for i := range nilRange {
								nilRange[i] = nil
							}
						}
					}
				} else {
					// this section is inlined by go-inline
					// source function is 'func (rg *registry) CopyRange(regv, start, limit, n int) ' in '_state.go'
					{
						rg := L.reg
						newSize := int(regv) + int(n)
						if newSize > cap(rg.array) {
							rg.resize(newSize)
						}
						limit := rg.top
						for i := 0; i < int(n); i++ {
							srcIdx := start + i
							if srcIdx >= limit || srcIdx < 0 {
								rg.array[int(regv)+i] = LNil
							} else {
								rg.array[int(regv)+i] = rg.array[srcIdx]
							}
						}

						// values beyond top don't need to be valid LValues, so setting them to nil is fine
						// setting them to nil rather than LNil lets us invoke the golang memclr opto
						oldtop := rg.top
						rg.top = int(regv) + int(n)
						if rg.top < oldtop {
							nilRange := rg.array[rg.top:oldtop]
							for i := range nilRange {
								nilRange[i] = nil
							}
						}
					}
					if b > 1 && int(n) > (b-1) {
						// this section is inlined by go-inline
						// source function is 'func (rg *registry) FillNil(regm, n int) ' in '_state.go'
						{
							rg := L.reg
							regm := int(regv) + b - 1
							n := int(n) - (b - 1)
							newSize := regm + n
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) checkSize(requiredSize int) ' in '_state.go'
							{
								requiredSize := newSize
								if requiredSize > cap(rg.array) {
									rg.resize(requiredSize)
								}
							}
							for i := 0; i < int(n); i++ {
								rg.array[int(regm)+i] = LNil
							}
							// values beyond top don't need to be valid LValues, so setting them to nil is fine
							// setting them to nil rather than LNil lets us invoke the golang memclr opto
							oldtop := rg.top
							rg.top = int(regm) + int(n)
							if rg.top < oldtop {
								nilRange := rg.array[rg.top:oldtop]
								for i := range nilRange {
									nilRange[i] = nil
								}
							}
						}
					}
				}
			}
			L.currentFrame = L.stack.Last()
			if islast || L.currentFrame == nil {
				return
			}
			// Check if returning to a Go function
			if L.currentFrame.GoFunc != nil || (L.currentFrame.Fn != nil && L.currentFrame.Fn.IsG) {
				// If it has a continuation, call it to handle the return
				ext := L.getFrameExt(L.currentFrame)
				if ext != nil && ext.Continuation != nil {
					if callGFunction(L, false) {
						return
					}
				} else {
					return
				}
			}

		case OP_FORLOOP:
			if checkCtx() {
				return
			}
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			// Fast path: check if all values are integers
			initVal := reg.Get(RA)
			limitVal := reg.Get(RA + 1)
			stepVal := reg.Get(RA + 2)
			if initI, ok1 := initVal.(LInteger); ok1 {
				if limitI, ok2 := limitVal.(LInteger); ok2 {
					if stepI, ok3 := stepVal.(LInteger); ok3 {
						init := int64(initI) + int64(stepI)
						limit := int64(limitI)
						step := int64(stepI)
						v := lintegerToValue(LInteger(init))
						newSize := RA + 1
						if newSize > cap(reg.array) {
							reg.resize(newSize)
						}
						reg.array[RA] = v
						if RA >= reg.top {
							reg.top = RA + 1
						}
						if (step > 0 && init <= limit) || (step <= 0 && init >= limit) {
							Sbx := int(inst&0x3ffff) - opMaxArgSbx
							cf.Pc += int32(Sbx)
							newSize := RA + 4
							if newSize > cap(reg.array) {
								reg.resize(newSize)
							}
							reg.array[RA+3] = v
							if RA+3 >= reg.top {
								reg.top = RA + 4
							}
						} else {
							topi := RA + 1
							if topi > cap(reg.array) {
								reg.resize(topi)
							}
							oldtopi := reg.top
							reg.top = topi
							for i := oldtopi; i < reg.top; i++ {
								reg.array[i] = LNil
							}
							if reg.top < oldtopi {
								nilRange := reg.array[reg.top:oldtopi]
								for i := range nilRange {
									nilRange[i] = nil
								}
							}
						}
						continue
					}
				}
			}
			// Slow path: use LNumber
			if init, ok1 := toNumber(initVal); ok1 {
				if limit, ok2 := toNumber(limitVal); ok2 {
					if step, ok3 := toNumber(stepVal); ok3 {
						init += step
						v := LNumber(init)
						{
							rg := reg
							regi := RA
							vali := v
							newSize := regi + 1
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
						}
						if (step > 0 && init <= limit) || (step <= 0 && init >= limit) {
							Sbx := int(inst&0x3ffff) - opMaxArgSbx //GETSBX
							cf.Pc += int32(Sbx)
							{
								rg := reg
								regi := RA + 3
								vali := v
								newSize := regi + 1
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
							}
						} else {
							// this section is inlined by go-inline
							// source function is 'func (rg *registry) SetTop(topi int) ' in '_state.go'
							{
								rg := reg
								topi := RA + 1
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
							}
						}
					} else {
						L.RaiseError("for statement step must be a number")
					}
				} else {
					L.RaiseError("for statement limit must be a number")
				}
			} else {
				L.RaiseError("for statement init must be a number")
			}

		case OP_FORPREP:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Sbx := int(inst&0x3ffff) - opMaxArgSbx //GETSBX
			initVal := reg.Get(RA)
			stepVal := reg.Get(RA + 2)
			// Fast path: integer-only for loops
			if initI, ok1 := initVal.(LInteger); ok1 {
				if stepI, ok2 := stepVal.(LInteger); ok2 {
					result := int64(initI) - int64(stepI)
					v := lintegerToValue(LInteger(result))
					newSize := RA + 1
					if newSize > cap(reg.array) {
						reg.resize(newSize)
					}
					reg.array[RA] = v
					if RA >= reg.top {
						reg.top = RA + 1
					}
					cf.Pc += int32(Sbx)
					continue
				}
			}
			// Slow path: use LNumber
			if init, ok1 := toNumber(initVal); ok1 {
				if step, ok2 := toNumber(stepVal); ok2 {
					{
						rg := reg
						regi := RA
						vali := LNumber(init - step)
						newSize := regi + 1
						if newSize > cap(rg.array) {
							rg.resize(newSize)
						}
						rg.array[regi] = lnumberToValue(vali)
						if regi >= rg.top {
							rg.top = regi + 1
						}
					}
				} else {
					L.RaiseError("for statement step must be a number")
				}
			} else {
				L.RaiseError("for statement init must be a number")
			}
			cf.Pc += int32(Sbx)

		case OP_TFORLOOP:
			if checkCtx() {
				return
			}
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			C := int(inst>>9) & 0x1ff //GETC
			nret := C
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) SetTop(topi int) ' in '_state.go'
			{
				rg := reg
				topi := RA + 3 + 2
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
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA + 3 + 2
				vali := reg.Get(RA + 2)
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
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA + 3 + 1
				vali := reg.Get(RA + 1)
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
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA + 3
				vali := reg.Get(RA)
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
			L.callR(2, nret, RA+3)
			if L.yieldState != yieldNone {
				L.yieldCont = yieldContTForLoop
				L.yieldContRA = int32(RA)
				L.yieldContIdx = cf.Idx
				cf.Pc--
				return
			}
			if value := reg.Get(RA + 3); value != LNil {
				// this section is inlined by go-inline
				// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
				{
					rg := reg
					regi := RA + 2
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
				pc := cf.Fn.Proto.Code[cf.Pc]
				cf.Pc += int32(int(pc&0x3ffff) - opMaxArgSbx)
			}
			cf.Pc++

		case OP_SETLIST:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff)    //GETB
			C := int(inst>>9) & 0x1ff //GETC
			if C == 0 {
				C = int(cf.Fn.Proto.Code[cf.Pc])
				cf.Pc++
			}
			offset := (C - 1) * FieldsPerFlush
			table := reg.Get(RA).(*LTable)
			nelem := B
			if B == 0 {
				nelem = reg.Top() - RA - 1
			}
			for i := 1; i <= nelem; i++ {
				table.RawSetInt(offset+i, reg.Get(RA+i))
			}

		case OP_CLOSE:
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			// this section is inlined by go-inline
			// source function is 'func (ls *LState) closeUpvalues(idx int) ' in '_state.go'
			{
				ls := L
				idx := RA
				if ls.uvcache != nil {
					var prev *Upvalue
					for uv := ls.uvcache; uv != nil; uv = uv.next {
						if uv.index >= int(idx) {
							if prev != nil {
								prev.next = nil
							} else {
								ls.uvcache = nil
							}
							uv.Close()
						}
						prev = uv
					}
				}
			}

		case OP_CLOSURE:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			Bx := int(inst & 0x3ffff) //GETBX
			proto := cf.Fn.Proto.FunctionPrototypes[Bx]
			closure := newLFunctionL(proto, cf.Fn.Env, int(proto.NumUpvalues))
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) Set(regi int, vali LValue) ' in '_state.go'
			{
				rg := reg
				regi := RA
				vali := closure
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
			for i := 0; i < int(proto.NumUpvalues); i++ {
				inst = cf.Fn.Proto.Code[cf.Pc]
				cf.Pc++
				B := opGetArgB(inst)
				switch opGetOpCode(inst) {
				case OP_MOVE:
					closure.Upvalues[i] = L.findUpvalue(int(lbase) + B)
				case OP_GETUPVAL:
					closure.Upvalues[i] = cf.Fn.Upvalues[B]
				default:
				}
			}

		case OP_VARARG:
			reg := L.reg
			lbase := cf.LocalBase
			A := int(inst>>18) & 0xff //GETA
			RA := int(lbase) + A
			B := int(inst & 0x1ff) //GETB
			nparams := int(cf.Fn.Proto.NumParameters)
			nvarargs := int(cf.NArgs) - nparams
			if nvarargs < 0 {
				nvarargs = 0
			}
			nwant := B - 1
			if B == 0 {
				nwant = nvarargs
			}
			// this section is inlined by go-inline
			// source function is 'func (rg *registry) CopyRange(regv, start, limit, n int) ' in '_state.go'
			{
				rg := reg
				regv := RA
				start := int(cf.Base) + nparams + 1
				limit := int(cf.LocalBase)
				n := nwant
				newSize := regv + int(n)
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
				for i := 0; i < int(n); i++ {
					srcIdx := start + i
					if srcIdx >= limit || srcIdx < 0 {
						rg.array[int(regv)+i] = LNil
					} else {
						rg.array[int(regv)+i] = rg.array[srcIdx]
					}
				}

				// values beyond top don't need to be valid LValues, so setting them to nil is fine
				// setting them to nil rather than LNil lets us invoke the golang memclr opto
				oldtop := rg.top
				rg.top = int(regv) + int(n)
				if rg.top < oldtop {
					nilRange := rg.array[rg.top:oldtop]
					for i := range nilRange {
						nilRange[i] = nil
					}
				}
			}

		case OP_NOP:
			// No operation

		default:
			panic(fmt.Sprintf("unknown opcode: %d", int(inst>>26)))
		}
	}
}

func switchToParentThread(L *LState, nargs int, haserror bool, kill bool) {
	parent := L.Parent
	if parent == nil {
		L.RaiseError("can not yield from outside of a coroutine")
	}
	L.G.CurrentThread = parent
	L.Parent = nil
	if !L.wrapped {
		if haserror {
			parent.Push(LFalse)
		} else {
			parent.Push(LTrue)
		}
	}
	L.XMoveTo(parent, nargs)
	L.stack.Pop()
	offset := L.currentFrame.LocalBase - L.currentFrame.ReturnBase
	L.currentFrame = L.stack.Last()
	L.reg.SetTop(L.reg.Top() - int(offset))
	if kill {
		L.kill()
	}

	// For yield, mark as yielded (no panic needed - callers check L.yieldState).
	// Preserve yieldUser if already set by coYield before this call.
	if !haserror && !kill {
		if L.yieldState == yieldNone {
			L.yieldState = yieldSystem
		}
	}
}

func returnFromTailcall(L *LState, baseframe *callFrame, cf *callFrame, RA int, B int) bool {
	if L == nil || cf == nil {
		return true
	}
	reg := L.reg
	lbase := int(cf.LocalBase)
	L.closeUpvalues(lbase)

	nret := B - 1
	if B == 0 {
		nret = reg.Top() - RA
	}
	n := cf.NRet
	if cf.NRet == MultRet {
		n = int16(nret)
	}

	if L.Parent != nil && L.stack.Sp() == 1 {
		regv := reg.Top()
		if B == 1 {
			reg.FillNil(regv, int(n))
		} else {
			reg.CopyRange(regv, RA, -1, int(n))
			if B > 1 && int(n) > (B-1) {
				reg.FillNil(regv+B-1, int(n)-(B-1))
			}
		}
		switchToParentThread(L, int(n), false, true)
		return true
	}

	islast := baseframe == L.stack.Pop() || L.stack.IsEmpty()
	regv := int(cf.ReturnBase)
	if B == 1 {
		reg.FillNil(regv, int(n))
	} else {
		reg.CopyRange(regv, RA, -1, int(n))
		if B > 1 && int(n) > (B-1) {
			reg.FillNil(regv+B-1, int(n)-(B-1))
		}
	}

	L.currentFrame = L.stack.Last()
	if islast || L.currentFrame == nil {
		return true
	}
	if L.currentFrame.GoFunc != nil || (L.currentFrame.Fn != nil && L.currentFrame.Fn.IsG) {
		ext := L.getFrameExt(L.currentFrame)
		if ext != nil && ext.Continuation != nil {
			if callGFunction(L, false) {
				return true
			}
		} else {
			return true
		}
	}

	return false
}

// handleYieldContinuation finishes an opcode whose inner call yielded.
// The called function has completed and OP_RETURN placed the result at
// yieldContRB. We execute only the post-call logic of the originating opcode.
func handleYieldContinuation(L *LState, cf *callFrame, inst uint32) {
	contType := L.yieldCont
	ra := int(L.yieldContRA)
	rb := int(L.yieldContRB)
	reg := L.reg

	// Clear continuation state before executing post-call logic.
	L.yieldCont = yieldContNone
	L.yieldContRA = 0
	L.yieldContRB = 0
	L.yieldContIdx = 0

	switch contType {
	case yieldContGetField, yieldContArith, yieldContUnm, yieldContLen, yieldContConcat:
		// All these place a single return value at RA.
		v := reg.Get(rb)
		reg.Set(ra, v)

	case yieldContSetField:
		// No result to store, execution continues.

	case yieldContSelf:
		// OP_SELF: store method result at RA, self object at RA+1.
		v := reg.Get(rb)
		reg.Set(ra, v)
		// Re-extract B from instruction to get the self object.
		lbase := cf.LocalBase
		B := int(inst & 0x1ff) //GETB
		selfobj := reg.Get(int(lbase) + B)
		reg.Set(ra+1, selfobj)

	case yieldContCompare:
		// RA holds the A operand from the comparison instruction (not a register index).
		// The metamethod result is at rb. Evaluate as bool and apply the skip logic.
		v := reg.Get(rb)
		result := 1
		if LVAsBool(v) {
			result = 0
		}
		if result == int(ra) {
			cf.Pc++
		}

	case yieldContTForLoop:
		// OP_TFORLOOP: iterator results are at RA+3..RA+3+nret-1 (placed by callR/OP_RETURN).
		// Check first result: if nil, loop ends. Otherwise update control variable.
		if value := reg.Get(ra + 3); value != LNil {
			reg.Set(ra+2, value)
			// Read the JMP instruction that follows OP_TFORLOOP.
			pc := cf.Fn.Proto.Code[cf.Pc]
			cf.Pc += int32(int(pc&0x3ffff) - opMaxArgSbx)
		}
		cf.Pc++
	}
}

func callGFunction(L *LState, tailcall bool) bool {
	frame := L.currentFrame
	var gfnret int

	// Check if this is a resume with continuation (after yield)
	ext := L.getFrameExt(frame)
	if ext != nil && ext.Continuation != nil {
		cont := ext.Continuation
		ctx := ext.ContinuationCtx
		ext.Continuation = nil
		ext.ContinuationCtx = nil
		gfnret = cont(L, ctx, ResumeYield)
	} else if frame.GoFunc != nil {
		gfnret = frame.GoFunc(L)
	} else {
		gfnret = frame.Fn.GFunction(L)
	}
	if tailcall {
		L.currentFrame = L.removeCallerFrame()
	}

	if gfnret < 0 {
		// Only call switchToParentThread for the first yield in the chain.
		// Subsequent Go functions returning -1 (pcall, xpcall, coResume) detect
		// the yield via yieldState and propagate it without a second thread switch.
		if L.yieldState == yieldNone {
			switchToParentThread(L, L.GetTop(), false, false)
			// -2 = user yield (coroutine.yield), -1 = system yield (Go function)
			if gfnret == -2 {
				L.yieldState = yieldUser
			}
		}
		return true
	}

	wantret := frame.NRet
	if wantret == MultRet {
		wantret = int16(gfnret)
	}

	if tailcall && L.Parent != nil && L.stack.Sp() == 1 {
		switchToParentThread(L, int(wantret), false, true)
		return true
	}

	// this section is inlined by go-inline
	// source function is 'func (rg *registry) CopyRange(regv, start, limit, n int) ' in '_state.go'
	{
		rg := L.reg
		regv := frame.ReturnBase
		start := L.reg.Top() - gfnret
		n := wantret
		newSize := int(regv) + int(n)
		if newSize > cap(rg.array) {
			rg.resize(newSize)
		}
		limit := rg.top
		for i := 0; i < int(n); i++ {
			srcIdx := start + i
			if srcIdx >= limit || srcIdx < 0 {
				rg.array[int(regv)+i] = LNil
			} else {
				rg.array[int(regv)+i] = rg.array[srcIdx]
			}
		}

		// values beyond top don't need to be valid LValues, so setting them to nil is fine
		// setting them to nil rather than LNil lets us invoke the golang memclr opto
		oldtop := rg.top
		rg.top = int(regv) + int(n)
		if rg.top < oldtop {
			nilRange := rg.array[rg.top:oldtop]
			for i := range nilRange {
				nilRange[i] = nil
			}
		}
	}

	L.stack.Pop()
	L.currentFrame = L.stack.Last()
	return false
}

func threadRun(L *LState) {
	if L.stack.IsEmpty() {
		return
	}

	defer func() {
		if rcv := recover(); rcv != nil {
			var lv LValue
			if v, ok := rcv.(*ApiError); ok {
				lv = v.Object
				// Ensure *Error has its metatable set
				if e, ok := lv.(*Error); ok {
					SetErrorMetatable(L, e)
				}
			} else {
				lv = LString(fmt.Sprint(rcv))
			}

			// Check if there's a protected frame that should catch this error
			if handleProtectedError(L, lv, rcv) {
				// Error was handled by a protected call, continue execution
				// Recursive call sets up new defer/recover for subsequent errors
				threadRun(L)
				return
			}

			if parent := L.Parent; parent != nil {
				if L.wrapped {
					L.Push(lv)
					parent.Panic(L)
				} else {
					L.SetTop(0)
					L.Push(lv)
					switchToParentThread(L, 1, true, true)
				}
			} else {
				panic(rcv)
			}
		}
	}()
	L.mainLoop(L, nil)
}

// handleProtectedError searches for a protected (pcall) frame and handles the error.
// Returns true if error was handled, false if it should propagate.
func handleProtectedError(L *LState, errValue LValue, _ interface{}) bool {
	// Search up the call stack for a protected frame
	sp := L.stack.Sp()
	for i := sp - 1; i >= 0; i-- {
		frame := L.stack.At(i)
		if frame == nil {
			break
		}
		if frame.Protected {
			// Capture frame values before popping (frame memory may be reused after pop)
			returnBase := frame.ReturnBase
			var errFunc *LFunction
			if ext := L.getFrameExt(frame); ext != nil {
				errFunc = ext.ErrFunc
			}

			// Clear frame extensions for all frames being popped (including protected frame)
			for j := L.stack.Sp() - 1; j >= i; j-- {
				if f := L.stack.At(j); f != nil {
					L.clearFrameExt(f)
				}
			}
			for L.stack.Sp() > i+1 {
				L.stack.Pop()
			}
			L.stack.Pop()
			L.currentFrame = L.stack.Last()

			// Call error handler if present (xpcall)
			if errFunc != nil {
				L.Push(errFunc)
				L.Push(errValue)
				err := L.PCall(1, 1, nil)
				if err == nil {
					errValue = L.Get(-1)
					L.Pop(1)
				}
			}

			// If errValue is an *Error, ensure it has its metatable set
			if e, ok := errValue.(*Error); ok {
				SetErrorMetatable(L, e)
			}

			// Set up return values: false, error_message
			L.reg.Set(int(returnBase), LFalse)
			L.reg.Set(int(returnBase)+1, errValue)

			return true
		}
	}
	return false
}

func luaModulo(lhs, rhs LNumber) LNumber {
	flhs := float64(lhs)
	frhs := float64(rhs)
	v := math.Mod(flhs, frhs)
	if frhs > 0 && v < 0 || frhs < 0 && v > 0 {
		v += frhs
	}
	return LNumber(v)
}

func numberArith(_ *LState, opcode int, lhs, rhs LNumber) LNumber {
	switch opcode {
	case OP_ADD:
		return lhs + rhs
	case OP_SUB:
		return lhs - rhs
	case OP_MUL:
		return lhs * rhs
	case OP_DIV:
		return lhs / rhs
	case OP_MOD:
		return luaModulo(lhs, rhs)
	case OP_POW:
		flhs := float64(lhs)
		frhs := float64(rhs)
		return LNumber(math.Pow(flhs, frhs))
	default:
		panic("should not reach here")
	}
}

// toNumber extracts numeric value from LNumber or LInteger
func toNumber(v LValue) (LNumber, bool) {
	switch n := v.(type) {
	case LNumber:
		return n, true
	case LInteger:
		return LNumber(n), true
	}
	return 0, false
}

func objectArith(L *LState, opcode int, lhs, rhs LValue) LValue {
	event := ""
	switch opcode {
	case OP_ADD:
		event = "__add"
	case OP_SUB:
		event = "__sub"
	case OP_MUL:
		event = "__mul"
	case OP_DIV:
		event = "__div"
	case OP_MOD:
		event = "__mod"
	case OP_POW:
		event = "__pow"
	default:
	}
	op := L.metaOp2(lhs, rhs, event)
	if _, ok := op.(*LFunction); ok {
		L.reg.Push(op)
		L.reg.Push(lhs)
		L.reg.Push(rhs)

		L.Call(2, 1)
		if L.yieldState != yieldNone {
			return LNil
		}
		return L.reg.Pop()
	}
	if str, ok := lhs.(LString); ok {
		if lnum, err := parseNumber(string(str)); err == nil {
			lhs = lnum
		}
	}
	if str, ok := rhs.(LString); ok {
		if rnum, err := parseNumber(string(str)); err == nil {
			rhs = rnum
		}
	}
	if v1, ok1 := toNumber(lhs); ok1 {
		if v2, ok2 := toNumber(rhs); ok2 {
			return numberArith(L, opcode, v1, v2)
		}
	}
	L.RaiseError(fmt.Sprintf("cannot perform %v operation between %v and %v",
		strings.TrimLeft(event, "_"), lhs.Type().String(), rhs.Type().String()))

	return LNil
}

// Add this at package level
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

var stringPartsPool = sync.Pool{
	New: func() interface{} {
		s := make([]string, 0, 16)
		return &s
	},
}

func stringConcat(L *LState, total, last int) LValue {
	rhs := L.reg.Get(last)
	total--
	for i := last - 1; total > 0; {
		lhs := L.reg.Get(i)
		if !LVCanConvToString(lhs) || !LVCanConvToString(rhs) {
			op := L.metaOp2(lhs, rhs, "__concat")
			if op.Type() == LTFunction {
				L.reg.Push(op)
				L.reg.Push(lhs)
				L.reg.Push(rhs)
				L.Call(2, 1)
				if L.yieldState != yieldNone {
					return LNil
				}
				rhs = L.reg.Pop()
				total--
				i--
			} else {
				L.RaiseError("cannot perform concat operation between %v and %v", lhs.Type().String(), rhs.Type().String())
				return LNil
			}
		} else {
			// Get builder from pool
			builder := stringBuilderPool.Get().(*strings.Builder)
			builder.Reset()

			// Get parts slice from pool
			partsPtr := stringPartsPool.Get().(*[]string)
			parts := (*partsPtr)[:0]

			// Collect strings backwards, estimate total size
			rhsStr := LVAsString(rhs)
			parts = append(parts, rhsStr)
			estimatedSize := len(rhsStr)

			for total > 0 {
				lhs = L.reg.Get(i)
				if !LVCanConvToString(lhs) {
					break
				}
				lhsStr := LVAsString(lhs)
				parts = append(parts, lhsStr)
				estimatedSize += len(lhsStr)
				i--
				total--
			}

			// Pre-allocate builder capacity
			builder.Grow(estimatedSize)

			// Write strings in reverse order (left to right in final result)
			for j := len(parts) - 1; j >= 0; j-- {
				builder.WriteString(parts[j])
			}

			result := LString(builder.String())

			// Return slices to pool
			*partsPtr = parts
			stringPartsPool.Put(partsPtr)
			stringBuilderPool.Put(builder)

			rhs = result
		}
	}
	return rhs
}

func lessThan(L *LState, lhs, rhs LValue) bool {
	// optimization for numbers
	if v1, ok1 := toNumber(lhs); ok1 {
		if v2, ok2 := toNumber(rhs); ok2 {
			return v1 < v2
		}
		L.RaiseError("attempt to compare %v with %v", lhs.Type().String(), rhs.Type().String())
	}
	if lhs.Type() != rhs.Type() {
		L.RaiseError("attempt to compare %v with %v", lhs.Type().String(), rhs.Type().String())
		return false
	}
	ret := false
	switch lhs.Type() {
	case LTString:
		ret = strCmp(string(lhs.(LString)), string(rhs.(LString))) < 0
	case LTType:
		l := lhs.(*LType)
		r := rhs.(*LType)
		ret = TypeIsSubtype(l, r) && !TypeEquals(l, r)
	default:
		ret = objectRationalWithError(L, lhs, rhs, "__lt")
	}
	return ret
}

func equals(L *LState, lhs, rhs LValue, raw bool) bool {
	lt := lhs.Type()
	rt := rhs.Type()

	// Numeric equality: LNumber and LInteger can be compared
	if (lt == LTNumber || lt == LTInteger) && (rt == LTNumber || rt == LTInteger) {
		v1, _ := toNumber(lhs)
		v2, _ := toNumber(rhs)
		return v1 == v2
	}

	if lt != rt {
		return false
	}

	ret := false
	switch lt {
	case LTNil:
		ret = true
	case LTNumber:
		v1, _ := lhs.(LNumber)
		v2, _ := rhs.(LNumber)
		ret = v1 == v2
	case LTInteger:
		v1, _ := lhs.(LInteger)
		v2, _ := rhs.(LInteger)
		ret = v1 == v2
	case LTBool:
		ret = bool(lhs.(LBool)) == bool(rhs.(LBool))
	case LTString:
		ret = string(lhs.(LString)) == string(rhs.(LString))
	case LTUserData, LTTable:
		if lhs == rhs {
			ret = true
		} else if !raw {
			switch objectRational(L, lhs, rhs, "__eq") {
			case 1:
				ret = true
			case -2:
				return false // yield happened, caller checks L.yieldState
			default:
				ret = false
			}
		}
	case LTFunction:
		// LGoFunc is not comparable with ==, so handle functions specially
		switch l := lhs.(type) {
		case *LFunction:
			r, ok := rhs.(*LFunction)
			ret = ok && l == r
		case LGoFunc:
			r, ok := rhs.(LGoFunc)
			// Compare function pointers via their string representation
			ret = ok && fmt.Sprintf("%p", l) == fmt.Sprintf("%p", r)
		}
	case LTType:
		ret = TypeEquals(lhs.(*LType), rhs.(*LType))
	default:
		ret = lhs == rhs
	}
	return ret
}

func objectRationalWithError(L *LState, lhs, rhs LValue, event string) bool {
	switch objectRational(L, lhs, rhs, event) {
	case 1:
		return true
	case 0:
		return false
	case -2:
		return false // yield happened, caller checks L.yieldState
	}
	L.RaiseError("attempt to compare %v with %v", lhs.Type().String(), rhs.Type().String())
	return false
}

func objectRational(L *LState, lhs, rhs LValue, event string) int {
	m1 := L.metaOp1(lhs, event)
	m2 := L.metaOp1(rhs, event)
	if m1.Type() == LTFunction && m1 == m2 {
		L.reg.Push(m1)
		L.reg.Push(lhs)
		L.reg.Push(rhs)
		L.Call(2, 1)
		if L.yieldState != yieldNone {
			return -2
		}
		if LVAsBool(L.reg.Pop()) {
			return 1
		}
		return 0
	}
	return -1
}
