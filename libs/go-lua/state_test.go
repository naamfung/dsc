package lua

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/parse"
)

func TestLStateIsClosed(t *testing.T) {
	L := NewState()
	L.Close()
	errorIfNotEqual(t, true, L.IsClosed())
}

func TestCallStackOverflowWhenFixed(t *testing.T) {
	L := NewState(Options{
		CallStackSize: 3,
	})
	defer L.Close()

	// expect fixed stack implementation by default (for backwards compatibility)
	stack := L.stack
	if _, ok := stack.(*fixedCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestCallStackOverflowWhenAutoGrow(t *testing.T) {
	L := NewState(Options{
		CallStackSize:       3,
		MinimizeStackMemory: true,
	})
	defer L.Close()

	// expect auto growing stack implementation when MinimizeStackMemory is set
	stack := L.stack
	if _, ok := stack.(*autoGrowingCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestSkipOpenLibs(t *testing.T) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()
	errorIfScriptNotFail(t, L, `print("")`,
		"attempt to call a non-function object")
	L2 := NewState()
	defer L2.Close()
	errorIfScriptFail(t, L2, `print("")`)
}

func TestGetAndReplace(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Replace(1, LString("b"))
	L.Replace(0, LString("c"))
	errorIfNotEqual(t, LNil, L.Get(0))
	errorIfNotEqual(t, LNil, L.Get(-10))
	errorIfNotEqual(t, L.Env, L.Get(EnvironIndex))
	errorIfNotEqual(t, LString("b"), L.Get(1))
	L.Push(LString("c"))
	L.Push(LString("d"))
	L.Replace(-2, LString("e"))
	errorIfNotEqual(t, LString("e"), L.Get(-2))
	registry := L.NewTable()
	L.Replace(RegistryIndex, registry)
	L.G.Registry = registry
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(RegistryIndex, LNil)
		return 0
	}, "registry must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		env := L.NewTable()
		L.Replace(EnvironIndex, env)
		errorIfNotEqual(t, env, L.Get(EnvironIndex))
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(EnvironIndex, LNil)
		return 0
	}, "environment must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		gbl := L.NewTable()
		L.Replace(GlobalsIndex, gbl)
		errorIfNotEqual(t, gbl, L.G.Global)
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(GlobalsIndex, LNil)
		return 0
	}, "_G must be a table")

	L2 := NewState()
	defer L2.Close()
	clo := L2.NewClosure(func(L2 *LState) int {
		L2.Replace(UpvalueIndex(1), LNumber(3))
		errorIfNotEqual(t, LNumber(3), L2.Get(UpvalueIndex(1)))
		return 0
	}, LNumber(1), LNumber(2))
	L2.SetGlobal("clo", clo)
	errorIfScriptFail(t, L2, `clo()`)
}

func TestRemove(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Push(LString("b"))
	L.Push(LString("c"))

	L.Remove(4)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(3)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
	L.Push(LString("c"))

	L.Remove(-10)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(2)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("c"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
}

func TestToInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, 10, L.ToInt(1))
	errorIfNotEqual(t, 99, L.ToInt(2))
	errorIfNotEqual(t, 0, L.ToInt(3))
}

func TestToInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, int64(10), L.ToInt64(1))
	errorIfNotEqual(t, int64(99), L.ToInt64(2))
	errorIfNotEqual(t, int64(0), L.ToInt64(3))
}

func TestToNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, LNumber(10), L.ToNumber(1))
	errorIfNotEqual(t, LNumber(99.9), L.ToNumber(2))
	errorIfNotEqual(t, LNumber(0), L.ToNumber(3))
}

func TestToString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, "10", L.ToString(1))
	errorIfNotEqual(t, "99.9", L.ToString(2))
	errorIfNotEqual(t, "", L.ToString(3))
}

func TestToTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfFalse(t, L.ToTable(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToTable(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToTable(3))
}

func TestToFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewFunction(func(L *LState) int { return 0 }))
	errorIfFalse(t, L.ToFunction(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToFunction(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToFunction(3))
}

func TestToUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewUserData())
	errorIfFalse(t, L.ToUserData(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToUserData(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToUserData(3))
}

func TestObjLen(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, 3, L.ObjLen(LString("abc")))
	tbl := L.NewTable()
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	errorIfNotEqual(t, 2, L.ObjLen(tbl))
	mt := L.NewTable()
	L.SetField(mt, "__len", L.NewFunction(func(L *LState) int {
		tbl := L.CheckTable(1)
		L.Push(LNumber(tbl.Len() + 1))
		return 1
	}))
	L.SetMetatable(tbl, mt)
	errorIfNotEqual(t, 3, L.ObjLen(tbl))
	errorIfNotEqual(t, 0, L.ObjLen(LNumber(10)))
}

func TestConcat(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, "a1c", L.Concat(LString("a"), LNumber(1), LString("c")))
}

func TestPCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Register("f1", func(L *LState) int {
		panic("panic!")
	})
	errorIfScriptNotFail(t, L, `f1()`, "panic!")
	L.Push(L.GetGlobal("f1"))
	err := L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.Push(LString("by handler"))
		return 1
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	errorIfFalse(t, strings.Contains(err.Error(), "by handler"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	errorIfFalse(t, strings.Contains(err.Error(), "error!"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		panic("panicc!")
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	errorIfFalse(t, strings.Contains(err.Error(), "panicc!"), "")

	// Issue #452, expected to be revert back to previous call stack after any error.
	currentFrame, currentTop, currentSp := L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, nil)
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")

	currentFrame, currentTop, currentSp = L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")
}

func TestCoroutineApi1(t *testing.T) {
	L := NewState()
	defer L.Close()
	co := L.NewThreadWithContext(context.TODO())
	errorIfScriptFail(t, L, `
      function coro(v)
        assert(v == 10)
        local ret1, ret2 = coroutine.yield(1,2,3)
        assert(ret1 == 11)
        assert(ret2 == 12)
        coroutine.yield(4)
        return 5
      end
    `)
	fn := L.GetGlobal("coro").(*LFunction)
	st, values, err := L.Resume(co, fn, LNumber(10))
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 3, len(values))
	errorIfNotEqual(t, LNumber(1), LVAsNumber(values[0]))
	errorIfNotEqual(t, LNumber(2), LVAsNumber(values[1]))
	errorIfNotEqual(t, LNumber(3), LVAsNumber(values[2]))

	st, values, err = L.Resume(co, fn, LNumber(11), LNumber(12))
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(4), LVAsNumber(values[0]))

	st, values, err = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeOK, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(5), LVAsNumber(values[0]))

	L.Register("myyield", func(L *LState) int {
		return L.Yield(L.ToNumber(1))
	})
	errorIfScriptFail(t, L, `
      function coro_error()
        coroutine.yield(1,2,3)
        myyield(4)
        assert(false, "--failed--")
      end
    `)
	fn = L.GetGlobal("coro_error").(*LFunction)
	co = L.NewThreadWithContext(context.TODO())
	st, values, err = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 3, len(values))
	errorIfNotEqual(t, LNumber(1), LVAsNumber(values[0]))
	errorIfNotEqual(t, LNumber(2), LVAsNumber(values[1]))
	errorIfNotEqual(t, LNumber(3), LVAsNumber(values[2]))

	st, values, err = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(4), LVAsNumber(values[0]))

	st, _, err = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeError, st)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "--failed--"), "error message must be '--failed--'")
	st, _, err = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeError, st)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "can not resume a dead thread"), "can not resume a dead thread")

}

func TestContextTimeout(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	L.SetContext(ctx)
	errorIfNotEqual(t, ctx, L.Context())

	start := time.Now()
	L.SetGlobal("clock", L.NewFunction(func(L *LState) int {
		L.Push(LNumber(time.Since(start).Seconds()))
		return 1
	}))

	err := L.DoString(`
      function sleep(n)
        local t0 = clock()
        while clock() - t0 <= n do end
      end
	  sleep(3)
	`)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context deadline exceeded"), "execution must be canceled")

	oldctx := L.RemoveContext()
	errorIfNotEqual(t, ctx, oldctx)
	errorIfNotNil(t, L.ctx)
}

func TestContextCancel(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithCancel(context.Background())
	errch := make(chan error, 1)
	L.SetContext(ctx)

	start := time.Now()
	L.SetGlobal("clock", L.NewFunction(func(L *LState) int {
		L.Push(LNumber(time.Since(start).Seconds()))
		return 1
	}))

	go func() {
		errch <- L.DoString(`
        function sleep(n)
          local t0 = clock()
          while clock() - t0 <= n do end
        end
	    sleep(3)
	  `)
	}()
	time.Sleep(1 * time.Second)
	cancel()
	err := <-errch
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context canceled"), "execution must be canceled")
}

func TestContextWithCroutine(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)
	defer cancel()
	_ = L.DoString(`
	    function coro()
		  local i = 0
		  while true do
		    coroutine.yield(i)
			i = i+1
		  end
		  return i
	    end
	`)
	co := L.NewThreadWithContext(ctx)
	fn := L.GetGlobal("coro").(*LFunction)
	_, values, err := L.Resume(co, fn)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, LNumber(0), LVAsNumber(values[0]))
	// cancel the parent context
	cancel()
	_, _, err = L.Resume(co, fn)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context canceled"), "coroutine execution must be canceled when the parent context is canceled")

}

func TestPCallAfterFail(t *testing.T) {
	L := NewState()
	defer L.Close()
	errFn := L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 0
	})
	changeError := L.NewFunction(func(L *LState) int {
		L.Push(errFn)
		err := L.PCall(0, 0, nil)
		if err != nil {
			L.RaiseError("A New Error")
		}
		return 0
	})
	L.Push(changeError)
	err := L.PCall(0, 0, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	errorIfFalse(t, strings.Contains(err.Error(), "A New Error"), "error not propogated correctly")
}

func TestRegistryFixedOverflow(t *testing.T) {
	state := NewState(Options{RegistrySize: 256, RegistryMaxSize: 256})
	defer state.Close()
	reg := state.reg
	expectedPanic := false
	// explicitly disable growth for this test
	errorIfFalse(t, reg.maxSize == 256, "state should have fixed max size")
	// fill the stack and check we get a panic
	test := LString("test")
	for i := 0; i < len(reg.array); i++ {
		reg.Push(test)
	}
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	expectedPanic = true
	reg.Push(test)
}

func TestRegistryAutoGrow(t *testing.T) {
	state := NewState(Options{RegistryMaxSize: 300, RegistrySize: 200, RegistryGrowStep: 25})
	defer state.Close()
	expectedPanic := false
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	reg := state.reg
	test := LString("test")
	for i := 0; i < 300; i++ {
		reg.Push(test)
	}
	expectedPanic = true
	reg.Push(test)
}

// This test exposed a panic caused by accessing an unassigned var in the lua registry.
// The panic was caused by initCallFrame. It was calling resize() on the registry after it had written some values
// directly to the reg's Array, but crucially, before it had updated "top". This meant when the resize occurred, the
// values beyond top where not copied, and were lost, leading to a later uninitialised value being found in the registry.
func TestUninitializedVarAccess(t *testing.T) {
	// This regression needs a fresh 128-slot registry to trigger a resize at
	// the exact point that exposed the original bug. Drain the pool and force
	// a GC to flush per-P caches, preventing recycled states with wrong capacity.
	for statePool.Get() != nil {
	}
	runtime.GC()
	for statePool.Get() != nil {
	}

	L := NewState(Options{
		RegistrySize:    128,
		RegistryMaxSize: 256,
	})
	defer L.Close()
	// This test needs to trigger a resize when the local vars are allocated, so we need it to
	// be 128 for the padding amount in the test function to work. If it's larger, we will need
	// more padding to force the error.
	errorIfNotEqual(t, cap(L.reg.array), 128)
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)
	defer cancel()
	errorIfScriptFail(t, L, `
		local function test(arg1, arg2, arg3)
			-- padding to cause a registry resize when the local vars for this func are reserved
			local a0,b0,c0,d0,e0,f0,g0,h0,i0,j0,k0,l0,m0,n0,o0,p0,q0,r0,s0,t0,u0,v0,w0,x0,y0,z0
			local a1,b1,c1,d1,e1,f1,g1,h1,i1,j1,k1,l1,m1,n1,o1,p1,q1,r1,s1,t1,u1,v1,w1,x1,y1,z1
			local a2,b2,c2,d2,e2,f2,g2,h2,i2,j2,k2,l2,m2,n2,o2,p2,q2,r2,s2,t2,u2,v2,w2,x2,y2,z2
			local a3,b3,c3,d3,e3,f3,g3,h3,i3,j3,k3,l3,m3,n3,o3,p3,q3,r3,s3,t3,u3,v3,w3,x3,y3,z3
			local a4,b4,c4,d4,e4,f4,g4,h4,i4,j4,k4,l4,m4,n4,o4,p4,q4,r4,s4,t4,u4,v4,w4,x4,y4,z4
			if arg3 == nil then
				return 1
			end
			return 0
		end

		test(1,2)
	`)
}

// TestCoroutineReturnNilType tests that coroutine return values are properly
// handled when returning uninitialized variables. The type() function must
// not crash when called on these values.
func TestCoroutineReturnNilType(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)
	defer cancel()
	errorIfScriptFail(t, L, `
		local r1, r2
		local co = coroutine.create(function()
			r1, r2 = coroutine.yield()
			return r1, r2
		end)
		coroutine.resume(co)
		local ok, ret1, ret2 = coroutine.resume(co)
		local t1 = type(ret1)
		local t2 = type(ret2)
		assert(t1 == "nil", "expected nil, got " .. t1)
		assert(t2 == "nil", "expected nil, got " .. t2)
	`)
}

func BenchmarkCallFrameStackPushPopAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

// this test will intentionally not incur stack growth in order to bench the performance when no allocations happen
func BenchmarkCallFrameStackPushPopShallowAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopShallowFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackUnwindAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

type registryTestHandler int

func (registryTestHandler) registryOverflow() {
	panic("registry overflow")
}

// test pushing and popping from the registry
func BenchmarkRegistryPushPopAutoGrow(t *testing.B) {
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz/2, 64, sz)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistryPushPopFixed(t *testing.B) {
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 0, sz)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistrySetTop(t *testing.B) {
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 32, sz*2)

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		reg.SetTop(sz)
		reg.SetTop(0)
	}
}

func TestMergingLoadNilBug(t *testing.T) {
	// there was a bug where a multiple load nils were being incorrectly merged, and the following code exposed it
	s := `
    function test()
        local a = 0
        local b = 1
        local c = 2
        local d = 3
        local e = 4		-- reg 4
        local f = 5
        local g = 6
        local h = 7
        if e == 4 then
            e = nil		-- should clear reg 4, but clears regs 4-8 by mistake
        end
        if f == nil then
            error("bad f")
        end
        if g == nil then
            error("bad g")
        end
        if h == nil then
            error("bad h")
        end
    end
    test()
`
	L := NewState()
	defer L.Close()
	if err := L.DoString(s); err != nil {
		t.Error(err)
	}
}
func TestMergingLoadNil(t *testing.T) {
	// multiple nil assignments to consecutive registers should be merged
	s := `
		function test()
			local a = 0
			local b = 1
			local c = 2
			-- this should generate just one LOADNIL byte code instruction
			a = nil
			b = nil
			c = nil
			print(a,b,c)
		end
		test()`
	chunk, err := parse.Parse(strings.NewReader(s), "test")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := Compile(chunk, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.FunctionPrototypes) != 1 {
		t.Fatal("expected 1 function prototype")
	}
	// there should be exactly 1 LOADNIL instruction in the byte code generated for the above
	// anymore, and the LOADNIL merging is not working correctly
	count := 0
	for _, instr := range compiled.FunctionPrototypes[0].Code {
		if opGetOpCode(instr) == OP_LOADNIL {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 LOADNIL instruction, found %d", count)
	}
}

func BenchmarkNewStateNoLibs(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(Options{SkipOpenLibs: true})
		L.Close()
	}
}

func BenchmarkNewStateWithLibs(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState()
		L.Close()
	}
}

func BenchmarkNewStateCoreOnly(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(Options{SkipOpenLibs: true})
		L.Push(L.NewFunction(OpenBase))
		L.Push(LString(BaseLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenTable))
		L.Push(LString(TabLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenString))
		L.Push(LString(StringLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenMath))
		L.Push(LString(MathLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenCoroutine))
		L.Push(LString(CoroutineLibName))
		L.Call(1, 0)
		L.Close()
	}
}

func BenchmarkNewThread(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := L.NewThreadWithContext(context.TODO())
		_ = t
	}
}

func BenchmarkNewThreadEngine2Options(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	L := NewState(opts)
	defer L.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := L.NewThreadWithContext(context.TODO())
		_ = t
	}
}

func BenchmarkNewStateEngine2Options(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(opts)
		L.Push(L.NewFunction(OpenBase))
		L.Push(LString(BaseLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenTable))
		L.Push(LString(TabLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenString))
		L.Push(LString(StringLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenMath))
		L.Push(LString(MathLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenCoroutine))
		L.Push(LString(CoroutineLibName))
		L.Call(1, 0)
		L.Close()
	}
}

func BenchmarkNewStateEngine2NoLibs(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(opts)
		L.Close()
	}
}

func BenchmarkLibBaseOnly(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(opts)
		L.Push(L.NewFunction(OpenBase))
		L.Push(LString(BaseLibName))
		L.Call(1, 0)
		L.Close()
	}
}

func BenchmarkLibCoroutineOnly(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(opts)
		L.Push(L.NewFunction(OpenCoroutine))
		L.Push(LString(CoroutineLibName))
		L.Call(1, 0)
		L.Close()
	}
}

func BenchmarkLibImmutableOnly(b *testing.B) {
	opts := Options{
		RegistrySize:        128,
		RegistryMaxSize:     256 * 256,
		RegistryGrowStep:    16,
		SkipOpenLibs:        true,
		CallStackSize:       128,
		MinimizeStackMemory: true,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(opts)
		L.Push(L.NewFunction(OpenTable))
		L.Push(LString(TabLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenString))
		L.Push(LString(StringLibName))
		L.Call(1, 0)
		L.Push(L.NewFunction(OpenMath))
		L.Push(LString(MathLibName))
		L.Call(1, 0)
		L.Close()
	}
}

func BenchmarkPooledStateReuse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState(Options{SkipOpenLibs: true})
		L.Close()
	}
}

func BenchmarkPooledThreadReuse(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := L.NewThreadWithContext(context.TODO())
		t.Close()
	}
}

func BenchmarkDefaultOptions(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L := NewState()
		L.Close()
	}
}

func BenchmarkDoString(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = L.DoString("local x = 1 + 2")
	}
}

func BenchmarkFunctionCall(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn := L.NewFunction(func(L *LState) int {
		L.Push(LNumber(42))
		return 1
	})
	L.SetGlobal("testfn", fn)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = L.DoString("testfn()")
	}
}
