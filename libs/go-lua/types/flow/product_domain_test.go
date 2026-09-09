package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProductDomain_ApplyCondition_UsesTypeNarrowedBaseForLeftoverFieldConstraint(t *testing.T) {
	allow := typ.NewAlias("Allow", typ.NewRecord().
		Field("kind", typ.LiteralString("allow")).
		Field("reason", typ.String).
		Build())
	deny := typ.NewAlias("Deny", typ.NewRecord().
		Field("kind", typ.LiteralString("deny")).
		Field("reason", typ.String).
		Build())
	deferType := typ.NewAlias("Defer", typ.NewRecord().
		Field("kind", typ.LiteralString("defer")).
		Field("queue", typ.String).
		Build())
	decision := typ.NewAlias("Decision", typ.NewUnion(allow, deny, deferType))
	baseType := typ.NewOptional(decision)

	path := constraint.Path{Root: "decision", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return baseType
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
		},
	}

	dom := NewProductDomain(env)
	ok := dom.ApplyCondition(constraint.FromConstraints(
		constraint.Truthy{Path: path},
		constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("defer")},
	))
	if !ok {
		t.Fatal("ApplyCondition returned false")
	}

	got := dom.TypeAt(key)
	if got == nil {
		t.Fatal("TypeAt(decision) returned nil")
	}
	if subtype.IsSubtype(typ.Nil, got) {
		t.Fatalf("TypeAt(decision) = %v, want non-nil Defer variant", got)
	}
	if queue, ok := core.Field(got, "queue"); !ok || !typ.TypeEquals(queue, typ.String) {
		t.Fatalf("queue field = %v, want string on narrowed Defer variant", queue)
	}
	if _, ok := core.Field(got, "reason"); ok {
		t.Fatalf("reason field should not remain on narrowed Defer variant: %v", got)
	}
}
