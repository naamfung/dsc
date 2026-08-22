package sibling

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
)

// ConstraintsForIdent returns sibling constraints derived from correlated
// multi-return assignments using bindings.
//
// wantNonNil means the queried symbol is known non-nil/truthy at this point.
func ConstraintsForIdent(ident *ast.IdentExpr, p cfg.Point, inputs *flow.Inputs, wantNonNil bool) []constraint.Constraint {
	if ident == nil || inputs == nil || inputs.SiblingAssignments == nil || inputs.Graph == nil {
		return nil
	}
	g, ok := inputs.Graph.(*cfg.Graph)
	if !ok {
		return nil
	}
	bindings := g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return nil
	}
	ver := g.VisibleVersion(p, sym)
	if ver.ID == 0 {
		return nil
	}
	return ConstraintsForSymbol(sym, ver.ID, inputs, wantNonNil, bindings)
}

// ConstraintsForSymbol returns sibling constraints given a resolved symbol.
func ConstraintsForSymbol(sym cfg.SymbolID, versionID int, inputs *flow.Inputs, wantNonNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	if sym == 0 || versionID == 0 || inputs == nil || inputs.SiblingAssignments == nil {
		return nil
	}
	key := flow.SiblingKey{Symbol: sym, VersionID: versionID}
	sibling, found := inputs.SiblingAssignments[key]
	if !found || sibling == nil || len(sibling.Names) < 2 {
		return nil
	}
	pos := -1
	for i, s := range sibling.Symbols {
		if s == sym {
			pos = i
			break
		}
	}
	if pos < 0 {
		return nil
	}

	if len(sibling.Correlations) > 0 || len(sibling.CoCorrelations) > 0 || len(sibling.GuardedCorrelations) > 0 {
		var result []constraint.Constraint
		result = append(result, correlatedSiblingConstraints(sibling, pos, wantNonNil, bindings)...)
		result = append(result, coCorrelatedSiblingConstraints(sibling, pos, wantNonNil, bindings)...)
		result = append(result, guardedTypeSiblingConstraints(sibling, pos, wantNonNil, bindings)...)
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

// correlatedSiblingConstraints emits constraints for inverse correlations
// (ErrorReturn): one side nil implies the partner non-nil, and vice versa.
// This is positional-agnostic: it applies symmetrically whether the queried
// symbol is the value slot or the error slot.
func correlatedSiblingConstraints(sibling *flow.SiblingAssignment, pos int, wantNonNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	var result []constraint.Constraint
	for _, cor := range sibling.Correlations {
		partnerIdx := -1
		switch pos {
		case cor.ErrorIndex:
			partnerIdx = cor.ValueIndex
		case cor.ValueIndex:
			partnerIdx = cor.ErrorIndex
		}
		if partnerIdx < 0 || partnerIdx >= len(sibling.Names) {
			continue
		}
		sibName := sibling.Names[partnerIdx]
		if sibName == "" {
			continue
		}
		var sibSym cfg.SymbolID
		if partnerIdx < len(sibling.Symbols) {
			sibSym = sibling.Symbols[partnerIdx]
		}
		root := sibName
		if sibSym != 0 && bindings != nil {
			if name := bindings.Name(sibSym); name != "" {
				root = name
			}
		}
		path := constraint.Path{Root: root, Symbol: sibSym}
		// Inverse relation: current non-nil -> partner nil, current nil -> partner non-nil.
		if wantNonNil {
			result = append(result, constraint.IsNil{Path: path})
		} else {
			result = append(result, constraint.NotNil{Path: path})
		}
	}
	return result
}

// coCorrelatedSiblingConstraints emits constraints for same-direction correlations
// (CorrelatedReturn): one side nil implies partner nil; non-nil implies non-nil.
func coCorrelatedSiblingConstraints(sibling *flow.SiblingAssignment, pos int, wantNonNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	var result []constraint.Constraint
	for _, cor := range sibling.CoCorrelations {
		partnerIdx := -1
		switch pos {
		case cor.ValueIndex:
			partnerIdx = cor.ErrorIndex
		case cor.ErrorIndex:
			partnerIdx = cor.ValueIndex
		}
		if partnerIdx < 0 || partnerIdx >= len(sibling.Names) {
			continue
		}
		sibName := sibling.Names[partnerIdx]
		if sibName == "" {
			continue
		}
		var sibSym cfg.SymbolID
		if partnerIdx < len(sibling.Symbols) {
			sibSym = sibling.Symbols[partnerIdx]
		}
		root := sibName
		if sibSym != 0 && bindings != nil {
			if name := bindings.Name(sibSym); name != "" {
				root = name
			}
		}
		path := constraint.Path{Root: root, Symbol: sibSym}
		// Same-direction relation: current non-nil -> partner non-nil, current nil -> partner nil.
		if wantNonNil {
			result = append(result, constraint.NotNil{Path: path})
		} else {
			result = append(result, constraint.IsNil{Path: path})
		}
	}
	return result
}

func guardedTypeSiblingConstraints(sibling *flow.SiblingAssignment, pos int, wantNonNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	var result []constraint.Constraint
	for _, cor := range sibling.GuardedCorrelations {
		if cor.TargetType == nil || pos != cor.GuardIndex || wantNonNil != cor.GuardOnTruthy {
			continue
		}
		partnerIdx := cor.TargetIndex
		if partnerIdx < 0 || partnerIdx >= len(sibling.Names) {
			continue
		}
		sibName := sibling.Names[partnerIdx]
		if sibName == "" {
			continue
		}
		var sibSym cfg.SymbolID
		if partnerIdx < len(sibling.Symbols) {
			sibSym = sibling.Symbols[partnerIdx]
		}
		root := sibName
		if sibSym != 0 && bindings != nil {
			if name := bindings.Name(sibSym); name != "" {
				root = name
			}
		}
		path := constraint.Path{Root: root, Symbol: sibSym}
		result = append(result, constraint.HasType{
			Path: path,
			Type: narrow.HashTypeKey(cor.TargetType.Hash()),
		})
	}
	return result
}
