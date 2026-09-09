package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestLoops_NumericFor(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "basic for loop",
			Code:      `for i = 1, 10 do local x: integer = i end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "for loop with step",
			Code:      `for i = 1, 10, 2 do local x: number = i end`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name:      "loop variable not available outside",
			Code:      `for i = 1, 10 do end; local x: number = i`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestLoops_GenericFor(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "ipairs loop assigns correct types",
			Code: `
				local arr: {number} = {1, 2, 3}
				for i, v in ipairs(arr) do
					local n: number = v
					local idx: integer = i
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pairs loop assigns correct types",
			Code: `
				local t: {[string]: number} = {a = 1, b = 2}
				for k, v in pairs(t) do
					local key: string = k
					local val: number = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "ipairs loop index is integer",
			Code: `
				local arr: {string} = {"a", "b"}
				for i, v in ipairs(arr) do
					local x: string = i
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "loop variables not available outside",
			Code: `
				local arr: {number} = {1, 2}
				for i, v in ipairs(arr) do end
				local x: number = v
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestLoops_While(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "while loop basic",
			Code: `
				local i = 0
				while i < 10 do
					i = i + 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "while loop with break",
			Code: `
				local i = 0
				while true do
					i = i + 1
					if i > 10 then break end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestLoops_Repeat(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "repeat until basic",
			Code: `
				local i = 0
				repeat
					i = i + 1
				until i > 10
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "repeat until can use local from body",
			Code: `
				local done = false
				repeat
					local x = 1
					done = true
				until done
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestLoops_TableMutation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "table.insert in loop propagates type",
			Code: `
				local chunks = {}
				while true do
					local chunk: string = "chunk"
					table.insert(chunks, chunk)
					break
				end
				for _, chunk in ipairs(chunks) do
					local s: string = chunk
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested block table mutation",
			Code: `
				local chunks = {}
				while true do
					do
						local chunk: string = "data"
						table.insert(chunks, chunk)
					end
					break
				end
				for _, chunk in ipairs(chunks) do
					local s: string = chunk
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "building table with dynamic keys in loop",
			Code: `
				local suites = {}
				local entries = {{meta={suite="a"}}, {meta={suite="b"}}}

				for _, entry in ipairs(entries) do
					local suite = entry.meta and entry.meta.suite
					if suite then
						suites[suite] = suites[suite] or {}
						table.insert(suites[suite], entry)
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "building array with index in loop",
			Code: `
				local uuids = {}
				for i = 1, 10 do
					local id = tostring(i)
					uuids[i] = id
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "insert then call methods via metatable",
			Code: `
				local Resource = {}

				local function createResource()
					return setmetatable({}, {__index = Resource})
				end

				local resources = {}
				for i = 1, 3 do
					table.insert(resources, createResource())
				end

				for i = #resources, 1, -1 do
					resources[i]:close()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "bounded loop access is safe",
			Code: `
				local items = {}
				for i = 1, 5 do
					table.insert(items, {name = "item" .. i, process = function(self) end})
				end

				for i = 1, #items do
					items[i]:process()
				end

				for i = #items, 1, -1 do
					items[i]:process()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
