package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SynthTableCore synthesizes a type for a table constructor expression.
//
// Table constructors are complex: they can represent:
//   - Records: {name = "Alice", age = 30}
//   - Arrays: {1, 2, 3}
//   - Tuples: {"a", 1, true} (mixed types, no expected array context)
//   - Maps: (via type annotations on expected type)
//
// Delegates to SynthTableWithExpected without contextual typing.
func (s *Synthesizer) SynthTableCore(ex *ast.TableExpr, sc *scope.State, recurse ExprSynth) typ.Type {
	return s.SynthTableWithExpected(ex, sc, recurse, nil)
}

// SynthTableWithExpected synthesizes a table type with bidirectional type checking.
//
// When expected is provided:
//   - Union expected: Attempts discriminated union matching via literal fields
//   - Record expected: Uses expected field types for nested synthesis
//   - Array expected: Synthesizes as array (not tuple) even without vararg
//   - Function fields: Passes expected function types for callback typing
//
// Self-type inference: For tables with function fields that have a "self" first
// parameter, infers self type from either expected type or synthesized table type.
//
// Empty tables return an open record (can have any additional fields assigned).
func (s *Synthesizer) SynthTableWithExpected(ex *ast.TableExpr, sc *scope.State, recurse ExprSynth, expected typ.Type) typ.Type {
	if len(ex.Fields) == 0 {
		return typ.NewRecord().SetOpen(true).Build()
	}

	if _, isUnion := unwrap.Alias(expected).(*typ.Union); isUnion {
		if match := querycore.TryDiscriminatedUnionMember(ex, expected); match != nil {
			return s.SynthTableWithExpected(ex, sc, recurse, match.Member)
		}
	}

	expectedFields := s.resolveExpectedFields(expected)
	selfType := expected
	if selfType == nil {
		selfBuilder := typ.NewRecord()
		fieldCount := 0
		for _, field := range ex.Fields {
			if field.Key == nil {
				continue
			}
			if _, ok := field.Value.(*ast.FunctionExpr); ok {
				continue
			}
			switch k := field.Key.(type) {
			case *ast.StringExpr:
				ft := recurse(field.Value)
				if ft == nil {
					ft = typ.Unknown
				}
				if inner, optional := typ.SplitNilableFieldType(ft); optional {
					selfBuilder.OptField(k.Value, inner)
				} else {
					selfBuilder.Field(k.Value, ft)
				}
				fieldCount++
			case *ast.IdentExpr:
				ft := recurse(field.Value)
				if ft == nil {
					ft = typ.Unknown
				}
				if inner, optional := typ.SplitNilableFieldType(ft); optional {
					selfBuilder.OptField(k.Value, inner)
				} else {
					selfBuilder.Field(k.Value, ft)
				}
				fieldCount++
			}
		}
		if fieldCount > 0 {
			selfType = selfBuilder.Build()
		}
	}

	builder := typ.NewRecord()
	var fieldDefs []ops.FieldDef
	var arrayElements []typ.Type
	hasVararg := false
	fieldCount := 0

	for _, field := range ex.Fields {
		if field.Key == nil {
			if _, ok := field.Value.(*ast.Comma3Expr); ok {
				hasVararg = true
			}
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElements))
			elemType := s.synthFieldValueWithExpected(field.Value, sc, recurse, elemExpected, selfType)
			if elemType == nil {
				elemType = typ.Unknown
			}
			arrayElements = append(arrayElements, elemType)
			continue
		}

		switch k := field.Key.(type) {
		case *ast.StringExpr:
			ft := s.synthFieldValueWithExpected(field.Value, sc, recurse, expectedFields[k.Value], selfType)
			if ft == nil {
				ft = typ.Unknown
			}
			fieldDefs = append(fieldDefs, ops.FieldDef{Name: k.Value, Type: ft})
			if inner, optional := typ.SplitNilableFieldType(ft); optional {
				builder.OptField(k.Value, inner)
			} else {
				builder.Field(k.Value, ft)
			}
			fieldCount++
		case *ast.IdentExpr:
			ft := s.synthFieldValueWithExpected(field.Value, sc, recurse, expectedFields[k.Value], selfType)
			if ft == nil {
				ft = typ.Unknown
			}
			fieldDefs = append(fieldDefs, ops.FieldDef{Name: k.Value, Type: ft})
			if inner, optional := typ.SplitNilableFieldType(ft); optional {
				builder.OptField(k.Value, inner)
			} else {
				builder.Field(k.Value, ft)
			}
			fieldCount++
		case *ast.NumberExpr:
			elemExpected := ops.ExpectedTableElementType(expected, len(arrayElements))
			elemType := s.synthFieldValueWithExpected(field.Value, sc, recurse, elemExpected, selfType)
			if elemType == nil {
				elemType = typ.Unknown
			}
			arrayElements = append(arrayElements, elemType)
		}
	}

	if len(arrayElements) > 0 && fieldCount == 0 {
		var result typ.Type
		if hasVararg {
			result = typ.NewArray(typ.NewUnion(arrayElements...))
		} else if querycore.IsArrayLike(expected) {
			result = typ.NewArray(typ.NewUnion(arrayElements...))
		} else {
			result = typ.NewTuple(arrayElements...)
		}
		if expected != nil && len(ops.CheckTable(nil, arrayElements, expected).Errors) == 0 {
			return expected
		}
		return result
	}

	result := builder.Build()
	if expected != nil && len(ops.CheckTable(fieldDefs, arrayElements, expected).Errors) == 0 {
		return expected
	}
	return result
}

// synthFieldValueWithExpected synthesizes type for a table field value with optional expected type.
func (s *Synthesizer) synthFieldValueWithExpected(value ast.Expr, sc *scope.State, recurse ExprSynth, expected typ.Type, selfType typ.Type) typ.Type {
	if tbl, ok := value.(*ast.TableExpr); ok {
		return s.SynthTableWithExpected(tbl, sc, recurse, expected)
	}
	if fn, ok := value.(*ast.FunctionExpr); ok {
		var expectedFn *typ.Function
		if expected != nil {
			expectedFn, _ = unwrap.Alias(expected).(*typ.Function)
		}
		bindings := s.deps.ModuleBindings
		if bindings == nil && s.deps.CheckCtx != nil {
			bindings = s.deps.CheckCtx.Bindings()
		}
		if expectedFn == nil && selfType != nil && phasecore.HasUnannotatedSelfParam(fn, bindings) {
			expectedFn = typ.Func().Param("self", selfType).Build()
		}
		return s.SynthFunctionTypeWithExpected(fn, sc, expectedFn)
	}
	return recurse(value)
}

// resolveExpectedFields extracts expected field types from the expected type.
func (s *Synthesizer) resolveExpectedFields(expected typ.Type) map[string]typ.Type {
	if expected == nil {
		return nil
	}

	if _, isUnion := unwrap.Alias(expected).(*typ.Union); !isUnion {
		return querycore.AllFieldTypesResolved(expected)
	}

	union := unwrap.Alias(expected).(*typ.Union)
	result := make(map[string]typ.Type)

	fieldNames := make(map[string]struct{})
	for _, member := range union.Members {
		memberFields := querycore.AllFieldTypesResolved(member)
		for name := range memberFields {
			fieldNames[name] = struct{}{}
		}
	}

	for name := range fieldNames {
		var fieldTypes []typ.Type
		for _, member := range union.Members {
			memberFields := querycore.AllFieldTypesResolved(member)
			if ft, ok := memberFields[name]; ok {
				fieldTypes = append(fieldTypes, ft)
			}
		}
		if len(fieldTypes) > 0 {
			result[name] = typ.NewUnion(fieldTypes...)
		}
	}

	return result
}
