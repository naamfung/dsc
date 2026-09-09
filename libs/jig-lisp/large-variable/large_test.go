package largevariable

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
)

var largefile = make([]byte, 4_000_000)

func TestMain(t *testing.T) {
	n, err := rand.Read(largefile)
	if err != nil {
		t.Fatal()
	}
	if n < 1_000_000 {
		t.Fatal("wrongfile1")
	}
	if len(largefile) < 1_000_000 {
		t.Fatal("wrongfile2")
	}
}

func BenchmarkLargeFileBase64Encode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = base64.StdEncoding.EncodeToString(largefile)
	}
}

func BenchmarkLargeFileBase64Decode(b *testing.B) {
	b64 := base64.StdEncoding.EncodeToString(largefile)
	for i := 0; i < b.N; i++ {
		_, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			b.Fatal()
		}
	}
}

func BenchmarkLargeFileLispBase64READ(b *testing.B) {
	b64 := base64.StdEncoding.EncodeToString(largefile)
	lispToken := fmt.Sprintf("(unbase64 %q)", b64)
	for i := 0; i < b.N; i++ {
		ns := env.NewEnv()
		_, err := lisp.READ(lispToken, nil, ns)
		if err != nil {
			b.Fatal()
		}
	}
}

func BenchmarkLargeFileLispBase64EVAL(b *testing.B) {
	b64 := base64.StdEncoding.EncodeToString(largefile)
	lispToken := fmt.Sprintf("(unbase64 %q)", b64)

	ns := env.NewEnv()
	err := nscore.Load(ns)
	if err != nil {
		b.Fatal()
	}
	ast, err := lisp.READ(lispToken, nil, ns)
	if err != nil {
		b.Fatal()
	}

	for i := 0; i < b.N; i++ {
		_, err := lisp.EVAL(context.Background(), ast, ns)
		if err != nil {
			b.Fatal()
		}
	}
}

func BenchmarkLargeFileLispBase64RUN(b *testing.B) {
	b64 := base64.StdEncoding.EncodeToString(largefile)
	lispToken := fmt.Sprintf("(unbase64 %q)", b64)

	for i := 0; i < b.N; i++ {
		ns := env.NewEnv()
		err := nscore.Load(ns)
		if err != nil {
			b.Fatal()
		}
		ast, err := lisp.READ(lispToken, nil, ns)
		if err != nil {
			b.Fatal()
		}
		res, err := lisp.EVAL(context.Background(), ast, ns)
		if err != nil {
			b.Fatal()
		}
		if !bytes.Equal(res.([]byte), largefile) {
			b.Fatal()
		}
	}
}
