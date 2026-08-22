package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestNewBindingTable(t *testing.T) {
	table := NewBindingTable()
	if table == nil {
		t.Fatal("NewBindingTable returned nil")
	}
	if table.symbols == nil {
		t.Error("symbols map not initialized")
	}
	if table.kind == nil {
		t.Error("kind map not initialized")
	}
	if table.names == nil {
		t.Error("names map not initialized")
	}
	if table.paramSymbols == nil {
		t.Error("paramSymbols map not initialized")
	}
	if table.localSymbolSingle == nil {
		t.Error("localSymbolSingle map not initialized")
	}
	if table.localSymbolsMulti == nil {
		t.Error("localSymbolsMulti map not initialized")
	}
	if table.numForSymbols == nil {
		t.Error("numForSymbols map not initialized")
	}
	if table.genericForSymbols == nil {
		t.Error("genericForSymbols map not initialized")
	}
}

func TestBindingTable_SymbolOf(t *testing.T) {
	table := NewBindingTable()
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.NextSymbolID()

	// Before binding
	if _, ok := table.SymbolOf(ident); ok {
		t.Error("SymbolOf should return false for unbound ident")
	}

	// Nil ident
	if _, ok := table.SymbolOf(nil); ok {
		t.Error("SymbolOf should return false for nil ident")
	}

	// After binding
	table.Bind(ident, sym)
	got, ok := table.SymbolOf(ident)
	if !ok {
		t.Error("SymbolOf should return true for bound ident")
	}
	if got != sym {
		t.Errorf("SymbolOf returned %v, want %v", got, sym)
	}
}

func TestBindingTable_Bind_NilIdent(t *testing.T) {
	table := NewBindingTable()
	sym := cfg.NextSymbolID()
	// Should not panic
	table.Bind(nil, sym)
}

func TestBindingTable_Kind(t *testing.T) {
	table := NewBindingTable()
	sym := cfg.NextSymbolID()

	// Before setting
	if _, ok := table.Kind(sym); ok {
		t.Error("Kind should return false for unknown symbol")
	}

	// After setting
	table.SetKind(sym, cfg.SymbolLocal)
	got, ok := table.Kind(sym)
	if !ok {
		t.Error("Kind should return true after SetKind")
	}
	if got != cfg.SymbolLocal {
		t.Errorf("Kind returned %v, want SymbolLocal", got)
	}
}

func TestBindingTable_Name(t *testing.T) {
	table := NewBindingTable()
	sym := cfg.NextSymbolID()

	// Before setting
	if name := table.Name(sym); name != "" {
		t.Errorf("Name should return empty for unknown symbol, got %q", name)
	}

	// After setting
	table.SetName(sym, "myVar")
	if got := table.Name(sym); got != "myVar" {
		t.Errorf("Name returned %q, want %q", got, "myVar")
	}
}

func TestBindingTable_ParamSymbols(t *testing.T) {
	table := NewBindingTable()
	fn := &ast.FunctionExpr{}
	syms := []cfg.SymbolID{cfg.NextSymbolID(), cfg.NextSymbolID()}

	// Before setting
	if got := table.ParamSymbols(fn); got != nil {
		t.Errorf("ParamSymbols should return nil for unset function, got %v", got)
	}

	// After setting
	table.SetParamSymbols(fn, syms)
	got := table.ParamSymbols(fn)
	if len(got) != len(syms) {
		t.Fatalf("ParamSymbols returned %d symbols, want %d", len(got), len(syms))
	}
	for i := range syms {
		if got[i] != syms[i] {
			t.Errorf("ParamSymbols[%d] = %v, want %v", i, got[i], syms[i])
		}
	}
}

func TestBindingTable_SetParamSymbols_NilFn(t *testing.T) {
	table := NewBindingTable()
	// Should not panic
	table.SetParamSymbols(nil, []cfg.SymbolID{cfg.NextSymbolID()})
}

func TestBindingTable_LocalSymbols(t *testing.T) {
	table := NewBindingTable()
	stmt := &ast.LocalAssignStmt{Names: []string{"a", "b"}}
	syms := []cfg.SymbolID{cfg.NextSymbolID(), cfg.NextSymbolID()}

	// Before setting
	if got := table.LocalSymbols(stmt); got != nil {
		t.Errorf("LocalSymbols should return nil for unset stmt, got %v", got)
	}

	// After setting
	table.SetLocalSymbols(stmt, syms)
	got := table.LocalSymbols(stmt)
	if len(got) != len(syms) {
		t.Fatalf("LocalSymbols returned %d symbols, want %d", len(got), len(syms))
	}
}

func TestBindingTable_SetLocalSymbols_NilStmt(t *testing.T) {
	table := NewBindingTable()
	// Should not panic
	table.SetLocalSymbols(nil, []cfg.SymbolID{cfg.NextSymbolID()})
}

func TestBindingTable_NumForSymbol(t *testing.T) {
	table := NewBindingTable()
	stmt := &ast.NumberForStmt{Name: "i"}
	sym := cfg.NextSymbolID()

	// Before setting
	if _, ok := table.NumForSymbol(stmt); ok {
		t.Error("NumForSymbol should return false for unset stmt")
	}

	// After setting
	table.SetNumForSymbol(stmt, sym)
	got, ok := table.NumForSymbol(stmt)
	if !ok {
		t.Error("NumForSymbol should return true after SetNumForSymbol")
	}
	if got != sym {
		t.Errorf("NumForSymbol returned %v, want %v", got, sym)
	}
}

func TestBindingTable_SetNumForSymbol_NilStmt(t *testing.T) {
	table := NewBindingTable()
	// Should not panic
	table.SetNumForSymbol(nil, cfg.NextSymbolID())
}

func TestBindingTable_GenericForSymbols(t *testing.T) {
	table := NewBindingTable()
	stmt := &ast.GenericForStmt{Names: []string{"k", "v"}}
	syms := []cfg.SymbolID{cfg.NextSymbolID(), cfg.NextSymbolID()}

	// Before setting
	if got := table.GenericForSymbols(stmt); got != nil {
		t.Errorf("GenericForSymbols should return nil for unset stmt, got %v", got)
	}

	// After setting
	table.SetGenericForSymbols(stmt, syms)
	got := table.GenericForSymbols(stmt)
	if len(got) != len(syms) {
		t.Fatalf("GenericForSymbols returned %d symbols, want %d", len(got), len(syms))
	}
}

func TestBindingTable_SetGenericForSymbols_NilStmt(t *testing.T) {
	table := NewBindingTable()
	// Should not panic
	table.SetGenericForSymbols(nil, []cfg.SymbolID{cfg.NextSymbolID()})
}

func TestBindingTable_AllSymbols(t *testing.T) {
	table := NewBindingTable()

	// Empty table
	if got := table.AllSymbols(); len(got) != 0 {
		t.Errorf("AllSymbols should return empty for new table, got %d", len(got))
	}

	// Add various symbols
	sym1 := cfg.NextSymbolID()
	sym2 := cfg.NextSymbolID()
	sym3 := cfg.NextSymbolID()
	sym4 := cfg.NextSymbolID()
	sym5 := cfg.NextSymbolID()

	ident := &ast.IdentExpr{Value: "x"}
	table.Bind(ident, sym1)

	table.SetKind(sym2, cfg.SymbolGlobal)

	fn := &ast.FunctionExpr{}
	table.SetParamSymbols(fn, []cfg.SymbolID{sym3})

	localStmt := &ast.LocalAssignStmt{}
	table.SetLocalSymbols(localStmt, []cfg.SymbolID{sym4})

	numFor := &ast.NumberForStmt{}
	table.SetNumForSymbol(numFor, sym5)

	allSyms := table.AllSymbols()
	if len(allSyms) != 5 {
		t.Errorf("AllSymbols returned %d symbols, want 5", len(allSyms))
	}

	// Check all symbols are present
	symSet := make(map[cfg.SymbolID]bool)
	for _, s := range allSyms {
		symSet[s] = true
	}
	for _, expected := range []cfg.SymbolID{sym1, sym2, sym3, sym4, sym5} {
		if !symSet[expected] {
			t.Errorf("AllSymbols missing symbol %v", expected)
		}
	}
}

func TestBindingTable_AllSymbols_SkipsZero(t *testing.T) {
	table := NewBindingTable()

	// Bind with zero symbol (should be skipped)
	ident := &ast.IdentExpr{Value: "x"}
	table.Bind(ident, 0)

	if got := table.AllSymbols(); len(got) != 0 {
		t.Errorf("AllSymbols should skip zero symbols, got %d", len(got))
	}
}

func TestBindingTable_AllSymbols_NoDuplicates(t *testing.T) {
	table := NewBindingTable()
	sym := cfg.NextSymbolID()

	// Same symbol in multiple places
	ident := &ast.IdentExpr{Value: "x"}
	table.Bind(ident, sym)
	table.SetKind(sym, cfg.SymbolLocal)
	table.SetName(sym, "x")

	allSyms := table.AllSymbols()
	if len(allSyms) != 1 {
		t.Errorf("AllSymbols should deduplicate, got %d", len(allSyms))
	}
}

func TestBindingTable_SymbolsByName(t *testing.T) {
	table := NewBindingTable()
	alpha1 := cfg.NextSymbolID()
	alpha2 := cfg.NextSymbolID()
	beta := cfg.NextSymbolID()

	table.SetName(alpha1, "collect")
	table.SetName(alpha2, "collect")
	table.SetName(beta, "other")

	got := table.SymbolsByName("collect")
	if len(got) != 2 {
		t.Fatalf("SymbolsByName(\"collect\") len = %d, want 2", len(got))
	}
	if got[0] != alpha1 || got[1] != alpha2 {
		t.Fatalf("SymbolsByName(\"collect\") = %v, want [%d %d]", got, alpha1, alpha2)
	}
}

func TestBindingTable_SymbolsByName_UnknownOrEmpty(t *testing.T) {
	table := NewBindingTable()
	if got := table.SymbolsByName(""); got != nil {
		t.Fatalf("SymbolsByName(\"\") = %v, want nil", got)
	}
	if got := table.SymbolsByName("missing"); len(got) != 0 {
		t.Fatalf("SymbolsByName(\"missing\") = %v, want empty", got)
	}
}

func TestBindingTable_SymbolsByNameReadOnly_TracksUpdatesAndCopyIsolation(t *testing.T) {
	table := NewBindingTable()
	alpha := cfg.NextSymbolID()
	beta := cfg.NextSymbolID()

	table.SetName(alpha, "collect")
	table.SetName(beta, "collect")

	got := table.SymbolsByNameReadOnly("collect")
	if len(got) != 2 || got[0] != alpha || got[1] != beta {
		t.Fatalf("SymbolsByNameReadOnly(\"collect\") = %v, want [%d %d]", got, alpha, beta)
	}

	copied := table.SymbolsByName("collect")
	copied[0] = beta
	got = table.SymbolsByNameReadOnly("collect")
	if len(got) != 2 || got[0] != alpha || got[1] != beta {
		t.Fatalf("SymbolsByName copy should not mutate stored index, got %v", got)
	}

	table.SetName(beta, "other")
	got = table.SymbolsByNameReadOnly("collect")
	if len(got) != 1 || got[0] != alpha {
		t.Fatalf("SymbolsByNameReadOnly(\"collect\") after rename = %v, want [%d]", got, alpha)
	}
	other := table.SymbolsByNameReadOnly("other")
	if len(other) != 1 || other[0] != beta {
		t.Fatalf("SymbolsByNameReadOnly(\"other\") after rename = %v, want [%d]", other, beta)
	}
}

func TestBindingTable_Globals(t *testing.T) {
	table := NewBindingTable()

	// Empty table
	if got := table.Globals(); len(got) != 0 {
		t.Errorf("Globals should return empty for new table, got %d", len(got))
	}

	// Add globals and non-globals
	globalSym := cfg.NextSymbolID()
	localSym := cfg.NextSymbolID()
	paramSym := cfg.NextSymbolID()

	table.SetKind(globalSym, cfg.SymbolGlobal)
	table.SetKind(localSym, cfg.SymbolLocal)
	table.SetKind(paramSym, cfg.SymbolParam)

	globals := table.Globals()
	if len(globals) != 1 {
		t.Errorf("Globals returned %d symbols, want 1", len(globals))
	}
	if globals[0] != globalSym {
		t.Error("Globals should only return global symbols")
	}
}

func TestBindingTable_GetOrCreateFieldSymbol(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()
	table.SetName(baseSym, "M")

	// First call creates new symbol
	fieldSym := table.GetOrCreateFieldSymbol(baseSym, "f")
	if fieldSym == 0 {
		t.Error("GetOrCreateFieldSymbol returned zero symbol")
	}
	if got := table.Name(fieldSym); got != "M.f" {
		t.Errorf("field symbol name = %q, want %q", got, "M.f")
	}
	kind, ok := table.Kind(fieldSym)
	if !ok || kind != cfg.SymbolLocal {
		t.Error("field symbol should have kind SymbolLocal")
	}

	// Second call returns same symbol
	fieldSym2 := table.GetOrCreateFieldSymbol(baseSym, "f")
	if fieldSym2 != fieldSym {
		t.Error("GetOrCreateFieldSymbol should return same symbol for same path")
	}

	// Different path creates different symbol
	fieldSym3 := table.GetOrCreateFieldSymbol(baseSym, "g")
	if fieldSym3 == fieldSym {
		t.Error("different path should create different symbol")
	}
}

func TestBindingTable_GetOrCreateFieldSymbol_NoBaseName(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()
	// Don't set a name for baseSym

	fieldSym := table.GetOrCreateFieldSymbol(baseSym, "f")
	if got := table.Name(fieldSym); got != "f" {
		t.Errorf("field symbol name = %q, want %q", got, "f")
	}
}

func TestBindingTable_FieldSymbol(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()

	// Before creation
	if _, ok := table.FieldSymbol(baseSym, "f"); ok {
		t.Error("FieldSymbol should return false for non-existent path")
	}

	// After creation
	created := table.GetOrCreateFieldSymbol(baseSym, "f")
	got, ok := table.FieldSymbol(baseSym, "f")
	if !ok {
		t.Error("FieldSymbol should return true after creation")
	}
	if got != created {
		t.Error("FieldSymbol should return the created symbol")
	}
}

func TestBindingTable_FieldSymbol_CanonicalPathAvoidsDotCollisions(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()
	table.SetName(baseSym, "T")

	dotSym := table.GetOrCreateFieldSymbol(baseSym, "a.b")
	if dotSym == 0 {
		t.Fatal("expected symbol for dotted path")
	}

	indexKey, ok := FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "a.b"},
	})
	if !ok {
		t.Fatal("expected canonical index-string key")
	}

	indexSym := table.GetOrCreateFieldSymbol(baseSym, indexKey)
	if indexSym == 0 {
		t.Fatal("expected symbol for index-string path")
	}

	if dotSym == indexSym {
		t.Fatalf("collision: dotted path and index-string path share symbol %d", dotSym)
	}

	if got, ok := table.FieldSymbol(baseSym, "a.b"); !ok || got != dotSym {
		t.Fatalf("FieldSymbol(base, \"a.b\") = (%d,%v), want (%d,true)", got, ok, dotSym)
	}
	if got, ok := table.FieldSymbol(baseSym, indexKey); !ok || got != indexSym {
		t.Fatalf("FieldSymbol(base, %q) = (%d,%v), want (%d,true)", indexKey, got, ok, indexSym)
	}
}

func TestBindingTable_FieldSymbol_CanonicalPathDistinguishesStringAndIntIndex(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()
	table.SetName(baseSym, "T")

	stringIndexKey, ok := FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "1"},
	})
	if !ok {
		t.Fatal("expected canonical string-index key")
	}

	intIndexKey, ok := FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexInt, Index: 1},
	})
	if !ok {
		t.Fatal("expected canonical int-index key")
	}

	stringSym := table.GetOrCreateFieldSymbol(baseSym, stringIndexKey)
	intSym := table.GetOrCreateFieldSymbol(baseSym, intIndexKey)

	if stringSym == 0 || intSym == 0 {
		t.Fatal("expected both symbols to be created")
	}
	if stringSym == intSym {
		t.Fatalf("collision: string index and int index share symbol %d", stringSym)
	}
}

func TestBindingTable_FieldSymbol_NormalizesLegacyBracketStringKey(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()
	table.SetName(baseSym, "T")

	canonicalKey, ok := FieldPathKeyFromSegments([]constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "k"},
	})
	if !ok {
		t.Fatal("expected canonical key")
	}

	created := table.GetOrCreateFieldSymbol(baseSym, canonicalKey)
	if created == 0 {
		t.Fatal("expected symbol creation")
	}

	gotLegacy, ok := table.FieldSymbol(baseSym, "[k]")
	if !ok {
		t.Fatal("expected legacy bracket string lookup to resolve")
	}
	if gotLegacy != created {
		t.Fatalf("legacy lookup returned %d, want %d", gotLegacy, created)
	}
}

func TestBindingTable_GetOrCreateFieldSymbol_InvalidPathRejected(t *testing.T) {
	table := NewBindingTable()
	baseSym := cfg.NextSymbolID()

	if sym := table.GetOrCreateFieldSymbol(baseSym, ".a["); sym != 0 {
		t.Fatalf("invalid canonical path should be rejected, got symbol %d", sym)
	}
	if sym := table.GetOrCreateFieldSymbol(baseSym, ""); sym != 0 {
		t.Fatalf("empty path should be rejected, got symbol %d", sym)
	}
}

func TestBindingTable_GetOrCreateFuncLitSymbol(t *testing.T) {
	table := NewBindingTable()
	fn := &ast.FunctionExpr{}

	// First call creates new symbol
	sym := table.GetOrCreateFuncLitSymbol(fn)
	if sym == 0 {
		t.Error("GetOrCreateFuncLitSymbol returned zero symbol")
	}
	kind, ok := table.Kind(sym)
	if !ok || kind != cfg.SymbolLocal {
		t.Error("func lit symbol should have kind SymbolLocal")
	}

	// Second call returns same symbol
	sym2 := table.GetOrCreateFuncLitSymbol(fn)
	if sym2 != sym {
		t.Error("GetOrCreateFuncLitSymbol should return same symbol for same function")
	}
}

func TestBindingTable_GetOrCreateFuncLitSymbol_Nil(t *testing.T) {
	table := NewBindingTable()
	sym := table.GetOrCreateFuncLitSymbol(nil)
	if sym != 0 {
		t.Error("GetOrCreateFuncLitSymbol(nil) should return 0")
	}
}

func TestBindingTable_SetFuncLitSymbol(t *testing.T) {
	table := NewBindingTable()
	fn := &ast.FunctionExpr{}
	sym := cfg.NextSymbolID()

	table.SetFuncLitSymbol(fn, sym)
	got, ok := table.FuncLitSymbol(fn)
	if !ok {
		t.Error("FuncLitSymbol should return true after SetFuncLitSymbol")
	}
	if got != sym {
		t.Error("FuncLitSymbol should return the set symbol")
	}
}

func TestBindingTable_SetFuncLitSymbol_Nil(t *testing.T) {
	table := NewBindingTable()
	// Should not panic
	table.SetFuncLitSymbol(nil, cfg.NextSymbolID())
}

func TestBindingTable_FuncLitSymbol(t *testing.T) {
	table := NewBindingTable()
	fn := &ast.FunctionExpr{}

	// Before setting
	if _, ok := table.FuncLitSymbol(fn); ok {
		t.Error("FuncLitSymbol should return false for unset function")
	}

	// Nil function
	if _, ok := table.FuncLitSymbol(nil); ok {
		t.Error("FuncLitSymbol should return false for nil function")
	}
}

func TestBindingTable_CapturedSymbols(t *testing.T) {
	// Build: function(x) local y = 1; return function() return x + y + z end end
	// Inner function references x, y, z - all are "captured" (referenced but not declared inside)
	table := NewBindingTable()

	innerFn := &ast.FunctionExpr{}

	xSym := cfg.NextSymbolID()
	ySym := cfg.NextSymbolID()
	zSym := cfg.NextSymbolID()

	table.SetKind(xSym, cfg.SymbolParam)
	table.SetName(xSym, "x")
	table.SetKind(ySym, cfg.SymbolLocal)
	table.SetName(ySym, "y")
	table.SetKind(zSym, cfg.SymbolGlobal)
	table.SetName(zSym, "z")

	// Bind identifiers in inner function
	xIdent := &ast.IdentExpr{Value: "x"}
	yIdent := &ast.IdentExpr{Value: "y"}
	zIdent := &ast.IdentExpr{Value: "z"}

	table.Bind(xIdent, xSym)
	table.Bind(yIdent, ySym)
	table.Bind(zIdent, zSym)

	// Set up inner function structure
	innerFn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{
			Exprs: []ast.Expr{
				&ast.ArithmeticOpExpr{
					Operator: "+",
					Lhs:      xIdent,
					Rhs: &ast.ArithmeticOpExpr{
						Operator: "+",
						Lhs:      yIdent,
						Rhs:      zIdent,
					},
				},
			},
		},
	}

	captured := table.CapturedSymbols(innerFn)

	// All referenced symbols not declared inside are captured
	capturedSet := make(map[cfg.SymbolID]bool)
	for _, sym := range captured {
		capturedSet[sym] = true
	}

	if !capturedSet[xSym] {
		t.Error("should capture x")
	}
	if !capturedSet[ySym] {
		t.Error("should capture y")
	}
	if !capturedSet[zSym] {
		t.Error("should capture z (globals are also captured)")
	}
}

func TestBindingTable_CapturedSymbols_NilFn(t *testing.T) {
	table := NewBindingTable()
	if got := table.CapturedSymbols(nil); got != nil {
		t.Errorf("CapturedSymbols(nil) should return nil, got %v", got)
	}
}

func TestBindingTable_CapturedSymbols_NoCaptures(t *testing.T) {
	table := NewBindingTable()
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		},
	}

	captured := table.CapturedSymbols(fn)
	if len(captured) != 0 {
		t.Errorf("expected no captures, got %d", len(captured))
	}
}

func TestBindingTable_AllSymbols_IncludesGenericFor(t *testing.T) {
	table := NewBindingTable()
	stmt := &ast.GenericForStmt{Names: []string{"k", "v"}}
	syms := []cfg.SymbolID{cfg.NextSymbolID(), cfg.NextSymbolID()}

	table.SetGenericForSymbols(stmt, syms)

	allSyms := table.AllSymbols()
	symSet := make(map[cfg.SymbolID]bool)
	for _, s := range allSyms {
		symSet[s] = true
	}

	for _, expected := range syms {
		if !symSet[expected] {
			t.Errorf("AllSymbols missing generic for symbol %v", expected)
		}
	}
}
