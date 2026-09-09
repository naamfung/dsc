package lisp_test

import (
	"context"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
)

func TestInt(t *testing.T) {
	ast, err := lisp.READ("1000", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(int) != 1000 {
		t.Fatal(`ast.(int) != 1000`)
	}
}

func TestIntUnderscoreSeparator(t *testing.T) {
	ast, err := lisp.READ("100_000", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(int) != 100_000 {
		t.Fatal(`ast.(int) != 100_000`)
	}
}

func TestFloat(t *testing.T) {
	ast, err := lisp.READ("3.1416", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(float32) != 3.1416 {
		t.Fatal(`ast.(float32) != 3.1416`)
	}
	res, err := lisp.EVAL(context.Background(), ast, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if res.(float32) != 3.1416 {
		t.Fatal(`ast.(float32) != 3.1416`)
	}
}

func TestHexa(t *testing.T) {
	ast, err := lisp.READ("0xCAFE", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(int) != 0xCAFE {
		t.Fatal(`ast.(int) != 0XCAFE`)
	}
	res, err := lisp.EVAL(context.Background(), ast, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if res.(int) != 0xCAFE {
		t.Fatal(`ast.(int) != 0XCAFE`)
	}
}

func TestOctal(t *testing.T) {
	ast, err := lisp.READ("0o7777", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(int) != 0o7777 {
		t.Fatal(`ast.(int) != 0o7777`)
	}
	res, err := lisp.EVAL(context.Background(), ast, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if res.(int) != 0o7777 {
		t.Fatal(`ast.(int) != 0o7777`)
	}
}

func TestBinary(t *testing.T) {
	ast, err := lisp.READ("0b1100", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(int) != 0b1100 {
		t.Fatal(`ast.(int) != 0b1100`)
	}
	res, err := lisp.EVAL(context.Background(), ast, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if res.(int) != 0b1100 {
		t.Fatal(`ast.(int) != 0b1100`)
	}
}
