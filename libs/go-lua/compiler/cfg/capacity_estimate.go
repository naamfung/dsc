package cfg

import "github.com/wippyai/go-lua/compiler/ast"

const (
	minNodeCapacity = 4
	minEdgeCapacity = 6
)

func estimateFunctionCFGCapacity(fn *ast.FunctionExpr) (nodes int, edges int) {
	if fn != nil && fn.ParList != nil {
		// Include one extra slot to absorb implicit-self parameter bindings.
		nodes += len(fn.ParList.Names) + 1
		edges += len(fn.ParList.Names) + 1
	}

	bodyNodes, bodyEdges := estimateStmtListCFGCapacity(fn.Stmts)
	nodes += bodyNodes
	edges += bodyEdges

	// Entry/exit + a small cushion for implicit return plumbing.
	nodes += 3
	edges += 5

	if nodes < minNodeCapacity {
		nodes = minNodeCapacity
	}
	if edges < minEdgeCapacity {
		edges = minEdgeCapacity
	}

	return nodes, edges
}

func estimateBlockCFGCapacity(stmts []ast.Stmt) (nodes int, edges int) {
	bodyNodes, bodyEdges := estimateStmtListCFGCapacity(stmts)
	nodes = bodyNodes + 3
	edges = bodyEdges + 3

	if nodes < minNodeCapacity {
		nodes = minNodeCapacity
	}
	if edges < minEdgeCapacity {
		edges = minEdgeCapacity
	}

	return nodes, edges
}

func estimateStmtListCFGCapacity(stmts []ast.Stmt) (nodes int, edges int) {
	for _, stmt := range stmts {
		n, e := estimateStmtCFGCapacity(stmt)
		nodes += n
		edges += e
	}

	return nodes, edges
}

func estimateStmtCFGCapacity(stmt ast.Stmt) (nodes int, edges int) {
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt, *ast.AssignStmt, *ast.FuncCallStmt, *ast.FuncDefStmt, *ast.TypeDefStmt:
		return 1, 1
	case *ast.ReturnStmt:
		return 1, 2
	case *ast.BreakStmt, *ast.LabelStmt, *ast.GotoStmt:
		return 1, 2
	case *ast.DoBlockStmt:
		bodyNodes, bodyEdges := estimateStmtListCFGCapacity(s.Stmts)
		return bodyNodes + 2, bodyEdges + 3
	case *ast.IfStmt:
		thenNodes, thenEdges := estimateStmtListCFGCapacity(s.Then)
		elseNodes, elseEdges := estimateStmtListCFGCapacity(s.Else)
		return thenNodes + elseNodes + 6, thenEdges + elseEdges + 8
	case *ast.WhileStmt:
		bodyNodes, bodyEdges := estimateStmtListCFGCapacity(s.Stmts)
		return bodyNodes + 5, bodyEdges + 7
	case *ast.RepeatStmt:
		bodyNodes, bodyEdges := estimateStmtListCFGCapacity(s.Stmts)
		return bodyNodes + 5, bodyEdges + 8
	case *ast.NumberForStmt:
		bodyNodes, bodyEdges := estimateStmtListCFGCapacity(s.Stmts)
		return bodyNodes + 7, bodyEdges + 10
	case *ast.GenericForStmt:
		bodyNodes, bodyEdges := estimateStmtListCFGCapacity(s.Stmts)
		return bodyNodes + 7, bodyEdges + 10
	default:
		return 1, 1
	}
}
