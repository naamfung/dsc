package typ

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Optional represents a nullable type: T | nil.
//
// Optional is syntactic sugar for a union with nil. It provides cleaner
// representation for the common case of a value that may be absent.
//
// The Inner field holds the non-nil type. An Optional never contains
// another Optional (they are flattened during construction).
type Optional struct {
	Inner        Type
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// NewOptional creates an optional type (T | nil).
//
// Normalization rules:
//   - nil or Nil → Nil (already optional)
//   - T? → T? (already optional)
//   - Any → Any (Any already includes nil)
//   - Union → adds nil and re-normalizes through NewUnion
func NewOptional(inner Type) Type {
	if inner == nil || inner.Kind() == kind.Nil {
		return Nil
	}

	if inner.Kind() == kind.Optional {
		return inner
	}

	if IsAny(inner) {
		return Any
	}

	if inner.Kind() == kind.Union {
		u := UnwrapAnnotated(inner).(*Union)
		members := make([]Type, 0, len(u.Members)+1)
		members = append(members, Nil)
		members = append(members, u.Members...)

		return NewUnion(members...)
	}

	h := internal.HashCombine(uint64(kind.Optional), inner.Hash())

	return &Optional{Inner: inner, hash: h, softPrunable: softPruneMayRewrite(inner)}
}

func (o *Optional) Kind() kind.Kind { return kind.Optional }

func (o *Optional) String() string {
	return o.strCache.get(func() string {
		if o.Inner == nil {
			return "nil?"
		}
		return o.Inner.String() + "?"
	})
}

func (o *Optional) Hash() uint64 { return o.hash }

func (o *Optional) Equals(other Type) bool {
	return TypeEquals(o, other)
}
