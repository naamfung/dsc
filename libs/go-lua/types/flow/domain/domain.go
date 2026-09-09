// Package domain provides abstract domain interfaces for constraint solving.
//
// The domain package defines the interfaces for type and numeric constraint domains
// used by the flow solver. Each domain processes constraints relevant to its
// abstraction level:
//
//   - TypeDomain: Handles type-based constraints (HasType, NotHasType, Truthy, Falsy)
//   - NumericDomain: Handles numeric constraints (Lt, Le, Gt, Ge, ModEq, Eq, Ne)
//
// The flow solver routes atoms to the appropriate domain based on ClassifyAtom,
// which returns AtomClassType, AtomClassNumeric, or AtomClassBoth.
//
// # Domain Protocol
//
// Domains implement the Domain interface with three methods:
//   - IsUnsat(): Returns true if the domain has reached a contradiction
//   - Clone(): Creates an independent copy for speculative narrowing
//   - Join(): Merges two domain states (used at phi nodes)
//
// Type and numeric domains extend this with ApplyAtom for constraint application.
package domain

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// Domain represents an abstract domain for constraint solving.
type Domain interface {
	IsUnsat() bool
	Clone() Domain
	Join(other Domain) Domain
}

// AtomClass classifies atoms for routing.
type AtomClass int

const (
	AtomClassNone AtomClass = iota
	AtomClassType
	AtomClassNumeric
	AtomClassBoth
)

// ClassifyAtom determines which domain should handle an atom.
func ClassifyAtom(atom constraint.Atom) AtomClass {
	switch atom.Kind {
	case constraint.AtomKindHasType, constraint.AtomKindNotHasType,
		constraint.AtomKindTruthy, constraint.AtomKindFalsy:
		return AtomClassType

	case constraint.AtomKindLt, constraint.AtomKindLe,
		constraint.AtomKindGt, constraint.AtomKindGe,
		constraint.AtomKindModEq:
		return AtomClassNumeric

	case constraint.AtomKindEq, constraint.AtomKindNe:
		if atom.Left.IsNil() || atom.Right.IsNil() {
			return AtomClassType
		}
		if atom.Left.IsConst() || atom.Right.IsConst() {
			return AtomClassNumeric
		}
		return AtomClassBoth
	}
	return AtomClassNone
}

// TypeNarrower provides type narrowing results.
type TypeNarrower interface {
	TypeAt(key constraint.PathKey) typ.Type
	NarrowedTypeAt(key constraint.PathKey) typ.Type
	ApplyAtom(atom constraint.Atom) bool
	IsUnsat() bool
}

// NumericNarrower provides numeric constraint results.
type NumericNarrower interface {
	ApplyAtom(atom constraint.Atom) bool
	IsUnsat() bool
	BoundsFor(key constraint.PathKey) (lower, upper int64, ok bool)
}
