package cfg

import "testing"

func TestBoundIdent_Valid(t *testing.T) {
	sym := SymbolID(42)
	point := Point(10)
	bound := NewBoundIdent("x", sym, point)

	if !bound.IsValid() {
		t.Error("expected valid BoundIdent")
	}
	if bound.Name() != "x" {
		t.Errorf("expected name 'x', got %q", bound.Name())
	}
	if bound.Symbol != sym {
		t.Errorf("expected symbol %d, got %d", sym, bound.Symbol)
	}
	if bound.Point != point {
		t.Errorf("expected point %d, got %d", point, bound.Point)
	}
}

func TestBoundIdent_Invalid(t *testing.T) {
	tests := []struct {
		testName string
		name     string
		symbol   SymbolID
	}{
		{"empty name", "", SymbolID(1)},
		{"zero symbol", "x", 0},
		{"both invalid", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			bound := NewBoundIdent(tt.name, tt.symbol, Point(1))
			if bound.IsValid() {
				t.Error("expected invalid BoundIdent")
			}
		})
	}
}

func TestBoundIdent_IdentityBySymbol(t *testing.T) {
	bound1 := NewBoundIdent("x", SymbolID(1), Point(1))
	bound2 := NewBoundIdent("x", SymbolID(2), Point(1))

	if bound1.Symbol == bound2.Symbol {
		t.Error("different symbols should not be equal")
	}

	bound3 := NewBoundIdent("y", SymbolID(1), Point(2))
	if bound1.Symbol != bound3.Symbol {
		t.Error("same symbol should be equal regardless of name or point")
	}
}
