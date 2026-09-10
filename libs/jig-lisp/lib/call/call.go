package call

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/types"
)

func Call(namespace types.EnvType, fIn types.MalType, args ...int) {
	call(nil, namespace, fIn, args...)
}

func CallOverrideFN(namespace types.EnvType, overrideFN string, fIn types.MalType, args ...int) {
	call(&overrideFN, namespace, fIn, args...)
}

func call(overrideFN *string, namespace types.EnvType, fIn types.MalType, args ...int) {
	functionFullName := strings.ToLower(runtime.FuncForPC(reflect.ValueOf(fIn).Pointer()).Name())
	n := strings.LastIndex(functionFullName, ".")
	if len(functionFullName) == -1 {
		panic(fmt.Errorf("invalid function full name (name is %s)", runtime.FuncForPC(reflect.ValueOf(fIn).Pointer()).Name()))
	}
	packageName := functionFullName[:n]
	var functionName string
	if overrideFN != nil {
		functionName = *overrideFN
		m := strings.LastIndex(packageName, ".")
		functionFullName = fmt.Sprintf("%s[%s]", packageName[:m], *overrideFN)
	} else {
		functionName = strings.Replace(functionFullName[n+1:], "_", "-", -1)
		functionFullName = fmt.Sprintf("%s[%s]", packageName, functionName)
	}

	finType := reflect.TypeOf(fIn)
	finValue := reflect.ValueOf(fIn)
	outParams := finType.NumOut()

	contextRequired := false
	if finType.NumIn() >= 1 && finType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		contextRequired = true
	}

	var minArgs, maxArgs int
	switch len(args) {
	case 1:
		if !finType.IsVariadic() {
			panic(fmt.Errorf("%s: argument maximum argument count defined but implementation is not variadic", functionFullName))
		}
		minArgs, maxArgs = args[0], unlimitedArgments // if only one argument: it is the minimum number of arguments
	case 2:
		if !finType.IsVariadic() {
			panic(fmt.Errorf("%s: argument maximum and minimum argument count defined but implementation is not variadic", functionFullName))
		}
		minArgs, maxArgs = args[0], args[1]
	default:
		if !finType.IsVariadic() {
			minArgs, maxArgs = finType.NumIn(), finType.NumIn()
		} else {
			minArgs, maxArgs = 0, unlimitedArgments
		}
	}
	if minArgs > maxArgs {
		panic(fmt.Errorf("%s: maximum arguments (%d) is lower than minimum arguments (%d)", functionFullName, maxArgs, minArgs))
	}
	if minArgs < 0 || maxArgs < 0 {
		panic(fmt.Errorf("%s: argument count bounds cannot be negative", functionFullName))
	}

	var extCall func(context.Context, []types.MalType) (types.MalType, error)
	switch finType.NumOut() {
	case 0:
		if contextRequired {
			extCall = func(ctx context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _nil_nil(finValue.Call(_args_ctx(ctx, minArgs, maxArgs, args)))
			}
		} else {
			extCall = func(_ context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _nil_nil(finValue.Call(_args(minArgs, maxArgs, args)))
			}
		}
	case 1:
		if contextRequired {
			extCall = func(ctx context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _nil_error(finValue.Call(_args_ctx(ctx, minArgs, maxArgs, args)))
			}
		} else {
			extCall = func(_ context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _nil_error(finValue.Call(_args(minArgs, maxArgs, args)))
			}
		}
	case 2:
		if contextRequired {
			extCall = func(ctx context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _result_error(finValue.Call(_args_ctx(ctx, minArgs, maxArgs, args)))
			}
		} else {
			extCall = func(_ context.Context, args []types.MalType) (result types.MalType, err error) {
				defer _recover(functionFullName, &err)
				return _result_error(finValue.Call(_args(minArgs, maxArgs, args)))
			}
		}
	default:
		panic(fmt.Errorf("%s: wrong number of results (%d instead of 2)", functionFullName, outParams))
	}

	namespace.Set(types.Symbol{Val: functionName}, types.Func{Fn: extCall})

	_, err := namespace.Update(types.Symbol{Val: "_PACKAGES_"}, func(_hm types.MalType) (types.MalType, error) {
		if _hm == nil {
			_hm = types.HashMap{Val: make(map[string]types.MalType)}
		}
		hm := _hm.(types.HashMap)
		set, ok := hm.Val[packageName].(types.Set)
		if !ok {
			set = types.Set{Val: make(map[string]struct{})}
		}
		set.Val[functionName] = struct{}{}
		hm.Val[packageName] = set
		return hm, nil
	})
	if err != nil {
		panic(fmt.Errorf("%s: error loading implementation", packageName))
	}
}

func _recover(fFullName string, err *error) {
	rerr := recover()
	if rerr != nil {
		switch rerr := rerr.(type) {
		case error:
			*err = lisperror.NewGoError(fFullName, rerr)
		default:
			*err = lisperror.NewLispError(rerr, nil)
		}

	}
}

const unlimitedArgments = 1000

func _args_ctx(ctx context.Context, minParams, maxParams int, args []types.MalType) []reflect.Value {
	if len(args) < minParams-1 || len(args) > maxParams-1 {
		if maxParams == unlimitedArgments {
			panic(fmt.Errorf("wrong number of arguments (%d instead of a minimum of %d)", len(args), minParams-1))
		} else {
			if minParams == maxParams {
				panic(fmt.Errorf("wrong number of arguments (%d instead of %d)", len(args), minParams-1))
			} else {
				panic(fmt.Errorf("wrong number of arguments (%d instead of %d…%d)", len(args), minParams-1, maxParams-1))
			}
		}
	}

	in := make([]reflect.Value, 1+len(args))
	in[0] = reflect.ValueOf(ctx)
	for k, param := range args {
		if param != nil {
			in[k+1] = reflect.ValueOf(param)
		} else {
			in[k+1] = reflect.Zero(reflect.TypeOf([]types.MalType{}).Elem())
		}
	}
	return in
}

func _args(minParams, maxParams int, args []types.MalType) []reflect.Value {
	if len(args) < minParams || len(args) > maxParams {
		if maxParams == unlimitedArgments {
			panic(fmt.Errorf("wrong number of arguments (%d instead of a minimum of %d)", len(args), minParams))
		} else {
			if minParams == maxParams {
				panic(fmt.Errorf("wrong number of arguments (%d instead of %d)", len(args), minParams))
			} else {
				panic(fmt.Errorf("wrong number of arguments (%d instead of %d…%d)", len(args), minParams, maxParams))
			}
		}
	}

	in := make([]reflect.Value, len(args))
	for k, param := range args {
		if param != nil {
			in[k] = reflect.ValueOf(param)
		} else {
			in[k] = reflect.Zero(reflect.TypeOf([]types.MalType{}).Elem())
		}
	}
	return in
}

func _nil_nil(res []reflect.Value) (result types.MalType, err error) {
	return nil, nil
}

func _nil_error(res []reflect.Value) (result types.MalType, err error) {
	if res[0].Interface() == nil {
		return nil, nil
	}
	return nil, res[0].Interface().(error)
}

func _result_error(res []reflect.Value) (result types.MalType, err error) {
	if res[1].Interface() == nil {
		return res[0].Interface(), nil
	}
	return res[0].Interface(), res[1].Interface().(error)
}
