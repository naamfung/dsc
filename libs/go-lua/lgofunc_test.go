package lua

import (
	"testing"
)

func TestLGoFuncCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create a stateless Go function
	add := LGoFunc(func(L *LState) int {
		a := L.CheckNumber(1)
		b := L.CheckNumber(2)
		L.Push(a + b)
		return 1
	})

	// Set it as a global
	L.SetGlobal("add", add)

	// Call it from Lua
	if err := L.DoString(`result = add(10, 20)`); err != nil {
		t.Fatal(err)
	}

	result := L.GetGlobal("result")
	if result.Type() != LTNumber {
		t.Fatalf("expected number, got %s", result.Type())
	}
	if float64(result.(LNumber)) != 30 {
		t.Fatalf("expected 30, got %v", result)
	}
}

func TestLGoFuncInTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Create a module table with LGoFunc
	mod := L.NewTable()
	mod.RawSetString("double", LGoFunc(func(L *LState) int {
		n := L.CheckNumber(1)
		L.Push(n * 2)
		return 1
	}))
	mod.RawSetString("half", LGoFunc(func(L *LState) int {
		n := L.CheckNumber(1)
		L.Push(n / 2)
		return 1
	}))

	L.SetGlobal("math2", mod)

	// Test both functions
	if err := L.DoString(`
		a = math2.double(5)
		b = math2.half(10)
	`); err != nil {
		t.Fatal(err)
	}

	a := L.GetGlobal("a")
	b := L.GetGlobal("b")

	if float64(a.(LNumber)) != 10 {
		t.Fatalf("expected double(5)=10, got %v", a)
	}
	if float64(b.(LNumber)) != 5 {
		t.Fatalf("expected half(10)=5, got %v", b)
	}
}

func TestLGoFuncType(t *testing.T) {
	fn := LGoFunc(func(L *LState) int { return 0 })

	if fn.Type() != LTFunction {
		t.Fatalf("LGoFunc type should be LTFunction, got %s", fn.Type())
	}

	str := fn.String()
	if str[:7] != "gofunc:" {
		t.Fatalf("LGoFunc string should start with 'gofunc:', got %s", str)
	}
}

func TestLGoFuncWithMultipleReturns(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Function returning multiple values
	divmod := LGoFunc(func(L *LState) int {
		a := L.CheckNumber(1)
		b := L.CheckNumber(2)
		q := int(a) / int(b)
		r := int(a) % int(b)
		L.Push(LNumber(q))
		L.Push(LNumber(r))
		return 2
	})

	L.SetGlobal("divmod", divmod)

	if err := L.DoString(`q, r = divmod(17, 5)`); err != nil {
		t.Fatal(err)
	}

	q := L.GetGlobal("q")
	r := L.GetGlobal("r")

	if float64(q.(LNumber)) != 3 {
		t.Fatalf("expected quotient 3, got %v", q)
	}
	if float64(r.(LNumber)) != 2 {
		t.Fatalf("expected remainder 2, got %v", r)
	}
}

func TestLGoFuncTailCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Set up a function that will be tail-called
	getValue := LGoFunc(func(L *LState) int {
		L.Push(LNumber(42))
		return 1
	})

	L.SetGlobal("getValue", getValue)

	// Lua function that tail-calls getValue
	if err := L.DoString(`
		function wrapper()
			return getValue()
		end
		result = wrapper()
	`); err != nil {
		t.Fatal(err)
	}

	result := L.GetGlobal("result")
	if float64(result.(LNumber)) != 42 {
		t.Fatalf("expected 42, got %v", result)
	}
}

func TestBaselibIterators(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Test ipairs
	if err := L.DoString(`
		t = {10, 20, 30}
		sum = 0
		for i, v in ipairs(t) do
			sum = sum + v
		end
	`); err != nil {
		t.Fatal(err)
	}

	sum := L.GetGlobal("sum")
	sumVal := LVAsNumber(sum)
	if float64(sumVal) != 60 {
		t.Fatalf("ipairs: expected sum=60, got %v", sum)
	}

	// Test pairs
	if err := L.DoString(`
		t2 = {a=1, b=2, c=3}
		count = 0
		for k, v in pairs(t2) do
			count = count + 1
		end
	`); err != nil {
		t.Fatal(err)
	}

	count := L.GetGlobal("count")
	countVal := LVAsNumber(count)
	if float64(countVal) != 3 {
		t.Fatalf("pairs: expected count=3, got %v", count)
	}
}

func TestLGoFuncEquality(t *testing.T) {
	L := NewState()
	defer L.Close()

	fn1 := LGoFunc(func(L *LState) int { return 0 })
	fn2 := LGoFunc(func(L *LState) int { return 0 })

	L.SetGlobal("fn1", fn1)
	L.SetGlobal("fn2", fn2)
	L.SetGlobal("fn1_copy", fn1) // Same function pointer

	// Test that comparing LGoFunc values doesn't panic
	if err := L.DoString(`
		same = (fn1 == fn1_copy)  -- same pointer, should be true
		diff = (fn1 == fn2)       -- different pointers, should be false
	`); err != nil {
		t.Fatal(err)
	}

	same := L.GetGlobal("same")
	if same != LTrue {
		t.Fatalf("fn1 == fn1_copy should be true, got %v", same)
	}

	diff := L.GetGlobal("diff")
	if diff != LFalse {
		t.Fatalf("fn1 == fn2 should be false, got %v", diff)
	}
}

func TestLGoFuncRawEqual(t *testing.T) {
	L := NewState()
	defer L.Close()

	fn := LGoFunc(func(L *LState) int { return 0 })

	L.SetGlobal("fn", fn)
	L.SetGlobal("fn_copy", fn)

	// Test rawequal with LGoFunc
	if err := L.DoString(`result = rawequal(fn, fn_copy)`); err != nil {
		t.Fatal(err)
	}

	result := L.GetGlobal("result")
	if result != LTrue {
		t.Fatalf("rawequal(fn, fn_copy) should be true, got %v", result)
	}
}
