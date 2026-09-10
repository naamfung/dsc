// Package kind defines type kind enumeration for efficient type discrimination.
//
// Kind values enable fast type classification without reflection, supporting
// switch-based dispatch throughout the type system. Each type in the typ package
// returns its Kind via the Kind() method.
//
// # Categories
//
// Primitive kinds: Nil, Boolean, Number, Integer, String
// Represent base Lua types with value semantics.
//
// Top/Bottom types: Any (top), Unknown (unresolved), Never (bottom)
// Special types for type system boundaries and error handling.
//
// Composite kinds: Optional, Union, Intersection, Tuple, Function, Array, Map, Record
// Structural types built from other types.
//
// Nominal kinds: Sum, Interface, Alias, Generic, Instantiated
// Named types with identity-based semantics.
//
// Deferred kinds: Ref, TypeParam, TypeVar, FieldAccess, IndexAccess
// Types requiring resolution (forward references, generics, projections).
//
// Other kinds: Platform, Literal, Self, Meta, Refined, Recursive
// Specialized types for platform APIs, literal values, and advanced features.
package kind

// Kind identifies the structural category of a type for efficient dispatch.
// Returned by typ.Type.Kind() to enable type classification without casts.
type Kind int

const (
	Nil Kind = iota
	Boolean
	Number
	Integer
	String
	Any
	Unknown
	Never
	Optional
	Union
	Intersection
	Tuple
	Function
	Array
	Map
	Record
	Sum
	Interface
	Alias
	Generic
	Instantiated
	Platform
	Literal
	Self
	Ref
	Meta
	TypeParam
	TypeVar
	Refined
	FieldAccess
	IndexAccess
	Recursive
)

var kindNames = [...]string{
	Nil:          "nil",
	Boolean:      "boolean",
	Number:       "number",
	Integer:      "integer",
	String:       "string",
	Any:          "any",
	Unknown:      "unknown",
	Never:        "never",
	Optional:     "optional",
	Union:        "union",
	Intersection: "intersection",
	Tuple:        "tuple",
	Function:     "function",
	Array:        "array",
	Map:          "map",
	Record:       "record",
	Sum:          "sum",
	Interface:    "interface",
	Alias:        "alias",
	Generic:      "generic",
	Instantiated: "instantiated",
	Platform:     "platform",
	Literal:      "literal",
	Self:         "self",
	Ref:          "ref",
	Meta:         "meta",
	TypeParam:    "typeparam",
	TypeVar:      "typevar",
	Refined:      "refined",
	FieldAccess:  "fieldaccess",
	IndexAccess:  "indexaccess",
	Recursive:    "recursive",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}

	return "unknown"
}

// IsPrimitive returns true for primitive types (nil, boolean, number, integer, string).
func (k Kind) IsPrimitive() bool {
	return k <= String
}

// IsComposite returns true for composite types (union, intersection, tuple, etc.).
func (k Kind) IsComposite() bool {
	switch k {
	case Union, Intersection, Tuple, Array, Map, Record, Function:
		return true
	}

	return false
}

// IsDeferred returns true for types that need resolution (ref, typeparam, typevar, fieldaccess, indexaccess).
func (k Kind) IsDeferred() bool {
	switch k {
	case Ref, TypeParam, TypeVar, FieldAccess, IndexAccess:
		return true
	}

	return false
}

// IsPlaceholder returns true for Any or Unknown kinds.
// These represent unresolved or open type positions.
func (k Kind) IsPlaceholder() bool {
	return k == Any || k == Unknown
}

// IsConcrete returns true for types that are fully resolved (not Any, Unknown, or Never).
func (k Kind) IsConcrete() bool {
	return k != Any && k != Unknown && k != Never
}

// IsTopOrBottom returns true for Any (top), Unknown (unresolved), or Never (bottom).
func (k Kind) IsTopOrBottom() bool {
	return k == Any || k == Unknown || k == Never
}

// IsNever returns true for the Never kind (bottom type).
func (k Kind) IsNever() bool {
	return k == Never
}

// FromString converts a Lua type() result string to a Kind.
// Returns Unknown for unrecognized strings.
func FromString(s string) Kind {
	switch s {
	case "string":
		return String
	case "number":
		return Number
	case "boolean":
		return Boolean
	case "table":
		return Record
	case "function":
		return Function
	case "nil":
		return Nil
	default:
		return Unknown
	}
}
