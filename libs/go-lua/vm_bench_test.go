package lua

import (
	"context"
	"runtime"
	"testing"
	"unsafe"
)

// Agent-typical workloads

func BenchmarkAgentDataTransform(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local data = {}
			for i = 1, 100 do
				data[i] = {id = i, value = i * 10, active = i % 2 == 0}
			end
			local result = {}
			local count = 0
			for i, item in ipairs(data) do
				if item.active and item.value > 200 then
					count = count + 1
					result[count] = {id = item.id, doubled = item.value * 2}
				end
			end
			return result
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkAgentStateMachine(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local state = "init"
			local count = 0
			for i = 1, 100 do
				if state == "init" then
					if i > 10 then state = "running" end
				elseif state == "running" then
					count = count + 1
					if count > 50 then state = "stopping" end
				elseif state == "stopping" then
					if i > 90 then state = "done" end
				end
			end
			return state, count
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 2)
		L.Pop(2)
	}
}

func BenchmarkAgentMessageBuild(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local parts = {}
			for i = 1, 20 do
				parts[i] = "field" .. i .. "=" .. (i * 100)
			end
			return table.concat(parts, ",")
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkAgentNestedAccess(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local config = {
			server = {host = "localhost", port = 8080},
			database = {host = "db", port = 5432, pool = 10},
			cache = {enabled = true, ttl = 3600},
		}
		function test()
			local sum = 0
			for i = 1, 500 do
				sum = sum + config.server.port + config.database.port + config.database.pool
				if config.cache.enabled then
					sum = sum + config.cache.ttl
				end
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkAgentBitwiseFlags(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local READ = 1
		local WRITE = 2
		local EXEC = 4
		local ADMIN = 8
		function test()
			local count = 0
			for i = 0, 255 do
				local perms = i
				if (perms & READ) ~= 0 then count = count + 1 end
				if (perms & WRITE) ~= 0 then count = count + 1 end
				if (perms & EXEC) ~= 0 then count = count + 1 end
				if (perms & ADMIN) ~= 0 then count = count + 1 end
				local combined = READ | WRITE
				if (perms & combined) == combined then count = count + 1 end
			end
			return count
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkAgentTableDispatch(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local handlers = {
			create = function() return 1 end,
			read = function() return 2 end,
			update = function() return 3 end,
			delete = function() return 4 end,
		}
		local actions = {"create", "read", "update", "delete", "read", "read", "update", "delete"}
		function test()
			local sum = 0
			for i = 1, 100 do
				for _, action in ipairs(actions) do
					local h = handlers[action]
					if h then sum = sum + h() end
				end
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkCoroutineYieldResume(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function worker()
			local sum = 0
			for i = 1, 10 do
				sum = sum + coroutine.yield(i)
			end
			return sum
		end
		function test()
			local co = coroutine.create(worker)
			local total = 0
			for i = 1, 10 do
				local ok, val = coroutine.resume(co, i)
				if ok then total = total + val end
			end
			return total
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkStringEquality(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local states = {"init", "running", "paused", "stopping", "done"}
			local current = "running"
			local count = 0
			for i = 1, 1000 do
				for _, s in ipairs(states) do
					if current == s then count = count + 1 end
				end
			end
			return count
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTypeChecking(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local values = {1, "hello", true, {}, function() end, 3.14, nil}
			local counts = {number=0, string=0, boolean=0, table=0, ["function"]=0, ["nil"]=0}
			for i = 1, 200 do
				for _, v in ipairs(values) do
					local t = type(v)
					counts[t] = (counts[t] or 0) + 1
				end
			end
			return counts.number
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTableConstruction(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local result = {}
			for i = 1, 50 do
				result[i] = {
					id = i,
					name = "item" .. i,
					metadata = {
						created = 12345 + i,
						modified = 67890 + i,
						tags = {"a", "b", "c"},
					},
					active = i % 2 == 0,
				}
			end
			return #result
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Bitwise operations

func BenchmarkBitwiseAND(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0
			for i = 1, 1000 do
				x = x & 0xFF
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkBitwiseOR(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0
			for i = 1, 1000 do
				x = x | i
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkBitwiseXOR(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0
			for i = 1, 1000 do
				x = x ~ i
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkBitwiseSHL(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 1
			for i = 1, 1000 do
				x = (x << 1) & 0xFFFFFFFF
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkBitwiseSHR(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0xFFFFFFFF
			for i = 1, 1000 do
				x = x >> 1
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkBitwiseNOT(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0
			for i = 1, 1000 do
				x = ~i
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkIntegerDiv(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 1000000
			for i = 1, 1000 do
				x = x // 2
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkFloatAdd(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0.0
			for i = 1, 1000 do
				x = x + 1.5
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkIntegerAdd(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 0
			for i = 1, 1000 do
				x = x + 1
			end
			return x
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Thread/memory benchmarks

func BenchmarkThreadCreation(b *testing.B) {
	L := NewState()
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th := L.NewThreadWithContext(context.TODO())
		_ = th
	}
}

func BenchmarkThreadCreationMinimal(b *testing.B) {
	L := NewState(Options{
		CallStackSize:       16,
		RegistrySize:        64,
		RegistryMaxSize:     128,
		RegistryGrowStep:    32,
		MinimizeStackMemory: true,
	})
	L.OpenLibs()
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th := L.NewThreadWithContext(context.TODO())
		_ = th
	}
}

func BenchmarkThreadWithExecution(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`function worker() return 42 end`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("worker").(*LFunction)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th := L.NewThreadWithContext(context.TODO())
		_, _, _ = L.Resume(th, fn)
		th.Close()
	}
}

func BenchmarkThreadWithContext(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`function worker() return 42 end`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("worker").(*LFunction)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		th := L.NewThreadWithContext(ctx)
		_, _, _ = L.Resume(th, fn)
		th.Close()
	}
}

// Table operations

func BenchmarkTableGetStringKey(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local t = {a=1, b=2, c=3, d=4, e=5, f=6, g=7, h=8}
		function test()
			local sum = 0
			for i = 1, 1000 do
				sum = sum + t.a + t.b + t.c + t.d
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTableSetStringKey(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local t = {}
			for i = 1, 1000 do
				t.a = i
				t.b = i + 1
				t.c = i + 2
				t.d = i + 3
			end
			return t
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTableGetIntKey(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local t = {10, 20, 30, 40, 50, 60, 70, 80}
		function test()
			local sum = 0
			for i = 1, 1000 do
				sum = sum + t[1] + t[2] + t[3] + t[4]
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTableSetIntKey(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local t = {}
			for i = 1, 1000 do
				t[1] = i
				t[2] = i + 1
				t[3] = i + 2
				t[4] = i + 3
			end
			return t
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkTableMixedAccess(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local t = {name="test", value=100, items={1,2,3}}
			local sum = 0
			for i = 1, 500 do
				t.value = t.value + 1
				t.items[1] = i
				sum = sum + t.value + t.items[1]
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// String operations

func BenchmarkVMStringConcat(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local s = ""
			for i = 1, 100 do
				s = s .. "x"
			end
			return s
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkStringTableConcat(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local t = {}
			for i = 1, 100 do
				t[i] = "x"
			end
			return table.concat(t)
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Arithmetic operations

func BenchmarkArithMixed(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local x = 1.5
			local y = 2
			local sum = 0
			for i = 1, 1000 do
				sum = sum + x * y - i / 2 + i % 3
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

func BenchmarkArithIntOnly(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local sum = 0
			for i = 1, 1000 do
				sum = sum + i * 2 - i // 2 + i % 3
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Function calls

func BenchmarkVMFunctionCall(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local function add(a, b)
			return a + b
		end
		function test()
			local sum = 0
			for i = 1, 1000 do
				sum = add(sum, i)
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Local variable access

func BenchmarkLocalAccess(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local a, b, c, d = 1, 2, 3, 4
			local sum = 0
			for i = 1, 1000 do
				sum = sum + a + b + c + d
				a = a + 1
				b = b + 1
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Upvalue access

func BenchmarkUpvalueAccess(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local counter = 0
			local function inc()
				counter = counter + 1
				return counter
			end
			for i = 1, 1000 do
				inc()
			end
			return counter
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Comparison operations

func BenchmarkComparison(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local count = 0
			for i = 1, 1000 do
				if i > 500 then
					count = count + 1
				elseif i < 250 then
					count = count + 2
				else
					count = count + 3
				end
			end
			return count
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Global access

func BenchmarkVMGlobalAccess(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		counter = 0
		function test()
			for i = 1, 1000 do
				counter = counter + 1
			end
			return counter
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// For loop numeric

func BenchmarkForNumeric(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local sum = 0
			for i = 1, 10000 do
				sum = sum + 1
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// For loop with pairs

func BenchmarkForPairs(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local t = {}
		for i = 1, 100 do
			t["key" .. i] = i
		end
		function test()
			local sum = 0
			for k, v in pairs(t) do
				sum = sum + v
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// For loop with ipairs

func BenchmarkForIPairs(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		local t = {}
		for i = 1, 100 do
			t[i] = i
		end
		function test()
			local sum = 0
			for i, v in ipairs(t) do
				sum = sum + v
			end
			return sum
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Table creation

func BenchmarkVMTableCreate(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local tables = {}
			for i = 1, 100 do
				tables[i] = {a=1, b=2, c=3}
			end
			return tables
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Closure creation

func BenchmarkClosureCreate(b *testing.B) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
		function test()
			local funcs = {}
			for i = 1, 100 do
				funcs[i] = function() return i end
			end
			return funcs
		end
	`); err != nil {
		b.Fatal(err)
	}
	fn := L.GetGlobal("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		L.Call(0, 1)
		L.Pop(1)
	}
}

// Bitwise correctness tests

func TestBitwiseAND_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 0xFF,0x0F; return a & b", 0x0F},
		{"local a,b = 255,15; return a & b", 15},
		{"local a,b = 0,0xFFFFFFFF; return a & b", 0},
		{"local a,b = -1,0xFF; return a & b", 0xFF},
		{"local a,b = 0x12345678,0xF0F0F0F0; return a & b", 0x10305070},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d (0x%x), got %d (0x%x)", tc.code, tc.expect, tc.expect, i, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestBitwiseOR_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 0xF0,0x0F; return a | b", 0xFF},
		{"local a,b,c,d = 1,2,4,8; return a | b | c | d", 15},
		{"local a,b = 0,0; return a | b", 0},
		{"local a,b = 0x1234,0x4321; return a | b", 0x5335},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d (0x%x), got %d (0x%x)", tc.code, tc.expect, tc.expect, i, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestBitwiseXOR_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 0xFF,0xFF; return a ~ b", 0},
		{"local a,b = 0xFF,0x00; return a ~ b", 0xFF},
		{"local a,b = 0xAAAA,0x5555; return a ~ b", 0xFFFF},
		{"local a = 123; return a ~ a", 0},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d (0x%x), got %d (0x%x)", tc.code, tc.expect, tc.expect, i, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestBitwiseSHL_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 1,0; return a << b", 1},
		{"local a,b = 1,1; return a << b", 2},
		{"local a,b = 1,8; return a << b", 256},
		{"local a,b = 0xFF,4; return a << b", 0xFF0},
		{"local a,b = 1,62; return a << b", 1 << 62},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d, got %d", tc.code, tc.expect, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestBitwiseSHR_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 256,1; return a >> b", 128},
		{"local a,b = 256,8; return a >> b", 1},
		{"local a,b = 0xFF0,4; return a >> b", 0xFF},
		{"local a,b = 1,1; return a >> b", 0},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d, got %d", tc.code, tc.expect, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestBitwiseNOT_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a = 0; return ~a", -1},
		{"local a = -1; return ~a", 0},
		{"local a = 42; return ~~a", 42},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d, got %d", tc.code, tc.expect, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestIntegerDiv_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local a,b = 10,3; return a // b", 3},
		{"local a,b = 17,5; return a // b", 3},
		{"local a,b = -10,3; return a // b", -4},
		{"local a,b = 10,-3; return a // b", -4},
		{"local a,b = -10,-3; return a // b", 3},
		{"local a,b = 100,1; return a // b", 100},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d, got %d", tc.code, tc.expect, i)
			}
		} else {
			t.Errorf("%s: expected LInteger, got %T", tc.code, v)
		}
	}
}

func TestArithmetic_MixedTypes(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect float64
	}{
		{"return 10 + 0.5", 10.5},
		{"return 10.5 + 10", 20.5},
		{"return 100 - 0.5", 99.5},
		{"return 10 * 1.5", 15.0},
		{"return 10 / 4", 2.5},
		{"return 3 ^ 2", 9.0},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		n := LVAsNumber(v)
		if float64(n) != tc.expect {
			t.Errorf("%s: expected %v, got %v", tc.code, tc.expect, n)
		}
	}
}

func TestArithmetic_IntegerPreservation(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"return 10 + 20", 30},
		{"return 100 - 50", 50},
		{"return 7 * 8", 56},
		{"return 1 + 2 + 3 + 4", 10},
		{"return 10 * 10 * 10", 1000},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		if i, ok := v.(LInteger); ok {
			if int64(i) != tc.expect {
				t.Errorf("%s: expected %d, got %d", tc.code, tc.expect, i)
			}
		} else {
			n := LVAsNumber(v)
			if float64(n) != float64(tc.expect) {
				t.Errorf("%s: expected %d, got %v", tc.code, tc.expect, n)
			}
		}
	}
}

func TestForLoop_IntegerCounter(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{"local s=0; for i=1,10 do s=s+i end; return s", 55},
		{"local s=0; for i=1,100 do s=s+1 end; return s", 100},
		{"local s=0; for i=10,1,-1 do s=s+i end; return s", 55},
		{"local s=0; for i=0,10,2 do s=s+i end; return s", 30},
		{"local s=0; for i=-5,5 do s=s+i end; return s", 0},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		n := LVAsNumber(v)
		if float64(n) != float64(tc.expect) {
			t.Errorf("%s: expected %d, got %v", tc.code, tc.expect, n)
		}
	}
}

func TestIpairs_Correctness(t *testing.T) {
	L := NewState()
	defer L.Close()
	tests := []struct {
		code   string
		expect int64
	}{
		{`local t = {"a", "b", "c"}; local c = 0; for i, v in ipairs(t) do c = c + i end; return c`, 6},
		{`local t = {}; local c = 0; for i, v in ipairs(t) do c = c + 1 end; return c`, 0},
		{`local t = {1, 2, nil, 4}; local c = 0; for i, v in ipairs(t) do c = c + 1 end; return c`, 2},
		{`local t = {}; for i = 1, 100 do t[i] = i end; local s = 0; for i, v in ipairs(t) do s = s + v end; return s`, 5050},
		{`local t = {1}; local ok = true; for i, v in ipairs(t) do if type(i) ~= "number" then ok = false end end; return ok and 1 or 0`, 1},
	}
	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		n := LVAsNumber(v)
		if float64(n) != float64(tc.expect) {
			t.Errorf("%s: expected %d, got %v", tc.code, tc.expect, n)
		}
	}
}

// Memory tests

func TestNewThreadWithContext(t *testing.T) {
	L := NewState()
	defer L.Close()

	var nilCtx context.Context
	th1 := L.NewThreadWithContext(nilCtx)
	if th1 == nil {
		t.Fatal("NewThreadWithContext(context.TODO()) should return a valid thread")
	}
	if th1.ctx != nil {
		t.Error("context should be nil when created with nil")
	}
	th1.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	th2 := L.NewThreadWithContext(ctx)
	if th2 == nil {
		t.Fatal("NewThreadWithContext(ctx) should return a valid thread")
	}
	if th2.ctx != ctx {
		t.Error("context should match the provided context")
	}
	if th2.ctxDone == nil {
		t.Error("ctxDone should be set when context is provided")
	}

	if err := L.DoString(`function test() return 42 end`); err != nil {
		t.Fatal(err)
	}
	fn := L.GetGlobal("test").(*LFunction)
	status, results, err := L.Resume(th2, fn)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if status != ResumeOK {
		t.Errorf("expected ResumeOK, got %v", status)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	th2.Close()
}

func measureMemory() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func TestThreadMemorySize(t *testing.T) {
	before := measureMemory()
	L := NewState()
	after := measureMemory()
	mainStateSize := after - before
	t.Logf("Main LState (with libs): %d bytes (~%.1f KB)", mainStateSize, float64(mainStateSize)/1024)
	t.Logf("sizeof(LState): %d bytes", unsafe.Sizeof(LState{}))
	t.Logf("sizeof(callFrame): %d bytes", unsafe.Sizeof(callFrame{}))
	t.Logf("sizeof(registry): %d bytes", unsafe.Sizeof(registry{}))

	const numCoroutines = 1000
	threads := make([]*LState, numCoroutines)
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	for i := 0; i < numCoroutines; i++ {
		threads[i] = L.NewThreadWithContext(context.TODO())
	}
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	totalAlloc := m2.TotalAlloc - m1.TotalAlloc
	perThread := totalAlloc / numCoroutines
	heapDelta := m2.HeapAlloc - m1.HeapAlloc
	t.Logf("TotalAlloc for %d threads: %d bytes (~%.1f MB)", numCoroutines, totalAlloc, float64(totalAlloc)/(1024*1024))
	t.Logf("Per thread (TotalAlloc): %d bytes (~%.1f KB)", perThread, float64(perThread)/1024)
	t.Logf("HeapAlloc delta: %d bytes (~%.1f MB)", heapDelta, float64(heapDelta)/(1024*1024))
	t.Logf("Estimated threads per 100MB (heap): %d", (100*1024*1024*numCoroutines)/(heapDelta+1))
	t.Logf("Estimated threads per 1GB (heap): %d", (1024*1024*1024*numCoroutines)/(heapDelta+1))
	_ = threads
	L.Close()
}

func TestMinimalThreadMemory(t *testing.T) {
	L := NewState(Options{
		CallStackSize:       16,
		RegistrySize:        64,
		RegistryMaxSize:     128,
		RegistryGrowStep:    32,
		MinimizeStackMemory: true,
		SkipOpenLibs:        true,
	})
	defer L.Close()

	const numCoroutines = 100
	threads := make([]*LState, numCoroutines)
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	for i := 0; i < numCoroutines; i++ {
		threads[i] = L.NewThreadWithContext(context.TODO())
	}
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	totalAlloc := m2.TotalAlloc - m1.TotalAlloc
	perCoroutine := totalAlloc / numCoroutines
	t.Logf("Per minimal coroutine (TotalAlloc): %d bytes (~%.1f KB)", perCoroutine, float64(perCoroutine)/1024)
	if perCoroutine > 0 {
		t.Logf("Estimated minimal threads per 100MB: %d", (100*1024*1024)/perCoroutine)
	}
	_ = threads
}

func TestScaleThreads(t *testing.T) {
	L := NewState(Options{
		CallStackSize:       32,
		RegistrySize:        128,
		RegistryMaxSize:     256,
		RegistryGrowStep:    64,
		MinimizeStackMemory: true,
	})
	L.OpenLibs()
	defer L.Close()

	if err := L.DoString(`
		function worker()
			local sum = 0
			for i = 1, 10 do
				sum = sum + coroutine.yield(i)
			end
			return sum
		end
	`); err != nil {
		t.Fatal(err)
	}

	const targetThreads = 1000
	before := measureMemory()
	threads := make([]*LState, targetThreads)
	for i := 0; i < targetThreads; i++ {
		threads[i] = L.NewThreadWithContext(context.TODO())
	}
	after := measureMemory()
	totalMem := after - before
	perThread := totalMem / targetThreads
	t.Logf("Created %d threads", targetThreads)
	t.Logf("Total memory: %d bytes (~%.1f MB)", totalMem, float64(totalMem)/(1024*1024))
	t.Logf("Per thread: %d bytes (~%.1f KB)", perThread, float64(perThread)/1024)

	before = measureMemory()
	fn := L.GetGlobal("worker")
	for i := 0; i < targetThreads; i++ {
		th := threads[i]
		th.Push(fn)
	}
	after = measureMemory()
	loadedMem := after - before
	t.Logf("Additional memory after loading function in all threads: %d bytes (~%.1f KB)", loadedMem, float64(loadedMem)/1024)
}
