package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestFindTableLiteralForSymbol_NilGraph(t *testing.T) {
	tbl, point := FindTableLiteralForSymbol(nil, 1)
	if tbl != nil || point != 0 {
		t.Error("expected nil result for nil graph")
	}
}

func TestFindTableLiteralForSymbol_ZeroSymbol(t *testing.T) {
	tbl, point := FindTableLiteralForSymbol(&cfg.Graph{}, 0)
	if tbl != nil || point != 0 {
		t.Error("expected nil result for zero symbol")
	}
}

func TestFindFieldAssignmentBase_NilInputs(t *testing.T) {
	sym, tbl, point := FindFieldAssignmentBase(nil, nil, 0)
	if sym != 0 || tbl != nil || point != 0 {
		t.Error("expected zero values for nil inputs")
	}
}

func TestFindFieldAssignmentBase_NilGraph(t *testing.T) {
	sym, tbl, point := FindFieldAssignmentBase(nil, nil, 1)
	if sym != 0 || tbl != nil || point != 0 {
		t.Error("expected zero values for nil graph")
	}
}

func TestFindTableLiteralOwner_NilInputs(t *testing.T) {
	tbl, sym := FindTableLiteralOwner(nil, nil)
	if tbl != nil || sym != 0 {
		t.Error("expected nil result for nil inputs")
	}
}

func TestFindTableLiteralOwner_NilGraph(t *testing.T) {
	tbl, sym := FindTableLiteralOwner(nil, nil)
	if tbl != nil || sym != 0 {
		t.Error("expected nil result for nil graph")
	}
}
