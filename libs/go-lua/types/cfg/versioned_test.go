package cfg

import "testing"

func TestSymbolKind_Values(t *testing.T) {
	tests := []struct {
		kind SymbolKind
		want int
	}{
		{SymbolUnknown, 0},
		{SymbolParam, 1},
		{SymbolLocal, 2},
		{SymbolGlobal, 3},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("SymbolKind %v = %d, want %d", tt.kind, tt.kind, tt.want)
		}
	}
}
