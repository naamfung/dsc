package lua

import (
	"testing"
)

func BenchmarkCompileSimpleScript(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	script := `local x = 1 + 2`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = L.LoadString(script)
	}
}

func BenchmarkCompileMediumScript(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	script := `
		local function add(a, b) return a + b end
		local function mul(a, b) return a * b end
		local sum = 0
		for i = 1, 100 do
			sum = add(sum, mul(i, 2))
		end
		return sum
	`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = L.LoadString(script)
	}
}

func BenchmarkCompileFibonacci(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	script := `
		local function fib(n)
			if n < 2 then return n end
			return fib(n-1) + fib(n-2)
		end
		return fib(10)
	`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = L.LoadString(script)
	}
}

func BenchmarkExecuteLoop(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local sum = 0
		for i = 1, 1000 do
			sum = sum + i
		end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkExecuteArithmetic(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local x = 0
		for i = 1, 100 do
			x = (x + i) * 2 - i / 2
		end
		return x
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkExecuteRecursion(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function fib(n)
			if n < 2 then return n end
			return fib(n-1) + fib(n-2)
		end
		return fib(15)
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkTableCreate(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := L.NewTable()
		_ = t
	}
}

func BenchmarkTableCreateSized(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := L.CreateTable(100, 10)
		_ = t
	}
}

func BenchmarkTableSetGet(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	t := L.NewTable()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t.RawSetInt(i%100, LNumber(i))
		_ = t.RawGetInt(i % 100)
	}
}

func BenchmarkTableSetGetString(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	t := L.NewTable()
	keys := []string{"key1", "key2", "key3", "key4", "key5"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		t.RawSetString(k, LNumber(i))
		_ = t.RawGetString(k)
	}
}

func BenchmarkTableIteration(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local t = {}
		for i = 1, 100 do t[i] = i end
		local sum = 0
		for i = 1, #t do sum = sum + t[i] end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkGoFunctionCall(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	counter := 0
	L.SetGlobal("inc", L.NewFunction(func(L *LState) int {
		counter++
		L.Push(LNumber(counter))
		return 1
	}))

	fn, _ := L.LoadString(`
		for i = 1, 100 do inc() end
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 0, nil)
	}
}

func BenchmarkLuaFunctionCall(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function noop() end
		for i = 1, 100 do noop() end
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 0, nil)
	}
}

func BenchmarkLuaFunctionWithArgs(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function add(a, b, c, d)
			return a + b + c + d
		end
		local sum = 0
		for i = 1, 100 do
			sum = sum + add(i, i+1, i+2, i+3)
		end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkStringConcat(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local s = ""
		for i = 1, 50 do
			s = s .. "x"
		end
		return s
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkLocalVariables(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local a, b, c, d, e = 1, 2, 3, 4, 5
		for i = 1, 100 do
			a = b + c
			b = c + d
			c = d + e
			d = e + a
			e = a + b
		end
		return a + b + c + d + e
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkUpvalues(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local counter = 0
		local function inc()
			counter = counter + 1
			return counter
		end
		for i = 1, 100 do
			inc()
		end
		return counter
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkClosureCreation(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function make_counter()
			local count = 0
			return function()
				count = count + 1
				return count
			end
		end
		for i = 1, 50 do
			local c = make_counter()
			c()
		end
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 0, nil)
	}
}

func BenchmarkMetatableCall(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local mt = {
			__index = function(t, k) return k * 2 end,
			__newindex = function(t, k, v) rawset(t, k, v + 1) end,
		}
		local t = setmetatable({}, mt)
		for i = 1, 50 do
			local x = t[i]
			t[i] = x
		end
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 0, nil)
	}
}

func BenchmarkConditionals(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local count = 0
		for i = 1, 100 do
			if i % 2 == 0 then
				count = count + 1
			elseif i % 3 == 0 then
				count = count + 2
			else
				count = count + 3
			end
		end
		return count
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkNestedTables(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local root = {}
		for i = 1, 10 do
			root[i] = {}
			for j = 1, 10 do
				root[i][j] = i * j
			end
		end
		local sum = 0
		for i = 1, 10 do
			for j = 1, 10 do
				sum = sum + root[i][j]
			end
		end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkGlobalAccess(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	L.SetGlobal("myvalue", LNumber(42))

	fn, _ := L.LoadString(`
		local sum = 0
		for i = 1, 100 do
			sum = sum + myvalue
		end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkTailCall(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function countdown(n)
			if n <= 0 then return 0 end
			return countdown(n - 1)
		end
		return countdown(100)
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkMultiReturn(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function multi()
			return 1, 2, 3, 4, 5
		end
		local sum = 0
		for i = 1, 100 do
			local a, b, c, d, e = multi()
			sum = sum + a + b + c + d + e
		end
		return sum
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}

func BenchmarkVararg(b *testing.B) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()

	fn, _ := L.LoadString(`
		local function sum(...)
			local args = {...}
			local total = 0
			for i = 1, #args do
				total = total + args[i]
			end
			return total
		end
		local result = 0
		for i = 1, 50 do
			result = result + sum(1, 2, 3, 4, 5)
		end
		return result
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		_ = L.PCall(0, 1, nil)
		L.Pop(1)
	}
}
