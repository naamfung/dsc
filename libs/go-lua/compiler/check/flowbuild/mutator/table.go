package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/literal"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractTableMutatorAssignments extracts table mutator assignments (table.insert-like)
// from call sites in the graph and appends them to inputs.TableMutatorAssignments.
func ExtractTableMutatorAssignments(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}

	bindings := fc.Graph.Bindings()

	fc.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		tm := TableMutatorFromCall(info, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Graph, bindings, fc.ModuleBindings)
		if tm == nil {
			return
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			return
		}

		sc := fc.Scopes[p]
		valueType := typ.Unknown
		if fc.Derived != nil && fc.Derived.Synth != nil {
			if t := fc.Derived.Synth(valueExpr, p); t != nil {
				valueType = t
			}
		}
		valueType = resolve.Ref(valueType, sc)

		// Build value path for flow-resolved lookup at solve time
		var valuePath constraint.Path
		if ident, ok := valueExpr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
				valuePath = constraint.Path{
					Root:   resolve.RootNameFromBindings(bindings, sym, ident.Value),
					Symbol: sym,
				}
			}
		}

		constResolver := predicate.BuildConstResolver(inputs, p)

		// Handle direct paths using bindings
		if path := flowpath.FromExprWithBindingsAt(targetExpr, constResolver, bindings, fc.Graph, p); !path.IsEmpty() && path.Symbol != 0 {
			inputs.TableMutatorAssignments = append(inputs.TableMutatorAssignments, flow.TableMutatorAssignment{
				Point: p,
				Target: constraint.Path{
					Root:     resolve.RootNameFromBindings(bindings, path.Symbol, path.Root),
					Symbol:   path.Symbol,
					Segments: path.Segments,
				},
				ValuePath: valuePath,
				ValueType: valueType,
			})
			return
		}

		// Handle dynamic index targets like suites[suite]
		attr, ok := targetExpr.(*ast.AttrGetExpr)
		if !ok {
			return
		}
		basePath := flowpath.FromExprWithBindingsAt(attr.Object, constResolver, bindings, fc.Graph, p)
		if basePath.IsEmpty() || basePath.Symbol == 0 {
			return
		}
		assign := flow.TableMutatorAssignment{
			Point: p,
			Target: constraint.Path{
				Root:     resolve.RootNameFromBindings(bindings, basePath.Symbol, basePath.Root),
				Symbol:   basePath.Symbol,
				Segments: basePath.Segments,
			},
			ValuePath: valuePath,
			ValueType: valueType,
		}

		if ident, ok := attr.Key.(*ast.IdentExpr); ok && ident.Value != "" {
			if bindings != nil {
				if keySym, found := bindings.SymbolOf(ident); found && keySym != 0 {
					assign.KeySymbol = keySym
					assign.KeyVar = resolve.RootNameFromBindings(bindings, keySym, ident.Value)
				}
			}
		} else if keyType := literal.KeyTypeFromExpr(attr.Key, constResolver); keyType != nil {
			assign.KeyType = keyType
		}

		inputs.TableMutatorAssignments = append(inputs.TableMutatorAssignments, assign)
	})
}

func TableMutatorFromCall(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) *effect.TableMutator {
	if info == nil {
		return nil
	}

	fnType := resolve.CalleeType(info, p, synth, symResolver, nil, graph, bindings, moduleBindings)
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}
	return spec.GetTableMutator()
}
