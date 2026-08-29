package nscore

import (
	"context"
	"reflect"
	"strings"

	"github.com/jig/lisp"
	"github.com/jig/lisp/lib/core"
	. "github.com/jig/lisp/types"
)

type Here struct{}

var (
	__package_fullpath__ = strings.Split(reflect.TypeFor[Here]().PkgPath(), "/")
	_package_            = "$" + __package_fullpath__[len(__package_fullpath__)-1]
)

func Load(env EnvType) error {
	core.Load(env)
	env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return lisp.EVAL(ctx, a[0], env)
	}})

	if _, err := lisp.REPL(context.Background(), env, core.HeaderBasic(), NewCursorFile(_package_)); err != nil {
		return err
	}
	return nil
}

func LoadInput(env EnvType) error {
	core.LoadInput(env)
	env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return lisp.EVAL(ctx, a[0], env)
	}})

	if _, err := lisp.REPL(context.Background(), env, core.HeaderLoadFile(), NewCursorFile(_package_)); err != nil {
		return err
	}
	return nil
}

func LoadCmdLineArgs(scriptArgs []string) func(EnvType) error {
	return func(env EnvType) error {
		return loadCmdLineArgs(env, scriptArgs)
	}
}

func loadCmdLineArgs(env EnvType, scriptArgs []string) error {
	if len(scriptArgs) > 0 {
		args := make([]MalType, 0, len(scriptArgs))
		for _, a := range scriptArgs {
			args = append(args, a)
		}
		env.Set(Symbol{Val: "*ARGV*"}, List{Val: args})
		return nil
	} else {
		return LoadNullArgs(env)
	}
}

func LoadNullArgs(env EnvType) error {
	env.Set(Symbol{Val: "*ARGV*"}, List{})
	return nil
}
