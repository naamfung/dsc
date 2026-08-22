package check

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

func BenchmarkCheck_Simple(b *testing.B) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	source := `
local x = 1
local y = 2
return x + y
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(source, "test.lua")
	}
}

func BenchmarkCheck_NestedFunctions(b *testing.B) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	source := `
local function add(a, b)
	return a + b
end

local function sub(a, b)
	return a - b
end

local function mul(a, b)
	return a * b
end

return add(1, sub(2, mul(3, 4)))
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(source, "test.lua")
	}
}

func BenchmarkCheck_Conditionals(b *testing.B) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	source := `
local function process(x)
	if x == nil then
		return 0
	elseif type(x) == "number" then
		return x * 2
	elseif type(x) == "string" then
		return #x
	else
		return -1
	end
end
return process(arg)
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(source, "test.lua")
	}
}

func BenchmarkCheck_Loops(b *testing.B) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	source := `
local function sum(n)
	local total = 0
	for i = 1, n do
		total = total + i
	end
	return total
end

local function sumTable(t)
	local total = 0
	for k, v in pairs(t) do
		total = total + v
	end
	return total
end

return sum(100), sumTable({1, 2, 3})
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(source, "test.lua")
	}
}

func BenchmarkCheck_LargeFunction(b *testing.B) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	source := `
local function complex(a, b, c, d, e)
	local x = a + b
	local y = c * d
	local z = e or 0

	if x ~= nil then
		x = x - y
	else
		y = y - x
	end

	local result = 0
	local i = 1
	while i <= 10 do
		if i % 2 == 0 then
			result = result + x
		else
			result = result + y
		end
		i = i + 1
	end

	local function inner(v)
		return v * z
	end

	return inner(result)
end

return complex(1, 2, 3, 4, 5)
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(source, "test.lua")
	}
}
