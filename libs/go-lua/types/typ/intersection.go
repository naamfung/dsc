package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Intersection represents a type satisfying all member constraints: T1 & T2 & ...
//
// Intersections require a value to satisfy every member type simultaneously.
// They are normalized during construction:
//   - Nested intersections are flattened
//   - Duplicate members are removed
//   - Any members are dropped (Any is identity for intersection)
//   - Never absorbs all other members (intersection with Never = Never)
//
// Members are sorted by hash for deterministic comparison.
type Intersection struct {
	Members  []Type
	hash     uint64
	strCache stringCache
}

// NewIntersection creates a normalized intersection type.
// Returns Any for empty intersections, the single type for one member,
// or a normalized Intersection for multiple distinct members.
func NewIntersection(members ...Type) Type {
	if len(members) == 0 {
		return Any
	}

	if len(members) == 1 {
		return members[0]
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))
	hasNever := false
	hasNil := false

	for _, m := range members {
		if m == nil {
			continue
		}

		unwrapped := UnwrapAnnotated(m)

		switch unwrapped.Kind() {
		case kind.Any:
			continue // Any is identity for intersection
		case kind.Never:
			hasNever = true
		case kind.Nil:
			hasNil = true
		case kind.Intersection:
			flat = append(flat, unwrapped.(*Intersection).Members...)
		default:
			flat = append(flat, m)
		}
	}

	if hasNever {
		return Never
	}

	// If nil is in the intersection, check if all other members accept nil.
	// If so, the intersection simplifies to nil (the only value satisfying all).
	if hasNil && len(flat) > 0 {
		allAcceptNil := true

		for _, m := range flat {
			if !containsNilValue(m) {
				allAcceptNil = false
				break
			}
		}

		if allAcceptNil {
			return Nil
		}
	}

	if hasNil {
		flat = append(flat, Nil)
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique := deduplicateTypes(flat)

	sort.Slice(unique, func(i, j int) bool {
		hi := unique[i].Hash()
		hj := unique[j].Hash()

		if hi != hj {
			return hi < hj
		}

		return unique[i].String() < unique[j].String()
	})

	if len(unique) == 0 {
		return Any
	}

	if len(unique) == 1 {
		return unique[0]
	}

	h := uint64(kind.Intersection)
	for _, m := range unique {
		h = internal.HashCombine(h, m.Hash())
	}

	return &Intersection{Members: unique, hash: h}
}

func (i *Intersection) Kind() kind.Kind { return kind.Intersection }

func (i *Intersection) String() string {
	return i.strCache.get(func() string {
		parts := make([]string, len(i.Members))
		for j, m := range i.Members {
			if m == nil {
				parts[j] = "unknown"
			} else {
				parts[j] = m.String()
			}
		}
		return strings.Join(parts, " & ")
	})
}

func (i *Intersection) Hash() uint64 { return i.hash }

func (i *Intersection) Equals(other Type) bool {
	return TypeEquals(i, other)
}

// containsNilValue checks if a type can hold nil values.
func containsNilValue(t Type) bool {
	if t == nil {
		return false
	}

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return true
		},
		Union: func(u *Union) bool {
			for _, m := range u.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, m := range in.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Default: func(t Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}
