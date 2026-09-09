package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TableCheckResult holds the outcome of a table literal check.
type TableCheckResult struct {
	Handled    bool
	Compatible bool
	Reason     string
}

// tableCheck validates a table literal against an expected type with union expansion.
func tableCheck(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) TableCheckResult {
	if table == nil || expected == nil || synth == nil {
		return TableCheckResult{}
	}
	expected = resolveLocalRefsFromScope(expected, synth, p)

	fields, arrayElems, _, earlyFail := extractTableFields(table, expected, synth, p)
	if earlyFail {
		return TableCheckResult{Handled: true, Compatible: false, Reason: "table shape is incompatible with expected record fields"}
	}

	if u := unwrap.Union(expected); u != nil {
		bestReason := ""
		for _, member := range u.Members {
			ok, reason := checkTableWithOptionalRelax(fields, arrayElems, member)
			if ok {
				return TableCheckResult{Handled: true, Compatible: true}
			}
			if bestReason == "" && reason != "" {
				bestReason = reason
			}
		}
		return TableCheckResult{Handled: true, Compatible: false, Reason: bestReason}
	}

	ok, reason := checkTableWithOptionalRelax(fields, arrayElems, expected)
	return TableCheckResult{Handled: true, Compatible: ok, Reason: reason}
}

// tableCompatible validates a table literal against an expected type without union expansion.
func tableCompatible(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) bool {
	if table == nil || expected == nil || synth == nil {
		return false
	}
	expected = resolveLocalRefsFromScope(expected, synth, p)

	fields, arrayElems, _, earlyFail := extractTableFields(table, expected, synth, p)
	if earlyFail {
		return false
	}

	ok, _ := checkTableWithOptionalRelax(fields, arrayElems, expected)
	return ok
}

func extractTableFields(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) ([]ops.FieldDef, []typ.Type, bool, bool) {
	recordOnly := false
	if u := unwrap.Union(expected); u != nil {
		recordOnly = unionAllRecordLike(u)
	} else if unwrap.Record(expected) != nil {
		recordOnly = true
	}

	expectedFields := core.AllFieldTypesResolved(expected)
	fields := make([]ops.FieldDef, 0, len(table.Fields))
	var arrayElems []typ.Type

	for _, field := range table.Fields {
		if field.Key == nil {
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElems))
			elemType := synth.SynthWithExpected(field.Value, p, elemExpected)
			if elemType == nil {
				elemType = typ.Unknown
			}
			arrayElems = append(arrayElems, elemType)
			continue
		}

		var name string
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			name = k.Value
		case *ast.IdentExpr:
			if recordOnly {
				return nil, nil, recordOnly, true
			}
			name = k.Value
		case *ast.NumberExpr:
			_ = k
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElems))
			elemType := synth.SynthWithExpected(field.Value, p, elemExpected)
			arrayElems = append(arrayElems, elemType)
			continue
		default:
			if recordOnly {
				return nil, nil, recordOnly, true
			}
			return nil, nil, recordOnly, true
		}

		expectedFieldType := resolveExpectedFieldType(expected, expectedFields, name)
		var ft typ.Type
		if isEmptyTableExpr(field.Value) {
			if promoted := promoteEmptyTableLiteral(expectedFieldType); promoted != nil {
				ft = promoted
			}
		}
		if ft == nil {
			if nested, ok := field.Value.(*ast.TableExpr); ok {
				if nestedType, ok := synthNestedTableWithExpected(nested, expectedFieldType, synth, p); ok {
					ft = nestedType
				}
			}
		}
		if ft == nil {
			ft = synth.SynthWithExpected(field.Value, p, expectedFieldType)
		}
		if ft == nil {
			ft = typ.Unknown
		}
		fields = append(fields, ops.FieldDef{Name: name, Type: ft})
	}

	return fields, arrayElems, recordOnly, false
}

func synthNestedTableWithExpected(table *ast.TableExpr, expected typ.Type, synth api.Synth, p cfg.Point) (typ.Type, bool) {
	if table == nil || expected == nil || synth == nil {
		return nil, false
	}
	expected = resolveLocalRefsFromScope(expected, synth, p)
	fields, arrayElems, _, earlyFail := extractTableFields(table, expected, synth, p)
	if earlyFail {
		return nil, false
	}
	result := ops.CheckTable(fields, arrayElems, expected)
	if len(result.Errors) == 0 {
		if result.Type != nil {
			return result.Type, true
		}
		return expected, true
	}
	ok, _ := checkTableWithOptionalRelax(fields, arrayElems, expected)
	if ok {
		if result.Type != nil {
			return result.Type, true
		}
		return expected, true
	}
	return nil, false
}

func resolveExpectedFieldType(expected typ.Type, expectedFields map[string]typ.Type, name string) typ.Type {
	if expectedFields != nil {
		if ft, ok := expectedFields[name]; ok && ft != nil {
			return ft
		}
	}

	if rec := unwrap.Record(expected); rec != nil {
		if f := rec.GetField(name); f != nil {
			return f.Type
		}
	}

	return nil
}

func isEmptyTableExpr(expr ast.Expr) bool {
	table, ok := expr.(*ast.TableExpr)
	return ok && table != nil && len(table.Fields) == 0
}

func promoteEmptyTableLiteral(expected typ.Type) typ.Type {
	if expected == nil {
		return nil
	}
	if a, ok := expected.(*typ.Alias); ok && a != nil {
		if promoted := promoteEmptyTableLiteral(a.Target); promoted != nil {
			return expected
		}
	}
	if opt, ok := expected.(*typ.Optional); ok && opt != nil {
		if promoteEmptyTableLiteral(opt.Inner) != nil {
			return expected
		}
	}

	switch unwrap.Alias(expected).Kind() {
	case kind.Map, kind.Array:
		return expected
	default:
		return nil
	}
}

func checkTableWithOptionalRelax(fields []ops.FieldDef, arrayElems []typ.Type, expected typ.Type) (bool, string) {
	result := ops.CheckTable(fields, arrayElems, expected)
	if len(result.Errors) == 0 {
		return true, ""
	}

	filtered := result.Errors[:0]
	for _, err := range result.Errors {
		if err.Message == "missing required field" && unwrap.IsOptionalLike(err.Expected) {
			continue
		}
		if err.Message == "unexpected field" {
			continue
		}
		filtered = append(filtered, err)
	}
	if len(filtered) == 0 {
		return true, ""
	}

	first := filtered[0]
	reason := first.Message
	if first.Field != "" {
		reason += " on field '" + first.Field + "'"
	}
	if first.Expected != nil {
		reason += ", expected " + typ.FormatShort(first.Expected)
	}
	if first.Got != nil {
		reason += ", got " + typ.FormatShort(first.Got)
	}
	return false, reason
}

func unionAllRecordLike(u *typ.Union) bool {
	if u == nil {
		return false
	}
	for _, m := range u.Members {
		if unwrap.Record(m) == nil {
			return false
		}
	}
	return true
}

type scopedSynth interface {
	Scopes() map[cfg.Point]*scope.State
}

func resolveLocalRefsFromScope(t typ.Type, synth api.Synth, p cfg.Point) typ.Type {
	if t == nil || synth == nil {
		return t
	}
	ss, ok := synth.(scopedSynth)
	if !ok {
		return t
	}
	sc := ss.Scopes()[p]
	if sc == nil {
		return t
	}

	visiting := make(map[string]bool)
	var resolve func(current typ.Type, depth int) typ.Type
	resolve = func(current typ.Type, depth int) typ.Type {
		if current == nil || typ.DepthExceeded(depth) {
			return current
		}
		return typ.Rewrite(current, func(node typ.Type) (typ.Type, bool) {
			ref, ok := node.(*typ.Ref)
			if !ok || ref.Module != "" {
				return nil, false
			}
			target, exists := sc.LookupType(ref.Name)
			if !exists || target == nil || visiting[ref.Name] {
				return nil, false
			}
			visiting[ref.Name] = true
			resolved := resolve(target, depth+1)
			delete(visiting, ref.Name)
			if resolved == nil {
				return nil, false
			}
			return resolved, true
		})
	}

	return resolve(t, 0)
}
