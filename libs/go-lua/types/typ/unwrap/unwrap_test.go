package unwrap

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestUnderlying(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if Underlying(nil) != nil {
			t.Error("Underlying(nil) should return nil")
		}
	})

	t.Run("primitive", func(t *testing.T) {
		if Underlying(typ.String) != typ.String {
			t.Error("Underlying(String) should return String")
		}
	})

	t.Run("alias", func(t *testing.T) {
		alias := typ.NewAlias("MyString", typ.String)
		if Underlying(alias) != typ.String {
			t.Error("Underlying should unwrap alias")
		}
	})

	t.Run("optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)
		if Underlying(opt) != typ.String {
			t.Error("Underlying should unwrap optional")
		}
	})
}

func TestAlias(t *testing.T) {
	t.Run("preserves optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)
		alias := typ.NewAlias("OptString", opt)
		result := Alias(alias)
		if _, ok := result.(*typ.Optional); !ok {
			t.Error("Alias should preserve Optional")
		}
	})
}

func TestOptional(t *testing.T) {
	t.Run("nested", func(t *testing.T) {
		inner := typ.NewOptional(typ.String)
		outer := typ.NewOptional(inner)
		result := Optional(outer)
		if result != typ.String {
			t.Error("Optional should fully unwrap nested optionals")
		}
	})
}

func TestIsOptionalLike(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, true},
		{"Nil", typ.Nil, true},
		{"Optional", typ.NewOptional(typ.String), true},
		{"Union with nil", typ.NewUnion(typ.String, typ.Nil), true},
		{"Any", typ.Any, true},
		{"Unknown", typ.Unknown, true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOptionalLike(tt.t); got != tt.want {
				t.Errorf("IsOptionalLike() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSingleton(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, false},
		{"Nil", typ.Nil, true},
		{"Literal", typ.LiteralString("foo"), true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSingleton(tt.t); got != tt.want {
				t.Errorf("IsSingleton() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEmptyRecord(t *testing.T) {
	t.Run("empty record", func(t *testing.T) {
		rec := typ.NewRecord().Build()
		if !IsEmptyRecord(rec) {
			t.Error("should be empty record")
		}
	})

	t.Run("record with fields", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Number).Build()
		if IsEmptyRecord(rec) {
			t.Error("should not be empty record")
		}
	})
}

func TestIsContainer(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"Array", typ.NewArray(typ.Number), true},
		{"Map", typ.NewMap(typ.String, typ.Number), true},
		{"Record", typ.NewRecord().Build(), true},
		{"Tuple", typ.NewTuple(typ.String, typ.Number), true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContainer(tt.t); got != tt.want {
				t.Errorf("IsContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBuiltinTableTop(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	nonTableIface := typ.NewInterface("Reader", nil)
	aliasedTable := typ.NewAlias("TTable", tableTop)

	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"builtin table marker", tableTop, true},
		{"aliased builtin table marker", aliasedTable, true},
		{"non-table interface", nonTableIface, false},
		{"string", typ.String, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltinTableTop(tt.t); got != tt.want {
				t.Errorf("IsBuiltinTableTop() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFunction(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()

	t.Run("direct function", func(t *testing.T) {
		if Function(fn) != fn {
			t.Error("should return same function")
		}
	})

	t.Run("optional function", func(t *testing.T) {
		opt := typ.NewOptional(fn)
		if Function(opt) != fn {
			t.Error("should unwrap optional")
		}
	})

	t.Run("aliased function", func(t *testing.T) {
		alias := typ.NewAlias("MyFunc", fn)
		if Function(alias) != fn {
			t.Error("should unwrap alias")
		}
	})

	t.Run("non-function", func(t *testing.T) {
		if Function(typ.String) != nil {
			t.Error("should return nil for non-function")
		}
	})
}

func TestRecord(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()

	t.Run("direct record", func(t *testing.T) {
		if Record(rec) != rec {
			t.Error("should return same record")
		}
	})

	t.Run("optional record", func(t *testing.T) {
		opt := typ.NewOptional(rec)
		if Record(opt) != rec {
			t.Error("should extract from optional")
		}
	})
}

func TestUnion(t *testing.T) {
	union := typ.NewUnion(typ.String, typ.Number).(*typ.Union)

	t.Run("direct union", func(t *testing.T) {
		if Union(union) != union {
			t.Error("should return same union")
		}
	})

	t.Run("aliased union", func(t *testing.T) {
		alias := typ.NewAlias("MyUnion", union)
		if Union(alias) != union {
			t.Error("should extract from alias")
		}
	})
}

func TestIsLiteralString(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"string literal", typ.LiteralString("foo"), true},
		{"int literal", typ.LiteralInt(42), false},
		{"string type", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLiteralString(tt.t); got != tt.want {
				t.Errorf("IsLiteralString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToKind(t *testing.T) {
	t.Run("direct match", func(t *testing.T) {
		if ToKind(typ.String, kind.String) != typ.String {
			t.Error("should return type when kind matches")
		}
	})

	t.Run("through alias", func(t *testing.T) {
		alias := typ.NewAlias("MyString", typ.String)
		if ToKind(alias, kind.String) != typ.String {
			t.Error("should unwrap alias to find kind")
		}
	})

	t.Run("not found", func(t *testing.T) {
		if ToKind(typ.String, kind.Number) != nil {
			t.Error("should return nil when kind not found")
		}
	})
}

func TestIsNilType(t *testing.T) {
	if !IsNilType(typ.Nil) {
		t.Error("typ.Nil should be nil type")
	}
	if IsNilType(typ.String) {
		t.Error("typ.String should not be nil type")
	}
	if IsNilType(nil) {
		t.Error("nil should not be nil type")
	}
}
