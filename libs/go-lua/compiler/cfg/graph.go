package cfg

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Graph holds CFG with pre-extracted node info. Immutable after Build().
type Graph struct {
	cfg         *basecfg.CFG
	infoByPoint []NodeInfo
	nested      []NestedFunc
	fn          *ast.FunctionExpr
	// Precomputed point order/indexes for hot iterator paths.
	orderedPoints         []Point
	orderedAssignPoints   []Point
	orderedStmtCallPoints []Point
	orderedReturnPoints   []Point
	orderedBranchPoints   []Point
	orderedFuncDefPoints  []Point
	orderedTypeDefPoints  []Point

	// Binding table (AST ident -> symbol, populated before CFG build)
	bindings *bind.BindingTable

	// SSA versioning (keyed by basecfg.SymbolID for scope-aware versioning)
	phiNodes              []PhiInfo                              // Phi nodes at join points
	visibleVersion        map[Point]map[basecfg.SymbolID]Version // (point, symbol) -> visible version
	visibleVersionByPoint []map[basecfg.SymbolID]Version         // dense point-indexed view of visibleVersion
	nextVersionID         map[basecfg.SymbolID]int               // symbol -> next version ID

	// Scope visibility (structural, computed during build)
	symbolScope        map[Point]map[string]basecfg.SymbolID   // point -> name -> symbol
	symbolScopeByPoint []map[string]basecfg.SymbolID           // dense point-indexed view of symbolScope
	globals            map[string]basecfg.SymbolID             // lazy globals overlay
	mergedSymbols      map[Point]map[string]basecfg.SymbolID   // cached merged local+globals
	declPoints         map[basecfg.SymbolID]Point              // symbol -> declaration point
	symbolNames        map[basecfg.SymbolID]string             // symbol -> name (reverse lookup for display)
	symbolKinds        map[basecfg.SymbolID]basecfg.SymbolKind // symbol -> kind (Param/Local/Global)
	directAliases      map[basecfg.SymbolID]basecfg.SymbolID   // target local symbol -> unambiguous direct source symbol

	// Function parameters (precomputed for downstream use)
	paramNames      []string
	paramSymbols    []basecfg.SymbolID
	paramDeclPoints []Point
	paramSlots      []ParamSlot
}

// Compile-time check that Graph implements VersionedGraph interface.
var _ basecfg.VersionedGraph = (*Graph)(nil)

type pointIndex struct {
	all      []Point
	assign   []Point
	stmtCall []Point
	ret      []Point
	branch   []Point
	funcDef  []Point
	typeDef  []Point
}

func buildPointIndex(info map[basecfg.Point]NodeInfo, size int) pointIndex {
	if len(info) == 0 {
		return pointIndex{}
	}

	all := make([]Point, 0, len(info))
	if size > 0 {
		for i := range size {
			p := Point(i)
			if _, ok := info[p]; ok {
				all = append(all, p)
			}
		}
	}
	if len(all) != len(info) {
		all = all[:0]
		for p := range info {
			all = append(all, p)
		}
		sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	}

	assignCount := 0
	stmtCallCount := 0
	retCount := 0
	branchCount := 0
	funcDefCount := 0
	typeDefCount := 0
	for _, p := range all {
		switch info[p].(type) {
		case *AssignInfo:
			assignCount++
		case *CallInfo:
			stmtCallCount++
		case *ReturnInfo:
			retCount++
		case *BranchInfo:
			branchCount++
		case *FuncDefInfo:
			funcDefCount++
		case *TypeDefInfo:
			typeDefCount++
		}
	}

	idx := pointIndex{
		all:      all,
		assign:   make([]Point, 0, assignCount),
		stmtCall: make([]Point, 0, stmtCallCount),
		ret:      make([]Point, 0, retCount),
		branch:   make([]Point, 0, branchCount),
		funcDef:  make([]Point, 0, funcDefCount),
		typeDef:  make([]Point, 0, typeDefCount),
	}
	for _, p := range all {
		switch info[p].(type) {
		case *AssignInfo:
			idx.assign = append(idx.assign, p)
		case *CallInfo:
			idx.stmtCall = append(idx.stmtCall, p)
		case *ReturnInfo:
			idx.ret = append(idx.ret, p)
		case *BranchInfo:
			idx.branch = append(idx.branch, p)
		case *FuncDefInfo:
			idx.funcDef = append(idx.funcDef, p)
		case *TypeDefInfo:
			idx.typeDef = append(idx.typeDef, p)
		}
	}

	return idx
}

// Build creates an immutable Graph for a function body.
// If globals is provided, those names are seeded into the binder's root scope
// with stable SymbolIDs before traversal, enabling global symbol resolution.
func Build(fn *ast.FunctionExpr, globals ...string) *Graph {
	if fn == nil {
		return nil
	}

	// Run binder pass first to bind all identifiers to symbols
	bindings := bind.Bind(fn, globals)

	return BuildWithBindings(fn, bindings)
}

// BuildWithBindings creates an immutable Graph using a pre-computed binding table.
// This allows sharing SymbolIDs across multiple function graphs within a module.
func BuildWithBindings(fn *ast.FunctionExpr, bindings *bind.BindingTable) *Graph {
	if fn == nil {
		return nil
	}

	if bindings == nil {
		return Build(fn)
	}

	nodeCap, edgeCap := estimateFunctionCFGCapacity(fn)
	b := NewBuilderWithCapacity(nodeCap, edgeCap)
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	// Register globals from bindings in scope tracker at entry point
	entry := b.Cfg.Entry()

	for _, sym := range bindings.Globals() {
		name := bindings.Name(sym)
		b.ScopeTracker.RegisterGlobal(sym, name, entry)
	}

	// Register captured variables as globals so they get SSA versions.
	// This enables flow narrowing for captured variables inside nested functions.
	for _, sym := range bindings.CapturedSymbols(fn) {
		name := bindings.Name(sym)
		if name != "" && sym != 0 {
			b.ScopeTracker.RegisterGlobal(sym, name, entry)
		}
	}

	b.ScopeTracker.SnapshotVisibility(b.Current)
	b.ParamDefs(fn)
	b.Stmts(fn.Stmts)

	b.ResolvePendingGotos()

	if b.CurrentLive {
		// Add implicit return node for functions that don't end with explicit return.
		// This allows effect inference to capture constraints at implicit returns.
		implicitReturn := b.Cfg.AddNode(basecfg.NodeReturn, 0, "")
		b.Cfg.AddEdge(b.Current, implicitReturn, false)
		b.ScopeTracker.SnapshotVisibility(implicitReturn)
		b.Info[implicitReturn] = &ReturnInfo{Exprs: nil}
		b.Cfg.AddEdge(implicitReturn, b.Cfg.Exit(), false)
	}

	b.ScopeTracker.SnapshotVisibility(b.Cfg.Exit())

	// Compute SSA versions and insert phi nodes
	b.ComputeSSAVersions()

	visibleVersion := b.VisibleVersion
	visibleVersionByPoint := b.VisibleVersionByPoint
	symbolScope := b.StealScopeVisibility()
	globalsMap := b.StealGlobals()
	declPoints := b.StealDeclPoints()
	symbolNames := b.StealSymbolNames()
	symbolKinds := b.StealSymbolKinds()
	paramSlots := buildParamSlots(fn, b.ParamNames, b.ParamSymbols, b.ParamDeclPoints, symbolNames)
	size := b.Cfg.Size()
	pointIdx := buildPointIndex(b.Info, size)
	infoByPoint := denseNodeInfoByPoint(b.Info, size)

	if len(visibleVersionByPoint) == 0 {
		visibleVersionByPoint = denseVisibleVersionByPoint(visibleVersion, size)
	}

	return &Graph{
		cfg:                   b.Cfg,
		infoByPoint:           infoByPoint,
		nested:                b.Nested,
		fn:                    fn,
		orderedPoints:         pointIdx.all,
		orderedAssignPoints:   pointIdx.assign,
		orderedStmtCallPoints: pointIdx.stmtCall,
		orderedReturnPoints:   pointIdx.ret,
		orderedBranchPoints:   pointIdx.branch,
		orderedFuncDefPoints:  pointIdx.funcDef,
		orderedTypeDefPoints:  pointIdx.typeDef,
		bindings:              bindings,
		phiNodes:              b.PhiNodes,
		visibleVersion:        visibleVersion,
		visibleVersionByPoint: visibleVersionByPoint,
		nextVersionID:         b.NextVersionID,
		symbolScope:           symbolScope,
		symbolScopeByPoint:    denseSymbolScopeByPoint(symbolScope, size),
		globals:               globalsMap,
		declPoints:            declPoints,
		symbolNames:           symbolNames,
		symbolKinds:           symbolKinds,
		directAliases:         computeDirectAliasIndex(b.Info, bindings),
		paramNames:            b.ParamNames,
		paramSymbols:          b.ParamSymbols,
		paramDeclPoints:       b.ParamDeclPoints,
		paramSlots:            paramSlots,
	}
}

// BuildBlock creates an immutable Graph for a block of statements.
// If globals is provided, those names are seeded into the binder's root scope
// with stable SymbolIDs before traversal, enabling global symbol resolution.
func BuildBlock(stmts []ast.Stmt, globals ...string) *Graph {
	// Run binder pass first to bind all identifiers to symbols
	// Wrap statements in a synthetic function for binding
	syntheticFn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(syntheticFn, globals)

	nodeCap, edgeCap := estimateBlockCFGCapacity(stmts)
	b := NewBuilderWithCapacity(nodeCap, edgeCap)
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	// Register globals from bindings in scope tracker at entry point
	entry := b.Cfg.Entry()

	for _, sym := range bindings.Globals() {
		name := bindings.Name(sym)
		b.ScopeTracker.RegisterGlobal(sym, name, entry)
	}

	b.ScopeTracker.SnapshotVisibility(b.Current)
	b.Stmts(stmts)
	b.ResolvePendingGotos()

	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, b.Cfg.Exit(), false)
	}

	b.ScopeTracker.SnapshotVisibility(b.Cfg.Exit())

	// Compute SSA versions and insert phi nodes
	b.ComputeSSAVersions()

	visibleVersion := b.VisibleVersion
	visibleVersionByPoint := b.VisibleVersionByPoint
	symbolScope := b.StealScopeVisibility()
	globalsMap := b.StealGlobals()
	declPoints := b.StealDeclPoints()
	symbolNames := b.StealSymbolNames()
	symbolKinds := b.StealSymbolKinds()
	paramSlots := buildParamSlots(syntheticFn, b.ParamNames, b.ParamSymbols, b.ParamDeclPoints, symbolNames)
	size := b.Cfg.Size()
	pointIdx := buildPointIndex(b.Info, size)
	infoByPoint := denseNodeInfoByPoint(b.Info, size)

	if len(visibleVersionByPoint) == 0 {
		visibleVersionByPoint = denseVisibleVersionByPoint(visibleVersion, size)
	}

	return &Graph{
		cfg:                   b.Cfg,
		infoByPoint:           infoByPoint,
		nested:                b.Nested,
		fn:                    syntheticFn,
		orderedPoints:         pointIdx.all,
		orderedAssignPoints:   pointIdx.assign,
		orderedStmtCallPoints: pointIdx.stmtCall,
		orderedReturnPoints:   pointIdx.ret,
		orderedBranchPoints:   pointIdx.branch,
		orderedFuncDefPoints:  pointIdx.funcDef,
		orderedTypeDefPoints:  pointIdx.typeDef,
		bindings:              bindings,
		phiNodes:              b.PhiNodes,
		visibleVersion:        visibleVersion,
		visibleVersionByPoint: visibleVersionByPoint,
		nextVersionID:         b.NextVersionID,
		symbolScope:           symbolScope,
		symbolScopeByPoint:    denseSymbolScopeByPoint(symbolScope, size),
		globals:               globalsMap,
		declPoints:            declPoints,
		symbolNames:           symbolNames,
		symbolKinds:           symbolKinds,
		directAliases:         computeDirectAliasIndex(b.Info, bindings),
		paramSlots:            paramSlots,
	}
}

func denseVisibleVersionByPoint(
	visible map[Point]map[basecfg.SymbolID]Version,
	size int,
) []map[basecfg.SymbolID]Version {
	if len(visible) == 0 || size <= 0 {
		return nil
	}
	out := make([]map[basecfg.SymbolID]Version, size)
	for p, versions := range visible {
		idx := int(p)
		if idx >= 0 && idx < size {
			out[idx] = versions
		}
	}
	return out
}

func denseNodeInfoByPoint(
	info map[Point]NodeInfo,
	size int,
) []NodeInfo {
	if len(info) == 0 || size <= 0 {
		return nil
	}
	out := make([]NodeInfo, size)
	for p, nodeInfo := range info {
		idx := int(p)
		if idx >= 0 && idx < size {
			out[idx] = nodeInfo
		}
	}
	return out
}

func denseSymbolScopeByPoint(
	scope map[Point]map[string]basecfg.SymbolID,
	size int,
) []map[string]basecfg.SymbolID {
	if len(scope) == 0 || size <= 0 {
		return nil
	}
	out := make([]map[string]basecfg.SymbolID, size)
	for p, symbols := range scope {
		idx := int(p)
		if idx >= 0 && idx < size {
			out[idx] = symbols
		}
	}
	return out
}

// CFG returns the underlying control flow graph.
func (g *Graph) CFG() *basecfg.CFG {
	if g == nil {
		return nil
	}

	return g.cfg
}

// Func returns the root function expression for this graph, if any.
func (g *Graph) Func() *ast.FunctionExpr {
	if g == nil {
		return nil
	}

	return g.fn
}

// Info returns the node info at point p, or nil if none.
func (g *Graph) Info(p Point) NodeInfo {
	if g == nil {
		return nil
	}

	if idx := int(p); idx >= 0 && idx < len(g.infoByPoint) {
		return g.infoByPoint[idx]
	}

	return nil
}

// Assign returns AssignInfo at p, or nil if not an assign node.
func (g *Graph) Assign(p Point) *AssignInfo {
	info, _ := g.Info(p).(*AssignInfo)

	return info
}

// Call returns CallInfo at p, or nil if not a call node.
func (g *Graph) Call(p Point) *CallInfo {
	info, _ := g.Info(p).(*CallInfo)

	return info
}

// CallSitesAt returns all callsites represented at point p.
//
// A single point may represent:
//   - a direct call node
//   - an assignment node with source call expressions
//   - a return node with source call expressions
func (g *Graph) CallSitesAt(p Point) []*CallInfo {
	if g == nil {
		return nil
	}

	if info := g.Call(p); info != nil {
		return []*CallInfo{info}
	}

	if assign := g.Assign(p); assign != nil {
		var calls []*CallInfo
		for _, call := range assign.SourceCalls {
			if call != nil {
				calls = append(calls, call)
			}
		}
		return calls
	}

	if ret := g.Return(p); ret != nil {
		var calls []*CallInfo
		for _, call := range ret.SourceCalls {
			if call != nil {
				calls = append(calls, call)
			}
		}
		return calls
	}

	return nil
}

// CallSiteAt returns the callsite at point p matching expression ex.
// Returns nil when no matching callsite exists at that point.
func (g *Graph) CallSiteAt(p Point, ex *ast.FuncCallExpr) *CallInfo {
	if g == nil || ex == nil {
		return nil
	}

	for _, call := range g.CallSitesAt(p) {
		if call != nil && call.Call == ex {
			return call
		}
	}

	return nil
}

// Return returns ReturnInfo at p, or nil if not a return node.
func (g *Graph) Return(p Point) *ReturnInfo {
	info, _ := g.Info(p).(*ReturnInfo)

	return info
}

// Branch returns BranchInfo at p, or nil if not a branch node.
func (g *Graph) Branch(p Point) *BranchInfo {
	info, _ := g.Info(p).(*BranchInfo)

	return info
}

// FuncDef returns FuncDefInfo at p, or nil if not a funcdef node.
func (g *Graph) FuncDef(p Point) *FuncDefInfo {
	info, _ := g.Info(p).(*FuncDefInfo)

	return info
}

// TypeDef returns TypeDefInfo at p, or nil if not a typedef node.
func (g *Graph) TypeDef(p Point) *TypeDefInfo {
	info, _ := g.Info(p).(*TypeDefInfo)

	return info
}

// Iteration helpers for bulk processing.

func (g *Graph) sortedPoints(match func(NodeInfo) bool) []Point {
	if g == nil || len(g.infoByPoint) == 0 {
		return nil
	}
	if len(g.orderedPoints) == 0 {
		points := make([]Point, 0, len(g.infoByPoint))
		for i, info := range g.infoByPoint {
			if info == nil {
				continue
			}
			if match == nil || match(info) {
				points = append(points, Point(i))
			}
		}
		return points
	}
	if match == nil {
		points := make([]Point, len(g.orderedPoints))
		copy(points, g.orderedPoints)
		return points
	}
	points := make([]Point, 0, len(g.orderedPoints))
	for _, p := range g.orderedPoints {
		if match(g.Info(p)) {
			points = append(points, p)
		}
	}
	return points
}

// EachAssign calls fn for each assignment node in point order.
func (g *Graph) EachAssign(fn func(Point, *AssignInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedAssignPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*AssignInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*AssignInfo))
	}
}

// AssignPoints returns all assignment points.
func (g *Graph) AssignPoints() []Point {
	if g == nil || len(g.infoByPoint) == 0 {
		return nil
	}
	points := g.orderedAssignPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*AssignInfo)
			return ok
		})
	}
	if len(points) == 0 {
		return nil
	}
	out := make([]Point, len(points))
	copy(out, points)
	return out
}

// EachStmtCall calls fn for each call statement node in point order.
//
// This does not include call expressions embedded in assignment or return nodes.
func (g *Graph) EachStmtCall(fn func(Point, *CallInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedStmtCallPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*CallInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*CallInfo))
	}
}

// EachCall calls fn for each call statement node in point order.
//
// Deprecated: use EachStmtCall for statement-only traversal or EachCallSite
// to include embedded call expressions in assignment/return nodes.
func (g *Graph) EachCall(fn func(Point, *CallInfo)) {
	g.EachStmtCall(fn)
}

// EachCallSite calls fn for each call site in point order.
//
// A call site includes:
//   - Call statement nodes (foo())
//   - Call expressions inside assignment sources (local x = foo())
//   - Call expressions inside return statements (return foo())
//
// Calls embedded in assignment/return sources are yielded in source order.
func (g *Graph) EachCallSite(fn func(Point, *CallInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(nil)
	}
	for _, p := range points {
		switch info := g.Info(p).(type) {
		case *CallInfo:
			if info != nil {
				fn(p, info)
			}
		case *AssignInfo:
			for _, call := range info.SourceCalls {
				if call != nil {
					fn(p, call)
				}
			}
		case *ReturnInfo:
			for _, call := range info.SourceCalls {
				if call != nil {
					fn(p, call)
				}
			}
		}
	}
}

// EachReturn calls fn for each return node in point order.
func (g *Graph) EachReturn(fn func(Point, *ReturnInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedReturnPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*ReturnInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*ReturnInfo))
	}
}

// EachBranch calls fn for each branch node in point order.
func (g *Graph) EachBranch(fn func(Point, *BranchInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedBranchPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*BranchInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*BranchInfo))
	}
}

// EachFuncDef calls fn for each function definition node in point order.
func (g *Graph) EachFuncDef(fn func(Point, *FuncDefInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedFuncDefPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*FuncDefInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*FuncDefInfo))
	}
}

// EachTypeDef calls fn for each type definition node in point order.
func (g *Graph) EachTypeDef(fn func(Point, *TypeDefInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedTypeDefPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(func(info NodeInfo) bool {
			_, ok := info.(*TypeDefInfo)
			return ok
		})
	}
	for _, p := range points {
		fn(p, g.Info(p).(*TypeDefInfo))
	}
}

// EachNode calls fn for each node with info in point order.
func (g *Graph) EachNode(fn func(Point, NodeInfo)) {
	if g == nil || len(g.infoByPoint) == 0 {
		return
	}
	points := g.orderedPoints
	if points == nil && len(g.infoByPoint) > 0 {
		points = g.sortedPoints(nil)
	}
	for _, p := range points {
		fn(p, g.Info(p))
	}
}

// NestedFunctions returns all nested functions found during build.
func (g *Graph) NestedFunctions() []NestedFunc {
	if g == nil {
		return nil
	}

	return g.nested
}

// CFG delegated methods.

// Node returns the base CFG node at point p.
func (g *Graph) Node(p Point) *basecfg.Node {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Node(p)
}

// ID returns the unique identifier.
func (g *Graph) ID() uint64 {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.ID()
}

// Entry returns the entry point.
func (g *Graph) Entry() Point {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Entry()
}

// Exit returns the exit point.
func (g *Graph) Exit() Point {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Exit()
}

// Size returns the number of nodes.
func (g *Graph) Size() int {
	if g == nil || g.cfg == nil {
		return 0
	}

	return g.cfg.Size()
}

// RPO returns nodes in reverse post-order.
func (g *Graph) RPO() []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.RPO()
}

// Predecessors returns all predecessors of p.
func (g *Graph) Predecessors(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.PredecessorsReadOnly(p)
}

// PredecessorsReadOnly returns predecessors without copying.
//
// The returned slice must be treated as read-only by callers.
func (g *Graph) PredecessorsReadOnly(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.PredecessorsReadOnly(p)
}

// Successors returns all successors of p.
func (g *Graph) Successors(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.SuccessorsReadOnly(p)
}

// SuccessorsReadOnly returns successors without copying.
//
// The returned slice must be treated as read-only by callers.
func (g *Graph) SuccessorsReadOnly(p Point) []Point {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.SuccessorsReadOnly(p)
}

// Successor returns single successor (for non-branch nodes).
func (g *Graph) Successor(p Point) Point {
	if g == nil || g.cfg == nil {
		return p
	}

	return g.cfg.Successor(p)
}

// Predecessor returns single predecessor (for non-join nodes).
func (g *Graph) Predecessor(p Point) Point {
	if g == nil || g.cfg == nil {
		return p
	}

	return g.cfg.Predecessor(p)
}

// IsJoin returns true if p has multiple predecessors.
func (g *Graph) IsJoin(p Point) bool {
	if g == nil || g.cfg == nil {
		return false
	}

	return g.cfg.IsJoin(p)
}

// IsBranch returns true if p has multiple successors.
func (g *Graph) IsBranch(p Point) bool {
	if g == nil || g.cfg == nil {
		return false
	}

	return g.cfg.IsBranch(p)
}

// EdgeCond returns the condition value for edge from->to.
func (g *Graph) EdgeCond(from, to Point) (bool, bool) {
	if g == nil || g.cfg == nil {
		return false, false
	}

	return g.cfg.EdgeCond(from, to)
}

// Edges returns all edges.
func (g *Graph) Edges() []basecfg.Edge {
	if g == nil || g.cfg == nil {
		return nil
	}

	return g.cfg.Edges()
}

// SSA versioning methods (implements cfg.SSAVersioned)

// PhiNodes returns all phi nodes in the graph.
// Implements cfg.SymbolScope.
func (g *Graph) PhiNodes() []basecfg.PhiNode {
	if g == nil {
		return nil
	}

	return g.phiNodes
}

// VisibleVersion returns the SSA version of a symbol visible at a point.
// Returns a zero Version if the symbol is not defined on all paths to this point.
// Implements cfg.SymbolScope.
func (g *Graph) VisibleVersion(p Point, sym basecfg.SymbolID) Version {
	if g == nil {
		return Version{}
	}

	if idx := int(p); idx >= 0 && idx < len(g.visibleVersionByPoint) {
		if m := g.visibleVersionByPoint[idx]; m != nil {
			return m[sym]
		}
	}

	if g.visibleVersion != nil {
		if m := g.visibleVersion[p]; m != nil {
			return m[sym]
		}
	}

	return Version{}
}

// AllVisibleVersions returns all symbol versions visible at a point.
// The returned map should not be modified.
// Implements cfg.SSAVersioned.
func (g *Graph) AllVisibleVersions(p Point) map[basecfg.SymbolID]Version {
	if g == nil {
		return nil
	}

	if idx := int(p); idx >= 0 && idx < len(g.visibleVersionByPoint) {
		if m := g.visibleVersionByPoint[idx]; m != nil {
			return m
		}
	}

	if g.visibleVersion == nil {
		return nil
	}

	return g.visibleVersion[p]
}

// HasPhiAt returns true if there's a phi node at point p for the given symbol.
func (g *Graph) HasPhiAt(_ Point, sym basecfg.SymbolID) bool {
	if g == nil {
		return false
	}

	for _, phi := range g.phiNodes {
		if phi.Target.Symbol == sym {
			return true
		}
	}

	return false
}

// SymbolResolver resolves variable names to SymbolIDs at CFG points.
type SymbolResolver func(p Point, name string) basecfg.SymbolID

// PopulateSymbols fills in basecfg.SymbolID fields for all node infos using the resolver.
func (g *Graph) PopulateSymbols(resolve SymbolResolver) {
	if g == nil || len(g.infoByPoint) == 0 || resolve == nil {
		return
	}

	points := g.orderedPoints
	if points == nil {
		points = g.sortedPoints(nil)
	}

	for _, p := range points {
		info := g.Info(p)
		if info == nil {
			continue
		}
		switch v := info.(type) {
		case *AssignInfo:
			for i := range v.Targets {
				target := &v.Targets[i]
				switch target.Kind {
				case TargetIdent:
					if target.Name != "" {
						target.Symbol = resolve(p, target.Name)
					}
				case TargetField:
					if target.BaseName != "" {
						target.BaseSymbol = resolve(p, target.BaseName)
					}
				}
			}
		case *BranchInfo:
			if v.CondVar != "" {
				root := extraction.ExtractRootName(v.CondVar)
				if root != "" {
					v.CondSymbol = resolve(p, root)
				}
			}
		case *CallInfo:
			if v.CalleeName != "" {
				v.CalleeSymbol = resolve(p, v.CalleeName)
			}

			if v.ReceiverName != "" {
				v.ReceiverSymbol = resolve(p, v.ReceiverName)
			}

			if len(v.ArgNames) > 0 {
				v.ArgSymbols = make([]basecfg.SymbolID, len(v.ArgNames))
				for i, name := range v.ArgNames {
					if name != "" {
						v.ArgSymbols[i] = resolve(p, name)
					}
				}
			}
		case *ReturnInfo:
			if len(v.Names) > 0 {
				v.Symbols = make([]basecfg.SymbolID, len(v.Names))
				for i, name := range v.Names {
					if name != "" {
						v.Symbols[i] = resolve(p, name)
					}
				}
			}
		case *FuncDefInfo:
			if v.Name != "" && v.TargetKind == FuncDefGlobal {
				v.Symbol = resolve(p, v.Name)
			}
		}
	}
}

// SymbolAt returns the basecfg.SymbolID for a variable name at a specific CFG point.
// This reflects lexical scoping - the same name may resolve to different
// symbols at different points due to shadowing.
// Returns (0, false) if the name is not visible at that point.
// Implements cfg.SSAVersioned.
func (g *Graph) SymbolAt(p Point, name string) (basecfg.SymbolID, bool) {
	if g == nil {
		return 0, false
	}

	if vis := g.localSymbolsAt(p); vis != nil {
		if sym, ok := vis[name]; ok {
			return sym, true
		}
	}

	if g.globals != nil {
		if sym, ok := g.globals[name]; ok {
			return sym, true
		}
	}

	return 0, false
}

// LocalSymbolsAt returns the point-local symbol visibility map.
// The returned map should not be modified.
func (g *Graph) LocalSymbolsAt(p Point) map[string]basecfg.SymbolID {
	return g.localSymbolsAt(p)
}

// AllSymbolsAt returns all visible symbols at a CFG point.
func (g *Graph) AllSymbolsAt(p Point) map[string]basecfg.SymbolID {
	if g == nil {
		return nil
	}

	local := g.localSymbolsAt(p)

	if len(g.globals) == 0 {
		return local
	}

	if local == nil {
		return g.globals
	}
	// Check cache
	if g.mergedSymbols != nil {
		if cached, ok := g.mergedSymbols[p]; ok {
			return cached
		}
	}
	// Merge local and globals (local shadows global)
	merged := make(map[string]basecfg.SymbolID, len(local)+len(g.globals))
	for name, sym := range g.globals {
		merged[name] = sym
	}

	for name, sym := range local {
		merged[name] = sym
	}

	// Cache result
	if g.mergedSymbols == nil {
		g.mergedSymbols = make(map[Point]map[string]basecfg.SymbolID)
	}

	g.mergedSymbols[p] = merged

	return merged
}

func (g *Graph) localSymbolsAt(p Point) map[string]basecfg.SymbolID {
	if g == nil {
		return nil
	}
	if idx := int(p); idx >= 0 && idx < len(g.symbolScopeByPoint) {
		if vis := g.symbolScopeByPoint[idx]; vis != nil {
			return vis
		}
	}
	if g.symbolScope == nil {
		return nil
	}
	return g.symbolScope[p]
}

// DeclarationPoint returns the CFG point where a symbol was declared.
// Returns (0, false) if the symbol is unknown.
// Implements cfg.SSAVersioned.
func (g *Graph) DeclarationPoint(sym basecfg.SymbolID) (Point, bool) {
	if g == nil || g.declPoints == nil {
		return 0, false
	}

	p, ok := g.declPoints[sym]

	return p, ok
}

// NameOf returns the variable name for a symbol (for display purposes).
// Returns empty string if the symbol is unknown.
func (g *Graph) NameOf(sym basecfg.SymbolID) string {
	if g == nil || g.symbolNames == nil {
		if g == nil || g.bindings == nil {
			return ""
		}

		return g.bindings.Name(sym)
	}

	if name := g.symbolNames[sym]; name != "" {
		return name
	}

	if g.bindings != nil {
		return g.bindings.Name(sym)
	}

	return ""
}

// GlobalSymbol returns the symbol ID for a global name.
// Returns (0, false) if the name is not a known global.
func (g *Graph) GlobalSymbol(name string) (basecfg.SymbolID, bool) {
	if g == nil || g.globals == nil {
		return 0, false
	}

	sym, ok := g.globals[name]

	return sym, ok
}

// AllSymbolIDs returns a set of all symbols known to this graph.
// Useful when only symbol identity is needed (avoids AllSymbolsAt merges).
func (g *Graph) AllSymbolIDs() map[basecfg.SymbolID]bool {
	if g == nil {
		return nil
	}

	out := make(map[basecfg.SymbolID]bool, len(g.symbolNames))

	for sym := range g.symbolNames {
		out[sym] = true
	}

	if g.globals != nil {
		for _, sym := range g.globals {
			out[sym] = true
		}
	}

	return out
}

// DirectAliasSymbol returns the source symbol for a direct local alias assignment.
// Handles patterns like `local f = B` in the current graph.
// Returns 0 if the alias is ambiguous or not a direct ident assignment.
func (g *Graph) DirectAliasSymbol(targetSym basecfg.SymbolID) basecfg.SymbolID {
	if g == nil || targetSym == 0 {
		return 0
	}
	if g.directAliases == nil {
		return 0
	}
	return g.directAliases[targetSym]
}

// EachAliasSymbol visits targetSym followed by its direct-alias chain.
// Iteration stops on zero, self-loop, cycle, or when fn returns true.
func (g *Graph) EachAliasSymbol(targetSym basecfg.SymbolID, fn func(basecfg.SymbolID) bool) {
	if targetSym == 0 || fn == nil {
		return
	}

	seen := make(map[basecfg.SymbolID]struct{}, 4)
	current := targetSym
	for current != 0 {
		if _, ok := seen[current]; ok {
			return
		}
		seen[current] = struct{}{}

		if fn(current) {
			return
		}

		next := g.DirectAliasSymbol(current)
		if next == 0 || next == current {
			return
		}
		current = next
	}
}

type aliasState struct {
	sourceSym basecfg.SymbolID
	hasLocal  bool
	ambiguous bool
}

func computeDirectAliasIndex(info map[basecfg.Point]NodeInfo, bindings *bind.BindingTable) map[basecfg.SymbolID]basecfg.SymbolID {
	if len(info) == 0 || bindings == nil {
		return nil
	}

	stateByTarget := make(map[basecfg.SymbolID]aliasState)

	for _, nodeInfo := range info {
		assign, ok := nodeInfo.(*AssignInfo)
		if !ok || assign == nil {
			continue
		}

		assign.EachTargetSource(func(_ int, target AssignTarget, src ast.Expr) {
			if target.Symbol == 0 {
				return
			}

			state := stateByTarget[target.Symbol]
			if state.ambiguous {
				return
			}
			if assign.IsLocal {
				state.hasLocal = true
			}

			srcIdent, ok := src.(*ast.IdentExpr)
			if !ok || srcIdent == nil {
				state.ambiguous = true
				state.sourceSym = 0
				stateByTarget[target.Symbol] = state
				return
			}

			sym, ok := bindings.SymbolOf(srcIdent)
			if !ok || sym == 0 {
				state.ambiguous = true
				state.sourceSym = 0
				stateByTarget[target.Symbol] = state
				return
			}

			if state.sourceSym == 0 {
				state.sourceSym = sym
			} else if state.sourceSym != sym {
				state.ambiguous = true
				state.sourceSym = 0
			}

			stateByTarget[target.Symbol] = state
		})
	}

	out := make(map[basecfg.SymbolID]basecfg.SymbolID)
	for targetSym, state := range stateByTarget {
		if state.ambiguous || !state.hasLocal || state.sourceSym == 0 {
			continue
		}
		out[targetSym] = state.sourceSym
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SymbolKind returns the kind of a symbol (Param, Local, or Global).
// Returns (SymbolUnknown, false) if the symbol is not known.
func (g *Graph) SymbolKind(sym basecfg.SymbolID) (basecfg.SymbolKind, bool) {
	if g == nil || g.symbolKinds == nil {
		return basecfg.SymbolUnknown, false
	}

	kind, ok := g.symbolKinds[sym]

	return kind, ok
}

// HasScopeTracking returns true if scope visibility was computed during build.
func (g *Graph) HasScopeTracking() bool {
	return g != nil && g.symbolScope != nil
}

// Bindings returns the binding table populated during binder pass.
func (g *Graph) Bindings() *bind.BindingTable {
	if g == nil {
		return nil
	}

	return g.bindings
}

// ParamNames returns a copy of the function parameter names.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamNames() []string {
	if g == nil || len(g.paramNames) == 0 {
		return nil
	}

	result := make([]string, len(g.paramNames))
	copy(result, g.paramNames)

	return result
}

// ParamSymbols returns a copy of the function parameter symbol IDs.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamSymbols() []basecfg.SymbolID {
	if g == nil || len(g.paramSymbols) == 0 {
		return nil
	}

	result := make([]basecfg.SymbolID, len(g.paramSymbols))
	copy(result, g.paramSymbols)

	return result
}

// ParamDeclPoints returns a copy of the CFG points where parameters are declared.
// Returns nil for block graphs or functions with no parameters.
func (g *Graph) ParamDeclPoints() []Point {
	if g == nil || len(g.paramDeclPoints) == 0 {
		return nil
	}

	result := make([]Point, len(g.paramDeclPoints))
	copy(result, g.paramDeclPoints)

	return result
}

// CalleePathAt returns the callee path for a call at point p.
// Returns empty path if p is not a call node.
func (g *Graph) CalleePathAt(p Point) constraint.Path {
	if g == nil {
		return constraint.Path{}
	}

	if info := g.Call(p); info != nil {
		return info.CalleePath
	}

	return constraint.Path{}
}

// FuncDefPathAt returns the target path for a function definition at point p.
// Returns empty path if p is not a function definition node.
func (g *Graph) FuncDefPathAt(p Point) constraint.Path {
	if g == nil {
		return constraint.Path{}
	}

	if info := g.FuncDef(p); info != nil {
		return info.TargetPath
	}
	return constraint.Path{}
}
