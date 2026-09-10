package join

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnVectors(t *testing.T) {
	t.Run("empty left", func(t *testing.T) {
		right := []typ.Type{typ.String}
		result := ReturnVectors(nil, right)
		if len(result) != 1 || result[0] != typ.String {
			t.Error("empty left should return right")
		}
	})

	t.Run("empty right", func(t *testing.T) {
		left := []typ.Type{typ.String}
		result := ReturnVectors(left, nil)
		if len(result) != 1 || result[0] != typ.String {
			t.Error("empty right should return left")
		}
	})

	t.Run("same length", func(t *testing.T) {
		left := []typ.Type{typ.String}
		right := []typ.Type{typ.Number}
		result := ReturnVectors(left, right)
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
		if _, ok := result[0].(*typ.Union); !ok {
			t.Error("should create union")
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		left := []typ.Type{typ.String, typ.Number}
		right := []typ.Type{typ.Boolean}
		result := ReturnVectors(left, right)
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
	})

	t.Run("preserves unknown when paired with implicit nil", func(t *testing.T) {
		left := []typ.Type{typ.Unknown}
		right := []typ.Type{}
		result := ReturnVectors(left, right)
		if len(result) != 1 {
			t.Fatalf("expected 1, got %d", len(result))
		}
		if !typ.TypeEquals(result[0], typ.Unknown) {
			t.Fatalf("expected unknown, got %v", result[0])
		}
	})
}

func TestWithReturns(t *testing.T) {
	t.Run("nil sig", func(t *testing.T) {
		if WithReturns(nil, []typ.Type{typ.String}) != nil {
			t.Error("nil sig should return nil")
		}
	})

	t.Run("graft returns", func(t *testing.T) {
		sig := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
		result := WithReturns(sig, []typ.Type{typ.Boolean})
		if len(result.Returns) != 1 || result.Returns[0] != typ.Boolean {
			t.Error("should graft new returns")
		}
		if len(result.Params) != 1 || result.Params[0].Type != typ.String {
			t.Error("should preserve params")
		}
	})

	t.Run("normalize nil returns", func(t *testing.T) {
		sig := typ.Func().Build()
		result := WithReturns(sig, []typ.Type{nil, typ.String})
		if result.Returns[0] != typ.Unknown {
			t.Error("nil should be normalized to Unknown")
		}
	})

	t.Run("preserves spec effects", func(t *testing.T) {
		spec := contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
		sig := typ.Func().
			Param("x", typ.String).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Spec(spec).
			Build()
		result := WithReturns(sig, []typ.Type{typ.String, typ.NewOptional(typ.LuaError)})
		if result.Spec == nil {
			t.Fatal("expected function spec to be preserved")
		}
		extracted := contract.ExtractSpec(result)
		if extracted == nil || extracted.Effects.GetErrorReturn(0) == nil {
			t.Fatal("expected ErrorReturn label to be preserved")
		}
	})
}

func TestWithReturnsOrUnknown(t *testing.T) {
	t.Run("nil signature", func(t *testing.T) {
		if WithReturnsOrUnknown(nil, []typ.Type{typ.String}) != nil {
			t.Fatal("expected nil for nil signature")
		}
	})

	t.Run("preserves existing returns", func(t *testing.T) {
		sig := typ.Func().Returns(typ.Number).Build()
		got := WithReturnsOrUnknown(sig, []typ.Type{typ.String})
		if got != sig {
			t.Fatal("expected existing return signature to be preserved")
		}
	})

	t.Run("defaults to unknown", func(t *testing.T) {
		sig := typ.Func().Build()
		got := WithReturnsOrUnknown(sig, nil)
		if got == nil || len(got.Returns) != 1 || got.Returns[0] != typ.Unknown {
			t.Fatalf("expected unknown default return, got %v", got)
		}
	})

	t.Run("uses provided returns", func(t *testing.T) {
		sig := typ.Func().Param("x", typ.Number).Build()
		got := WithReturnsOrUnknown(sig, []typ.Type{typ.String})
		if got == nil || len(got.Returns) != 1 || got.Returns[0] != typ.String {
			t.Fatalf("expected provided return vector, got %v", got)
		}
	})

	t.Run("replaces placeholder returns with provided summary", func(t *testing.T) {
		sig := typ.Func().Returns(typ.Unknown).Build()
		got := WithReturnsOrUnknown(sig, []typ.Type{typ.Integer})
		if got == nil || len(got.Returns) != 1 || got.Returns[0] != typ.Integer {
			t.Fatalf("expected placeholder return to be replaced, got %v", got)
		}
	})
}
