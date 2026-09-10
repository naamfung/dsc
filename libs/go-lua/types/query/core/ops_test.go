package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeOps_Interface(t *testing.T) {
	var _ TypeOps = (*Engine)(nil)
}

func TestTypeOps_Engine(t *testing.T) {
	e := NewEngine()
	var ops TypeOps = e

	rec := typ.NewRecord().Field("x", typ.Number).Build()
	_, ok := ops.Field(nil, rec, "x")
	if !ok {
		t.Error("Field through interface should work")
	}
}
