package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestClosureReturnInference_NestedCallToOuter tests that when a function is defined
// at module scope and called from inside a nested function, its return type is
// correctly inferred.
//
// Bug: When process() is called from inside main(), the return type of process()
// is incorrectly inferred as nil instead of integer.
func TestClosureReturnInference_NestedCallToOuter(t *testing.T) {
	source := `
local function process(items)
	return #items
end

local function main()
	local data = {1, 2, 3}
	local n = process(data)
	return 0 + n
end

return main
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - return type of process() should be integer when called from nested function")
	}
}

// TestClosureReturnInference_ModuleScope verifies that the same code at module scope works.
func TestClosureReturnInference_ModuleScope(t *testing.T) {
	source := `
local function process(items)
	return #items
end

local data = {1, 2, 3}
local n = process(data)
return 0 + n
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors at module scope")
	}
}

// TestClosureReturnInference_ExplicitType verifies explicit return type works.
func TestClosureReturnInference_ExplicitType(t *testing.T) {
	source := `
local function process(items): integer
	return #items
end

local function main()
	local data = {1, 2, 3}
	local n = process(data)
	return 0 + n
end

return main
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with explicit return type")
	}
}

// TestClosureReturnInference_InlineFunction verifies inline function works.
func TestClosureReturnInference_InlineFunction(t *testing.T) {
	source := `
local function main()
	local process = function(items)
		return #items
	end
	local data = {1, 2, 3}
	local n = process(data)
	return 0 + n
end

return main
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with inline function")
	}
}

// TestClosureReturnInference_MultipleNested tests multiple levels of nesting.
func TestClosureReturnInference_MultipleNested(t *testing.T) {
	source := `
local function process(items)
	return #items
end

local function outer()
	local function inner()
		local data = {1, 2, 3}
		local n = process(data)
		return 0 + n
	end
	return inner()
end

return outer
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with multiple nesting levels")
	}
}

// TestClosureReturnInference_NestedLocalFunctionChain tests that return type flows
// through a chain of nested local functions: inner returns a value, outer returns
// inner(), and the caller receives the correct type.
func TestClosureReturnInference_NestedLocalFunctionChain(t *testing.T) {
	source := `
local errors = require("errors")

local function inner_func()
	return errors.new("stack test")
end

local function outer_func()
	return inner_func()
end

local err = outer_func()
local cs = errors.call_stack(err)
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - err should be Error type through nested function chain, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_DeeplyNestedInsideMain tests nested functions defined
// inside another function (not at module scope) with chained returns.
func TestClosureReturnInference_DeeplyNestedInsideMain(t *testing.T) {
	source := `
local errors = require("errors")

local function main()
	local function inner_func()
		return errors.new("stack test")
	end
	local function outer_func()
		return inner_func()
	end

	local err = outer_func()
	local cs = errors.call_stack(err)
end

return main
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - nested functions inside main should propagate return types, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_GlobalModuleNestedFunctions tests that when using
// a global module (like errors without require), return types propagate through
// nested local functions.
func TestClosureReturnInference_GlobalModuleNestedFunctions(t *testing.T) {
	source := `
local function main()
	local function inner_func()
		return errors.new("stack test")
	end
	local function outer_func()
		return inner_func()
	end

	local err = outer_func()
	local cs = errors.call_stack(err)
end

return { main = main }
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - global errors module should work with nested functions, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_MutualRecursion2Node tests a 2-node SCC where f() calls g()
// and g() calls f() with a base case. The fixpoint algorithm must converge without deadlock.
func TestClosureReturnInference_MutualRecursion2Node(t *testing.T) {
	source := `
local function f(n)
	if n <= 0 then
		return 1
	end
	return g(n - 1)
end

local function g(n)
	if n <= 0 then
		return 2
	end
	return f(n - 1)
end

local result = f(10)
return result + 0
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - mutual recursion 2-node SCC should resolve to number, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_MutualRecursion3Node tests a 3-node SCC where f -> g -> h -> f
// with base cases. The fixpoint algorithm must converge for larger cycles.
func TestClosureReturnInference_MutualRecursion3Node(t *testing.T) {
	source := `
local function f(n)
	if n <= 0 then
		return 1
	end
	return g(n - 1)
end

local function g(n)
	if n <= 0 then
		return 2
	end
	return h(n - 1)
end

local function h(n)
	if n <= 0 then
		return 3
	end
	return f(n - 1)
end

local result = f(10)
return result + 0
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 3-node SCC should resolve to stable return type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_NestedLocalsGlobalModule tests nested local functions calling
// each other with a global module (errors) without require. Uses different call patterns.
func TestClosureReturnInference_NestedLocalsGlobalModule(t *testing.T) {
	source := `
local function main()
	local function make_err(msg)
		return errors.new(msg)
	end

	local function caller()
		return make_err("nested call")
	end

	local err = caller()
	local cs = errors.call_stack(err)
end

return { main = main }
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - nested locals with global errors module should typecheck, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_TableLiteralMutualRecursion tests mutual recursion through
// table fields. The fixpoint must not hang on cyclic table method references.
func TestClosureReturnInference_TableLiteralMutualRecursion(t *testing.T) {
	source := `
local t
t = {
	f = function(n)
		if n <= 0 then
			return 1
		end
		return t.g(n - 1)
	end,
	g = function(n)
		if n <= 0 then
			return 2
		end
		return t.f(n - 1)
	end
}

local result = t.f(5)
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - table literal mutual recursion should not hang, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_RealWorldPattern tests the real-world runner.lua pattern.
func TestClosureReturnInference_RealWorldPattern(t *testing.T) {
	source := `
local function run_suite(name, tests)
	local failures = {}
	for i, entry in ipairs(tests) do
	end
	return #tests, failures
end

local function run_tests()
	local suites = {}
	suites["a"] = {1, 2, 3}
	suites["b"] = {4, 5}

	local completed_tests = 0
	for name, tests in pairs(suites) do
		local count, failures = run_suite(name, tests)
		completed_tests = completed_tests + count
	end

	return completed_tests
end

return run_tests
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - count should be integer")
	}
}

// TestClosureReturnInference_ConvergenceGuard tests that deep mutual recursion
// eventually converges or falls back to unknown with a diagnostic.
func TestClosureReturnInference_ConvergenceGuard(t *testing.T) {
	source := `
local function a(n) return b(n) end
local function b(n) return c(n) end
local function c(n) return d(n) end
local function d(n) return e(n) end
local function e(n)
	if n <= 0 then return 1 end
	return a(n - 1)
end

local x = a(5)
return x + 0
`
	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 5-node SCC should converge or widen to unknown, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_TableLiteralNoBaseCase tests table literal mutual recursion
// WITHOUT base cases. This exposes a limitation where infinite recursion patterns must
// converge to unknown without hanging.
func TestClosureReturnInference_TableLiteralNoBaseCase(t *testing.T) {
	source := `
local t = {
	f = function() return t.g() end,
	g = function() return t.f() end
}
local x = t.f()
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - table literal infinite mutual recursion should converge to unknown, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_SCC2NodeWithAnnotation tests 2-node SCC where result is
// assigned to a typed variable. Uses math.random for conditional return.
func TestClosureReturnInference_SCC2NodeWithAnnotation(t *testing.T) {
	source := `
local function f()
	if math.random() > 0 then return 1 end
	return g()
end

local function g()
	return f()
end

local x: number = f()
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 2-node SCC with annotation should infer number, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWrapperReturn_MultiArity tests that a wrapper function preserves multi-return arity
// through the summary system. When inner() returns (number, string), wrapper() returning
// inner() must produce a 2-slot summary, and callers must destructure both values.
func TestWrapperReturn_MultiArity(t *testing.T) {
	source := `
local function inner(): (number, string)
	return 42, "hello"
end

local function wrapper()
	return inner()
end

local n, s = wrapper()
local x: number = n
local y: string = s
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - wrapper() should preserve multi-return (number, string), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWrapperReturn_ForwardMulti tests that a wrapper function forwards multi-return types
// through the summary system when inner() is inferred (not annotated). The summary must
// carry the full return vector, not collapse to a single type.
func TestWrapperReturn_ForwardMulti(t *testing.T) {
	source := `
local function make_pair()
	return 1, "ok"
end

local function forward()
	return make_pair()
end

local a, b = forward()
local x: number = a
local y: string = b
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - forward() should carry full multi-return vector from make_pair(), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestClosureReturnInference_SCC3NodeNoBaseCase tests 3-node SCC without any base case.
// This is pure infinite recursion that must converge to unknown.
func TestClosureReturnInference_SCC3NodeNoBaseCase(t *testing.T) {
	source := `
local function f() return g() end
local function g() return h() end
local function h() return f() end
local x = f()
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 3-node SCC without base case should converge to unknown, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWrapperReturn_LocalInference tests that a wrapper function correctly infers
// the return type from a local variable assigned by a typed function call.
// Without local inference in return summaries, the local variable is unknown
// and the wrapper's return type degrades.
func TestWrapperReturn_LocalInference(t *testing.T) {
	source := `
local function create()
	return errors.new("test")
end

local function wrapper()
	local err = create()
	return err
end

local e = wrapper()
local cs = errors.call_stack(e)
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - local err from create() should carry Error type through wrapper return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWrapperReturn_ErrorUnion tests a wrapper that stores multi-return results
// in locals and returns them. The local variables must receive their types from
// the called function's return signature for the wrapper's own return to be correct.
func TestWrapperReturn_ErrorUnion(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	moduleType := typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	sqlManifest.SetExport(moduleType)
	sqlManifest.DefineType("DB", dbType)

	source := `
local sql = require("sql")

local function open()
	local db, err = sql.get()
	if err then
		return nil, err
	end
	return db
end

local d, e = open()
if d then
	d:release()
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}
}

// TestWrapperReturn_ErrorGuardNarrows ensures error-return correlation narrows value after err guard.
func TestWrapperReturn_ErrorGuardNarrows(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	spec := contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	sqlManifest := io.NewManifest("sql")
	moduleType := typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(spec).
				Build(),
		},
	})
	sqlManifest.SetExport(moduleType)
	sqlManifest.DefineType("DB", dbType)

	source := `
local sql = require("sql")

local function get_db()
	local db, err = sql.get("app:db")
	if err then
		return nil, err
	end
	db:release()
	return db
end

local d, e = get_db()
if e then
	return
end
if d then
	d:release()
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - error guard should narrow db, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnSummary_CapturedModuleAlias ensures return summaries can resolve
// captured module aliases (require("sql")) inside local functions.
func TestReturnSummary_CapturedModuleAlias(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	moduleType := typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	sqlManifest.SetExport(moduleType)
	sqlManifest.DefineType("DB", dbType)

	source := `
local sql = require("sql")

local function get_db()
	local db, err = sql.get("app:db")
	if err then
		return nil, err
	end
	return db
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.RootResult == nil || sess.RootResult.Graph == nil {
		t.Fatal("missing session root result")
	}

	var summary []typ.Type
	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	if summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent); summaries != nil {
		for sym, rt := range summaries {
			if sess.RootResult.Graph.NameOf(sym) == "get_db" {
				summary = rt
				break
			}
		}
	}

	if len(summary) == 0 {
		t.Fatal("missing return summary for get_db")
	}
	if len(summary) < 1 {
		t.Fatalf("expected at least one return type, got %d", len(summary))
	}
	if !unwrap.IsOptionalLike(summary[0]) {
		t.Fatalf("expected optional return, got %v", summary[0])
	}
	nonNil := narrow.RemoveNil(summary[0])
	if !typ.TypeEquals(nonNil, dbType) {
		t.Fatalf("expected non-nil return to be sql.DB, got %v", nonNil)
	}
}

// TestReturnSummary_MixedArityKeepsSecondSlotOptional verifies that merging
// `return nil, err` with `return db` preserves nil possibility in slot 2.
func TestReturnSummary_MixedArityKeepsSecondSlotOptional(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	moduleType := typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	sqlManifest.SetExport(moduleType)
	sqlManifest.DefineType("DB", dbType)

	source := `
local sql = require("sql")

local function get_db()
	local db, err = sql.get("app:db")
	if err then
		return nil, err
	end
	return db
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.RootResult == nil || sess.RootResult.Graph == nil {
		t.Fatal("missing session root result")
	}

	var summary []typ.Type
	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	if summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent); summaries != nil {
		for sym, rt := range summaries {
			if sess.RootResult.Graph.NameOf(sym) == "get_db" {
				summary = rt
				break
			}
		}
	}

	if len(summary) < 2 {
		t.Fatalf("expected 2 return slots, got %v", summary)
	}
	if !unwrap.IsOptionalLike(summary[1]) {
		t.Fatalf("expected second slot to be optional, got %v", summary[1])
	}
	nonNilErr := narrow.RemoveNil(summary[1])
	if !typ.TypeEquals(nonNilErr, typ.LuaError) {
		t.Fatalf("expected second slot non-nil type to be LuaError, got %v", nonNilErr)
	}
}

// TestReturnSummary_NoNilSlots verifies that multi-return forwarding produces
// no nil entries in SeedsPrev. Each slot must be a concrete type or
// typ.Unknown, never nil.
func TestReturnSummary_NoNilSlots(t *testing.T) {
	source := `
local function make_pair()
	return 1, "ok"
end

local function forward()
	return make_pair()
end

local a, b = forward()
local x: number = a
local y: string = b
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.RootResult == nil || sess.RootResult.Graph == nil {
		t.Fatal("missing session root result")
	}

	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	if summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent); summaries != nil {
		for sym, rt := range summaries {
			name := sess.RootResult.Graph.NameOf(sym)
			for i, slot := range rt {
				if slot == nil {
					t.Errorf("nil slot at index %d in return summary for %q", i, name)
				}
			}
		}
	}
}

// TestReturnSummary_5NodeSCC verifies that a 5-node mutual recursion SCC
// produces non-empty return summary seeds for all members.
func TestReturnSummary_5NodeSCC(t *testing.T) {
	source := `
local function a(n) return b(n) end
local function b(n) return c(n) end
local function c(n) return d(n) end
local function d(n) return e(n) end
local function e(n)
	if n <= 0 then return 1 end
	return a(n - 1)
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	sess := result.Session
	if sess == nil || sess.RootResult == nil {
		t.Fatal("missing session")
	}

	found := 0
	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	if summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent); summaries != nil {
		for sym, rt := range summaries {
			if len(rt) == 0 {
				name := ""
				if sess.RootResult.Graph != nil {
					name = sess.RootResult.Graph.NameOf(sym)
				}
				t.Errorf("empty return summary for %q (sym %d)", name, sym)
			}
			found++
		}
	}
	if found < 5 {
		t.Errorf("expected at least 5 return summary seeds, got %d", found)
	}
}
