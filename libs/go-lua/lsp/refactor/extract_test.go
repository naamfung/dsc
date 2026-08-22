package refactor

import (
	"testing"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

func TestNewExtractProvider(t *testing.T) {
	symbols := index.NewSymbolIndex()
	provider := NewExtractProvider(symbols)
	if provider.symbols != symbols {
		t.Error("expected symbols to be set")
	}
}

func TestExtractProvider_AnalyzeSelection_InvalidSpan(t *testing.T) {
	provider := NewExtractProvider(nil)
	info := provider.AnalyzeSelection("test.lua", diag.Span{})
	if info.CanExtractVariable || info.CanExtractFunction {
		t.Error("expected false for invalid span")
	}
}

func TestExtractProvider_AnalyzeSelection_NilSymbols(t *testing.T) {
	provider := NewExtractProvider(nil)
	info := provider.AnalyzeSelection("test.lua", diag.Span{
		StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10,
	})
	if !info.CanExtractVariable || !info.CanExtractFunction {
		t.Error("expected true for valid span with nil symbols")
	}
}

func TestExtractProvider_AnalyzeSelection_WithSymbols(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Define a variable outside the selection
	outerSym := symbols.AddDefinition("test.lua", "outerVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 15}, "")

	// Add reference to outer variable inside selection
	symbols.AddReference("test.lua", outerSym,
		diag.Span{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 13})

	provider := NewExtractProvider(symbols)
	info := provider.AnalyzeSelection("test.lua", diag.Span{
		StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20,
	})

	if len(info.CapturedVariables) != 1 {
		t.Errorf("expected 1 captured variable, got %d", len(info.CapturedVariables))
	}
	if len(info.CapturedVariables) > 0 && info.CapturedVariables[0] != "outerVar" {
		t.Errorf("expected 'outerVar', got %s", info.CapturedVariables[0])
	}
}

func TestExtractProvider_AnalyzeSelection_DefinedVariables(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Define a variable inside the selection
	symbols.AddDefinition("test.lua", "innerVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 15}, "")

	provider := NewExtractProvider(symbols)
	info := provider.AnalyzeSelection("test.lua", diag.Span{
		StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20,
	})

	if len(info.DefinedVariables) != 1 {
		t.Errorf("expected 1 defined variable, got %d", len(info.DefinedVariables))
	}
	if !info.CanExtractFunction {
		t.Error("should be able to extract function")
	}
	if info.CanExtractVariable {
		t.Error("should not be able to extract variable when defining variables")
	}
}

func TestExtractProvider_AnalyzeSelection_UsedAfter(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Define variable in selection
	innerSym := symbols.AddDefinition("test.lua", "result", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 13}, "")

	// Reference after selection (same line, after span)
	symbols.AddReference("test.lua", innerSym,
		diag.Span{StartLine: 5, StartCol: 25, EndLine: 5, EndCol: 31})

	// Reference after selection (different line)
	symbols.AddReference("test.lua", innerSym,
		diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 7})

	provider := NewExtractProvider(symbols)
	info := provider.AnalyzeSelection("test.lua", diag.Span{
		StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20,
	})

	if len(info.UsedAfter) != 1 {
		t.Errorf("expected 1 used after, got %d", len(info.UsedAfter))
	}
}

func TestExtractProvider_ExtractVariable_InvalidSpan(t *testing.T) {
	provider := NewExtractProvider(nil)
	_, err := provider.ExtractVariable("test.lua", diag.Span{}, "x", "1 + 2")
	if err != ErrEmptySelection {
		t.Errorf("expected ErrEmptySelection, got %v", err)
	}
}

func TestExtractProvider_ExtractVariable_InvalidName(t *testing.T) {
	provider := NewExtractProvider(nil)
	_, err := provider.ExtractVariable("test.lua",
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10},
		"function", "1 + 2") // keyword
	if err != ErrInvalidName {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestExtractProvider_ExtractVariable_CantExtract(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "x", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 8}, "")

	provider := NewExtractProvider(symbols)
	_, err := provider.ExtractVariable("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20},
		"newVar", "x + 1")
	if err != ErrInvalidSelection {
		t.Errorf("expected ErrInvalidSelection, got %v", err)
	}
}

func TestExtractProvider_ExtractVariable_Success(t *testing.T) {
	provider := NewExtractProvider(nil)
	edit, err := provider.ExtractVariable("test.lua",
		diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
		"extracted", "foo() + bar()")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
	if edit.EditCount() != 2 {
		t.Errorf("expected 2 edits (declaration + replacement), got %d", edit.EditCount())
	}
}

func TestExtractProvider_ExtractFunction_InvalidSpan(t *testing.T) {
	provider := NewExtractProvider(nil)
	_, err := provider.ExtractFunction("test.lua", diag.Span{}, "fn", "x = 1")
	if err != ErrEmptySelection {
		t.Errorf("expected ErrEmptySelection, got %v", err)
	}
}

func TestExtractProvider_ExtractFunction_InvalidName(t *testing.T) {
	provider := NewExtractProvider(nil)
	_, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 3, EndCol: 10},
		"local", "x = 1") // keyword
	if err != ErrInvalidName {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestExtractProvider_ExtractFunction_Success(t *testing.T) {
	provider := NewExtractProvider(nil)
	edit, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
		"extractedFunc", "local x = 1\nlocal y = 2")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
}

func TestExtractProvider_ExtractFunction_WithCaptures(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Outer variable
	outerSym := symbols.AddDefinition("test.lua", "config", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 13}, "")
	symbols.AddReference("test.lua", outerSym,
		diag.Span{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 11})

	provider := NewExtractProvider(symbols)
	edit, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
		"helper", "print(config)")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
}

func TestExtractProvider_ExtractFunction_WithUsedAfter(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Variable defined in selection, used after
	innerSym := symbols.AddDefinition("test.lua", "result", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 13}, "")
	symbols.AddReference("test.lua", innerSym,
		diag.Span{StartLine: 15, StartCol: 1, EndLine: 15, EndCol: 7})

	provider := NewExtractProvider(symbols)
	edit, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
		"compute", "result = calculate()")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
}

func TestExtractProvider_ExtractFunction_MultipleCaptures(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Multiple outer variables referenced in selection
	var1 := symbols.AddDefinition("test.lua", "config", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 13}, "")
	symbols.AddReference("test.lua", var1,
		diag.Span{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 11})

	var2 := symbols.AddDefinition("test.lua", "options", index.SymbolVariable, nil,
		diag.Span{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 14}, "")
	symbols.AddReference("test.lua", var2,
		diag.Span{StartLine: 6, StartCol: 5, EndLine: 6, EndCol: 12})

	provider := NewExtractProvider(symbols)
	edit, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
		"helper", "process(config, options)")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
}

func TestExtractProvider_ExtractFunction_MultipleUsedAfter(t *testing.T) {
	symbols := index.NewSymbolIndex()

	// Multiple variables defined in selection, used after
	var1 := symbols.AddDefinition("test.lua", "result1", index.SymbolVariable, nil,
		diag.Span{StartLine: 5, StartCol: 7, EndLine: 5, EndCol: 14}, "")
	symbols.AddReference("test.lua", var1,
		diag.Span{StartLine: 15, StartCol: 1, EndLine: 15, EndCol: 8})

	var2 := symbols.AddDefinition("test.lua", "result2", index.SymbolVariable, nil,
		diag.Span{StartLine: 6, StartCol: 7, EndLine: 6, EndCol: 14}, "")
	symbols.AddReference("test.lua", var2,
		diag.Span{StartLine: 16, StartCol: 1, EndLine: 16, EndCol: 8})

	provider := NewExtractProvider(symbols)
	edit, err := provider.ExtractFunction("test.lua",
		diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
		"compute", "result1, result2 = calculate()")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected non-nil edit")
	}
}

func TestSpanContainsSpan(t *testing.T) {
	tests := []struct {
		name  string
		outer diag.Span
		inner diag.Span
		want  bool
	}{
		{
			name:  "inner completely inside",
			outer: diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 100},
			inner: diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20},
			want:  true,
		},
		{
			name:  "same span",
			outer: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10},
			inner: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10},
			want:  true,
		},
		{
			name:  "inner starts before outer",
			outer: diag.Span{StartLine: 5, StartCol: 5, EndLine: 10, EndCol: 10},
			inner: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10},
			want:  false,
		},
		{
			name:  "inner starts on earlier line",
			outer: diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
			inner: diag.Span{StartLine: 4, StartCol: 1, EndLine: 5, EndCol: 10},
			want:  false,
		},
		{
			name:  "inner ends after outer",
			outer: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10},
			inner: diag.Span{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 15},
			want:  false,
		},
		{
			name:  "inner ends on later line",
			outer: diag.Span{StartLine: 5, StartCol: 1, EndLine: 10, EndCol: 10},
			inner: diag.Span{StartLine: 5, StartCol: 1, EndLine: 11, EndCol: 1},
			want:  false,
		},
		{
			name:  "invalid outer",
			outer: diag.Span{},
			inner: diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10},
			want:  false,
		},
		{
			name:  "invalid inner",
			outer: diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 10},
			inner: diag.Span{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spanContainsSpan(tt.outer, tt.inner)
			if got != tt.want {
				t.Errorf("spanContainsSpan() = %v, want %v", got, tt.want)
			}
		})
	}
}
