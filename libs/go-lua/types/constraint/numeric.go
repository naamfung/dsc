package constraint

import "github.com/wippyai/go-lua/internal"

// Numeric constraints encode arithmetic relationships between paths.
//
// These constraints are used by theory solvers in the [theory] sub-package
// to reason about numeric properties like array bounds and loop invariants.
// Unlike type constraints, numeric constraints operate on values rather than types.
//
// # Supported Constraints
//
//   - Comparisons: [Le] (x-y≤c), [Lt] (x<y), [Ge] (x≥y), [Gt] (x>y), [Eq] (x==y)
//   - Constants: [EqConst] (x==c), [LeConst] (x≤c), [GeConst] (x≥c)
//   - Modular: [ModEq] (x%m==r)
//   - Symbolic: [LeLenOf] (x≤len(arr)+c)
//
// # Usage with Theory Solvers
//
// Numeric constraints are consumed by:
//   - Difference Logic solver: handles Le, Lt, Ge, Gt constraints
//   - Modular Arithmetic solver: handles ModEq constraints
//
// All numeric constraints are AST-free and can be serialized for caching.

// NumKind classifies numeric constraint terms.
type NumKind uint8

const (
	NumInvalid NumKind = iota
	NumLe              // x - y <= c
	NumLt              // x < y
	NumGe              // x >= y
	NumGt              // x > y
	NumEq              // x == y
	NumEqConst         // x == c
	NumLeConst         // x <= c
	NumGeConst         // x >= c
	NumModEq           // x % m == r
	NumLeLenOf         // x <= len(arr) + offset
)

// NumericConstraint is a marker interface for numeric constraints.
type NumericConstraint interface {
	NumKind() NumKind
	Paths() []Path
	Hash() uint64
	Equals(other NumericConstraint) bool
}

// Le represents x - y <= c.
type Le struct {
	X Path
	Y Path
	C int64
}

func (c Le) NumKind() NumKind { return NumLe }
func (c Le) Paths() []Path    { return []Path{c.X, c.Y} }
func (c Le) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Y, c.C) }
func (c Le) Equals(o NumericConstraint) bool {
	other, ok := o.(Le)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y) && c.C == other.C
}

// Lt represents x < y.
type Lt struct {
	X Path
	Y Path
}

func (c Lt) NumKind() NumKind { return NumLt }
func (c Lt) Paths() []Path    { return []Path{c.X, c.Y} }
func (c Lt) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Lt) Equals(o NumericConstraint) bool {
	other, ok := o.(Lt)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Ge represents x >= y.
type Ge struct {
	X Path
	Y Path
}

func (c Ge) NumKind() NumKind { return NumGe }
func (c Ge) Paths() []Path    { return []Path{c.X, c.Y} }
func (c Ge) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Ge) Equals(o NumericConstraint) bool {
	other, ok := o.(Ge)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Gt represents x > y.
type Gt struct {
	X Path
	Y Path
}

func (c Gt) NumKind() NumKind { return NumGt }
func (c Gt) Paths() []Path    { return []Path{c.X, c.Y} }
func (c Gt) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Gt) Equals(o NumericConstraint) bool {
	other, ok := o.(Gt)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// Eq represents x == y.
type Eq struct {
	X Path
	Y Path
}

func (c Eq) NumKind() NumKind { return NumEq }
func (c Eq) Paths() []Path    { return []Path{c.X, c.Y} }
func (c Eq) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Y, 0) }
func (c Eq) Equals(o NumericConstraint) bool {
	other, ok := o.(Eq)
	return ok && c.X.Equal(other.X) && c.Y.Equal(other.Y)
}

// EqConst represents x == c.
type EqConst struct {
	X Path
	C int64
}

func (c EqConst) NumKind() NumKind { return NumEqConst }
func (c EqConst) Paths() []Path    { return []Path{c.X} }
func (c EqConst) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, Path{}, c.C) }
func (c EqConst) Equals(o NumericConstraint) bool {
	other, ok := o.(EqConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// LeConst represents x <= c.
type LeConst struct {
	X Path
	C int64
}

func (c LeConst) NumKind() NumKind { return NumLeConst }
func (c LeConst) Paths() []Path    { return []Path{c.X} }
func (c LeConst) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, Path{}, c.C) }
func (c LeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(LeConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// GeConst represents x >= c.
type GeConst struct {
	X Path
	C int64
}

func (c GeConst) NumKind() NumKind { return NumGeConst }
func (c GeConst) Paths() []Path    { return []Path{c.X} }
func (c GeConst) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, Path{}, c.C) }
func (c GeConst) Equals(o NumericConstraint) bool {
	other, ok := o.(GeConst)
	return ok && c.X.Equal(other.X) && c.C == other.C
}

// ModEq represents x % m == r.
type ModEq struct {
	X Path
	M int64
	R int64
}

func (c ModEq) NumKind() NumKind { return NumModEq }
func (c ModEq) Paths() []Path    { return []Path{c.X} }
func (c ModEq) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, Path{}, c.M, c.R) }
func (c ModEq) Equals(o NumericConstraint) bool {
	other, ok := o.(ModEq)
	return ok && c.X.Equal(other.X) && c.M == other.M && c.R == other.R
}

// LeLenOf represents x <= len(arr)+offset, a symbolic upper bound.
type LeLenOf struct {
	X      Path  // variable being bounded
	Array  Path  // array whose length is the upper bound
	Offset int64 // additive offset (can be negative)
}

func (c LeLenOf) NumKind() NumKind { return NumLeLenOf }
func (c LeLenOf) Paths() []Path    { return []Path{c.X, c.Array} }
func (c LeLenOf) Hash() uint64     { return hashNumConstraint(c.NumKind(), c.X, c.Array, c.Offset) }
func (c LeLenOf) Equals(o NumericConstraint) bool {
	other, ok := o.(LeLenOf)
	return ok && c.X.Equal(other.X) && c.Array.Equal(other.Array) && c.Offset == other.Offset
}

func hashNumConstraint(kind NumKind, a, b Path, extra ...int64) uint64 {
	h := internal.HashCombine(uint64(kind), a.Hash())
	if !b.IsEmpty() {
		h = internal.HashCombine(h, b.Hash())
	}

	for _, v := range extra {
		h = internal.HashCombine(h, uint64(v))
	}

	return h
}
