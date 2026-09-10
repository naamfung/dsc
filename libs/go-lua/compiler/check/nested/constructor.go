package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// This file implements detection and analysis of the Lua OOP constructor pattern.
//
// Lua doesn't have built-in classes, but a common idiom uses metatables for OOP:
//
//	local T = {}
//	function T.new()
//	    local self = setmetatable({}, {__index = T})
//	    self.field = value
//	    return self
//	end
//
// The constructor pattern consists of:
//  1. A function named "new" on a table (the class)
//  2. Creating self via setmetatable with the class as metatable
//  3. Assigning instance fields to self
//  4. Returning self
//
// Detecting this pattern enables the type checker to:
//   - Track which fields are assigned in constructors
//   - Provide accurate self-type in instance methods
//   - Enable autocompletion for instance fields

// DetectConstructorPattern checks if a function is a constructor that:
// 1. Is named T.new (assigned to a field named "new" on a table)
// 2. Creates self via setmetatable({}, T) or setmetatable({}, {__index = T})
// 3. Returns the self variable
//
// The nestedGraph is the CFG of the function being analyzed.
// The parentGraph is the CFG where the function is defined (needed for T.new = function() pattern).
func DetectConstructorPattern(nestedGraph, parentGraph *cfg.Graph, fn *ast.FunctionExpr, funcDef *cfg.FuncDefInfo) (classSymbol, selfSymbol cfg.SymbolID) {
	if nestedGraph == nil || fn == nil {
		return 0, 0
	}

	// Check if function is T.new pattern
	var receiverSymbol cfg.SymbolID
	var receiverName string
	if funcDef != nil && funcDef.TargetKind == cfg.FuncDefField {
		if funcDef.TargetPath.Symbol != 0 && len(funcDef.TargetPath.Segments) == 1 {
			seg := funcDef.TargetPath.Segments[0]
			if seg.Kind == constraint.SegmentField && seg.Name == "new" {
				receiverSymbol = funcDef.TargetPath.Symbol
				receiverName = funcDef.ReceiverName
			}
		}
	}

	// Also check for T.new = function(...) pattern in the parent graph
	if receiverSymbol == 0 && parentGraph != nil {
		var found cfg.SymbolID
		var foundName string
		parentGraph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
			if found != 0 {
				return
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
				fnExpr, ok := src.(*ast.FunctionExpr)
				if !ok || fnExpr != fn {
					return
				}
				if target.Kind == cfg.TargetField && target.BaseSymbol != 0 && len(target.FieldPath) == 1 && target.FieldPath[0] == "new" {
					found = target.BaseSymbol
					foundName = target.BaseName
				}
			})
		})
		receiverSymbol = found
		receiverName = foundName
	}

	if receiverSymbol == 0 {
		return 0, 0
	}

	// Find setmetatable call that creates self
	selfSym := findSetmetatablePatternByName(nestedGraph, receiverName)
	if selfSym == 0 {
		return 0, 0
	}

	// Check that self is returned
	if !isSymbolReturned(nestedGraph, selfSym) {
		return 0, 0
	}

	return receiverSymbol, selfSym
}

func findSetmetatablePatternByName(graph *cfg.Graph, expectedClassName string) cfg.SymbolID {
	if graph == nil {
		return 0
	}

	var selfSym cfg.SymbolID

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if selfSym != 0 {
			return
		}
		if !info.IsLocal || len(info.Targets) == 0 {
			return
		}

		// Look for setmetatable call
		call, ok := info.SourceAt(0).(*ast.FuncCallExpr)
		if !ok {
			return
		}

		// Check if it's a setmetatable call
		ident, ok := call.Func.(*ast.IdentExpr)
		if !ok || ident.Value != "setmetatable" {
			return
		}

		if len(call.Args) < 2 {
			return
		}

		// First arg should be an empty table literal or table with initial values
		if _, ok := call.Args[0].(*ast.TableExpr); !ok {
			return
		}

		// Second arg is the metatable - check for T or {__index = T}
		var foundClassName string

		switch mt := call.Args[1].(type) {
		case *ast.IdentExpr:
			foundClassName = mt.Value
		case *ast.TableExpr:
			for _, field := range mt.Fields {
				if field.Key == nil {
					continue
				}
				keyStr, ok := field.Key.(*ast.StringExpr)
				if !ok || keyStr.Value != "__index" {
					continue
				}
				if valIdent, ok := field.Value.(*ast.IdentExpr); ok {
					foundClassName = valIdent.Value
				}
			}
		}

		// Validate class name if expected
		if expectedClassName != "" && foundClassName != expectedClassName {
			return
		}

		if target, ok := info.FirstTarget(); ok {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				selfSym = target.Symbol
			}
		}
	})

	return selfSym
}

// isSymbolReturnedOnAllPaths checks if a symbol is returned on ALL return paths.
// Returns false if any return path returns something other than the symbol.
func isSymbolReturned(graph *cfg.Graph, sym cfg.SymbolID) bool {
	if graph == nil || sym == 0 {
		return false
	}

	bindings := graph.Bindings()
	if bindings == nil {
		return false
	}

	hasReturn := false
	allReturnSym := true
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if !allReturnSym {
			return
		}
		hasReturn = true

		// Empty return or return with no expressions doesn't return self.
		if len(info.Exprs) == 0 {
			allReturnSym = false
			return
		}

		// Check if first return expression is the symbol.
		ident, ok := info.Exprs[0].(*ast.IdentExpr)
		if !ok {
			allReturnSym = false
			return
		}
		retSym, ok := bindings.SymbolOf(ident)
		if !ok || retSym != sym {
			allReturnSym = false
		}
	})

	return hasReturn && allReturnSym
}

// CollectConstructorFields collects field assignments to a self symbol in a constructor.
//
// This scans the constructor's CFG for statements like `self.field = value` and
// builds a map of field names to their types. These fields become part of the
// class's instance type, enabling the type checker to validate field access
// on instances created by this constructor.
func CollectConstructorFields(graph *cfg.Graph, selfSym cfg.SymbolID, synth func(ast.Expr, cfg.Point) typ.Type) map[string]typ.Type {
	if graph == nil || selfSym == 0 {
		return nil
	}

	filterSyms := map[cfg.SymbolID]bool{selfSym: true}
	fields := assign.CollectFieldAssignments(graph, synth, filterSyms)

	if selfFields, ok := fields[selfSym]; ok && len(selfFields) > 0 {
		return selfFields
	}
	return nil
}
