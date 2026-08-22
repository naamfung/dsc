package constraint

import (
	"fmt"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/narrow"
)

// TermKind classifies term variants.
type TermKind uint8

const (
	TermKindInvalid TermKind = iota
	TermKindVar              // variable path reference
	TermKindLen              // length of array/table
	TermKindConst            // integer constant
	TermKindNil              // nil value
)

// Term represents a value in the constraint language.
type Term struct {
	Kind  TermKind
	Path  PathKey // for Var and Len
	Const int64   // for Const
}

// TermVar creates a variable term from a path key.
func TermVar(key PathKey) Term {
	return Term{Kind: TermKindVar, Path: key}
}

// TermLen creates a length term from a path key.
func TermLen(key PathKey) Term {
	return Term{Kind: TermKindLen, Path: key}
}

// TermConst creates a constant term from an integer value.
func TermConst(c int64) Term {
	return Term{Kind: TermKindConst, Const: c}
}

// TermNil returns the nil term.
func TermNil() Term {
	return Term{Kind: TermKindNil}
}

func (t Term) IsVar() bool   { return t.Kind == TermKindVar }
func (t Term) IsLen() bool   { return t.Kind == TermKindLen }
func (t Term) IsConst() bool { return t.Kind == TermKindConst }
func (t Term) IsNil() bool   { return t.Kind == TermKindNil }

func (t Term) Hash() uint64 {
	h := uint64(t.Kind)
	switch t.Kind {
	case TermKindVar, TermKindLen:
		h = internal.HashCombine(h, internal.FnvString(string(t.Path)))
	case TermKindConst:
		h = internal.HashCombine(h, uint64(t.Const))
	}
	return h
}

func (t Term) Equal(other Term) bool {
	if t.Kind != other.Kind {
		return false
	}
	switch t.Kind {
	case TermKindVar, TermKindLen:
		return t.Path == other.Path
	case TermKindConst:
		return t.Const == other.Const
	case TermKindNil:
		return true
	}
	return false
}

func (t Term) String() string {
	switch t.Kind {
	case TermKindVar:
		return string(t.Path)
	case TermKindLen:
		return fmt.Sprintf("len(%s)", t.Path)
	case TermKindConst:
		return fmt.Sprintf("%d", t.Const)
	case TermKindNil:
		return "nil"
	}
	return "<invalid>"
}

// Paths returns all path keys referenced by this term.
func (t Term) Paths() []PathKey {
	switch t.Kind {
	case TermKindVar, TermKindLen:
		return []PathKey{t.Path}
	}
	return nil
}

// AtomKind classifies atom variants.
type AtomKind uint8

const (
	AtomKindInvalid    AtomKind = iota
	AtomKindEq                  // X == Y
	AtomKindNe                  // X != Y
	AtomKindLt                  // X < Y
	AtomKindLe                  // X <= Y
	AtomKindGt                  // X > Y
	AtomKindGe                  // X >= Y
	AtomKindHasType             // X has type T
	AtomKindNotHasType          // X does not have type T
	AtomKindTruthy              // X is truthy
	AtomKindFalsy               // X is falsy
	AtomKindModEq               // X % M == R
)

// Atom is a unified atomic constraint in the constraint language.
type Atom struct {
	Kind    AtomKind
	Left    Term           // primary term
	Right   Term           // second term (for binary relations)
	TypeKey narrow.TypeKey // for HasType/NotHasType
	Mod     int64          // modulus for ModEq
	Rem     int64          // remainder for ModEq
}

// AtomEq creates an equality atom: left == right.
func AtomEq(left, right Term) Atom {
	return Atom{Kind: AtomKindEq, Left: left, Right: right}
}

// AtomNe creates an inequality atom: left != right.
func AtomNe(left, right Term) Atom {
	return Atom{Kind: AtomKindNe, Left: left, Right: right}
}

// AtomLt creates a less-than atom: left < right.
func AtomLt(left, right Term) Atom {
	return Atom{Kind: AtomKindLt, Left: left, Right: right}
}

// AtomLe creates a less-or-equal atom: left <= right.
func AtomLe(left, right Term) Atom {
	return Atom{Kind: AtomKindLe, Left: left, Right: right}
}

// AtomGt creates a greater-than atom: left > right.
func AtomGt(left, right Term) Atom {
	return Atom{Kind: AtomKindGt, Left: left, Right: right}
}

// AtomGe creates a greater-or-equal atom: left >= right.
func AtomGe(left, right Term) Atom {
	return Atom{Kind: AtomKindGe, Left: left, Right: right}
}

// AtomHasType creates a type constraint atom.
func AtomHasType(term Term, key narrow.TypeKey) Atom {
	return Atom{Kind: AtomKindHasType, Left: term, TypeKey: key}
}

// AtomNotHasType creates a negated type constraint atom.
func AtomNotHasType(term Term, key narrow.TypeKey) Atom {
	return Atom{Kind: AtomKindNotHasType, Left: term, TypeKey: key}
}

// AtomTruthy creates a truthy constraint atom.
func AtomTruthy(term Term) Atom {
	return Atom{Kind: AtomKindTruthy, Left: term}
}

// AtomFalsy creates a falsy constraint atom.
func AtomFalsy(term Term) Atom {
	return Atom{Kind: AtomKindFalsy, Left: term}
}

// AtomModEq creates a modular equality atom: term % mod == rem.
func AtomModEq(term Term, mod, rem int64) Atom {
	return Atom{Kind: AtomKindModEq, Left: term, Mod: mod, Rem: rem}
}

func (a Atom) Hash() uint64 {
	h := internal.HashCombine(uint64(a.Kind), a.Left.Hash())
	switch a.Kind {
	case AtomKindEq, AtomKindNe, AtomKindLt, AtomKindLe, AtomKindGt, AtomKindGe:
		h = internal.HashCombine(h, a.Right.Hash())
	case AtomKindHasType, AtomKindNotHasType:
		h = internal.HashCombine(h, a.TypeKey.Hash64())
	case AtomKindModEq:
		h = internal.HashCombine(h, uint64(a.Mod))
		h = internal.HashCombine(h, uint64(a.Rem))
	}
	return h
}

func (a Atom) Equal(other Atom) bool {
	if a.Kind != other.Kind {
		return false
	}
	if !a.Left.Equal(other.Left) {
		return false
	}
	switch a.Kind {
	case AtomKindEq, AtomKindNe, AtomKindLt, AtomKindLe, AtomKindGt, AtomKindGe:
		return a.Right.Equal(other.Right)
	case AtomKindHasType, AtomKindNotHasType:
		return a.TypeKey.Equal(other.TypeKey)
	case AtomKindModEq:
		return a.Mod == other.Mod && a.Rem == other.Rem
	case AtomKindTruthy, AtomKindFalsy:
		return true
	}
	return false
}

func (a Atom) String() string {
	switch a.Kind {
	case AtomKindEq:
		return fmt.Sprintf("%s == %s", a.Left, a.Right)
	case AtomKindNe:
		return fmt.Sprintf("%s != %s", a.Left, a.Right)
	case AtomKindLt:
		return fmt.Sprintf("%s < %s", a.Left, a.Right)
	case AtomKindLe:
		return fmt.Sprintf("%s <= %s", a.Left, a.Right)
	case AtomKindGt:
		return fmt.Sprintf("%s > %s", a.Left, a.Right)
	case AtomKindGe:
		return fmt.Sprintf("%s >= %s", a.Left, a.Right)
	case AtomKindHasType:
		return fmt.Sprintf("hastype(%s)", a.Left)
	case AtomKindNotHasType:
		return fmt.Sprintf("nothastype(%s)", a.Left)
	case AtomKindTruthy:
		return fmt.Sprintf("truthy(%s)", a.Left)
	case AtomKindFalsy:
		return fmt.Sprintf("falsy(%s)", a.Left)
	case AtomKindModEq:
		return fmt.Sprintf("%s %% %d == %d", a.Left, a.Mod, a.Rem)
	}
	return "<invalid>"
}

// Paths returns all path keys referenced by this atom.
func (a Atom) Paths() []PathKey {
	paths := a.Left.Paths()
	if rightPaths := a.Right.Paths(); len(rightPaths) > 0 {
		paths = append(paths, rightPaths...)
	}
	return paths
}

// TypeConstraintToAtom converts a type constraint to a unified Atom.
// Returns (atom, true) for supported constraints, (Atom{}, false) for unsupported.
func TypeConstraintToAtom(c Constraint) (Atom, bool) {
	type result struct {
		atom Atom
		ok   bool
	}
	out := VisitConstraint(c, ConstraintVisitor[result]{
		Truthy: func(v Truthy) result {
			return result{atom: AtomTruthy(TermVar(v.Path.Key())), ok: true}
		},
		Falsy: func(v Falsy) result {
			return result{atom: AtomFalsy(TermVar(v.Path.Key())), ok: true}
		},
		IsNil: func(v IsNil) result {
			return result{atom: AtomEq(TermVar(v.Path.Key()), TermNil()), ok: true}
		},
		NotNil: func(v NotNil) result {
			return result{atom: AtomNe(TermVar(v.Path.Key()), TermNil()), ok: true}
		},
		HasType: func(v HasType) result {
			return result{atom: AtomHasType(TermVar(v.Path.Key()), v.Type), ok: true}
		},
		NotHasType: func(v NotHasType) result {
			return result{atom: AtomNotHasType(TermVar(v.Path.Key()), v.Type), ok: true}
		},
		EqPath: func(v EqPath) result {
			return result{atom: AtomEq(TermVar(v.Left.Key()), TermVar(v.Right.Key())), ok: true}
		},
		NotEqPath: func(v NotEqPath) result {
			return result{atom: AtomNe(TermVar(v.Left.Key()), TermVar(v.Right.Key())), ok: true}
		},
		Default: func(Constraint) result {
			return result{}
		},
	})
	// Unsupported: HasField, FieldEquals, FieldNotEquals, IndexEquals, IndexNotEquals,
	// FieldEqualsPath, FieldNotEqualsPath, IndexEqualsPath, IndexNotEqualsPath.
	return out.atom, out.ok
}

// NumericConstraintToAtom converts a numeric constraint to a unified Atom.
// Returns (atom, true) for supported constraints, (Atom{}, false) for unsupported.
func NumericConstraintToAtom(c NumericConstraint) (Atom, bool) {
	type result struct {
		atom Atom
		ok   bool
	}
	out := VisitNumericConstraint(c, NumericConstraintVisitor[result]{
		Lt: func(v Lt) result {
			return result{atom: AtomLt(TermVar(v.X.Key()), TermVar(v.Y.Key())), ok: true}
		},
		Le: func(v Le) result {
			// Le represents x - y <= c, which is x <= y + c.
			// For simplicity, when c == 0, this is just x <= y.
			if v.C == 0 {
				return result{atom: AtomLe(TermVar(v.X.Key()), TermVar(v.Y.Key())), ok: true}
			}
			return result{}
		},
		Gt: func(v Gt) result {
			return result{atom: AtomGt(TermVar(v.X.Key()), TermVar(v.Y.Key())), ok: true}
		},
		Ge: func(v Ge) result {
			return result{atom: AtomGe(TermVar(v.X.Key()), TermVar(v.Y.Key())), ok: true}
		},
		Eq: func(v Eq) result {
			return result{atom: AtomEq(TermVar(v.X.Key()), TermVar(v.Y.Key())), ok: true}
		},
		EqConst: func(v EqConst) result {
			return result{atom: AtomEq(TermVar(v.X.Key()), TermConst(v.C)), ok: true}
		},
		LeConst: func(v LeConst) result {
			return result{atom: AtomLe(TermVar(v.X.Key()), TermConst(v.C)), ok: true}
		},
		GeConst: func(v GeConst) result {
			return result{atom: AtomGe(TermVar(v.X.Key()), TermConst(v.C)), ok: true}
		},
		ModEq: func(v ModEq) result {
			return result{atom: AtomModEq(TermVar(v.X.Key()), v.M, v.R), ok: true}
		},
		LeLenOf: func(v LeLenOf) result {
			return result{atom: AtomLe(TermVar(v.X.Key()), TermLen(v.Array.Key())), ok: true}
		},
		Default: func(NumericConstraint) result {
			return result{}
		},
	})
	return out.atom, out.ok
}

// AtomResult holds the result of converting constraints to atoms.
type AtomResult struct {
	Atoms    []Atom       // successfully converted atoms
	Leftover []Constraint // constraints that couldn't be converted
}

// ToAtoms converts type constraints to unified Atoms.
// Constraints that don't map cleanly are returned in Leftover.
// Uses path.Key() for path resolution (symbol-only keys).
func ToAtoms(constraints []Constraint) AtomResult {
	var result AtomResult
	for _, c := range constraints {
		if atom, ok := TypeConstraintToAtom(c); ok {
			result.Atoms = append(result.Atoms, atom)
		} else {
			result.Leftover = append(result.Leftover, c)
		}
	}
	return result
}

// ToAtomsWithResolver converts type constraints to unified Atoms using a PathResolver.
// The resolver converts Path objects to canonical PathKeys.
// Constraints that don't map cleanly are returned in Leftover.
func ToAtomsWithResolver(constraints []Constraint, resolve PathResolver) AtomResult {
	if resolve == nil {
		return ToAtoms(constraints)
	}
	var result AtomResult
	for _, c := range constraints {
		if atom, ok := TypeConstraintToAtomWithResolver(c, resolve); ok {
			result.Atoms = append(result.Atoms, atom)
		} else {
			result.Leftover = append(result.Leftover, c)
		}
	}
	return result
}

// TypeConstraintToAtomWithResolver converts a type constraint to a unified Atom using a PathResolver.
// Returns (atom, true) for supported constraints, (Atom{}, false) for unsupported.
func TypeConstraintToAtomWithResolver(c Constraint, resolve PathResolver) (Atom, bool) {
	type result struct {
		atom Atom
		ok   bool
	}
	out := VisitConstraint(c, ConstraintVisitor[result]{
		Truthy: func(v Truthy) result {
			return result{atom: AtomTruthy(TermVar(resolve(v.Path))), ok: true}
		},
		Falsy: func(v Falsy) result {
			return result{atom: AtomFalsy(TermVar(resolve(v.Path))), ok: true}
		},
		IsNil: func(v IsNil) result {
			return result{atom: AtomEq(TermVar(resolve(v.Path)), TermNil()), ok: true}
		},
		NotNil: func(v NotNil) result {
			return result{atom: AtomNe(TermVar(resolve(v.Path)), TermNil()), ok: true}
		},
		HasType: func(v HasType) result {
			return result{atom: AtomHasType(TermVar(resolve(v.Path)), v.Type), ok: true}
		},
		NotHasType: func(v NotHasType) result {
			return result{atom: AtomNotHasType(TermVar(resolve(v.Path)), v.Type), ok: true}
		},
		EqPath: func(v EqPath) result {
			return result{atom: AtomEq(TermVar(resolve(v.Left)), TermVar(resolve(v.Right))), ok: true}
		},
		NotEqPath: func(v NotEqPath) result {
			return result{atom: AtomNe(TermVar(resolve(v.Left)), TermVar(resolve(v.Right))), ok: true}
		},
		Default: func(Constraint) result {
			return result{}
		},
	})
	return out.atom, out.ok
}
