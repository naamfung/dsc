package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRequireIntercept_NilManifests_ReturnsEmptyResult(t *testing.T) {
	r := &RequireIntercept{Manifests: nil}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "module"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for nil manifests")
	}
}

func TestRequireIntercept_NotRequireCall_ReturnsEmptyResult(t *testing.T) {
	querier := &requireTestManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "print"},
		Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-require call")
	}
}

func TestRequireIntercept_WrongArgCount_Zero(t *testing.T) {
	querier := &requireTestManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for zero args")
	}
}

func TestRequireIntercept_WrongArgCount_Multiple(t *testing.T) {
	querier := &requireTestManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{
			&ast.StringExpr{Value: "module1"},
			&ast.StringExpr{Value: "module2"},
		},
	}
	result := r.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for multiple args")
	}
}

func TestRequireIntercept_NonStringArg_SkipsFalse(t *testing.T) {
	querier := &requireTestManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "require" {
				return requireFn
			}
			return nil
		},
	}
	result := r.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for non-string arg")
	}
}

func TestRequireIntercept_ModuleFound_ReturnsExportType(t *testing.T) {
	exportType := typ.NewRecord().Field("value", typ.Integer).Build()
	manifest := &io.Manifest{Export: exportType}
	querier := &requireTestManifestQuerier{
		manifests: map[string]*io.Manifest{"mymodule": manifest},
	}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymodule"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "require" {
				return requireFn
			}
			return nil
		},
	}
	result := r.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for found module")
	}
	if len(result.Types) != 1 {
		t.Fatal("expected one type")
	}
	if result.Types[0] != exportType {
		t.Fatal("expected export type")
	}
}

func TestRequireIntercept_ModuleNotFound_SkipsFalse(t *testing.T) {
	querier := &requireTestManifestQuerier{manifests: map[string]*io.Manifest{}}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "unknown"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "require" {
				return requireFn
			}
			return nil
		},
	}
	result := r.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for unknown module")
	}
}

func TestRequireIntercept_ModuleFromImports_ReturnsExportType(t *testing.T) {
	exportType := typ.NewRecord().Field("imported", typ.Boolean).Build()
	manifest := &io.Manifest{Export: exportType}
	querier := &requireTestManifestQuerier{
		manifests: map[string]*io.Manifest{},
		imports:   map[string]*io.Manifest{"alias": manifest},
	}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "alias"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "require" {
				return requireFn
			}
			return nil
		},
	}
	result := r.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for import alias")
	}
}

func TestRequireIntercept_NilEnrichedExport(t *testing.T) {
	manifest := &io.Manifest{Export: nil}
	querier := &requireTestManifestQuerier{
		manifests: map[string]*io.Manifest{"mymodule": manifest},
	}
	r := &RequireIntercept{Manifests: querier}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymodule"}},
	}
	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "require" {
				return requireFn
			}
			return nil
		},
	}
	result := r.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false when enriched export is nil")
	}
}

func TestIsRequireCall_NilExpr_ReturnsFalse(t *testing.T) {
	if isRequireCall(nil, CallEnv{}) {
		t.Fatal("expected false for nil expr")
	}
}

func TestIsRequireCall_NilFunc_ReturnsFalse(t *testing.T) {
	ex := &ast.FuncCallExpr{Func: nil}
	if isRequireCall(ex, CallEnv{}) {
		t.Fatal("expected false for nil func")
	}
}

type requireTestManifestQuerier struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (q *requireTestManifestQuerier) Manifest(path string) *io.Manifest {
	if q.manifests == nil {
		return nil
	}
	return q.manifests[path]
}

func (q *requireTestManifestQuerier) Imports() map[string]*io.Manifest {
	return q.imports
}
