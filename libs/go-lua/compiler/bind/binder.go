// Package bind implements name resolution and symbol binding for Lua AST nodes.
//
// The binding phase occurs after parsing and before type checking. It resolves
// all identifier references in the AST to unique SymbolIDs, establishing the
// connection between uses of a name and its declaration.
//
// The binder performs lexical scope analysis using a scope stack. Each scope
// frame tracks local variable declarations. Name lookup searches from innermost
// to outermost scope, implementing Lua's lexical scoping rules:
//
//   - Local variables shadow outer declarations of the same name
//   - Undeclared variables become implicit globals
//   - Function parameters create a new scope for the function body
//   - Loop variables (for statements) are local to the loop body
//   - repeat-until condition can see variables declared in the loop body
//
// The binding process handles several Lua-specific behaviors:
//
//   - Method definitions (function obj:method()) receive an implicit 'self' parameter
//   - Local function definitions are predeclared to support mutual recursion
//   - Expressions on the right side of 'local x = expr' see the outer scope
//   - Generic for loops can declare multiple iteration variables
//
// The result is a BindingTable that maps AST nodes to their resolved symbols,
// enabling subsequent compiler phases to work with unique symbol identities
// rather than string names.
package bind

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
)

// scopeFrame represents a single lexical scope level during binding.
// Each scope tracks the local variables declared at that level.
type scopeFrame struct {
	locals map[string]cfg.SymbolID
}

// Binder performs name resolution by walking the AST and assigning unique
// SymbolIDs to all identifier declarations and references.
//
// The binder maintains a stack of scope frames to implement lexical scoping.
// The bottom frame (index 0) holds global variables. Each function body,
// block statement, and loop body pushes a new frame.
//
// Usage:
//
//	table := bind.Bind(functionAST, predeclaredGlobals)
//	sym, ok := table.SymbolOf(someIdentifier)
type Binder struct {
	table   *BindingTable
	stack   []scopeFrame
	globals map[string]cfg.SymbolID
}

// NewBinder creates a binder initialized with predeclared global names.
//
// The globals parameter specifies names that should be treated as predefined
// global variables (e.g., "print", "pairs", "ipairs"). These are placed in
// the bottom scope frame and receive SymbolGlobal kind.
//
// Globals are sorted before processing to ensure deterministic SymbolID
// assignment across multiple bindings of the same source.
func NewBinder(globals []string) *Binder {
	return NewBinderWithDeclHint(globals, 0)
}

// NewBinderWithDeclHint creates a binder initialized with global names and an
// optional declaration-count hint for sizing internal binding maps.
func NewBinderWithDeclHint(globals []string, declHint int) *Binder {
	if declHint < 0 {
		declHint = 0
	}

	symbolHint := len(globals)
	if declHint >= 32 {
		symbolHint += declHint
	}

	b := &Binder{
		table:   NewBindingTableWithHint(symbolHint, declHint),
		stack:   []scopeFrame{{locals: make(map[string]cfg.SymbolID)}},
		globals: make(map[string]cfg.SymbolID),
	}
	// Sort and dedupe globals for deterministic SymbolID assignment
	sortedGlobals := make([]string, len(globals))
	copy(sortedGlobals, globals)
	sort.Strings(sortedGlobals)
	uniqueGlobals := make([]string, 0, len(sortedGlobals))
	seen := make(map[string]struct{}, len(sortedGlobals))
	for _, name := range sortedGlobals {
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		uniqueGlobals = append(uniqueGlobals, name)
	}
	start := cfg.ReserveSymbolIDs(len(uniqueGlobals))
	for i, name := range uniqueGlobals {
		sym := start + cfg.SymbolID(i)
		b.stack[0].locals[name] = sym
		b.globals[name] = sym
		b.table.SetKind(sym, cfg.SymbolGlobal)
		b.table.SetName(sym, name)
	}
	return b
}

// NewBinderWithStmtHint preserves the previous API surface; stmtHint is treated
// as a declaration-density hint.
func NewBinderWithStmtHint(globals []string, stmtHint int) *Binder {
	return NewBinderWithDeclHint(globals, stmtHint)
}

// Bind performs complete name resolution on a function AST.
//
// This is the main entry point for the binding phase. It creates a binder
// with the specified predeclared globals, processes the entire function
// (including nested functions), and returns the resulting BindingTable.
//
// The returned table maps all IdentExpr nodes to their resolved symbols
// and records parameter/local symbol lists for each function and declaration.
func Bind(fn *ast.FunctionExpr, globals []string) *BindingTable {
	declHint := 0
	if fn != nil {
		declHint = countDeclHintsInFunction(fn, false)
	}

	b := NewBinderWithDeclHint(globals, declHint)
	b.bindFunctionWithImplicitSelf(fn, false)
	return b.table
}

func countDeclHintsInFunction(fn *ast.FunctionExpr, hasImplicitSelf bool) int {
	if fn == nil {
		return 0
	}

	count := 0

	hasExplicitSelf := fn.ParList != nil && len(fn.ParList.Names) > 0 && fn.ParList.Names[0] == "self"
	if hasImplicitSelf && !hasExplicitSelf {
		count++
	}

	if fn.ParList != nil {
		count += len(fn.ParList.Names)
	}

	count += countDeclHintsInStmts(fn.Stmts)

	return count
}

func countDeclHintsInStmts(stmts []ast.Stmt) int {
	count := 0

	for _, stmt := range stmts {
		count += countDeclHintsInStmt(stmt)
	}

	return count
}

func countDeclHintsInStmt(stmt ast.Stmt) int {
	if stmt == nil {
		return 0
	}

	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		count := len(s.Names)
		for _, expr := range s.Exprs {
			count += countDeclHintsInExpr(expr)
		}

		return count
	case *ast.AssignStmt:
		count := 0
		for _, expr := range s.Lhs {
			count += countDeclHintsInExpr(expr)
		}
		for _, expr := range s.Rhs {
			count += countDeclHintsInExpr(expr)
		}

		return count
	case *ast.FuncCallStmt:
		return countDeclHintsInExpr(s.Expr)
	case *ast.DoBlockStmt:
		return countDeclHintsInStmts(s.Stmts)
	case *ast.WhileStmt:
		return countDeclHintsInExpr(s.Condition) + countDeclHintsInStmts(s.Stmts)
	case *ast.RepeatStmt:
		return countDeclHintsInStmts(s.Stmts) + countDeclHintsInExpr(s.Condition)
	case *ast.IfStmt:
		return countDeclHintsInExpr(s.Condition) + countDeclHintsInStmts(s.Then) + countDeclHintsInStmts(s.Else)
	case *ast.NumberForStmt:
		return 1 + countDeclHintsInExpr(s.Init) + countDeclHintsInExpr(s.Limit) + countDeclHintsInExpr(s.Step) +
			countDeclHintsInStmts(s.Stmts)
	case *ast.GenericForStmt:
		count := len(s.Names)
		for _, expr := range s.Exprs {
			count += countDeclHintsInExpr(expr)
		}
		count += countDeclHintsInStmts(s.Stmts)

		return count
	case *ast.FuncDefStmt:
		isMethod := s.Name != nil && s.Name.Method != ""
		count := countDeclHintsInFunction(s.Func, isMethod)
		if s.Name != nil {
			count += countDeclHintsInExpr(s.Name.Func) + countDeclHintsInExpr(s.Name.Receiver)
		}

		return count
	case *ast.ReturnStmt:
		count := 0
		for _, expr := range s.Exprs {
			count += countDeclHintsInExpr(expr)
		}

		return count
	default:
		return 0
	}
}

func countDeclHintsInExpr(expr ast.Expr) int {
	if expr == nil {
		return 0
	}

	switch e := expr.(type) {
	case *ast.FunctionExpr:
		return countDeclHintsInFunction(e, false)
	case *ast.AttrGetExpr:
		return countDeclHintsInExpr(e.Object) + countDeclHintsInExpr(e.Key)
	case *ast.TableExpr:
		count := 0
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			count += countDeclHintsInExpr(field.Key)
			count += countDeclHintsInExpr(field.Value)
		}

		return count
	case *ast.FuncCallExpr:
		count := countDeclHintsInExpr(e.Func) + countDeclHintsInExpr(e.Receiver)
		for _, arg := range e.Args {
			count += countDeclHintsInExpr(arg)
		}

		return count
	case *ast.LogicalOpExpr:
		return countDeclHintsInExpr(e.Lhs) + countDeclHintsInExpr(e.Rhs)
	case *ast.RelationalOpExpr:
		return countDeclHintsInExpr(e.Lhs) + countDeclHintsInExpr(e.Rhs)
	case *ast.StringConcatOpExpr:
		return countDeclHintsInExpr(e.Lhs) + countDeclHintsInExpr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return countDeclHintsInExpr(e.Lhs) + countDeclHintsInExpr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return countDeclHintsInExpr(e.Expr)
	case *ast.UnaryNotOpExpr:
		return countDeclHintsInExpr(e.Expr)
	case *ast.UnaryLenOpExpr:
		return countDeclHintsInExpr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return countDeclHintsInExpr(e.Expr)
	case *ast.CastExpr:
		return countDeclHintsInExpr(e.Expr)
	case *ast.NonNilAssertExpr:
		return countDeclHintsInExpr(e.Expr)
	default:
		return 0
	}
}

// enterScope pushes a new empty scope frame onto the stack.
func (b *Binder) enterScope() {
	b.stack = append(b.stack, scopeFrame{locals: make(map[string]cfg.SymbolID)})
}

// exitScope pops the topmost scope frame, but never below the global scope.
func (b *Binder) exitScope() {
	if len(b.stack) > 1 {
		b.stack = b.stack[:len(b.stack)-1]
	}
}

// lookup searches for a name from innermost to outermost scope.
// Returns the symbol and true if found, zero and false otherwise.
func (b *Binder) lookup(name string) (cfg.SymbolID, bool) {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if sym, ok := b.stack[i].locals[name]; ok {
			return sym, true
		}
	}
	return 0, false
}

// declareParam creates a new parameter symbol in the current scope.
func (b *Binder) declareParam(name string) cfg.SymbolID {
	sym := cfg.NextSymbolID()
	b.stack[len(b.stack)-1].locals[name] = sym
	b.table.SetKind(sym, cfg.SymbolParam)
	b.table.SetName(sym, name)
	return sym
}

// declareLocal creates a new local variable symbol in the current scope.
func (b *Binder) declareLocal(name string) cfg.SymbolID {
	sym := cfg.NextSymbolID()
	b.stack[len(b.stack)-1].locals[name] = sym
	b.table.SetKind(sym, cfg.SymbolLocal)
	b.table.SetName(sym, name)
	return sym
}

// declareGlobal returns an existing global symbol or creates a new one.
// Globals are always stored in the bottom scope frame (index 0).
func (b *Binder) declareGlobal(name string) cfg.SymbolID {
	if sym, ok := b.stack[0].locals[name]; ok {
		return sym
	}
	sym := cfg.NextSymbolID()
	b.stack[0].locals[name] = sym
	b.globals[name] = sym
	b.table.SetKind(sym, cfg.SymbolGlobal)
	b.table.SetName(sym, name)
	return sym
}

// bindIdent resolves an identifier reference to its declaration.
// If the name is not found in any scope, it becomes an implicit global.
func (b *Binder) bindIdent(ident *ast.IdentExpr) {
	if ident == nil {
		return
	}
	sym, ok := b.lookup(ident.Value)
	if !ok {
		sym = b.declareGlobal(ident.Value)
	}
	b.table.Bind(ident, sym)
}

// bindStmts processes a list of statements within the current scope.
// Local functions are predeclared first to enable mutual recursion.
func (b *Binder) bindStmts(stmts []ast.Stmt) {
	b.predeclareLocalFunctions(stmts)

	for _, stmt := range stmts {
		b.bindStmt(stmt)
	}
}

// predeclareLocalFunctions scans for local function declarations and declares
// their names before processing any function bodies.
//
// This enables mutual recursion between local functions defined in the same
// block. The pattern detected is: local f = function() ... end
//
// Example where this matters:
//
//	local function even(n) return n == 0 or odd(n-1) end
//	local function odd(n) return n ~= 0 and even(n-1) end
//
// Without predeclaration, 'odd' would not be visible inside 'even'.
func (b *Binder) predeclareLocalFunctions(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		local, ok := stmt.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		if len(local.Names) != 1 || len(local.Exprs) != 1 {
			continue
		}
		if _, isFunc := local.Exprs[0].(*ast.FunctionExpr); !isFunc {
			continue
		}
		// Already declared (shouldn't happen, but be safe)
		if b.table.HasLocalSymbols(local) {
			continue
		}
		// Declare the local function name now
		sym := b.declareLocal(local.Names[0])
		b.table.SetLocalSymbol(local, sym)
	}
}

// bindStmt dispatches to the appropriate handler for each statement type.
func (b *Binder) bindStmt(stmt ast.Stmt) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		b.bindAssignStmt(s)
	case *ast.LocalAssignStmt:
		b.bindLocalAssignStmt(s)
	case *ast.FuncCallStmt:
		b.bindExpr(s.Expr)
	case *ast.DoBlockStmt:
		b.enterScope()
		b.bindStmts(s.Stmts)
		b.exitScope()
	case *ast.WhileStmt:
		b.bindExpr(s.Condition)
		b.enterScope()
		b.bindStmts(s.Stmts)
		b.exitScope()
	case *ast.RepeatStmt:
		b.enterScope()
		b.bindStmts(s.Stmts)
		b.bindExpr(s.Condition)
		b.exitScope()
	case *ast.IfStmt:
		b.bindExpr(s.Condition)
		b.enterScope()
		b.bindStmts(s.Then)
		b.exitScope()
		b.enterScope()
		b.bindStmts(s.Else)
		b.exitScope()
	case *ast.NumberForStmt:
		b.bindExpr(s.Init)
		b.bindExpr(s.Limit)
		b.bindExpr(s.Step)
		b.enterScope()
		sym := b.declareLocal(s.Name)
		b.table.SetNumForSymbol(s, sym)
		b.bindStmts(s.Stmts)
		b.exitScope()
	case *ast.GenericForStmt:
		for _, expr := range s.Exprs {
			b.bindExpr(expr)
		}
		b.enterScope()
		syms := make([]cfg.SymbolID, 0, len(s.Names))
		for _, name := range s.Names {
			sym := b.declareLocal(name)
			syms = append(syms, sym)
		}
		b.table.SetGenericForSymbols(s, syms)
		b.bindStmts(s.Stmts)
		b.exitScope()
	case *ast.FuncDefStmt:
		b.bindFuncDefStmt(s)
	case *ast.ReturnStmt:
		for _, expr := range s.Exprs {
			b.bindExpr(expr)
		}
	case *ast.BreakStmt:
	case *ast.LabelStmt:
	case *ast.GotoStmt:
	}
}

// bindAssignStmt processes an assignment statement.
// Right-hand side expressions are bound before left-hand side targets,
// ensuring 'x = x + 1' correctly references the existing x.
func (b *Binder) bindAssignStmt(s *ast.AssignStmt) {
	for _, expr := range s.Rhs {
		b.bindExpr(expr)
	}
	for _, expr := range s.Lhs {
		b.bindAssignTarget(expr)
	}
}

// bindAssignTarget resolves an assignment target (left-hand side).
// Identifiers become globals if not found in scope. Field accesses
// have their object and key expressions bound.
func (b *Binder) bindAssignTarget(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := b.lookup(e.Value)
		if !ok {
			sym = b.declareGlobal(e.Value)
		}
		b.table.Bind(e, sym)
	case *ast.AttrGetExpr:
		b.bindExpr(e.Object)
		b.bindExpr(e.Key)
	}
}

// bindLocalAssignStmt processes a local variable declaration.
//
// For predeclared local functions (mutual recursion support), only the
// initializer expressions are bound since the names are already declared.
//
// For regular locals, expressions are bound first (seeing the outer scope),
// then the new local names are declared. This ensures 'local x = x' binds
// the RHS x to any outer declaration before shadowing it.
func (b *Binder) bindLocalAssignStmt(s *ast.LocalAssignStmt) {
	if b.table.HasLocalSymbols(s) {
		for _, expr := range s.Exprs {
			b.bindExpr(expr)
		}
		return
	}

	for _, expr := range s.Exprs {
		b.bindExpr(expr)
	}
	if len(s.Names) == 1 {
		b.table.SetLocalSymbol(s, b.declareLocal(s.Names[0]))

		return
	}

	syms := make([]cfg.SymbolID, len(s.Names))
	for i, name := range s.Names {
		syms[i] = b.declareLocal(name)
	}
	b.table.SetLocalSymbols(s, syms)
}

// bindFuncDefStmt processes a function definition statement.
//
// For simple global functions (function foo() end), the name becomes a global.
// For field functions (function M.f() end), the receiver path is bound.
// Method definitions (function obj:method() end) receive implicit self.
func (b *Binder) bindFuncDefStmt(s *ast.FuncDefStmt) {
	if s.Name != nil {
		if ident, ok := s.Name.Func.(*ast.IdentExpr); ok && s.Name.Receiver == nil && s.Name.Method == "" {
			sym, ok := b.lookup(ident.Value)
			if !ok {
				sym = b.declareGlobal(ident.Value)
			}
			b.table.Bind(ident, sym)
		} else {
			if s.Name.Func != nil {
				b.bindExpr(s.Name.Func)
			}
			if s.Name.Receiver != nil {
				b.bindExpr(s.Name.Receiver)
			}
		}
	}
	isMethod := s.Name != nil && s.Name.Method != ""
	b.bindFunctionWithImplicitSelf(s.Func, isMethod)
}

// bindFunctionWithImplicitSelf processes a function expression.
//
// Creates a new scope for the function body. If hasImplicitSelf is true
// and 'self' is not already in the parameter list, an implicit 'self'
// parameter is added as the first parameter (for method definitions).
func (b *Binder) bindFunctionWithImplicitSelf(fn *ast.FunctionExpr, hasImplicitSelf bool) {
	if fn == nil {
		return
	}
	b.enterScope()

	var syms []cfg.SymbolID
	hasExplicitSelf := fn.ParList != nil && len(fn.ParList.Names) > 0 && fn.ParList.Names[0] == "self"
	if hasImplicitSelf && !hasExplicitSelf {
		selfSym := b.declareParam("self")
		syms = append(syms, selfSym)
	}

	if fn.ParList != nil {
		for _, name := range fn.ParList.Names {
			sym := b.declareParam(name)
			syms = append(syms, sym)
		}
	}

	if len(syms) > 0 {
		b.table.SetParamSymbols(fn, syms)
	}

	b.bindStmts(fn.Stmts)
	b.exitScope()
}

// bindExpr dispatches to the appropriate handler for each expression type.
func (b *Binder) bindExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		b.bindIdent(e)
	case *ast.AttrGetExpr:
		b.bindExpr(e.Object)
		b.bindExpr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			b.bindExpr(field.Key)
			b.bindExpr(field.Value)
		}
	case *ast.FunctionExpr:
		b.bindFunctionWithImplicitSelf(e, false)
	case *ast.FuncCallExpr:
		b.bindExpr(e.Func)
		b.bindExpr(e.Receiver)
		for _, arg := range e.Args {
			b.bindExpr(arg)
		}
	case *ast.LogicalOpExpr:
		b.bindExpr(e.Lhs)
		b.bindExpr(e.Rhs)
	case *ast.RelationalOpExpr:
		b.bindExpr(e.Lhs)
		b.bindExpr(e.Rhs)
	case *ast.StringConcatOpExpr:
		b.bindExpr(e.Lhs)
		b.bindExpr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		b.bindExpr(e.Lhs)
		b.bindExpr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		b.bindExpr(e.Expr)
	case *ast.UnaryNotOpExpr:
		b.bindExpr(e.Expr)
	case *ast.UnaryLenOpExpr:
		b.bindExpr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		b.bindExpr(e.Expr)
	case *ast.CastExpr:
		b.bindExpr(e.Expr)
	case *ast.NonNilAssertExpr:
		b.bindExpr(e.Expr)
	}
}
