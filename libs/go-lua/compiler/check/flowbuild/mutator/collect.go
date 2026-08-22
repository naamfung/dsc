package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexerInfo holds key and value types for dynamic index assignments.
type IndexerInfo struct {
	KeyType typ.Type
	ValType typ.Type
}

// CollectTableInsertMutations scans the graph for table mutator calls on indexed expressions.
// For table.insert(t[k], v), returns mutations grouped by the base symbol of t.
// Uses the effect-based detection via TableMutatorFromCall.
func CollectTableInsertMutations(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
) map[cfg.SymbolID][]IndexerInfo {
	result := make(map[cfg.SymbolID][]IndexerInfo)
	if graph == nil {
		return result
	}

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		tm := TableMutatorFromCall(info, p, synth, nil, graph, bindings, nil)
		if tm == nil {
			return
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			return
		}

		// Check if target is an indexed expression: t[k]
		targetAttr, ok := targetExpr.(*ast.AttrGetExpr)
		if !ok {
			return
		}

		baseSym := callsite.SymbolOrCreateFieldFromExpr(targetAttr.Object, bindings)
		if baseSym == 0 {
			return
		}

		// Get key type from the index key
		var keyType typ.Type
		switch k := targetAttr.Key.(type) {
		case *ast.IdentExpr:
			if synth != nil {
				keyType = synth(k, p)
			}
		case *ast.StringExpr:
			keyType = typ.LiteralString(k.Value)
		case *ast.NumberExpr:
			keyType = typ.Integer
		default:
			if synth != nil && targetAttr.Key != nil {
				keyType = synth(targetAttr.Key, p)
			}
		}
		if keyType == nil {
			keyType = typ.String
		}

		// Strip falsy types from key types
		keyType = narrow.ToTruthy(keyType)
		if keyType == nil {
			keyType = typ.String
		}

		// Get value type from the inserted element
		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		// The value type is an array of the element type
		valType := typ.NewArray(elemType)

		result[baseSym] = append(result[baseSym], IndexerInfo{
			KeyType: keyType,
			ValType: valType,
		})
	})

	return result
}

// CollectTableInsertOnDirect scans for table mutator calls on direct variables.
// For table.insert(t, v), returns mutations grouped by the symbol of t.
// Uses the effect-based detection via TableMutatorFromCall.
func CollectTableInsertOnDirect(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
) map[cfg.SymbolID]typ.Type {
	result := make(map[cfg.SymbolID]typ.Type)
	if graph == nil {
		return result
	}

	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		tm := TableMutatorFromCall(info, p, synth, nil, graph, bindings, nil)
		if tm == nil {
			return
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			return
		}

		// Check if target resolves to a direct symbol (identifier or static field path).
		sym := callsite.SymbolOrCreateFieldFromExpr(targetExpr, bindings)
		if sym == 0 {
			return
		}

		// Get value type from the inserted element
		var elemType typ.Type
		if synth != nil && valueExpr != nil {
			elemType = synth(valueExpr, p)
		}
		if elemType == nil {
			elemType = typ.Unknown
		}

		// Join with existing element type
		if existing := result[sym]; existing != nil {
			result[sym] = typ.JoinPreferNonSoft(existing, elemType)
		} else {
			result[sym] = elemType
		}
	})

	return result
}

// MergeIndexerMutations merges table mutator mutations into indexer assignments.
func MergeIndexerMutations(
	indexers map[cfg.SymbolID][]IndexerInfo,
	mutations map[cfg.SymbolID][]IndexerInfo,
) {
	for sym, infos := range mutations {
		indexers[sym] = append(indexers[sym], infos...)
	}
}
