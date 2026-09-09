package lua

import (
	"context"
	"strings"
	"testing"
)

// Tests for yielding across call boundaries that use callR/Call internally.
// These opcodes use nested mainLoop calls which don't propagate yields correctly.
//
// Affected paths:
//   - OP_TFORLOOP: generic for loop iterator calls (callR)
//   - getFieldString: __index metamethod (Call)
//   - setField: __newindex metamethod (Call)
//   - objectArith: __add/__sub/__mul/__div/__mod/__pow metamethods (Call)
//   - objectConcat: __concat metamethod (Call)
//   - objectRational: __eq/__lt/__le metamethods (Call)
//   - OP_UNM: __unm metamethod (Call)
//   - OP_LEN: __len metamethod (Call)

// helper: creates a Go function that yields the first argument.
func yieldingGoFunc(L *LState) int {
	return L.Yield(L.Get(1))
}

// helper: resume coroutine expecting yield, return yielded values.
func expectYield(t *testing.T, L *LState, co *LState, fn *LFunction, args ...LValue) []LValue {
	t.Helper()
	state, results, err := L.Resume(co, fn, args...)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeYield {
		t.Fatalf("Expected ResumeYield, got %v (results: %v)", state, results)
	}
	return results
}

// helper: resume coroutine expecting completion, return results.
func expectDone(t *testing.T, L *LState, co *LState, fn *LFunction, args ...LValue) []LValue {
	t.Helper()
	state, results, err := L.Resume(co, fn, args...)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if state != ResumeOK {
		t.Fatalf("Expected ResumeOK, got %v (results: %v)", state, results)
	}
	return results
}

// ---------------------------------------------------------------------------
// OP_TFORLOOP: yield from generic for-loop iterator
// ---------------------------------------------------------------------------

func TestYieldFromForInIterator_LuaYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Iterator that yields each value before returning it
	if err := L.DoString(`
		function yielding_iter(items)
			local i = 0
			return function()
				i = i + 1
				if i > #items then return nil end
				coroutine.yield("producing:" .. items[i])
				return items[i]
			end
		end

		function test()
			local results = {}
			for val in yielding_iter({"a", "b", "c"}) do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// Each iteration yields before returning the value
	r := expectYield(t, L, co, fn)
	if r[0].String() != "producing:a" {
		t.Fatalf("Expected 'producing:a', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "producing:b" {
		t.Fatalf("Expected 'producing:b', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "producing:c" {
		t.Fatalf("Expected 'producing:c', got %v", r[0])
	}

	// Final resume completes
	results := expectDone(t, L, co, fn)
	if results[0].String() != "a,b,c" {
		t.Errorf("Expected 'a,b,c', got %v", results[0])
	}
}

func TestYieldFromForInIterator_GoFunctionYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Go function that yields its argument
	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function yielding_iter(items)
			local i = 0
			return function()
				i = i + 1
				if i > #items then return nil end
				go_yield("producing:" .. items[i])
				return items[i]
			end
		end

		function test()
			local results = {}
			for val in yielding_iter({"a", "b", "c"}) do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// Each iteration: Go function yields, then resume returns to iterator
	for _, expected := range []string{"producing:a", "producing:b", "producing:c"} {
		r := expectYield(t, L, co, fn)
		if r[0].String() != expected {
			t.Fatalf("Expected %q, got %v", expected, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "a,b,c" {
		t.Errorf("Expected 'a,b,c', got %v", results[0])
	}
}

func TestYieldFromForInIterator_ResumeValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Iterator that yields a request and uses the resume value
	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function lazy_iter(ids)
			local i = 0
			return function()
				i = i + 1
				if i > #ids then return nil end
				-- yield request, receive loaded data on resume
				local data = go_yield("load:" .. ids[i])
				return data
			end
		end

		function test()
			local results = {}
			for val in lazy_iter({"x", "y"}) do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// First iteration yields load request
	r := expectYield(t, L, co, fn)
	if r[0].String() != "load:x" {
		t.Fatalf("Expected 'load:x', got %v", r[0])
	}

	// Resume with loaded data
	r = expectYield(t, L, co, fn, LString("data_x"))
	if r[0].String() != "load:y" {
		t.Fatalf("Expected 'load:y', got %v", r[0])
	}

	// Resume with second loaded data, completes
	results := expectDone(t, L, co, fn, LString("data_y"))
	if results[0].String() != "data_x,data_y" {
		t.Errorf("Expected 'data_x,data_y', got %v", results[0])
	}
}

func TestYieldFromForInIterator_MultipleReturnValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	// Iterator returns key, value pairs (like pairs/ipairs)
	if err := L.DoString(`
		function kv_iter(tbl)
			local keys = {}
			for k in pairs(tbl) do keys[#keys + 1] = k end
			table.sort(keys)
			local i = 0
			return function()
				i = i + 1
				if i > #keys then return nil end
				go_yield("fetch:" .. keys[i])
				return keys[i], tbl[keys[i]]
			end
		end

		function test()
			local results = {}
			for k, v in kv_iter({a=1, b=2}) do
				results[#results + 1] = k .. "=" .. v
			end
			table.sort(results)
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn) // fetch:a
	expectYield(t, L, co, fn) // fetch:b
	results := expectDone(t, L, co, fn)
	if results[0].String() != "a=1,b=2" {
		t.Errorf("Expected 'a=1,b=2', got %v", results[0])
	}
}

func TestYieldFromForInIterator_WithPcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function yielding_iter(n)
			local i = 0
			return function()
				i = i + 1
				if i > n then return nil end
				go_yield("item:" .. i)
				return i
			end
		end

		function test()
			local ok, result = pcall(function()
				local sum = 0
				for v in yielding_iter(3) do
					sum = sum + v
				end
				return sum
			end)
			return ok, result
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn) // item:1
	expectYield(t, L, co, fn) // item:2
	expectYield(t, L, co, fn) // item:3

	results := expectDone(t, L, co, fn)
	if results[0] != LTrue {
		t.Errorf("Expected pcall success, got %v", results[0])
	}
	if LVAsNumber(results[1]) != 6 {
		t.Errorf("Expected 6, got %v", results[1])
	}
}

func TestYieldFromForInIterator_ErrorAfterYield(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Track how many times the iterator is called per resume cycle.
	// The bug causes double-execution: iterator called twice per single resume.
	callCount := 0
	L.SetGlobal("go_yield", L.NewFunction(func(L *LState) int {
		callCount++
		return L.Yield(L.Get(1))
	}))

	if err := L.DoString(`
		function failing_iter()
			local i = 0
			return function()
				i = i + 1
				if i > 2 then error("iterator exhausted badly") end
				go_yield("item:" .. i)
				return i
			end
		end

		function test()
			local ok, err = pcall(function()
				local sum = 0
				for v in failing_iter() do
					sum = sum + v
				end
				return sum
			end)
			return ok, err
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	callCount = 0
	r := expectYield(t, L, co, fn)
	if r[0].String() != "item:1" {
		t.Fatalf("Expected 'item:1', got %v", r[0])
	}
	if callCount != 1 {
		t.Fatalf("Expected go_yield called once per resume, called %d times (double-execution bug)", callCount)
	}

	callCount = 0
	r = expectYield(t, L, co, fn)
	if r[0].String() != "item:2" {
		t.Fatalf("Expected 'item:2', got %v", r[0])
	}
	if callCount != 1 {
		t.Fatalf("Expected go_yield called once per resume, called %d times (double-execution bug)", callCount)
	}

	// Third iteration errors before yield
	results := expectDone(t, L, co, fn)
	if results[0] != LFalse {
		t.Errorf("Expected pcall failure, got %v", results[0])
	}
}

func TestYieldFromForInIterator_NestedForLoops(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function yiter(items)
			local i = 0
			return function()
				i = i + 1
				if i > #items then return nil end
				go_yield("y:" .. items[i])
				return items[i]
			end
		end

		function test()
			local results = {}
			for a in yiter({"x", "y"}) do
				for b in yiter({"1", "2"}) do
					results[#results + 1] = a .. b
				end
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// outer "x", inner "1", inner "2", outer "y", inner "1", inner "2"
	expectedYields := []string{"y:x", "y:1", "y:2", "y:y", "y:1", "y:2"}
	for _, expected := range expectedYields {
		r := expectYield(t, L, co, fn)
		if r[0].String() != expected {
			t.Fatalf("Expected %q, got %v", expected, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "x1,x2,y1,y2" {
		t.Errorf("Expected 'x1,x2,y1,y2', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __index metamethod: yield from field access
// ---------------------------------------------------------------------------

func TestYieldFromIndexMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__index = function(self, key)
				go_yield("index:" .. key)
				return rawget(self, "_data")[key]
			end
		}

		function make_proxy(data)
			local obj = {_data = data}
			setmetatable(obj, mt)
			return obj
		end

		function test()
			local p = make_proxy({name = "alice", age = 30})
			local n = p.name
			local a = p.age
			return n .. ":" .. a
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "index:name" {
		t.Fatalf("Expected 'index:name', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "index:age" {
		t.Fatalf("Expected 'index:age', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "alice:30" {
		t.Errorf("Expected 'alice:30', got %v", results[0])
	}
}

func TestYieldFromIndexMetamethod_MethodCall(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	// OP_SELF uses getFieldString which calls __index
	if err := L.DoString(`
		local mt = {
			__index = function(self, key)
				go_yield("lookup:" .. key)
				if key == "greet" then
					return function(self)
						return "hello " .. rawget(self, "name")
					end
				end
				return rawget(self, key)
			end
		}

		function test()
			local obj = setmetatable({name = "bob"}, mt)
			return obj:greet()
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "lookup:greet" {
		t.Fatalf("Expected 'lookup:greet', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "hello bob" {
		t.Errorf("Expected 'hello bob', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __newindex metamethod: yield from field assignment
// ---------------------------------------------------------------------------

func TestYieldFromNewIndexMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local log = {}
		local mt = {
			__newindex = function(self, key, value)
				go_yield("set:" .. key .. "=" .. tostring(value))
				rawset(self, key, value)
			end
		}

		function test()
			local obj = setmetatable({}, mt)
			obj.x = 10
			obj.y = 20
			return obj.x + obj.y
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "set:x=10" {
		t.Fatalf("Expected 'set:x=10', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "set:y=20" {
		t.Fatalf("Expected 'set:y=20', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != 30 {
		t.Errorf("Expected 30, got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __add metamethod: yield from arithmetic
// ---------------------------------------------------------------------------

func TestYieldFromAddMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__add = function(a, b)
				go_yield("add:" .. tostring(a.v) .. "+" .. tostring(b.v))
				return setmetatable({v = a.v + b.v}, getmetatable(a))
			end
		}

		function num(v)
			return setmetatable({v = v}, mt)
		end

		function test()
			local a = num(10)
			local b = num(20)
			local c = a + b
			return c.v
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "add:10+20" {
		t.Fatalf("Expected 'add:10+20', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != 30 {
		t.Errorf("Expected 30, got %v", results[0])
	}
}

func TestYieldFromSubMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__sub = function(a, b)
				go_yield("sub")
				return setmetatable({v = a.v - b.v}, getmetatable(a))
			end
		}
		function num(v) return setmetatable({v = v}, mt) end

		function test()
			local r = num(50) - num(8)
			return r.v
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn)
	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != 42 {
		t.Errorf("Expected 42, got %v", results[0])
	}
}

func TestYieldFromMulMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__mul = function(a, b)
				go_yield("mul")
				return setmetatable({v = a.v * b.v}, getmetatable(a))
			end
		}
		function num(v) return setmetatable({v = v}, mt) end

		function test()
			local r = num(6) * num(7)
			return r.v
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn)
	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != 42 {
		t.Errorf("Expected 42, got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __concat metamethod: yield from string concatenation
// ---------------------------------------------------------------------------

func TestYieldFromConcatMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__concat = function(a, b)
				local av = type(a) == "table" and a.v or tostring(a)
				local bv = type(b) == "table" and b.v or tostring(b)
				go_yield("concat:" .. av .. ".." .. bv)
				return setmetatable({v = av .. bv}, getmetatable(a) or getmetatable(b))
			end
		}
		function str(v) return setmetatable({v = v}, mt) end

		function test()
			local r = str("hello") .. str(" world")
			return r.v
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "concat:hello.. world" {
		t.Fatalf("Expected 'concat:hello.. world', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "hello world" {
		t.Errorf("Expected 'hello world', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __unm metamethod: yield from unary minus
// ---------------------------------------------------------------------------

func TestYieldFromUnmMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__unm = function(a)
				go_yield("neg:" .. a.v)
				return setmetatable({v = -a.v}, getmetatable(a))
			end
		}
		function num(v) return setmetatable({v = v}, mt) end

		function test()
			local r = -num(42)
			return r.v
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "neg:42" {
		t.Fatalf("Expected 'neg:42', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != -42 {
		t.Errorf("Expected -42, got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __len metamethod: yield from # operator
// ---------------------------------------------------------------------------

func TestYieldFromLenMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__len = function(a)
				go_yield("len")
				return #rawget(a, "items")
			end
		}

		function test()
			local obj = setmetatable({items = {1, 2, 3, 4, 5}}, mt)
			return #obj
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn)
	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != 5 {
		t.Errorf("Expected 5, got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// __eq metamethod: yield from equality comparison
// ---------------------------------------------------------------------------

func TestYieldFromEqMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__eq = function(a, b)
				go_yield("eq:" .. a.v .. "==" .. b.v)
				return a.v == b.v
			end
		}
		function val(v) return setmetatable({v = v}, mt) end

		function test()
			local a = val(42)
			local b = val(42)
			local c = val(99)
			local r1 = a == b
			local r2 = a == c
			return r1, r2
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "eq:42==42" {
		t.Fatalf("Expected 'eq:42==42', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "eq:42==99" {
		t.Fatalf("Expected 'eq:42==99', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0] != LTrue {
		t.Errorf("Expected true for 42==42, got %v", results[0])
	}
	if results[1] != LFalse {
		t.Errorf("Expected false for 42==99, got %v", results[1])
	}
}

// ---------------------------------------------------------------------------
// __lt metamethod: yield from less-than comparison
// ---------------------------------------------------------------------------

func TestYieldFromLtMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__lt = function(a, b)
				go_yield("lt:" .. a.v .. "<" .. b.v)
				return a.v < b.v
			end
		}
		function val(v) return setmetatable({v = v}, mt) end

		function test()
			local r1 = val(1) < val(2)
			local r2 = val(5) < val(3)
			return r1, r2
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn) // 1<2
	expectYield(t, L, co, fn) // 5<3

	results := expectDone(t, L, co, fn)
	if results[0] != LTrue {
		t.Errorf("Expected true for 1<2, got %v", results[0])
	}
	if results[1] != LFalse {
		t.Errorf("Expected false for 5<3, got %v", results[1])
	}
}

// ---------------------------------------------------------------------------
// __le metamethod: yield from less-than-or-equal comparison
// ---------------------------------------------------------------------------

func TestYieldFromLeMetamethod(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__le = function(a, b)
				go_yield("le:" .. a.v .. "<=" .. b.v)
				return a.v <= b.v
			end
		}
		function val(v) return setmetatable({v = v}, mt) end

		function test()
			local r1 = val(3) <= val(3)
			local r2 = val(4) <= val(3)
			return r1, r2
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expectYield(t, L, co, fn) // 3<=3
	expectYield(t, L, co, fn) // 4<=3

	results := expectDone(t, L, co, fn)
	if results[0] != LTrue {
		t.Errorf("Expected true for 3<=3, got %v", results[0])
	}
	if results[1] != LFalse {
		t.Errorf("Expected false for 4<=3, got %v", results[1])
	}
}

// ---------------------------------------------------------------------------
// Combined: yield from multiple boundary types in one coroutine
// ---------------------------------------------------------------------------

func TestYieldFromMixedBoundaries(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__index = function(self, key)
				go_yield("index:" .. key)
				return rawget(self, "_d")[key]
			end,
			__add = function(a, b)
				go_yield("add")
				return a._d.v + b._d.v
			end
		}

		function wrap(tbl)
			return setmetatable({_d = tbl}, mt)
		end

		function yielding_iter(items)
			local i = 0
			return function()
				i = i + 1
				if i > #items then return nil end
				go_yield("iter:" .. i)
				return items[i]
			end
		end

		function test()
			local a = wrap({v = 10})
			local b = wrap({v = 20})

			-- triggers __index yield
			local av = a.v

			-- triggers __add yield
			local sum = a + b

			-- triggers iterator yield
			local items = {}
			for val in yielding_iter({av, sum}) do
				items[#items + 1] = val
			end

			return table.concat(items, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// __index for a.v
	r := expectYield(t, L, co, fn)
	if r[0].String() != "index:v" {
		t.Fatalf("Expected 'index:v', got %v", r[0])
	}

	// __add for a + b
	r = expectYield(t, L, co, fn)
	if r[0].String() != "add" {
		t.Fatalf("Expected 'add', got %v", r[0])
	}

	// iterator yields
	r = expectYield(t, L, co, fn)
	if r[0].String() != "iter:1" {
		t.Fatalf("Expected 'iter:1', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "iter:2" {
		t.Fatalf("Expected 'iter:2', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "10,30" {
		t.Errorf("Expected '10,30', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// Edge case: yield from iterator inside pcall inside another for loop
// ---------------------------------------------------------------------------

func TestYieldFromIterator_InsidePcallInsideForLoop(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function yiter(items)
			local i = 0
			return function()
				i = i + 1
				if i > #items then return nil end
				go_yield("y:" .. items[i])
				return items[i]
			end
		end

		function test()
			local results = {}
			for outer in yiter({"A", "B"}) do
				local ok, inner_result = pcall(function()
					local inner = {}
					for v in yiter({"1", "2"}) do
						inner[#inner + 1] = v
					end
					return table.concat(inner, "+")
				end)
				if ok then
					results[#results + 1] = outer .. ":" .. inner_result
				end
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	// A, 1, 2, B, 1, 2
	expected := []string{"y:A", "y:1", "y:2", "y:B", "y:1", "y:2"}
	for _, exp := range expected {
		r := expectYield(t, L, co, fn)
		if r[0].String() != exp {
			t.Fatalf("Expected %q, got %v", exp, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "A:1+2,B:1+2" {
		t.Errorf("Expected 'A:1+2,B:1+2', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// coroutine.wrap: yield from iterator using coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromCoroutineWrapIterator(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function make_iter(items)
			return coroutine.wrap(function()
				for i = 1, #items do
					go_yield("load:" .. items[i])
					coroutine.yield(items[i])
				end
			end)
		end

		function test()
			local results = {}
			for val in make_iter({"a", "b", "c"}) do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	for _, item := range []string{"a", "b", "c"} {
		r := expectYield(t, L, co, fn)
		if r[0].String() != "load:"+item {
			t.Fatalf("Expected 'load:%s', got %v", item, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "a,b,c" {
		t.Errorf("Expected 'a,b,c', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield from coroutine.resume (non-wrapped, explicit resume)
// ---------------------------------------------------------------------------

func TestYieldFromCoroutineResumeExplicit(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function test()
			local th = coroutine.create(function()
				go_yield("sys:1")
				coroutine.yield("user:1")
				go_yield("sys:2")
				return "done"
			end)

			local results = {}
			while true do
				local ok, val = coroutine.resume(th)
				if not ok then break end
				results[#results + 1] = val
				if coroutine.status(th) == "dead" then break end
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "sys:1" {
		t.Fatalf("Expected 'sys:1', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "sys:2" {
		t.Fatalf("Expected 'sys:2', got %v", r[0])
	}

	// System yields propagate to the host transparently — they don't appear
	// in the Lua code's coroutine.resume return values. The Lua code only sees
	// the user yield and the final return.
	results := expectDone(t, L, co, fn)
	if results[0].String() != "user:1,done" {
		t.Errorf("Expected 'user:1,done', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield through nested coroutine.wrap (2 levels deep)
// ---------------------------------------------------------------------------

func TestYieldFromNestedCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function inner_iter(items)
			return coroutine.wrap(function()
				for _, v in ipairs(items) do
					go_yield("inner:" .. v)
					coroutine.yield(v)
				end
			end)
		end

		function outer_iter()
			return coroutine.wrap(function()
				for v in inner_iter({"x", "y"}) do
					go_yield("outer:" .. v)
					coroutine.yield("got:" .. v)
				end
			end)
		end

		function test()
			local results = {}
			for val in outer_iter() do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	expected := []string{"inner:x", "outer:x", "inner:y", "outer:y"}
	for _, exp := range expected {
		r := expectYield(t, L, co, fn)
		if r[0].String() != exp {
			t.Fatalf("Expected %q, got %v", exp, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "got:x,got:y" {
		t.Errorf("Expected 'got:x,got:y', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield interleaved with pcall inside coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromCoroutineWrapWithPcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function test()
			local iter = coroutine.wrap(function()
				go_yield("before_pcall")
				local ok, err = pcall(function()
					go_yield("inside_pcall")
					error("planned_error")
				end)
				coroutine.yield(ok)
				go_yield("after_pcall")
				coroutine.yield(tostring(err))
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = tostring(val)
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "before_pcall" {
		t.Fatalf("Expected 'before_pcall', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "inside_pcall" {
		t.Fatalf("Expected 'inside_pcall', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "after_pcall" {
		t.Fatalf("Expected 'after_pcall', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	got := results[0].String()
	if !strings.Contains(got, "false") || !strings.Contains(got, "planned_error") {
		t.Errorf("Expected result containing 'false' and 'planned_error', got %v", got)
	}
}

// ---------------------------------------------------------------------------
// System yield from __index inside coroutine.wrap iterator
// ---------------------------------------------------------------------------

func TestYieldFromMetamethodInsideCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__index = function(t, k)
				go_yield("index:" .. k)
				return rawget(t, "_" .. k)
			end
		}

		function test()
			local obj = setmetatable({_name = "alice", _age = "30"}, mt)
			local iter = coroutine.wrap(function()
				coroutine.yield(obj.name)
				coroutine.yield(obj.age)
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "index:name" {
		t.Fatalf("Expected 'index:name', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "index:age" {
		t.Fatalf("Expected 'index:age', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "alice,30" {
		t.Errorf("Expected 'alice,30', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield with multiple return values propagated through wrap
// ---------------------------------------------------------------------------

func TestYieldFromCoroutineWrapMultipleValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield_multi", L.NewFunction(func(L *LState) int {
		return L.Yield(L.Get(1), L.Get(2))
	}))

	if err := L.DoString(`
		function test()
			local iter = coroutine.wrap(function()
				go_yield_multi("a", "b")
				coroutine.yield("single")
				go_yield_multi("c", "d")
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if len(r) < 2 || r[0].String() != "a" || r[1].String() != "b" {
		t.Fatalf("Expected [a, b], got %v", r)
	}

	r = expectYield(t, L, co, fn)
	if len(r) < 2 || r[0].String() != "c" || r[1].String() != "d" {
		t.Fatalf("Expected [c, d], got %v", r)
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "single" {
		t.Errorf("Expected 'single', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield from arithmetic metamethod inside coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromArithInsideCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__add = function(a, b)
				go_yield("add:" .. rawget(a, "v") .. "+" .. rawget(b, "v"))
				return setmetatable({v = rawget(a, "v") + rawget(b, "v")}, getmetatable(a))
			end
		}

		function num(x)
			return setmetatable({v = x}, mt)
		end

		function test()
			local iter = coroutine.wrap(function()
				local a = num(10)
				local b = num(20)
				local c = a + b
				coroutine.yield(rawget(c, "v"))
				local d = c + num(5)
				coroutine.yield(rawget(d, "v"))
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = tostring(val)
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "add:10+20" {
		t.Fatalf("Expected 'add:10+20', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "add:30+5" {
		t.Fatalf("Expected 'add:30+5', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "30,35" {
		t.Errorf("Expected '30,35', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield from for-in iterator with resume values passed back
// ---------------------------------------------------------------------------

func TestYieldFromIteratorWithResumeValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_request", L.NewFunction(func(L *LState) int {
		query := L.CheckString(1)
		L.SetTop(0)
		L.Push(LString(query))
		return -1
	}))

	if err := L.DoString(`
		function test()
			local iter = coroutine.wrap(function()
				local resp1 = go_request("get_name")
				coroutine.yield(resp1)
				local resp2 = go_request("get_age")
				coroutine.yield(resp2)
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "get_name" {
		t.Fatalf("Expected 'get_name', got %v", r[0])
	}

	r = expectYield(t, L, co, fn, LString("Alice"))
	if r[0].String() != "get_age" {
		t.Fatalf("Expected 'get_age', got %v", r[0])
	}

	results := expectDone(t, L, co, fn, LString("30"))
	if results[0].String() != "Alice,30" {
		t.Errorf("Expected 'Alice,30', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield through coroutine.wrap that dies mid-iteration
// ---------------------------------------------------------------------------

func TestYieldFromCoroutineWrapWithError(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function test()
			local iter = coroutine.wrap(function()
				go_yield("step1")
				coroutine.yield("ok1")
				go_yield("step2")
				error("boom")
			end)

			local results = {}
			local ok, err = pcall(function()
				for val in iter do
					results[#results + 1] = val
				end
			end)
			results[#results + 1] = tostring(err)
			return table.concat(results, "|")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "step1" {
		t.Fatalf("Expected 'step1', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "step2" {
		t.Fatalf("Expected 'step2', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	got := results[0].String()
	if !strings.HasPrefix(got, "ok1|") || !strings.Contains(got, "boom") {
		t.Errorf("Expected 'ok1|...boom', got %v", got)
	}
}

// ---------------------------------------------------------------------------
// System yield with __newindex inside coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromNewIndexInsideCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local store = {}
		local mt = {
			__newindex = function(t, k, v)
				go_yield("set:" .. k .. "=" .. tostring(v))
				store[k] = v
			end,
			__index = function(t, k)
				return store[k]
			end
		}

		function test()
			local obj = setmetatable({}, mt)
			local iter = coroutine.wrap(function()
				obj.x = 10
				obj.y = 20
				coroutine.yield(obj.x + obj.y)
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = tostring(val)
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "set:x=10" {
		t.Fatalf("Expected 'set:x=10', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "set:y=20" {
		t.Fatalf("Expected 'set:y=20', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "30" {
		t.Errorf("Expected '30', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// High-frequency system yields: many yields in tight loop
// ---------------------------------------------------------------------------

func TestYieldFromIteratorHighFrequency(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		function test()
			local sum = 0
			for i = 1, 100 do
				go_yield(i)
				sum = sum + i
			end
			return sum
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	for i := 1; i <= 100; i++ {
		r := expectYield(t, L, co, fn)
		if LVAsNumber(r[0]) != LNumber(i) {
			t.Fatalf("Yield %d: expected %d, got %v", i, i, r[0])
		}
	}

	results := expectDone(t, L, co, fn)
	if LVAsNumber(results[0]) != LNumber(5050) {
		t.Errorf("Expected 5050, got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield from comparison metamethod inside coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromComparisonInsideCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__lt = function(a, b)
				go_yield("cmp:" .. rawget(a, "v") .. "<" .. rawget(b, "v"))
				return rawget(a, "v") < rawget(b, "v")
			end
		}

		function val(x)
			return setmetatable({v = x}, mt)
		end

		function test()
			local iter = coroutine.wrap(function()
				local a, b = val(3), val(7)
				if a < b then
					coroutine.yield("less")
				else
					coroutine.yield("greater")
				end
				local c = val(10)
				if c < b then
					coroutine.yield("less2")
				else
					coroutine.yield("greater2")
				end
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = val
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "cmp:3<7" {
		t.Fatalf("Expected 'cmp:3<7', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "cmp:10<7" {
		t.Fatalf("Expected 'cmp:10<7', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "less,greater2" {
		t.Errorf("Expected 'less,greater2', got %v", results[0])
	}
}

// ---------------------------------------------------------------------------
// System yield from __len and __concat inside coroutine.wrap
// ---------------------------------------------------------------------------

func TestYieldFromLenConcatInsideCoroutineWrap(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetGlobal("go_yield", L.NewFunction(yieldingGoFunc))

	if err := L.DoString(`
		local mt = {
			__len = function(t)
				go_yield("len:" .. rawget(t, "name"))
				return rawget(t, "size")
			end,
			__concat = function(a, b)
				local av = type(a) == "table" and rawget(a, "name") or tostring(a)
				local bv = type(b) == "table" and rawget(b, "name") or tostring(b)
				go_yield("concat:" .. av .. ".." .. bv)
				return av .. bv
			end
		}

		function obj(name, size)
			return setmetatable({name = name, size = size}, mt)
		end

		function test()
			local iter = coroutine.wrap(function()
				local a = obj("foo", 3)
				local b = obj("bar", 5)
				coroutine.yield(#a)
				coroutine.yield(#b)
				coroutine.yield(a .. b)
			end)

			local results = {}
			for val in iter do
				results[#results + 1] = tostring(val)
			end
			return table.concat(results, ",")
		end
	`); err != nil {
		t.Fatal(err)
	}

	co := L.NewThreadWithContext(context.TODO())
	fn := L.GetGlobal("test").(*LFunction)

	r := expectYield(t, L, co, fn)
	if r[0].String() != "len:foo" {
		t.Fatalf("Expected 'len:foo', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "len:bar" {
		t.Fatalf("Expected 'len:bar', got %v", r[0])
	}

	r = expectYield(t, L, co, fn)
	if r[0].String() != "concat:foo..bar" {
		t.Fatalf("Expected 'concat:foo..bar', got %v", r[0])
	}

	results := expectDone(t, L, co, fn)
	if results[0].String() != "3,5,foobar" {
		t.Errorf("Expected '3,5,foobar', got %v", results[0])
	}
}
