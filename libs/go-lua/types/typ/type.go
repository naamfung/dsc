// Package typ defines the core Type interface and type implementations.
//
// All types in the type system implement the Type interface, which provides
// kind classification, string representation, hashing, and equality. Types
// are designed to be immutable and structurally comparable.
//
// # Primitive Types
//
// Nil, Boolean, Number, Integer, String are singleton types representing
// Lua primitives. Integer is a subtype of Number.
//
// # Special Types
//
// Any: Explicit dynamic type (opt-out of type checking). Operations on Any
// are permissive but lose type safety.
//
// Unknown: Unresolved type information. Forces narrowing/inference rather
// than silently permitting operations.
//
// Never: Bottom type (empty set). Represents unreachable code or impossible
// values. Never is a subtype of all types.
//
// Self: Placeholder for the receiver type in method signatures. Substituted
// with the actual type when methods are resolved.
//
// # Composite Types
//
// See the respective type files for: Function, Record, Union, Optional,
// Intersection, Tuple, Array, Map, Generic, Alias, Interface, and others.
package typ

import (
	"github.com/wippyai/go-lua/types/kind"
)

// Type is the interface implemented by all types in the type system.
// Types are immutable and support structural equality and hashing.
type Type interface {
	Kind() kind.Kind
	String() string
	Hash() uint64
	Equals(other Type) bool
}

// Primitives are singletons.
//
// Any vs Unknown semantics:
//   - Any: explicit dynamic type (opt-out). Use only when the source is explicitly "any"
//     or a deliberate dynamic escape hatch. Operations on Any are permissive.
//   - Unknown: missing or unresolved information. It should force narrowing/inference
//     rather than silently permitting operations.
var (
	Nil     Type = nilType{}
	Boolean Type = booleanType{}
	Number  Type = numberType{}
	Integer Type = integerType{}
	String  Type = stringType{}
	Any     Type = anyType{}
	Unknown Type = unknownType{}
	Never   Type = neverType{}
	Self    Type = selfType{}
)

// Primitive type implementations

type nilType struct{}

func (nilType) Kind() kind.Kind    { return kind.Nil }
func (nilType) String() string     { return "nil" }
func (nilType) Hash() uint64       { return uint64(kind.Nil) }
func (nilType) Equals(o Type) bool { return o.Kind() == kind.Nil }

type booleanType struct{}

func (booleanType) Kind() kind.Kind    { return kind.Boolean }
func (booleanType) String() string     { return "boolean" }
func (booleanType) Hash() uint64       { return uint64(kind.Boolean) }
func (booleanType) Equals(o Type) bool { return o.Kind() == kind.Boolean }

type numberType struct{}

func (numberType) Kind() kind.Kind    { return kind.Number }
func (numberType) String() string     { return "number" }
func (numberType) Hash() uint64       { return uint64(kind.Number) }
func (numberType) Equals(o Type) bool { return o.Kind() == kind.Number }

type integerType struct{}

func (integerType) Kind() kind.Kind    { return kind.Integer }
func (integerType) String() string     { return "integer" }
func (integerType) Hash() uint64       { return uint64(kind.Integer) }
func (integerType) Equals(o Type) bool { return o.Kind() == kind.Integer }

type stringType struct{}

func (stringType) Kind() kind.Kind    { return kind.String }
func (stringType) String() string     { return "string" }
func (stringType) Hash() uint64       { return uint64(kind.String) }
func (stringType) Equals(o Type) bool { return o.Kind() == kind.String }

type anyType struct{}

func (anyType) Kind() kind.Kind    { return kind.Any }
func (anyType) String() string     { return "any" }
func (anyType) Hash() uint64       { return uint64(kind.Any) }
func (anyType) Equals(o Type) bool { return IsAny(o) }

type unknownType struct{}

func (unknownType) Kind() kind.Kind    { return kind.Unknown }
func (unknownType) String() string     { return "unknown" }
func (unknownType) Hash() uint64       { return uint64(kind.Unknown) }
func (unknownType) Equals(o Type) bool { return IsUnknown(o) }

type neverType struct{}

func (neverType) Kind() kind.Kind    { return kind.Never }
func (neverType) String() string     { return "never" }
func (neverType) Hash() uint64       { return uint64(kind.Never) }
func (neverType) Equals(o Type) bool { return IsNever(o) }

type selfType struct{}

func (selfType) Kind() kind.Kind    { return kind.Self }
func (selfType) String() string     { return "self" }
func (selfType) Hash() uint64       { return uint64(kind.Self) }
func (selfType) Equals(o Type) bool { return o.Kind() == kind.Self }

// LuaError is the standard error type for Lua functions.
// It represents structured errors with message, kind, retryable, etc.
var LuaError Type = NewInterface("Error", []Method{
	{Name: "kind", Type: Func().Param("self", Self).Returns(String).Build()},
	{Name: "retryable", Type: Func().Param("self", Self).Returns(Boolean).Build()},
	{Name: "details", Type: Func().Param("self", Self).Returns(Any).Build()},
	{Name: "message", Type: Func().Param("self", Self).Returns(String).Build()},
	{Name: "stack", Type: Func().Param("self", Self).Returns(String).Build()},
})
