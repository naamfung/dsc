package ast

import "fmt"

// Position represents a location in source code.
type Position struct {
	Source    string // Source file name
	Line      int    // Start line number (1-based)
	Column    int    // Start column number (1-based)
	EndLine   int    // End line number (0 means same as Line)
	EndColumn int    // End column number (0 means same as Column)
}

// Token represents a lexical token from the scanner.
type Token struct {
	Type int      // Token type identifier
	Name string   // Human-readable token name
	Str  string   // Token string value
	Pos  Position // Source position
}

// String returns a debug representation of the token.
func (t *Token) String() string {
	return fmt.Sprintf("<type:%v, str:%v>", t.Name, t.Str)
}

// Stmt is the interface implemented by all statement nodes.
type Stmt interface {
	PositionHolder
	stmtMarker()
}

// StmtBase is the base struct embedded in all statement nodes.
type StmtBase struct {
	Node
}

func (stmt *StmtBase) stmtMarker() {}

// AssignStmt represents an assignment statement (a, b = x, y).
type AssignStmt struct {
	StmtBase
	Lhs []Expr // Left-hand side expressions
	Rhs []Expr // Right-hand side expressions
}

// LocalAssignStmt represents a local variable declaration (local a, b = x, y).
type LocalAssignStmt struct {
	StmtBase
	Names         []string   // Variable names
	NamePositions []Position // Per-name token positions, parallel to Names
	Types         []TypeExpr // Type annotations, parallel to Names (nil entries = inferred)
	Exprs         []Expr     // Initializer expressions
}

// FuncCallStmt represents a function call used as a statement.
type FuncCallStmt struct {
	StmtBase
	Expr Expr // The function call expression
}

// DoBlockStmt represents a do...end block.
type DoBlockStmt struct {
	StmtBase
	Stmts []Stmt // Statements within the block
}

// WhileStmt represents a while loop (while cond do ... end).
type WhileStmt struct {
	StmtBase
	Condition Expr   // Loop condition
	Stmts     []Stmt // Loop body
}

// RepeatStmt represents a repeat-until loop (repeat ... until cond).
type RepeatStmt struct {
	StmtBase
	Condition Expr   // Loop termination condition
	Stmts     []Stmt // Loop body
}

// IfStmt represents an if statement with optional else branch.
type IfStmt struct {
	StmtBase
	Condition Expr   // Branch condition
	Then      []Stmt // Statements when condition is true
	Else      []Stmt // Statements when condition is false (may contain nested IfStmt for elseif)
}

// NumberForStmt represents a numeric for loop (for i = start, limit, step do ... end).
type NumberForStmt struct {
	StmtBase
	Name         string   // Loop variable name
	NamePosition Position // Token position for the loop variable
	Init         Expr     // Initial value
	Limit        Expr     // Upper bound
	Step         Expr     // Step increment (nil means 1)
	Stmts        []Stmt   // Loop body
}

// GenericForStmt represents a generic for loop (for k, v in iter do ... end).
type GenericForStmt struct {
	StmtBase
	Names         []string   // Loop variable names
	NamePositions []Position // Per-name token positions, parallel to Names
	Exprs         []Expr     // Iterator expressions
	Stmts         []Stmt     // Loop body
}

// FuncDefStmt represents a function definition statement.
type FuncDefStmt struct {
	StmtBase
	Name *FuncName     // Function name (may include receiver for methods)
	Func *FunctionExpr // Function body
}

// ReturnStmt represents a return statement.
type ReturnStmt struct {
	StmtBase
	Exprs []Expr // Return values
}

// BreakStmt represents a break statement.
type BreakStmt struct {
	StmtBase
}

// LabelStmt represents a label definition (::name::).
type LabelStmt struct {
	StmtBase
	Name string // Label name
}

// GotoStmt represents a goto statement.
type GotoStmt struct {
	StmtBase
	Label string // Target label name
}
