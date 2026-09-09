package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// ForceMethodReceiver reports whether method-call receiver injection must be
// enforced for this callsite.
//
// Canonical policy:
//   - only method syntax calls (`obj:method(...)`) are considered
//   - calls to statically known function-literal-backed symbols force receiver
//   - function definition symbols with field/method targets force receiver
func ForceMethodReceiver(bindings *bind.BindingTable, graph *compcfg.Graph, info *compcfg.CallInfo) bool {
	if info == nil || info.Method == "" {
		return false
	}

	sym := SelectPreferredSymbol(
		CallableCalleeSymbolCandidates(info, graph, bindings, nil),
		func(candidate typecfg.SymbolID) bool {
			return symbolForcesMethodReceiver(bindings, graph, candidate)
		},
	)

	if sym != 0 && symbolForcesMethodReceiver(bindings, graph, sym) {
		return true
	}

	if bindings != nil {
		if methodSym, ok := methodCalleeSymbolFromCall(bindings, graph, info); ok {
			sym = methodSym
		}
	}
	if sym == 0 {
		return false
	}

	return symbolForcesMethodReceiver(bindings, graph, sym)
}

// ForceMethodReceiverAtPoint resolves callsite info at a CFG point and applies
// the canonical receiver-forcing policy.
func ForceMethodReceiverAtPoint(bindings *bind.BindingTable, graph *compcfg.Graph, p typecfg.Point, ex *ast.FuncCallExpr) bool {
	if ex == nil || ex.Method == "" {
		return false
	}

	if graph != nil {
		if info := graph.CallSiteAt(p, ex); info != nil {
			return ForceMethodReceiver(bindings, graph, info)
		}
	}

	if bindings == nil {
		return false
	}
	sym, ok := methodCalleeSymbolFromExpr(bindings, graph, ex)
	if !ok || sym == 0 {
		return false
	}
	return symbolForcesMethodReceiver(bindings, graph, sym)
}

func symbolForcesMethodReceiver(bindings *bind.BindingTable, graph *compcfg.Graph, sym typecfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	if bindings != nil {
		if _, ok := bindings.FuncLitBySymbol(sym); ok {
			return true
		}
	}
	if graph == nil {
		return false
	}

	forced := false
	graph.EachFuncDef(func(_ typecfg.Point, info *compcfg.FuncDefInfo) {
		if forced || info == nil || info.Symbol == 0 {
			return
		}
		if info.Symbol != sym {
			return
		}
		forced = info.TargetKind == compcfg.FuncDefField || info.TargetKind == compcfg.FuncDefMethod
	})

	return forced
}

func methodCalleeSymbolFromCall(bindings *bind.BindingTable, graph *compcfg.Graph, info *compcfg.CallInfo) (typecfg.SymbolID, bool) {
	if bindings == nil || info == nil || info.Method == "" {
		return 0, false
	}
	if sym, ok := methodSymbolFromCalleePath(bindings, graph, info.CalleePath, info.Method); ok {
		return sym, true
	}
	if info.Receiver == nil {
		return 0, false
	}
	return methodSymbolFromReceiver(bindings, graph, info.Receiver, info.Method)
}

func methodCalleeSymbolFromExpr(bindings *bind.BindingTable, graph *compcfg.Graph, ex *ast.FuncCallExpr) (typecfg.SymbolID, bool) {
	if bindings == nil || ex == nil || ex.Method == "" || ex.Receiver == nil {
		return 0, false
	}
	return methodSymbolFromReceiver(bindings, graph, ex.Receiver, ex.Method)
}

func methodSymbolFromReceiver(bindings *bind.BindingTable, graph *compcfg.Graph, receiver ast.Expr, method string) (typecfg.SymbolID, bool) {
	baseSym, receiverSegs, ok := StaticPathWithBaseSymbol(bindings, receiver)
	if !ok || baseSym == 0 {
		return 0, false
	}

	segs := make([]constraint.Segment, 0, len(receiverSegs)+1)
	segs = append(segs, receiverSegs...)
	segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: method})

	path, ok := bind.FieldPathKeyFromSegments(segs)
	if !ok {
		return 0, false
	}

	return methodSymbolFromBaseWithAliases(bindings, graph, baseSym, path)
}

func methodSymbolFromCalleePath(bindings *bind.BindingTable, graph *compcfg.Graph, calleePath constraint.Path, method string) (typecfg.SymbolID, bool) {
	if bindings == nil || method == "" || calleePath.Symbol == 0 {
		return 0, false
	}

	parts := make([]constraint.Segment, 0, len(calleePath.Segments)+1)
	for _, seg := range calleePath.Segments {
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString, constraint.SegmentIndexInt:
			parts = append(parts, seg)
		default:
			return 0, false
		}
	}

	parts = append(parts, constraint.Segment{Kind: constraint.SegmentField, Name: method})

	path, ok := bind.FieldPathKeyFromSegments(parts)
	if !ok {
		return 0, false
	}

	return methodSymbolFromBaseWithAliases(bindings, graph, calleePath.Symbol, path)
}

func methodSymbolFromBaseWithAliases(
	bindings *bind.BindingTable,
	graph *compcfg.Graph,
	baseSym typecfg.SymbolID,
	path string,
) (typecfg.SymbolID, bool) {
	if bindings == nil || baseSym == 0 {
		return 0, false
	}

	// Always try the direct base symbol first. Alias expansion is optional
	// enrichment and must not be required for non-alias method resolution.
	if sym, ok := bindings.FieldSymbol(baseSym, path); ok && sym != 0 {
		return sym, true
	}
	if graph == nil {
		return 0, false
	}

	var methodSym typecfg.SymbolID
	graph.EachAliasSymbol(baseSym, func(candidate typecfg.SymbolID) bool {
		if sym, ok := bindings.FieldSymbol(candidate, path); ok && sym != 0 {
			methodSym = sym
			return true
		}
		return false
	})
	if methodSym == 0 {
		return 0, false
	}
	return methodSym, true
}
