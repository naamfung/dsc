package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestDetectConstructorPattern_NilInputs(t *testing.T) {
	classSym, selfSym := DetectConstructorPattern(nil, nil, nil, nil)
	if classSym != 0 || selfSym != 0 {
		t.Errorf("expected (0, 0) for nil inputs, got (%d, %d)", classSym, selfSym)
	}
}

func TestDetectConstructorPattern_NilGraph(t *testing.T) {
	classSym, selfSym := DetectConstructorPattern(nil, &cfg.Graph{}, nil, nil)
	if classSym != 0 || selfSym != 0 {
		t.Errorf("expected (0, 0) for nil nestedGraph, got (%d, %d)", classSym, selfSym)
	}
}

func TestFindSetmetatablePatternByName_NilGraph(t *testing.T) {
	result := findSetmetatablePatternByName(nil, "Test")
	if result != 0 {
		t.Errorf("expected 0 for nil graph, got %d", result)
	}
}

func TestIsSymbolReturned_NilGraph(t *testing.T) {
	result := isSymbolReturned(nil, 1)
	if result {
		t.Error("expected false for nil graph")
	}
}

func TestIsSymbolReturned_ZeroSymbol(t *testing.T) {
	result := isSymbolReturned(&cfg.Graph{}, 0)
	if result {
		t.Error("expected false for zero symbol")
	}
}

func TestCollectConstructorFields_NilInputs(t *testing.T) {
	result := CollectConstructorFields(nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for nil graph and zero symbol, got %v", result)
	}
}

func TestCollectConstructorFields_ZeroSymbol(t *testing.T) {
	result := CollectConstructorFields(&cfg.Graph{}, 0, nil)
	if result != nil {
		t.Errorf("expected nil for zero symbol, got %v", result)
	}
}

func TestCollectConstructorFields_NilGraph(t *testing.T) {
	result := CollectConstructorFields(nil, cfg.SymbolID(1), nil)
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}
