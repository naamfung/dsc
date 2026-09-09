package refactor

import (
	"testing"

	"github.com/wippyai/go-lua/lsp"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

func TestRenameProvider_PrepareRename(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *index.SymbolIndex
		file    string
		line    int
		col     int
		wantErr error
		want    *PrepareResult
	}{
		{
			name: "valid symbol",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "myVar", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 12}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     8,
			wantErr: nil,
			want: &PrepareResult{
				Placeholder: "myVar",
				Kind:        index.SymbolVariable,
			},
		},
		{
			name:    "nil symbols",
			setup:   func() *index.SymbolIndex { return nil },
			file:    "test.lua",
			line:    1,
			col:     1,
			wantErr: ErrNoSymbol,
		},
		{
			name: "no symbol at position",
			setup: func() *index.SymbolIndex {
				return index.NewSymbolIndex()
			},
			file:    "test.lua",
			line:    1,
			col:     1,
			wantErr: ErrNoSymbol,
		},
		{
			name: "builtin not renameable",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "print", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 6}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     3,
			wantErr: ErrNotRenameable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := tt.setup()
			provider := NewRenameProvider(idx)

			result, err := provider.PrepareRename(tt.file, tt.line, tt.col)

			if err != tt.wantErr {
				t.Errorf("PrepareRename() error = %v, want %v", err, tt.wantErr)
				return
			}

			if tt.want != nil {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.Placeholder != tt.want.Placeholder {
					t.Errorf("Placeholder = %s, want %s", result.Placeholder, tt.want.Placeholder)
				}
				if result.Kind != tt.want.Kind {
					t.Errorf("Kind = %v, want %v", result.Kind, tt.want.Kind)
				}
			}
		})
	}
}

func TestRenameProvider_Rename(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *index.SymbolIndex
		file      string
		line      int
		col       int
		newName   string
		wantErr   error
		wantEdits int
	}{
		{
			name: "rename variable with references",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition("test.lua", "oldName", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 14}, "")
				idx.AddReference("test.lua", sym, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 8})
				idx.AddReference("test.lua", sym, diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 8})
				return idx
			},
			file:      "test.lua",
			line:      1,
			col:       10,
			newName:   "newName",
			wantEdits: 3, // definition + 2 references
		},
		{
			name: "rename across files",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition("lib.lua", "helper", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 16}, "")
				idx.AddReference("main.lua", sym, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 7})
				return idx
			},
			file:      "lib.lua",
			line:      1,
			col:       12,
			newName:   "helperFunc",
			wantEdits: 2,
		},
		{
			name:    "nil symbols",
			setup:   func() *index.SymbolIndex { return nil },
			file:    "test.lua",
			line:    1,
			col:     1,
			newName: "x",
			wantErr: ErrNoSymbol,
		},
		{
			name: "invalid name - empty",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     1,
			newName: "",
			wantErr: ErrInvalidName,
		},
		{
			name: "invalid name - keyword",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     1,
			newName: "function",
			wantErr: ErrInvalidName,
		},
		{
			name: "invalid name - starts with digit",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     1,
			newName: "1invalid",
			wantErr: ErrInvalidName,
		},
		{
			name: "no symbol at position",
			setup: func() *index.SymbolIndex {
				return index.NewSymbolIndex()
			},
			file:    "test.lua",
			line:    1,
			col:     1,
			newName: "x",
			wantErr: ErrNoSymbol,
		},
		{
			name: "builtin not renameable",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "require", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     3,
			newName: "import",
			wantErr: ErrNotRenameable,
		},
		{
			name: "name conflict",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "varA", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 11}, "")
				idx.AddDefinition("test.lua", "varB", index.SymbolVariable, nil,
					diag.Span{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 11}, "")
				return idx
			},
			file:    "test.lua",
			line:    1,
			col:     8,
			newName: "varB",
			wantErr: ErrNameConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := tt.setup()
			provider := NewRenameProvider(idx)

			result, err := provider.Rename(tt.file, tt.line, tt.col, tt.newName)

			if err != tt.wantErr {
				t.Errorf("Rename() error = %v, want %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == nil {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.EditCount() != tt.wantEdits {
					t.Errorf("EditCount() = %d, want %d", result.EditCount(), tt.wantEdits)
				}
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myVar", false},
		{"valid with underscore", "_private", false},
		{"valid with numbers", "var123", false},
		{"valid underscore only", "_", false},
		{"valid mixed", "_my_Var_123", false},

		{"empty", "", true},
		{"starts with digit", "1var", true},
		{"has space", "my var", true},
		{"has dash", "my-var", true},
		{"has dot", "my.var", true},
		{"keyword and", "and", true},
		{"keyword function", "function", true},
		{"keyword local", "local", true},
		{"keyword if", "if", true},
		{"keyword nil", "nil", true},
		{"keyword true", "true", true},
		{"keyword false", "false", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidLuaIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc", true},
		{"_abc", true},
		{"abc123", true},
		{"ABC", true},
		{"_", true},
		{"__", true},
		{"a_b_c", true},

		{"", false},
		{"123", false},
		{"1abc", false},
		{"a-b", false},
		{"a b", false},
		{"a.b", false},
		{"a@b", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isValidLuaIdentifier(tt.input); got != tt.want {
				t.Errorf("isValidLuaIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsLuaKeyword(t *testing.T) {
	keywords := []string{
		"and", "break", "do", "else", "elseif", "end",
		"false", "for", "function", "goto", "if", "in",
		"local", "nil", "not", "or", "repeat", "return",
		"then", "true", "until", "while",
	}

	for _, kw := range keywords {
		if !lsp.IsLuaKeyword(kw) {
			t.Errorf("IsLuaKeyword(%q) = false, want true", kw)
		}
	}

	nonKeywords := []string{"foo", "bar", "print", "myVar", "_G"}
	for _, nk := range nonKeywords {
		if lsp.IsLuaKeyword(nk) {
			t.Errorf("IsLuaKeyword(%q) = true, want false", nk)
		}
	}
}

func TestIsRenameable(t *testing.T) {
	tests := []struct {
		name   string
		symbol *index.Symbol
		want   bool
	}{
		{
			name:   "nil symbol",
			symbol: nil,
			want:   false,
		},
		{
			name:   "regular variable",
			symbol: &index.Symbol{Name: "myVar"},
			want:   true,
		},
		{
			name:   "builtin _G",
			symbol: &index.Symbol{Name: "_G"},
			want:   false,
		},
		{
			name:   "builtin print",
			symbol: &index.Symbol{Name: "print"},
			want:   false,
		},
		{
			name:   "builtin require",
			symbol: &index.Symbol{Name: "require"},
			want:   false,
		},
		{
			name:   "builtin type",
			symbol: &index.Symbol{Name: "type"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRenameable(tt.symbol); got != tt.want {
				t.Errorf("isRenameable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenameProvider_RenameNoReferences(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "lonely", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 13}, "")

	provider := NewRenameProvider(idx)
	result, err := provider.Rename("test.lua", 1, 8, "newName")

	if err != nil {
		t.Fatalf("Rename() error: %v", err)
	}

	// Only the definition should be renamed
	if result.EditCount() != 1 {
		t.Errorf("EditCount() = %d, want 1", result.EditCount())
	}
}

func TestRenameProvider_ConflictDifferentScope(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, "")
	idx.AddDefinition("test.lua", "y", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 8}, "myFunc")

	provider := NewRenameProvider(idx)
	// Rename x to y should succeed because y is in different scope
	result, err := provider.Rename("test.lua", 1, 7, "y")

	if err != nil {
		t.Fatalf("Rename() should succeed for different scopes: %v", err)
	}

	if result.EditCount() != 1 {
		t.Errorf("EditCount() = %d, want 1", result.EditCount())
	}
}
