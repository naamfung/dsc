// Package flow provides control flow type propagation and flow-sensitive analysis.
//
// Flow analysis computes the types of variables at each program point by propagating
// type information through the control flow graph. Unlike simple declared types,
// flow types account for assignments, conditionals, and narrowing constraints.
//
// # Usage
//
// Build an Inputs struct from the CFG, declared types, and extracted constraints,
// then call Solve to compute the flow solution:
//
//	inputs := &flow.Inputs{
//		Graph:         cfg,
//		DeclaredTypes: declaredTypes,
//		Assignments:   assignments,
//		EdgeConditions: edgeConditions,
//		TypeKeys:      typeKeys,
//		Decomposer:    decomposer,
//	}
//	solution := flow.Solve(inputs, resolver)
//
// Query the solution for types at specific program points:
//
//	t := solution.TypeAt(point, path)           // Base type lookup
//	narrowed := solution.NarrowedTypeAt(point, path)  // Type with narrowing applied
//	cond := solution.ConditionAt(point)         // Active constraints
//
// # Core Concepts
//
// Inputs: All data needed by the flow solver, including CFG, declared types,
// assignments, conditions, and constraint metadata. Inputs are AST-free and
// deterministic for caching and incremental analysis.
//
// Solution: The result of flow analysis, mapping versioned path keys to their
// narrowed types. Query methods apply point conditions to refine types further.
//
// TypeState: Tracks whether a type at a program point is resolved, pending,
// or conflicted. Used for fixed-point iteration during solving.
//
// SymbolTypes: Maps each CFG point to its per-symbol types, representing the
// complete type state at that point in the program.
//
// # Special Assignments
//
// The solver handles several special assignment patterns:
//
//   - IteratorSource: Derives iterator variable types from the iterated container
//   - ContainerElementSource: Derives types from container methods (channel:receive())
//   - MapElementSource: Derives types from dynamic map index reads (t[k])
//   - SiblingAssignment: Correlates multi-return values (result, err patterns)
//
// # Widening
//
// When recursive dependencies prevent convergence, symbols are widened to Unknown.
// WideningEvent records these cases for diagnostic reporting.
//
// # Subpackages
//
// The flow package has several subpackages:
//
//   - domain: Abstract domain interfaces for constraint solving
//   - join: Type joining operations for phi node merging
//   - numeric: Numeric range and interval tracking
//   - pathkey: Canonical path key resolution for SSA versions
//   - propagate: Condition propagation through the CFG
//
// # Integration
//
// The flow package is used by the type checker to get refined types at each
// program point. The solver is invoked after CFG construction and constraint
// extraction, producing SymbolTypes consumed by narrowing and error reporting.
package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnKind classifies the constant value of a return statement for predicate analysis.
// Used to determine if a return is definitively true, false, or dynamic.
type ReturnKind uint8

const (
	ReturnUnknown ReturnKind = iota
	ReturnTrue               // return true
	ReturnFalse              // return false
)

// ConstKind classifies the type of a constant value tracked for type narrowing.
// Constants are tracked to enable literal-based narrowing (e.g., x == "foo").
type ConstKind uint8

const (
	ConstUnknown ConstKind = iota // 0 = zero value = unknown
	ConstString
	ConstInt
	ConstFloat
	ConstBool
	ConstNil
)

// ConstValue represents a constant value (string, int, float, bool, or nil).
type ConstValue struct {
	Kind  ConstKind
	Str   string
	Int   int64
	Float float64
	Bool  bool
}

// ToLiteralType converts a ConstValue to its corresponding literal type.
func (c *ConstValue) ToLiteralType() typ.Type {
	if c == nil {
		return nil
	}
	switch c.Kind {
	case ConstString:
		return typ.LiteralString(c.Str)
	case ConstInt:
		return typ.LiteralInt(c.Int)
	case ConstFloat:
		return typ.LiteralNumber(c.Float)
	case ConstBool:
		return typ.LiteralBool(c.Bool)
	case ConstNil:
		return typ.Nil
	default:
		return nil
	}
}

// IteratorKind describes the type of iteration for iterator variable derivation.
type IteratorKind int

const (
	IterateIndexed IteratorKind = iota // ipairs-style: (integer, element)
	IterateKeyed                       // pairs-style: (key, value)
)

// IteratorSource stores info for deriving iterator variable types from flow solution.
type IteratorSource struct {
	Path     constraint.Path // Path to iterator source (e.g., array being iterated)
	Kind     IteratorKind    // Type of iteration
	VarIndex int             // 0=key/index, 1=value
}

// UnifiedAssignment describes an assignment in the CFG with typed info.
type UnifiedAssignment struct {
	Point      cfg.Point
	TargetPath constraint.Path
	SourcePath constraint.Path
	Type       typ.Type
	IterSource *IteratorSource // For iterator vars, derives type from source at solve time

	// ContainerElementSource tracks assignments from container methods (e.g., channel:receive())
	// that return element types. At solve time, the type is derived from the container's
	// widened element type instead of using the statically extracted Type field.
	ContainerElementSource *ContainerElementSource

	// MapElementSource tracks assignments from dynamic map index reads (t[k]).
	// At solve time, the type is derived from the map's value type instead of
	// using the statically extracted Type field.
	MapElementSource *MapElementSource
}

// EdgeCondition ties a DNF condition to a control-flow edge.
type EdgeCondition struct {
	From      cfg.Point
	To        cfg.Point
	Condition constraint.Condition
}

// EdgeNumericConstraint ties numeric constraints to a control-flow edge.
type EdgeNumericConstraint struct {
	From        cfg.Point
	To          cfg.Point
	Constraints []constraint.NumericConstraint
}

// WideningEvent records when preflow inference widens a symbol to Unknown.
type WideningEvent struct {
	Symbol   cfg.SymbolID   // Widened symbol
	SCCIndex int            // Index of the non-converged SCC
	SCC      []cfg.SymbolID // All symbols in the SCC
}

// TypeDecomposer extracts element, key, and value types from composite types.
// Used by the flow solver for iterator and container element derivation
// without depending on query/core.
type TypeDecomposer interface {
	ElementType(t typ.Type) typ.Type
	KeyType(t typ.Type) typ.Type
	ValueType(t typ.Type) typ.Type
}

// Inputs bundles all data needed by the flow solver for type propagation.
// Inputs are AST-free and deterministic, enabling serialization, caching,
// and reproducible analysis across sessions.
type Inputs struct {
	// Graph provides CFG, SSA versioning, and symbol scope information.
	Graph cfg.VersionedGraph

	// Decomposer extracts element/key/value types from composite types.
	Decomposer TypeDecomposer

	// DeclaredTypes maps SymbolID to declared type.
	// All per-variable type data is keyed by SymbolID for proper scope handling.
	DeclaredTypes map[cfg.SymbolID]typ.Type

	// AnnotatedVars tracks variables with explicit type annotations.
	AnnotatedVars map[cfg.SymbolID]bool

	Assignments    []UnifiedAssignment
	ConstValues    map[cfg.SymbolID]map[cfg.Point]*ConstValue
	EdgeConditions []EdgeCondition

	// EdgeNumericConstraints stores numeric constraints from comparison operators.
	// These are fed to theory solvers to detect contradictions.
	EdgeNumericConstraints []EdgeNumericConstraint

	// Type key resolution for HasType constraints.
	TypeKeys map[uint64]typ.Type

	// ReturnKinds classifies return points (true/false/unknown).
	// Populated by checker which has AST access.
	ReturnKinds map[cfg.Point]ReturnKind

	// ReturnConstraints stores constraints extracted from return expressions.
	// For predicate functions, the return expression encodes the constraint.
	// Example: "return type(x) == string" gives OnTrue: HasType{x, string}
	ReturnConstraints map[cfg.Point]ReturnExprConstraints

	// PredicateLinks tracks variables assigned from predicate calls.
	// Key is "varname@defpoint", value contains constraints to apply when var is truthy/falsy.
	// Example: local _, err = Point:is(data) -> err nil implies HasType{data, Point}
	PredicateLinks map[string]PredicateLink

	// SiblingAssignments tracks variables assigned from the same multi-return call.
	// Key is "varname@defpoint", maps to the sibling group.
	// Used for error return pattern where checking err narrows result.
	SiblingAssignments map[SiblingKey]*SiblingAssignment

	// IndexerAssignments tracks dynamic index assignments: t[k] = v with non-const k.
	// Used to widen {} to {[K]: V} based on key/value types.
	IndexerAssignments []IndexerAssignment

	// TableMutatorAssignments tracks table.insert-like mutations that widen
	// array element types (including map values that are arrays).
	TableMutatorAssignments []TableMutatorAssignment

	// ContainerMutatorAssignments tracks container mutations (e.g., channel.send)
	// that widen element types via ContainerElementUnion effects.
	ContainerMutatorAssignments []ContainerMutatorAssignment

	// DeadPoints marks CFG points that are unreachable.
	// Used when a terminating function (one that never returns) is called.
	DeadPoints map[cfg.Point]bool

	// ModuleAliases maps symbol IDs to module paths from require() assignments.
	// Example: local http = require("http_client") -> sym(http) -> "http_client"
	ModuleAliases map[cfg.SymbolID]string

	// FunctionAliases maps local symbols to their source function symbols.
	// For patterns like `local f = B`, maps sym(f) -> sym(B).
	// Used for effect propagation through aliases.
	FunctionAliases map[cfg.SymbolID]cfg.SymbolID

	// SiblingTypes provides captured variable types from parent scope.
	// Explicit overlay, never merged into DeclaredTypes.
	SiblingTypes map[cfg.SymbolID]typ.Type

	// LiteralTypes provides function literal types synthesized in the literal phase.
	// Explicit overlay, never merged into DeclaredTypes.
	LiteralTypes map[cfg.SymbolID]typ.Type

	// WideningEvents records symbols that were widened to Unknown during preflow inference.
	// Used for diagnostics to report precision loss.
	WideningEvents []WideningEvent

	// KeysProvenance tracks variables that contain keys of another table.
	// Key: symbol of variable holding keys (e.g., suite_names)
	// Value: symbol of table the keys came from (e.g., suites)
	// Used to emit KeyOf constraints when iterating over such variables.
	KeysProvenance map[cfg.SymbolID]cfg.SymbolID
}

// ReturnExprConstraints holds constraints extracted from a return expression.
type ReturnExprConstraints struct {
	OnTrue  constraint.Condition
	OnFalse constraint.Condition
}

// PredicateLink stores predicate constraints for a variable assigned from a predicate call.
// Example: local ok = Point:is(data) -> OnTruthy contains HasType{data, Point}
type PredicateLink struct {
	OnTruthy constraint.Condition
	OnFalsy  constraint.Condition
}

// ReturnCorrelation describes a correlated (value, error) pair in a multi-return.
// Derived from effect.ErrorReturn on the callee's spec.
type ReturnCorrelation struct {
	ValueIndex int
	ErrorIndex int
}

// GuardedTypeCorrelation describes branch-sensitive sibling narrowing:
// when guard return at GuardIndex is truthy/falsy (per GuardOnTruthy),
// target return at TargetIndex narrows to TargetType.
type GuardedTypeCorrelation struct {
	GuardIndex    int
	TargetIndex   int
	GuardOnTruthy bool
	TargetType    typ.Type
}

// SiblingAssignment tracks variables assigned from the same multi-return call.
// Used for error return pattern: `local result, err = call()` where checking err narrows result.
type SiblingAssignment struct {
	Symbols             []cfg.SymbolID           // Symbol IDs in order (primary identity)
	Names               []string                 // Variable names (for constraint path construction)
	Types               []typ.Type               // Declared types for each variable
	Correlations        []ReturnCorrelation      // Inverse correlations (ErrorReturn): value nil <-> error non-nil
	CoCorrelations      []ReturnCorrelation      // Same-direction correlations (CorrelatedReturn): all nil or all non-nil
	GuardedCorrelations []GuardedTypeCorrelation // Branch-sensitive type narrowing from guard/result relations
}

// SiblingKey uniquely identifies a variable in a sibling assignment by SymbolID+SSA version.
type SiblingKey struct {
	Symbol    cfg.SymbolID
	VersionID int
}

// IndexerAssignment describes an assignment via dynamic index: t[k] = v
// where k is non-const. Used to widen empty tables to maps.
// KeyVar stores the variable name if key is an identifier; KeyType is resolved
// during flow solving when we have flow-narrowed types.
type IndexerAssignment struct {
	Point     cfg.Point
	Root      string               // Variable name (for display)
	Symbol    cfg.SymbolID         // Unique symbol ID for the root variable
	Segments  []constraint.Segment // Field/index segments for nested tables
	KeyVar    string               // Variable name if key is an identifier
	KeySymbol cfg.SymbolID         // Symbol ID for the key variable (for SSA-aware lookup)
	KeyType   typ.Type             // Optional explicit key type (overrides KeySymbol lookup)
	ValuePath constraint.Path      // Path to value expression for flow-resolved type lookup
	ValType   typ.Type             // Fallback type when ValuePath is unavailable
}

// TableMutatorAssignment describes table.insert-like mutations that widen
// array element types. If KeySymbol/KeyType is set, the target is treated as
// a map index (e.g., suites[suite]) and the map value's array element type is widened.
type TableMutatorAssignment struct {
	Point     cfg.Point
	Target    constraint.Path // Base table path (or base map path for indexed targets)
	KeyVar    string          // Variable name if key is an identifier
	KeySymbol cfg.SymbolID    // Symbol ID for the key variable (for SSA-aware lookup)
	KeyType   typ.Type        // Optional explicit key type (overrides KeySymbol lookup)
	ValuePath constraint.Path // Path to value expression for flow-resolved type lookup
	ValueType typ.Type        // Fallback type if ValuePath doesn't resolve
}

// ContainerMutatorAssignment describes container mutations (channel.send, etc.)
// that widen element types. Uses the ContainerElementUnion effect pattern from specs.
type ContainerMutatorAssignment struct {
	Point     cfg.Point
	Target    constraint.Path // Container path (symbol-only, e.g., channel variable)
	ValuePath constraint.Path // Path to value expression for flow-resolved type lookup
	ValueType typ.Type        // Fallback type if ValuePath doesn't resolve
}

// ContainerElementSource tracks that an assignment's type should be derived
// from a container's element type at solve time. Used for methods like
// channel:receive() where the return type depends on the widened container type.
type ContainerElementSource struct {
	ContainerPath constraint.Path // Path to the container (e.g., channel variable)
	ReturnIndex   int             // Which return value (0-based)
}

// MapElementSource tracks that an assignment's type should be derived from
// a map/table's value type at solve time. Used for dynamic index reads like
// t[k] where k is a non-const variable and the path cannot be statically built.
type MapElementSource struct {
	MapPath   constraint.Path // Path to the map/table being indexed
	KeySymbol cfg.SymbolID    // Symbol for key variable (SSA lookup)
	KeyVar    string          // Key variable name (display)
}

// TypeState tracks the resolution progress of a type during fixed-point iteration.
// The solver uses this to detect convergence and handle cyclic dependencies.
type TypeState uint8

const (
	StateUnknown    TypeState = iota // Type not yet resolved
	StateResolved                    // Type fully resolved
	StatePending                     // Type resolution in progress
	StateConflicted                  // Conflicting type assignments
)

// TypedValue pairs a type with its resolution state.
type TypedValue struct {
	Type  typ.Type
	State TypeState
}

// SymbolTypes is the per-point symbol type map.
type SymbolTypes = map[cfg.Point]map[cfg.SymbolID]TypedValue
