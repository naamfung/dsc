package typ

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Ref represents an unresolved reference to a named type definition.
//
// Refs are created during parsing when a type name is encountered before
// its definition. They are resolved to the actual type during semantic
// analysis using the Module path to locate the definition.
//
// Local refs (Module == "") refer to types in the current module.
// Cross-module refs include the module path for import resolution.
type Ref struct {
	Module string // Module path (empty for local references)
	Name   string // Type name
	hash   uint64
	str    string
}

// NewRef creates a type reference.
func NewRef(module, name string) *Ref {
	h := internal.HashCombine(uint64(kind.Ref), internal.FnvString(module))
	h = internal.HashCombine(h, internal.FnvString(name))

	str := name
	if module != "" {
		str = module + "." + name
	}
	return &Ref{Module: module, Name: name, hash: h, str: str}
}

func (r *Ref) Kind() kind.Kind { return kind.Ref }

func (r *Ref) String() string {
	return r.str
}

func (r *Ref) Hash() uint64 { return r.hash }

func (r *Ref) Equals(other Type) bool {
	if other.Kind() != kind.Ref {
		return false
	}

	or := other.(*Ref)

	return r.Module == or.Module && r.Name == or.Name
}

// Alias represents a named type alias.
//
// Aliases provide alternative names for types without creating new types.
// An Alias is structurally equivalent to its Target for subtyping and
// equality, but preserves the name for error messages and documentation.
//
// Example: type UserId = number creates Alias{Name: "UserId", Target: number}
type Alias struct {
	Name         string // Alias name
	Target       Type   // Underlying type
	unaliased    Type
	hash         uint64
	softPrunable bool
}

// NewAlias creates a type alias.
func NewAlias(name string, target Type) *Alias {
	h := internal.HashCombine(uint64(kind.Alias), internal.FnvString(name))
	h = internal.HashCombine(h, target.Hash())

	return &Alias{
		Name:         name,
		Target:       target,
		unaliased:    flattenAliasTarget(target),
		hash:         h,
		softPrunable: softPruneMayRewrite(target),
	}
}

func (a *Alias) Kind() kind.Kind { return kind.Alias }
func (a *Alias) String() string  { return a.Name }
func (a *Alias) Hash() uint64    { return a.hash }

func (a *Alias) UnaliasedTarget() Type {
	if a == nil || a.unaliased == nil {
		return a.Target
	}
	return a.unaliased
}

// Equals compares structurally through the alias target.
func (a *Alias) Equals(other Type) bool {
	return TypeEquals(a.Target, other)
}

func flattenAliasTarget(target Type) Type {
	current := target
	for depth := 0; depth < DefaultRecursionDepth; depth++ {
		alias, ok := current.(*Alias)
		if !ok || alias == nil {
			return current
		}
		next := alias.Target
		if alias.unaliased != nil {
			next = alias.unaliased
		}
		if next == nil || next == current {
			return current
		}
		current = next
	}
	return current
}

// Platform represents a platform-specific opaque type.
//
// Platform types are provided by the runtime environment and have
// no structural representation in the type system. They are compared
// nominally by name.
//
// Example: userdata types, file handles, coroutines.
type Platform struct {
	Name string // Platform type name
	hash uint64
}

// NewPlatform creates a platform type.
func NewPlatform(name string) *Platform {
	h := internal.HashCombine(uint64(kind.Platform), internal.FnvString(name))
	return &Platform{Name: name, hash: h}
}

func (p *Platform) Kind() kind.Kind { return kind.Platform }
func (p *Platform) String() string  { return p.Name }
func (p *Platform) Hash() uint64    { return p.hash }
func (p *Platform) Equals(other Type) bool {
	if other.Kind() != kind.Platform {
		return false
	}

	return p.Name == other.(*Platform).Name
}

// Meta represents a metatype (the type of a type value).
//
// Meta types are used when types themselves are values, such as in
// type predicates or reflection. Meta{Of: T} represents the type of
// a runtime value that carries type T.
//
// Example: typeof(Point) has type Meta{Of: Point}
type Meta struct {
	Of       Type // The type being wrapped
	hash     uint64
	strCache stringCache
}

// NewMeta creates a metatype.
func NewMeta(of Type) *Meta {
	h := internal.HashCombine(uint64(kind.Meta), of.Hash())
	return &Meta{Of: of, hash: h}
}

func (m *Meta) Kind() kind.Kind { return kind.Meta }
func (m *Meta) String() string {
	return m.strCache.get(func() string {
		return "typeof(" + m.Of.String() + ")"
	})
}
func (m *Meta) Hash() uint64 { return m.hash }
func (m *Meta) Equals(other Type) bool {
	if other.Kind() != kind.Meta {
		return false
	}

	return m.Of.Equals(other.(*Meta).Of)
}
