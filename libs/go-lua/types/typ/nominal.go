package typ

import (
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Variant represents a case in a sum type (tagged union).
// Tag is the discriminant, Types holds the associated data (empty for unit variants).
type Variant struct {
	Tag   string // Discriminant value (e.g., "Some", "None")
	Types []Type // Associated data types (empty for unit variant like None)
}

// Sum represents a tagged union (sum type / discriminated union).
//
// Sum types enable safe pattern matching over a closed set of variants.
// Each variant has a tag and optional associated data.
//
// Example: Option<T> = Some(T) | None
type Sum struct {
	Name     string    // Type name for display
	Variants []Variant // Possible cases
	hash     uint64
	strCache stringCache
}

// NewSum creates a sum type.
func NewSum(name string, variants []Variant) *Sum {
	h := internal.HashCombine(uint64(kind.Sum), internal.FnvString(name))
	for _, v := range variants {
		h = internal.HashCombine(h, internal.FnvString(v.Tag))
		for _, t := range v.Types {
			h = internal.HashCombine(h, t.Hash())
		}
	}
	// Defensive copy to prevent external mutation
	copied := make([]Variant, len(variants))
	copy(copied, variants)

	return &Sum{Name: name, Variants: copied, hash: h}
}

func (s *Sum) Kind() kind.Kind { return kind.Sum }

func (s *Sum) String() string {
	return s.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString("enum ")
		sb.WriteString(s.Name)
		sb.WriteString(" { ")

		for i, v := range s.Variants {
			if i > 0 {
				sb.WriteString(" | ")
			}

			sb.WriteString(v.Tag)

			if len(v.Types) > 0 {
				sb.WriteString("(")

				for j, t := range v.Types {
					if j > 0 {
						sb.WriteString(", ")
					}

					sb.WriteString(t.String())
				}

				sb.WriteString(")")
			}
		}

		sb.WriteString(" }")

		return sb.String()
	})
}

func (s *Sum) Hash() uint64 { return s.hash }

func (s *Sum) Equals(other Type) bool {
	if other.Kind() != kind.Sum {
		return false
	}

	os := other.(*Sum)
	if s.Name != os.Name || len(s.Variants) != len(os.Variants) {
		return false
	}

	for i, v := range s.Variants {
		ov := os.Variants[i]
		if v.Tag != ov.Tag || len(v.Types) != len(ov.Types) {
			return false
		}

		for j, t := range v.Types {
			if !t.Equals(ov.Types[j]) {
				return false
			}
		}
	}

	return true
}

// Method represents a method in an interface type.
type Method struct {
	Name string    // Method name
	Type *Function // Method signature
}

// Interface represents a structural interface type (trait/protocol).
//
// Interfaces define a set of methods that a type must implement.
// Unlike nominal interfaces, subtyping is structural: any type with
// matching methods satisfies the interface.
//
// Named interfaces (Name != "") use nominal identity for marker interfaces
// (interfaces with no methods, like Channel<T>).
type Interface struct {
	Name     string   // Interface name (empty for anonymous)
	Methods  []Method // Required methods
	hash     uint64
	strCache stringCache
}

// NewInterface creates an interface type.
func NewInterface(name string, methods []Method) *Interface {
	h := internal.HashCombine(uint64(kind.Interface), internal.FnvString(name))
	for _, m := range methods {
		h = internal.HashCombine(h, internal.FnvString(m.Name))
		h = internal.HashCombine(h, m.Type.Hash())
	}
	// Defensive copy to prevent external mutation
	copied := make([]Method, len(methods))
	copy(copied, methods)

	return &Interface{Name: name, Methods: copied, hash: h}
}

func (i *Interface) Kind() kind.Kind { return kind.Interface }

func (i *Interface) String() string {
	return i.strCache.get(func() string {
		if i.Name != "" {
			return i.Name
		}

		var sb strings.Builder
		sb.WriteString("interface { ")

		for j, m := range i.Methods {
			if j > 0 {
				sb.WriteString("; ")
			}

			sb.WriteString(m.Name)
			sb.WriteString(": ")
			sb.WriteString(m.Type.String())
		}

		sb.WriteString(" }")

		return sb.String()
	})
}

func (i *Interface) Hash() uint64 { return i.hash }

func (i *Interface) Equals(other Type) bool {
	if other.Kind() != kind.Interface {
		return false
	}

	oi := other.(*Interface)
	if i.Name != oi.Name || len(i.Methods) != len(oi.Methods) {
		return false
	}

	for j, m := range i.Methods {
		om := oi.Methods[j]
		if m.Name != om.Name || !m.Type.Equals(om.Type) {
			return false
		}
	}

	return true
}
