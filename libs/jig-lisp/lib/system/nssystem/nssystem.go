package nssystem

import (
	"github.com/jig/lisp/lib/system"
	"github.com/jig/lisp/types"
)

// type Here struct{}

// var (
// 	__package_fullpath__ = strings.Split(reflect.TypeFor[Here]().PkgPath(), "/")
// 	_package_            = "$" + __package_fullpath__[len(__package_fullpath__)-1]
// )

func Load(env types.EnvType) error {
	system.Load(env)
	return nil
}
