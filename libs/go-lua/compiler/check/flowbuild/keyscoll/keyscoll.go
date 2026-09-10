package keyscoll

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
)

// KeysCollectorInfo tracks that a function returns keys of one of its parameters.
type KeysCollectorInfo struct {
	ParamIndex  int // Which parameter the keys come from (0-based)
	ReturnIndex int // Which return slot carries the keys table (0-based)
}

// DetectKeysCollector analyzes a function body to detect if it follows the
// "keys collector" pattern: creates a table, iterates with pairs over a param,
// inserts keys into the table, and returns it.
//
// Pattern:
//
//	local keys = {}
//	for k in pairs(param) do
//	    table.insert(keys, k)
//	end
//	return keys
func DetectKeysCollector(fn *ast.FunctionExpr) *KeysCollectorInfo {
	if fn == nil || fn.Stmts == nil || len(fn.Stmts) == 0 {
		return nil
	}

	graph := cfg.Build(fn)
	if graph == nil {
		return nil
	}

	// Use graph's own bindings since we build a fresh CFG.
	// Passed-in bindings may have different symbol IDs.
	bindings := graph.Bindings()

	// Track: which local symbol is the "keys" table
	// Track: which param symbol is being iterated with pairs
	var keysTableSym cfg.SymbolID
	var pairsParamSym cfg.SymbolID
	pairsParamIndex := -1
	var keyVarSym cfg.SymbolID
	insertedKeyIntoTable := false
	keysReturnIndex := -1

	paramSymbols := graph.ParamSymbols()

	// Scan for local keys = {} pattern and generic for loop with pairs
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 {
			return
		}

		// Check for local keys = {} pattern
		if info.IsLocal && len(info.Sources) > 0 {
			if target, ok := info.FirstTarget(); ok {
				if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
					if tbl, ok := info.SourceAt(0).(*ast.TableExpr); ok && tbl != nil && len(tbl.Fields) == 0 {
						if keysTableSym == 0 {
							keysTableSym = target.Symbol
						}
					}
				}
			}
		}

		// Check for generic for loop with pairs
		if len(info.IterExprs) > 0 {
			call, ok := info.IterExprs[0].(*ast.FuncCallExpr)
			if !ok || call == nil {
				return
			}
			// Check if it's pairs(something)
			if !isPairsCall(call) {
				return
			}
			if len(call.Args) == 0 {
				return
			}
			// Check if the argument is a parameter
			argIdent, ok := call.Args[0].(*ast.IdentExpr)
			if !ok {
				return
			}
			var argSym cfg.SymbolID
			if bindings != nil {
				argSym, _ = bindings.SymbolOf(argIdent)
			}
			if argSym == 0 {
				// Try graph bindings
				if gb := graph.Bindings(); gb != nil {
					argSym, _ = gb.SymbolOf(argIdent)
				}
			}
			// Check if argSym is a parameter
			for i, ps := range paramSymbols {
				if ps == argSym {
					pairsParamSym = argSym
					pairsParamIndex = i
					break
				}
			}
			// Track the key variable (first loop variable)
			if target, ok := info.FirstTarget(); ok && target.Kind == cfg.TargetIdent {
				keyVarSym = target.Symbol
			}
		}
	})

	if keysTableSym == 0 || pairsParamSym == 0 || pairsParamIndex < 0 || keyVarSym == 0 {
		return nil
	}

	// Scan for table.insert(keys, k) pattern
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		if !isTableInsertCall(info) {
			return
		}
		if len(info.Args) < 2 {
			return
		}
		// Check first arg is the keys table
		argIdent, ok := info.Args[0].(*ast.IdentExpr)
		if !ok {
			return
		}
		var argSym cfg.SymbolID
		if bindings != nil {
			argSym, _ = bindings.SymbolOf(argIdent)
		}
		if argSym == 0 {
			if gb := graph.Bindings(); gb != nil {
				argSym, _ = gb.SymbolOf(argIdent)
			}
		}
		if argSym != keysTableSym {
			return
		}
		// Check second arg is the key variable
		valIdent, ok := info.Args[1].(*ast.IdentExpr)
		if !ok {
			return
		}
		var valSym cfg.SymbolID
		if bindings != nil {
			valSym, _ = bindings.SymbolOf(valIdent)
		}
		if valSym == 0 {
			if gb := graph.Bindings(); gb != nil {
				valSym, _ = gb.SymbolOf(valIdent)
			}
		}
		if valSym == keyVarSym {
			insertedKeyIntoTable = true
		}
	})

	if !insertedKeyIntoTable {
		return nil
	}

	resolveIdentSym := func(expr ast.Expr) cfg.SymbolID {
		ident, ok := expr.(*ast.IdentExpr)
		if !ok {
			return 0
		}
		var sym cfg.SymbolID
		if bindings != nil {
			sym, _ = bindings.SymbolOf(ident)
		}
		if sym == 0 {
			if gb := graph.Bindings(); gb != nil {
				sym, _ = gb.SymbolOf(ident)
			}
		}
		return sym
	}

	// Scan for return keys pattern with a stable return slot index.
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}

		foundIdx := -1
		for i, expr := range info.Exprs {
			if resolveIdentSym(expr) != keysTableSym {
				continue
			}
			if foundIdx >= 0 {
				// Ambiguous: same return statement exposes keys at multiple slots.
				keysReturnIndex = -2
				return
			}
			foundIdx = i
		}
		if foundIdx < 0 {
			// Every return statement must carry the keys table for sound provenance.
			keysReturnIndex = -2
			return
		}
		if keysReturnIndex == -1 {
			keysReturnIndex = foundIdx
			return
		}
		if keysReturnIndex != foundIdx {
			// Ambiguous across return statements.
			keysReturnIndex = -2
		}
	})

	if keysReturnIndex < 0 {
		return nil
	}

	return &KeysCollectorInfo{ParamIndex: pairsParamIndex, ReturnIndex: keysReturnIndex}
}

func isPairsCall(call *ast.FuncCallExpr) bool {
	if call == nil || callsite.IsMethodLikeExpr(call) {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return false
	}
	return ident.Value == "pairs"
}

func isTableInsertCall(info *cfg.CallInfo) bool {
	if info == nil || callsite.IsMethodLikeCallInfo(info) {
		return false
	}
	attr, ok := info.Callee.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj.Value != "table" {
		return false
	}
	key, ok := attr.Key.(*ast.StringExpr)
	if !ok || key.Value != "insert" {
		return false
	}
	return true
}

// BuildKeysCollectorDetector returns a callback that detects if a call is to a
// keys collector function and returns the symbol of the table argument.
func BuildKeysCollectorDetector(graph *cfg.Graph, moduleBindings *bind.BindingTable) func(*cfg.CallInfo, cfg.Point, int) cfg.SymbolID {
	cache := make(map[cfg.SymbolID]*KeysCollectorInfo)
	bindings := graph.Bindings()

	return func(callInfo *cfg.CallInfo, _ cfg.Point, retIndex int) cfg.SymbolID {
		if callInfo == nil || callInfo.Callee == nil {
			return 0
		}
		candidates := callsite.CallableCalleeSymbolCandidates(callInfo, graph, bindings, moduleBindings)
		for _, calleeSym := range candidates {
			// Check cache
			if info, ok := cache[calleeSym]; ok {
				if info == nil {
					continue
				}
				if info.ReturnIndex != retIndex {
					continue
				}
				return callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(callInfo, info.ParamIndex), bindings)
			}

			// Try to resolve callee to function literal
			fn := resolve.ResolveSymbolToFunctionLiteral(graph, calleeSym)
			if fn == nil {
				cache[calleeSym] = nil
				continue
			}

			info := DetectKeysCollector(fn)
			cache[calleeSym] = info
			if info == nil {
				continue
			}
			if info.ReturnIndex != retIndex {
				continue
			}
			return callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(callInfo, info.ParamIndex), bindings)
		}
		return 0
	}
}
