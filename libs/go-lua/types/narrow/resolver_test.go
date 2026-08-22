package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestResolver_Interface(t *testing.T) {
	var r Resolver = &testResolver{}
	ft, ok := r.Field(typ.String, "test")
	if ft != nil || ok {
		t.Error("expected nil, false from test resolver Field")
	}
	it, ok := r.Index(typ.String, typ.Integer)
	if it != nil || ok {
		t.Error("expected nil, false from test resolver Index")
	}
}

type testResolver struct{}

func (r *testResolver) Field(_ typ.Type, _ string) (typ.Type, bool) {
	return nil, false
}

func (r *testResolver) Index(_ typ.Type, _ typ.Type) (typ.Type, bool) {
	return nil, false
}
