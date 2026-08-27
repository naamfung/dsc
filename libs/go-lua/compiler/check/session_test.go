package check

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func newSessionTestChecker(imports map[string]*io.Manifest) *Checker {
	database := db.New()
	for path, manifest := range imports {
		if manifest != nil {
			database.Connect(path, manifest)
		}
	}

	globalTypes := make(map[string]typ.Type)
	for name, t := range stdlib.Library() {
		globalTypes[name] = t
	}
	for _, manifest := range imports {
		if manifest == nil {
			continue
		}
		if manifest.Export != nil {
			globalTypes[manifest.Path] = manifest.Export
		}
		for name, t := range manifest.AllGlobals() {
			globalTypes[name] = t
		}
	}

	return NewChecker(database, Deps{
		Types:       core.NewEngineWithStdlib(stdlib.EngineConfig()),
		Stdlib:      scope.NewWithBuiltins(),
		GlobalTypes: globalTypes,
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	})
}

func TestNew(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	sess := New(ctx, "test.lua")

	if sess.Ctx != ctx {
		t.Error("Ctx not set")
	}
	if sess.SourceName != "test.lua" {
		t.Errorf("SourceName = %q, want %q", sess.SourceName, "test.lua")
	}
	if sess.Imports == nil {
		t.Error("Imports not initialized")
	}
	if sess.Results == nil {
		t.Error("Results not initialized")
	}
	if sess.Terminators == nil {
		t.Error("Terminators not initialized")
	}
	if sess.ManifestVars == nil {
		t.Error("ManifestVars not initialized")
	}
}

func TestSession_DiagnosticsAccumulate(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	// Diagnostics starts as nil, append works
	sess.Diagnostics = append(sess.Diagnostics, diag.Diagnostic{})
	sess.Diagnostics = append(sess.Diagnostics, diag.Diagnostic{})

	if len(sess.Diagnostics) != 2 {
		t.Errorf("len(Diagnostics) = %d, want 2", len(sess.Diagnostics))
	}
}

func TestSession_Results(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	fn := &ast.FunctionExpr{}
	result := &api.FuncResult{
		BaseScope: nil,
	}

	sess.Results[fn] = result

	got := sess.Results[fn]
	if got != result {
		t.Error("Results[fn] not stored correctly")
	}
}

func TestSession_Imports(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	manifest := &io.Manifest{}
	sess.Imports["module"] = manifest

	got := sess.Imports["module"]
	if got != manifest {
		t.Error("Imports not stored correctly")
	}
}

func TestSession_Terminators(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	sess.Terminators["error"] = true
	sess.Terminators["assert"] = true

	if !sess.Terminators["error"] {
		t.Error("Terminators['error'] should be true")
	}
	if sess.Terminators["print"] {
		t.Error("Terminators['print'] should be false")
	}
}

func TestSession_ManifestVars(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	sess.ManifestVars["PKG"] = "/path/to/pkg"

	if sess.ManifestVars["PKG"] != "/path/to/pkg" {
		t.Error("ManifestVars not stored correctly")
	}
}

func TestSession_ExportManifest_IncludesFunctionSummaries(t *testing.T) {
	checker := newSessionTestChecker(nil)
	sess := checker.Check(`
		local M = {}

		function M.not_nil(x: any): any
			if x == nil then
				error("nil")
			end
			return x
		end

		return M
	`, "assert.lua")

	manifest := sess.ExportManifest("assert")
	if manifest == nil {
		t.Fatal("ExportManifest should return manifest")
	}
	if manifest.Path != "assert" {
		t.Fatalf("manifest.Path = %q, want %q", manifest.Path, "assert")
	}

	summary, ok := manifest.LookupSummary("not_nil")
	if !ok || summary == nil {
		t.Fatal("expected not_nil summary in exported manifest")
	}
	if !summary.Ensures.HasConstraints() {
		t.Fatal("expected not_nil summary to carry ensures constraints")
	}
}

func TestSession_ExportManifest_EnablesCrossModuleNarrowing(t *testing.T) {
	producerChecker := newSessionTestChecker(nil)
	producer := producerChecker.Check(`
		local M = {}

		function M.not_nil(x: any): any
			if x == nil then
				error("nil")
			end
			return x
		end

		return M
	`, "assert.lua")

	assertManifest := producer.ExportManifest("assert")
	consumerChecker := newSessionTestChecker(map[string]*io.Manifest{
		"assert": assertManifest,
	})
	consumer := consumerChecker.Check(`
		local assert = require("assert")

		local function maybe_name(): string?
			return "ok"
		end

		local x = maybe_name()
		assert.not_nil(x)
		local n = #x
		return n
	`, "consumer.lua")

	for _, d := range consumer.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Fatalf("unexpected error diagnostic: %s", d.Message)
		}
	}
}

func TestSession_ExportManifest_PreservesEqSummaryConstraints(t *testing.T) {
	producerChecker := newSessionTestChecker(nil)
	producer := producerChecker.Check(`
		local M = {}

		function M.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		return M
	`, "assert.lua")

	assertManifest := producer.ExportManifest("assert")
	summary, ok := assertManifest.LookupSummary("eq")
	if !ok || summary == nil {
		t.Fatal("expected eq summary in exported manifest")
	}
	if !summary.Ensures.HasConstraints() {
		t.Fatal("expected eq summary to carry ensures constraints")
	}

	foundEq := false
	param0 := constraint.ParamPath(0).Key()
	param1 := constraint.ParamPath(1).Key()
	for _, c := range summary.Ensures.AllConstraints() {
		if eq, ok := c.(constraint.EqPath); ok {
			leftKey := eq.Left.Key()
			rightKey := eq.Right.Key()
			if leftKey == param0 && rightKey == param1 {
				foundEq = true
				break
			}
			if leftKey == param1 && rightKey == param0 {
				foundEq = true
				break
			}
		}
	}
	if !foundEq {
		t.Fatalf("expected EqPath($0,$1) in ensures, got: %v", summary.Ensures)
	}
}

func TestSession_ExportManifest_EnablesCrossModuleEqLenNarrowing(t *testing.T) {
	producerChecker := newSessionTestChecker(nil)
	producer := producerChecker.Check(`
		local M = {}

		function M.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		return M
	`, "assert.lua")

	assertManifest := producer.ExportManifest("assert")
	consumerChecker := newSessionTestChecker(map[string]*io.Manifest{
		"assert": assertManifest,
	})

	consumer := consumerChecker.Check(`
		type Row = { stream: string }
		local assert = require("assert")

		local function parse_stream_lines(raw: string?): {Row}
			local lines = {}
			if raw and raw ~= "" then
				table.insert(lines, { stream = "ok" })
			end
			return lines
		end

		local maybe_raw: string? = "raw"
		local result = parse_stream_lines(maybe_raw)
		assert.eq(#result, 1, "one row")
		local line: string = result[1].stream
		return line
	`, "consumer.lua")

	for _, d := range consumer.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Fatalf("unexpected error diagnostic: %s", d.Message)
		}
	}
}

func TestFuncResult_IsData(t *testing.T) {
	// FuncResult is pure data, no methods to test
	// This test verifies the struct can be created
	result := &api.FuncResult{
		Graph:     nil,
		BaseScope: nil,
	}

	if result.Graph != nil {
		t.Error("Graph should be nil")
	}
	if result.Scopes != nil {
		t.Error("Scopes should be nil when not set")
	}
}

func TestSession_PluginStore(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	type coreKey struct{}

	sess.PluginStore(coreKey{}, "core_value")

	got := sess.PluginLoad(coreKey{})
	if got != "core_value" {
		t.Errorf("PluginLoad() = %v; want 'core_value'", got)
	}
}

func TestSession_PluginLoad_NotFound(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	type unknownKey struct{}

	got := sess.PluginLoad(unknownKey{})
	if got != nil {
		t.Errorf("PluginLoad(unknown) = %v; want nil", got)
	}
}

func TestStoreFrom(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	sess := New(ctx, "test.lua")

	store := api.StoreFrom(ctx)
	if store != sess.Store {
		t.Error("StoreFrom should return the session's store")
	}
}

func TestGraphsFrom(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	sess := New(ctx, "test.lua")

	graphs := api.GraphsFrom(ctx)
	if graphs != sess {
		t.Error("GraphsFrom should return the session graph provider")
	}
}

func TestStoreFrom_NilContext(t *testing.T) {
	store := api.StoreFrom(nil)
	if store != nil {
		t.Error("StoreFrom(nil) should return nil")
	}
}

func TestAttachStore(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	store := store.NewSessionStore()

	api.AttachStore(ctx, store)

	retrieved := api.StoreFrom(ctx)
	if retrieved != store {
		t.Error("AttachStore should attach the store for retrieval via StoreFrom")
	}
}

func TestAttachStore_NilContext(t *testing.T) {
	store := store.NewSessionStore()
	api.AttachStore(nil, store) // should not panic
}

func TestAttachStore_NilStore(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	api.AttachStore(ctx, nil) // should not panic

	retrieved := api.StoreFrom(ctx)
	if retrieved != nil {
		t.Error("StoreFrom should return nil when nil store was attached")
	}
}

func TestSessionStore_EffectMaps(t *testing.T) {
	store := store.NewSessionStore()

	if store.InterprocPrev == nil || store.InterprocPrev.Refinements == nil {
		t.Error("InterprocPrev effects not initialized")
	}
	if store.InterprocNext == nil || store.InterprocNext.Refinements == nil {
		t.Error("InterprocNext effects not initialized")
	}
}

func TestFixpointChannelDiffs_IsolatedBetweenStores(t *testing.T) {
	storeA := store.NewSessionStore()
	storeB := store.NewSessionStore()

	storeA.StoreFunctionRefinement(cfg.SymbolID(42), &constraint.FunctionRefinement{})
	storeB.StoreFunctionRefinement(cfg.SymbolID(42), &constraint.FunctionRefinement{})

	if !storeA.FixpointSwap() {
		t.Fatal("expected storeA FixpointSwap to report change")
	}
	if diffs := storeA.FixpointChannelDiffs(); len(diffs) == 0 {
		t.Fatal("expected storeA diffs to be non-empty")
	}

	if diffs := storeB.FixpointChannelDiffs(); len(diffs) != 0 {
		t.Fatalf("expected storeB diffs to be empty, got %v", diffs)
	}
}

func TestSessionStore_ClearIterationChannels(t *testing.T) {
	store := store.NewSessionStore()

	store.StoreConstructorFields(cfg.SymbolID(2), map[string]typ.Type{"name": typ.String})
	store.InterprocPrev.Refinements[cfg.SymbolID(4)] = &constraint.FunctionRefinement{}
	store.StoreFunctionRefinement(cfg.SymbolID(5), &constraint.FunctionRefinement{})

	store.ClearIterationChannels()

	if store.InterprocNext == nil || len(store.InterprocNext.ConstructorFields) != 0 {
		t.Fatal("expected constructor fields to be cleared")
	}
	if len(store.InterprocPrev.Refinements) != 0 || len(store.InterprocNext.Refinements) != 0 {
		t.Fatal("expected effects to be cleared")
	}
}

func TestSession_Release(t *testing.T) {
	sess := New(db.NewQueryContext(db.New()), "test.lua")

	// Populate session with data
	fn := &ast.FunctionExpr{}
	sess.RootFunc = fn
	sess.RootResult = &api.FuncResult{
		Scopes: make(map[cfg.Point]*scope.State),
	}
	sess.Results[fn] = sess.RootResult
	sess.Store.Graphs()[1] = nil
	sess.Store.Funcs()[1] = fn
	sess.Store.Module.ModuleAliases = map[cfg.SymbolID]string{cfg.SymbolID(7): "mod"}
	sess.PluginStore("key", "value")
	sess.scopeDepthDiagEmitted[fn] = true

	// Add diagnostics (should survive release)
	sess.Diagnostics = append(sess.Diagnostics, diag.Diagnostic{Message: "test"})

	// Release
	sess.Release()

	// Verify heavy data is cleared
	if sess.RootFunc != nil {
		t.Error("RootFunc should be nil after Release")
	}
	if sess.RootResult != nil {
		t.Error("RootResult should be nil after Release")
	}
	if len(sess.Store.Graphs()) != 0 {
		t.Error("Store.Graphs should be empty after Release")
	}
	if len(sess.Store.Funcs()) != 0 {
		t.Error("Store.Funcs should be empty after Release")
	}
	if len(sess.Store.ModuleAliases()) != 0 {
		t.Error("Store.ModuleAliases should be empty after Release")
	}
	if len(sess.Results) != 0 {
		t.Error("Results should be empty after Release")
	}
	if len(sess.scopeDepthDiagEmitted) != 0 {
		t.Error("scopeDepthDiagEmitted should be empty after Release")
	}
	if sess.coreStore != nil {
		t.Error("coreStore should be nil after Release")
	}

	// Verify diagnostics survive
	if len(sess.Diagnostics) != 1 {
		t.Error("Diagnostics should survive Release")
	}
	if sess.Diagnostics[0].Message != "test" {
		t.Error("Diagnostic content should survive Release")
	}
}

func TestSession_Release_Nil(t *testing.T) {
	var sess *Session
	sess.Release() // should not panic
}

func TestStoreConstructorFields_ZeroSymbol(t *testing.T) {
	store := store.NewSessionStore()
	store.StoreConstructorFields(0, map[string]typ.Type{"x": typ.Number})
	if store.InterprocNext == nil || len(store.InterprocNext.ConstructorFields) != 0 {
		t.Error("zero symbol should not store fields")
	}
}

func TestStoreConstructorFields_EmptyFields(t *testing.T) {
	store := store.NewSessionStore()
	store.StoreConstructorFields(1, nil)
	if store.InterprocNext == nil || len(store.InterprocNext.ConstructorFields) != 0 {
		t.Error("empty fields should not store")
	}
}

func TestStoreConstructorFields_Basic(t *testing.T) {
	store := store.NewSessionStore()
	fields := map[string]typ.Type{"x": typ.Number, "y": typ.String}
	store.StoreConstructorFields(1, fields)

	next := store.InterprocNext
	if next == nil || next.ConstructorFields == nil {
		t.Fatal("ConstructorFieldsNext should be initialized")
	}
	if len(next.ConstructorFields[1]) != 2 {
		t.Errorf("expected 2 fields, got %d", len(next.ConstructorFields[1]))
	}
}

func TestStoreConstructorFields_Join(t *testing.T) {
	store := store.NewSessionStore()
	store.StoreConstructorFields(1, map[string]typ.Type{"x": typ.Number})
	store.StoreConstructorFields(1, map[string]typ.Type{"x": typ.String})

	next := store.InterprocNext
	if next == nil || next.ConstructorFields == nil || next.ConstructorFields[1]["x"] == typ.Number {
		t.Error("field should be joined")
	}
}

func TestLookupConstructorFields_ZeroSymbol(t *testing.T) {
	store := store.NewSessionStore()
	result := store.LookupConstructorFields(0)
	if result != nil {
		t.Error("zero symbol should return nil")
	}
}

func TestLookupConstructorFields_FromNext(t *testing.T) {
	store := store.NewSessionStore()
	setConstructorFieldsNextForTest(store, map[cfg.SymbolID]map[string]typ.Type{
		1: {"x": typ.Number},
	})
	result := store.LookupConstructorFields(1)
	if result != nil {
		t.Fatal("should not read constructor fields from Next snapshot")
	}
}

func TestLookupConstructorFields_FromPrev(t *testing.T) {
	store := store.NewSessionStore()
	setConstructorFieldsPrevForTest(store, map[cfg.SymbolID]map[string]typ.Type{
		1: {"y": typ.String},
	})
	result := store.LookupConstructorFields(1)
	if result == nil {
		t.Fatal("should find fields from snapshot")
	}
	if result["y"] != typ.String {
		t.Error("wrong field type")
	}
}
