package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestInferDiscriminator(t *testing.T) {
	t.Run("nil union", func(t *testing.T) {
		result := InferDiscriminator(nil)
		if result != nil {
			t.Error("expected nil for nil union")
		}
	})

	t.Run("single member union", func(t *testing.T) {
		rec := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		union := &typ.Union{Members: []typ.Type{rec}}

		result := InferDiscriminator(union)
		if result != nil {
			t.Error("expected nil for single member union")
		}
	})

	t.Run("union with non-record members", func(t *testing.T) {
		union := typ.NewUnion(typ.String, typ.Integer)
		if u, ok := union.(*typ.Union); ok {
			result := InferDiscriminator(u)
			if result != nil {
				t.Error("expected nil for non-record union")
			}
		}
	})

	t.Run("valid discriminator with tag field", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		rec2 := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result == nil {
			t.Error("expected discriminator")
			return
		}

		if result.FieldName != "tag" {
			t.Errorf("expected field 'tag', got '%s'", result.FieldName)
		}

		if len(result.Values) != 2 {
			t.Errorf("expected 2 values, got %d", len(result.Values))
		}
	})

	t.Run("valid discriminator with type field", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("type", typ.LiteralString("user")).Build()
		rec2 := typ.NewRecord().Field("type", typ.LiteralString("admin")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result == nil {
			t.Error("expected discriminator")
			return
		}

		if result.FieldName != "type" {
			t.Errorf("expected field 'type', got '%s'", result.FieldName)
		}
	})

	t.Run("valid discriminator with kind field", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("kind", typ.LiteralString("success")).Build()
		rec2 := typ.NewRecord().Field("kind", typ.LiteralString("error")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result == nil {
			t.Error("expected discriminator")
			return
		}

		if result.FieldName != "kind" {
			t.Errorf("expected field 'kind', got '%s'", result.FieldName)
		}
	})

	t.Run("no valid discriminator - duplicate values", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		rec2 := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result != nil {
			t.Error("expected nil for duplicate discriminator values")
		}
	})

	t.Run("no valid discriminator - non-literal field", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("tag", typ.String).Build()
		rec2 := typ.NewRecord().Field("tag", typ.String).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result != nil {
			t.Error("expected nil for non-literal discriminator field")
		}
	})

	t.Run("no valid discriminator - missing field", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		rec2 := typ.NewRecord().Field("other", typ.LiteralString("b")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result != nil {
			t.Error("expected nil when field missing in one member")
		}
	})

	t.Run("prefers preferred discriminator names", func(t *testing.T) {
		rec1 := typ.NewRecord().
			Field("tag", typ.LiteralString("a")).
			Field("foo", typ.LiteralString("x")).
			Build()
		rec2 := typ.NewRecord().
			Field("tag", typ.LiteralString("b")).
			Field("foo", typ.LiteralString("y")).
			Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result == nil {
			t.Error("expected discriminator")
			return
		}

		if result.FieldName != "tag" {
			t.Errorf("expected preferred field 'tag', got '%s'", result.FieldName)
		}
	})

	t.Run("nil field type", func(t *testing.T) {
		rec1 := typ.NewRecord().Build()
		rec1.Fields = append(rec1.Fields, typ.Field{Name: "x", Type: nil})
		rec2 := typ.NewRecord().Field("x", typ.LiteralString("a")).Build()
		union := &typ.Union{Members: []typ.Type{rec1, rec2}}

		result := InferDiscriminator(union)
		if result != nil {
			t.Error("expected nil for nil field type")
		}
	})
}

func TestAllMembers_FlatUnion(t *testing.T) {
	union := &typ.Union{Members: []typ.Type{typ.String, typ.Integer}}

	result := AllMembers(union)
	if len(result) != 2 {
		t.Errorf("expected 2 members, got %d", len(result))
	}
}

func TestAllMembers_NestedUnion(t *testing.T) {
	inner := &typ.Union{Members: []typ.Type{typ.String, typ.Integer}}
	outer := &typ.Union{Members: []typ.Type{inner, typ.Boolean}}

	result := AllMembers(outer)
	if len(result) != 3 {
		t.Errorf("expected 3 members, got %d", len(result))
	}
}

func TestHasDiscriminator(t *testing.T) {
	t.Run("has discriminator", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
		rec2 := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()

		union := &typ.Union{Members: []typ.Type{rec1, rec2}}
		if !HasDiscriminator(union) {
			t.Error("expected true")
		}
	})

	t.Run("no discriminator", func(t *testing.T) {
		rec1 := typ.NewRecord().Field("x", typ.String).Build()
		rec2 := typ.NewRecord().Field("y", typ.Integer).Build()

		union := &typ.Union{Members: []typ.Type{rec1, rec2}}
		if HasDiscriminator(union) {
			t.Error("expected false")
		}
	})

	t.Run("nil union", func(t *testing.T) {
		if HasDiscriminator(nil) {
			t.Error("expected false for nil")
		}
	})
}

func TestGetDiscriminatorValue(t *testing.T) {
	t.Run("existing literal field", func(t *testing.T) {
		rec := typ.NewRecord().Field("tag", typ.LiteralString("hello")).Build()

		val, ok := GetDiscriminatorValue(rec, "tag")
		if !ok {
			t.Error("expected to find value")
		}

		if val != "hello" {
			t.Errorf("expected 'hello', got %v", val)
		}
	})

	t.Run("missing field", func(t *testing.T) {
		rec := typ.NewRecord().Field("other", typ.LiteralString("x")).Build()

		_, ok := GetDiscriminatorValue(rec, "tag")
		if ok {
			t.Error("expected not to find value")
		}
	})

	t.Run("non-literal field", func(t *testing.T) {
		rec := typ.NewRecord().Field("tag", typ.String).Build()

		_, ok := GetDiscriminatorValue(rec, "tag")
		if ok {
			t.Error("expected not to find value for non-literal")
		}
	})

	t.Run("nil record", func(t *testing.T) {
		_, ok := GetDiscriminatorValue(nil, "tag")
		if ok {
			t.Error("expected not to find value for nil record")
		}
	})
}
