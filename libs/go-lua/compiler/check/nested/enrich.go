package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// This file provides type enrichment utilities for self-type resolution.
//
// When a method is defined in a table literal, the function's literal signature
// (with inferred refinements and return types) may be more precise than the initially
// synthesized type. These utilities replace placeholder types with literal sigs.

// EnrichTableTypeWithFuncTypes replaces method function types in a record
// with canonical function types derived from the interproc queries.
//
// For table literals with method fields, the initially synthesized record may
// have function types without inferred return types. After analyzing the methods,
// canonical function types are available per symbol. This function updates the
// record with those more precise signatures.
func EnrichTableTypeWithFuncTypes(
	rec *typ.Record,
	tableExpr *ast.TableExpr,
	graph *cfg.Graph,
	funcTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if rec == nil || tableExpr == nil || graph == nil || len(funcTypes) == 0 {
		return rec
	}

	modified := false
	builder := typ.NewRecord()
	bindings := graph.Bindings()

	for _, f := range rec.Fields {
		fieldType := f.Type
		for _, tf := range tableExpr.Fields {
			if tf.Key == nil {
				continue
			}
			var keyName string
			switch k := tf.Key.(type) {
			case *ast.StringExpr:
				keyName = k.Value
			}
			if keyName != f.Name {
				continue
			}
			fnExpr, ok := tf.Value.(*ast.FunctionExpr)
			if !ok {
				continue
			}
			if bindings != nil {
				if sym, ok := bindings.FuncLitSymbol(fnExpr); ok {
					if t := funcTypes[sym]; t != nil {
						fieldType = t
						modified = true
					}
				}
			}
		}
		if f.Optional {
			builder = builder.OptField(f.Name, fieldType)
		} else {
			builder = builder.Field(f.Name, fieldType)
		}
	}

	if !modified {
		return rec
	}

	if rec.Metatable != nil {
		builder = builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder = builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.SetOpen(rec.Open).Build()
}

// CollectCapturedFieldAssignments scans a nested function's graph for field assignments
// to captured variables.
//
// When a nested function assigns fields to a captured variable (e.g., `parent.field = v`),
// those assignments affect the type visible in the parent scope. This function collects
// such assignments for propagation back to the parent.
func CollectCapturedFieldAssignments(
	graph *cfg.Graph,
	capturedSyms map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	if graph == nil || len(capturedSyms) == 0 {
		return make(map[cfg.SymbolID]map[string]typ.Type)
	}
	return assign.CollectFieldAssignments(graph, synth, capturedSyms)
}

// CollectCapturedContainerMutations scans a nested function's graph for container mutations
// (e.g., channel.send) that target captured variables.
func CollectCapturedContainerMutations(
	graph *cfg.Graph,
	capturedSyms map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID][]api.ContainerMutation {
	result := make(map[cfg.SymbolID][]api.ContainerMutation)
	if graph == nil || len(capturedSyms) == 0 {
		return result
	}

	bindings := graph.Bindings()
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		ceu := mutator.ContainerMutatorFromCall(info, p, synth, nil, nil, graph, bindings, nil)
		if ceu == nil {
			return
		}

		targetExpr := callsite.RuntimeArgAt(info, ceu.Container.Index)
		valueExpr := callsite.RuntimeArgAt(info, ceu.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			return
		}

		targetPath := flowpath.FromExprWithBindings(targetExpr, nil, bindings)
		if targetPath.IsEmpty() || targetPath.Symbol == 0 {
			return
		}
		if !capturedSyms[targetPath.Symbol] {
			return
		}

		var valueType typ.Type
		if synth != nil {
			valueType = synth(valueExpr, p)
		}
		if valueType == nil {
			valueType = typ.Unknown
		}
		valueType = subtype.WidenForInference(valueType)

		segs := make([]constraint.Segment, len(targetPath.Segments))
		copy(segs, targetPath.Segments)
		result[targetPath.Symbol] = append(result[targetPath.Symbol], api.ContainerMutation{
			Segments:  segs,
			ValueType: valueType,
		})
	})

	return result
}

// EnrichSelfTypeWithConstructorFields merges constructor instance fields into a self-type.
//
// When a method is defined on a class that has a constructor, the self-type should
// include fields assigned in the constructor. This function looks up constructor
// fields for the class and merges them into the self-type.
//
// This enables the type checker to recognize instance fields in methods:
//
//	function T.new()
//	    local self = setmetatable({}, T)
//	    self.name = ""  -- Collected as constructor field
//	    return self
//	end
//	function T:greet()
//	    print(self.name)  -- self.name is recognized because of constructor fields
//	end
func EnrichSelfTypeWithConstructorFields(selfType typ.Type, classSymbol cfg.SymbolID, store Store) typ.Type {
	if selfType == nil || store == nil || classSymbol == 0 {
		return selfType
	}

	fields := store.LookupConstructorFields(classSymbol)
	if len(fields) == 0 {
		return selfType
	}

	return mergeFieldsIntoSelfType(selfType, fields)
}

// NormalizeMethodSelfType widens literal-heavy self shapes so method-local
// flow constraints do not treat mutable receiver state as compile-time constants.
func NormalizeMethodSelfType(selfType typ.Type) typ.Type {
	if selfType == nil {
		return nil
	}
	return subtype.WidenForInference(selfType)
}

func mergeFieldsIntoSelfType(selfType typ.Type, fields map[string]typ.Type) typ.Type {
	if len(fields) == 0 {
		return selfType
	}

	switch v := selfType.(type) {
	case *typ.Record:
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}

		existingFields := make(map[string]bool)
		for _, f := range v.Fields {
			if f.Optional {
				builder.OptField(f.Name, f.Type)
			} else {
				builder.Field(f.Name, f.Type)
			}
			existingFields[f.Name] = true
		}

		for name, t := range fields {
			if !existingFields[name] {
				builder.Field(name, t)
			}
		}

		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, v.MapValue)
		}
		return builder.Build()

	case *typ.Interface:
		builder := typ.NewRecord().SetOpen(true)
		existingFields := make(map[string]bool)
		for _, m := range v.Methods {
			builder.Field(m.Name, m.Type)
			existingFields[m.Name] = true
		}
		for name, t := range fields {
			if !existingFields[name] {
				builder.Field(name, t)
			}
		}
		return builder.Build()

	default:
		return selfType
	}
}
