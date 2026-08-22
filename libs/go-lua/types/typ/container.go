package typ

import (
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Array represents a homogeneous sequence type: T[].
//
// Arrays are Lua tables with integer keys starting at 1. The Element type
// describes what each element contains. Arrays support ipairs iteration
// and length operator (#).
type Array struct {
	Element      Type
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// NewArray creates an array type.
func NewArray(elem Type) *Array {
	if elem == nil {
		elem = Unknown
	}
	h := internal.HashCombine(uint64(kind.Array), elem.Hash())
	return &Array{Element: elem, hash: h, softPrunable: softPruneMayRewrite(elem)}
}

func (a *Array) Kind() kind.Kind { return kind.Array }
func (a *Array) String() string {
	return a.strCache.get(func() string {
		if a.Element == nil {
			return "unknown[]"
		}
		return a.Element.String() + "[]"
	})
}
func (a *Array) Hash() uint64 { return a.hash }
func (a *Array) Equals(o Type) bool {
	return TypeEquals(a, o)
}

// Map represents a homogeneous key-value mapping: {[K]: V}.
//
// Maps are Lua tables where all keys have type K and all values have type V.
// Unlike Records, Maps have uniform types for all entries rather than
// named fields with potentially different types.
type Map struct {
	Key          Type
	Value        Type
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// NewMap creates a map type.
func NewMap(key, value Type) *Map {
	if key == nil {
		key = Unknown
	}
	if value == nil {
		value = Unknown
	}
	h := internal.HashCombine(uint64(kind.Map), key.Hash())
	h = internal.HashCombine(h, value.Hash())

	return &Map{Key: key, Value: value, hash: h, softPrunable: softPruneAny(key, value)}
}

func (m *Map) Kind() kind.Kind { return kind.Map }
func (m *Map) String() string {
	return m.strCache.get(func() string {
		ks, vs := "unknown", "unknown"
		if m.Key != nil {
			ks = m.Key.String()
		}
		if m.Value != nil {
			vs = m.Value.String()
		}
		return "{[" + ks + "]: " + vs + "}"
	})
}
func (m *Map) Hash() uint64 { return m.hash }
func (m *Map) Equals(o Type) bool {
	return TypeEquals(m, o)
}

// Tuple represents a fixed-length heterogeneous sequence: (T1, T2, ...).
//
// Tuples are used for multi-value returns and destructuring assignments.
// Unlike Arrays, each position can have a different type and the length
// is fixed at compile time.
type Tuple struct {
	Elements     []Type
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// NewTuple creates a tuple type.
func NewTuple(elems ...Type) *Tuple {
	h := uint64(kind.Tuple)
	cleaned := make([]Type, len(elems))
	softPrunable := false
	for i, e := range elems {
		if e == nil {
			e = Unknown
		}
		cleaned[i] = e
		h = internal.HashCombine(h, e.Hash())
		if !softPrunable && softPruneMayRewrite(e) {
			softPrunable = true
		}
	}

	return &Tuple{Elements: cleaned, hash: h, softPrunable: softPrunable}
}

func (t *Tuple) Kind() kind.Kind { return kind.Tuple }

func (t *Tuple) String() string {
	return t.strCache.get(func() string {
		parts := make([]string, len(t.Elements))
		for i, e := range t.Elements {
			if e == nil {
				parts[i] = "unknown"
			} else {
				parts[i] = e.String()
			}
		}
		return "(" + strings.Join(parts, ", ") + ")"
	})
}

func (t *Tuple) Hash() uint64 { return t.hash }

func (t *Tuple) Equals(o Type) bool {
	return TypeEquals(t, o)
}
