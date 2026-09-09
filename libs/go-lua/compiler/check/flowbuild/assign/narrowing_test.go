package assign_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNarrowReturnTypeBySpec(t *testing.T) {
	// Create narrowed type
	narrowedType := typ.String

	// Create spec like the test does
	spec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConstraints(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			}),
			narrowedType,
		).
		WithDefaultReturn(typ.Any)

	// Create function type with spec
	fn := typ.Func().
		Param("topic", typ.String).
		OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
		Returns(typ.Any).
		Spec(spec).
		Build()

	// Create CallInfo simulating process.listen("increment", {message = true})
	callInfo := &cfg.CallInfo{
		Callee:     &ast.IdentExpr{Value: "listen"},
		CalleeName: "listen",
		Args: []ast.Expr{
			&ast.StringExpr{Value: "increment"},
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.IdentExpr{Value: "message"},
						Value: &ast.TrueExpr{},
					},
				},
			},
		},
	}

	// Create scope (synth function returns fn directly without scope lookup)
	sc := scope.New()

	// Create synth function that returns fn for the callee expression
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && ident.Value == "listen" {
			return fn
		}
		return nil
	}

	// Test narrowing
	result := assign.NarrowReturnTypeBySpec(callInfo, sc, synth, 0, nil, nil, nil, nil)

	if result == nil {
		t.Fatalf("narrowReturnTypeBySpec returned nil, expected narrowed type")
	}

	if result != narrowedType {
		t.Errorf("expected %v, got %v", narrowedType, result)
	}
}

func TestNarrowReturnTypeBySpec_NoMatch(t *testing.T) {
	// Create spec
	spec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConstraints(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			}),
			typ.String,
		).
		WithDefaultReturn(typ.Any)

	// Create function type with spec
	fn := typ.Func().
		Param("topic", typ.String).
		OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
		Returns(typ.Any).
		Spec(spec).
		Build()

	// Create CallInfo with message=false (should NOT match)
	callInfo := &cfg.CallInfo{
		Callee:     &ast.IdentExpr{Value: "listen"},
		CalleeName: "listen",
		Args: []ast.Expr{
			&ast.StringExpr{Value: "increment"},
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:   &ast.IdentExpr{Value: "message"},
						Value: &ast.FalseExpr{},
					},
				},
			},
		},
	}

	// synth function returns fn directly without scope lookup
	sc := scope.New()

	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && ident.Value == "listen" {
			return fn
		}
		return nil
	}

	result := assign.NarrowReturnTypeBySpec(callInfo, sc, synth, 0, nil, nil, nil, nil)

	if result != nil {
		t.Errorf("expected nil (no match), got %v", result)
	}
}
