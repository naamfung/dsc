// type_facts.go provides phase-safe type access for the type checker.
//
// During type checking, it's critical to distinguish between declared types
// (from annotations) and refined types (from flow analysis). Mixing these
// can cause "early synthesis poisoning" where prematurely narrowed types
// influence later analysis incorrectly.
//
// The TypeFacts interface provides clean separation:
//   - DeclaredAt: Only annotation-based types
//   - RefinedAt: Only flow-narrowed types
//   - EffectiveTypeAt: Best available type for practical use
package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeFacts provides phase-safe type access by separating declared types from refined types.
//
// The interface has three access methods with distinct semantics:
//
//   - DeclaredAt: Returns type from annotations only (SiblingTypes > DeclaredTypes).
//     Never includes types synthesized from RHS expressions.
//
//   - RefinedAt: Returns type from flow analysis only. Returns nil Type if the
//     symbol has no flow-narrowed type at this point.
//
//   - EffectiveTypeAt: Returns refined type if available, else declared type.
//     This is the practical "best known type" for error checking.
//
// This separation prevents early synthesis poisoning where:
//  1. A variable gets an imprecise type from RHS before narrowing
//  2. That type influences constraint extraction
//  3. Constraints based on the imprecise type cause false errors or missed errors
type TypeFacts interface {
	// DeclaredAt returns the declared (annotated) type for a symbol at a point.
	// Returns typ.Unknown if no declaration exists.
	DeclaredAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// RefinedAt returns the flow-narrowed type for a symbol at a point.
	// Returns TypedValue with nil Type if no refinement exists.
	RefinedAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// EffectiveTypeAt returns the best available type: refined if exists, else declared, else unknown.
	EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// IsAnnotated returns true if the symbol has an explicit type annotation.
	IsAnnotated(sym cfg.SymbolID) bool
}

// DeclaredTypes maps SymbolID to its declared type.
//
// This type alias documents that the map should contain only annotation-derived
// types, not types synthesized from expression analysis. This distinction is
// enforced by the code that populates the map, not by the type system.
type DeclaredTypes = map[cfg.SymbolID]typ.Type

// Compile-time check that Solution implements TypeFacts.
var _ TypeFacts = (*Solution)(nil)

// DeclaredAt returns the declared (annotated) type for a symbol.
// Lookup order: LiteralTypes > SiblingTypes (captured vars) > DeclaredTypes.
// Returns typ.Unknown if no declaration exists.
func (s *Solution) DeclaredAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	if s == nil || s.inputs == nil || sym == 0 {
		return TypedValue{Type: typ.Unknown, State: StateUnknown}
	}
	// For explicitly annotated symbols, prefer the declared type over literal overlays.
	if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
		if t := s.inputs.DeclaredTypes[sym]; t != nil {
			return TypedValue{Type: t, State: StateResolved}
		}
	}
	// Check literal types first (function literals synthesized in current scope),
	// but do not override explicit annotations.
	if s.inputs.AnnotatedVars == nil || !s.inputs.AnnotatedVars[sym] {
		if t := s.inputs.LiteralTypes[sym]; t != nil {
			return TypedValue{Type: t, State: StateResolved}
		}
	}
	// Check sibling types (captured variables from parent scope)
	if t := s.inputs.SiblingTypes[sym]; t != nil {
		return TypedValue{Type: t, State: StateResolved}
	}
	if t := s.inputs.DeclaredTypes[sym]; t != nil {
		return TypedValue{Type: t, State: StateResolved}
	}
	return TypedValue{Type: typ.Unknown, State: StateUnknown}
}

// RefinedAt returns the flow-narrowed type for a symbol at a point.
// Returns TypedValue with nil Type if no refinement exists.
func (s *Solution) RefinedAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	if s == nil || s.inputs == nil || s.pkResolver == nil || sym == 0 {
		return TypedValue{Type: nil, State: StateUnknown}
	}

	// Use canonical key for lookup
	path := constraint.Path{Symbol: sym}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return TypedValue{Type: nil, State: StateUnknown}
	}

	// Check if we have a narrowed value for this version
	if t := s.values[string(key)]; t != nil {
		narrowed := s.NarrowedTypeAt(p, path)
		if narrowed != nil {
			return TypedValue{Type: narrowed, State: StateResolved}
		}
		return TypedValue{Type: t, State: StateResolved}
	}

	return TypedValue{Type: nil, State: StateUnknown}
}

// EffectiveTypeAt returns the best available type: refined if exists, else declared.
func (s *Solution) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	refined := s.RefinedAt(p, sym)
	if refined.Type != nil {
		// For annotated symbols, only accept refinements that are subtypes of the declared type.
		// This prevents unsound narrowing that drops required fields from annotated records.
		if s != nil && s.inputs != nil && s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
			declared := s.DeclaredAt(p, sym)
			if declared.Type != nil && declared.State == StateResolved {
				if !subtype.IsSubtype(refined.Type, declared.Type) {
					return declared
				}
				// If the declared type is not optional/union, keep its structural kind authoritative.
				declaredBase := unwrap.Alias(declared.Type)
				if _, ok := declaredBase.(*typ.Optional); !ok {
					if _, ok := declaredBase.(*typ.Union); !ok {
						if refined.Type.Kind() != declaredBase.Kind() {
							return declared
						}
					}
				}
			}
		}
		return refined
	}
	return s.DeclaredAt(p, sym)
}

// IsAnnotated returns true if the symbol has an explicit type annotation.
func (s *Solution) IsAnnotated(sym cfg.SymbolID) bool {
	if s == nil || s.inputs == nil {
		return false
	}
	return s.inputs.AnnotatedVars[sym]
}
