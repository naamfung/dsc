// Package bind provides name resolution for Lua programs.
//
// The binder traverses AST nodes and resolves identifier references to
// unique symbols. This establishes the binding between uses and definitions,
// enabling type checking and CFG construction to work with stable identities.
//
// # Symbol Assignment
//
// Symbols are assigned to:
//   - Local variables at their declaration point
//   - Function parameters at function entry
//   - For loop iteration variables at loop entry
//   - Global references (assumed external)
//   - Field paths like M.f.g (each qualified access gets a symbol)
//
// # Usage
//
// Call [Bind] with a function AST and optional global names:
//
//	bindings := bind.Bind(funcExpr, "print", "error", "assert")
//
// The returned [BindingTable] provides:
//   - [BindingTable.SymbolOf]: Look up symbol for an IdentExpr
//   - [BindingTable.Name]: Get the source name for a symbol
//   - [BindingTable.Kind]: Get the declaration kind (local/param/global)
//   - [BindingTable.ParamSymbols]: Get parameter symbols for a function
package bind

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
)

// BindingTable stores the results of name resolution for a Lua program.
//
// The table provides bidirectional mapping between AST nodes and symbols:
//   - IdentExpr nodes map to their resolved SymbolID
//   - FunctionExpr nodes map to their parameter symbols
//   - LocalAssignStmt nodes map to their declared local symbols
//   - For loop statements map to their iteration variable symbols
//
// Each symbol has an associated kind (global, local, param) and name.
// The table also supports field paths (M.f.g) and anonymous function symbols.
//
// BindingTable is the primary output of the binding phase and is consumed
// by type checking and CFG construction phases.
type BindingTable struct {
	// symbols maps identifier references to their resolved symbols
	symbols map[*ast.IdentExpr]cfg.SymbolID

	// kind stores the declaration kind for each symbol
	kind map[cfg.SymbolID]cfg.SymbolKind

	// names stores the original source name for each symbol
	names map[cfg.SymbolID]string

	// symbolsByName lazily indexes symbols by source name in ascending symbol order.
	symbolsByName map[string][]cfg.SymbolID

	// paramSymbols maps functions to their parameter symbol list
	paramSymbols map[*ast.FunctionExpr][]cfg.SymbolID

	// localSymbolSingle maps single-name local declarations to their symbol.
	// This avoids allocating a one-element slice for the common local form.
	localSymbolSingle map[*ast.LocalAssignStmt]cfg.SymbolID

	// localSymbolsMulti maps multi-name local declarations to their symbol list.
	localSymbolsMulti map[*ast.LocalAssignStmt][]cfg.SymbolID

	// numForSymbols maps numeric for loops to their iteration variable
	numForSymbols map[*ast.NumberForStmt]cfg.SymbolID

	// genericForSymbols maps generic for loops to their iteration variables
	genericForSymbols map[*ast.GenericForStmt][]cfg.SymbolID

	// fieldSymbols maps qualified paths (base + field chain) to symbols
	fieldSymbols map[fieldPathKey]cfg.SymbolID

	// funcLitSymbols maps anonymous function expressions to symbols
	funcLitSymbols map[*ast.FunctionExpr]cfg.SymbolID

	// funcLitBySymbol maps symbols back to their function literals
	funcLitBySymbol map[cfg.SymbolID]*ast.FunctionExpr

	// capturedCache memoizes captured symbols per function for repeated queries.
	capturedMu    sync.RWMutex
	capturedCache map[*ast.FunctionExpr][]cfg.SymbolID
}

// fieldPathKey identifies a field access path rooted at a base symbol.
// Used to give unique symbols to qualified names like M.f or M.f.g.
type fieldPathKey struct {
	base cfg.SymbolID
	path string
}

// NewBindingTable creates an empty binding table with all maps initialized.
func NewBindingTable() *BindingTable {
	return NewBindingTableWithHint(0, 0)
}

// NewBindingTableWithHint creates a binding table with optional size hints.
//
// symbolHint estimates total symbols in the bound unit (locals/params/globals),
// while stmtHint estimates top-level statement volume.
func NewBindingTableWithHint(symbolHint, stmtHint int) *BindingTable {
	if symbolHint < 0 {
		symbolHint = 0
	}
	if stmtHint < 0 {
		stmtHint = 0
	}

	identHint := 0
	localSingleHint := 0

	// Only pre-size maps on larger units where map growth dominates.
	if stmtHint >= 32 {
		localSingleHint = stmtHint
	}

	return &BindingTable{
		symbols:           make(map[*ast.IdentExpr]cfg.SymbolID, identHint),
		kind:              make(map[cfg.SymbolID]cfg.SymbolKind, symbolHint),
		names:             make(map[cfg.SymbolID]string, symbolHint),
		paramSymbols:      make(map[*ast.FunctionExpr][]cfg.SymbolID),
		localSymbolSingle: make(map[*ast.LocalAssignStmt]cfg.SymbolID, localSingleHint),
		localSymbolsMulti: make(map[*ast.LocalAssignStmt][]cfg.SymbolID),
		numForSymbols:     make(map[*ast.NumberForStmt]cfg.SymbolID),
		genericForSymbols: make(map[*ast.GenericForStmt][]cfg.SymbolID),
		fieldSymbols:      make(map[fieldPathKey]cfg.SymbolID),
		funcLitSymbols:    make(map[*ast.FunctionExpr]cfg.SymbolID),
		funcLitBySymbol:   make(map[cfg.SymbolID]*ast.FunctionExpr),
		capturedCache:     make(map[*ast.FunctionExpr][]cfg.SymbolID),
	}
}

// SymbolOf returns the symbol that an identifier reference resolves to.
// Returns zero and false if the identifier is nil or not bound.
func (t *BindingTable) SymbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	if ident == nil {
		return 0, false
	}
	sym, ok := t.symbols[ident]
	return sym, ok
}

// Bind records that an identifier references a specific symbol.
func (t *BindingTable) Bind(ident *ast.IdentExpr, sym cfg.SymbolID) {
	if ident == nil {
		return
	}
	t.symbols[ident] = sym
}

// SetKind records whether a symbol is a global, local, or parameter.
func (t *BindingTable) SetKind(sym cfg.SymbolID, k cfg.SymbolKind) {
	t.kind[sym] = k
}

// Kind returns the declaration kind of a symbol.
// Returns false if the symbol has no recorded kind.
func (t *BindingTable) Kind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	k, ok := t.kind[sym]
	return k, ok
}

// SetName records the source name associated with a symbol.
func (t *BindingTable) SetName(sym cfg.SymbolID, name string) {
	if t == nil || sym == 0 {
		return
	}
	if prev, ok := t.names[sym]; ok {
		if prev == name {
			return
		}
	}
	t.names[sym] = name
	t.symbolsByName = nil
}

// Name returns the source name of a symbol, or empty if unknown.
func (t *BindingTable) Name(sym cfg.SymbolID) string {
	return t.names[sym]
}

// SymbolsByName returns all symbols recorded with the given source name.
//
// Results are sorted by symbol ID for deterministic iteration.
func (t *BindingTable) SymbolsByName(name string) []cfg.SymbolID {
	syms := t.SymbolsByNameReadOnly(name)
	if len(syms) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, len(syms))
	copy(out, syms)
	return out
}

// SymbolsByNameReadOnly returns the stored symbols for a source name.
//
// The returned slice is sorted by symbol ID and must be treated as read-only.
func (t *BindingTable) SymbolsByNameReadOnly(name string) []cfg.SymbolID {
	if t == nil || name == "" {
		return nil
	}
	if t.symbolsByName == nil {
		t.symbolsByName = t.buildSymbolsByNameIndex()
	}
	return t.symbolsByName[name]
}

func (t *BindingTable) buildSymbolsByNameIndex() map[string][]cfg.SymbolID {
	if t == nil || len(t.names) == 0 {
		return nil
	}
	index := make(map[string][]cfg.SymbolID)
	for sym, name := range t.names {
		if sym == 0 || name == "" {
			continue
		}
		index[name] = append(index[name], sym)
	}
	for name, syms := range index {
		if len(syms) > 1 {
			sort.Slice(syms, func(i, j int) bool {
				return syms[i] < syms[j]
			})
		}
		index[name] = syms
	}
	return index
}

// SetParamSymbols records the ordered parameter symbols for a function.
// For methods with implicit self, self is the first symbol in the list.
func (t *BindingTable) SetParamSymbols(fn *ast.FunctionExpr, syms []cfg.SymbolID) {
	if fn == nil {
		return
	}
	t.paramSymbols[fn] = syms
}

// ParamSymbols returns the parameter symbols for a function in declaration order.
func (t *BindingTable) ParamSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	return t.paramSymbols[fn]
}

// SetLocalSymbols records the symbols declared by a local assignment.
// For 'local a, b, c = ...', the symbols correspond to a, b, c in order.
func (t *BindingTable) SetLocalSymbols(stmt *ast.LocalAssignStmt, syms []cfg.SymbolID) {
	if stmt == nil {
		return
	}
	delete(t.localSymbolSingle, stmt)

	switch len(syms) {
	case 0:
		delete(t.localSymbolsMulti, stmt)
	case 1:
		if syms[0] != 0 {
			t.localSymbolSingle[stmt] = syms[0]
			delete(t.localSymbolsMulti, stmt)
		} else {
			delete(t.localSymbolsMulti, stmt)
		}
	default:
		t.localSymbolsMulti[stmt] = syms
	}
}

// SetLocalSymbol records the symbol declared by a single-name local assignment.
func (t *BindingTable) SetLocalSymbol(stmt *ast.LocalAssignStmt, sym cfg.SymbolID) {
	if stmt == nil || sym == 0 {
		return
	}
	t.localSymbolSingle[stmt] = sym
	delete(t.localSymbolsMulti, stmt)
}

// HasLocalSymbols reports whether local symbols were recorded for stmt.
func (t *BindingTable) HasLocalSymbols(stmt *ast.LocalAssignStmt) bool {
	if stmt == nil {
		return false
	}
	if _, ok := t.localSymbolSingle[stmt]; ok {
		return true
	}
	_, ok := t.localSymbolsMulti[stmt]

	return ok
}

// LocalSymbolAt returns the i-th symbol declared by a local assignment.
func (t *BindingTable) LocalSymbolAt(stmt *ast.LocalAssignStmt, i int) (cfg.SymbolID, bool) {
	if stmt == nil || i < 0 {
		return 0, false
	}

	if sym, ok := t.localSymbolSingle[stmt]; ok {
		if i == 0 {
			return sym, true
		}

		return 0, false
	}

	syms, ok := t.localSymbolsMulti[stmt]
	if !ok || i >= len(syms) {
		return 0, false
	}

	sym := syms[i]
	if sym == 0 {
		return 0, false
	}

	return sym, true
}

// LocalSymbols returns the symbols declared by a local assignment.
func (t *BindingTable) LocalSymbols(stmt *ast.LocalAssignStmt) []cfg.SymbolID {
	if stmt == nil {
		return nil
	}
	if sym, ok := t.localSymbolSingle[stmt]; ok && sym != 0 {
		return []cfg.SymbolID{sym}
	}
	return t.localSymbolsMulti[stmt]
}

// SetNumForSymbol records the iteration variable for a numeric for loop.
// For 'for i = 1, 10 do ... end', this records the symbol for 'i'.
func (t *BindingTable) SetNumForSymbol(stmt *ast.NumberForStmt, sym cfg.SymbolID) {
	if stmt == nil {
		return
	}
	t.numForSymbols[stmt] = sym
}

// NumForSymbol returns the iteration variable symbol for a numeric for.
func (t *BindingTable) NumForSymbol(stmt *ast.NumberForStmt) (cfg.SymbolID, bool) {
	sym, ok := t.numForSymbols[stmt]
	return sym, ok
}

// SetGenericForSymbols records the iteration variables for a generic for loop.
// For 'for k, v in pairs(t) do ... end', this records symbols for k and v.
func (t *BindingTable) SetGenericForSymbols(stmt *ast.GenericForStmt, syms []cfg.SymbolID) {
	if stmt == nil {
		return
	}
	t.genericForSymbols[stmt] = syms
}

// GenericForSymbols returns the iteration variable symbols for a generic for.
func (t *BindingTable) GenericForSymbols(stmt *ast.GenericForStmt) []cfg.SymbolID {
	return t.genericForSymbols[stmt]
}

// GetOrCreateFieldSymbol returns or creates a symbol for a qualified field path.
//
// Field paths represent nested property access like M.f or M.f.g. The baseSym
// identifies the root object, and path is the dot-separated field chain.
//
// The created symbol gets a composite name (e.g., "M.f") and SymbolLocal kind.
// Subsequent calls with the same base and path return the existing symbol.
func (t *BindingTable) GetOrCreateFieldSymbol(baseSym cfg.SymbolID, path string) cfg.SymbolID {
	canonicalPath, ok := NormalizeFieldPathKey(path)
	if !ok {
		return 0
	}

	key := fieldPathKey{base: baseSym, path: canonicalPath}
	if sym, ok := t.fieldSymbols[key]; ok {
		return sym
	}
	sym := cfg.NextSymbolID()
	t.fieldSymbols[key] = sym
	baseName := t.names[baseSym]
	displayPath := displayFieldPathKey(canonicalPath)

	if baseName != "" {
		if len(canonicalPath) > 0 && (canonicalPath[0] == '.' || canonicalPath[0] == '[') {
			t.names[sym] = baseName + canonicalPath
		} else {
			t.names[sym] = baseName + "." + displayPath
		}
	} else {
		t.names[sym] = displayPath
	}
	t.kind[sym] = cfg.SymbolLocal
	return sym
}

// FieldSymbol looks up an existing symbol for a field path.
// Returns zero and false if no symbol exists for the given base and path.
func (t *BindingTable) FieldSymbol(baseSym cfg.SymbolID, path string) (cfg.SymbolID, bool) {
	canonicalPath, ok := NormalizeFieldPathKey(path)
	if !ok {
		return 0, false
	}

	key := fieldPathKey{base: baseSym, path: canonicalPath}
	sym, ok := t.fieldSymbols[key]
	return sym, ok
}

// GetOrCreateFuncLitSymbol returns or creates a symbol for an anonymous function.
//
// Anonymous functions (function literals) need symbols for type assignment
// and reference tracking. Each unique FunctionExpr AST node gets its own symbol.
func (t *BindingTable) GetOrCreateFuncLitSymbol(fn *ast.FunctionExpr) cfg.SymbolID {
	if fn == nil {
		return 0
	}
	if sym, ok := t.funcLitSymbols[fn]; ok {
		return sym
	}
	sym := cfg.NextSymbolID()
	t.funcLitSymbols[fn] = sym
	t.funcLitBySymbol[sym] = fn
	t.kind[sym] = cfg.SymbolLocal
	return sym
}

// SetFuncLitSymbol assigns a specific symbol to a function literal.
func (t *BindingTable) SetFuncLitSymbol(fn *ast.FunctionExpr, sym cfg.SymbolID) {
	if fn == nil {
		return
	}
	t.funcLitSymbols[fn] = sym
	if sym != 0 {
		t.funcLitBySymbol[sym] = fn
	}
}

// FuncLitSymbol returns the symbol for a function literal if one exists.
func (t *BindingTable) FuncLitSymbol(fn *ast.FunctionExpr) (cfg.SymbolID, bool) {
	if fn == nil {
		return 0, false
	}
	sym, ok := t.funcLitSymbols[fn]
	return sym, ok
}

// FuncLitBySymbol returns the function literal for a symbol if known.
func (t *BindingTable) FuncLitBySymbol(sym cfg.SymbolID) (*ast.FunctionExpr, bool) {
	if sym == 0 {
		return nil, false
	}
	fn, ok := t.funcLitBySymbol[sym]
	return fn, ok
}

// CapturedSymbols identifies free variables in a function.
//
// A captured symbol is one that is referenced inside the function body but
// declared outside it. This includes both upvalues (locals from enclosing
// functions) and globals.
//
// The analysis walks the function body to find all referenced symbols,
// then subtracts symbols declared within the function (parameters and locals).
func (t *BindingTable) CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	if fn == nil {
		return nil
	}
	t.capturedMu.RLock()
	if cached, ok := t.capturedCache[fn]; ok {
		t.capturedMu.RUnlock()
		return cached
	}
	t.capturedMu.RUnlock()

	declared := make(map[cfg.SymbolID]bool)

	for _, sym := range t.paramSymbols[fn] {
		if sym != 0 {
			declared[sym] = true
		}
	}

	t.collectDeclaredInStmts(fn.Stmts, declared)

	referenced := make(map[cfg.SymbolID]bool)
	t.collectReferencedInStmts(fn.Stmts, referenced)

	var captured []cfg.SymbolID
	for sym := range referenced {
		if !declared[sym] {
			captured = append(captured, sym)
		}
	}
	if len(captured) > 1 {
		sort.Slice(captured, func(i, j int) bool { return captured[i] < captured[j] })
	}
	t.capturedMu.Lock()
	if existing, ok := t.capturedCache[fn]; ok {
		t.capturedMu.Unlock()
		return existing
	}
	t.capturedCache[fn] = captured
	t.capturedMu.Unlock()
	return captured
}

// collectDeclaredInStmts gathers all symbols declared within a statement list.
func (t *BindingTable) collectDeclaredInStmts(stmts []ast.Stmt, declared map[cfg.SymbolID]bool) {
	for _, stmt := range stmts {
		t.collectDeclaredInStmt(stmt, declared)
	}
}

// collectDeclaredInStmt gathers symbols declared by a single statement.
// Recurses into nested blocks but not into nested function bodies.
func (t *BindingTable) collectDeclaredInStmt(stmt ast.Stmt, declared map[cfg.SymbolID]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		if sym, ok := t.localSymbolSingle[s]; ok && sym != 0 {
			declared[sym] = true
			break
		}
		for _, sym := range t.localSymbolsMulti[s] {
			if sym != 0 {
				declared[sym] = true
			}
		}
	case *ast.NumberForStmt:
		if sym, ok := t.numForSymbols[s]; ok && sym != 0 {
			declared[sym] = true
		}
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.GenericForStmt:
		for _, sym := range t.genericForSymbols[s] {
			if sym != 0 {
				declared[sym] = true
			}
		}
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.DoBlockStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.WhileStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.RepeatStmt:
		t.collectDeclaredInStmts(s.Stmts, declared)
	case *ast.IfStmt:
		t.collectDeclaredInStmts(s.Then, declared)
		t.collectDeclaredInStmts(s.Else, declared)
	case *ast.FuncDefStmt:
		// Nested function parameters belong to the nested scope, not this one
	}
}

// collectReferencedInStmts gathers all symbols referenced in a statement list.
func (t *BindingTable) collectReferencedInStmts(stmts []ast.Stmt, referenced map[cfg.SymbolID]bool) {
	for _, stmt := range stmts {
		t.collectReferencedInStmt(stmt, referenced)
	}
}

// collectReferencedInStmt gathers symbols referenced in a single statement.
// Recurses into nested blocks and function bodies to find all references.
func (t *BindingTable) collectReferencedInStmt(stmt ast.Stmt, referenced map[cfg.SymbolID]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, e := range s.Lhs {
			t.collectReferencedInExpr(e, referenced)
		}
		for _, e := range s.Rhs {
			t.collectReferencedInExpr(e, referenced)
		}
	case *ast.LocalAssignStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
	case *ast.FuncCallStmt:
		t.collectReferencedInExpr(s.Expr, referenced)
	case *ast.DoBlockStmt:
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.WhileStmt:
		t.collectReferencedInExpr(s.Condition, referenced)
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.RepeatStmt:
		t.collectReferencedInStmts(s.Stmts, referenced)
		t.collectReferencedInExpr(s.Condition, referenced)
	case *ast.IfStmt:
		t.collectReferencedInExpr(s.Condition, referenced)
		t.collectReferencedInStmts(s.Then, referenced)
		t.collectReferencedInStmts(s.Else, referenced)
	case *ast.NumberForStmt:
		t.collectReferencedInExpr(s.Init, referenced)
		t.collectReferencedInExpr(s.Limit, referenced)
		t.collectReferencedInExpr(s.Step, referenced)
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.GenericForStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
		t.collectReferencedInStmts(s.Stmts, referenced)
	case *ast.FuncDefStmt:
		if s.Func != nil {
			t.collectReferencedInStmts(s.Func.Stmts, referenced)
		}
	case *ast.ReturnStmt:
		for _, e := range s.Exprs {
			t.collectReferencedInExpr(e, referenced)
		}
	}
}

// collectReferencedInExpr gathers symbols referenced in an expression.
// Recurses into subexpressions and nested function bodies.
func (t *BindingTable) collectReferencedInExpr(expr ast.Expr, referenced map[cfg.SymbolID]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := t.symbols[e]; ok && sym != 0 {
			referenced[sym] = true
		}
	case *ast.AttrGetExpr:
		t.collectReferencedInExpr(e.Object, referenced)
		t.collectReferencedInExpr(e.Key, referenced)
	case *ast.TableExpr:
		for _, f := range e.Fields {
			if f != nil {
				t.collectReferencedInExpr(f.Key, referenced)
				t.collectReferencedInExpr(f.Value, referenced)
			}
		}
	case *ast.FuncCallExpr:
		t.collectReferencedInExpr(e.Func, referenced)
		t.collectReferencedInExpr(e.Receiver, referenced)
		for _, a := range e.Args {
			t.collectReferencedInExpr(a, referenced)
		}
	case *ast.FunctionExpr:
		// Recurse into nested function bodies
		t.collectReferencedInStmts(e.Stmts, referenced)
	case *ast.LogicalOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.RelationalOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.StringConcatOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.ArithmeticOpExpr:
		t.collectReferencedInExpr(e.Lhs, referenced)
		t.collectReferencedInExpr(e.Rhs, referenced)
	case *ast.UnaryMinusOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryNotOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryLenOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.UnaryBNotOpExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.CastExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	case *ast.NonNilAssertExpr:
		t.collectReferencedInExpr(e.Expr, referenced)
	}
}

// Globals returns all symbols with SymbolGlobal kind.
// This includes both predeclared globals and implicitly created globals.
func (t *BindingTable) Globals() []cfg.SymbolID {
	result := make([]cfg.SymbolID, 0)
	for sym, k := range t.kind {
		if k == cfg.SymbolGlobal && sym != 0 {
			result = append(result, sym)
		}
	}
	if len(result) > 1 {
		sort.Slice(result, func(i, j int) bool {
			return result[i] < result[j]
		})
	}
	return result
}

// AllSymbols returns every symbol in the table without duplicates.
//
// This collects symbols from all sources: bound identifiers, kinds map,
// parameter lists, local declarations, and for loop variables.
// Zero symbols are excluded.
func (t *BindingTable) AllSymbols() []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]bool)

	for sym := range t.kind {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, sym := range t.symbols {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, syms := range t.paramSymbols {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	for _, sym := range t.localSymbolSingle {
		if sym != 0 {
			seen[sym] = true
		}
	}
	for _, syms := range t.localSymbolsMulti {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	for _, sym := range t.numForSymbols {
		if sym != 0 {
			seen[sym] = true
		}
	}

	for _, syms := range t.genericForSymbols {
		for _, sym := range syms {
			if sym != 0 {
				seen[sym] = true
			}
		}
	}

	result := make([]cfg.SymbolID, 0, len(seen))
	for sym := range seen {
		result = append(result, sym)
	}
	if len(result) > 1 {
		sort.Slice(result, func(i, j int) bool {
			return result[i] < result[j]
		})
	}
	return result
}
