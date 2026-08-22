package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Union represents a type that can be any of its member types: T1 | T2 | ...
//
// Unions are normalized during construction:
//   - Nested unions are flattened
//   - Duplicate members are removed
//   - Never members are dropped (Never is identity for union)
//   - Any absorbs all other members (union with Any = Any)
//   - Literals are subsumed by their base types (string | "foo" = string)
//   - Single non-nil member with nil becomes Optional
//
// Members are sorted by hash for deterministic comparison and serialization.
type Union struct {
	Members      []Type
	hash         uint64
	hasSoftMbr   bool // true if any member is a soft placeholder
	softPrunable bool
	strCache     stringCache
}

// NewUnion creates a normalized union type from the given members.
// Returns Never for empty unions, the single type for one member,
// or a normalized Union for multiple distinct members.
func NewUnion(members ...Type) Type {
	if len(members) == 0 {
		return Never
	}

	if len(members) == 1 {
		return members[0]
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))
	hasNil := false
	hasAny := false
	hasUnknown := false

	var addMember func(Type)
	addMember = func(m Type) {
		if m == nil {
			return
		}

		// Unwrap Annotated to access structural type for flattening.
		// Annotations delegate Kind() to their inner type, so type
		// assertions on concrete wrappers (Union, Optional) require
		// operating on the unwrapped type.
		unwrapped := UnwrapAnnotated(m)

		switch unwrapped.Kind() {
		case kind.Never:
			return // Never is identity for union
		case kind.Unknown:
			hasUnknown = true
			return // Unknown doesn't contribute information to union
		case kind.Any:
			hasAny = true
		case kind.Nil:
			hasNil = true
		case kind.Union:
			for _, member := range unwrapped.(*Union).Members {
				addMember(member)
			}
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*Optional).Inner)
		default:
			flat = append(flat, m)
		}
	}

	for _, m := range members {
		addMember(m)
	}

	if hasAny {
		return Any
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique := deduplicateTypes(flat)

	// If nothing concrete remains, preserve Unknown (and Optional<Unknown> when nil present).
	if len(unique) == 0 && hasUnknown {
		if hasNil {
			return NewOptional(Unknown)
		}
		return Unknown
	}

	// Primitive subsumption:
	// number subsumes integer in the lattice, so keep number only.
	hasNumberType := false
	for _, m := range unique {
		if m.Kind() == kind.Number {
			hasNumberType = true
			break
		}
	}
	if hasNumberType {
		filtered := unique[:0]
		for _, m := range unique {
			if m.Kind() == kind.Integer {
				continue
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Subsume literals: if a base type is present, drop literal members with matching base.
	// e.g. string | "" => string, number | 42 => number
	var baseMask uint8
	for _, m := range unique {
		switch m.Kind() {
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
	}
	if baseMask != 0 {
		filtered := unique[:0]
		for _, m := range unique {
			if lit, ok := m.(*Literal); ok {
				drop := false
				switch lit.Base {
				case kind.String:
					drop = baseMask&1 != 0
				case kind.Number:
					drop = baseMask&2 != 0
				case kind.Integer:
					drop = baseMask&4 != 0 || baseMask&2 != 0
				case kind.Boolean:
					drop = baseMask&8 != 0
				}
				if drop {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Sort by hash then string for deterministic order
	sort.Slice(unique, func(i, j int) bool {
		hi := unique[i].Hash()
		hj := unique[j].Hash()

		if hi != hj {
			return hi < hj
		}

		return unique[i].String() < unique[j].String()
	})

	// Add nil back if present
	if hasNil {
		// If single member + nil, return Optional
		if len(unique) == 1 {
			return NewOptional(unique[0])
		}

		unique = append([]Type{Nil}, unique...)
	}

	if len(unique) == 0 {
		if hasNil {
			return Nil
		}

		return Never
	}

	if len(unique) == 1 {
		return unique[0]
	}

	// Compute hash and check for soft members
	h := uint64(kind.Union)
	hasSoft := false
	hasNonSoft := false
	softPrunable := false
	for _, m := range unique {
		h = internal.HashCombine(h, m.Hash())
		if softPruneMayRewrite(m) {
			softPrunable = true
		}
		if memberIsSoft(m) {
			hasSoft = true
		} else {
			hasNonSoft = true
		}
	}
	if hasSoft && hasNonSoft {
		softPrunable = true
	}

	return &Union{Members: unique, hash: h, hasSoftMbr: hasSoft, softPrunable: softPrunable}
}

// HasSoftMember reports whether any union member is a soft placeholder type.
// Computed at construction time for O(1) access.
func (u *Union) HasSoftMember() bool { return u.hasSoftMbr }

func (u *Union) Kind() kind.Kind { return kind.Union }

// memberIsSoft checks if a type is a soft placeholder.
// Used at Union construction to set the hasSoftMbr flag.
// Mirrors isSoft logic but without recursion guard (unions are already flat).
func memberIsSoft(t Type) bool {
	if t == nil {
		return false
	}
	for {
		if ann, ok := t.(*Annotated); ok && ann.Inner != nil {
			t = ann.Inner
			continue
		}
		break
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	switch v := t.(type) {
	case *Optional:
		return memberIsSoft(v.Inner)
	case *Alias:
		return memberIsSoft(v.Target)
	case *Array:
		return memberIsSoft(v.Element)
	case *Map:
		return memberIsSoft(v.Value)
	case *Record:
		if len(v.Fields) == 0 && !v.HasMapComponent() {
			return true
		}
		if v.HasMapComponent() && len(v.Fields) == 0 {
			return memberIsSoft(v.MapValue)
		}
		return false
	case *Union:
		for _, m := range v.Members {
			if !memberIsSoft(m) {
				return false
			}
		}
		return len(v.Members) > 0
	}
	return false
}

func (u *Union) String() string {
	return u.strCache.get(func() string {
		parts := make([]string, len(u.Members))
		for i, m := range u.Members {
			if m == nil {
				parts[i] = "nil"
			} else {
				parts[i] = m.String()
			}
		}
		return strings.Join(parts, " | ")
	})
}

func (u *Union) Hash() uint64 { return u.hash }

func (u *Union) Equals(other Type) bool {
	return TypeEquals(u, other)
}

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := t.Hash()
	for _, m := range u.Members {
		if m.Hash() == h && TypeEquals(m, t) {
			return true
		}
	}

	return false
}
