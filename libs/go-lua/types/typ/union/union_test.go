package union

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldTypes(t *testing.T) {
	t.Run("union of records", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("name", typ.String).Build()
		rec2 := typ.NewRecord().Field("name", typ.Number).Build()
		union := typ.NewUnion(rec1, rec2)

		result := FieldTypes(union, "name")
		if result == nil {
			t.Fatal("should return union of field types")
		}
		u, ok := result.(*typ.Union)
		if !ok {
			t.Fatal("result should be union")
		}
		if len(u.Members) != 2 {
			t.Errorf("expected 2 members, got %d", len(u.Members))
		}
	})

	t.Run("non-union", func(t *testing.T) {
		if FieldTypes(typ.String, "x") != nil {
			t.Error("should return nil for non-union")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		rec := typ.NewRecord().Field("x", typ.Number).Build()
		union := typ.NewUnion(rec, typ.String)
		if FieldTypes(union, "y") != nil {
			t.Error("should return nil for missing field")
		}
	})
}

func TestFunctionTypes(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	fn2 := typ.Func().Param("y", typ.Number).Returns(typ.Boolean).Build()

	t.Run("union of functions", func(t *testing.T) {
		union := typ.NewUnion(fn1, fn2)
		fns := FunctionTypes(union)
		if len(fns) != 2 {
			t.Errorf("expected 2 functions, got %d", len(fns))
		}
	})

	t.Run("single function", func(t *testing.T) {
		fns := FunctionTypes(fn1)
		if len(fns) != 1 {
			t.Errorf("expected 1 function, got %d", len(fns))
		}
	})

	t.Run("non-function", func(t *testing.T) {
		fns := FunctionTypes(typ.String)
		if fns != nil {
			t.Error("should return nil for non-function")
		}
	})
}

func TestMergeFunctions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if MergeFunctions(nil) != nil {
			t.Error("should return nil for empty")
		}
	})

	t.Run("single", func(t *testing.T) {
		fn := typ.Func().Param("x", typ.String).Build()
		if MergeFunctions([]*typ.Function{fn}) != fn {
			t.Error("should return same function for single")
		}
	})

	t.Run("multiple", func(t *testing.T) {
		fn1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
		fn2 := typ.Func().Param("x", typ.Number).Returns(typ.Boolean).Build()
		merged := MergeFunctions([]*typ.Function{fn1, fn2})
		if merged == nil {
			t.Fatal("should merge functions")
		}
		if len(merged.Params) != 1 {
			t.Errorf("expected 1 param, got %d", len(merged.Params))
		}
	})

	t.Run("multi_return_preserves_all_positions", func(t *testing.T) {
		// Regression test: MergeFunctions must preserve all return positions
		// fn1 returns (string, nil)
		// fn2 returns (number, error)
		// merged should return (string|number, nil|error)
		fn1 := typ.Func().Returns(typ.String, typ.Nil).Build()
		fn2 := typ.Func().Returns(typ.Number, typ.String).Build()
		merged := MergeFunctions([]*typ.Function{fn1, fn2})
		if merged == nil {
			t.Fatal("should merge functions")
		}
		if len(merged.Returns) != 2 {
			t.Errorf("expected 2 return values, got %d", len(merged.Returns))
		}
	})

	t.Run("multi_return_different_lengths", func(t *testing.T) {
		// fn1 returns (string)
		// fn2 returns (number, boolean)
		// merged should return (string|number, boolean?) - or at least 2 positions
		fn1 := typ.Func().Returns(typ.String).Build()
		fn2 := typ.Func().Returns(typ.Number, typ.Boolean).Build()
		merged := MergeFunctions([]*typ.Function{fn1, fn2})
		if merged == nil {
			t.Fatal("should merge functions")
		}
		if len(merged.Returns) < 2 {
			t.Errorf("expected at least 2 return values, got %d", len(merged.Returns))
		}
	})
}

func TestDistributeExpected(t *testing.T) {
	t.Run("union", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Number)
		result := DistributeExpected(union)
		if len(result) != 2 {
			t.Errorf("expected 2 members, got %d", len(result))
		}
	})

	t.Run("non-union", func(t *testing.T) {
		result := DistributeExpected(typ.String)
		if len(result) != 1 || result[0] != typ.String {
			t.Error("should return slice with single element")
		}
	})
}
