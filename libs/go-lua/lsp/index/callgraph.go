package index

import (
	"sync"

	"github.com/wippyai/go-lua/types/diag"
)

// CallEdge represents a call from one function to another.
type CallEdge struct {
	CallerFile string
	CallerName string
	CallerSpan diag.Span // Definition span of caller

	CalleeFile string // File where callee is defined
	CalleeName string
	CalleeSpan diag.Span // Definition span of callee

	CallSpan diag.Span // Location of the call expression
}

// CallGraph tracks function call relationships for LSP features.
// It enables "find all callers" and "find all callees" queries.
type CallGraph struct {
	mu sync.RWMutex

	// edges stores all call edges keyed by caller file for invalidation
	edges map[string][]*CallEdge

	// byCallee indexes edges by callee name for caller lookup
	byCallee map[string][]*CallEdge

	// byLine indexes edges by file:line for position-based lookup
	byLine map[filePos][]*CallEdge
}

// NewCallGraph creates an empty call graph.
func NewCallGraph() *CallGraph {
	return &CallGraph{
		edges:    make(map[string][]*CallEdge),
		byCallee: make(map[string][]*CallEdge),
		byLine:   make(map[filePos][]*CallEdge),
	}
}

// AddCall records a call edge from caller to callee.
func (cg *CallGraph) AddCall(callerFile, callerName string, callerSpan diag.Span,
	calleeFile, calleeName string, calleeSpan, callSpan diag.Span) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	edge := &CallEdge{
		CallerFile: callerFile,
		CallerName: callerName,
		CallerSpan: callerSpan,
		CalleeFile: calleeFile,
		CalleeName: calleeName,
		CalleeSpan: calleeSpan,
		CallSpan:   callSpan,
	}

	// Index by caller file
	cg.edges[callerFile] = append(cg.edges[callerFile], edge)

	// Index by callee name
	cg.byCallee[calleeName] = append(cg.byCallee[calleeName], edge)

	// Index by call position
	pos := filePos{file: callerFile, line: callSpan.StartLine}
	cg.byLine[pos] = append(cg.byLine[pos], edge)
}

// CallersOf returns all functions that call the given function defined in file.
func (cg *CallGraph) CallersOf(file, funcName string) []*CallEdge {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	edges := cg.byCallee[funcName]
	var result []*CallEdge
	for _, edge := range edges {
		if edge.CalleeFile == file {
			result = append(result, edge)
		}
	}
	return result
}

// CalleesOf returns all functions called by the given function.
func (cg *CallGraph) CalleesOf(file, funcName string) []*CallEdge {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var result []*CallEdge
	for _, edge := range cg.edges[file] {
		if edge.CallerName == funcName {
			result = append(result, edge)
		}
	}
	return result
}

// CallAt returns the call edge at the given position, if any.
func (cg *CallGraph) CallAt(file string, line, col int) *CallEdge {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	pos := filePos{file: file, line: line}
	for _, edge := range cg.byLine[pos] {
		if spanContains(edge.CallSpan, line, col) {
			return edge
		}
	}
	return nil
}

// InvalidateFile removes all call edges originating from a file.
func (cg *CallGraph) InvalidateFile(file string) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Get edges to remove
	edgesToRemove := cg.edges[file]
	if len(edgesToRemove) == 0 {
		return
	}

	// Remove from byCallee index
	for _, edge := range edgesToRemove {
		cg.removeFromCalleeIndex(edge)
	}

	// Remove from byLine index
	for pos := range cg.byLine {
		if pos.file == file {
			delete(cg.byLine, pos)
		}
	}

	// Remove from main edges map
	delete(cg.edges, file)
}

// removeFromCalleeIndex removes an edge from the callee index.
func (cg *CallGraph) removeFromCalleeIndex(edge *CallEdge) {
	edges := cg.byCallee[edge.CalleeName]
	for i, e := range edges {
		if e == edge {
			cg.byCallee[edge.CalleeName] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

// Clear removes all call edges.
func (cg *CallGraph) Clear() {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	cg.edges = make(map[string][]*CallEdge)
	cg.byCallee = make(map[string][]*CallEdge)
	cg.byLine = make(map[filePos][]*CallEdge)
}

// AllEdges returns all call edges (for debugging/analysis).
func (cg *CallGraph) AllEdges() []*CallEdge {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var result []*CallEdge
	for _, edges := range cg.edges {
		result = append(result, edges...)
	}
	return result
}

// CallCount returns the number of calls from a function.
func (cg *CallGraph) CallCount(file, funcName string) int {
	return len(cg.CalleesOf(file, funcName))
}

// CallerCount returns the number of callers to a function.
func (cg *CallGraph) CallerCount(funcName string) int {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	return len(cg.byCallee[funcName])
}
