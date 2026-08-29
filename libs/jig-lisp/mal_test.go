package lisp

import (
	"context"
	"sync"
	"testing"

	. "github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/lib/coreextented"
	"github.com/jig/lisp/types"
	. "github.com/jig/lisp/types"
)

func BenchmarkLoadSymbols(b *testing.B) {
	repl_env := NewEnv()
	for i := 0; i < b.N; i++ {
		core.Load(repl_env)
	}
}

func BenchmarkMAL1(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	concurrent.Load(repl_env)
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
			return EVAL(ctx, a[0], repl_env)
		}})
		repl_env.Set(Symbol{Val: "*ARGV*"}, List{})

		if _, err := REPL(ctx, repl_env, core.HeaderBasic(), types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
		if _, err := REPL(ctx, repl_env, core.HeaderLoadFile(), types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
		if _, err := REPL(ctx, repl_env, coreextented.HeaderCoreExtended(), types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
		if _, err := REPL(ctx, repl_env, concurrent.HeaderConcurrent(), types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMAL2(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if _, err := REPL(ctx, repl_env, `(def not (fn (a) (if a false true)))`, types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelREAD(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			str := `(def not (fn (a) (if a false true)))`

			var e error
			if _, e = READ(str, nil, repl_env); e != nil {
				b.Fatal(e)
			}

		}
	})
}

func BenchmarkParallelREP(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	ctx := context.Background()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := REPL(ctx, repl_env, `(def not (fn (a) (if a false true)))`, types.NewCursorFile(b.Name())); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkREP(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if _, err := REPL(ctx, repl_env, `(def not (fn (a) (if a false true)))`, types.NewCursorFile(b.Name())); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFibonacci(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_, err := REPL(ctx, repl_env, `(do
			(def fib
			(fn [n]                              ; non-negative number
			(if (<= n 1)
				n
				(+ (fib (- n 1)) (fib (- n 2))))))
			(fib 10))`, types.NewCursorFile(b.Name()))

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelFibonacci(b *testing.B) {
	repl_env := NewEnv()
	core.Load(repl_env)
	ctx := context.Background()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := REPL(ctx, repl_env, `(do
					(def fib
					(fn [n]                              ; non-negative number
					(if (<= n 1)
						n
						(+ (fib (- n 1)) (fib (- n 2))))))
					(fib 9))`, types.NewCursorFile(b.Name())); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestAtomParallel(t *testing.T) {
	repl_env := NewEnv()

	core.Load(repl_env)
	concurrent.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	ctx := context.Background()

	if _, err := REPL(ctx, repl_env, core.HeaderBasic(), types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, core.HeaderLoadFile(), types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, coreextented.HeaderCoreExtended(), types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, concurrent.HeaderConcurrent(), types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}

	if _, err := REPL(ctx, repl_env, "(def count (atom 0))", types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, "(def inc (fn [x] (+ 1 x)))", types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(err)
	}

	wd := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wd.Add(1)
		go func() {
			for j := 0; j < 1000; j++ {
				if _, err := REPL(ctx, repl_env, "(swap! count inc)", types.NewCursorFile(t.Name())); err != nil {
					return
				}
			}
			wd.Done()
		}()
	}
	wd.Wait()
	if _, err := REPL(ctx, repl_env, "(if (not (= @count 100000)) (throw @count))", types.NewCursorFile(t.Name())); err != nil {
		t.Fatal(REPL(ctx, repl_env, `(println "@count != " @count)`, types.NewCursorFile(t.Name())))
	}
}

func BenchmarkAtomParallel(b *testing.B) {
	repl_env := NewEnv()

	core.Load(repl_env)
	concurrent.Load(repl_env)
	repl_env.Set(Symbol{Val: "eval"}, Func{Fn: func(ctx context.Context, a []MalType) (MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})
	repl_env.Set(Symbol{Val: "*ARGV*"}, List{})
	ctx := context.Background()

	if _, err := REPL(ctx, repl_env, core.HeaderBasic(), types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, core.HeaderLoadFile(), types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, coreextented.HeaderCoreExtended(), types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, concurrent.HeaderConcurrent(), types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}

	if _, err := REPL(ctx, repl_env, "(def count (atom 0))", types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, "(def inc (fn [x] (+ 1 x)))", types.NewCursorFile(b.Name())); err != nil {
		b.Fatal(err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := REPL(ctx, repl_env, "(swap! count inc)", types.NewCursorFile(b.Name())); err != nil {
				b.Fatal(err)
			}
			// exp, err := READ("(swap! count inc)")
			// if err != nil {
			// 	b.Fatal(err)
			// }
			// if exp, err = EVAL(exp, repl_env, types.NewCursorFile(b.Name())); err != nil {
			// 	b.Fatal(err)
			// }
			// if _, err = PRINT(exp, types.NewCursorFile(b.Name())); err != nil {
			// 	b.Fatal(err)
			// }
		}
	})
}
