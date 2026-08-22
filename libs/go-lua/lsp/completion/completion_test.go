package completion

import (
	"testing"

	"github.com/wippyai/go-lua/lsp"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewProvider(t *testing.T) {
	symbols := index.NewSymbolIndex()
	provider := NewProvider(symbols)
	if provider.symbols != symbols {
		t.Error("expected symbols to be set")
	}
}

func TestProvider_Complete_NilContext(t *testing.T) {
	provider := NewProvider(nil)
	result := provider.Complete(nil)
	if result != nil {
		t.Error("expected nil for nil context")
	}
}

func TestProvider_Complete_Identifier(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "myVar", index.SymbolVariable, typ.Number,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 12}, "")
	symbols.AddDefinition("test.lua", "myFunc", index.SymbolFunction,
		typ.Func().Build(),
		diag.Span{StartLine: 3, StartCol: 10, EndLine: 3, EndCol: 16}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Line:   5,
		Col:    1,
		Kind:   ContextIdentifier,
		Prefix: "my",
	}

	items := provider.Complete(ctx)
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}

	// Check we got both symbols
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}
	if !found["myVar"] {
		t.Error("expected myVar in completions")
	}
	if !found["myFunc"] {
		t.Error("expected myFunc in completions")
	}
}

func TestProvider_Complete_IdentifierNoPrefix(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Line:   5,
		Col:    1,
		Kind:   ContextIdentifier,
		Prefix: "",
	}

	items := provider.Complete(ctx)
	// Should include symbols and keywords
	if len(items) < 20 { // 22 keywords + 1 symbol
		t.Errorf("expected many items (keywords + symbols), got %d", len(items))
	}
}

func TestProvider_Complete_IdentifierFiltered(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "alpha", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 12}, "")
	symbols.AddDefinition("test.lua", "beta", index.SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 11}, "")
	symbols.AddDefinition("test.lua", "gamma", index.SymbolVariable, nil,
		diag.Span{StartLine: 3, StartCol: 7, EndLine: 3, EndCol: 12}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextIdentifier,
		Prefix: "a", // only matches "alpha" and keywords like "and"
	}

	items := provider.Complete(ctx)
	// Should have alpha and "and" keyword
	alphaFound := false
	for _, item := range items {
		if item.Label == "alpha" {
			alphaFound = true
		}
		if item.Label == "beta" || item.Label == "gamma" {
			t.Error("beta and gamma should be filtered out")
		}
	}
	if !alphaFound {
		t.Error("expected alpha in results")
	}
}

func TestProvider_Complete_Keyword(t *testing.T) {
	provider := NewProvider(nil)
	ctx := &Context{
		File:   "test.lua",
		Line:   1,
		Col:    1,
		Kind:   ContextKeyword,
		Prefix: "func",
	}

	items := provider.Complete(ctx)
	if len(items) != 1 {
		t.Fatalf("expected 1 item for 'func' prefix, got %d", len(items))
	}
	if items[0].Label != "function" {
		t.Errorf("expected 'function', got '%s'", items[0].Label)
	}
	if items[0].Kind != KindKeyword {
		t.Errorf("expected KindKeyword, got %d", items[0].Kind)
	}
}

func TestProvider_Complete_KeywordNoPrefix(t *testing.T) {
	provider := NewProvider(nil)
	ctx := &Context{
		File:   "test.lua",
		Line:   1,
		Col:    1,
		Kind:   ContextKeyword,
		Prefix: "",
	}

	items := provider.Complete(ctx)
	if len(items) != 22 { // all Lua keywords
		t.Errorf("expected 22 keywords, got %d", len(items))
	}
}

func TestProvider_Complete_Member(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "name", index.SymbolField, typ.String,
		diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 9}, "")
	symbols.AddDefinition("test.lua", "age", index.SymbolField, typ.Number,
		diag.Span{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 8}, "")
	symbols.AddDefinition("test.lua", "helper", index.SymbolFunction, nil,
		diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 7}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Line:   5,
		Col:    5,
		Kind:   ContextMember,
		Prefix: "n",
	}

	items := provider.Complete(ctx)
	if len(items) != 1 {
		t.Fatalf("expected 1 item (only 'name' field starts with 'n'), got %d", len(items))
	}
	if items[0].Label != "name" {
		t.Errorf("expected 'name', got '%s'", items[0].Label)
	}
}

func TestProvider_Complete_MemberReceiverType(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "helper", index.SymbolField, typ.String,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7}, "")

	rec := typ.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()

	provider := NewProvider(symbols)
	ctx := &Context{
		File:         "test.lua",
		Kind:         ContextMember,
		Prefix:       "c",
		ReceiverType: rec,
	}

	items := provider.Complete(ctx)
	if len(items) != 1 {
		t.Fatalf("expected 1 item for receiver type prefix 'c', got %d", len(items))
	}
	if items[0].Label != "count" {
		t.Errorf("expected 'count', got '%s'", items[0].Label)
	}
	if items[0].Kind != KindField {
		t.Errorf("expected KindField, got %d", items[0].Kind)
	}
}

func TestProvider_Complete_LocalsPreferred(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "dup", index.SymbolVariable, typ.Number,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3}, "")
	symbols.AddDefinition("test.lua", "globalOnly", index.SymbolVariable, typ.Number,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 10}, "")

	localSym := &index.Symbol{
		Name: "dup",
		Kind: index.SymbolVariable,
		Type: typ.String,
	}
	localOnly := &index.Symbol{
		Name: "localOnly",
		Kind: index.SymbolVariable,
		Type: typ.Boolean,
	}

	provider := NewProvider(symbols)
	ctx := &Context{
		File:         "test.lua",
		Kind:         ContextIdentifier,
		Prefix:       "",
		LocalSymbols: []*index.Symbol{localSym, localOnly},
	}

	items := provider.Complete(ctx)
	found := make(map[string]Item)
	for _, item := range items {
		found[item.Label] = item
	}
	if _, ok := found["localOnly"]; !ok {
		t.Fatalf("expected localOnly in results")
	}
	if item, ok := found["dup"]; ok {
		if item.Detail != typ.String.String() {
			t.Fatalf("expected local dup to win (string), got %q", item.Detail)
		}
	} else {
		t.Fatalf("expected dup in results")
	}
}

func TestResolveMemberType(t *testing.T) {
	rec := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.String).
		Build()
	if got := ResolveMemberType(rec, "x"); got != typ.Number {
		t.Errorf("ResolveMemberType(record, \"x\") = %v, want %v", got, typ.Number)
	}
	if got := ResolveMemberType(rec, "missing"); got != nil {
		t.Errorf("ResolveMemberType(record, \"missing\") = %v, want nil", got)
	}
}

func TestProvider_Complete_MemberNoPrefix(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "x", index.SymbolField, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, "")
	symbols.AddDefinition("test.lua", "y", index.SymbolField, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 2}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextMember,
		Prefix: "",
	}

	items := provider.Complete(ctx)
	if len(items) != 2 {
		t.Errorf("expected 2 fields, got %d", len(items))
	}
}

func TestProvider_Complete_MemberNilSymbols(t *testing.T) {
	provider := NewProvider(nil)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextMember,
		Prefix: "",
	}

	items := provider.Complete(ctx)
	if items != nil {
		t.Error("expected nil for nil symbols")
	}
}

func TestProvider_Complete_IdentifierNilSymbols(t *testing.T) {
	provider := NewProvider(nil)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextIdentifier,
		Prefix: "n", // matches "nil" and "not"
	}

	items := provider.Complete(ctx)
	// Should still return keywords that match
	found := false
	for _, item := range items {
		if item.Kind == KindKeyword {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected keywords even with nil symbols")
	}
}

func TestProvider_Complete_Sorting(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "zebra", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 6}, "")
	symbols.AddDefinition("test.lua", "alpha", index.SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 6}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextUnknown, // default behavior
		Prefix: "",
	}

	items := provider.Complete(ctx)
	// Find alpha and zebra indices
	alphaIdx, zebraIdx := -1, -1
	for i, item := range items {
		if item.Label == "alpha" {
			alphaIdx = i
		}
		if item.Label == "zebra" {
			zebraIdx = i
		}
	}
	if alphaIdx == -1 || zebraIdx == -1 {
		t.Fatal("expected both alpha and zebra in results")
	}
	if alphaIdx > zebraIdx {
		t.Error("expected alpha before zebra (alphabetical sort)")
	}
}

func TestProvider_Complete_SortTextPriority(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "localVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 15}, "")

	provider := NewProvider(symbols)
	ctx := &Context{
		File:   "test.lua",
		Kind:   ContextIdentifier,
		Prefix: "l",
	}

	items := provider.Complete(ctx)
	// Should have localVar and 'local' keyword
	// Keywords have sortText "z" + name, so symbols should come first
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	// localVar should come before 'local' keyword
	localVarIdx, localKwIdx := -1, -1
	for i, item := range items {
		if item.Label == "localVar" {
			localVarIdx = i
		}
		if item.Label == "local" {
			localKwIdx = i
		}
	}
	if localVarIdx == -1 || localKwIdx == -1 {
		t.Fatal("expected both localVar and local keyword")
	}
	if localVarIdx > localKwIdx {
		t.Error("expected symbols before keywords")
	}
}

func TestSymbolToItem(t *testing.T) {
	tests := []struct {
		name     string
		sym      *index.Symbol
		wantKind Kind
	}{
		{
			name:     "function",
			sym:      &index.Symbol{Name: "foo", Kind: index.SymbolFunction},
			wantKind: KindFunction,
		},
		{
			name:     "variable",
			sym:      &index.Symbol{Name: "x", Kind: index.SymbolVariable},
			wantKind: KindVariable,
		},
		{
			name:     "parameter",
			sym:      &index.Symbol{Name: "arg", Kind: index.SymbolParameter},
			wantKind: KindVariable,
		},
		{
			name:     "field",
			sym:      &index.Symbol{Name: "name", Kind: index.SymbolField},
			wantKind: KindField,
		},
		{
			name:     "type",
			sym:      &index.Symbol{Name: "MyType", Kind: index.SymbolType},
			wantKind: KindClass,
		},
	}

	provider := NewProvider(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := provider.symbolToItem(tt.sym)
			if item.Label != tt.sym.Name {
				t.Errorf("Label = %s, want %s", item.Label, tt.sym.Name)
			}
			if item.Kind != tt.wantKind {
				t.Errorf("Kind = %d, want %d", item.Kind, tt.wantKind)
			}
		})
	}
}

func TestSymbolToItem_WithType(t *testing.T) {
	sym := &index.Symbol{
		Name: "x",
		Kind: index.SymbolVariable,
		Type: typ.Number,
	}

	provider := NewProvider(nil)
	item := provider.symbolToItem(sym)
	if item.Detail != "number" {
		t.Errorf("Detail = %s, want 'number'", item.Detail)
	}
	if item.Type == nil {
		t.Error("expected Type to be set")
	}
}

func TestSymbolKindToCompletionKind(t *testing.T) {
	tests := []struct {
		kind     index.SymbolKind
		wantKind Kind
	}{
		{index.SymbolFunction, KindFunction},
		{index.SymbolVariable, KindVariable},
		{index.SymbolParameter, KindVariable},
		{index.SymbolField, KindField},
		{index.SymbolType, KindClass},
		{index.SymbolKind(999), KindVariable},
	}

	for _, tt := range tests {
		got := symbolKindToCompletionKind(tt.kind)
		if got != tt.wantKind {
			t.Errorf("symbolKindToCompletionKind(%d) = %d, want %d", tt.kind, got, tt.wantKind)
		}
	}
}

func TestProvider_ResolveItem(t *testing.T) {
	provider := NewProvider(nil)

	// Nil item
	if provider.ResolveItem(nil) != nil {
		t.Error("expected nil for nil item")
	}

	// Non-nil item
	item := &Item{Label: "test"}
	resolved := provider.ResolveItem(item)
	if resolved != item {
		t.Error("expected same item returned")
	}
}

func TestLuaKeywords(t *testing.T) {
	if len(lsp.LuaKeywords) != 22 {
		t.Errorf("expected 22 Lua keywords, got %d", len(lsp.LuaKeywords))
	}

	expected := map[string]bool{
		"and": true, "break": true, "do": true, "else": true,
		"elseif": true, "end": true, "false": true, "for": true,
		"function": true, "goto": true, "if": true, "in": true,
		"local": true, "nil": true, "not": true, "or": true,
		"repeat": true, "return": true, "then": true, "true": true,
		"until": true, "while": true,
	}

	for _, kw := range lsp.LuaKeywords {
		if !expected[kw] {
			t.Errorf("unexpected keyword: %s", kw)
		}
	}
}
