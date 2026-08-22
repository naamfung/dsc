package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func TestLocalFunctionNarrowReturnRepairsPreflowNeverSummary(t *testing.T) {
	source := `
local function f(blocks)
    local tool_use_block = nil
    for _, block in ipairs(blocks) do
        if block.type == "tool_use" and block.name == "structured_output" then
            tool_use_block = block
            break
        end
    end
    if not tool_use_block then
        return { success = false, error = "missing" }
    end
    return { success = true, result = { data = tool_use_block.input } }
end

return { f = f }
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected clean check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.RootResult == nil || sess.RootResult.Graph == nil {
		t.Fatal("missing root result")
	}

	var sym cfg.SymbolID
	sess.RootResult.Graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind == cfg.TargetIdent && target.Name == "f" {
				if _, ok := source.(*ast.FunctionExpr); ok {
					sym = target.Symbol
				}
			}
		})
	})
	if sym == 0 {
		t.Fatal("missing local function symbol for f")
	}

	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	snap := sess.Store.GetInterprocFactsSnapshot(sess.RootResult.Graph, parent)

	if got := snap.ReturnSummaries[sym]; len(got) != 1 || containsNever(got[0]) {
		t.Fatalf("summary contains never artifact: %v", got)
	}
	if got := snap.NarrowReturns[sym]; len(got) != 1 || containsNever(got[0]) {
		t.Fatalf("narrow contains never artifact: %v", got)
	}
	if got := snap.FuncTypes[sym]; got == nil || containsNever(got) {
		t.Fatalf("function fact contains never artifact: %v", got)
	}

	mod := testutil.CheckAndExport(source, "mod", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("expected clean export check, got: %v", mod.Errors)
	}
	wantExport := typ.NewRecord().
		Field("f", typ.Func().
			OptParam("blocks", typ.Any).
			Returns(
				typ.NewUnion(
					typ.NewRecord().
						Field("success", typ.True).
						Field("result", typ.NewRecord().Field("data", typ.Unknown).Build()).
						Build(),
					typ.NewRecord().
						Field("success", typ.False).
						Field("error", typ.LiteralString("missing")).
						Build(),
				),
			).
			Build()).
		Build()
	if mod.Manifest == nil || !typ.TypeEquals(mod.Manifest.Export, wantExport) {
		t.Fatalf("export = %v, want %v", mod.Manifest.Export, wantExport)
	}
}

func containsNever(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsNever(t) {
		return true
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return containsNever(o.Inner)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if containsNever(m) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if containsNever(m) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return containsNever(a.Element)
		},
		Map: func(m *typ.Map) bool {
			return containsNever(m.Key) || containsNever(m.Value)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, e := range tup.Elements {
				if containsNever(e) {
					return true
				}
			}
			return false
		},
		Record: func(r *typ.Record) bool {
			for _, f := range r.Fields {
				if containsNever(f.Type) {
					return true
				}
			}
			if r.HasMapComponent() {
				return containsNever(r.MapKey) || containsNever(r.MapValue)
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, p := range fn.Params {
				if containsNever(p.Type) {
					return true
				}
			}
			if fn.Variadic != nil && containsNever(fn.Variadic) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsNever(ret) {
					return true
				}
			}
			return false
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}
