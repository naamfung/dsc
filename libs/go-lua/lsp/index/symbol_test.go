package index

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestNewSymbolIndex(t *testing.T) {
	idx := NewSymbolIndex()

	if idx == nil {
		t.Fatal("NewSymbolIndex returned nil")
	}
	if idx.byFile == nil {
		t.Error("byFile map not initialized")
	}
	if idx.byName == nil {
		t.Error("byName map not initialized")
	}
	if idx.byLine == nil {
		t.Error("byLine map not initialized")
	}
	if idx.references == nil {
		t.Error("references map not initialized")
	}
}

func TestAddDefinition(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		sname string
		kind  SymbolKind
		typ   any
		span  diag.Span
		scope string
	}{
		{
			name:  "add variable",
			file:  "test.lua",
			sname: "myVar",
			kind:  SymbolVariable,
			typ:   "string",
			span:  diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10},
			scope: "",
		},
		{
			name:  "add function",
			file:  "test.lua",
			sname: "myFunc",
			kind:  SymbolFunction,
			typ:   "function",
			span:  diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 3},
			scope: "",
		},
		{
			name:  "add parameter",
			file:  "test.lua",
			sname: "param1",
			kind:  SymbolParameter,
			typ:   "number",
			span:  diag.Span{StartLine: 5, StartCol: 15, EndLine: 5, EndCol: 21},
			scope: "myFunc",
		},
		{
			name:  "add type",
			file:  "types.lua",
			sname: "MyType",
			kind:  SymbolType,
			typ:   "table",
			span:  diag.Span{StartLine: 1, StartCol: 1, EndLine: 3, EndCol: 1},
			scope: "",
		},
		{
			name:  "add field",
			file:  "types.lua",
			sname: "field1",
			kind:  SymbolField,
			typ:   "string",
			span:  diag.Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 9},
			scope: "MyType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := NewSymbolIndex()
			sym := idx.AddDefinition(tt.file, tt.sname, tt.kind, tt.typ, tt.span, tt.scope)

			if sym == nil {
				t.Fatal("AddDefinition returned nil")
			}
			if sym.Name != tt.sname {
				t.Errorf("expected name %q, got %q", tt.sname, sym.Name)
			}
			if sym.Kind != tt.kind {
				t.Errorf("expected kind %v, got %v", tt.kind, sym.Kind)
			}
			if sym.Type != tt.typ {
				t.Errorf("expected type %v, got %v", tt.typ, sym.Type)
			}
			if sym.File != tt.file {
				t.Errorf("expected file %q, got %q", tt.file, sym.File)
			}
			if sym.Scope != tt.scope {
				t.Errorf("expected scope %q, got %q", tt.scope, sym.Scope)
			}
			if sym.DefSpan != tt.span {
				t.Errorf("expected span %v, got %v", tt.span, sym.DefSpan)
			}

			// Verify it's in byFile
			syms := idx.SymbolsInFile(tt.file)
			if len(syms) != 1 {
				t.Errorf("expected 1 symbol in file, got %d", len(syms))
			}

			// Verify it's in byName
			found := idx.LookupByName(tt.file, tt.sname)
			if found != sym {
				t.Error("symbol not found by name")
			}

			// Verify it's in byLine
			foundByLine := idx.SymbolAt(tt.file, tt.span.StartLine, tt.span.StartCol)
			if foundByLine != sym {
				t.Error("symbol not found at position")
			}
		})
	}
}

func TestAddDefinition_MultipleSymbols(t *testing.T) {
	idx := NewSymbolIndex()

	sym1 := idx.AddDefinition("test.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	sym2 := idx.AddDefinition("test.lua", "var2", SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}, "")
	sym3 := idx.AddDefinition("other.lua", "var3", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")

	syms := idx.SymbolsInFile("test.lua")
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols in test.lua, got %d", len(syms))
	}

	syms = idx.SymbolsInFile("other.lua")
	if len(syms) != 1 {
		t.Errorf("expected 1 symbol in other.lua, got %d", len(syms))
	}

	if idx.LookupByName("test.lua", "var1") != sym1 {
		t.Error("var1 not found")
	}
	if idx.LookupByName("test.lua", "var2") != sym2 {
		t.Error("var2 not found")
	}
	if idx.LookupByName("other.lua", "var3") != sym3 {
		t.Error("var3 not found")
	}
}

func TestAddDefinition_SameLineDifferentSymbols(t *testing.T) {
	idx := NewSymbolIndex()

	sym1 := idx.AddDefinition("test.lua", "a", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}, "")
	sym2 := idx.AddDefinition("test.lua", "b", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 5}, "")

	// Both should be findable
	found1 := idx.SymbolAt("test.lua", 1, 1)
	if found1 != sym1 {
		t.Error("sym1 not found at position")
	}

	found2 := idx.SymbolAt("test.lua", 1, 5)
	if found2 != sym2 {
		t.Error("sym2 not found at position")
	}
}

func TestAddReference(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("def.lua", "myVar", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

	// Add references
	idx.AddReference("use1.lua", sym, diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 14})
	idx.AddReference("use2.lua", sym, diag.Span{StartLine: 3, StartCol: 2, EndLine: 3, EndCol: 6})
	idx.AddReference("use1.lua", sym, diag.Span{StartLine: 8, StartCol: 5, EndLine: 8, EndCol: 9})

	refs := idx.ReferencesTo(sym)
	if len(refs) != 3 {
		t.Errorf("expected 3 references, got %d", len(refs))
	}

	// Verify reference details
	if refs[0].Symbol != sym {
		t.Error("reference[0] symbol mismatch")
	}
	if refs[0].File != "use1.lua" {
		t.Errorf("expected file 'use1.lua', got %q", refs[0].File)
	}
	if refs[1].File != "use2.lua" {
		t.Errorf("expected file 'use2.lua', got %q", refs[1].File)
	}
	if refs[2].File != "use1.lua" {
		t.Errorf("expected file 'use1.lua', got %q", refs[2].File)
	}
}

func TestAddReference_NilSymbol(t *testing.T) {
	idx := NewSymbolIndex()

	// Should not panic or error
	idx.AddReference("test.lua", nil, diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1})

	// Should have no effect
	refs := idx.ReferencesTo(nil)
	if len(refs) != 0 {
		t.Errorf("expected 0 references to nil, got %d", len(refs))
	}
}

func TestSymbolAt(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("test.lua", "myVar", SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 14}, "")

	tests := []struct {
		name     string
		file     string
		line     int
		col      int
		expected *Symbol
	}{
		{"exact start", "test.lua", 5, 10, sym},
		{"within span", "test.lua", 5, 12, sym},
		{"exact end", "test.lua", 5, 14, sym},
		{"before span", "test.lua", 5, 9, nil},
		{"after span", "test.lua", 5, 15, nil},
		{"wrong line", "test.lua", 4, 10, nil},
		{"wrong file", "other.lua", 5, 10, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := idx.SymbolAt(tt.file, tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSymbolAt_MultilineSpan(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("test.lua", "func", SymbolFunction, nil,
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 3}, "")

	tests := []struct {
		name     string
		line     int
		col      int
		expected *Symbol
	}{
		{"start line, start col", 5, 1, sym},
		{"start line, after start col", 5, 5, sym},
		{"start line, before start col", 5, 0, nil},
		{"before start line", 4, 1, nil},
		{"after end line", 11, 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := idx.SymbolAt("test.lua", tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSymbolAt_Reference(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("def.lua", "myVar", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

	idx.AddReference("use.lua", sym, diag.Span{StartLine: 10, StartCol: 5, EndLine: 10, EndCol: 9})

	// Should find symbol through reference
	result := idx.SymbolAt("use.lua", 10, 7)
	if result != sym {
		t.Error("expected to find symbol through reference")
	}

	// Outside reference span should return nil
	result = idx.SymbolAt("use.lua", 10, 4)
	if result != nil {
		t.Error("expected nil outside reference span")
	}
}

func TestSymbolAt_InvalidSpan(t *testing.T) {
	idx := NewSymbolIndex()

	// Add symbol with invalid span (StartLine = 0)
	idx.AddDefinition("test.lua", "invalid", SymbolVariable, nil,
		diag.Span{StartLine: 0, StartCol: 0, EndLine: 0, EndCol: 0}, "")

	result := idx.SymbolAt("test.lua", 0, 0)
	if result != nil {
		t.Error("expected nil for invalid span")
	}
}

func TestDefinitionOf(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("def.lua", "myFunc", SymbolFunction, nil,
		diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 15}, "")

	idx.AddReference("use.lua", sym, diag.Span{StartLine: 20, StartCol: 3, EndLine: 20, EndCol: 8})

	// On definition
	def := idx.DefinitionOf("def.lua", 5, 12)
	if def != sym {
		t.Error("expected to find definition at definition site")
	}

	// On reference
	def = idx.DefinitionOf("use.lua", 20, 5)
	if def != sym {
		t.Error("expected to find definition from reference")
	}

	// Not found
	def = idx.DefinitionOf("other.lua", 1, 1)
	if def != nil {
		t.Error("expected nil for non-existent position")
	}
}

func TestReferencesTo(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("def.lua", "myVar", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

	// No references initially
	refs := idx.ReferencesTo(sym)
	if len(refs) != 0 {
		t.Errorf("expected 0 references, got %d", len(refs))
	}

	// Add references
	idx.AddReference("use1.lua", sym, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 5})
	idx.AddReference("use2.lua", sym, diag.Span{StartLine: 3, StartCol: 10, EndLine: 3, EndCol: 14})

	refs = idx.ReferencesTo(sym)
	if len(refs) != 2 {
		t.Errorf("expected 2 references, got %d", len(refs))
	}

	// Verify returned slice is a copy (mutation safe)
	refs[0] = nil
	refs2 := idx.ReferencesTo(sym)
	if refs2[0] == nil {
		t.Error("expected independent copy of references")
	}
}

func TestReferencesTo_NilSymbol(t *testing.T) {
	idx := NewSymbolIndex()

	refs := idx.ReferencesTo(nil)
	if refs != nil {
		t.Error("expected nil for nil symbol")
	}
}

func TestLookupByName(t *testing.T) {
	idx := NewSymbolIndex()

	sym1 := idx.AddDefinition("test.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	sym2 := idx.AddDefinition("test.lua", "var2", SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}, "")
	sym3 := idx.AddDefinition("other.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")

	tests := []struct {
		name     string
		file     string
		sname    string
		expected *Symbol
	}{
		{"find var1 in test.lua", "test.lua", "var1", sym1},
		{"find var2 in test.lua", "test.lua", "var2", sym2},
		{"find var1 in other.lua", "other.lua", "var1", sym3},
		{"not found - wrong name", "test.lua", "var3", nil},
		{"not found - wrong file", "missing.lua", "var1", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := idx.LookupByName(tt.file, tt.sname)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLookupByName_Override(t *testing.T) {
	idx := NewSymbolIndex()

	// Add first definition
	idx.AddDefinition("test.lua", "var", SymbolVariable, "string",
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")

	// Add second definition with same name (override)
	sym2 := idx.AddDefinition("test.lua", "var", SymbolVariable, "number",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 3}, "")

	// Should return the latest definition
	result := idx.LookupByName("test.lua", "var")
	if result != sym2 {
		t.Error("expected latest definition")
	}
	if result.Type != "number" {
		t.Errorf("expected type 'number', got %v", result.Type)
	}
}

func TestSymbolsInFile(t *testing.T) {
	idx := NewSymbolIndex()

	sym1 := idx.AddDefinition("test.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	sym2 := idx.AddDefinition("test.lua", "var2", SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}, "")
	idx.AddDefinition("other.lua", "var3", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")

	syms := idx.SymbolsInFile("test.lua")
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(syms))
	}

	// Verify symbols are correct
	found1, found2 := false, false
	for _, sym := range syms {
		if sym == sym1 {
			found1 = true
		}
		if sym == sym2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("expected to find both sym1 and sym2")
	}

	// Test empty file
	syms = idx.SymbolsInFile("empty.lua")
	if len(syms) != 0 {
		t.Errorf("expected 0 symbols for empty file, got %d", len(syms))
	}

	// Verify returned slice is a copy (mutation safe)
	syms = idx.SymbolsInFile("test.lua")
	syms[0] = nil
	syms2 := idx.SymbolsInFile("test.lua")
	if syms2[0] == nil {
		t.Error("expected independent copy of symbols")
	}
}

func TestInvalidateFile(t *testing.T) {
	idx := NewSymbolIndex()

	// Add symbols to multiple files
	sym1 := idx.AddDefinition("a.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	idx.AddDefinition("a.lua", "var2", SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}, "")
	sym3 := idx.AddDefinition("b.lua", "var3", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")

	// Add references
	idx.AddReference("a.lua", sym1, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 4})
	idx.AddReference("b.lua", sym1, diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 4})
	idx.AddReference("a.lua", sym3, diag.Span{StartLine: 6, StartCol: 1, EndLine: 6, EndCol: 4})

	// Invalidate a.lua
	idx.InvalidateFile("a.lua")

	// Symbols from a.lua should be gone
	if idx.LookupByName("a.lua", "var1") != nil {
		t.Error("var1 should be invalidated")
	}
	if idx.LookupByName("a.lua", "var2") != nil {
		t.Error("var2 should be invalidated")
	}

	// Symbols from b.lua should remain
	if idx.LookupByName("b.lua", "var3") != sym3 {
		t.Error("var3 should still exist")
	}

	// References to sym1 should be removed
	refs := idx.ReferencesTo(sym1)
	if len(refs) != 0 {
		t.Errorf("expected 0 references to sym1 after invalidation, got %d", len(refs))
	}

	// References to sym3 from a.lua should be removed, but sym3 itself should remain
	refs = idx.ReferencesTo(sym3)
	if len(refs) != 0 {
		t.Errorf("expected 0 references to sym3 after invalidating a.lua, got %d", len(refs))
	}

	// SymbolsInFile should return empty
	syms := idx.SymbolsInFile("a.lua")
	if len(syms) != 0 {
		t.Errorf("expected 0 symbols in a.lua after invalidation, got %d", len(syms))
	}

	// SymbolAt should not find invalidated symbols
	if idx.SymbolAt("a.lua", 1, 1) != nil {
		t.Error("expected nil after invalidation")
	}
}

func TestInvalidateFile_References(t *testing.T) {
	idx := NewSymbolIndex()

	// Define symbol in one file
	sym := idx.AddDefinition("def.lua", "myVar", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

	// Add references from multiple files
	idx.AddReference("use1.lua", sym, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 5})
	idx.AddReference("use2.lua", sym, diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 5})
	idx.AddReference("use1.lua", sym, diag.Span{StartLine: 8, StartCol: 1, EndLine: 8, EndCol: 5})

	// Invalidate use1.lua (which has references, not definitions)
	idx.InvalidateFile("use1.lua")

	// Symbol should still exist
	if idx.LookupByName("def.lua", "myVar") != sym {
		t.Error("symbol should still exist after invalidating reference file")
	}

	// Only reference from use2.lua should remain
	refs := idx.ReferencesTo(sym)
	if len(refs) != 1 {
		t.Errorf("expected 1 reference remaining, got %d", len(refs))
	}
	if refs[0].File != "use2.lua" {
		t.Errorf("expected reference from use2.lua, got %s", refs[0].File)
	}
}

func TestInvalidateFile_NonExistent(t *testing.T) {
	idx := NewSymbolIndex()

	idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")

	// Should not panic or affect other files
	idx.InvalidateFile("nonexistent.lua")

	if idx.LookupByName("test.lua", "var") == nil {
		t.Error("symbol in test.lua should still exist")
	}
}

func TestClear(t *testing.T) {
	idx := NewSymbolIndex()

	// Add various symbols and references
	sym1 := idx.AddDefinition("a.lua", "var1", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	sym2 := idx.AddDefinition("b.lua", "var2", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")

	idx.AddReference("a.lua", sym1, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 4})
	idx.AddReference("b.lua", sym2, diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 4})

	// Clear everything
	idx.Clear()

	// All lookups should return empty/nil
	if idx.LookupByName("a.lua", "var1") != nil {
		t.Error("expected nil after clear")
	}
	if idx.LookupByName("b.lua", "var2") != nil {
		t.Error("expected nil after clear")
	}

	syms := idx.SymbolsInFile("a.lua")
	if len(syms) != 0 {
		t.Errorf("expected 0 symbols after clear, got %d", len(syms))
	}

	refs := idx.ReferencesTo(sym1)
	if len(refs) != 0 {
		t.Errorf("expected 0 references after clear, got %d", len(refs))
	}

	if idx.SymbolAt("a.lua", 1, 1) != nil {
		t.Error("expected nil after clear")
	}
}

func TestClear_Reuse(t *testing.T) {
	idx := NewSymbolIndex()

	// Add symbol, clear, then add again
	idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")

	idx.Clear()

	sym := idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")

	// Should work as before
	if idx.LookupByName("test.lua", "var") != sym {
		t.Error("index should work after clear and reuse")
	}
}

func TestSpanContains(t *testing.T) {
	tests := []struct {
		name     string
		span     diag.Span
		line     int
		col      int
		expected bool
	}{
		{
			name:     "invalid span - zero start line",
			span:     diag.Span{StartLine: 0, StartCol: 1, EndLine: 1, EndCol: 5},
			line:     1,
			col:      1,
			expected: false,
		},
		{
			name:     "invalid span - zero start col",
			span:     diag.Span{StartLine: 1, StartCol: 0, EndLine: 1, EndCol: 5},
			line:     1,
			col:      1,
			expected: false,
		},
		{
			name:     "single line - at start",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     5,
			col:      10,
			expected: true,
		},
		{
			name:     "single line - in middle",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     5,
			col:      15,
			expected: true,
		},
		{
			name:     "single line - at end",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     5,
			col:      20,
			expected: true,
		},
		{
			name:     "single line - before start col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     5,
			col:      9,
			expected: false,
		},
		{
			name:     "single line - after end col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     5,
			col:      21,
			expected: false,
		},
		{
			name:     "single line - before start line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     4,
			col:      15,
			expected: false,
		},
		{
			name:     "single line - after end line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			line:     6,
			col:      15,
			expected: false,
		},
		{
			name:     "multiline - start line start col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     5,
			col:      10,
			expected: true,
		},
		{
			name:     "multiline - start line before start col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     5,
			col:      9,
			expected: false,
		},
		{
			name:     "multiline - middle line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     7,
			col:      1,
			expected: true,
		},
		{
			name:     "multiline - end line before end col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     10,
			col:      15,
			expected: true,
		},
		{
			name:     "multiline - end line at end col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     10,
			col:      20,
			expected: true,
		},
		{
			name:     "multiline - end line after end col",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     10,
			col:      21,
			expected: false,
		},
		{
			name:     "multiline - before start line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     4,
			col:      15,
			expected: false,
		},
		{
			name:     "multiline - after end line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 20},
			line:     11,
			col:      15,
			expected: false,
		},
		{
			name:     "zero end col - should allow any col on end line",
			span:     diag.Span{StartLine: 5, StartCol: 10, EndLine: 10, EndCol: 0},
			line:     10,
			col:      999,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spanContains(tt.span, tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("spanContains(%v, %d, %d) = %v, expected %v",
					tt.span, tt.line, tt.col, result, tt.expected)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	idx := NewSymbolIndex()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
				diag.Span{StartLine: i, StartCol: 1, EndLine: i, EndCol: 3}, "")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.LookupByName("test.lua", "var")
			idx.SymbolsInFile("test.lua")
			idx.SymbolAt("test.lua", 1, 1)
		}()
	}

	wg.Wait()
}

func TestConcurrentReferences(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("def.lua", "var", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")

	var wg sync.WaitGroup

	// Concurrent reference additions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx.AddReference("use.lua", sym,
				diag.Span{StartLine: i, StartCol: 1, EndLine: i, EndCol: 3})
		}(i)
	}

	// Concurrent reference reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.ReferencesTo(sym)
		}()
	}

	wg.Wait()

	// Verify all references were added
	refs := idx.ReferencesTo(sym)
	if len(refs) != 100 {
		t.Errorf("expected 100 references, got %d", len(refs))
	}
}

func TestConcurrentInvalidation(t *testing.T) {
	idx := NewSymbolIndex()
	var wg sync.WaitGroup

	// Add initial data
	for i := 0; i < 10; i++ {
		idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
			diag.Span{StartLine: i, StartCol: 1, EndLine: i, EndCol: 3}, "")
	}

	// Concurrent invalidations and operations
	for i := 0; i < 50; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			idx.InvalidateFile("test.lua")
		}()

		go func() {
			defer wg.Done()
			idx.AddDefinition("test.lua", "var", SymbolVariable, nil,
				diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")
		}()

		go func() {
			defer wg.Done()
			idx.SymbolsInFile("test.lua")
		}()
	}

	wg.Wait()
}

func TestSymbolKind(t *testing.T) {
	tests := []struct {
		kind SymbolKind
		name string
	}{
		{SymbolVariable, "variable"},
		{SymbolFunction, "function"},
		{SymbolType, "type"},
		{SymbolField, "field"},
		{SymbolParameter, "parameter"},
	}

	idx := NewSymbolIndex()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sym := idx.AddDefinition("test.lua", tt.name, tt.kind, nil,
				diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

			if sym.Kind != tt.kind {
				t.Errorf("expected kind %v, got %v", tt.kind, sym.Kind)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	idx := NewSymbolIndex()

	idx.AddDefinition("a.lua", "getUser", SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "")
	idx.AddDefinition("a.lua", "getUserById", SymbolFunction, nil,
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 12}, "")
	idx.AddDefinition("b.lua", "setUser", SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "")
	idx.AddDefinition("b.lua", "config", SymbolVariable, nil,
		diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 7}, "")

	// Search for "user" - should find 3 results
	results := idx.Search("user")
	if len(results) != 3 {
		t.Errorf("expected 3 results for 'user', got %d", len(results))
	}

	// Search for "get" - should find 2 results
	results = idx.Search("get")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'get', got %d", len(results))
	}

	// Search for "config" - should find 1 result
	results = idx.Search("config")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'config', got %d", len(results))
	}

	// Search for non-existent - should find 0 results
	results = idx.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'nonexistent', got %d", len(results))
	}

	// Search is case-insensitive
	results = idx.Search("USER")
	if len(results) != 3 {
		t.Errorf("expected 3 results for 'USER' (case-insensitive), got %d", len(results))
	}
}

func TestSearch_EmptyIndex(t *testing.T) {
	idx := NewSymbolIndex()
	results := idx.Search("anything")
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestMarkEscape(t *testing.T) {
	idx := NewSymbolIndex()

	sym := idx.AddDefinition("test.lua", "captured", SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 15}, "")

	// Initially not escaped
	if sym.Escapes {
		t.Error("expected symbol not to escape initially")
	}

	// Mark as escaped
	idx.MarkEscape("test.lua", "captured", "returned from function")

	// Verify escape was marked
	if !sym.Escapes {
		t.Error("expected symbol to be marked as escaped")
	}
	if sym.EscapeReason != "returned from function" {
		t.Errorf("expected escape reason 'returned from function', got %q", sym.EscapeReason)
	}
}

func TestMarkEscape_NonExistent(t *testing.T) {
	idx := NewSymbolIndex()

	// Should not panic
	idx.MarkEscape("test.lua", "nonexistent", "reason")
	idx.MarkEscape("nonexistent.lua", "var", "reason")
}

func TestWorkspaceSymbol(t *testing.T) {
	idx := NewSymbolIndex()

	idx.AddDefinition("test.lua", "myFunc", SymbolFunction, nil,
		diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 16}, "")

	results := idx.Search("myFunc")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	ws := results[0]
	if ws.Name != "myFunc" {
		t.Errorf("expected name 'myFunc', got %q", ws.Name)
	}
	if ws.Kind != SymbolFunction {
		t.Errorf("expected kind SymbolFunction, got %v", ws.Kind)
	}
	if ws.File != "test.lua" {
		t.Errorf("expected file 'test.lua', got %q", ws.File)
	}
	if ws.Span.StartLine != 5 {
		t.Errorf("expected span start line 5, got %d", ws.Span.StartLine)
	}
}
