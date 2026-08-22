package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// Builder constructs CFG Graphs from AST nodes through incremental traversal.
//
// Builder maintains the state needed to transform an AST function body into
// a control flow graph. It tracks the current program point, pending gotos,
// loop exits, and scope visibility during traversal.
//
// The build process involves:
//  1. Traversing statements to create CFG nodes
//  2. Connecting nodes with edges (including conditional edges)
//  3. Tracking scope entry/exit for lexical scoping
//  4. Computing SSA versions via dominance frontiers
//  5. Collecting nested function definitions
//
// Usage is internal to the package. External code should use [Build] or
// [BuildWithBindings] to construct graphs.
type Builder struct {
	Cfg         *basecfg.CFG
	Info        map[basecfg.Point]NodeInfo
	Nested      []NestedFunc
	Current     basecfg.Point
	CurrentLive bool
	Labels      map[string]basecfg.Point
	Pending     map[string][]basecfg.Point
	LoopExits   []basecfg.Point

	// SSA versioning state (keyed by basecfg.SymbolID for scope-aware versioning)
	NextVersionID         map[basecfg.SymbolID]int                       // symbol -> next version ID
	VisibleVersion        map[basecfg.Point]map[basecfg.SymbolID]Version // (point, symbol) -> visible version (legacy sparse)
	VisibleVersionByPoint []map[basecfg.SymbolID]Version                 // dense point-indexed view
	PhiNodes              []PhiInfo                                      // Collected phi nodes

	// Scope tracking (structural visibility)
	ScopeTracker *ScopeTracker

	// Binding table (AST ident -> symbol, populated by binder pass)
	Bindings *bind.BindingTable

	// Function parameters (collected during ParamDefs)
	ParamNames      []string
	ParamSymbols    []basecfg.SymbolID
	ParamDeclPoints []basecfg.Point
}

// NewBuilder creates a new Builder with initialized fields.
func NewBuilder() *Builder {
	return NewBuilderWithCapacity(0, 0)
}

// NewBuilderWithCapacity creates a new Builder with initial CFG size hints.
func NewBuilderWithCapacity(nodeCap, edgeCap int) *Builder {
	if nodeCap < 0 {
		nodeCap = 0
	}
	mapCap := nodeCap
	switch {
	case mapCap > 256:
		mapCap = 256
	case mapCap > 128:
		mapCap = 128
	case mapCap > 64:
		mapCap = 64
	}

	return &Builder{
		Cfg:            basecfg.NewWithCapacity(nodeCap, edgeCap),
		Info:           make(map[basecfg.Point]NodeInfo, mapCap),
		CurrentLive:    true,
		Labels:         make(map[string]basecfg.Point),
		Pending:        make(map[string][]basecfg.Point),
		NextVersionID:  make(map[basecfg.SymbolID]int),
		VisibleVersion: make(map[basecfg.Point]map[basecfg.SymbolID]Version),
		ScopeTracker:   NewScopeTrackerWithCapacity(nodeCap),
	}
}

// AddNodeWithSnapshot creates a CFG node and immediately takes a visibility snapshot.
func (b *Builder) AddNodeWithSnapshot(kind basecfg.NodeKind, target basecfg.SymbolID, callee string) basecfg.Point {
	point := b.Cfg.AddNode(kind, target, callee)
	b.ScopeTracker.SnapshotVisibility(point)

	return point
}

// symbolFromIdent looks up the symbol for an identifier expression using bindings.
func (b *Builder) symbolFromIdent(ident *ast.IdentExpr) (basecfg.SymbolID, bool) {
	if ident == nil || b.Bindings == nil {
		return 0, false
	}

	return b.Bindings.SymbolOf(ident)
}

// symbolFromExpr looks up the symbol for an expression if it's an identifier.
func (b *Builder) symbolFromExpr(expr ast.Expr) (basecfg.SymbolID, bool) {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		return b.symbolFromIdent(ident)
	}

	return 0, false
}

// pathFromExpr builds a binding-based constraint.Path from an expression.
func (b *Builder) pathFromExpr(expr ast.Expr) constraint.Path {
	switch exprTyped := expr.(type) {
	case *ast.IdentExpr:
		var sym basecfg.SymbolID
		if b.Bindings != nil {
			sym, _ = b.Bindings.SymbolOf(exprTyped)
		}

		return constraint.Path{Root: exprTyped.Value, Symbol: sym, Segments: nil, Version: 0}

	case *ast.AttrGetExpr:
		base := b.pathFromExpr(exprTyped.Object)
		if base.IsEmpty() {
			return constraint.Path{Root: "", Symbol: 0, Segments: nil, Version: 0}
		}

		switch key := exprTyped.Key.(type) {
		case *ast.StringExpr:
			if pathkey.IsIdentName(key.Value) {
				return base.Append(constraint.Segment{Kind: constraint.SegmentField, Name: key.Value, Index: 0})
			}

			return base.Append(constraint.Segment{Kind: constraint.SegmentIndexString, Name: key.Value, Index: 0})

		case *ast.NumberExpr:
			if idx, ok := pathkey.ParseIntLiteral(key.Value); ok {
				return base.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx, Name: ""})
			}
		}
	}

	return constraint.Path{Root: "", Symbol: 0, Segments: nil, Version: 0}
}

// resolveCallInfoSymbols resolves symbol IDs for a call using bindings.
func (b *Builder) resolveCallInfoSymbols(info *CallInfo) {
	if info == nil {
		return
	}

	if info.Callee != nil {
		if sym, ok := b.symbolFromExpr(info.Callee); ok {
			info.CalleeSymbol = sym
		}
	}

	if info.Receiver != nil {
		if sym, ok := b.symbolFromExpr(info.Receiver); ok {
			info.ReceiverSymbol = sym
		}
	}

	if len(info.Args) > 0 {
		var argSymbols []basecfg.SymbolID

		for argIndex, arg := range info.Args {
			if sym, ok := b.symbolFromExpr(arg); ok {
				if argSymbols == nil {
					argSymbols = make([]basecfg.SymbolID, len(info.Args))
				}
				argSymbols[argIndex] = sym
			}
		}
		info.ArgSymbols = argSymbols

		if info.IsTypeCheck {
			info.TypeCheckPath = b.pathFromExpr(info.Args[0])
		}
	}

	if info.Method != "" {
		info.CalleePath = b.pathFromExpr(info.Receiver)
		if info.CalleeSymbol == 0 && !info.CalleePath.IsEmpty() {
			methodPath := info.CalleePath.Append(constraint.Segment{Kind: constraint.SegmentField, Name: info.Method, Index: 0})
			info.CalleeSymbol = b.resolveFieldPathSymbol(methodPath)
		}
	} else {
		info.CalleePath = b.pathFromExpr(info.Callee)
		if info.CalleeSymbol == 0 && !info.CalleePath.IsEmpty() && len(info.CalleePath.Segments) > 0 {
			info.CalleeSymbol = b.resolveFieldPathSymbol(info.CalleePath)
		}
	}

	if info.CalleeSymbol == 0 && info.Callee != nil {
		if fnLit, ok := info.Callee.(*ast.FunctionExpr); ok && b.Bindings != nil {
			info.CalleeSymbol = b.Bindings.GetOrCreateFuncLitSymbol(fnLit)
		}
	}
}

// resolveCallInfos resolves symbol IDs for a slice of call infos.
func (b *Builder) resolveCallInfos(infos []*CallInfo) {
	for _, info := range infos {
		b.resolveCallInfoSymbols(info)
	}
}

// resolveSourceSymbols resolves symbol IDs for source expressions in AssignInfo.
func (b *Builder) resolveSourceSymbols(info *AssignInfo, exprs []ast.Expr) {
	if info == nil || len(exprs) == 0 {
		return
	}

	var sourceSymbols []basecfg.SymbolID

	for exprIndex, expr := range exprs {
		if sym, ok := b.symbolFromExpr(expr); ok {
			if sourceSymbols == nil {
				sourceSymbols = make([]basecfg.SymbolID, len(exprs))
			}
			sourceSymbols[exprIndex] = sym

			continue
		}

		path := b.pathFromExpr(expr)
		if !path.IsEmpty() && len(path.Segments) > 0 {
			if sym := b.resolveFieldPathSymbol(path); sym != 0 {
				if sourceSymbols == nil {
					sourceSymbols = make([]basecfg.SymbolID, len(exprs))
				}
				sourceSymbols[exprIndex] = sym
			}
		}
	}
	info.SourceSymbols = sourceSymbols
}

// resolveFieldBaseSymbol resolves the base symbol for a field access expression.
func (b *Builder) resolveFieldBaseSymbol(expr ast.Expr) basecfg.SymbolID {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if baseIdent, ok := e.Object.(*ast.IdentExpr); ok {
			sym, _ := b.symbolFromIdent(baseIdent)

			return sym
		}

		return b.resolveFieldBaseSymbol(e.Object)
	case *ast.IdentExpr:
		sym, _ := b.symbolFromIdent(e)

		return sym
	}

	return 0
}

// resolveIdentsToSymbols converts ident expressions to SymbolIDs using bindings.
func (b *Builder) resolveIdentsToSymbols(idents []*ast.IdentExpr) []basecfg.SymbolID {
	if len(idents) == 0 || b.Bindings == nil {
		return nil
	}

	symbols := make([]basecfg.SymbolID, 0, len(idents))

	for _, ident := range idents {
		if sym, ok := b.Bindings.SymbolOf(ident); ok && sym != 0 {
			symbols = append(symbols, sym)
		}
	}

	return symbols
}

// getOrCreateFieldPathSymbol returns or creates a symbol for a field path.
func (b *Builder) getOrCreateFieldPathSymbol(baseSym basecfg.SymbolID, segments []constraint.Segment) basecfg.SymbolID {
	if baseSym == 0 || len(segments) == 0 || b.Bindings == nil {
		return 0
	}

	path, ok := bind.FieldPathKeyFromSegments(segments)
	if !ok {
		return 0
	}

	return b.Bindings.GetOrCreateFieldSymbol(baseSym, path)
}

// resolveFieldPathSymbol looks up the symbol for a field path from the binder.
func (b *Builder) resolveFieldPathSymbol(path constraint.Path) basecfg.SymbolID {
	if b.Bindings == nil || path.IsEmpty() || path.Symbol == 0 || len(path.Segments) == 0 {
		return 0
	}

	currentSym := path.Symbol

	for _, seg := range path.Segments {
		pathKey, ok := bind.FieldPathKeyFromSegments([]constraint.Segment{seg})
		if !ok {
			return 0
		}

		fieldSym, ok := b.Bindings.FieldSymbol(currentSym, pathKey)
		if !ok {
			return b.resolveFieldPathSymbolFlat(path)
		}

		currentSym = fieldSym
	}

	return currentSym
}

// resolveFieldPathSymbolFlat resolves using a single flat path string.
func (b *Builder) resolveFieldPathSymbolFlat(path constraint.Path) basecfg.SymbolID {
	if len(path.Segments) == 0 {
		return 0
	}

	pathStr, ok := bind.FieldPathKeyFromSegments(path.Segments)
	if !ok {
		return 0
	}
	sym, _ := b.Bindings.FieldSymbol(path.Symbol, pathStr)

	return sym
}
