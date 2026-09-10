// Package cfg provides control flow graph representation for dataflow analysis.
//
// A control flow graph (CFG) models the flow of control through a function body.
// Each node represents a program point (assignment, branch, call, etc.) and edges
// represent possible control flow between points.
//
// # Core Types
//
// Point: An index identifying a location in the CFG. Points are local to each
// function's CFG and serve as keys for type state maps.
//
// Node: Contains metadata about a program point including its kind, target variable
// (for assignments), callee (for calls), and condition info (for branches).
//
// Edge: A directed edge from one point to another, with a condition flag indicating
// whether it's the then-branch (true) or else-branch (false) of a conditional.
//
// Graph: Interface for CFG access, implemented by CFG. Provides node lookup,
// edge traversal, predecessor/successor queries, and reverse post-order iteration.
//
// # SSA-Like Identity
//
// The cfg package provides SymbolID for SSA-style value identity. Unlike SSA which
// creates new versions at each assignment, SymbolID identifies the lexical variable
// binding. Combined with Point, this enables tracking which value a name refers to
// at each program point.
//
// # Analysis Support
//
// The CFG supports forward dataflow analysis via RPO (reverse post-order) traversal.
// Branch conditions are exposed for constraint extraction, and join points are
// identified for type merging.
package cfg

import "sync/atomic"

// Point represents a location in a control flow graph.
// Each Point is an index into the CFG's node array, local to that CFG.
type Point uint32

// NodeKind identifies the type of CFG node.
type NodeKind uint8

// Node kind constants identify the type of each CFG node.
const (
	NodeEntry      NodeKind = iota // Function entry point, always first node
	NodeExit                       // Function exit point, always last node
	NodeAssign                     // Assignment statement (local x = expr)
	NodeCall                       // Function call expression
	NodeBranch                     // Conditional branch (if, while, for condition)
	NodeJoin                       // Join point where multiple paths merge
	NodeReturn                     // Return statement
	NodeScopeEnter                 // Lexical scope entry (function, block, loop body)
	NodeScopeExit                  // Lexical scope exit (end of block)
	NodeTypeDef                    // Type definition (type annotation)
)

// CondCheckKind identifies the type of condition check in a branch.
//
// The type checker uses CondCheckKind to extract constraints from branch
// conditions without re-analyzing the expression. This enables efficient
// type narrowing in then/else branches.
type CondCheckKind uint8

// Condition check kind constants represent recognizable branch patterns.
const (
	CheckNone      CondCheckKind = iota // Complex expression, no simple constraint
	CheckTruthy                         // if x: narrows x to truthy values
	CheckFalsy                          // if not x: narrows x to falsy values
	CheckNil                            // x == nil: narrows x to nil
	CheckNotNil                         // x ~= nil: narrows x to non-nil
	CheckLimit                          // Numeric for loop limit (i <= n)
	CheckTypeEqual                      // type(x) == "typename": narrows to that type
	CheckTypeNot                        // type(x) ~= "typename": excludes that type
)

// CondCheck represents a condition check in a branch node.
type CondCheck struct {
	Kind     CondCheckKind
	TypeName string // Only for CheckTypeEqual/CheckTypeNot
}

// Node represents a CFG node with metadata about the program point.
//
// Different node kinds use different fields:
//   - NodeAssign: Target holds the assigned variable's SymbolID
//   - NodeCall: Callee holds the function name for global/external calls
//   - NodeBranch: CondVar and CondCheck describe the condition for narrowing
//   - NodeJoin: LoopVars, LoopLocals, LoopPreheader describe loop structure
//
// The Point field is the node's index in the CFG's Nodes slice.
type Node struct {
	Point      Point
	Kind       NodeKind
	Target     SymbolID  // Variable for assignments (0 = none or unresolved)
	Callee     string    // Function for calls (global/external name)
	CondVar    SymbolID  // Variable being tested (0 = none or complex expression)
	CondCheck  CondCheck // Condition check type and optional type name
	LoopVars   []SymbolID
	LoopLocals []SymbolID
	// LoopPreheader is the unique predecessor that enters a loop from outside
	// (i.e., not a back-edge). For loops with multiple entry points or complex
	// goto patterns, this points to the primary loop entry.
	LoopPreheader    Point
	LoopPreheaderSet bool
}

// Edge represents a directed control flow edge between two points.
//
// For branch nodes (NodeBranch), the Cond field distinguishes branches:
//   - Cond=true: the "then" branch, taken when condition is truthy
//   - Cond=false: the "else" branch, taken when condition is falsy
//
// For non-branch nodes, Cond is typically true (single successor).
type Edge struct {
	From Point
	To   Point
	Cond bool // true = then branch, false = else branch
}

// Graph is the interface for control flow graphs.
//
// This interface abstracts over different CFG implementations, allowing the
// type checker to work with various CFG builders.
//
// Key methods for dataflow analysis:
//   - RPO(): Returns nodes in reverse post-order for forward analysis
//   - Predecessors/Successors: Enable backward/forward traversal
//   - EdgeCond: Retrieves branch condition for type narrowing
//   - IsJoin/IsBranch: Identifies merge points and decision points
type Graph interface {
	ID() uint64                           // Unique identifier for this CFG instance
	Entry() Point                         // Function entry point (always node 0)
	Exit() Point                          // Function exit point
	Node(p Point) *Node                   // Node metadata at point p
	RPO() []Point                         // Reverse post-order for forward analysis
	Predecessors(p Point) []Point         // Incoming edges
	Successor(p Point) Point              // Single successor (non-branch nodes)
	Successors(p Point) []Point           // All successors (branch nodes have 2)
	Edges() []Edge                        // All edges in the graph
	Size() int                            // Number of nodes
	EdgeCond(from, to Point) (bool, bool) // Branch condition for an edge
	IsJoin(p Point) bool                  // True if p has multiple predecessors
	IsBranch(p Point) bool                // True if p has multiple successors
}

// CFG represents the control flow graph for a function.
//
// A CFG is built during AST analysis and consumed by the type checker for
// flow-sensitive analysis. The graph is immutable after construction.
//
// Structure:
//   - Nodes: flat array indexed by Point
//   - Edges: directed edges with predecessor/successor maps for efficient lookup
//   - Entry/Exit: special nodes for function boundaries
//
// The CFG is built incrementally via AddNode/AddEdge, then traversed via
// RPO for forward dataflow analysis.
type CFG struct {
	id    uint64
	entry Point
	exit  Point
	Nodes []Node
	edges []Edge
	preds [][]Point
	succs [][]Point
	rpo   []Point
}

var cfgCounter uint64

func nextCFGID() uint64 {
	return atomic.AddUint64(&cfgCounter, 1)
}

// New creates an empty CFG.
func New() *CFG {
	return NewWithCapacity(0, 0)
}

// NewWithCapacity creates an empty CFG with initial node/edge capacity hints.
func NewWithCapacity(nodeCap, edgeCap int) *CFG {
	if nodeCap < 2 {
		nodeCap = 2
	}
	if edgeCap < 0 {
		edgeCap = 0
	}

	c := &CFG{
		id:    nextCFGID(),
		Nodes: make([]Node, 0, nodeCap),
		edges: make([]Edge, 0, edgeCap),
		preds: make([][]Point, 0, nodeCap),
		succs: make([][]Point, 0, nodeCap),
	}
	c.entry = c.AddNode(NodeEntry, 0, "")
	c.exit = c.AddNode(NodeExit, 0, "")
	return c
}

// Reserve increases node/edge capacities to at least the provided values.
func (c *CFG) Reserve(nodeCap, edgeCap int) {
	if c == nil {
		return
	}

	if nodeCap > cap(c.Nodes) {
		nodes := make([]Node, len(c.Nodes), nodeCap)
		copy(nodes, c.Nodes)
		c.Nodes = nodes
	}
	if nodeCap > cap(c.preds) {
		preds := make([][]Point, len(c.preds), nodeCap)
		copy(preds, c.preds)
		c.preds = preds
	}
	if nodeCap > cap(c.succs) {
		succs := make([][]Point, len(c.succs), nodeCap)
		copy(succs, c.succs)
		c.succs = succs
	}

	if edgeCap > cap(c.edges) {
		edges := make([]Edge, len(c.edges), edgeCap)
		copy(edges, c.edges)
		c.edges = edges
	}
}

// Entry returns the entry point.
func (c *CFG) Entry() Point {
	if c == nil {
		return 0
	}
	return c.entry
}

// Exit returns the exit point.
func (c *CFG) Exit() Point {
	if c == nil {
		return 0
	}
	return c.exit
}

// Successor returns single successor (for non-branch nodes).
func (c *CFG) Successor(p Point) Point {
	idx := int(p)
	if idx >= 0 && idx < len(c.succs) {
		if succs := c.succs[idx]; len(succs) > 0 {
			return succs[0]
		}
	}
	return p
}

// ID returns the stable identifier for this CFG instance.
func (c *CFG) ID() uint64 {
	if c == nil {
		return 0
	}
	return c.id
}

// AddNode adds a node and returns its point.
func (c *CFG) AddNode(kind NodeKind, target SymbolID, callee string) Point {
	c.invalidateRPO()
	p := Point(len(c.Nodes))
	c.Nodes = append(c.Nodes, Node{Point: p, Kind: kind, Target: target, Callee: callee})
	c.ensureAdjacencyLen(len(c.Nodes))
	return p
}

// AddEdge adds an edge.
func (c *CFG) AddEdge(from, to Point, cond bool) {
	fromIdx := int(from)
	toIdx := int(to)
	if fromIdx < 0 || toIdx < 0 {
		return
	}
	maxIdx := fromIdx
	if toIdx > maxIdx {
		maxIdx = toIdx
	}
	c.ensureAdjacencyLen(maxIdx + 1)

	c.invalidateRPO()
	c.edges = append(c.edges, Edge{From: from, To: to, Cond: cond})
	succs := c.succs[fromIdx]
	if succs == nil {
		succs = make([]Point, 0, 2)
	}
	succs = append(succs, to)
	c.succs[fromIdx] = succs

	preds := c.preds[toIdx]
	if preds == nil {
		preds = make([]Point, 0, 2)
	}
	preds = append(preds, from)
	c.preds[toIdx] = preds
}

// RemoveOutgoing removes all outgoing edges from a node.
func (c *CFG) RemoveOutgoing(from Point) {
	if c == nil {
		return
	}
	fromIdx := int(from)
	if fromIdx < 0 || fromIdx >= len(c.succs) {
		return
	}
	succs := c.succs[fromIdx]
	if len(succs) == 0 {
		return
	}
	c.invalidateRPO()
	filtered := c.edges[:0]
	for _, e := range c.edges {
		if e.From == from {
			continue
		}
		filtered = append(filtered, e)
	}
	c.edges = filtered
	c.succs[fromIdx] = nil
	for _, to := range succs {
		toIdx := int(to)
		if toIdx < 0 || toIdx >= len(c.preds) {
			continue
		}
		preds := c.preds[toIdx]
		if len(preds) == 0 {
			continue
		}
		next := preds[:0]
		for _, p := range preds {
			if p != from {
				next = append(next, p)
			}
		}
		if len(next) == 0 {
			c.preds[toIdx] = nil
			continue
		}
		c.preds[toIdx] = next
	}
}

// Node returns the node at point p.
func (c *CFG) Node(p Point) *Node {
	if int(p) < len(c.Nodes) {
		return &c.Nodes[p]
	}
	return nil
}

// Predecessors returns all predecessors of p.
func (c *CFG) Predecessors(p Point) []Point {
	idx := int(p)
	if idx < 0 || idx >= len(c.preds) {
		return nil
	}
	preds := c.preds[idx]
	if len(preds) == 0 {
		return nil
	}
	result := make([]Point, len(preds))
	copy(result, preds)
	return result
}

// PredecessorsReadOnly returns the internal predecessor slice for p.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) PredecessorsReadOnly(p Point) []Point {
	idx := int(p)
	if idx < 0 || idx >= len(c.preds) {
		return nil
	}
	return c.preds[idx]
}

// Predecessor returns single predecessor (for non-join nodes).
func (c *CFG) Predecessor(p Point) Point {
	idx := int(p)
	if idx >= 0 && idx < len(c.preds) {
		if preds := c.preds[idx]; len(preds) > 0 {
			return preds[0]
		}
	}
	return p
}

// Successors returns all successors of p.
func (c *CFG) Successors(p Point) []Point {
	idx := int(p)
	if idx < 0 || idx >= len(c.succs) {
		return nil
	}
	succs := c.succs[idx]
	if len(succs) == 0 {
		return nil
	}
	result := make([]Point, len(succs))
	copy(result, succs)
	return result
}

// SuccessorsReadOnly returns the internal successor slice for p.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) SuccessorsReadOnly(p Point) []Point {
	idx := int(p)
	if idx < 0 || idx >= len(c.succs) {
		return nil
	}
	return c.succs[idx]
}

// IsJoin returns true if p has multiple predecessors.
func (c *CFG) IsJoin(p Point) bool {
	idx := int(p)
	return idx >= 0 && idx < len(c.preds) && len(c.preds[idx]) > 1
}

// IsBranch returns true if p has multiple successors.
func (c *CFG) IsBranch(p Point) bool {
	idx := int(p)
	return idx >= 0 && idx < len(c.succs) && len(c.succs[idx]) > 1
}

// Edges returns all edges.
func (c *CFG) Edges() []Edge {
	if len(c.edges) == 0 {
		return nil
	}
	result := make([]Edge, len(c.edges))
	copy(result, c.edges)
	return result
}

// Size returns node count.
func (c *CFG) Size() int {
	return len(c.Nodes)
}

// EdgeCond returns the condition value for edge from->to.
// Returns (true, ok) for then-branch, (false, ok) for else-branch.
func (c *CFG) EdgeCond(from, to Point) (bool, bool) {
	for _, e := range c.edges {
		if e.From == from && e.To == to {
			return e.Cond, true
		}
	}
	return false, false
}

// AddBranch adds a branch node with condition info.
func (c *CFG) AddBranch(condVar SymbolID, condCheck CondCheck) Point {
	c.invalidateRPO()
	p := Point(len(c.Nodes))
	c.Nodes = append(c.Nodes, Node{
		Point:     p,
		Kind:      NodeBranch,
		CondVar:   condVar,
		CondCheck: condCheck,
	})
	return p
}

// RPO returns nodes in reverse post-order for forward dataflow analysis.
//
// Reverse post-order (RPO) is the optimal traversal order for forward dataflow
// analysis: predecessors are visited before successors (except for back-edges
// in loops). This ensures that type state propagates correctly through the
// control flow graph.
//
// Only nodes reachable from the entry point are included. Unreachable code
// (after unconditional return, for example) is excluded.
func (c *CFG) RPO() []Point {
	ro := c.RPOReadOnly()
	if len(ro) == 0 {
		return nil
	}
	out := make([]Point, len(ro))
	copy(out, ro)
	return out
}

// RPOReadOnly returns cached reverse post-order without making a copy.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) RPOReadOnly() []Point {
	if c == nil {
		return nil
	}
	if len(c.rpo) > 0 {
		return c.rpo
	}

	n := len(c.Nodes)
	visited := make([]bool, n)
	order := make([]Point, 0, n)

	var visit func(p Point)
	visit = func(p Point) {
		if int(p) >= n || visited[p] {
			return
		}
		visited[p] = true
		for _, succ := range c.succs[p] {
			visit(succ)
		}
		order = append(order, p)
	}

	visit(c.entry)

	// Reverse to get RPO
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	c.rpo = make([]Point, len(order))
	copy(c.rpo, order)
	return c.rpo
}

func (c *CFG) invalidateRPO() {
	if c == nil {
		return
	}
	c.rpo = nil
}

func (c *CFG) ensureAdjacencyLen(n int) {
	if c == nil || n <= len(c.preds) {
		return
	}
	grow := n - len(c.preds)
	c.preds = append(c.preds, make([][]Point, grow)...)
	c.succs = append(c.succs, make([][]Point, grow)...)
}

// Reachable returns a set of all nodes reachable from entry.
func (c *CFG) Reachable() map[Point]bool {
	if c == nil {
		return nil
	}
	n := len(c.Nodes)
	visited := make([]bool, n)
	result := make(map[Point]bool)

	var visit func(p Point)
	visit = func(p Point) {
		if int(p) >= n || visited[p] {
			return
		}
		visited[p] = true
		result[p] = true
		for _, succ := range c.succs[int(p)] {
			visit(succ)
		}
	}
	visit(c.entry)
	return result
}

// UnreachablePoints returns all nodes not reachable from entry.
func (c *CFG) UnreachablePoints() []Point {
	if c == nil {
		return nil
	}
	reachable := c.Reachable()
	var unreachable []Point
	for i := range c.Nodes {
		p := Point(i)
		if !reachable[p] {
			unreachable = append(unreachable, p)
		}
	}
	return unreachable
}

// ValidateEdges checks that all edges reference valid node indices.
// Returns nil if valid, otherwise returns an error describing the issue.
func (c *CFG) ValidateEdges() error {
	if c == nil {
		return nil
	}
	n := len(c.Nodes)
	for _, e := range c.edges {
		if int(e.From) >= n {
			return &EdgeError{From: e.From, To: e.To, Reason: "from point out of bounds"}
		}
		if int(e.To) >= n {
			return &EdgeError{From: e.From, To: e.To, Reason: "to point out of bounds"}
		}
	}
	return nil
}

// EdgeError describes an invalid edge.
type EdgeError struct {
	From   Point
	To     Point
	Reason string
}

func (e *EdgeError) Error() string {
	return "invalid edge: " + e.Reason
}
