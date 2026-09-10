package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression: metatable-backed colon method calls with dynamic option payloads
// should not misclassify non-self first params as implicit receiver.
func TestMetatableColonCallArityPatterns(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "runner self calls any_optional arg method",
			source: `
local Runner = {}
Runner.__index = Runner

function Runner:greet(name: string): string
	return "hello " .. name
end

function Runner:find_items(options: any?): ({any}?, string?)
	return {}, nil
end

function Runner:run(options: any?): any
	options = options or {}
	local items, err = self:find_items(options)
	if err then
		return nil
	end
	local result = self:greet("world")
	return result
end

local function setup(id: string): any
	local self = setmetatable({}, Runner)
	self.id = id
	return self
end

local function main(): boolean
	local r = setup("test")
	local result = r:run()
	return result == "hello world"
end

return { main = main }
`,
		},
		{
			name: "builder chaining through self and returned receiver",
			source: `
local QueryBuilder = {}
QueryBuilder.__index = QueryBuilder

function QueryBuilder:with_filter(filter: any): any
	self.filter = filter
	return self
end

function QueryBuilder:execute(options: any): ({string}, string?)
	local out = { "row" }
	if options and options.limit then
		out[2] = tostring(options.limit)
	end
	return out, nil
end

function QueryBuilder:run(filter: any): number
	local first, err = self:execute({ limit = 10 })
	if err then
		return 0
	end

	local chained = self:with_filter(filter)
	local second, err2 = chained:execute({ limit = 5 })
	if err2 then
		return 0
	end

	return #first + #second
end

local function new_builder(): any
	return setmetatable({}, QueryBuilder)
end

local function main(): boolean
	local builder = new_builder()
	local count = builder:run({ kind = "active" })
	return count == 4
end

return { main = main }
`,
		},
		{
			name: "typed options with top-like setup receiver",
			source: `
type RunnerOptions = {
	tags: {string}?,
	allowed_ids: {string}?,
	count: number?,
}

local Runner = {}
Runner.__index = Runner

function Runner:find_migrations(options: RunnerOptions?): ({any}?, string?)
	options = options or {}
	return {}, nil
end

function Runner:run(options: RunnerOptions?): any
	options = options or {}
	local migrations, err = self:find_migrations(options)
	if err then
		return nil
	end
	return migrations
end

function Runner:status(options: RunnerOptions?): any
	local migrations, err = self:find_migrations(options)
	if err then
		return nil
	end
	return migrations
end

local function setup(id: string): any
	local self = setmetatable({}, Runner)
	self.id = id
	return self
end

local function main(): boolean
	local r = setup("db-main")
	return r ~= nil
end

return { main = main }
`,
		},
		{
			name: "repository flow with options bag",
			source: `
local UserRepo = {}
UserRepo.__index = UserRepo

function UserRepo:find(options: any?): ({any}?, string?)
	if options and options.fail then
		return nil, "query failed"
	end
	return { { id = "u1" } }, nil
end

function UserRepo:first(options: any?): any?
	local rows, err = self:find(options)
	if err or not rows or #rows == 0 then
		return nil
	end
	return rows[1]
end

local function new_repo(): any
	return setmetatable({}, UserRepo)
end

local function main(): boolean
	local repo = new_repo()
	local user = repo:first({ limit = 1 })
	return user ~= nil
end

return { main = main }
`,
		},
		{
			name: "worker pipeline with multiple method hops",
			source: `
local Worker = {}
Worker.__index = Worker

function Worker:prepare(payload: any): (any, string?)
	return { prepared = payload }, nil
end

function Worker:dispatch(payload: any): (boolean, string?)
	local prepared, err = self:prepare(payload)
	if err then
		return false, err
	end
	return prepared ~= nil, nil
end

function Worker:run(payload: any): boolean
	local ok, err = self:dispatch(payload)
	if err then
		return false
	end
	return ok
end

local function new_worker(): any
	return setmetatable({}, Worker)
end

local function main(): boolean
	local worker = new_worker()
	return worker:run({ task = "sync" }) == true
end

return { main = main }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.source, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
