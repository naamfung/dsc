package decl

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/tblutil"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractTypeKeys collects all type keys from scope into inputs.TypeKeys.
func ExtractTypeKeys(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || inputs == nil {
		return
	}

	seen := make(map[*scope.State]bool)
	visitScopeChain := func(root *scope.State) {
		for s := root; s != nil; s = s.Parent() {
			if seen[s] {
				continue
			}
			seen[s] = true
			s.RangeTypes(func(_ string, t typ.Type) bool {
				AddTypeKey(inputs, t)
				return true
			})
		}
	}

	visitScopeChain(fc.Base)
	for _, sc := range fc.Scopes {
		visitScopeChain(sc)
	}
}

// AddTypeKey registers a type in the TypeKeys map.
func AddTypeKey(inputs *flow.Inputs, t typ.Type) {
	if inputs == nil || t == nil {
		return
	}
	switch v := t.(type) {
	case *typ.Alias:
		if v.Target == nil {
			return
		}
		h := v.Hash()
		if h != 0 {
			inputs.TypeKeys[h] = v.Target
		}
		AddTypeKey(inputs, v.Target)
		return
	case *typ.Meta:
		if v.Of != nil {
			h := v.Hash()
			if h != 0 {
				inputs.TypeKeys[h] = v.Of
			}
		}
		AddTypeKey(inputs, v.Of)
		return
	}
	h := t.Hash()
	if h != 0 {
		if _, ok := inputs.TypeKeys[h]; !ok {
			inputs.TypeKeys[h] = t
		}
	}
}

// ExtractDeclaredTypes collects declared types from CFG annotations.
// Collects:
// 1. Inherited symbols from globals map (globals, stdlib)
// 2. Explicitly annotated local variables (tracked in AnnotatedVars)
// 3. Local function definitions with explicit return type annotations only
// Non-annotated variables are NOT stored here; their types come from Assignments + flow solver.
func ExtractDeclaredTypes(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil {
		return
	}

	entry := fc.Graph.Entry()

	bindings := fc.Graph.Bindings()
	if fc.CheckCtx != nil && fc.CheckCtx.Bindings() != nil {
		bindings = fc.CheckCtx.Bindings()
	}
	for _, name := range cfg.SortedFieldNames(fc.Globals) {
		t := fc.Globals[name]
		if t == nil {
			continue
		}
		sym, ok := fc.Graph.SymbolAt(entry, name)
		if !ok {
			continue
		}
		if bindings != nil {
			kind, ok := bindings.Kind(sym)
			if !ok || kind != basecfg.SymbolGlobal {
				continue
			}
		}
		inputs.DeclaredTypes[sym] = resolve.Ref(t, fc.Base)
	}

	if fc.Services != nil {
		fc.Graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Name == "" || info.FuncExpr == nil {
				return
			}
			if info.TargetKind != cfg.FuncDefGlobal {
				return
			}
			sym := info.Symbol
			if sym == 0 {
				return
			}
			sc := fc.Scopes[p]
			fnType := fc.Services.ResolveFunctionSignature(info.FuncExpr, sc)
			if fnType != nil && len(fnType.Returns) > 0 {
				inputs.DeclaredTypes[sym] = fnType
			}
		})
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		sc := fc.Scopes[p]

		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}

			sym := target.Symbol
			annExpr := info.TypeAnnotationAt(i)
			hasAnnotation := annExpr != nil

			if hasAnnotation {
				if inputs.AnnotatedVars == nil {
					inputs.AnnotatedVars = make(map[cfg.SymbolID]bool)
				}
				annotate := false
				var annType typ.Type
				if fc.Services != nil {
					annType = fc.Services.ResolveTypeExpr(annExpr, sc)
					if annType != nil {
						resolved := resolve.Ref(annType, sc)
						if typ.IsRefinableAnnotation(annType) {
							if existing := inputs.DeclaredTypes[sym]; existing == nil || typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
								inputs.DeclaredTypes[sym] = resolved
							}
						} else {
							inputs.DeclaredTypes[sym] = resolved
							annotate = true
						}
					}
				} else if fc.CheckCtx != nil && fc.CheckCtx.Types() != nil {
					tv := fc.CheckCtx.Types().DeclaredAt(p, sym)
					if tv.State == flow.StateResolved && tv.Type != nil {
						resolved := resolve.Ref(tv.Type, sc)
						if typ.IsRefinableAnnotation(tv.Type) {
							if existing := inputs.DeclaredTypes[sym]; existing == nil || typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
								inputs.DeclaredTypes[sym] = resolved
							}
						} else {
							inputs.DeclaredTypes[sym] = resolved
							annotate = true
						}
					}
				}
				if annotate {
					inputs.AnnotatedVars[sym] = true
				}
			} else if fc.Services != nil {
				if fnExpr, ok := info.SourceAt(i).(*ast.FunctionExpr); ok && tblutil.FunctionHasAnnotations(fnExpr) {
					fnType := fc.Services.ResolveFunctionSignature(fnExpr, sc)
					if fnType != nil && len(fnType.Returns) > 0 {
						if inputs.AnnotatedVars == nil {
							inputs.AnnotatedVars = make(map[cfg.SymbolID]bool)
						}
						inputs.AnnotatedVars[sym] = true
						inputs.DeclaredTypes[sym] = fnType
					}
				}
			}
		})
	})
}

// ExtractModuleAliases collects symbol -> module path mappings from require() assignments.
// Detects patterns like: local x = require("mod") or x = require("mod")
func ExtractModuleAliases(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}
	aliases := modules.CollectAliases(fc.Graph)
	if len(aliases) == 0 {
		return
	}
	if inputs.ModuleAliases == nil {
		inputs.ModuleAliases = make(map[cfg.SymbolID]string, len(aliases))
	}
	for sym, path := range aliases {
		inputs.ModuleAliases[sym] = path
	}
}
