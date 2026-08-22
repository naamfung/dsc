package typ

import (
	"fmt"
	"sync/atomic"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// recursiveIDCounter generates unique IDs for recursive types.
var recursiveIDCounter uint64

// Recursive represents a self-referential (mu) type.
// Recursive types are identified by a unique ID to allow cycle detection
// during equality comparison and hashing without infinite recursion.
//
// Example: type Node = { next: Node? } is represented as:
//
//	Recursive{ID: 1, Name: "Node", Body: Record{Fields: [{name: "next", type: <self-ref>}]}}
type Recursive struct {
	ID   uint64
	Name string
	Body Type
}

// RecursiveBuilder is used during construction to provide a self-reference.
type RecursiveBuilder func(self Type) Type

// NewRecursive creates a new recursive type.
// The builder function receives a placeholder that represents self-references
// and should return the body type using that placeholder where needed.
func NewRecursive(name string, builder RecursiveBuilder) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	rec := &Recursive{
		ID:   id,
		Name: name,
	}

	rec.Body = builder(rec)
	return rec
}

// NewRecursiveWithBody creates a recursive type with a pre-built body.
// Use this when the body is already constructed with proper self-references.
func NewRecursiveWithBody(name string, body Type) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	return &Recursive{
		ID:   id,
		Name: name,
		Body: body,
	}
}

// NewRecursivePlaceholder creates an empty recursive type for deferred body assignment.
// Use SetBody to assign the body after creation. This is useful for mutual recursion.
func NewRecursivePlaceholder(name string) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)
	return &Recursive{
		ID:   id,
		Name: name,
	}
}

// SetBody assigns the body to a placeholder recursive type.
func (r *Recursive) SetBody(body Type) {
	r.Body = body
}

// hashWithVisited computes hash with cycle detection for recursive types.
// Uses structural traversal to ensure order-independent hashing for mutual recursion.
func hashWithVisited(t Type, visited map[*Recursive]bool) uint64 {
	if t == nil {
		return 0
	}

	// Check if this is a recursive type we've already seen
	if rec, ok := t.(*Recursive); ok {
		if visited[rec] {
			// Self-reference: use a sentinel hash value
			return internal.HashCombine(uint64(kind.Recursive), internal.FnvString("$self"))
		}
		visited[rec] = true
		defer delete(visited, rec)

		// Compute structurally rather than using pre-computed hash.
		// This ensures correct hashing during mutual recursion setup
		// when the other recursive type's hash may not be computed yet.
		h := internal.HashCombine(uint64(kind.Recursive), internal.FnvString(rec.Name))
		if rec.Body != nil {
			h = internal.HashCombine(h, hashBodyWithVisited(rec.Body, visited))
		}
		return h
	}

	// For non-recursive types, use their standard hash
	return t.Hash()
}

// hashBodyWithVisited hashes a type's structure with cycle detection.
// Handles compound types that may contain recursive references.
// Mirrors the real Hash() semantics of each type constructor for consistency.
func hashBodyWithVisited(t Type, visited map[*Recursive]bool) uint64 {
	if t == nil {
		return 0
	}

	// Check for recursive type reference
	if rec, ok := t.(*Recursive); ok {
		return hashWithVisited(rec, visited)
	}

	// For compound types, traverse their components
	return Visit(t, Visitor[uint64]{
		Optional: func(o *Optional) uint64 {
			return internal.HashCombine(uint64(kind.Optional), hashBodyWithVisited(o.Inner, visited))
		},
		Union: func(u *Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = internal.HashCombine(h, hashBodyWithVisited(m, visited))
			}
			return h
		},
		Intersection: func(in *Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = internal.HashCombine(h, hashBodyWithVisited(m, visited))
			}
			return h
		},
		Record: func(r *Record) uint64 {
			h := uint64(kind.Record)
			for _, f := range r.Fields {
				h = internal.HashCombine(h, internal.FnvString(f.Name))
				h = internal.HashCombine(h, hashBodyWithVisited(f.Type, visited))
				if f.Optional {
					h = internal.HashCombine(h, 1)
				}
				if f.Readonly {
					h = internal.HashCombine(h, 2)
				}
			}
			if r.Metatable != nil {
				h = internal.HashCombine(h, hashBodyWithVisited(r.Metatable, visited))
			}
			if r.Open {
				h = internal.HashCombine(h, 3)
			}
			if r.HasMapComponent() {
				h = internal.HashCombine(h, internal.FnvString("$mapKey"))
				h = internal.HashCombine(h, hashBodyWithVisited(r.MapKey, visited))
				h = internal.HashCombine(h, internal.FnvString("$mapValue"))
				h = internal.HashCombine(h, hashBodyWithVisited(r.MapValue, visited))
			}
			return h
		},
		Array: func(a *Array) uint64 {
			return internal.HashCombine(uint64(kind.Array), hashBodyWithVisited(a.Element, visited))
		},
		Map: func(m *Map) uint64 {
			h := uint64(kind.Map)
			h = internal.HashCombine(h, hashBodyWithVisited(m.Key, visited))
			h = internal.HashCombine(h, hashBodyWithVisited(m.Value, visited))
			return h
		},
		Tuple: func(t *Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range t.Elements {
				h = internal.HashCombine(h, hashBodyWithVisited(e, visited))
			}
			return h
		},
		Function: func(fn *Function) uint64 {
			h := uint64(kind.Function)
			// Type parameters
			for _, tp := range fn.TypeParams {
				h = internal.HashCombine(h, tp.Hash())
			}
			// Parameters with optional flags
			for _, p := range fn.Params {
				h = internal.HashCombine(h, hashBodyWithVisited(p.Type, visited))
				if p.Optional {
					h = internal.HashCombine(h, 1)
				}
			}
			// Variadic
			if fn.Variadic != nil {
				h = internal.HashCombine(h, hashBodyWithVisited(fn.Variadic, visited))
			}
			// Returns
			for _, r := range fn.Returns {
				h = internal.HashCombine(h, hashBodyWithVisited(r, visited))
			}
			return h
		},
		Default: func(t Type) uint64 {
			return t.Hash()
		},
	})
}

func (r *Recursive) Kind() kind.Kind { return kind.Recursive }

func (r *Recursive) String() string {
	return fmt.Sprintf("%s#%d", r.Name, r.ID)
}

func (r *Recursive) Hash() uint64 {
	// Compute hash on demand with cycle detection.
	// This ensures correct hashing for mutual recursion.
	return hashWithVisited(r, make(map[*Recursive]bool))
}

// Equals compares two recursive types by their structural identity.
// Two recursive types are equal if they have the same structure when
// the self-references are treated as equivalent.
func (r *Recursive) Equals(other Type) bool {
	return TypeEquals(r, other)
}

// IsRecursiveRef returns true if t is a reference to the given recursive type.
func IsRecursiveRef(t Type, rec *Recursive) bool {
	if t == rec {
		return true
	}
	if r, ok := t.(*Recursive); ok {
		return r.ID == rec.ID
	}
	return false
}
