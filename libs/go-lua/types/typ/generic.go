package typ

import (
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// TypeParam represents a type parameter in a generic type or function.
//
// Type parameters enable parametric polymorphism: a single definition that
// works with multiple types. The Constraint field bounds what types can
// instantiate this parameter (nil means any type is allowed).
//
// Example: In Array<T>, T is a TypeParam with Name="T" and Constraint=nil.
type TypeParam struct {
	Name       string
	Constraint Type // Upper bound (nil means any type)
	hash       uint64
}

// NewTypeParam creates a type parameter.
func NewTypeParam(name string, constraint Type) *TypeParam {
	h := internal.HashCombine(uint64(kind.TypeParam), internal.FnvString(name))
	if constraint != nil {
		h = internal.HashCombine(h, constraint.Hash())
	}

	return &TypeParam{Name: name, Constraint: constraint, hash: h}
}

func (t *TypeParam) Kind() kind.Kind { return kind.TypeParam }
func (t *TypeParam) String() string {
	if t.Constraint != nil {
		return t.Name + " : " + t.Constraint.String()
	}

	return t.Name
}
func (t *TypeParam) Hash() uint64 { return t.hash }
func (t *TypeParam) Equals(other Type) bool {
	if other.Kind() != kind.TypeParam {
		return false
	}

	ot := other.(*TypeParam)
	if t.Name != ot.Name {
		return false
	}

	if (t.Constraint == nil) != (ot.Constraint == nil) {
		return false
	}

	if t.Constraint != nil && !t.Constraint.Equals(ot.Constraint) {
		return false
	}

	return true
}

// Generic represents a parameterized type definition awaiting instantiation.
//
// Generics have type parameters that are substituted with concrete types
// when instantiated. The Body contains the type with TypeParam references.
//
// Identity:
//   - Named generics use nominal identity (name + params, ignoring body)
//   - Anonymous generics use structural identity (includes body in hash)
type Generic struct {
	Name       string       // Type name (empty for anonymous generics)
	TypeParams []*TypeParam // Type parameters to be substituted
	Body       Type         // Template type with TypeParam references
	hash       uint64
	strCache   stringCache
}

// NewGeneric creates a generic type definition.
// Named generics use nominal identity, anonymous generics use structural identity.
func NewGeneric(name string, params []*TypeParam, body Type) *Generic {
	h := internal.HashCombine(uint64(kind.Generic), internal.FnvString(name))
	for _, p := range params {
		h = internal.HashCombine(h, p.Hash())
	}

	// Only include body in hash for anonymous generics (structural identity).
	// Named generics are nominal: identity is name + type params.
	if name == "" && body != nil {
		h = internal.HashCombine(h, body.Hash())
	}

	copied := make([]*TypeParam, len(params))
	copy(copied, params)

	return &Generic{Name: name, TypeParams: copied, Body: body, hash: h}
}

func (g *Generic) Kind() kind.Kind { return kind.Generic }
func (g *Generic) String() string {
	return g.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString(g.Name)
		sb.WriteString("<")

		for i, p := range g.TypeParams {
			if i > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(p.String())
		}

		sb.WriteString(">")

		return sb.String()
	})
}
func (g *Generic) Hash() uint64 { return g.hash }
func (g *Generic) Equals(other Type) bool {
	return TypeEquals(g, other)
}

// Instantiated represents a generic type with concrete type arguments applied.
//
// When a generic like Array<T> is used as Array<number>, an Instantiated
// type is created with Generic=Array and TypeArgs=[number]. The body can
// be expanded by substituting type parameters with arguments.
type Instantiated struct {
	Generic      *Generic // The generic being instantiated
	TypeArgs     []Type   // Concrete types for each type parameter
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// Instantiate creates an instantiated generic type with the given arguments.
func Instantiate(g *Generic, args ...Type) *Instantiated {
	h := internal.HashCombine(uint64(kind.Instantiated), g.Hash())
	softPrunable := false
	for _, a := range args {
		h = internal.HashCombine(h, a.Hash())
		if !softPrunable && softPruneMayRewrite(a) {
			softPrunable = true
		}
	}

	return &Instantiated{Generic: g, TypeArgs: args, hash: h, softPrunable: softPrunable}
}

func (i *Instantiated) Kind() kind.Kind { return kind.Instantiated }
func (i *Instantiated) String() string {
	return i.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString(i.Generic.Name)
		sb.WriteString("<")

		for j, a := range i.TypeArgs {
			if j > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(a.String())
		}

		sb.WriteString(">")

		return sb.String()
	})
}
func (i *Instantiated) Hash() uint64 { return i.hash }
func (i *Instantiated) Equals(other Type) bool {
	return TypeEquals(i, other)
}

// TypeVar represents an inference variable during type checking.
//
// Type variables are placeholders created during generic instantiation
// and constraint solving. They are unified with concrete types as the
// checker gathers information. ID distinguishes different variables.
type TypeVar struct {
	ID   int
	hash uint64
}

// NewTypeVar creates a type variable with the given ID.
func NewTypeVar(id int) *TypeVar {
	h := internal.HashCombine(uint64(kind.TypeVar), uint64(id))
	return &TypeVar{ID: id, hash: h}
}

func (t *TypeVar) Kind() kind.Kind { return kind.TypeVar }
func (t *TypeVar) String() string  { return "$" + string(rune('a'+t.ID%26)) }
func (t *TypeVar) Hash() uint64    { return t.hash }
func (t *TypeVar) Equals(other Type) bool {
	if other.Kind() != kind.TypeVar {
		return false
	}

	return t.ID == other.(*TypeVar).ID
}
