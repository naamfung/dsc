package testlib

import (
	"context"
	"embed"
	"log"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/types"
)

type PackageDecl []struct {
	Name string
	Load func(tenv types.EnvType) error
}

func Directory(t *testing.T, directory embed.FS, packages PackageDecl) error {
	d, err := directory.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range d {
		t.Run(entry.Name(), func(t *testing.T) {
			testFile, err := directory.ReadFile(entry.Name())
			if err != nil {
				t.Fatalf("%s/ReadFile Error: %s", entry.Name(), err)
			}
			File(t, entry.Name(), string(testFile), packages)
		})
	}
	return nil
}

func File(t *testing.T, entryName, testFile string, packages PackageDecl) {
	tenv := env.NewEnv()
	for _, library := range packages {
		if err := library.Load(tenv); err != nil {
			log.Fatalf("Library Load Error: %v\n", err)
		}
	}
	expr, err := lisp.READ(string(testFile), types.NewCursorFile(entryName), tenv)
	if err != nil {
		t.Fatalf("%s/READ Error: %s", entryName, err)
	}
	ctx := context.Background()
	res, err := lisp.EVAL(ctx, expr, tenv)
	if err != nil {
		t.Fatalf("%s/EVAL Error: %s", entryName, err)
	}
	if r, ok := res.(bool); !r || !ok {
		t.Fatalf("%s/TEST failed: %v", entryName, res)
	}
}
