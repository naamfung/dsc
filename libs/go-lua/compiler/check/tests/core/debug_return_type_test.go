package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestDebugTypedEntriesFlow(t *testing.T) {
	source := `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function group_by_suite(entries: {Entry})
    local suites = {}
    local no_suite = {}

    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    return suites, no_suite
end

local entries: {Entry} = {}
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

-- Test accessing suites via sorted_keys result
for idx, name in ipairs(suite_names) do
    -- What type is suites[name]?
    local tests: {Entry} = suites[name]
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic: %s", d.Message)
	}
	if result.HasError() {
		t.Fatalf("expected no errors, got errors")
	}
}
