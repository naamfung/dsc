package modules

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// ExportType computes the module's exported type from return statements.
// Pass refinementsBySym to enrich exported functions with effect summaries.
func ExportType(result *api.FuncResult, refinementsBySym map[cfg.SymbolID]*constraint.FunctionRefinement) typ.Type {
	if result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return typ.Nil
	}

	synth := result.NarrowSynth

	var export typ.Type
	var exportRootName string
	var exportRootSet bool

	result.Graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if result.FlowInputs != nil && result.FlowInputs.DeadPoints[p] {
			return
		}
		if len(info.Exprs) == 0 {
			if export == nil {
				export = typ.Nil
			} else {
				export = typ.NewUnion(export, typ.Nil)
			}
			return
		}

		valueType := synth.TypeOf(info.Exprs[0], p)
		if valueType == nil {
			valueType = typ.Nil
		}

		// Track the root name for export path-based summaries (e.g., return M).
		if ident, ok := info.Exprs[0].(*ast.IdentExpr); ok && ident != nil {
			if !exportRootSet {
				exportRootName = ident.Value
				exportRootSet = true
			} else if exportRootName != ident.Value {
				exportRootName = ""
			}
		} else if exportRootSet {
			exportRootName = ""
		}

		if export == nil {
			export = valueType
		} else if !typ.TypeEquals(export, valueType) {
			export = typ.NewUnion(export, valueType)
		}
	})

	if export != nil && len(refinementsBySym) > 0 && result.Graph != nil {
		export = effects.EnrichExportWithEffects(export, exportRootName, refinementsBySym, result.Graph)
	}

	if export == nil {
		return typ.Nil
	}
	return export
}

// ExportTypes extracts module-local type definitions for manifest generation.
func ExportTypes(result *api.FuncResult) map[string]typ.Type {
	if result == nil || result.Graph == nil {
		return nil
	}

	types := make(map[string]typ.Type)
	result.Graph.EachTypeDef(func(p cfg.Point, info *cfg.TypeDefInfo) {
		if info.Name == "" {
			return
		}

		sc := result.Scopes[p]
		if sc == nil {
			return
		}

		if resolved, ok := sc.LookupType(info.Name); ok {
			types[info.Name] = resolved
		}
	})
	return types
}

// CopyRefinementsForExport returns a defensive copy of refinements for manifest export.
func CopyRefinementsForExport(refinementsBySym map[cfg.SymbolID]*constraint.FunctionRefinement) map[cfg.SymbolID]*constraint.FunctionRefinement {
	if len(refinementsBySym) == 0 {
		return nil
	}
	refinements := make(map[cfg.SymbolID]*constraint.FunctionRefinement, len(refinementsBySym))
	for sym, refinement := range refinementsBySym {
		if refinement != nil {
			refinements[sym] = refinement
		}
	}
	return refinements
}

// ResolveExportTypeNames resolves type names to concrete types for a given scope.
// This is exposed for tests that validate exported type definitions.
