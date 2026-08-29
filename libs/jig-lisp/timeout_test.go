package lisp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/types"
)

func TestContextTimeoutFiresOnTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := REPL(ctx, newEnv(t.Name()), `(sleep 1000)`, types.NewCursorFile(t.Name())); err == nil {
		t.Fatalf("Must fail")
	} else {
		if !strings.Contains(err.Error(), "timeout while evaluating expression") {
			t.Fatalf("%s != %s", err.Error(), "timeout while evaluating expression")
		}
	}
}

func TestContextNoTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := REPL(ctx, newEnv(t.Name()), `(sleep 1)`, types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
}

func TestFutureContextTimeoutFiresOnTime(t *testing.T) {
	ctxB := context.Background()
	env := newEnv(t.Name())
	concurrent.Load(env)
	concurrentHeader := concurrent.HeaderConcurrent()
	if _, err := REPL(ctxB, env, concurrentHeader, types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := REPL(ctx, newEnv(t.Name()), `@(future (sleep 1000))`, types.NewCursorFile(t.Name())); err == nil {
		t.Fatalf("Must fail")
	} else {
		if !strings.Contains(err.Error(), "timeout while dereferencing future") {
			t.Fatalf("%s != %s", err.Error(), "timeout while dereferencing future")
		}
	}
}

func TestFutureContextNoTimeout(t *testing.T) {
	ctxB := context.Background()
	env := newEnv(t.Name())
	concurrent.Load(env)
	concurrentHeader := concurrent.HeaderConcurrent()
	if _, err := REPL(ctxB, env, concurrentHeader, types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()

	if _, err := REPL(ctx, newEnv(t.Name()), `@(future (sleep 1))`, types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutOnTryCatch(t *testing.T) {
	ns := newEnv(t.Name())
	ast, err := READ(`(try (sleep 10000) (catch e (str "ERR: " (error-string e))))`, types.NewCursorFile(t.Name()), ns)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	res, err := EVAL(ctx, ast, ns)
	if err != nil {
		if err.Error() == "timeout while evaluating expression" {
			t.Fatalf("timeout not catched: %s", err)
		}
		t.Fatalf("unexpected error caught %s", err)
	}

	if res.(string) != "ERR: timeout while evaluating expression" {
		t.Fatalf("unexpected result %s", res)
	}
}
