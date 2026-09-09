package narrow

import "github.com/wippyai/go-lua/types/typ"

// Resolver provides context-free type queries for field and index access.
//
// Narrowing operations need to resolve field types (for discriminant checks)
// and index types (for array/map access) without access to full semantic
// context. Resolver abstracts these operations to decouple narrowing logic
// from the full type checker.
//
// # Purpose
//
// The Resolver interface enables the narrow package to work with structured
// types (records, interfaces, arrays, maps) without depending on the full
// type checking infrastructure. This separation allows:
//   - Testing narrowing logic in isolation.
//   - Using narrowing from different contexts (checker, solver).
//   - Avoiding circular dependencies between packages.
//
// # Implementation
//
// Implementors should handle the following type structures:
//
// For Field:
//   - Record: Return the field's type if it exists.
//   - Interface: Return the method's type if it exists.
//   - Intersection: Try each member, return first match.
//   - Union: Only if all members have the field with same type.
//
// For Index:
//   - Array: Return element type for integer keys.
//   - Map: Return value type if key is compatible.
//   - Tuple: Return element type for integer literal keys.
//   - Record with map component: Return map value type.
//
// # Thread Safety
//
// Resolver implementations should be safe for concurrent use if the
// narrowing operations may be called from multiple goroutines.
//
// # Example Implementation
//
//	type MyResolver struct {
//	    // ... fields for type environment ...
//	}
//
//	func (r *MyResolver) Field(t typ.Type, name string) (typ.Type, bool) {
//	    if rec, ok := t.(*typ.Record); ok {
//	        if f := rec.GetField(name); f != nil {
//	            return f.Type, true
//	        }
//	    }
//	    return nil, false
//	}
//
//	func (r *MyResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
//	    if arr, ok := t.(*typ.Array); ok {
//	        return arr.Element, true
//	    }
//	    return nil, false
//	}
type Resolver interface {
	// Field returns the type of field 'name' on type t.
	//
	// Returns (fieldType, true) if the field exists and is accessible.
	// Returns (nil, false) if the field does not exist or t is not a
	// structured type that supports field access.
	//
	// For records, this looks up named fields.
	// For interfaces, this looks up methods.
	Field(t typ.Type, name string) (typ.Type, bool)

	// Index returns the element type when indexing t with key.
	//
	// Returns (elementType, true) if the index operation is valid.
	// Returns (nil, false) if t does not support indexing with the given key.
	//
	// For arrays, key should be an integer type.
	// For maps, key should be compatible with the map's key type.
	// For tuples, key should be an integer literal within bounds.
	Index(t typ.Type, key typ.Type) (typ.Type, bool)
}
