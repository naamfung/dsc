package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

type manifestQuerierStub struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (s *manifestQuerierStub) Manifest(path string) *io.Manifest {
	if s == nil || s.manifests == nil {
		return nil
	}
	return s.manifests[path]
}

func (s *manifestQuerierStub) Imports() map[string]*io.Manifest {
	if s == nil {
		return nil
	}
	return s.imports
}

func newManifest(name string, export typ.Type) *io.Manifest {
	m := io.NewManifest(name)
	m.SetExport(export)
	return m
}

func TestApplyModuleAliasExports_AssignsResolvedExports(t *testing.T) {
	aliases := map[cfg.SymbolID]string{
		0: "skip-zero",
		1: "a",
		2: "",
		3: "nil-export",
		4: "from-imports",
	}
	q := &manifestQuerierStub{
		manifests: map[string]*io.Manifest{
			"a":          newManifest("a", typ.String),
			"nil-export": newManifest("nil-export", nil),
		},
		imports: map[string]*io.Manifest{
			"from-imports": newManifest("from-imports", typ.Boolean),
		},
	}

	got := applyModuleAliasExports(nil, aliases, q)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("sym 1 = %v, want string", got[1])
	}
	if !typ.TypeEquals(got[4], typ.Boolean) {
		t.Fatalf("sym 4 = %v, want boolean", got[4])
	}
}

func TestApplyModuleAliasExports_PreservesExistingOnNoResolution(t *testing.T) {
	declared := flow.DeclaredTypes{
		9: typ.Integer,
	}
	aliases := map[cfg.SymbolID]string{
		9: "missing",
	}
	got := applyModuleAliasExports(declared, aliases, &manifestQuerierStub{})
	if !typ.TypeEquals(got[9], typ.Integer) {
		t.Fatalf("existing type changed: got %v, want integer", got[9])
	}
}
