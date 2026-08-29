package nsconcurrent

import (
	"context"
	"reflect"
	"strings"

	"github.com/jig/lisp"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/types"
)

type Here struct{}

var (
	__package_fullpath__ = strings.Split(reflect.TypeFor[Here]().PkgPath(), "/")
	_package_            = "$" + __package_fullpath__[len(__package_fullpath__)-1]
)

func Load(env types.EnvType) error {
	concurrent.Load(env)

	if _, err := lisp.REPL(context.Background(), env, concurrent.HeaderConcurrent(), types.NewCursorFile(_package_)); err != nil {
		return err
	}

	return nil
}
