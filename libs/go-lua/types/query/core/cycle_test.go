package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestObjectClassString(t *testing.T) {
	tests := []struct {
		class  ObjectClass
		expect string
	}{
		{Terminating, "terminating"},
		{Linking, "linking"},
		{Cyclic, "cyclic"},
		{ObjectClass(99), "cyclic"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if tt.class.String() != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, tt.class.String())
			}
		})
	}
}

func TestCanFormCycle(t *testing.T) {
	tests := []struct {
		name   string
		t      typ.Type
		expect bool
	}{
		{"nil type", nil, true},
		{"primitive string", typ.String, false},
		{"primitive integer", typ.Integer, false},
		{"primitive boolean", typ.Boolean, false},
		{"literal int", typ.LiteralInt(5), false},
		{"literal string", typ.LiteralString("x"), false},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
		{"function", typ.Func().Build(), true},
		{"interface", typ.NewInterface("I", nil), true},
		{"ref", typ.NewRef("mod", "Type"), true},
		{"typevar", typ.NewTypeVar(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanFormCycle(tt.t) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestCanFormCycleRecord(t *testing.T) {
	t.Run("record with primitives only", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("name", typ.String).
			Field("age", typ.Integer).
			Build()
		if CanFormCycle(rec) {
			t.Error("expected false for record with only primitives")
		}
	})

	t.Run("record with any field", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("name", typ.String).
			Field("data", typ.Any).
			Build()
		if !CanFormCycle(rec) {
			t.Error("expected true for record with any field")
		}
	})

	t.Run("record with function field", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("callback", typ.Func().Build()).
			Build()
		if !CanFormCycle(rec) {
			t.Error("expected true for record with function field")
		}
	})
}

func TestCanFormCycleArray(t *testing.T) {
	t.Run("array of primitives", func(t *testing.T) {
		arr := typ.NewArray(typ.String)
		if CanFormCycle(arr) {
			t.Error("expected false for array of primitives")
		}
	})

	t.Run("array of any", func(t *testing.T) {
		arr := typ.NewArray(typ.Any)
		if !CanFormCycle(arr) {
			t.Error("expected true for array of any")
		}
	})
}

func TestCanFormCycleMap(t *testing.T) {
	t.Run("map of primitives", func(t *testing.T) {
		m := typ.NewMap(typ.String, typ.Integer)
		if CanFormCycle(m) {
			t.Error("expected false for map of primitives")
		}
	})

	t.Run("map with any value", func(t *testing.T) {
		m := typ.NewMap(typ.String, typ.Any)
		if !CanFormCycle(m) {
			t.Error("expected true for map with any value")
		}
	})

	t.Run("map with any key", func(t *testing.T) {
		m := typ.NewMap(typ.Any, typ.String)
		if !CanFormCycle(m) {
			t.Error("expected true for map with any key")
		}
	})
}

func TestCanFormCycleTuple(t *testing.T) {
	t.Run("tuple of primitives", func(t *testing.T) {
		tuple := typ.NewTuple(typ.String, typ.Integer)
		if CanFormCycle(tuple) {
			t.Error("expected false for tuple of primitives")
		}
	})

	t.Run("tuple with any element", func(t *testing.T) {
		tuple := typ.NewTuple(typ.String, typ.Any)
		if !CanFormCycle(tuple) {
			t.Error("expected true for tuple with any")
		}
	})
}

func TestCanFormCycleOptional(t *testing.T) {
	t.Run("optional primitive", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)
		if CanFormCycle(opt) {
			t.Error("expected false for optional primitive")
		}
	})

	t.Run("optional any", func(t *testing.T) {
		opt := typ.NewOptional(typ.Any)
		if !CanFormCycle(opt) {
			t.Error("expected true for optional any")
		}
	})
}

func TestCanFormCycleUnion(t *testing.T) {
	t.Run("union of primitives", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer)
		if CanFormCycle(union) {
			t.Error("expected false for union of primitives")
		}
	})

	t.Run("union with cyclic member", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Any)
		if !CanFormCycle(union) {
			t.Error("expected true for union with cyclic member")
		}
	})
}

func TestCanFormCycleIntersection(t *testing.T) {
	t.Run("intersection of acyclic records", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("a", typ.String).Build()
		rec2 := typ.NewRecord().Field("b", typ.Integer).Build()

		inter := typ.NewIntersection(rec1, rec2)
		if CanFormCycle(inter) {
			t.Error("expected false for intersection of acyclic")
		}
	})

	t.Run("intersection with cyclic member", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("a", typ.String).Build()
		rec2 := typ.NewRecord().Field("b", typ.Any).Build()

		inter := typ.NewIntersection(rec1, rec2)
		if !CanFormCycle(inter) {
			t.Error("expected true for intersection with cyclic member")
		}
	})
}

func TestCanFormCycleAlias(t *testing.T) {
	t.Run("alias to primitive", func(t *testing.T) {
		alias := typ.NewAlias("S", typ.String)
		if CanFormCycle(alias) {
			t.Error("expected false for alias to primitive")
		}
	})

	t.Run("alias to cyclic", func(t *testing.T) {
		alias := typ.NewAlias("A", typ.Any)
		if !CanFormCycle(alias) {
			t.Error("expected true for alias to cyclic")
		}
	})
}

func TestIsProvenAcyclic(t *testing.T) {
	if !IsProvenAcyclic(typ.String) {
		t.Error("expected string to be proven acyclic")
	}

	if IsProvenAcyclic(typ.Any) {
		t.Error("expected any to not be proven acyclic")
	}
}

func TestGetObjectClass(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		if GetObjectClass(typ.String) != Terminating {
			t.Error("expected Terminating")
		}
	})

	t.Run("any type", func(t *testing.T) {
		if GetObjectClass(typ.Any) != Cyclic {
			t.Error("expected Cyclic")
		}
	})

	t.Run("record with any field", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Any).Build()
		if GetObjectClass(rec) != Cyclic {
			t.Error("expected Cyclic for record with any")
		}
	})

	t.Run("record with unknown field", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Unknown).Build()
		if GetObjectClass(rec) != Cyclic {
			t.Error("expected Cyclic for record with unknown")
		}
	})

	t.Run("record with primitive fields", func(t *testing.T) {
		rec := typ.NewRecord().
			Field("name", typ.String).
			Field("age", typ.Integer).
			Build()

		result := GetObjectClass(rec)
		if result == Cyclic {
			t.Error("expected non-Cyclic for record with primitives")
		}
	})
}
