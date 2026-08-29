package env

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/types"
)

type Env struct {
	mu    *sync.RWMutex
	data  map[string]interface{}
	outer *Env
}

func NewEnv() types.EnvType {
	return _newEnv()
}

func NewSubordinateEnv(outer types.EnvType) types.EnvType {
	return _newSubordinateEnv(outer.(*Env))
}

func NewSubordinateEnvWithBinds(outer types.EnvType, binds_mt types.MalType, exprs_mt types.MalType) (types.EnvType, error) {
	return _newSubordinateEnvWithBinds(outer.(*Env), binds_mt, exprs_mt)
}

func _newEnv() *Env {
	return &Env{
		data: map[string]interface{}{},
		mu:   &sync.RWMutex{},
	}
}

func _newSubordinateEnv(outer *Env) *Env {
	env := _newEnv()
	env.outer = outer
	return env
}

func _newSubordinateEnvWithBinds(outer *Env, binds_mt types.MalType, exprs_mt types.MalType) (types.EnvType, error) {
	env := _newSubordinateEnv(outer)

	if binds_mt != nil && exprs_mt != nil {
		binds, e := types.GetSlice(binds_mt)
		if e != nil {
			return nil, e
		}
		exprs, e := types.GetSlice(exprs_mt)
		if e != nil {
			return nil, e
		}
		// Return a new Env with types.symbols in binds bound to
		// corresponding values in exprs
		var varargs bool
		i := 0
		for ; i < len(binds); i++ {
			if types.Q[types.Symbol](binds[i]) && binds[i].(types.Symbol).Val == "&" {
				env.data[binds[i+1].(types.Symbol).Val] = types.List{Val: exprs[i:]}
				varargs = true
				break
			} else {
				if i == len(exprs) {
					return nil, lisperror.NewLispError(fmt.Errorf("too few arguments passed (%d binds, %d arguments passed)", len(binds), len(exprs)), nil)
				}
				env.data[binds[i].(types.Symbol).Val] = exprs[i]
			}
		}
		if !varargs && len(exprs) != i {
			// return nil, fmt.Errorf("too many arguments passed (%d binds, %d arguments passed)", len(binds), len(exprs))
			return nil, lisperror.NewLispError(fmt.Errorf("too many arguments passed (%d binds, %d arguments passed)", len(binds), len(exprs)), nil)
		}
	}
	//return &et, nil
	return env, nil
}

func (e *Env) Find(key types.Symbol) types.EnvType {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.FindNT(key)
}

func (e *Env) Set(key types.Symbol, value types.MalType) types.MalType {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.SetNT(key, value)
}

func (e *Env) Remove(key types.Symbol) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.RemoveNT(key)
}

func (e *Env) Get(key types.Symbol) (types.MalType, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.GetNT(key)
}

func (e *Env) Update(key types.Symbol, f func(types.MalType) (types.MalType, error)) (types.MalType, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	v, _ := e.GetNT(key)
	newV, err := f(v)
	if err != nil {
		return nil, err
	}
	return e.SetNT(key, newV), nil
}

func (e *Env) Symbols(newLine [][]rune, lastPartial string) [][]rune {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var localNewLine []string

	for key := range e.data {
		if strings.HasPrefix(key, lastPartial) {
			localNewLine = append(localNewLine, key[len(lastPartial):])
		}
	}
	sort.Strings(localNewLine)

	// append localNewLine to newLine
	for _, s := range localNewLine {
		newLine = append(newLine, []rune(s))
	}

	if e.outer != nil {
		return e.outer.Symbols(newLine, lastPartial)
	}
	return newLine
}

func (e *Env) FindNT(key types.Symbol) types.EnvType {
	if _, ok := e.data[key.Val]; ok {
		return e
	} else if e.outer != nil {
		// do-not-use-FindNT-here
		return e.outer.Find(key)
	} else {
		return nil
	}
}

func (e *Env) SetNT(key types.Symbol, value types.MalType) types.MalType {
	e.data[key.Val] = value
	return value
}

func (e *Env) GetNT(key types.Symbol) (types.MalType, error) {
	if v, ok := e.data[key.Val]; ok {
		return v, nil
	} else if e.outer != nil {
		// do-not-use-GetNT-here
		return e.outer.Get(key)
	} else {
		// return nil, errors.New("'" + key.Val + "' not found")
		return nil, lisperror.NewLispError(fmt.Errorf("symbol '%w' not found", errors.New(key.Val)), key)
	}
}

func (e *Env) RemoveNT(key types.Symbol) error {
	if _, ok := e.data[key.Val]; !ok {
		// return errors.New("types.symbol not found")
		return lisperror.NewLispError(fmt.Errorf("symbol '%w' not found", errors.New(key.Val)), key)
	}
	delete(e.data, key.Val)
	return nil
}
