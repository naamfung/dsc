package nestedinfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
)

func TestProcessNestedFunctions_NilResult(t *testing.T) {
	p := New(Config{})
	p.ProcessNestedFunctions(nil, nil)
}

func TestProcessNestedFunctions_NilScopes(t *testing.T) {
	p := New(Config{})
	p.ProcessNestedFunctions(nil, &api.FuncResultView{})
}
