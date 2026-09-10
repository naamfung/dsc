package core

import (
	"sort"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// fieldKey uniquely identifies a field lookup operation for memoization.
// Used as cache key for Engine.fieldQ and Engine.metaQ queries.
type fieldKey struct {
	t    typ.Type
	name string
}

// indexKey uniquely identifies an index lookup operation for memoization.
// Used as cache key for Engine.indexQ queries where t[key] is evaluated.
type indexKey struct {
	t   typ.Type
	key typ.Type
}

// methodKey uniquely identifies a method lookup operation for memoization.
// Used as cache key for Engine.methodQ queries where t:name() is resolved.
type methodKey struct {
	t    typ.Type
	name string
}

// unaryKey uniquely identifies a unary operator resolution for memoization.
// Used as cache key for Engine.unaryQ queries (e.g., -x, #x, not x).
type unaryKey struct {
	op string
	t  typ.Type
}

// binaryKey uniquely identifies a binary operator resolution for memoization.
// Used as cache key for Engine.binaryQ queries (e.g., a + b, a == b, a and b).
type binaryKey struct {
	left  typ.Type
	op    string
	right typ.Type
}

// unwrapKey identifies a type for operations that unwrap or transform a single type.
// Used for callable checks, type expansion, and widening operations.
type unwrapKey struct {
	t typ.Type
}

// subtypeKey identifies a subtype relationship check for memoization.
// Used as cache key for Engine.subtypeQ queries testing sub <: super.
type subtypeKey struct {
	sub   typ.Type
	super typ.Type
}

// fieldResult captures the outcome of a field/method/index lookup.
// The ok field indicates whether the lookup succeeded; t holds the resolved type.
type fieldResult struct {
	t  typ.Type
	ok bool
}

// callableResult captures the outcome of checking if a type is callable.
// The ok field indicates callability; fn holds the function signature if callable.
type callableResult struct {
	fn *typ.Function
	ok bool
}

// StdlibConfig configures standard library type information for the engine.
//
// Lua's standard library provides methods on primitive types (string.upper,
// string.len, etc.) that are accessed via the colon syntax (s:upper()).
// This configuration maps type kinds to record types that define these methods,
// enabling the engine to resolve method calls on primitive types.
//
// Example configuration:
//
//	cfg := StdlibConfig{
//	    MethodProviders: map[kind.Kind]*typ.Record{
//	        kind.String: stringLibRecord,  // provides upper, lower, len, etc.
//	    },
//	}
type StdlibConfig struct {
	// MethodProviders maps type kinds to records containing method definitions.
	// When Engine.Method is called on a type with a matching kind, the engine
	// first checks the provider record for the method before falling back to
	// structural lookup (metatables, etc.).
	MethodProviders map[kind.Kind]*typ.Record
}

// Engine provides memoized type query operations for performance-critical paths.
//
// The engine caches results of expensive type computations (field lookup, subtype
// checks, operator resolution) using a query-based memoization system. This is
// essential for type checking performance since the same type queries occur
// repeatedly during analysis of loops, function calls, and complex expressions.
//
// Engine is stateless in terms of mutation - all state is query caches that are
// populated on demand. Context for individual queries is passed per-call via
// db.QueryContext. The engine is safe for concurrent use.
//
// Query Widening:
//
// Some queries (field, method, index, operators) use widening to handle
// recursive types and ensure termination. When a cycle is detected, results
// are widened (e.g., specific types become unions) to produce a sound
// over-approximation.
//
// Usage:
//
//	engine := core.NewEngine()
//	fieldType, ok := engine.Field(ctx, recordType, "name")
//	if ok {
//	    // fieldType is the type of the "name" field
//	}
type Engine struct {
	// Primary lookup queries
	fieldQ    *db.Query[fieldKey, fieldResult]     // t.name field access
	methodQ   *db.Query[methodKey, fieldResult]    // t:name() method access
	indexQ    *db.Query[indexKey, fieldResult]     // t[key] index access
	unaryQ    *db.Query[unaryKey, typ.Type]        // -t, #t, ~t, not t
	binaryQ   *db.Query[binaryKey, typ.Type]       // a op b
	callableQ *db.Query[unwrapKey, callableResult] // callable check
	metaQ     *db.Query[fieldKey, fieldResult]     // __index, __call, etc.

	// Subtype and transformation queries
	subtypeQ    *db.Query[subtypeKey, bool]    // sub <: super check
	expandQ     *db.Query[unwrapKey, typ.Type] // expand Instantiated types
	widenQ      *db.Query[unwrapKey, typ.Type] // literal -> base type
	widenInferQ *db.Query[unwrapKey, typ.Type] // deep widening for inference

	// methodProviders maps type kinds to records that provide methods.
	// Used for stdlib types like string that have library methods.
	methodProviders map[kind.Kind]*typ.Record
}

// internType ensures type identity for efficient cache lookups.
// Type interning allows pointer equality checks instead of deep structural
// comparison, significantly improving cache hit performance. Returns the
// original type if no database context is available.
func internType(ctx *db.QueryContext, t typ.Type) typ.Type {
	if ctx == nil || ctx.DB() == nil {
		return t
	}

	return ctx.DB().InternType(t)
}

// NewEngine constructs a new query engine with empty caches and no stdlib providers.
//
// The engine initializes queries for all supported operations with appropriate
// equality and widening functions. Each query is configured to handle recursive
// types safely through widening when cycles are detected.
//
// For engines that need stdlib method resolution (string.upper, etc.), use
// NewEngineWithStdlib instead.
func NewEngine() *Engine {
	e := &Engine{
		methodProviders: make(map[kind.Kind]*typ.Record),
	}
	e.fieldQ = db.NewQueryWithWiden("Field", func(_ *db.QueryContext, key fieldKey) fieldResult {
		t, ok := fieldDepth(key.t, key.name, 0)
		return fieldResult{t: t, ok: ok}
	}, fieldResultEqual, widenFieldResult)
	e.methodQ = db.NewQueryWithWiden("Method", func(_ *db.QueryContext, key methodKey) fieldResult {
		t, ok := methodDepth(key.t, key.name, 0)
		return fieldResult{t: t, ok: ok}
	}, fieldResultEqual, widenFieldResult)
	e.indexQ = db.NewQueryWithWiden("Index", func(_ *db.QueryContext, key indexKey) fieldResult {
		t, ok := indexDepth(key.t, key.key, 0)
		return fieldResult{t: t, ok: ok}
	}, fieldResultEqual, widenFieldResult)
	e.unaryQ = db.NewQueryWithWiden("UnaryOp", func(_ *db.QueryContext, key unaryKey) typ.Type {
		return unaryOpCompute(key.op, key.t)
	}, typesEqual, widenType)
	e.binaryQ = db.NewQueryWithWiden("BinaryOp", func(_ *db.QueryContext, key binaryKey) typ.Type {
		return binaryOpCompute(key.left, key.op, key.right)
	}, typesEqual, widenType)
	e.callableQ = db.NewQuery("Callable", func(_ *db.QueryContext, key unwrapKey) callableResult {
		fn, ok := Callable(key.t)
		return callableResult{fn: fn, ok: ok}
	}, callableResultEqual)
	e.metaQ = db.NewQueryWithWiden("GetMetamethod", func(_ *db.QueryContext, key fieldKey) fieldResult {
		t, ok := getMetamethodDepth(key.t, key.name, 0)
		return fieldResult{t: t, ok: ok}
	}, fieldResultEqual, widenFieldResult)

	e.subtypeQ = db.NewQuery("IsSubtype", func(_ *db.QueryContext, key subtypeKey) bool {
		return subtype.IsSubtype(key.sub, key.super)
	}, boolEqual)

	e.expandQ = db.NewQueryWithWiden("ExpandInstantiated", func(_ *db.QueryContext, key unwrapKey) typ.Type {
		return subst.ExpandInstantiated(key.t)
	}, typesEqual, widenType)

	e.widenQ = db.NewQuery("Widen", func(_ *db.QueryContext, key unwrapKey) typ.Type {
		return subtype.Widen(key.t)
	}, typesEqual)

	e.widenInferQ = db.NewQuery("WidenForInference", func(_ *db.QueryContext, key unwrapKey) typ.Type {
		return subtype.WidenForInference(key.t)
	}, typesEqual)

	return e
}

// NewEngineWithStdlib constructs a query engine with standard library method providers.
//
// This is the preferred constructor when analyzing code that uses Lua's standard
// library. The method providers enable resolution of calls like s:upper() on
// string types without requiring explicit metatable definitions.
//
// Example:
//
//	engine := core.NewEngineWithStdlib(core.StdlibConfig{
//	    MethodProviders: map[kind.Kind]*typ.Record{
//	        kind.String: stdlib.StringMethods,
//	    },
//	})
func NewEngineWithStdlib(cfg StdlibConfig) *Engine {
	e := NewEngine()
	if len(cfg.MethodProviders) > 0 {
		keys := make([]kind.Kind, 0, len(cfg.MethodProviders))
		for k := range cfg.MethodProviders {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, k := range keys {
			e.methodProviders[k] = cfg.MethodProviders[k]
		}
	}
	return e
}

// Field returns the type of a named field on a structural type.
//
// This resolves dot access expressions (t.name) by searching for the field in:
//  1. Record fields directly
//  2. Map key-value pairs if the key type accepts string literals
//  3. Interface method signatures
//  4. __index metamethod fallback for records with metatables
//
// For union types, the field must exist in ALL members; the result is a union
// of each member's field type. For intersection types, the field from ANY
// member suffices. Optional types propagate optionality to the result.
//
// Returns (nil, false) if the field does not exist or the type does not support
// field access.
func (e *Engine) Field(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if e == nil || e.fieldQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	t = internType(ctx, t)
	res := e.fieldQ.Get(ctx, fieldKey{t: t, name: name})

	return res.t, res.ok
}

// Method returns the type of a named method on a type.
//
// This resolves colon syntax method calls (t:name()) by searching for methods in:
//  1. Stdlib method providers (for primitive types like string)
//  2. Record fields that are function types
//  3. Interface method signatures
//  4. Metatable fields for records with metatables
//  5. __index chain for inherited methods
//
// For union types, the method must exist with compatible signatures in all
// non-nil members. Nil members in unions are skipped (nil | T allows T's methods).
//
// Returns (nil, false) if the method does not exist or the type does not support
// method calls.
func (e *Engine) Method(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if e == nil || e.methodQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	// Check method providers for builtin types
	if t != nil {
		// Try exact kind match first
		if provider, ok := e.methodProviders[t.Kind()]; ok && provider != nil {
			if f := provider.GetField(name); f != nil {
				return f.Type, true
			}
			return nil, false
		}
		// Handle string literals (Kind=Literal but Base=String)
		if isStrictString(t) {
			if provider, ok := e.methodProviders[kind.String]; ok && provider != nil {
				if f := provider.GetField(name); f != nil {
					return f.Type, true
				}
				return nil, false
			}
		}
	}

	t = internType(ctx, t)
	res := e.methodQ.Get(ctx, methodKey{t: t, name: name})

	return res.t, res.ok
}

// Index returns the element type for bracket index access (t[key]).
//
// This resolves index expressions by examining the container type:
//   - Array: returns element type for numeric keys
//   - Map: returns Optional(value type) if key is subtype of map's key type
//   - Tuple: returns specific element for literal integer keys, union for dynamic
//   - Record: returns field type for literal string keys, map component for others
//
// The result is typically Optional because Lua tables return nil for missing keys.
// Returns (nil, false) if the index operation is not valid for the type/key combination.
func (e *Engine) Index(ctx *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool) {
	if e == nil || e.indexQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	t = internType(ctx, t)
	key = internType(ctx, key)
	res := e.indexQ.Get(ctx, indexKey{t: t, key: key})

	return res.t, res.ok
}

// UnaryOp resolves the result type of a unary operator expression.
//
// Supported operators:
//   - "-": arithmetic negation, returns integer or number
//   - "#": length operator, returns integer
//   - "~": bitwise NOT (Lua 5.3+), returns integer
//   - "not": logical negation, always returns boolean
//
// For non-primitive types, metamethods are checked (__unm, __len, __bnot).
// Returns nil if the operator is not applicable to the operand type.
func (e *Engine) UnaryOp(ctx *db.QueryContext, op string, operand typ.Type) typ.Type {
	if e == nil || e.unaryQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	operand = internType(ctx, operand)

	return e.unaryQ.Get(ctx, unaryKey{op: op, t: operand})
}

// BinaryOp resolves the result type of a binary operator expression.
//
// Supported operators:
//   - Arithmetic: +, -, *, /, %, ^, // (floor division)
//   - Bitwise: &, |, ~ (xor), <<, >> (Lua 5.3+)
//   - Concatenation: ..
//   - Comparison: ==, ~=, <, <=, >, >=
//   - Logical: and, or
//
// Type resolution follows Lua semantics:
//   - Arithmetic on integers stays integer (except ^ which promotes to number)
//   - Logical operators return operand types, not boolean
//   - Comparison operators return boolean
//   - Metamethods are checked for non-primitive operands
//
// Returns nil if the operator is not applicable to the operand types.
func (e *Engine) BinaryOp(ctx *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type {
	if e == nil || e.binaryQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	left = internType(ctx, left)
	right = internType(ctx, right)

	return e.binaryQ.Get(ctx, binaryKey{left: left, op: op, right: right})
}

// Callable returns the function type if t is callable.
//
// A type is callable if it is:
//   - A function type directly
//   - A record with __call metamethod
//   - A union where all members are callable
//   - An intersection where any member is callable
//   - A generic type whose body is callable
//
// Returns (nil, false) if the type cannot be called as a function.
func (e *Engine) Callable(ctx *db.QueryContext, t typ.Type) (*typ.Function, bool) {
	if e == nil || e.callableQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	t = internType(ctx, t)
	res := e.callableQ.Get(ctx, unwrapKey{t: t})

	return res.fn, res.ok
}

// GetMetamethod looks up a metamethod on a type's metatable.
//
// Lua metamethods control operator behavior and special operations:
//   - __index: field access fallback
//   - __newindex: field assignment fallback
//   - __call: function call operator
//   - __add, __sub, etc.: arithmetic operators
//   - __eq, __lt, __le: comparison operators
//   - __tostring: string conversion
//
// This only checks the immediate metatable; it does not walk __index chains.
// Returns (nil, false) if the type has no metatable or the metamethod is not defined.
func (e *Engine) GetMetamethod(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if e == nil || e.metaQ == nil {
		panic("unwrap.Engine requires initialization")
	}

	t = internType(ctx, t)
	res := e.metaQ.Get(ctx, fieldKey{t: t, name: name})

	return res.t, res.ok
}

// IsSubtype returns whether sub is a subtype of super with memoization.
//
// The subtype relation determines assignability: a value of type sub can be
// used where a value of type super is expected. This includes:
//   - Literal types are subtypes of their base types (42 <: integer)
//   - Union members are subtypes of the union (A <: A | B)
//   - Records with more fields are subtypes of records with fewer fields
//   - Functions with contravariant parameters and covariant returns
//
// The engine caches results for frequently checked type pairs.
func (e *Engine) IsSubtype(ctx *db.QueryContext, sub, super typ.Type) bool {
	if e == nil || e.subtypeQ == nil {
		return subtype.IsSubtype(sub, super)
	}

	sub = internType(ctx, sub)
	super = internType(ctx, super)

	return e.subtypeQ.Get(ctx, subtypeKey{sub: sub, super: super})
}

// ExpandInstantiated expands generic instantiations with memoization.
//
// Given a type that may contain Instantiated nodes (generic types with type
// arguments), this recursively expands them by substituting type arguments
// into the generic body. The expansion is cached to avoid redundant work.
//
// Example: List<number> expands to the record type with number elements.
func (e *Engine) ExpandInstantiated(ctx *db.QueryContext, t typ.Type) typ.Type {
	if e == nil || e.expandQ == nil {
		return subst.ExpandInstantiated(t)
	}

	t = internType(ctx, t)

	return e.expandQ.Get(ctx, unwrapKey{t: t})
}

// Widen converts literal types to their base types with memoization.
//
// Widening is used when a more general type is needed:
//   - LiteralString("hello") -> string
//   - LiteralInteger(42) -> integer
//   - true -> boolean
//
// This is a shallow operation; nested types are not widened.
func (e *Engine) Widen(ctx *db.QueryContext, t typ.Type) typ.Type {
	if e == nil || e.widenQ == nil {
		return subtype.Widen(t)
	}

	t = internType(ctx, t)

	return e.widenQ.Get(ctx, unwrapKey{t: t})
}

// WidenForInference performs deep widening for type inference with memoization.
//
// Unlike Widen, this recursively widens all nested types. It is used during
// type inference to produce general types from specific observed values:
//   - {x: 42, y: "hi"} -> {x: integer, y: string}
//   - [1, 2, 3] -> integer[]
//
// This ensures inferred types are suitable for variable declarations and
// function signatures.
func (e *Engine) WidenForInference(ctx *db.QueryContext, t typ.Type) typ.Type {
	if e == nil || e.widenInferQ == nil {
		return subtype.WidenForInference(t)
	}

	t = internType(ctx, t)

	return e.widenInferQ.Get(ctx, unwrapKey{t: t})
}

// fieldResultEqual compares two field lookup results for equality.
// Used by the query cache to detect when a result has stabilized.
func fieldResultEqual(a, b fieldResult) bool {
	if a.ok != b.ok {
		return false
	}

	return typesEqual(a.t, b.t)
}

// widenType combines two types during query cycle resolution.
// When a recursive query detects a cycle, this function produces a sound
// over-approximation by unioning the previous and current results and widening.
// This ensures termination while maintaining type soundness.
func widenType(prev, next typ.Type) typ.Type {
	if prev == nil {
		return subtype.WidenForInference(next)
	}

	if next == nil {
		return subtype.WidenForInference(prev)
	}

	return subtype.WidenForInference(typ.NewUnion(prev, next))
}

// widenFieldResult combines two field results during query cycle resolution.
// Prefers successful results; when both succeed, widens their types into a union.
func widenFieldResult(prev, next fieldResult) fieldResult {
	if !prev.ok {
		return next
	}

	if !next.ok {
		return prev
	}

	return fieldResult{t: widenType(prev.t, next.t), ok: true}
}

// callableResultEqual compares two callable check results for equality.
// Used by the query cache to detect result stabilization.
func callableResultEqual(a, b callableResult) bool {
	if a.ok != b.ok {
		return false
	}

	if a.fn == b.fn {
		return true
	}

	if a.fn == nil || b.fn == nil {
		return false
	}

	return typ.TypeEquals(a.fn, b.fn)
}

// typesEqual compares two types for structural equality.
// Uses pointer equality as a fast path, falling back to deep comparison.
func typesEqual(a, b typ.Type) bool {
	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return typ.TypeEquals(a, b)
}

// boolEqual compares two booleans for equality.
// Trivial but required for query system interface uniformity.
func boolEqual(a, b bool) bool {
	return a == b
}
