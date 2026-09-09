package call

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/types"
)

func TestOK(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, divExample)

	f, err := ns.Get(types.Symbol{Val: "divexample"})
	if err != nil {
		t.Fatal("test failed")
	}
	fcall, ok := f.(types.Func)
	if !ok {
		t.Fatal("test failed")
	}
	result, err := fcall.Fn(context.Background(), []types.MalType{2, 6})
	if result.(int) != 8 || err != nil {
		t.Fatal("test failed")
	}
}

func TestNoOKResult(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, divExample)

	f, err := ns.Get(types.Symbol{Val: "divexample"})
	if err != nil {
		t.Fatal("test failed")
	}
	if _, err = f.(types.Func).Fn(context.Background(), []types.MalType{2, 0}); err.Error() != "divide by zero" {
		t.Fatal("test failed")
	}
}

func TestNoOKArguments(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, divExample)

	f, _ := ns.Get(types.Symbol{Val: "divexample"})
	_, err := f.(types.Func).Fn(context.Background(), []types.MalType{2, 3, 4})
	if !strings.HasSuffix(err.Error(), "wrong number of arguments (3 instead of 2)") {
		t.Fatal(err)
	}
}

func TestOKWithContext(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sleepExample)

	f, err := ns.Get(types.Symbol{Val: "sleepexample"})
	if err != nil {
		t.Fatal("test failed")
	}
	if _, err = f.(types.Func).Fn(context.Background(), []types.MalType{10}); err != nil {
		t.Fatal("test failed")
	}
}

func TestOKWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ns := env.NewEnv()
	Call(ns, sleepExample)

	f, err := ns.Get(types.Symbol{Val: "sleepexample"})
	if err != nil {
		t.Fatal("test failed")
	}
	if _, err := f.(types.Func).Fn(ctx, []types.MalType{10}); err != nil {
		t.Fatal("test failed")
	}
}

func TestPackageRegister(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sleepExample)
	Call(ns, divExample)
	Call(ns, name_with_hyphens)
	Call(ns, name_With_Caps)

	hm, err := ns.Get(types.Symbol{Val: "_PACKAGES_"})
	if err != nil {
		t.Fatal(err)
	}
	set := hm.(types.HashMap).Val["github.com/jig/lisp/lib/call"].(types.Set).Val
	if len(set) != 4 {
		t.Fatal("test failed")
	}
	if _, ok := set["divexample"]; !ok {
		t.Fatal("test failed")
	}
	if _, ok := set["sleepexample"]; !ok {
		t.Fatal("test failed")
	}
	if _, ok := set["name-with-hyphens"]; !ok {
		t.Fatal("test failed")
	}
	if _, ok := set["name-with-caps"]; !ok {
		t.Fatal("test failed")
	}
}

func TestNoArgsNoResult(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, no_args_no_res)

	f, err := ns.Get(types.Symbol{Val: "no-args-no-res"})
	if err != nil {
		t.Fatal("test failed")
	}
	fcall, ok := f.(types.Func)
	if !ok {
		t.Fatal("test failed")
	}
	result, err := fcall.Fn(context.Background(), []types.MalType{})
	if result != nil || err != nil {
		t.Fatal("test failed")
	}
}

func TestVariadic(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sum_Example)

	f, err := ns.Get(types.Symbol{Val: "sum-example"})
	if err != nil {
		t.Fatal(err)
	}
	fcall, ok := f.(types.Func)
	if !ok {
		t.Fatal("test failed")
	}
	result, err := fcall.Fn(context.Background(), []types.MalType{1, 2, 3, 4, 5, 6})
	if result.(int) != 21 || err != nil {
		t.Fatal("test failed")
	}
}

func TestVariadicNoArgs(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sum_Example)

	f, err := ns.Get(types.Symbol{Val: "sum-example"})
	if err != nil {
		t.Fatal(err)
	}
	fcall, ok := f.(types.Func)
	if !ok {
		t.Fatal("test failed")
	}
	result, err := fcall.Fn(context.Background(), []types.MalType{})
	if result.(int) != 0 || err != nil {
		t.Fatal("test failed")
	}
}

func TestLisp(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sum_Example)

	res, err := lisp.REPL(context.Background(), ns, `(sum-example 33)`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if res.(string) != "33" {
		t.Fatal("test failed")
	}
}

func TestLispNil(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, sum_Example)

	ast, err := lisp.READ(`(sum-example nil)`, types.NewCursorFile(t.Name()), ns)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lisp.EVAL(context.Background(), ast, ns)
	if !strings.Contains(err.Error(), "reflect: cannot use types.MalType as type int in Call") {
		t.Fatal(err)
	}
}

func TestWrongTypePassed(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, divExample)

	_, err := lisp.REPL(context.Background(), ns, `(divexample "hello" "world")`, types.NewCursorFile(t.Name()))
	if !strings.Contains(err.Error(), "reflect: Call using string as type int") {
		t.Fatal(err)
	}
}

func TestCount(t *testing.T) {
	ns := env.NewEnv()
	Call(ns, count)

	res, err := lisp.REPL(context.Background(), ns, `(count nil)`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if res.(string) != "0" {
		t.Fatal("test failed")
	}
}

func TestEmpty(t *testing.T) {
	ns := env.NewEnv()
	CallOverrideFN(ns, "empty?", empty_Q)

	ast, err := lisp.READ(`(empty? "hello")`, types.NewCursorFile(t.Name()), ns)
	if err != nil {
		t.Fatal(err)
	}
	res, err := lisp.EVAL(context.Background(), ast, ns)
	if !strings.Contains(err.Error(), "empty? called on non-sequence") {
		t.Fatal(err)
	}
	if res != nil {
		t.Fatal("test failed")
	}
}

func TestNilResponse(t *testing.T) {
	ns := env.NewEnv()
	CallOverrideFN(ns, "nilly", func() (types.MalType, error) { return nil, nil })

	res, err := lisp.REPL(context.Background(), ns, `(nilly)`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if res.(string) != "nil" {
		t.Fatal("test failed")
	}
}

func count(seq types.MalType) (types.MalType, error) {
	switch seq := seq.(type) {
	case types.List:
		return len(seq.Val), nil
	case types.Vector:
		return len(seq.Val), nil
	case types.HashMap:
		return len(seq.Val), nil
	case types.Set:
		return len(seq.Val), nil
	case nil:
		return 0, nil
	default:
		return nil, fmt.Errorf("count called on non-sequence %T, %s, %#v", seq, seq, seq)
	}
}

func empty_Q(seq types.MalType) (types.MalType, error) {
	switch seq := seq.(type) {
	case types.List:
		return len(seq.Val) == 0, nil
	case types.Vector:
		return len(seq.Val) == 0, nil
	case types.HashMap:
		return len(seq.Val) == 0, nil
	case types.Set:
		return len(seq.Val) == 0, nil
	case nil:
		return true, nil
	default:
		return nil, errors.New("empty? called on non-sequence")
	}
}

// Function examples

func divExample(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a + b, nil
}

func sleepExample(ctx context.Context, ms int) error {
	select {
	case <-ctx.Done():
		return errors.New("timeout while evaluating expression")
	case <-time.After(time.Millisecond * time.Duration(ms)):
		return nil
	}
}

func name_with_hyphens(ctx context.Context, ms int) error {
	return nil
}

func no_args_no_res() {}

func name_With_Caps(ctx context.Context, ms int) error {
	return nil
}

func sum_Example(a ...int) (int, error) {
	acc := 0
	for _, item := range a {
		acc += item
	}
	return acc, nil
}
