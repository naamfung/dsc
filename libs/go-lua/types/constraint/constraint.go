// Package constraint provides multi-path narrowing constraints for type refinement.
//
// Constraints represent facts learned from conditionals and type guards that
// refine types along control flow paths. Unlike simple single-variable narrowing,
// constraints can relate multiple paths (e.g., "if x == y") enabling richer
// type-level reasoning.
//
// # Core Concepts
//
// Path: An SSA-stable identifier for a value (variable.field.index). Paths track
// identity through control flow using CFG symbol IDs, enabling precise narrowing
// even when the same variable name refers to different values at different points.
//
// Constraint: A narrowing term that refines types when satisfied. Constraints are
// AST-free and deterministic, making them suitable for serialization and caching.
//
// Condition: A DNF (disjunctive normal form) formula of constraint conjunctions,
// representing the complete narrowing information from a conditional expression.
//
// # Constraint Kinds
//
// Single-path constraints:
//   - Truthy/Falsy: Path evaluates to truthy/falsy value
//   - IsNil/NotNil: Path is/is not nil
//   - HasType/NotHasType: Path has/lacks a specific type
//   - HasField: Path has a specific field (narrows unions)
//
// Literal comparison constraints:
//   - FieldEquals/FieldNotEquals: path.field equals/not-equals a literal
//   - IndexEquals/IndexNotEquals: path[key] equals/not-equals a literal
//
// Multi-path constraints:
//   - EqPath/NotEqPath: Two paths are equal/not-equal
//   - FieldEqualsPath/FieldNotEqualsPath: path.field equals/not-equals another path
//   - IndexEqualsPath/IndexNotEqualsPath: path[key] equals/not-equals another path
//   - KeyOf: Key path is a known key of table path
//
// # Solver
//
// The Solver applies constraints to base types, computing narrowed types for
// each path. It handles constraint combination, contradiction detection, and
// produces a PathTypes map ready for use by the type checker.
//
// # Example
//
// For the code:
//
//	if type(x) == "string" then
//	    -- x is narrowed to string
//	end
//
// The constraint HasType{x, string} is applied in the then-branch, and the
// solver narrows x's type from its declared type to string.
package constraint

import (
	"fmt"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// Kind classifies a constraint term for dispatch and serialization.
type Kind uint8

const (
	KindInvalid Kind = iota
	KindTruthy
	KindFalsy
	KindIsNil
	KindNotNil
	KindHasType
	KindNotHasType
	KindHasField
	KindFieldEquals
	KindFieldNotEquals
	KindIndexEquals
	KindIndexNotEquals
	KindEqPath
	KindNotEqPath
	KindFieldEqualsPath
	KindFieldNotEqualsPath
	KindIndexEqualsPath
	KindIndexNotEqualsPath
	KindKeyOf
)

// Constraint is a narrowing term that refines types when satisfied.
//
// Constraints are AST-free, deterministic, and implement structural equality
// for memoization. Each constraint tracks which paths it affects via Paths(),
// enabling efficient incremental narrowing.
//
// Substitute replaces placeholder paths ($0, $1) with concrete argument paths,
// used when applying function refinement constraints at call sites.
type Constraint interface {
	Kind() Kind
	Paths() []Path
	Hash() uint64
	Equals(other Constraint) bool
	Substitute(args []Path) (Constraint, bool)
}

// Truthy requires path to evaluate to a truthy value (not nil and not false).
// Removes nil and false from the path's type in the then-branch.
type Truthy struct {
	Path Path
}

func (c Truthy) Kind() Kind { return KindTruthy }
func (c Truthy) Paths() []Path {
	paths := []Path{c.Path}
	// Also include parent path for field-based union narrowing
	if len(c.Path.Segments) > 0 && c.Path.Segments[len(c.Path.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Path.Root, Symbol: c.Path.Symbol}
		if len(c.Path.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Path.Segments[:len(c.Path.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c Truthy) Hash() uint64 { return hashPathConstraint(c.Kind(), c.Path) }
func (c Truthy) Equals(o Constraint) bool {
	other, ok := o.(Truthy)
	return ok && c.Path.Equal(other.Path)
}
func (c Truthy) String() string { return fmt.Sprintf("truthy(%s)", c.Path.String()) }
func (c Truthy) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return Truthy{Path: p}, true
}

// Falsy requires path to evaluate to a falsy value (nil or false).
// Narrows the path's type to only nil or false in the then-branch.
type Falsy struct {
	Path Path
}

func (c Falsy) Kind() Kind { return KindFalsy }
func (c Falsy) Paths() []Path {
	paths := []Path{c.Path}
	// Also include parent path for field-based union narrowing
	if len(c.Path.Segments) > 0 && c.Path.Segments[len(c.Path.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Path.Root, Symbol: c.Path.Symbol}
		if len(c.Path.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Path.Segments[:len(c.Path.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c Falsy) Hash() uint64 { return hashPathConstraint(c.Kind(), c.Path) }
func (c Falsy) Equals(o Constraint) bool {
	other, ok := o.(Falsy)
	return ok && c.Path.Equal(other.Path)
}
func (c Falsy) String() string { return fmt.Sprintf("falsy(%s)", c.Path.String()) }
func (c Falsy) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return Falsy{Path: p}, true
}

// IsNil requires path to be nil.
type IsNil struct {
	Path Path
}

func (c IsNil) Kind() Kind    { return KindIsNil }
func (c IsNil) Paths() []Path { return []Path{c.Path} }
func (c IsNil) Hash() uint64  { return hashPathConstraint(c.Kind(), c.Path) }
func (c IsNil) Equals(o Constraint) bool {
	other, ok := o.(IsNil)
	return ok && c.Path.Equal(other.Path)
}
func (c IsNil) String() string { return fmt.Sprintf("isnil(%s)", c.Path.String()) }
func (c IsNil) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return IsNil{Path: p}, true
}

// NotNil requires path to be non-nil.
type NotNil struct {
	Path Path
}

func (c NotNil) Kind() Kind    { return KindNotNil }
func (c NotNil) Paths() []Path { return []Path{c.Path} }
func (c NotNil) Hash() uint64  { return hashPathConstraint(c.Kind(), c.Path) }
func (c NotNil) Equals(o Constraint) bool {
	other, ok := o.(NotNil)
	return ok && c.Path.Equal(other.Path)
}
func (c NotNil) String() string { return fmt.Sprintf("notnil(%s)", c.Path.String()) }
func (c NotNil) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return NotNil{Path: p}, true
}

// HasType constrains a path to have a specific runtime type.
// Generated from type(x) == "string" checks and similar guards.
// TypeKey identifies the type category (string, number, table, etc.).
type HasType struct {
	Path Path
	Type narrow.TypeKey
}

func (c HasType) Kind() Kind { return KindHasType }
func (c HasType) Paths() []Path {
	return []Path{c.Path}
}
func (c HasType) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Path)
	return internal.HashCombine(h, c.Type.Hash64())
}
func (c HasType) Equals(o Constraint) bool {
	other, ok := o.(HasType)
	return ok && c.Path.Equal(other.Path) && c.Type.Equal(other.Type)
}
func (c HasType) String() string { return fmt.Sprintf("hastype(%s)", c.Path.String()) }
func (c HasType) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return HasType{Path: p, Type: c.Type}, true
}

// NotHasType constrains a path to NOT have a type key.
// Used for negation of HasType in inequality conditions.
type NotHasType struct {
	Path Path
	Type narrow.TypeKey
}

func (c NotHasType) Kind() Kind { return KindNotHasType }
func (c NotHasType) Paths() []Path {
	return []Path{c.Path}
}
func (c NotHasType) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Path)
	return internal.HashCombine(h, c.Type.Hash64())
}
func (c NotHasType) Equals(o Constraint) bool {
	other, ok := o.(NotHasType)
	return ok && c.Path.Equal(other.Path) && c.Type.Equal(other.Type)
}
func (c NotHasType) String() string { return fmt.Sprintf("nothastype(%s)", c.Path.String()) }
func (c NotHasType) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return NotHasType{Path: p, Type: c.Type}, true
}

// HasField narrows a union to members that have a specific field.
// Used for truthy field access narrowing: `if x.field then` narrows x to types with field.
type HasField struct {
	Path  Path
	Field string
}

func (c HasField) Kind() Kind { return KindHasField }
func (c HasField) Paths() []Path {
	return []Path{c.Path}
}
func (c HasField) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Path)
	return internal.HashCombine(h, internal.FnvString(c.Field))
}
func (c HasField) Equals(o Constraint) bool {
	other, ok := o.(HasField)
	return ok && c.Path.Equal(other.Path) && c.Field == other.Field
}
func (c HasField) String() string { return fmt.Sprintf("hasfield(%s.%s)", c.Path.String(), c.Field) }
func (c HasField) Substitute(args []Path) (Constraint, bool) {
	p, ok := c.Path.Substitute(args)
	if !ok {
		return nil, false
	}

	return HasField{Path: p, Field: c.Field}, true
}

// FieldEquals constrains a field on a path to a literal value.
type FieldEquals struct {
	Target Path
	Field  string
	Value  *typ.Literal
}

func (c FieldEquals) Kind() Kind { return KindFieldEquals }
func (c FieldEquals) Paths() []Path {
	paths := []Path{c.Target}
	// Include parent path for nested field narrowing
	if len(c.Target.Segments) > 0 && c.Target.Segments[len(c.Target.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Target.Root, Symbol: c.Target.Symbol}
		if len(c.Target.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Target.Segments[:len(c.Target.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c FieldEquals) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	h = internal.HashCombine(h, internal.FnvString(c.Field))

	if c.Value != nil {
		h = internal.HashCombine(h, c.Value.Hash())
	}

	return h
}
func (c FieldEquals) Equals(o Constraint) bool {
	other, ok := o.(FieldEquals)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) || c.Field != other.Field {
		return false
	}

	if c.Value == nil || other.Value == nil {
		return c.Value == other.Value
	}

	return c.Value.Equals(other.Value)
}
func (c FieldEquals) String() string { return fmt.Sprintf("field(%s.%s)", c.Target.String(), c.Field) }
func (c FieldEquals) Substitute(args []Path) (Constraint, bool) {
	t, ok := c.Target.Substitute(args)
	if !ok {
		return nil, false
	}

	return FieldEquals{Target: t, Field: c.Field, Value: c.Value}, true
}

// FieldNotEquals constrains a field on a path to NOT equal a literal value.
type FieldNotEquals struct {
	Target Path
	Field  string
	Value  *typ.Literal
}

func (c FieldNotEquals) Kind() Kind { return KindFieldNotEquals }
func (c FieldNotEquals) Paths() []Path {
	paths := []Path{c.Target}
	// Include parent path for nested field exclusion
	if len(c.Target.Segments) > 0 && c.Target.Segments[len(c.Target.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Target.Root, Symbol: c.Target.Symbol}
		if len(c.Target.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Target.Segments[:len(c.Target.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c FieldNotEquals) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	h = internal.HashCombine(h, internal.FnvString(c.Field))

	if c.Value != nil {
		h = internal.HashCombine(h, c.Value.Hash())
	}

	return h
}
func (c FieldNotEquals) Equals(o Constraint) bool {
	other, ok := o.(FieldNotEquals)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) || c.Field != other.Field {
		return false
	}

	if c.Value == nil || other.Value == nil {
		return c.Value == other.Value
	}

	return c.Value.Equals(other.Value)
}
func (c FieldNotEquals) String() string {
	return fmt.Sprintf("fieldNot(%s.%s)", c.Target.String(), c.Field)
}
func (c FieldNotEquals) Substitute(args []Path) (Constraint, bool) {
	t, ok := c.Target.Substitute(args)
	if !ok {
		return nil, false
	}

	return FieldNotEquals{Target: t, Field: c.Field, Value: c.Value}, true
}

// IndexEquals constrains an index on a path to a literal value.
type IndexEquals struct {
	Target Path
	Key    typ.Type
	Value  *typ.Literal
}

func (c IndexEquals) Kind() Kind { return KindIndexEquals }
func (c IndexEquals) Paths() []Path {
	return []Path{c.Target}
}
func (c IndexEquals) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	if c.Key != nil {
		h = internal.HashCombine(h, c.Key.Hash())
	}

	if c.Value != nil {
		h = internal.HashCombine(h, c.Value.Hash())
	}

	return h
}
func (c IndexEquals) Equals(o Constraint) bool {
	other, ok := o.(IndexEquals)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) {
		return false
	}

	if c.Key == nil || other.Key == nil {
		if c.Key != other.Key {
			return false
		}
	} else if !c.Key.Equals(other.Key) {
		return false
	}

	if c.Value == nil || other.Value == nil {
		return c.Value == other.Value
	}

	return c.Value.Equals(other.Value)
}
func (c IndexEquals) String() string {
	if c.Key == nil {
		return fmt.Sprintf("index(%s[?])", c.Target.String())
	}

	return fmt.Sprintf("index(%s[%s])", c.Target.String(), c.Key.String())
}
func (c IndexEquals) Substitute(args []Path) (Constraint, bool) {
	t, ok := c.Target.Substitute(args)
	if !ok {
		return nil, false
	}

	return IndexEquals{Target: t, Key: c.Key, Value: c.Value}, true
}

// IndexNotEquals constrains an index on a path to NOT equal a literal value.
type IndexNotEquals struct {
	Target Path
	Key    typ.Type
	Value  *typ.Literal
}

func (c IndexNotEquals) Kind() Kind { return KindIndexNotEquals }
func (c IndexNotEquals) Paths() []Path {
	return []Path{c.Target}
}
func (c IndexNotEquals) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)

	if c.Key != nil {
		h = internal.HashCombine(h, c.Key.Hash())
	}

	if c.Value != nil {
		h = internal.HashCombine(h, c.Value.Hash())
	}

	return h
}
func (c IndexNotEquals) Equals(o Constraint) bool {
	other, ok := o.(IndexNotEquals)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) {
		return false
	}

	if c.Key == nil || other.Key == nil {
		if c.Key != other.Key {
			return false
		}
	} else if !c.Key.Equals(other.Key) {
		return false
	}

	if c.Value == nil || other.Value == nil {
		return c.Value == other.Value
	}

	return c.Value.Equals(other.Value)
}
func (c IndexNotEquals) String() string {
	if c.Key == nil {
		return fmt.Sprintf("indexNot(%s[?])", c.Target.String())
	}

	return fmt.Sprintf("indexNot(%s[%s])", c.Target.String(), c.Key.String())
}
func (c IndexNotEquals) Substitute(args []Path) (Constraint, bool) {
	t, ok := c.Target.Substitute(args)
	if !ok {
		return nil, false
	}

	return IndexNotEquals{Target: t, Key: c.Key, Value: c.Value}, true
}

// EqPath constrains two paths to have equal values at runtime.
// When satisfied, the types of both paths can be intersected since they
// must be compatible. Canonicalized by ordering endpoints for stable hashing.
type EqPath struct {
	Left  Path
	Right Path
}

func NewEqPath(left, right Path) EqPath {
	if right.Less(left) {
		return EqPath{Left: right, Right: left}
	}

	return EqPath{Left: left, Right: right}
}

func (c EqPath) Kind() Kind { return KindEqPath }
func (c EqPath) Paths() []Path {
	return []Path{c.Left, c.Right}
}
func (c EqPath) Hash() uint64 {
	h := internal.HashCombine(uint64(c.Kind()), c.Left.Hash())
	return internal.HashCombine(h, c.Right.Hash())
}
func (c EqPath) Equals(o Constraint) bool {
	other, ok := o.(EqPath)
	return ok && c.Left.Equal(other.Left) && c.Right.Equal(other.Right)
}
func (c EqPath) String() string {
	return fmt.Sprintf("eq(%s,%s)", c.Left.String(), c.Right.String())
}
func (c EqPath) Substitute(args []Path) (Constraint, bool) {
	l, okL := c.Left.Substitute(args)
	r, okR := c.Right.Substitute(args)

	if !okL || !okR {
		return nil, false
	}

	return NewEqPath(l, r), true
}

// NotEqPath constrains two paths to NOT be equal.
// Used for negation of EqPath in inequality conditions.
type NotEqPath struct {
	Left  Path
	Right Path
}

func NewNotEqPath(left, right Path) NotEqPath {
	if right.Less(left) {
		return NotEqPath{Left: right, Right: left}
	}

	return NotEqPath{Left: left, Right: right}
}

func (c NotEqPath) Kind() Kind { return KindNotEqPath }
func (c NotEqPath) Paths() []Path {
	return []Path{c.Left, c.Right}
}
func (c NotEqPath) Hash() uint64 {
	h := internal.HashCombine(uint64(c.Kind()), c.Left.Hash())
	return internal.HashCombine(h, c.Right.Hash())
}
func (c NotEqPath) Equals(o Constraint) bool {
	other, ok := o.(NotEqPath)
	return ok && c.Left.Equal(other.Left) && c.Right.Equal(other.Right)
}
func (c NotEqPath) String() string {
	return fmt.Sprintf("noteq(%s,%s)", c.Left.String(), c.Right.String())
}
func (c NotEqPath) Substitute(args []Path) (Constraint, bool) {
	l, okL := c.Left.Substitute(args)
	r, okR := c.Right.Substitute(args)

	if !okL || !okR {
		return nil, false
	}

	return NewNotEqPath(l, r), true
}

// FieldEqualsPath constrains target.field to equal a value path.
type FieldEqualsPath struct {
	Target Path
	Field  string
	Value  Path
}

func (c FieldEqualsPath) Kind() Kind { return KindFieldEqualsPath }
func (c FieldEqualsPath) Paths() []Path {
	paths := []Path{c.Target, c.Value}
	// Include parent path for nested field narrowing
	if len(c.Target.Segments) > 0 && c.Target.Segments[len(c.Target.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Target.Root, Symbol: c.Target.Symbol}
		if len(c.Target.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Target.Segments[:len(c.Target.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c FieldEqualsPath) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	h = internal.HashCombine(h, internal.FnvString(c.Field))

	return internal.HashCombine(h, c.Value.Hash())
}
func (c FieldEqualsPath) Equals(o Constraint) bool {
	other, ok := o.(FieldEqualsPath)
	return ok && c.Target.Equal(other.Target) && c.Field == other.Field && c.Value.Equal(other.Value)
}
func (c FieldEqualsPath) String() string {
	return fmt.Sprintf("fieldEq(%s.%s,%s)", c.Target.String(), c.Field, c.Value.String())
}
func (c FieldEqualsPath) Substitute(args []Path) (Constraint, bool) {
	t, okT := c.Target.Substitute(args)
	v, okV := c.Value.Substitute(args)

	if !okT || !okV {
		return nil, false
	}

	return FieldEqualsPath{Target: t, Field: c.Field, Value: v}, true
}

// FieldNotEqualsPath constrains target.field to NOT equal a value path.
// Used for else-branches in channel select narrowing.
type FieldNotEqualsPath struct {
	Target Path
	Field  string
	Value  Path
}

func (c FieldNotEqualsPath) Kind() Kind { return KindFieldNotEqualsPath }
func (c FieldNotEqualsPath) Paths() []Path {
	paths := []Path{c.Target, c.Value}
	// Include parent path for nested field exclusion
	if len(c.Target.Segments) > 0 && c.Target.Segments[len(c.Target.Segments)-1].Kind == SegmentField {
		parent := Path{Root: c.Target.Root, Symbol: c.Target.Symbol}
		if len(c.Target.Segments) > 1 {
			// Copy segments to avoid slice aliasing
			parent.Segments = append([]Segment(nil), c.Target.Segments[:len(c.Target.Segments)-1]...)
		}
		paths = append(paths, parent)
	}
	return paths
}
func (c FieldNotEqualsPath) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	h = internal.HashCombine(h, internal.FnvString(c.Field))

	return internal.HashCombine(h, c.Value.Hash())
}
func (c FieldNotEqualsPath) Equals(o Constraint) bool {
	other, ok := o.(FieldNotEqualsPath)
	return ok && c.Target.Equal(other.Target) && c.Field == other.Field && c.Value.Equal(other.Value)
}
func (c FieldNotEqualsPath) String() string {
	return fmt.Sprintf("fieldNotEq(%s.%s,%s)", c.Target.String(), c.Field, c.Value.String())
}
func (c FieldNotEqualsPath) Substitute(args []Path) (Constraint, bool) {
	t, okT := c.Target.Substitute(args)
	v, okV := c.Value.Substitute(args)

	if !okT || !okV {
		return nil, false
	}

	return FieldNotEqualsPath{Target: t, Field: c.Field, Value: v}, true
}

// IndexEqualsPath constrains target[key] to equal a value path.
type IndexEqualsPath struct {
	Target Path
	Key    typ.Type
	Value  Path
}

func (c IndexEqualsPath) Kind() Kind { return KindIndexEqualsPath }
func (c IndexEqualsPath) Paths() []Path {
	return []Path{c.Target, c.Value}
}
func (c IndexEqualsPath) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	if c.Key != nil {
		h = internal.HashCombine(h, c.Key.Hash())
	}

	return internal.HashCombine(h, c.Value.Hash())
}
func (c IndexEqualsPath) Equals(o Constraint) bool {
	other, ok := o.(IndexEqualsPath)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) || !c.Value.Equal(other.Value) {
		return false
	}

	if c.Key == nil || other.Key == nil {
		return c.Key == other.Key
	}

	return c.Key.Equals(other.Key)
}
func (c IndexEqualsPath) String() string {
	if c.Key == nil {
		return fmt.Sprintf("indexEq(%s[?],%s)", c.Target.String(), c.Value.String())
	}

	return fmt.Sprintf("indexEq(%s[%s],%s)", c.Target.String(), c.Key.String(), c.Value.String())
}
func (c IndexEqualsPath) Substitute(args []Path) (Constraint, bool) {
	t, okT := c.Target.Substitute(args)
	v, okV := c.Value.Substitute(args)

	if !okT || !okV {
		return nil, false
	}

	return IndexEqualsPath{Target: t, Key: c.Key, Value: v}, true
}

// IndexNotEqualsPath constrains target[key] to NOT equal a value path.
type IndexNotEqualsPath struct {
	Target Path
	Key    typ.Type
	Value  Path
}

func (c IndexNotEqualsPath) Kind() Kind { return KindIndexNotEqualsPath }
func (c IndexNotEqualsPath) Paths() []Path {
	return []Path{c.Target, c.Value}
}
func (c IndexNotEqualsPath) Hash() uint64 {
	h := hashPathConstraint(c.Kind(), c.Target)
	if c.Key != nil {
		h = internal.HashCombine(h, c.Key.Hash())
	}

	return internal.HashCombine(h, c.Value.Hash())
}
func (c IndexNotEqualsPath) Equals(o Constraint) bool {
	other, ok := o.(IndexNotEqualsPath)
	if !ok {
		return false
	}

	if !c.Target.Equal(other.Target) || !c.Value.Equal(other.Value) {
		return false
	}

	if c.Key == nil || other.Key == nil {
		return c.Key == other.Key
	}

	return c.Key.Equals(other.Key)
}
func (c IndexNotEqualsPath) String() string {
	if c.Key == nil {
		return fmt.Sprintf("indexNotEq(%s[?],%s)", c.Target.String(), c.Value.String())
	}

	return fmt.Sprintf("indexNotEq(%s[%s],%s)", c.Target.String(), c.Key.String(), c.Value.String())
}
func (c IndexNotEqualsPath) Substitute(args []Path) (Constraint, bool) {
	t, okT := c.Target.Substitute(args)
	v, okV := c.Value.Substitute(args)

	if !okT || !okV {
		return nil, false
	}

	return IndexNotEqualsPath{Target: t, Key: c.Key, Value: v}, true
}

// KeyOf constrains a key path to be a known key of a table path.
// When satisfied, index access table[key] is guaranteed to return a value,
// allowing removal of the optional wrapper from the result type.
// Generated from patterns like: for k in pairs(t) do t[k] end
type KeyOf struct {
	Table Path
	Key   Path
}

func (c KeyOf) Kind() Kind { return KindKeyOf }
func (c KeyOf) Paths() []Path {
	return []Path{c.Table, c.Key}
}
func (c KeyOf) Hash() uint64 {
	h := internal.HashCombine(uint64(c.Kind()), c.Table.Hash())
	return internal.HashCombine(h, c.Key.Hash())
}
func (c KeyOf) Equals(o Constraint) bool {
	other, ok := o.(KeyOf)
	return ok && c.Table.Equal(other.Table) && c.Key.Equal(other.Key)
}
func (c KeyOf) String() string {
	return fmt.Sprintf("keyof(%s,%s)", c.Table.String(), c.Key.String())
}
func (c KeyOf) Substitute(args []Path) (Constraint, bool) {
	t, okT := c.Table.Substitute(args)
	k, okK := c.Key.Substitute(args)

	if !okT || !okK {
		return nil, false
	}

	return KeyOf{Table: t, Key: k}, true
}

func hashPathConstraint(kind Kind, path Path) uint64 {
	h := internal.HashCombine(uint64(kind), path.Hash())
	return h
}
