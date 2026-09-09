package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

type structuredWrite struct {
	point     cfg.Point
	versionID int
	segments  []constraint.Segment
	source    ast.Expr
}

// indexStructuredWrites collects static field/index writes keyed by base symbol.
func indexStructuredWrites(graph *cfg.Graph) map[cfg.SymbolID][]structuredWrite {
	result := make(map[cfg.SymbolID][]structuredWrite)
	if graph == nil {
		return result
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			write, sym, ok := structuredWriteForTarget(graph, p, info.SourceAt(i), target)
			if !ok {
				continue
			}
			result[sym] = append(result[sym], write)
		}
	})

	return result
}

// enrichStructuredOverlayAtPoint applies dominating visible field writes for the
// current symbol version into a point-specific identifier overlay.
func enrichStructuredOverlayAtPoint(
	graph *cfg.Graph,
	idom map[cfg.Point]cfg.Point,
	writes map[cfg.SymbolID][]structuredWrite,
	p cfg.Point,
	overlay api.SpecTypes,
	resolveSym func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	synth func(ast.Expr, cfg.Point) typ.Type,
) api.SpecTypes {
	if graph == nil || len(writes) == 0 {
		return overlay
	}

	out := overlay
	copied := false
	for sym, symWrites := range writes {
		if sym == 0 || len(symWrites) == 0 {
			continue
		}

		baseType, ok := out[sym]
		if !ok && resolveSym != nil {
			baseType, ok = resolveSym(p, sym)
		}

		merged := mergeVisibleStructuredWrites(graph, idom, symWrites, sym, p, baseType, synth)
		if merged == nil || (ok && typ.TypeEquals(merged, baseType)) {
			continue
		}

		if !copied {
			if len(overlay) == 0 {
				out = make(api.SpecTypes, 1)
			} else {
				out = make(api.SpecTypes, len(overlay)+1)
				for k, v := range overlay {
					out[k] = v
				}
			}
			copied = true
		}
		out[sym] = merged
	}

	return out
}

func structuredWriteForTarget(graph *cfg.Graph, p cfg.Point, source ast.Expr, target cfg.AssignTarget) (structuredWrite, cfg.SymbolID, bool) {
	if graph == nil || target.BaseSymbol == 0 {
		return structuredWrite{}, 0, false
	}

	var segments []constraint.Segment
	switch target.Kind {
	case cfg.TargetField:
		if len(target.FieldPath) == 0 {
			return structuredWrite{}, 0, false
		}
		segments = make([]constraint.Segment, len(target.FieldPath))
		for i, field := range target.FieldPath {
			if field == "" {
				return structuredWrite{}, 0, false
			}
			segments[i] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
		}
	case cfg.TargetIndex:
		switch key := target.Key.(type) {
		case *ast.StringExpr:
			if key.Value == "" {
				return structuredWrite{}, 0, false
			}
			segments = []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: key.Value}}
		case *ast.NumberExpr:
			segments = []constraint.Segment{{Kind: constraint.SegmentIndexInt}}
		default:
			return structuredWrite{}, 0, false
		}
	default:
		return structuredWrite{}, 0, false
	}

	version := graph.VisibleVersion(p, target.BaseSymbol)
	if version.ID == 0 {
		return structuredWrite{}, 0, false
	}

	return structuredWrite{
		point:     p,
		versionID: version.ID,
		segments:  segments,
		source:    source,
	}, target.BaseSymbol, true
}

func mergeVisibleStructuredWrites(
	graph *cfg.Graph,
	idom map[cfg.Point]cfg.Point,
	writes []structuredWrite,
	sym cfg.SymbolID,
	at cfg.Point,
	baseType typ.Type,
	synth func(ast.Expr, cfg.Point) typ.Type,
) typ.Type {
	if graph == nil || sym == 0 || len(writes) == 0 {
		return baseType
	}

	currentVersion := graph.VisibleVersion(at, sym)
	if currentVersion.ID == 0 {
		return baseType
	}

	current := baseType
	for _, write := range writes {
		if write.versionID != currentVersion.ID {
			continue
		}
		if write.point == at || !cfganalysis.StrictlyDominates(idom, write.point, at) {
			continue
		}

		valueType := typ.Unknown
		if write.source != nil && synth != nil {
			if resolved := synth(write.source, write.point); resolved != nil {
				valueType = resolved
			}
		}
		current = applyStructuredWrite(current, write.segments, valueType)
	}

	return current
}

func applyStructuredWrite(baseType typ.Type, segments []constraint.Segment, valueType typ.Type) typ.Type {
	if len(segments) == 0 {
		if valueType == nil {
			return baseType
		}
		return valueType
	}

	seg := segments[0]
	child := childTypeForStructuredSegment(baseType, seg)
	updatedChild := applyStructuredWrite(child, segments[1:], valueType)

	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return overwriteStructuredField(baseType, seg.Name, updatedChild)
	case constraint.SegmentIndexInt:
		return overwriteStructuredIntIndex(baseType, updatedChild)
	default:
		return baseType
	}
}

func childTypeForStructuredSegment(baseType typ.Type, seg constraint.Segment) typ.Type {
	if baseType == nil {
		return nil
	}

	switch t := baseType.(type) {
	case *typ.Alias:
		return childTypeForStructuredSegment(t.Target, seg)
	case *typ.Record:
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if field := t.GetField(seg.Name); field != nil {
				return field.Type
			}
			if t.HasMapComponent() && (typ.IsAny(t.MapKey) || t.MapKey.Kind() == kind.String) {
				return t.MapValue
			}
		case constraint.SegmentIndexInt:
			if t.HasMapComponent() && (typ.IsAny(t.MapKey) || t.MapKey.Kind() == kind.Integer || t.MapKey.Kind() == kind.Number) {
				return t.MapValue
			}
		}
	case *typ.Map:
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if typ.IsAny(t.Key) || t.Key.Kind() == kind.String {
				return t.Value
			}
		case constraint.SegmentIndexInt:
			if typ.IsAny(t.Key) || t.Key.Kind() == kind.Integer || t.Key.Kind() == kind.Number {
				return t.Value
			}
		}
	case *typ.Array:
		if seg.Kind == constraint.SegmentIndexInt {
			return t.Element
		}
	}

	return nil
}

func overwriteStructuredField(baseType typ.Type, field string, fieldType typ.Type) typ.Type {
	if field == "" || fieldType == nil {
		return baseType
	}

	switch t := baseType.(type) {
	case *typ.Alias:
		updated := overwriteStructuredField(t.Target, field, fieldType)
		if updated == nil || typ.TypeEquals(updated, t.Target) {
			return baseType
		}
		return typ.NewAlias(t.Name, updated)
	case *typ.Map:
		return typ.NewRecord().
			SetOpen(true).
			MapComponent(t.Key, t.Value).
			Field(field, fieldType).
			Build()
	default:
		return typ.ExtendRecordWithField(baseType, field, fieldType)
	}
}

func overwriteStructuredIntIndex(baseType typ.Type, elemType typ.Type) typ.Type {
	if elemType == nil {
		return baseType
	}

	switch t := baseType.(type) {
	case *typ.Alias:
		updated := overwriteStructuredIntIndex(t.Target, elemType)
		if updated == nil || typ.TypeEquals(updated, t.Target) {
			return baseType
		}
		return typ.NewAlias(t.Name, updated)
	case *typ.Array:
		return typ.NewArray(elemType)
	case *typ.Map:
		return typ.NewMap(t.Key, elemType)
	case *typ.Record:
		builder := typ.NewRecord()
		if t.Open {
			builder.SetOpen(true)
		}
		for _, f := range t.Fields {
			if f.Optional {
				if f.Readonly {
					builder.OptReadonlyField(f.Name, f.Type)
				} else {
					builder.OptField(f.Name, f.Type)
				}
			} else if f.Readonly {
				builder.ReadonlyField(f.Name, f.Type)
			} else {
				builder.Field(f.Name, f.Type)
			}
		}
		if t.Metatable != nil {
			builder.Metatable(t.Metatable)
		}
		builder.MapComponent(typ.Integer, elemType)
		return builder.Build()
	default:
		return typ.NewMap(typ.Integer, elemType)
	}
}
