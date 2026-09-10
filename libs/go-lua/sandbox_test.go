package lua

import (
	"testing"
)

func TestSandbox_BasicArithmetic(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		code   string
		expect LNumber
	}{
		{"return 1 + 2", 3},
		{"return 10 - 3", 7},
		{"return 4 * 5", 20},
		{"return 15 / 3", 5},
		{"return 17 % 5", 2},
		{"return 2 ^ 10", 1024},
		{"return -5", -5},
	}

	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		result := LVAsNumber(L.Get(-1))
		L.Pop(1)
		if result != tc.expect {
			t.Errorf("%s: expected %v, got %v", tc.code, tc.expect, result)
		}
	}
}

func TestSandbox_IntegerArithmetic(t *testing.T) {
	L := NewState()
	defer L.Close()

	tests := []struct {
		code   string
		expect int64
	}{
		{"return 100 + 200", 300},
		{"return 1000 - 1", 999},
		{"return 7 * 8", 56},
		{"return 100 / 10", 10},
	}

	for _, tc := range tests {
		if err := L.DoString(tc.code); err != nil {
			t.Errorf("%s: %v", tc.code, err)
			continue
		}
		v := L.Get(-1)
		L.Pop(1)
		switch n := v.(type) {
		case LInteger:
			if int64(n) != tc.expect {
				t.Errorf("%s: expected %v, got %v", tc.code, tc.expect, n)
			}
		case LNumber:
			if int64(n) != tc.expect {
				t.Errorf("%s: expected %v, got %v (as float)", tc.code, tc.expect, n)
			}
		default:
			t.Errorf("%s: unexpected type %T", tc.code, v)
		}
	}
}

func TestSandbox_Tables(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local t = {1, 2, 3, foo = "bar"}
		return t[1], t[2], t[3], t.foo
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-4)) != 1 {
		t.Error("t[1] != 1")
	}
	if LVAsNumber(L.Get(-3)) != 2 {
		t.Error("t[2] != 2")
	}
	if LVAsNumber(L.Get(-2)) != 3 {
		t.Error("t[3] != 3")
	}
	if LVAsString(L.Get(-1)) != "bar" {
		t.Error("t.foo != 'bar'")
	}
}

func TestSandbox_Functions(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local function add(a, b)
			return a + b
		end
		return add(10, 20)
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-1)) != 30 {
		t.Errorf("expected 30, got %v", L.Get(-1))
	}
}

func TestSandbox_Pcall(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local ok, err = pcall(function()
			error("test error")
		end)
		return ok, err
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	ok := L.Get(-2)
	if ok != LFalse {
		t.Error("pcall should return false on error")
	}
}

func TestSandbox_Iteration(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local sum = 0
		local t = {10, 20, 30}
		for i, v in ipairs(t) do
			sum = sum + v
		end
		return sum
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-1)) != 60 {
		t.Errorf("expected 60, got %v", L.Get(-1))
	}
}

func TestSandbox_Pairs(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local count = 0
		local t = {a = 1, b = 2, c = 3}
		for k, v in pairs(t) do
			count = count + 1
		end
		return count
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-1)) != 3 {
		t.Errorf("expected 3, got %v", L.Get(-1))
	}
}

func TestSandbox_Metatables(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local mt = {
			__add = function(a, b) return a.value + b.value end
		}
		local a = setmetatable({value = 10}, mt)
		local b = setmetatable({value = 20}, mt)
		return a + b
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-1)) != 30 {
		t.Errorf("expected 30, got %v", L.Get(-1))
	}
}

func TestSandbox_Strings(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local s = "hello"
		return #s, s .. " world"
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-2)) != 5 {
		t.Error("#s != 5")
	}
	if LVAsString(L.Get(-1)) != "hello world" {
		t.Error("concat failed")
	}
}

func TestSandbox_StringGmatch(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local s = "hello world foo bar"
		local result = {}
		for word in string.gmatch(s, "%w+") do
			table.insert(result, word)
		end
		return table.concat(result, ",")
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	result := LVAsString(L.Get(-1))
	if result != "hello,world,foo,bar" {
		t.Errorf("gmatch failed: got %q, expected %q", result, "hello,world,foo,bar")
	}
}

func TestSandbox_StringGmatchCaptures(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local s = "key1=val1;key2=val2"
		local result = {}
		for k, v in string.gmatch(s, "(%w+)=(%w+)") do
			table.insert(result, k .. ":" .. v)
		end
		return table.concat(result, ",")
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	result := LVAsString(L.Get(-1))
	if result != "key1:val1,key2:val2" {
		t.Errorf("gmatch with captures failed: got %q, expected %q", result, "key1:val1,key2:val2")
	}
}

func TestSandbox_ForLoop(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local sum = 0
		for i = 1, 100 do
			sum = sum + i
		end
		return sum
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-1)) != 5050 {
		t.Errorf("expected 5050, got %v", L.Get(-1))
	}
}

func TestSandbox_Closures(t *testing.T) {
	L := NewState()
	defer L.Close()

	code := `
		local function counter()
			local count = 0
			return function()
				count = count + 1
				return count
			end
		end
		local c = counter()
		return c(), c(), c()
	`
	if err := L.DoString(code); err != nil {
		t.Fatal(err)
	}

	if LVAsNumber(L.Get(-3)) != 1 {
		t.Error("first call != 1")
	}
	if LVAsNumber(L.Get(-2)) != 2 {
		t.Error("second call != 2")
	}
	if LVAsNumber(L.Get(-1)) != 3 {
		t.Error("third call != 3")
	}
}

func TestSandbox_RemovedFunctions(t *testing.T) {
	L := NewState()
	defer L.Close()

	removed := []string{
		"dofile", "loadfile", "loadstring", "load",
		"module", "require", "setfenv", "getfenv",
		"collectgarbage", "newproxy",
	}

	for _, fn := range removed {
		v := L.GetGlobal(fn)
		if v != LNil {
			t.Errorf("%s should not exist in sandbox, got %v", fn, v)
		}
	}
}

func TestSandbox_AvailableFunctions(t *testing.T) {
	L := NewState()
	defer L.Close()

	available := []string{
		"assert", "error", "pcall", "xpcall",
		"getmetatable", "setmetatable",
		"ipairs", "pairs", "next",
		"print", "type", "tostring", "tonumber",
		"rawget", "rawset", "rawequal",
		"select", "unpack",
	}

	for _, fn := range available {
		v := L.GetGlobal(fn)
		if v == LNil {
			t.Errorf("%s should exist in sandbox", fn)
		}
	}
}
