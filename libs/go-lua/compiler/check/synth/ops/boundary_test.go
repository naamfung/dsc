package ops_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpsSynthHasNoCompilerImports verifies the architectural boundary:
// compiler/check/synth/ops must not import compiler/ or spec/contract packages.
func TestOpsSynthHasNoCompilerImports(t *testing.T) {
	forbiddenPrefixes := []string{
		"github.com/wippyai/go-lua/compiler/",
		"github.com/wippyai/go-lua/types/contract",
	}

	synthDir := "."
	fset := token.NewFileSet()

	err := filepath.Walk(synthDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Errorf("failed to parse %s: %v", path, parseErr)
			return nil
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, forbidden) {
					t.Errorf("%s imports forbidden package %s", path, importPath)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}
}
