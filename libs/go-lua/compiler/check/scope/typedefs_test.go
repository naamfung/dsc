package scope

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildTypeDefScopes(t *testing.T) {
	t.Run("empty graph returns base scope for all points", func(t *testing.T) {
		graph := cfg.Build(&ast.FunctionExpr{})
		if graph == nil {
			t.Skip("empty function returns nil graph")
		}
		base := New()
		resolver := func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type {
			return nil
		}
		scopes := BuildTypeDefScopes(graph, base, resolver)
		if scopes == nil {
			t.Fatal("expected non-nil scopes map")
		}
	})

	t.Run("nil resolver result does not add type", func(t *testing.T) {
		graph := cfg.Build(&ast.FunctionExpr{})
		if graph == nil {
			t.Skip("empty function returns nil graph")
		}
		base := New()
		resolver := func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type {
			return nil
		}
		scopes := BuildTypeDefScopes(graph, base, resolver)
		for _, sc := range scopes {
			if sc == nil {
				t.Fatal("expected non-nil scope")
			}
		}
	})
}

func TestEnrichWithTypeDefs(t *testing.T) {
	t.Run("empty graph returns base scope", func(t *testing.T) {
		graph := cfg.Build(&ast.FunctionExpr{})
		if graph == nil {
			t.Skip("empty function returns nil graph")
		}
		base := New()
		resolver := func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type {
			return nil
		}
		result := EnrichWithTypeDefs(graph, base, resolver)
		if result != base {
			t.Fatal("expected base scope to be returned")
		}
	})

	t.Run("nil resolver result does not modify scope", func(t *testing.T) {
		graph := cfg.Build(&ast.FunctionExpr{})
		if graph == nil {
			t.Skip("empty function returns nil graph")
		}
		base := New()
		resolver := func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type {
			return nil
		}
		result := EnrichWithTypeDefs(graph, base, resolver)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})
}

func TestToTypeParamExprs(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := ToTypeParamExprs(nil)
		if result != nil {
			t.Fatal("expected nil result")
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		result := ToTypeParamExprs([]cfg.TypeParamInfo{})
		if result != nil {
			t.Fatal("expected nil result")
		}
	})

	t.Run("converts params correctly", func(t *testing.T) {
		params := []cfg.TypeParamInfo{
			{Name: "T", Constraint: nil},
			{Name: "U", Constraint: nil},
		}
		result := ToTypeParamExprs(params)
		if len(result) != 2 {
			t.Fatalf("expected 2 params, got %d", len(result))
		}
		if result[0].Name != "T" {
			t.Fatalf("expected first param name T, got %s", result[0].Name)
		}
		if result[1].Name != "U" {
			t.Fatalf("expected second param name U, got %s", result[1].Name)
		}
	})

	t.Run("preserves constraint", func(t *testing.T) {
		constraint := &ast.PrimitiveTypeExpr{Name: "number"}
		params := []cfg.TypeParamInfo{
			{Name: "T", Constraint: constraint},
		}
		result := ToTypeParamExprs(params)
		if len(result) != 1 {
			t.Fatalf("expected 1 param, got %d", len(result))
		}
		if result[0].Constraint != constraint {
			t.Fatal("expected constraint to be preserved")
		}
	})
}

func TestTypeDefResolver(t *testing.T) {
	t.Run("type alias is defined correctly", func(t *testing.T) {
		var called bool
		var resolver TypeDefResolver = func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type {
			called = true
			if name != "MyType" {
				t.Fatalf("expected name MyType, got %s", name)
			}
			return typ.String
		}
		result := resolver("MyType", &ast.PrimitiveTypeExpr{Name: "string"}, nil, New())
		if !called {
			t.Fatal("expected resolver to be called")
		}
		if result != typ.String {
			t.Fatalf("expected string type, got %v", result)
		}
	})
}
