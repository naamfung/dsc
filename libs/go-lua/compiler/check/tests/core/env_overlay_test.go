package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// migrationManifest creates a manifest for a migration DSL module.
// The define function accepts a callback and injects "up" and "down"
// as callback-scoped globals via EnvOverlay.
func migrationManifest() *io.Manifest {
	upFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()

	downFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Build()

	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(map[string]typ.Type{
		"up":   upFn,
		"down": downFn,
	})

	defineSpec := contract.NewSpec().
		WithCallback(1, callbackSpec)

	defineFn := typ.Func().
		Param("name", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(defineSpec).
		Build()

	moduleType := typ.NewRecord().
		Field("define", defineFn).
		Build()

	m := io.NewManifest("migration")
	m.SetExport(moduleType)
	return m
}

// testFrameworkManifest creates a manifest for a test DSL module.
// The "it" function accepts a callback with "expect" injected via EnvOverlay.
func testFrameworkManifest() *io.Manifest {
	expectFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.Boolean).
		Build()

	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(map[string]typ.Type{
		"expect": expectFn,
	})

	itSpec := contract.NewSpec().
		WithCallback(1, callbackSpec)

	itFn := typ.Func().
		Param("name", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(itSpec).
		Build()

	m := io.NewManifest("testing")
	m.AddGlobal("it", itFn)
	return m
}

func TestEnvOverlay_MigrationDSL(t *testing.T) {
	source := `
		migration.define("Create users table", function()
			up(function()
				local x = 1
			end)
			down(function()
				local y = 2
			end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
	if result.HasError() {
		t.Errorf("expected no errors inside migration callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_TestDSL(t *testing.T) {
	manifest := testFrameworkManifest()

	source := `
		it("should work", function()
			expect(42)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("testing", manifest))
	if result.HasError() {
		t.Errorf("expected no errors inside test callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredBasic(t *testing.T) {
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
			_G.up = nil
		end

		define("test", function()
			up(function() end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with inferred overlay, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredMultipleGlobals(t *testing.T) {
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			_G.down = function(cb: fun()) cb() end
			fn()
			_G.up = nil
			_G.down = nil
		end

		define("test", function()
			up(function() end)
			down(function() end)
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with multiple inferred overlays, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_InferredMissingCleanup(t *testing.T) {
	// No _G.up = nil cleanup, so overlay should NOT be inferred.
	// Calling up() should produce an error since it is not typed.
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
		end

		define("test", function()
			local x: fun(cb: fun()) = up
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Error("expected error when cleanup is missing (no overlay inferred)")
	}
}

func TestEnvOverlay_InferredScopeIsolation(t *testing.T) {
	// Inferred globals should not be visible outside the callback.
	source := `
		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			fn()
			_G.up = nil
		end

		define("test", function()
			up(function() end)
		end)

		local x: fun(cb: fun()) = up
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Error("inferred overlay globals should not be typed outside callback")
	}
}

func TestEnvOverlay_InferredNonParamCall(t *testing.T) {
	// Call to a non-parameter function should not trigger overlay.
	source := `
		local function helper() end

		local function define(name: string, fn: fun())
			_G.up = function(cb: fun()) cb() end
			helper()
			_G.up = nil
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("non-param call should not cause errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEnvOverlay_ScopeIsolation(t *testing.T) {
	// "up" is available inside the callback via EnvOverlay
	t.Run("inside callback", func(t *testing.T) {
		source := `
			migration.define("Create table", function()
				up(function() end)
			end)
		`
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
		if result.HasError() {
			t.Errorf("up should be visible inside callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	// "up" is NOT available outside the callback, so assigning it to a
	// typed variable should produce an error (it resolves as unknown).
	t.Run("outside callback", func(t *testing.T) {
		source := `
			migration.define("Create table", function()
				up(function() end)
			end)

			local x: fun(fn: fun(): nil): nil = up
		`
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("migration", migrationManifest()))
		if !result.HasError() {
			t.Error("up should NOT be typed outside callback, expected type error")
		}
	})
}
