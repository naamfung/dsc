package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_HttpTimeoutOptionInference(t *testing.T) {
	source := `
type HttpOptions = {
	headers?: {[string]: string},
	timeout?: number,
	body?: string,
	stream?: boolean
}

local http_client = {
	post = function(url: string, opts: HttpOptions): {status_code: number, body: string}
		return {status_code = 200, body = ""}
	end
}

local function build_context_values(): {[string]: any}
	return {}
end

local options = build_context_values()
local config = build_context_values()

local timeout_val = 600
local timeout_custom = tonumber(options.timeout or config.timeout)
if timeout_custom then
	timeout_val = timeout_custom
end

local headers: {[string]: string} = { ["content-type"] = "application/json" }
local http_options = {
	headers = headers,
	timeout = timeout_val,
}

if true then
	http_options.body = "{}"
	if true then
		http_options.stream = true
	end
end

local _ = http_client.post("https://example.local", http_options)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for timeout option inference")
	}
}

func TestRegression_HttpTimeoutValueNarrowing(t *testing.T) {
	source := `
local options: {[string]: any} = {}
local config: {[string]: any} = {}

local timeout_val = 600
local timeout_custom = tonumber(options.timeout or config.timeout)
if timeout_custom then
	timeout_val = timeout_custom
end

local must_be_number: number = timeout_val
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected timeout_val to narrow to number after tonumber guard")
	}
}

func TestRegression_RecordMutationPreservesUnchangedFieldType(t *testing.T) {
	source := `
local timeout_val = 600
local opts = {
	timeout = timeout_val,
}

opts.body = "{}"
opts.stream = true

local must_be_number: number = opts.timeout
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected record mutation to preserve timeout:number")
	}
}
