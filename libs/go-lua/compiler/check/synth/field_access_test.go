package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldAccessResult_Zero(t *testing.T) {
	var r FieldAccessResult
	if r.Found || r.SkipCheck || r.NotIndexable || r.Type != nil {
		t.Fatal("zero FieldAccessResult should have all false/nil")
	}
}

func TestFieldAccessResult_WithType(t *testing.T) {
	r := FieldAccessResult{Type: typ.String, Found: true}
	if !r.Found {
		t.Fatal("expected Found=true")
	}
	if r.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolveFieldAccess_NilObjType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, nil, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for nil obj type")
	}
}

func TestResolveFieldAccess_AnyType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.Any, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for any type")
	}
}

func TestResolveFieldAccess_UnknownType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.Unknown, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for unknown type")
	}
}

func TestResolveFieldAccess_RecordField(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := e.ResolveFieldAccess(nil, rec, "name", cfg.Point(0))
	if !result.Found {
		t.Fatal("expected field to be found")
	}
	if result.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolveFieldAccess_RecordFieldNotFound(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := e.ResolveFieldAccess(nil, rec, "age", cfg.Point(0))
	if result.Found {
		t.Fatal("expected field not found")
	}
}

func TestResolveFieldAccess_EmptyFieldNameMap(t *testing.T) {
	e := newTestEngine()
	m := typ.NewMap(typ.String, typ.Integer)
	result := e.ResolveFieldAccess(nil, m, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for map with empty field")
	}
}

func TestResolveFieldAccess_EmptyFieldNameArray(t *testing.T) {
	e := newTestEngine()
	arr := typ.NewArray(typ.String)
	result := e.ResolveFieldAccess(nil, arr, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for array with empty field")
	}
}

func TestResolveFieldAccess_MapWithFieldName(t *testing.T) {
	e := newTestEngine()
	m := typ.NewMap(typ.String, typ.Integer)
	result := e.ResolveFieldAccess(nil, m, "key", cfg.Point(0))
	if result.SkipCheck {
		t.Fatal("expected map field lookup to be checked")
	}
	if !result.Found {
		t.Fatal("expected field to be found on map")
	}
	opt, ok := result.Type.(*typ.Optional)
	if !ok || opt.Inner != typ.Integer {
		t.Fatal("expected optional integer type for map field")
	}
}

func TestResolveFieldAccess_ArrayWithFieldName(t *testing.T) {
	e := newTestEngine()
	arr := typ.NewArray(typ.String)
	result := e.ResolveFieldAccess(nil, arr, "index", cfg.Point(0))
	if result.SkipCheck {
		t.Fatal("expected array field lookup to be checked")
	}
	if result.Found {
		t.Fatal("expected field not found on array")
	}
}

func TestResolveFieldAccess_IdentKey(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	ex := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "name"},
	}
	result := e.ResolveFieldAccess(ex, rec, "name", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for ident key")
	}
}

func TestResolveFieldAccess_EmptyFieldNamePrimitive(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.String, "", cfg.Point(0))
	if !result.NotIndexable {
		t.Fatal("expected NotIndexable for primitive with empty field")
	}
}

func TestResolveFieldAccess_TypeParam(t *testing.T) {
	e := newTestEngine()
	tp := typ.NewTypeParam("T", nil)
	result := e.ResolveFieldAccess(nil, tp, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param")
	}
}

func TestResolveFieldAccess_EmptyFieldTypeParam(t *testing.T) {
	e := newTestEngine()
	tp := typ.NewTypeParam("T", nil)
	result := e.ResolveFieldAccess(nil, tp, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param with empty field")
	}
}
