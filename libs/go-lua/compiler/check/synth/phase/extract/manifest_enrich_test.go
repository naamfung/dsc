package extract

import (
	"testing"

	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

type manifestQuerierStub struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (m manifestQuerierStub) Manifest(path string) *io.Manifest {
	return m.manifests[path]
}

func (m manifestQuerierStub) Imports() map[string]*io.Manifest {
	return m.imports
}

func TestEnrichWithManifest_DirectLookup(t *testing.T) {
	manifest := io.NewManifest("m")
	manifest.SetExport(typ.NewRecord().Field("name", typ.String).Build())
	got := enrichWithManifest(manifestQuerierStub{
		manifests: map[string]*io.Manifest{"m": manifest},
	}, typ.Number, "m", "name")
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("enrichWithManifest direct = %v, want string", got)
	}
}

func TestEnrichWithManifest_ImportsFallback(t *testing.T) {
	manifest := io.NewManifest("m")
	manifest.SetExport(typ.NewRecord().Field("name", typ.String).Build())
	got := enrichWithManifest(manifestQuerierStub{
		manifests: map[string]*io.Manifest{},
		imports:   map[string]*io.Manifest{"m": manifest},
	}, typ.Number, "m", "name")
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("enrichWithManifest imports fallback = %v, want string", got)
	}
}

func TestEnrichWithManifest_MissingFieldKeepsBase(t *testing.T) {
	manifest := io.NewManifest("m")
	manifest.SetExport(typ.NewRecord().Field("name", typ.String).Build())
	got := enrichWithManifest(manifestQuerierStub{
		manifests: map[string]*io.Manifest{"m": manifest},
	}, typ.Number, "m", "missing")
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("enrichWithManifest missing = %v, want base number", got)
	}
}
