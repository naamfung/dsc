package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func contractManifestForDynamicOpen() *io.Manifest {
	m := io.NewManifest("contract")

	contractType := typ.NewInterface("contract.Contract", []typ.Method{
		{
			Name: "open",
			Type: typ.Func().
				Param("self", typ.Self).
				OptParam("name", typ.String).
				OptParam("options", typ.Any).
				Returns(typ.Any, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	moduleType := typ.NewInterface("contract", []typ.Method{
		{
			Name: "get",
			Type: typ.Func().
				Param("name", typ.String).
				Returns(contractType, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	m.SetExport(moduleType)
	return m
}

// Regression: dynamic return (`any`) from contract:open() plus explicit `return nil`
// in a helper must not collapse the helper's return type to nil.
func TestContractOpen_DynamicReturnNotCollapsedToNil(t *testing.T) {
	source := `
local contract = require("contract")

local function get_tracker()
	local c, err = contract.get("wippy.llm:usage_tracker")
	if not err and c then
		local inst, open_err = c:open()
		if not open_err then
			return inst
		end
	end
	return nil
end

local function use_tracker()
	local tracker = get_tracker()
	if not tracker then
		return nil
	end
	local usage_id, err = tracker:track_usage("model", 1, 1, 0, 0, 0, {})
	return usage_id, err
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("contract", contractManifestForDynamicOpen()))
	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Fatalf("expected no errors for dynamic contract helper return")
	}

	if result.Session == nil || result.Session.Store == nil || result.Session.RootResult == nil || result.Session.RootResult.Graph == nil {
		t.Fatal("missing session data")
	}
	root := result.Session.RootResult.Graph
	parentHash := result.Session.Store.GraphParentHashOf(root.ID())
	parent := result.Session.Store.Parents()[parentHash]
	funcTypes := result.Session.Store.GetLocalFuncTypesSnapshot(root, parent)

	sym, ok := root.SymbolAt(root.Exit(), "get_tracker")
	if !ok || sym == 0 {
		t.Fatal("missing symbol get_tracker")
	}
	fn := unwrap.Function(funcTypes[sym])
	if fn == nil || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		t.Fatalf("expected get_tracker function return type, got %v", funcTypes[sym])
	}
	if fn.Returns[0].Kind() == kind.Nil {
		t.Fatalf("expected get_tracker return not to collapse to nil, got %v", fn.Returns[0])
	}
}
