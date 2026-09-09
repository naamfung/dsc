package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestUnwrapAliasBasic(t *testing.T) {
	alias := typ.NewAlias("MyString", typ.String)
	if unwrap.Alias(alias) != typ.String {
		t.Error("UnwrapAlias should return underlying type")
	}
}

func TestFieldRecord(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Field("age", typ.Number).Build()

	ft, ok := Field(rec, "name")
	if !ok || ft != typ.String {
		t.Error("Field(name) should return string")
	}

	_, ok = Field(rec, "missing")
	if ok {
		t.Error("Field(missing) should return false")
	}
}

func TestIndexArray(t *testing.T) {
	arr := typ.NewArray(typ.String)

	et, ok := Index(arr, typ.Integer)
	if !ok {
		t.Error("Index on string[] with integer should return string")
	}

	if et != typ.String {
		t.Errorf("expected string, got %v", et)
	}
}

func TestIndexMap(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)

	et, ok := Index(m, typ.String)
	if !ok {
		t.Error("Index on {[string]: number} with string should return optional number")
	}

	if _, isOpt := et.(*typ.Optional); !isOpt {
		t.Errorf("expected optional number, got %v", et)
	}
}
