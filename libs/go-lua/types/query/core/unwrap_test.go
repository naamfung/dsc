package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestUnwrapAlias(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect typ.Type
	}{
		{"nil input", nil, nil},
		{"primitive string", typ.String, typ.String},
		{"primitive integer", typ.Integer, typ.Integer},
		{"primitive any", typ.Any, typ.Any},
		{"primitive never", typ.Never, typ.Never},
		{"simple alias", typ.NewAlias("MyString", typ.String), typ.String},
		{"nested alias", typ.NewAlias("A", typ.NewAlias("B", typ.Integer)), typ.Integer},
		{"alias to record", typ.NewAlias("Rec", typ.NewRecord().Field("x", typ.Number).Build()), typ.NewRecord().Field("x", typ.Number).Build()},
		{"ref type unchanged", typ.NewRef("mod", "Type"), typ.NewRef("mod", "Type")},
		{"union unchanged", typ.NewUnion(typ.String, typ.Integer), typ.NewUnion(typ.String, typ.Integer)},
		{"optional unchanged", typ.NewOptional(typ.String), typ.NewOptional(typ.String)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unwrap.Alias(tt.input)
			if tt.expect == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}

				return
			}

			if result == nil {
				t.Errorf("expected %v, got nil", tt.expect)
				return
			}

			if !result.Equals(tt.expect) {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestUnwrapToKind(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	alias := typ.NewAlias("Rec", rec)
	nestedAlias := typ.NewAlias("Nested", alias)

	tests := []struct {
		name      string
		input     typ.Type
		kind      kind.Kind
		expectNil bool
	}{
		{"nil input", nil, kind.String, true},
		{"direct match", typ.String, kind.String, false},
		{"alias to string", typ.NewAlias("S", typ.String), kind.String, false},
		{"alias to record", alias, kind.Record, false},
		{"nested alias to record", nestedAlias, kind.Record, false},
		{"no match - wrong kind", typ.String, kind.Integer, true},
		{"alias no match", typ.NewAlias("S", typ.String), kind.Number, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unwrap.ToKind(tt.input, tt.kind)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected non-nil, got nil")
				} else if result.Kind() != tt.kind {
					t.Errorf("expected kind %v, got %v", tt.kind, result.Kind())
				}
			}
		})
	}
}

func TestIsNilType(t *testing.T) {
	tests := []struct {
		name   string
		input  typ.Type
		expect bool
	}{
		{"nil input", nil, false},
		{"nil type", typ.Nil, true},
		{"string type", typ.String, false},
		{"optional type", typ.NewOptional(typ.String), false},
		{"never type", typ.Never, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unwrap.IsNilType(tt.input)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}
