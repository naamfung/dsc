package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// FP-2: When a function uses `if err then return end` (not error()),
// the sibling value must narrow to non-nil after the guard.
func TestFP_ErrorGuardReturnNarrowsSibling(t *testing.T) {
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
if err then
	return
end

db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; return guard on err should narrow db to non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// FP-2 variant: A wrapper function returns (T?, error?) with a return guard,
// and the caller uses the returned value after its own error guard.
func TestFP_ErrorGuardReturnNarrowsSibling_Wrapped(t *testing.T) {
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

local function open_db()
	local db, err = sql:get("app:db")
	if err then
		return nil, err
	end
	return db
end

local db, err = open_db()
if err then
	return
end

db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; cascading return guard should narrow db, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// FP-2 variant: Multiple error-returning calls each guarded with return.
func TestFP_ErrorGuardReturnNarrowsSibling_Multiple(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Returns(typ.String, typ.NewOptional(typ.LuaError)).
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

	source := `
local sql = require("sql")

local function run()
	local db, err = sql:get("app:db")
	if err then
		return nil, err
	end

	local result, qerr = db:query("SELECT 1")
	if qerr then
		db:release()
		return nil, qerr
	end

	db:release()
	return result
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; multiple return guards should narrow siblings, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// FP-3: When a nil-check guard returns early, the checked variable
// narrows to non-nil and sibling constraints propagate correctly.
func TestFP_NilCheckReturnNarrowsToNonNil(t *testing.T) {
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
if db == nil then
	return
end

db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; nil-check return on db should narrow db to non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// FP-3 variant: Nil-check return guard in a wrapper function, caller uses result.
func TestFP_NilCheckReturnNarrowsToNonNil_Wrapped(t *testing.T) {
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

local function open_db()
	local db, err = sql:get("app:db")
	if err then
		return nil, err
	end
	return db
end

local db = open_db()
if db == nil then
	return
end

db:release()
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))

	if result.HasError() {
		t.Fatalf("expected no errors; nil-check return on wrapped result should narrow to non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
