package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

func TestTokenType_String(t *testing.T) {
	tests := []struct {
		typ  TokenType
		want string
	}{
		{TokNamespace, "namespace"},
		{TokFunction, "function"},
		{TokVariable, "variable"},
		{TokParameter, "parameter"},
		{TokProperty, "property"},
		{TokType, "type"},
		{TokenType(999), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("TokenType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestTokenModifier_String(t *testing.T) {
	tests := []struct {
		mod  TokenModifier
		want string
	}{
		{0, ""},
		{ModDefinition, "definition"},
		{ModReadonly, "readonly"},
		{ModDefinition | ModReadonly, "definition,readonly"},
		{ModEscapes | ModMutable, "escapes,mutable"},
		{ModDeclaration | ModDefinition | ModReadonly | ModStatic |
			ModDeprecated | ModAbstract | ModAsync | ModModification |
			ModDocumentation | ModDefaultLibrary | ModPure | ModEscapes |
			ModUnused | ModMutable,
			"declaration,definition,readonly,static,deprecated,abstract,async,modification,documentation,defaultLibrary,pure,escapes,unused,mutable"},
	}

	for _, tt := range tests {
		if got := tt.mod.String(); got != tt.want {
			t.Errorf("TokenModifier(%d).String() = %q, want %q", tt.mod, got, tt.want)
		}
	}
}

func TestDefaultLegend(t *testing.T) {
	legend := DefaultLegend()

	if len(legend.TokenTypes) == 0 {
		t.Error("expected non-empty TokenTypes")
	}
	if len(legend.TokenModifiers) == 0 {
		t.Error("expected non-empty TokenModifiers")
	}

	// Check some expected values
	hasFunction := false
	for _, tt := range legend.TokenTypes {
		if tt == "function" {
			hasFunction = true
		}
	}
	if !hasFunction {
		t.Error("expected 'function' in TokenTypes")
	}

	hasDefinition := false
	for _, tm := range legend.TokenModifiers {
		if tm == "definition" {
			hasDefinition = true
		}
	}
	if !hasDefinition {
		t.Error("expected 'definition' in TokenModifiers")
	}
}

func TestProvider_TokensFull(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() *index.SymbolIndex
		file       string
		wantCount  int
		wantTokens []struct {
			line int
			typ  TokenType
		}
	}{
		{
			name:      "nil symbols",
			setup:     func() *index.SymbolIndex { return nil },
			file:      "test.lua",
			wantCount: 0,
		},
		{
			name: "empty file",
			setup: func() *index.SymbolIndex {
				return index.NewSymbolIndex()
			},
			file:      "test.lua",
			wantCount: 0,
		},
		{
			name: "single variable",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, "")
				return idx
			},
			file:      "test.lua",
			wantCount: 1,
			wantTokens: []struct {
				line int
				typ  TokenType
			}{
				{1, TokVariable},
			},
		},
		{
			name: "function with references",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition("test.lua", "foo", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 13}, "")
				idx.AddReference("test.lua", sym, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 4})
				idx.AddReference("test.lua", sym, diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 4})
				return idx
			},
			file:      "test.lua",
			wantCount: 3, // definition + 2 references
			wantTokens: []struct {
				line int
				typ  TokenType
			}{
				{1, TokFunction},
				{5, TokFunction},
				{10, TokFunction},
			},
		},
		{
			name: "mixed symbols sorted",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "b", index.SymbolVariable, nil,
					diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 2}, "")
				idx.AddDefinition("test.lua", "a", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
				return idx
			},
			file:      "test.lua",
			wantCount: 2,
			wantTokens: []struct {
				line int
				typ  TokenType
			}{
				{1, TokVariable}, // a first (sorted by position)
				{5, TokVariable}, // b second
			},
		},
		{
			name: "same line sorted by column",
			setup: func() *index.SymbolIndex {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "second", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 20, EndLine: 1, EndCol: 26}, "")
				idx.AddDefinition("test.lua", "first", index.SymbolVariable, nil,
					diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10}, "")
				return idx
			},
			file:      "test.lua",
			wantCount: 2,
			wantTokens: []struct {
				line int
				typ  TokenType
			}{
				{1, TokVariable}, // first (col 5)
				{1, TokVariable}, // second (col 20)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := tt.setup()
			provider := NewProvider(idx)

			tokens := provider.TokensFull(tt.file)

			if len(tokens) != tt.wantCount {
				t.Errorf("TokensFull() count = %d, want %d", len(tokens), tt.wantCount)
				return
			}

			for i, want := range tt.wantTokens {
				if i >= len(tokens) {
					break
				}
				if tokens[i].Span.StartLine != want.line {
					t.Errorf("tokens[%d].Line = %d, want %d", i, tokens[i].Span.StartLine, want.line)
				}
				if tokens[i].Type != want.typ {
					t.Errorf("tokens[%d].Type = %v, want %v", i, tokens[i].Type, want.typ)
				}
			}

			// Verify tokens are sorted
			for i := 1; i < len(tokens); i++ {
				prev := tokens[i-1]
				curr := tokens[i]
				if prev.Span.StartLine > curr.Span.StartLine {
					t.Errorf("tokens not sorted by line: [%d]=%d > [%d]=%d",
						i-1, prev.Span.StartLine, i, curr.Span.StartLine)
				}
				if prev.Span.StartLine == curr.Span.StartLine &&
					prev.Span.StartCol > curr.Span.StartCol {
					t.Errorf("tokens not sorted by col on same line: [%d]=%d > [%d]=%d",
						i-1, prev.Span.StartCol, i, curr.Span.StartCol)
				}
			}
		})
	}
}

func TestProvider_TokensRange(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "a", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
	idx.AddDefinition("test.lua", "b", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 2}, "")
	idx.AddDefinition("test.lua", "c", index.SymbolVariable, nil,
		diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 2}, "")

	provider := NewProvider(idx)

	// Range covering only middle token
	tokens := provider.TokensRange("test.lua", diag.Span{
		StartLine: 3,
		StartCol:  1,
		EndLine:   7,
		EndCol:    10,
	})

	if len(tokens) != 1 {
		t.Errorf("expected 1 token in range, got %d", len(tokens))
	}
	if len(tokens) > 0 && tokens[0].Span.StartLine != 5 {
		t.Errorf("expected token at line 5, got %d", tokens[0].Span.StartLine)
	}
}

func TestProvider_TokensRange_NilSymbols(t *testing.T) {
	provider := NewProvider(nil)
	tokens := provider.TokensRange("test.lua", diag.Span{StartLine: 1, EndLine: 10})
	if tokens != nil {
		t.Error("expected nil for nil symbols")
	}
}

func TestTokenInRange(t *testing.T) {
	tests := []struct {
		name string
		tok  Token
		span diag.Span
		want bool
	}{
		{
			name: "token inside range",
			tok:  Token{Span: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 5}},
			span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 100},
			want: true,
		},
		{
			name: "token before range",
			tok:  Token{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}},
			span: diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 100},
			want: false,
		},
		{
			name: "token after range",
			tok:  Token{Span: diag.Span{StartLine: 20, StartCol: 1, EndLine: 20, EndCol: 5}},
			span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 100},
			want: false,
		},
		{
			name: "token overlaps start",
			tok:  Token{Span: diag.Span{StartLine: 3, StartCol: 1, EndLine: 7, EndCol: 5}},
			span: diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 100},
			want: true,
		},
		{
			name: "token ends at range start line, before col",
			tok:  Token{Span: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 3}},
			span: diag.Span{StartLine: 5, StartCol: 5, EndLine: 10, EndCol: 100},
			want: false,
		},
		{
			name: "token starts at range end line, after col",
			tok:  Token{Span: diag.Span{StartLine: 10, StartCol: 50, EndLine: 10, EndCol: 55}},
			span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 10},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenInRange(tt.tok, tt.span); got != tt.want {
				t.Errorf("tokenInRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_TokensFull_WithEscapedSymbol(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "escaped", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "")
	idx.MarkEscape("test.lua", "escaped", "captured")

	provider := NewProvider(idx)
	tokens := provider.TokensFull("test.lua")

	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}

	if tokens[0].Modifiers&ModEscapes == 0 {
		t.Error("expected ModEscapes modifier")
	}
	if tokens[0].Modifiers&ModDefinition == 0 {
		t.Error("expected ModDefinition modifier")
	}
}

func TestSymbolKindToTokenType(t *testing.T) {
	tests := []struct {
		kind index.SymbolKind
		want TokenType
	}{
		{index.SymbolFunction, TokFunction},
		{index.SymbolVariable, TokVariable},
		{index.SymbolParameter, TokParameter},
		{index.SymbolField, TokProperty},
		{index.SymbolType, TokType},
		{index.SymbolKind(999), TokVariable}, // unknown defaults to variable
	}

	for _, tt := range tests {
		if got := symbolKindToTokenType(tt.kind); got != tt.want {
			t.Errorf("symbolKindToTokenType(%v) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestSymbolToModifiers(t *testing.T) {
	tests := []struct {
		name         string
		sym          *index.Symbol
		isDefinition bool
		wantHas      TokenModifier
	}{
		{
			name:         "definition",
			sym:          &index.Symbol{},
			isDefinition: true,
			wantHas:      ModDefinition,
		},
		{
			name:         "reference (not definition)",
			sym:          &index.Symbol{},
			isDefinition: false,
			wantHas:      0,
		},
		{
			name:         "escapes",
			sym:          &index.Symbol{Escapes: true},
			isDefinition: false,
			wantHas:      ModEscapes,
		},
		{
			name:         "definition that escapes",
			sym:          &index.Symbol{Escapes: true},
			isDefinition: true,
			wantHas:      ModDefinition | ModEscapes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := symbolToModifiers(tt.sym, tt.isDefinition)
			if got&tt.wantHas != tt.wantHas {
				t.Errorf("symbolToModifiers() = %v, want to contain %v", got, tt.wantHas)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	tokens := []Token{
		{
			Span:      diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4},
			Type:      TokVariable,
			Modifiers: ModDefinition,
		},
		{
			Span:      diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 13},
			Type:      TokFunction,
			Modifiers: 0,
		},
		{
			Span:      diag.Span{StartLine: 3, StartCol: 5, EndLine: 3, EndCol: 8},
			Type:      TokVariable,
			Modifiers: ModEscapes,
		},
	}

	encoded := Encode(tokens)

	// Check we have 15 values (3 tokens * 5)
	if len(encoded) != 15 {
		t.Fatalf("Encode() len = %d, want 15", len(encoded))
	}

	// First token: line 0, col 0, len 3, type variable, mod definition
	if encoded[0] != 0 || encoded[1] != 0 || encoded[2] != 3 {
		t.Errorf("first token: delta=(%d,%d) len=%d", encoded[0], encoded[1], encoded[2])
	}

	// Second token: same line (delta 0), delta col 9
	if encoded[5] != 0 || encoded[6] != 9 {
		t.Errorf("second token: delta=(%d,%d)", encoded[5], encoded[6])
	}

	// Third token: delta line 2
	if encoded[10] != 2 {
		t.Errorf("third token: deltaLine=%d, want 2", encoded[10])
	}
}

func TestEncode_Empty(t *testing.T) {
	if result := Encode(nil); result != nil {
		t.Error("Encode(nil) should return nil")
	}
	if result := Encode([]Token{}); result != nil {
		t.Error("Encode([]) should return nil")
	}
}

func TestDecode(t *testing.T) {
	original := []Token{
		{
			Span:      diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4},
			Type:      TokVariable,
			Modifiers: ModDefinition,
		},
		{
			Span:      diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 13},
			Type:      TokFunction,
			Modifiers: 0,
		},
		{
			Span:      diag.Span{StartLine: 3, StartCol: 5, EndLine: 3, EndCol: 8},
			Type:      TokVariable,
			Modifiers: ModEscapes,
		},
	}

	encoded := Encode(original)
	decoded := Decode(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("Decode() len = %d, want %d", len(decoded), len(original))
	}

	for i := range original {
		if decoded[i].Span != original[i].Span {
			t.Errorf("token[%d].Span = %v, want %v", i, decoded[i].Span, original[i].Span)
		}
		if decoded[i].Type != original[i].Type {
			t.Errorf("token[%d].Type = %v, want %v", i, decoded[i].Type, original[i].Type)
		}
		if decoded[i].Modifiers != original[i].Modifiers {
			t.Errorf("token[%d].Modifiers = %v, want %v", i, decoded[i].Modifiers, original[i].Modifiers)
		}
	}
}

func TestDecode_Invalid(t *testing.T) {
	if result := Decode(nil); result != nil {
		t.Error("Decode(nil) should return nil")
	}
	if result := Decode([]uint32{}); result != nil {
		t.Error("Decode([]) should return nil")
	}
	if result := Decode([]uint32{1, 2, 3}); result != nil {
		t.Error("Decode with invalid length should return nil")
	}
}

func TestTokenLength(t *testing.T) {
	tests := []struct {
		span diag.Span
		want int
	}{
		{diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, 4},
		{diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}, 0},
		{diag.Span{StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 5}, 5}, // multiline
	}

	for _, tt := range tests {
		tok := Token{Span: tt.span}
		if got := tokenLength(tok); got != tt.want {
			t.Errorf("tokenLength(%v) = %d, want %d", tt.span, got, tt.want)
		}
	}
}
