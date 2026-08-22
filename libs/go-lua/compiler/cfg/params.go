package cfg

import "github.com/wippyai/go-lua/compiler/ast"

// ParamSlot is the canonical parameter layout entry for a function graph.
//
// SourceIndex maps this slot back to fn.ParList. A value of -1 means this is
// an implicit parameter introduced by binder/CFG (for example implicit method
// receiver `self` on `function T:m(...)`).
type ParamSlot struct {
	Name           string
	Symbol         SymbolID
	DeclPoint      Point
	TypeAnnotation ast.TypeExpr
	SourceIndex    int
	IsImplicitSelf bool
}

// HasSourceParam reports whether this slot maps to a source parameter in fn.ParList.
func (s ParamSlot) HasSourceParam() bool {
	return s.SourceIndex >= 0
}

// SourceParamIndex returns the source parameter index when present.
func (s ParamSlot) SourceParamIndex() (int, bool) {
	if !s.HasSourceParam() {
		return 0, false
	}
	return s.SourceIndex, true
}

// ParamSlots returns the canonical parameter layout for this graph.
//
// All downstream phases should consume this API rather than re-deriving
// ParList-to-symbol mapping, which can drift for implicit method receivers.
func (g *Graph) ParamSlots() []ParamSlot {
	if g == nil || len(g.paramSlots) == 0 {
		return nil
	}
	slots := make([]ParamSlot, len(g.paramSlots))
	copy(slots, g.paramSlots)
	return slots
}

// ParamSlotsReadOnly returns the canonical parameter layout for this graph.
// The returned slice must be treated as immutable by callers.
func (g *Graph) ParamSlotsReadOnly() []ParamSlot {
	if g == nil {
		return nil
	}
	return g.paramSlots
}

func buildParamSlots(
	fn *ast.FunctionExpr,
	paramNames []string,
	paramSymbols []SymbolID,
	paramDeclPoints []Point,
	symbolNames map[SymbolID]string,
) []ParamSlot {
	if len(paramSymbols) == 0 {
		return nil
	}
	slots := make([]ParamSlot, 0, len(paramSymbols))
	var parNames []string
	var parTypes []ast.TypeExpr
	if fn != nil && fn.ParList != nil {
		parNames = fn.ParList.Names
		parTypes = fn.ParList.Types
	}

	hasImplicitSelf := len(paramSymbols) == len(parNames)+1 &&
		len(paramNames) == len(paramSymbols) &&
		len(paramNames) > 0 &&
		paramNames[0] == "self" &&
		(len(parNames) == 0 || parNames[0] != "self")

	for i, sym := range paramSymbols {
		slot := ParamSlot{
			Symbol:      sym,
			SourceIndex: i,
		}

		if i < len(paramNames) {
			slot.Name = paramNames[i]
		}
		if slot.Name == "" && sym != 0 {
			slot.Name = symbolNames[sym]
		}
		if i < len(paramDeclPoints) {
			slot.DeclPoint = paramDeclPoints[i]
		}

		if hasImplicitSelf {
			if i == 0 {
				slot.SourceIndex = -1
				slot.IsImplicitSelf = true
			} else {
				slot.SourceIndex = i - 1
			}
		}

		if srcIdx, ok := slot.SourceParamIndex(); ok && srcIdx < len(parTypes) {
			slot.TypeAnnotation = parTypes[srcIdx]
		}

		slots = append(slots, slot)
	}

	return slots
}
