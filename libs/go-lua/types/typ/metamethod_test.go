package typ

import "testing"

func TestIsMetamethod(t *testing.T) {
	valid := []string{
		"__index", "__newindex", "__call",
		"__add", "__sub", "__mul", "__div",
		"__eq", "__lt", "__le",
		"__tostring", "__len",
	}
	for _, name := range valid {
		if !IsMetamethod(name) {
			t.Errorf("IsMetamethod(%q) should be true", name)
		}
	}

	invalid := []string{"index", "__foo", "add", ""}
	for _, name := range invalid {
		if IsMetamethod(name) {
			t.Errorf("IsMetamethod(%q) should be false", name)
		}
	}
}

func TestIsBinaryMetamethod(t *testing.T) {
	binary := []Metamethod{MetaAdd, MetaSub, MetaMul, MetaDiv, MetaConcat, MetaEq}
	for _, m := range binary {
		if !IsBinaryMetamethod(m) {
			t.Errorf("IsBinaryMetamethod(%s) should be true", m)
		}
	}

	notBinary := []Metamethod{MetaUnm, MetaLen, MetaIndex, MetaCall}
	for _, m := range notBinary {
		if IsBinaryMetamethod(m) {
			t.Errorf("IsBinaryMetamethod(%s) should be false", m)
		}
	}
}

func TestIsUnaryMetamethod(t *testing.T) {
	unary := []Metamethod{MetaUnm, MetaBnot, MetaLen}
	for _, m := range unary {
		if !IsUnaryMetamethod(m) {
			t.Errorf("IsUnaryMetamethod(%s) should be true", m)
		}
	}

	notUnary := []Metamethod{MetaAdd, MetaIndex, MetaCall}
	for _, m := range notUnary {
		if IsUnaryMetamethod(m) {
			t.Errorf("IsUnaryMetamethod(%s) should be false", m)
		}
	}
}

func TestIsComparisonMetamethod(t *testing.T) {
	cmp := []Metamethod{MetaEq, MetaLt, MetaLe}
	for _, m := range cmp {
		if !IsComparisonMetamethod(m) {
			t.Errorf("IsComparisonMetamethod(%s) should be true", m)
		}
	}

	notCmp := []Metamethod{MetaAdd, MetaLen, MetaIndex}
	for _, m := range notCmp {
		if IsComparisonMetamethod(m) {
			t.Errorf("IsComparisonMetamethod(%s) should be false", m)
		}
	}
}

func TestOperatorToMetamethod(t *testing.T) {
	cases := []struct {
		op   string
		want Metamethod
		ok   bool
	}{
		{"+", MetaAdd, true},
		{"-", MetaSub, true},
		{"*", MetaMul, true},
		{"/", MetaDiv, true},
		{"%", MetaMod, true},
		{"^", MetaPow, true},
		{"//", MetaIDiv, true},
		{"..", MetaConcat, true},
		{"==", MetaEq, true},
		{"<", MetaLt, true},
		{"<=", MetaLe, true},
		{"&", MetaBand, true},
		{"|", MetaBor, true},
		{"~", MetaBxor, true},
		{"<<", MetaShl, true},
		{">>", MetaShr, true},
		{"!", "", false},
		{"&&", "", false},
	}

	for _, tc := range cases {
		got, ok := OperatorToMetamethod(tc.op)
		if ok != tc.ok {
			t.Errorf("OperatorToMetamethod(%q) ok = %v, want %v", tc.op, ok, tc.ok)
		}

		if got != tc.want {
			t.Errorf("OperatorToMetamethod(%q) = %s, want %s", tc.op, got, tc.want)
		}
	}
}

func TestUnaryOperatorToMetamethod(t *testing.T) {
	cases := []struct {
		op   string
		want Metamethod
		ok   bool
	}{
		{"-", MetaUnm, true},
		{"~", MetaBnot, true},
		{"#", MetaLen, true},
		{"!", "", false},
		{"+", "", false},
	}

	for _, tc := range cases {
		got, ok := UnaryOperatorToMetamethod(tc.op)
		if ok != tc.ok {
			t.Errorf("UnaryOperatorToMetamethod(%q) ok = %v, want %v", tc.op, ok, tc.ok)
		}

		if got != tc.want {
			t.Errorf("UnaryOperatorToMetamethod(%q) = %s, want %s", tc.op, got, tc.want)
		}
	}
}
