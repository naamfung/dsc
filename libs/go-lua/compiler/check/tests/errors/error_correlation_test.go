package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestAssertIsNilNarrowsSiblingRequire(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	assertManifest := io.NewManifest("assert2")
	assertManifest.SetExport(typ.NewRecord().
		Field("is_nil", typ.Func().
			Param("val", typ.Any).
			OptParam("msg", typ.String).
			WithRefinement(constraint.NewRefinement(
				[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
				nil, nil,
			)).
			Build()).
		Build())

	source := `
local assert = require("assert2")
local sql = require("sql")

local db, err = sql.get("app:db")
assert.is_nil(err, "db error")
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("sql", sqlManifest),
		testutil.WithManifest("assert2", assertManifest),
	)

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; assert.is_nil(err) should narrow db, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestErrorTerminatesEliminatesNilReturn(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	source := `
local sql = require("sql")

local function get_db()
	local db, err = sql:get("app:db")
	if err then
		error(err:message())
	end
	return db
end

local db = get_db()
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.Session != nil && result.Session.RootResult != nil && result.Session.RootResult.Graph != nil {
		root := result.Session.RootResult
		var getDbSym cfg.SymbolID
		root.Graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil || !info.IsLocal {
				return
			}
			for i, target := range info.Targets {
				if target.Kind != cfg.TargetIdent || target.Name != "get_db" || i >= len(info.Sources) {
					continue
				}
				if _, ok := info.Sources[i].(*ast.FunctionExpr); ok {
					getDbSym = target.Symbol
					return
				}
			}
		})
		if getDbSym == 0 {
			t.Logf("get_db symbol not found in root Assign info")
		}
		if getDbSym != 0 && result.Session.Store != nil {
			parentHash := result.Session.Store.GraphParentHashOf(root.Graph.ID())
			parent := result.Session.Store.Parents()[parentHash]
			if summaries := result.Session.Store.GetReturnSummariesSnapshot(root.Graph, parent); summaries != nil {
				if returns, ok := summaries[getDbSym]; ok {
					t.Logf("ReturnSummaries[%d][get_db]=%v", parentHash, returns)
				}
			}
		}
	}
	if result.Session != nil {
		for fn, res := range result.Session.Results {
			if fn == nil || res == nil || res.Graph == nil || res.FlowSolution == nil {
				continue
			}
			if fn.Line() != 4 {
				continue
			}
			if res.NarrowSynth != nil {
				if fnType := res.NarrowSynth.FunctionType(fn, res.BaseScope); fnType != nil {
					t.Logf("NarrowSynth.FunctionType(get_db) returns %v", fnType.Returns)
				}
			}
			res.Graph.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
				if sym, ok := res.Graph.SymbolAt(p, "db"); ok && sym != 0 {
					path := constraint.Path{Root: "db", Symbol: sym}
					if narrowed := res.FlowSolution.NarrowedTypeAt(p, path); narrowed != nil {
						t.Logf("NarrowedTypeAt(db) at return point %d = %v", p, narrowed)
					}
				}
			})
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; error() should terminate and remove nil path, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_NestedWrapperChain(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	source := `
local sql = require("sql")

local function connect()
	local db, err = sql:get("app:db")
	if err then
		error(err:message())
	end
	return db
end

local function get_connection()
	return connect()
end

local db = get_connection()
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; nested wrapper should preserve non-nil after error() guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_NestedWrapperChain_IntersectionModuleExport(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Returns(typ.NewArray(typ.Any), typ.NewOptional(typ.LuaError)).
				Build(),
		},
		{
			Name: "release",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	moduleMethods := typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	})
	moduleFields := typ.NewRecord().
		Field("NULL", typ.Any).
		Build()
	sqlManifest.SetExport(typ.NewIntersection(moduleMethods, moduleFields))

	source := `
local sql = require("sql")

local function connect()
	local db, err = sql:get("postgres://localhost/test")
	if err then
		error(err:message())
	end
	return db
end

local function get_connection()
	return connect()
end

local db = get_connection()
local rows, err = db:query("SELECT 1")
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))
	if result.HasError() {
		t.Fatalf("expected no errors for intersection-export wrapper chain, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	if result.Session == nil || result.Session.Store == nil || result.Session.RootResult == nil || result.Session.RootResult.Graph == nil {
		t.Fatal("missing session data")
	}
	root := result.Session.RootResult.Graph
	parentHash := result.Session.Store.GraphParentHashOf(root.ID())
	parent := result.Session.Store.Parents()[parentHash]
	summaries := result.Session.Store.GetReturnSummariesSnapshot(root, parent)

	for _, name := range []string{"connect", "get_connection"} {
		sym, ok := root.SymbolAt(root.Exit(), name)
		if !ok || sym == 0 {
			t.Fatalf("missing symbol for %s", name)
		}
		rets := returns.NormalizeReturnVector(summaries[sym])
		if len(rets) == 0 {
			t.Fatalf("missing return summary for %s", name)
		}
		if unwrap.IsOptionalLike(rets[0]) {
			t.Fatalf("expected non-optional summary for %s, got %v", name, rets[0])
		}
	}
}

func TestErrorTerminates_TableFieldsDoNotCollapseToNever(t *testing.T) {
	txType := typ.NewInterface("sql.Tx", []typ.Method{
		{
			Name: "rollback",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "begin",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(txType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))
	sqlManifest.DefineType("DB", dbType)
	sqlManifest.DefineType("Tx", txType)

	source := `
local sql = require("sql")

local function create_test_ctx()
	local db, err = sql:get("sqlite::memory:")
	if err then
		error(err:message())
	end

	local tx, terr = db:begin()
	if terr then
		error(terr:message())
	end

	return { db = db, tx = tx }
end

local test_ctx = create_test_ctx()
test_ctx.tx:rollback()
test_ctx.db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; error() guards should prevent never/optional propagation into table fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestArrayIndexAfterErrorGuard_ReturnsElement(t *testing.T) {
	rowType := typ.NewRecord().
		Field("commit_id", typ.String).
		Field("metadata", typ.Any).
		Build()

	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Returns(typ.NewArray(rowType), typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))
	sqlManifest.DefineType("DB", dbType)

	source := `
local sql = require("sql")

local db, err = sql:get("postgres://localhost/test")
if err then error(err:message()) end

local rows, qerr = db:query("SELECT commit_id, metadata FROM commits")
if qerr then error(qerr:message()) end

local first = rows[1]
local id = first.commit_id
local meta = first.metadata
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; array index after error guard should yield element type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestErrorReturnBranchPruned(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	source := `
local sql = require("sql")

local function get_db()
	local db, err = sql:get("app:db")
	if err then
		error(err:message())
		return nil
	end
	return db
end

local db = get_db()
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; dead return after error() should not contribute to summary, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestErrorReturnWithReturnGuard(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	source := `
local sql = require("sql")

local function try_get()
	local db, err = sql:get("app:db")
	if err then return nil, err end
	return db
end

local db, err = try_get()
if err then error(err:message()) end
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; error guard on err should narrow db, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestValueCheckNarrowsError(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().Param("self", typ.Self).Build(),
		},
	})

	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewInterface("sql", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("dsn", typ.String).
				Returns(dbType, typ.NewOptional(typ.LuaError)).
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	}))

	source := `
local sql = require("sql")

local db, err = sql:get("app:db")
if not db then
	error(err:message())
end
db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors; value falsy guard should narrow err to non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
