package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// DispatchKind indicates the code generation strategy for method/function calls.
//
// The dispatch strategy affects generated code performance:
//   - Monomorphic calls can be inlined or directly dispatched
//   - Polymorphic calls use inline caches for common types
//   - Megamorphic calls require hash table lookups
//   - Dynamic calls have no type information available
type DispatchKind int

const (
	// DispatchDynamic indicates no static type information is available.
	// Generated code must perform full runtime dispatch.
	DispatchDynamic DispatchKind = iota

	// DispatchMono indicates a single concrete type is known.
	// Generated code can use direct calls without type checks.
	DispatchMono

	// DispatchPoly indicates 2-4 concrete types are possible.
	// Generated code can use an inline cache with type guards.
	DispatchPoly

	// DispatchMega indicates many (5+) types are possible.
	// Generated code should use a hash-based dispatch table.
	DispatchMega
)

// String returns the human-readable name of the dispatch kind.
func (d DispatchKind) String() string {
	switch d {
	case DispatchMono:
		return "mono"
	case DispatchPoly:
		return "poly"
	case DispatchMega:
		return "mega"
	default:
		return "dynamic"
	}
}

// Layout indicates the memory representation strategy for table types.
//
// The layout affects memory efficiency and access performance:
//   - Hash layout provides flexible key types but slower access
//   - Flat layout provides cache-friendly array access
//   - Struct layout provides direct field access by offset
type Layout int

const (
	// LayoutHash uses a generic hash table for flexible key-value storage.
	// This is the most general but least optimized representation.
	LayoutHash Layout = iota

	// LayoutFlat uses a contiguous C-style array for integer-keyed data.
	// Provides excellent cache locality and O(1) indexed access.
	LayoutFlat

	// LayoutStruct uses fixed field offsets like a C struct.
	// Provides O(1) field access by compile-time computed offsets.
	LayoutStruct
)

// String returns the human-readable name of the layout.
func (l Layout) String() string {
	switch l {
	case LayoutFlat:
		return "flat"
	case LayoutStruct:
		return "struct"
	default:
		return "hash"
	}
}

// GetDispatch determines the dispatch strategy for a type.
//
// The strategy is based on how many concrete types the type represents:
//   - Concrete types (single known type): DispatchMono
//   - Small unions (2-4 members): DispatchPoly
//   - Large unions (5+ members): DispatchMega
//   - Abstract types (any, interface, etc.): DispatchDynamic
func GetDispatch(t typ.Type) DispatchKind {
	if t == nil {
		return DispatchDynamic
	}

	if IsConcreteType(t) {
		return DispatchMono
	}

	if u, ok := t.(*typ.Union); ok {
		n := len(u.Members)
		if n <= 4 {
			return DispatchPoly
		}

		return DispatchMega
	}

	return DispatchDynamic
}

// IsConcreteType returns true if the type is a single concrete type.
//
// Concrete types have a known runtime representation:
//   - Primitive types (nil, boolean, number, string)
//   - Structural types (record, array, map, tuple, function)
//
// Abstract types are not concrete:
//   - any, unknown, never
//   - Union, intersection
//   - Interface (structural interface)
//   - Type variables and generics
func IsConcreteType(t typ.Type) bool {
	if t == nil {
		return false
	}

	k := t.Kind()
	if k.IsTopOrBottom() {
		return false
	}

	switch k {
	case kind.Union, kind.Intersection:
		return false
	case kind.Interface:
		return false
	case kind.TypeVar, kind.Generic:
		return false
	default:
		return true
	}
}

// CanElideNilCheck returns true if nil checks can be safely eliminated.
//
// If the type cannot contain nil (is not optional, not any, not a union with
// nil member), then runtime nil checks are unnecessary and can be optimized away.
func CanElideNilCheck(t typ.Type) bool {
	if t == nil {
		return false
	}

	return !ContainsNil(t)
}

// CanElideTypeCheck returns true if runtime type checking can be eliminated.
//
// When the actual type is known to be exactly the target type (same kind and
// both are primitive), no runtime type check is needed. This optimization
// applies to: nil, boolean, number, integer, string.
func CanElideTypeCheck(actual, target typ.Type) bool {
	if actual == nil || target == nil {
		return false
	}

	k := actual.Kind()
	if k != target.Kind() {
		return false
	}

	switch k {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return true
	default:
		return false
	}
}

// GetLayout determines the optimal memory layout for a type.
//
// Layout selection:
//   - Array types use LayoutFlat (contiguous elements)
//   - Record types use LayoutStruct (fixed field offsets)
//   - Other types (maps, unions, etc.) use LayoutHash (general purpose)
func GetLayout(t typ.Type) Layout {
	if t == nil {
		return LayoutHash
	}

	switch t.Kind() {
	case kind.Array:
		return LayoutFlat
	case kind.Record:
		return LayoutStruct
	default:
		return LayoutHash
	}
}
