package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestMetatableTypedConstructor(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "typed table with metatable passes",
			Code: `
				type T = { x: number }
				local mt: metatable<T> = { __index = {} }
				local obj: T = setmetatable({ x = 1 }, mt)
				local y: number = obj.x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty table with metatable does not infer shape",
			Code: `
				type T = { x: number }
				local mt: metatable<T> = { __index = {} }
				local obj: T = setmetatable({}, mt)
			`,
			WantError: true,
			Stdlib:    true,
		},
	}

	testutil.RunCases(t, tests)
}
