package typ

import (
	"fmt"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// FieldAccess represents a projected field type from an unresolved base.
//
// Used in generic contexts where we need T.field but T is a type parameter.
// When T is instantiated with a concrete type, FieldAccess resolves to
// the actual field type.
//
// Example: In a generic function returning T.name, if T is unresolved,
// the return type is FieldAccess{Base: T, Field: "name"}.
type FieldAccess struct {
	Base  Type   // Base type (usually a TypeParam)
	Field string // Field name to access
	hash  uint64
}

// NewFieldAccess creates a deferred field access.
func NewFieldAccess(base Type, field string) *FieldAccess {
	h := internal.HashCombine(uint64(kind.FieldAccess), base.Hash())
	h = internal.HashCombine(h, internal.FnvString(field))

	return &FieldAccess{Base: base, Field: field, hash: h}
}

func (f *FieldAccess) Kind() kind.Kind { return kind.FieldAccess }
func (f *FieldAccess) String() string  { return f.Base.String() + "." + f.Field }
func (f *FieldAccess) Hash() uint64    { return f.hash }
func (f *FieldAccess) Equals(other Type) bool {
	if other.Kind() != kind.FieldAccess {
		return false
	}

	of := other.(*FieldAccess)

	return f.Field == of.Field && f.Base.Equals(of.Base)
}

// IndexAccess represents a projected index type from an unresolved base.
//
// Used in generic contexts where we need T[K] but T is a type parameter.
// When T is instantiated with a concrete type, IndexAccess resolves to
// the actual indexed element type.
//
// Example: For Array<T>[number], if T is unresolved, the result is
// IndexAccess{Base: Array<T>, Index: number}.
type IndexAccess struct {
	Base  Type // Base type (usually contains a TypeParam)
	Index Type // Index type for the access
	hash  uint64
}

// NewIndexAccess creates a deferred index access.
func NewIndexAccess(base, index Type) *IndexAccess {
	h := internal.HashCombine(uint64(kind.IndexAccess), base.Hash())
	h = internal.HashCombine(h, index.Hash())

	return &IndexAccess{Base: base, Index: index, hash: h}
}

func (i *IndexAccess) Kind() kind.Kind { return kind.IndexAccess }
func (i *IndexAccess) String() string {
	return fmt.Sprintf("%s[%s]", i.Base.String(), i.Index.String())
}
func (i *IndexAccess) Hash() uint64 { return i.hash }
func (i *IndexAccess) Equals(other Type) bool {
	if other.Kind() != kind.IndexAccess {
		return false
	}

	oi := other.(*IndexAccess)

	return i.Base.Equals(oi.Base) && i.Index.Equals(oi.Index)
}
