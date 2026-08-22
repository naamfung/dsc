package api

import (
	"testing"
)

func TestGraphKey_Zero(t *testing.T) {
	k := GraphKey{}
	if k.GraphID != 0 || k.ParentHash != 0 {
		t.Error("zero GraphKey should have zero fields")
	}
}

func TestGraphKey_Equality(t *testing.T) {
	a := GraphKey{GraphID: 1, ParentHash: 2}
	b := GraphKey{GraphID: 1, ParentHash: 2}
	if a != b {
		t.Error("equal GraphKeys should be ==")
	}
}

func TestGraphKey_Inequality(t *testing.T) {
	a := GraphKey{GraphID: 1, ParentHash: 2}
	b := GraphKey{GraphID: 1, ParentHash: 3}
	if a == b {
		t.Error("different GraphKeys should not be ==")
	}
}

func TestSymbolKey_Zero(t *testing.T) {
	k := SymbolKey{}
	if k.Symbol != 0 || k.ParentHash != 0 {
		t.Error("zero SymbolKey should have zero fields")
	}
}

func TestSymbolKey_Equality(t *testing.T) {
	a := SymbolKey{Symbol: 1, ParentHash: 2}
	b := SymbolKey{Symbol: 1, ParentHash: 2}
	if a != b {
		t.Error("equal SymbolKeys should be ==")
	}
}

func TestFuncKey_Zero(t *testing.T) {
	k := FuncKey{}
	if k.GraphID != 0 || k.ParentHash != 0 || k.StoreRevision != 0 {
		t.Error("zero FuncKey should have zero fields")
	}
}

func TestFuncKey_Equality(t *testing.T) {
	a := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 3}
	b := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 3}
	if a != b {
		t.Error("equal FuncKeys should be ==")
	}
}

func TestFuncKey_DifferentRevision(t *testing.T) {
	a := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 3}
	b := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 4}
	if a == b {
		t.Error("FuncKeys with different revisions should not be ==")
	}
}

func TestKeyForGraph_NilGraph(t *testing.T) {
	k := KeyForGraph(nil, 123)
	if k.GraphID != 0 {
		t.Errorf("expected GraphID 0 for nil graph, got %d", k.GraphID)
	}
	if k.ParentHash != 123 {
		t.Errorf("expected ParentHash 123, got %d", k.ParentHash)
	}
}

func TestKeyForGraph_AsMapKey(t *testing.T) {
	m := make(map[GraphKey]int)
	k1 := GraphKey{GraphID: 1, ParentHash: 2}
	k2 := GraphKey{GraphID: 1, ParentHash: 2}
	m[k1] = 42
	if m[k2] != 42 {
		t.Error("GraphKey should work as map key")
	}
}

func TestFuncKey_AsMapKey(t *testing.T) {
	m := make(map[FuncKey]int)
	k1 := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 3}
	k2 := FuncKey{GraphID: 1, ParentHash: 2, StoreRevision: 3}
	m[k1] = 42
	if m[k2] != 42 {
		t.Error("FuncKey should work as map key")
	}
}

func TestCompareGraphKeys(t *testing.T) {
	cases := []struct {
		name string
		a    GraphKey
		b    GraphKey
		want int
	}{
		{
			name: "graph id smaller",
			a:    GraphKey{GraphID: 1, ParentHash: 10},
			b:    GraphKey{GraphID: 2, ParentHash: 1},
			want: -1,
		},
		{
			name: "parent hash smaller when graph id equal",
			a:    GraphKey{GraphID: 2, ParentHash: 1},
			b:    GraphKey{GraphID: 2, ParentHash: 3},
			want: -1,
		},
		{
			name: "equal",
			a:    GraphKey{GraphID: 7, ParentHash: 9},
			b:    GraphKey{GraphID: 7, ParentHash: 9},
			want: 0,
		},
	}
	for _, tc := range cases {
		got := CompareGraphKeys(tc.a, tc.b)
		switch tc.want {
		case -1:
			if got >= 0 {
				t.Fatalf("%s: CompareGraphKeys = %d, want < 0", tc.name, got)
			}
		case 0:
			if got != 0 {
				t.Fatalf("%s: CompareGraphKeys = %d, want 0", tc.name, got)
			}
		case 1:
			if got <= 0 {
				t.Fatalf("%s: CompareGraphKeys = %d, want > 0", tc.name, got)
			}
		}
	}
}

func TestSortedGraphKeys(t *testing.T) {
	m := map[GraphKey]struct{}{
		{GraphID: 2, ParentHash: 3}: {},
		{GraphID: 1, ParentHash: 9}: {},
		{GraphID: 2, ParentHash: 1}: {},
	}
	got := SortedGraphKeys(m)
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	want := []GraphKey{
		{GraphID: 1, ParentHash: 9},
		{GraphID: 2, ParentHash: 1},
		{GraphID: 2, ParentHash: 3},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
