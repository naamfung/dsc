package core

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeOps defines the interface for type query operations.
//
// This interface abstracts the core type operations needed by the type checker
// and other analysis tools. It enables dependency injection of type resolution
// logic, allowing different implementations for different contexts:
//
//   - [Engine] provides memoized implementations for production use
//   - Test doubles can provide controlled behavior for unit testing
//   - Adapters can bridge to external type systems
//
// All methods accept a QueryContext for memoization coordination. Methods that
// perform lookups (Field, Index, Method) return a boolean indicating success.
// Methods that transform types (BinaryOp, UnaryOp, Widen) return nil on failure.
//
// Thread Safety: Implementations must be safe for concurrent use.
type TypeOps interface {
	// Field resolves field access (t.name) and returns the field type.
	Field(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool)

	// Index resolves index access (t[key]) and returns the element type.
	Index(ctx *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool)

	// Method resolves method access (t:name) and returns the method type.
	Method(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool)

	// BinaryOp resolves binary operator type (left op right).
	BinaryOp(ctx *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type

	// UnaryOp resolves unary operator type (op operand).
	UnaryOp(ctx *db.QueryContext, op string, operand typ.Type) typ.Type

	// IsSubtype checks whether sub is a subtype of super.
	IsSubtype(ctx *db.QueryContext, sub, super typ.Type) bool

	// ExpandInstantiated expands generic instantiations in a type.
	ExpandInstantiated(ctx *db.QueryContext, t typ.Type) typ.Type

	// Widen converts literal types to their base types.
	Widen(ctx *db.QueryContext, t typ.Type) typ.Type

	// WidenForInference performs deep widening for type inference.
	WidenForInference(ctx *db.QueryContext, t typ.Type) typ.Type
}

// Verify Engine implements TypeOps at compile time.
var _ TypeOps = (*Engine)(nil)
