package reader_test

import (
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/call"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/reader"
	"github.com/jig/lisp/types"
)

type Example struct {
	N int
	S string
}

type Embeddable struct {
	in *Embeddable
}

func new_embeddable(in ...Embeddable) (Embeddable, error) {
	if len(in) == 1 {
		return Embeddable{
			in: &in[0],
		}, nil
	}
	return Embeddable{}, nil
}

func new_example(n int, s string) (Example, error) {
	return Example{
		N: n,
		S: s,
	}, nil
}

func (ex Example) LispPrint(_Pr_str func(types.MalType, bool) string) string {
	return "«example " + _Pr_str(ex.N, true) + " " + _Pr_str(ex.S, true) + "»"
}

func (em Embeddable) LispPrint(_Pr_str func(types.MalType, bool) string) string {
	if em.in == nil {
		return "«embeddable»"
	}
	return "«embeddable " + _Pr_str(em.in, true) + "»"
}

func TestAdHocReaders(t *testing.T) {
	ns := env.NewEnv()
	if err := nscore.Load(ns); err != nil {
		t.Fatal()
	}
	call.Call(ns, new_example)
	call.Call(ns, new_embeddable, 0, 1)

	t.Run("example", func(t *testing.T) {
		ast, err := reader.Read_str(`«example 33 "hello"»`, types.NewCursorFile(t.Name()), nil, ns)
		if err != nil {
			t.Error(err)
		}
		switch ast := ast.(type) {
		case Example:
			if ast.N != 33 || ast.S != "hello" {
				t.Fatal()
			}
		default:
			t.Fatal()
		}
	})
	t.Run("error", func(t *testing.T) {
		ast, err := reader.Read_str(`«error "poum"»`, types.NewCursorFile(t.Name()), nil, ns)
		if err != nil {
			t.Error(err)
		}
		switch ast := ast.(type) {
		case lisperror.LispError:
			if ast.ErrorValue() != "poum" {
				t.Fatal()
			}
		default:
			t.Fatal()
		}
	})
	t.Run("error in error", func(t *testing.T) {
		ast, err := reader.Read_str(`«error «error "poum"»»`, types.NewCursorFile(t.Name()), nil, ns)
		if err != nil {
			t.Error(err)
		}
		switch ast.(type) {
		case lisperror.LispError:
			// currently internal LispError is not wrapped in another LispError so this is not tested
		default:
			t.Fatal()
		}
	})
	t.Run("embeddable", func(t *testing.T) {
		ast, err := reader.Read_str(`«embeddable»`, types.NewCursorFile(t.Name()), nil, ns)
		if err != nil {
			t.Error(err)
		}
		switch ast := ast.(type) {
		case nil:
			t.Fatal()
		case Embeddable:
			if ast.in != nil {
				t.Fatal()
			}
		default:
			t.Fatal()
		}
	})
	t.Run("embeddable in embeddable", func(t *testing.T) {
		ast, err := reader.Read_str(`«embeddable «embeddable»»`, types.NewCursorFile(t.Name()), nil, ns)
		if err != nil {
			t.Error(err)
		}
		switch ast.(type) {
		case Embeddable:
		case nil:
			t.Fatal()
		default:
			t.Fatal()
		}
	})
}
