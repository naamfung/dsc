package system

import (
	"context"
	_ "embed"
	"os"

	"github.com/jig/lisp/lib/call"
	"github.com/jig/lisp/types"
	. "github.com/jig/lisp/types"
)

func Load(env types.EnvType) {
	call.Call(env, getenv)
	call.Call(env, setenv)
	call.Call(env, unsetenv)
}

func getenv(ctx context.Context, k string) (MalType, error) {
	if v, ok := os.LookupEnv(k); ok {
		return v, nil
	}
	return nil, nil
}

func setenv(ctx context.Context, k, v string) error {
	return os.Setenv(k, v)
}

func unsetenv(ctx context.Context, k string) error {
	return os.Unsetenv(k)
}
