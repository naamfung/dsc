// Package scope provides immutable lexical scope state with HAMT-based symbol tables.
//
// This package implements the lexical scoping model for the Lua type checker. Each State
// represents a snapshot of the type namespace and lexical metadata at a specific point
// in code. States are immutable and support efficient structural sharing through
// Hash Array Mapped Tries (HAMTs), enabling cheap copy-on-write semantics.
//
// # Architecture
//
// The scope state tracks two categories of information:
//
// Type Namespace: Type definitions and type parameters visible at this scope level.
// Type definitions are created by `type` annotations and propagate to child scopes.
// Type parameters are bound by generic function definitions.
//
// Lexical Metadata: Local variable declarations and mutation tracking. Local names
// indicate which variables are declared in the current scope (vs inherited). Mutation
// tracking records which variables have been assigned after their initial declaration,
// enabling flow-sensitive analysis.
//
// # Value Type Separation
//
// Value types (the types of variables at each program point) are NOT stored in scope.
// They are stored externally in flow.DeclaredTypes, which provides flow-sensitive
// type maps. This separation allows the scope state to remain stable across flow
// iterations while value types evolve during fixpoint computation.
//
// # Immutability and Structural Sharing
//
// All State methods that modify state return a new State instance. The underlying
// HAMT data structures share unchanged nodes between old and new states, making
// modifications O(log n) in both time and space. This enables efficient snapshotting
// and backtracking during type analysis.
//
// # Scope Hierarchy
//
// Scopes form a tree structure through parent links. Each function body, block, and
// control flow construct can introduce a new scope. Type definitions propagate down
// (children see parent types), while local declarations do not (each scope tracks
// only its own locals).
package scope

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/typ"
)

var scopeCounter uint64
var scopeStampCounter uint64

func nextScopeID() uint64 {
	return atomic.AddUint64(&scopeCounter, 1)
}

func nextScopeStamp() uint64 {
	return atomic.AddUint64(&scopeStampCounter, 1)
}

// State is immutable lexical scope state.
//
// State contains the type namespace (type definitions, type parameters) and lexical
// metadata (local declarations, mutation markers). Value types are stored separately
// in flow.DeclaredTypes to support flow-sensitive analysis.
//
// All modification methods return new State instances. The underlying HAMT structures
// provide O(log n) copy-on-write semantics with structural sharing.
//
// # Scope Metadata
//
// Each State has a unique ID (for identity), a stamp (for content versioning), a depth
// (nesting level from root), an optional name, and a parent link. The ID never changes
// for a logical scope, while the stamp updates on any content modification.
//
// # Function Context
//
// States also carry function-level context: self type (for method definitions),
// variadic type (for ... parameters), and expected return types. These propagate
// to child scopes via Child() and can be overridden with With* methods.
type State struct {
	// Type namespace (complete - includes inherited)
	types      *internal.HAMT[string, typ.Type]
	typeParams *internal.HAMT[string, typ.Type]

	// Lexical metadata
	locals  *internal.HAMT[string, bool]
	mutated *internal.HAMT[string, bool]

	// Scope metadata
	id     uint64
	stamp  uint64
	depth  int
	name   string
	parent *State

	// Function context
	selfType     typ.Type
	variadicType typ.Type
	returnTypes  []typ.Type

	// Cached hash (computed lazily, State is immutable after creation)
	cachedHash uint64
}

// New creates an empty root scope.
func New() *State {
	return &State{
		types:      internal.New[string, typ.Type](),
		typeParams: internal.New[string, typ.Type](),
		locals:     internal.New[string, bool](),
		mutated:    internal.New[string, bool](),
		id:         nextScopeID(),
		stamp:      nextScopeStamp(),
		depth:      0,
		parent:     nil,
	}
}

// NewWithBuiltins creates a root scope with builtin type definitions.
func NewWithBuiltins() *State {
	s := New()
	s = s.WithType("any", typ.Any)
	s = s.WithType("nil", typ.Nil)
	s = s.WithType("boolean", typ.Boolean)
	s = s.WithType("bool", typ.Boolean)
	s = s.WithType("number", typ.Number)
	s = s.WithType("integer", typ.Integer)
	s = s.WithType("int", typ.Integer)
	s = s.WithType("string", typ.String)
	s = s.WithType("table", typ.NewInterface("table", nil))

	return s
}

// ID returns the unique scope identifier.
func (s *State) ID() uint64 {
	if s == nil {
		return 0
	}
	return s.id
}

// Stamp returns the stable content stamp for this state instance.
func (s *State) Stamp() uint64 {
	if s == nil {
		return 0
	}
	return s.stamp
}

// Hash returns a stable content hash for this scope.
//
// The hash incorporates parent hash, depth, name, function context (self type,
// variadic type, return types), and the contents of type/typeParam/local/mutated
// maps. Two scopes with identical content produce identical hashes.
//
// The hash is computed lazily and cached. Since State is immutable after creation,
// the cached value remains valid for the lifetime of the State.
func (s *State) Hash() uint64 {
	if s == nil {
		return 0
	}
	if h := atomic.LoadUint64(&s.cachedHash); h != 0 {
		return h
	}
	h := s.computeHash()
	atomic.StoreUint64(&s.cachedHash, h)
	return h
}

// GroupHash returns the hash used to group sibling functions.
//
// Functions defined in the same parent scope share a group hash, enabling the
// type checker to identify and process mutually recursive function groups together.
// The group hash is the parent scope's content hash, ensuring that functions
// see consistent types for their siblings.
//
// For root-level scopes (no parent), the scope's own hash is used as the group hash.
// This maintains the invariant that all scopes have a valid group hash.
func (s *State) GroupHash() uint64 {
	if s == nil {
		return 0
	}
	groupParent := s.parent
	if groupParent == nil {
		groupParent = s
	}
	return groupParent.Hash()
}

func (s *State) computeHash() uint64 {
	var h uint64 = internal.FnvOffset64
	if s.parent != nil {
		h = internal.HashCombine(h, s.parent.Hash())
	}
	h = internal.HashCombine(h, uint64(s.depth))
	if s.name != "" {
		h = internal.HashCombine(h, internal.FnvString(s.name))
	}
	h = internal.HashCombine(h, typeHash(s.selfType))
	h = internal.HashCombine(h, typeHash(s.variadicType))
	for _, t := range s.returnTypes {
		h = internal.HashCombine(h, typeHash(t))
	}
	h = internal.HashCombine(h, hashTypeMap(s.RangeTypes))
	h = internal.HashCombine(h, hashTypeMap(s.RangeTypeParams))
	h = internal.HashCombine(h, hashStringSet(s.RangeLocals))
	h = internal.HashCombine(h, hashStringSet(s.RangeMutations))
	return h
}

// Depth returns scope nesting depth (0 for root).
func (s *State) Depth() int {
	if s == nil {
		return 0
	}
	return s.depth
}

// Name returns the scope name.
func (s *State) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Parent returns the parent scope, if any.
func (s *State) Parent() *State {
	if s == nil {
		return nil
	}
	return s.parent
}

// Child creates a child scope inheriting type namespace.
//
// The child scope receives:
//   - All type definitions from the parent (inherited)
//   - All type parameters from the parent (inherited)
//   - Function context: self type, variadic type, return types (inherited)
//   - Mutation markers from the parent (mutations propagate up)
//
// The child scope does NOT inherit:
//   - Local declarations (each scope tracks only its own locals)
//
// Child scopes are used for function bodies, block statements, and control flow
// constructs. The parent link enables scope chain traversal for debugging and
// group hash computation.
func (s *State) Child() *State {
	if s == nil {
		return New()
	}
	var ret []typ.Type
	if len(s.returnTypes) > 0 {
		ret = make([]typ.Type, len(s.returnTypes))
		copy(ret, s.returnTypes)
	}
	mutated := s.mutated
	if mutated == nil {
		mutated = internal.New[string, bool]()
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       internal.New[string, bool](),
		mutated:      mutated,
		id:           nextScopeID(),
		stamp:        nextScopeStamp(),
		depth:        s.depth + 1,
		name:         s.name,
		parent:       s,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  ret,
	}
}

// IsLocal reports whether the name is declared in the current scope.
func (s *State) IsLocal(name string) bool {
	if s == nil || name == "" {
		return false
	}
	_, ok := s.locals.Get(name)
	return ok
}

// WithLocalName marks a name as locally declared.
func (s *State) WithLocalName(name string) *State {
	if s == nil {
		s = New()
	}
	if name == "" {
		return s
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals.Set(name, true),
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// WithLocalNames marks multiple names as locally declared.
func (s *State) WithLocalNames(names []string) *State {
	if s == nil {
		s = New()
	}
	if len(names) == 0 {
		return s
	}
	locals := s.locals
	for _, name := range names {
		if name != "" {
			locals = locals.Set(name, true)
		}
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// IsMutated reports whether the name was mutated in this scope chain.
func (s *State) IsMutated(name string) bool {
	if s == nil || name == "" {
		return false
	}
	_, ok := s.mutated.Get(name)
	return ok
}

// WithMutated marks a name as mutated.
func (s *State) WithMutated(name string) *State {
	if s == nil {
		s = New()
	}
	if name == "" {
		return s
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated.Set(name, true),
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// WithMutatedNames marks multiple names as mutated.
func (s *State) WithMutatedNames(names []string) *State {
	if s == nil {
		s = New()
	}
	if len(names) == 0 {
		return s
	}
	mutated := s.mutated
	for _, name := range names {
		if name != "" {
			mutated = mutated.Set(name, true)
		}
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// LookupType finds a type definition. O(log n).
func (s *State) LookupType(name string) (typ.Type, bool) {
	if s == nil {
		return nil, false
	}
	return s.types.Get(name)
}

// MetaForName looks up a type definition by name and wraps it in Meta.
func (s *State) MetaForName(name string) *typ.Meta {
	if s == nil {
		return nil
	}
	for sc := s; sc != nil; sc = sc.parent {
		if sc.IsLocal(name) {
			return nil
		}
	}
	if t, ok := s.LookupType(name); ok {
		return typ.NewMeta(t)
	}
	return nil
}

// WithType returns new state with type definition added.
func (s *State) WithType(name string, t typ.Type) *State {
	if s == nil {
		s = New()
	}
	return &State{
		types:        s.types.Set(name, t),
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// WithTypes returns new state with multiple type definitions added.
func (s *State) WithTypes(types map[string]typ.Type) *State {
	if s == nil {
		s = New()
	}
	tables := s.types
	for name, t := range types {
		tables = tables.Set(name, t)
	}
	return &State{
		types:        tables,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// LookupTypeParam finds a type parameter. O(log n).
func (s *State) LookupTypeParam(name string) (typ.Type, bool) {
	if s == nil {
		return nil, false
	}
	return s.typeParams.Get(name)
}

// WithTypeParams returns new state with type parameters added.
func (s *State) WithTypeParams(params map[string]typ.Type) *State {
	if s == nil {
		s = New()
	}
	typeParams := s.typeParams
	for name, t := range params {
		typeParams = typeParams.Set(name, t)
	}
	return &State{
		types:        s.types,
		typeParams:   typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// WithName returns new state with scope name set.
func (s *State) WithName(name string) *State {
	if s == nil {
		s = New()
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// SelfType returns the current self type.
func (s *State) SelfType() typ.Type {
	if s == nil {
		return nil
	}
	return s.selfType
}

// WithSelf returns new state with self type set.
func (s *State) WithSelf(self typ.Type) *State {
	if s == nil {
		s = New()
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     self,
		variadicType: s.variadicType,
		returnTypes:  s.returnTypes,
	}
}

// VariadicType returns the variadic parameter type.
func (s *State) VariadicType() typ.Type {
	if s == nil {
		return nil
	}
	return s.variadicType
}

// WithVariadic returns new state with variadic type set.
func (s *State) WithVariadic(t typ.Type) *State {
	if s == nil {
		s = New()
	}
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: t,
		returnTypes:  s.returnTypes,
	}
}

// ReturnTypes returns expected return types.
func (s *State) ReturnTypes() []typ.Type {
	if s == nil || len(s.returnTypes) == 0 {
		return nil
	}
	ret := make([]typ.Type, len(s.returnTypes))
	copy(ret, s.returnTypes)
	return ret
}

// WithReturn returns new state with return types set.
func (s *State) WithReturn(types []typ.Type) *State {
	if s == nil {
		s = New()
	}
	ret := make([]typ.Type, len(types))
	copy(ret, types)
	return &State{
		types:        s.types,
		typeParams:   s.typeParams,
		locals:       s.locals,
		mutated:      s.mutated,
		id:           s.id,
		stamp:        nextScopeStamp(),
		depth:        s.depth,
		name:         s.name,
		parent:       s.parent,
		selfType:     s.selfType,
		variadicType: s.variadicType,
		returnTypes:  ret,
	}
}

// AllLocals returns locally declared names for this scope.
func (s *State) AllLocals() map[string]bool {
	if s == nil {
		return nil
	}
	result := make(map[string]bool)
	s.locals.Range(func(name string, value bool) bool {
		if value {
			result[name] = true
		}
		return true
	})
	return result
}

// AllMutated returns all names marked as mutated in this scope chain.
func (s *State) AllMutated() map[string]bool {
	if s == nil {
		return nil
	}
	result := make(map[string]bool)
	s.mutated.Range(func(name string, value bool) bool {
		if value {
			result[name] = true
		}
		return true
	})
	return result
}

// RangeLocals iterates over local names without allocation.
func (s *State) RangeLocals(fn func(name string) bool) {
	if s == nil || fn == nil {
		return
	}
	s.locals.Range(func(name string, value bool) bool {
		if value {
			return fn(name)
		}
		return true
	})
}

// RangeMutations iterates over mutated names without allocation.
func (s *State) RangeMutations(fn func(name string) bool) {
	if s == nil || fn == nil {
		return
	}
	s.mutated.Range(func(name string, value bool) bool {
		if value {
			return fn(name)
		}
		return true
	})
}

// AllTypes returns all visible type definitions.
func (s *State) AllTypes() map[string]typ.Type {
	if s == nil {
		return nil
	}
	result := make(map[string]typ.Type)
	s.types.Range(func(name string, t typ.Type) bool {
		result[name] = t
		return true
	})
	return result
}

// RangeTypes iterates over all visible type definitions without allocation.
func (s *State) RangeTypes(fn func(name string, t typ.Type) bool) {
	if s == nil || fn == nil {
		return
	}
	s.types.Range(fn)
}

// TypeParams returns all type parameters.
func (s *State) TypeParams() map[string]typ.Type {
	if s == nil {
		return nil
	}
	result := make(map[string]typ.Type)
	s.typeParams.Range(func(name string, t typ.Type) bool {
		result[name] = t
		return true
	})
	return result
}

// RangeTypeParams iterates over all type parameters without allocation.
func (s *State) RangeTypeParams(fn func(name string, t typ.Type) bool) {
	if s == nil || fn == nil {
		return
	}
	s.typeParams.Range(fn)
}

type hashEntry struct {
	name string
	hash uint64
}

func hashTypeMap(rangeFn func(func(name string, t typ.Type) bool)) uint64 {
	if rangeFn == nil {
		return 0
	}
	var entries []hashEntry
	rangeFn(func(name string, t typ.Type) bool {
		entries = append(entries, hashEntry{name: name, hash: typeHash(t)})
		return true
	})
	if len(entries) == 0 {
		return 0
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	var h uint64 = internal.FnvOffset64
	for _, e := range entries {
		h = internal.HashCombine(h, internal.FnvString(e.name))
		h = internal.HashCombine(h, e.hash)
	}
	return h
}

func hashStringSet(rangeFn func(func(name string) bool)) uint64 {
	if rangeFn == nil {
		return 0
	}
	var entries []string
	rangeFn(func(name string) bool {
		entries = append(entries, name)
		return true
	})
	if len(entries) == 0 {
		return 0
	}
	sort.Strings(entries)
	var h uint64 = internal.FnvOffset64
	for _, name := range entries {
		h = internal.HashCombine(h, internal.FnvString(name))
	}
	return h
}

func typeHash(t typ.Type) uint64 {
	if t == nil {
		return 0
	}
	return t.Hash()
}
