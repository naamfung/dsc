package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression guard: after not_nil + length assertion on an optional array-like
// field, indexing must remain valid in the same flow segment.
func TestNotNilAfterLenAssert_AllowsIndex(t *testing.T) {
	source := `
		local test = {}

		function test.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error((msg or "assertion failed") .. ": expected non-nil", 2)
			end
			return val
		end

		local function map_options(contract_options)
			local openai_options = {}
			if contract_options.stop_sequences then
				openai_options.stop = contract_options.stop_sequences
			end
			return openai_options
		end

		local contract_options = { stop_sequences = {"STOP", "END"} }
		local openai_options = map_options(contract_options)
		test.not_nil(openai_options.stop)
		test.eq(#openai_options.stop, 2)
		local first = openai_options.stop[1]
		local second = openai_options.stop[2]
		return first, second
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Mirrors framework/src/llm/src/openai/mapper.lua map_options shape more closely.
func TestNotNilAfterLenAssert_AllowsIndex_OpenAIMapOptionsShape(t *testing.T) {
	source := `
		local test = {}

		function test.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error((msg or "assertion failed") .. ": expected non-nil", 2)
			end
			return val
		end

		local function map_options(contract_options)
			if not contract_options then return {} end

			local openai_options = {}
			local is_reasoning_request = contract_options.reasoning_model_request == true

			if contract_options.max_tokens then
				if is_reasoning_request then
					openai_options.max_completion_tokens = contract_options.max_tokens
				else
					openai_options.max_tokens = contract_options.max_tokens
				end
			end

			if is_reasoning_request and contract_options.thinking_effort then
				local effort = contract_options.thinking_effort
				if effort < 25 then
					openai_options.reasoning_effort = "low"
				elseif effort < 75 then
					openai_options.reasoning_effort = "medium"
				else
					openai_options.reasoning_effort = "high"
				end
			else
				if contract_options.temperature ~= nil and not is_reasoning_request then
					openai_options.temperature = contract_options.temperature
				end
			end

			openai_options.top_p = contract_options.top_p
			openai_options.frequency_penalty = contract_options.frequency_penalty
			openai_options.presence_penalty = contract_options.presence_penalty
			openai_options.seed = contract_options.seed
			openai_options.user = contract_options.user

			if contract_options.stop_sequences then
				openai_options.stop = contract_options.stop_sequences
			end

			return openai_options
		end

		local contract_options = {
			temperature = 0.7,
			max_tokens = 150,
			top_p = 0.9,
			frequency_penalty = 0.5,
			presence_penalty = 0.3,
			stop_sequences = {"STOP", "END"},
			seed = 42,
			user = "test-user",
		}

		local openai_options = map_options(contract_options)
		test.not_nil(openai_options.stop)
		test.eq(#openai_options.stop, 2)
		local first = openai_options.stop[1]
		local second = openai_options.stop[2]
		return first, second
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Mirrors framework/src/llm/src/openai/mapper_test.lua behavior:
// imported test.not_nil + test.eq summaries, and an untyped map_options builder.
func TestImportedNotNilAfterLenAssert_OpenAIMapOptionsShape(t *testing.T) {
	assertManifest := io.NewManifest("test")
	assertExport := typ.NewRecord().
		Field("not_nil", typ.Func().
			Param("x", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Field("eq", typ.Func().
			Param("actual", typ.Any).
			Param("expected", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Build()
	assertManifest.SetExport(assertExport)

	notNilSummary := io.NewSummary([]typ.Type{typ.Any, typ.NewOptional(typ.String)}, nil)
	notNilSummary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})
	assertManifest.DefineSummary("not_nil", notNilSummary)

	eqSummary := io.NewSummary([]typ.Type{typ.Any, typ.Any, typ.NewOptional(typ.String)}, nil)
	eqSummary.Ensures = constraint.FromConstraints(constraint.EqPath{
		Left:  constraint.ParamPath(0),
		Right: constraint.ParamPath(1),
	})
	assertManifest.DefineSummary("eq", eqSummary)

	source := `
		local test = require("test")

		local function map_options(contract_options)
			if not contract_options then return {} end
			local openai_options = {}
			if contract_options.stop_sequences then
				openai_options.stop = contract_options.stop_sequences
			end
			return openai_options
		end

		local contract_options = { stop_sequences = {"STOP", "END"} }
		local openai_options = map_options(contract_options)
		test.not_nil(openai_options.stop)
		test.eq(#openai_options.stop, 2)
		local first = openai_options.stop[1]
		local second = openai_options.stop[2]
		return first, second
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("test", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
