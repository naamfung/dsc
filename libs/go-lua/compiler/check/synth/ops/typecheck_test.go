package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func makeAliasChain(base typ.Type, depth int, prefix string) typ.Type {
	out := base
	for i := 0; i < depth; i++ {
		out = &typ.Alias{Name: prefix, Target: out}
	}
	return out
}

func TestIsNumeric_Primitives(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"integer", typ.Integer, true},
		{"number", typ.Number, true},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
		{"string", typ.String, false},
		{"boolean", typ.Boolean, false},
		{"nil", typ.Nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNumeric(tt.t); got != tt.want {
				t.Errorf("IsNumeric(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsNumeric_Union(t *testing.T) {
	numUnion := typ.NewUnion(typ.Integer, typ.Number)
	if !IsNumeric(numUnion) {
		t.Error("union of numeric types should be numeric")
	}

	mixedUnion := typ.NewUnion(typ.Integer, typ.String)
	if IsNumeric(mixedUnion) {
		t.Error("union with non-numeric should not be numeric")
	}
}

func TestIsNumeric_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.Integer)
	if IsNumeric(opt) {
		t.Error("optional should not be numeric (needs narrowing)")
	}
}

func TestIsNumeric_Alias(t *testing.T) {
	alias := &typ.Alias{Name: "MyInt", Target: typ.Integer}
	if !IsNumeric(alias) {
		t.Error("alias of integer should be numeric")
	}
}

func TestIsNumeric_Literal(t *testing.T) {
	intLit := &typ.Literal{Base: kind.Integer, Value: int64(42)}
	if !IsNumeric(intLit) {
		t.Error("integer literal should be numeric")
	}

	floatLit := &typ.Literal{Base: kind.Number, Value: 3.14}
	if !IsNumeric(floatLit) {
		t.Error("float literal should be numeric")
	}

	strLit := &typ.Literal{Base: kind.String, Value: "hello"}
	if IsNumeric(strLit) {
		t.Error("string literal should not be numeric")
	}
}

func TestIsNumeric_Nil(t *testing.T) {
	if IsNumeric(nil) {
		t.Error("nil should not be numeric")
	}
}

func TestIsOrderable_Primitives(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"integer", typ.Integer, true},
		{"number", typ.Number, true},
		{"string", typ.String, true},
		{"any", typ.Any, true},
		{"boolean", typ.Boolean, false},
		{"nil", typ.Nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOrderable(tt.t); got != tt.want {
				t.Errorf("IsOrderable(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsOrderable_Literal(t *testing.T) {
	strLit := &typ.Literal{Base: kind.String, Value: "hello"}
	if !IsOrderable(strLit) {
		t.Error("string literal should be orderable")
	}

	boolLit := typ.LiteralBool(true)
	if IsOrderable(boolLit) {
		t.Error("boolean literal should not be orderable")
	}
}

func TestIsStringable(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string", typ.String, true},
		{"integer", typ.Integer, true},
		{"number", typ.Number, true},
		{"any", typ.Any, true},
		{"boolean", typ.Boolean, false},
		{"nil", typ.Nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStringable(tt.t); got != tt.want {
				t.Errorf("IsStringable(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsStringable_Union(t *testing.T) {
	strNumUnion := typ.NewUnion(typ.String, typ.Number)
	if !IsStringable(strNumUnion) {
		t.Error("union of string and number should be stringable")
	}

	strBoolUnion := typ.NewUnion(typ.String, typ.Boolean)
	if IsStringable(strBoolUnion) {
		t.Error("union with boolean should not be stringable")
	}
}

func TestHasLength(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string", typ.String, true},
		{"any", typ.Any, true},
		{"builtin table marker", typ.NewInterface("table", nil), true},
		{"integer", typ.Integer, false},
		{"boolean", typ.Boolean, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasLength(tt.t); got != tt.want {
				t.Errorf("HasLength(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestHasLength_Collections(t *testing.T) {
	arr := &typ.Array{Element: typ.Integer}
	if !HasLength(arr) {
		t.Error("array should have length")
	}

	m := &typ.Map{Key: typ.String, Value: typ.Integer}
	if !HasLength(m) {
		t.Error("map should have length")
	}

	rec := &typ.Record{Fields: []typ.Field{{Name: "x", Type: typ.Integer}}}
	if !HasLength(rec) {
		t.Error("record should have length")
	}

	tuple := typ.NewTuple(typ.Integer, typ.Integer, typ.Integer)
	if !HasLength(tuple) {
		t.Error("tuple should have length")
	}
}

func TestHasLength_Optional(t *testing.T) {
	opt := typ.NewOptional(&typ.Array{Element: typ.Integer})
	if HasLength(opt) {
		t.Error("optional should not have length (needs narrowing)")
	}
}

func TestIsStringOnly(t *testing.T) {
	if !IsStringOnly(typ.String) {
		t.Error("string should be string only")
	}

	if IsStringOnly(typ.Number) {
		t.Error("number should not be string only")
	}

	if IsStringOnly(nil) {
		t.Error("nil should not be string only")
	}

	strLit := &typ.Literal{Base: kind.String, Value: "hello"}
	if !IsStringOnly(strLit) {
		t.Error("string literal should be string only")
	}

	intLit := &typ.Literal{Base: kind.Integer, Value: int64(42)}
	if IsStringOnly(intLit) {
		t.Error("integer literal should not be string only")
	}
}

func TestIsBitwiseNumeric(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"integer", typ.Integer, true},
		{"number", typ.Number, true},
		{"integer literal", typ.LiteralInt(5), true},
		{"number literal", typ.LiteralNumber(5.5), true},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
		{"string", typ.String, false},
		{"optional integer", typ.NewOptional(typ.Integer), false},
		{"integer or nil", typ.NewUnion(typ.Integer, typ.Nil), false},
		{"nil", typ.Nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBitwiseNumeric(tt.t); got != tt.want {
				t.Errorf("IsBitwiseNumeric(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsBitwiseNumeric_Nil(t *testing.T) {
	if IsBitwiseNumeric(nil) {
		t.Error("nil should not be bitwise numeric")
	}
}

func TestIsNumeric_TypeParam(t *testing.T) {
	tp := &typ.TypeParam{Name: "T", Constraint: typ.Number}
	if !IsNumeric(tp) {
		t.Error("type param constrained to number should be numeric")
	}

	tpUnconstrained := &typ.TypeParam{Name: "T", Constraint: nil}
	if IsNumeric(tpUnconstrained) {
		t.Error("unconstrained type param should not be numeric")
	}
}

func TestIsNumeric_DeepAliasChain(t *testing.T) {
	deep := makeAliasChain(typ.Number, 40, "N")
	if !IsNumeric(deep) {
		t.Fatal("deep alias chain to number should be numeric")
	}
}

func TestIsStringable_DeepAliasChain(t *testing.T) {
	deep := makeAliasChain(typ.String, 40, "S")
	if !IsStringable(deep) {
		t.Fatal("deep alias chain to string should be stringable")
	}
}

func TestHasLength_DeepAliasChain(t *testing.T) {
	deep := makeAliasChain(typ.NewArray(typ.Number), 40, "A")
	if !HasLength(deep) {
		t.Fatal("deep alias chain to array should have length")
	}
}

func TestIsOrderable_TypeParam(t *testing.T) {
	tp := &typ.TypeParam{Name: "T", Constraint: typ.String}
	if !IsOrderable(tp) {
		t.Error("type param constrained to string should be orderable")
	}
}

func TestHasLength_TypeParam(t *testing.T) {
	tp := &typ.TypeParam{Name: "T", Constraint: &typ.Array{Element: typ.Any}}
	if !HasLength(tp) {
		t.Error("type param constrained to array should have length")
	}

	tpStr := &typ.TypeParam{Name: "T", Constraint: typ.String}
	if !HasLength(tpStr) {
		t.Error("type param constrained to string should have length")
	}
}

func TestMayHaveLength(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string", typ.String, true},
		{"builtin table marker", typ.NewInterface("table", nil), true},
		{"integer", typ.Integer, false},
		{"optional string", typ.NewOptional(typ.String), true},
		{"string or nil", typ.NewUnion(typ.String, typ.Nil), true},
		{"integer or nil", typ.NewUnion(typ.Integer, typ.Nil), false},
		{"unknown", typ.Unknown, true},
		{"any", typ.Any, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MayHaveLength(tt.t); got != tt.want {
				t.Errorf("MayHaveLength(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMayBeStringable(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string", typ.String, true},
		{"number", typ.Number, true},
		{"boolean", typ.Boolean, false},
		{"error interface", typ.NewInterface("Error", nil), true},
		{"optional string", typ.NewOptional(typ.String), true},
		{"string or nil", typ.NewUnion(typ.String, typ.Nil), true},
		{"boolean or nil", typ.NewUnion(typ.Boolean, typ.Nil), false},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MayBeStringable(tt.t); got != tt.want {
				t.Errorf("MayBeStringable(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMayBeOrderable(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string", typ.String, true},
		{"number", typ.Number, true},
		{"integer", typ.Integer, true},
		{"boolean", typ.Boolean, false},
		{"optional number", typ.NewOptional(typ.Number), true},
		{"number or nil", typ.NewUnion(typ.Number, typ.Nil), true},
		{"boolean or nil", typ.NewUnion(typ.Boolean, typ.Nil), false},
		{"unknown", typ.Unknown, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MayBeOrderable(tt.t); got != tt.want {
				t.Errorf("MayBeOrderable(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
