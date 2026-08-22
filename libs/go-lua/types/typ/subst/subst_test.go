package subst

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestSubstitute(t *testing.T) {
	t.Run("empty subs", func(t *testing.T) {
		if Substitute(typ.String, nil) != typ.String {
			t.Error("empty subs should return original")
		}
	})

	t.Run("type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		subs := map[string]typ.Type{"T": typ.String}
		result := Substitute(tp, subs)
		if result != typ.String {
			t.Error("should substitute type param")
		}
	})

	t.Run("no match", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		subs := map[string]typ.Type{"U": typ.String}
		result := Substitute(tp, subs)
		if result != tp {
			t.Error("unmatched param should remain")
		}
	})

	t.Run("in function", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		fn := typ.Func().Param("x", tp).Returns(tp).Build()
		subs := map[string]typ.Type{"T": typ.Number}
		result := Substitute(fn, subs)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatal("result should be function")
		}
		if resultFn.Params[0].Type != typ.Number {
			t.Error("param type should be substituted")
		}
		if resultFn.Returns[0] != typ.Number {
			t.Error("return type should be substituted")
		}
	})
}

func TestParams(t *testing.T) {
	t.Run("mismatched lengths", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		params := []*typ.TypeParam{tp}
		var args []typ.Type
		if Params(typ.String, params, args) != typ.String {
			t.Error("mismatched lengths should return original")
		}
	})

	t.Run("substitute params", func(t *testing.T) {
		tp1 := typ.NewTypeParam("T", nil)
		tp2 := typ.NewTypeParam("U", nil)
		tuple := typ.NewTuple(tp1, tp2)
		params := []*typ.TypeParam{tp1, tp2}
		args := []typ.Type{typ.String, typ.Number}
		result := Params(tuple, params, args)
		resultTuple, ok := result.(*typ.Tuple)
		if !ok {
			t.Fatal("result should be tuple")
		}
		if resultTuple.Elements[0] != typ.String {
			t.Error("first element should be String")
		}
		if resultTuple.Elements[1] != typ.Number {
			t.Error("second element should be Number")
		}
	})
}

func TestSelf(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		if Self(nil, typ.String) != nil {
			t.Error("nil type should return nil")
		}
	})

	t.Run("nil self", func(t *testing.T) {
		if Self(typ.String, nil) != typ.String {
			t.Error("nil self should return original")
		}
	})

	t.Run("replace self", func(t *testing.T) {
		fn := typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()
		result := Self(fn, typ.String)
		resultFn, ok := result.(*typ.Function)
		if !ok {
			t.Fatal("result should be function")
		}
		if resultFn.Params[0].Type != typ.String {
			t.Error("self param should be substituted")
		}
		if resultFn.Returns[0] != typ.String {
			t.Error("self return should be substituted")
		}
	})
}

func TestExpandInstantiated(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if ExpandInstantiated(nil) != nil {
			t.Error("nil should return nil")
		}
	})

	t.Run("non-instantiated", func(t *testing.T) {
		if ExpandInstantiated(typ.String) != typ.String {
			t.Error("non-instantiated should return original")
		}
	})

	t.Run("array of type param", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Array", []*typ.TypeParam{tp}, typ.NewArray(tp))
		inst := typ.Instantiate(generic, typ.Number)
		result := ExpandInstantiated(inst)
		arr, ok := result.(*typ.Array)
		if !ok {
			t.Fatalf("expected array, got %T", result)
		}
		if arr.Element != typ.Number {
			t.Error("element should be Number")
		}
	})

	t.Run("optional", func(t *testing.T) {
		tp := typ.NewTypeParam("T", nil)
		generic := typ.NewGeneric("Opt", []*typ.TypeParam{tp}, typ.NewOptional(tp))
		inst := typ.Instantiate(generic, typ.String)
		opt := typ.NewOptional(inst)
		result := ExpandInstantiated(opt)
		if result == opt {
			t.Error("should expand nested instantiated")
		}
	})
}
