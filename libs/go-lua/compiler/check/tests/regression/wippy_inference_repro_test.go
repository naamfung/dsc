package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Repro for wippy consts.get_db_resource inference via env.get
func TestWippy_ConstGetDbResource_ReturnType(t *testing.T) {
	envManifest := io.NewManifest("env")
	envType := typ.NewInterface("env", []typ.Method{
		{Name: "get", Type: typ.Func().Param("key", typ.String).Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
	})
	envManifest.SetExport(envType)
	envMod := &testutil.ModuleResult{Manifest: envManifest}

	constsMod := testutil.CheckAndExport(`
        local env = require("env")
        local M = {}
        function M.get_db_resource()
            local v, _ = env.get("DB_RESOURCE")
            return v
        end
        return M
    `, "consts", testutil.WithStdlib(), testutil.WithModule("env", envMod))

	if constsMod.HasError() {
		for _, e := range constsMod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("consts module has errors")
	}

	result := testutil.Check(`
        local consts = require("consts")
        local s: string = consts.get_db_resource()
    `, testutil.WithStdlib(), testutil.WithModule("consts", constsMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("consumer error: %s", e.Message)
		}
		t.Fatal("expected consts.get_db_resource() to be string")
	}
}
