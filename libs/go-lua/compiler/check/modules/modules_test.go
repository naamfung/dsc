package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConnect_CreatesManifest(t *testing.T) {
	database := db.New()
	name := "test_module"
	exportType := typ.NewRecord().Build()

	manifest := Connect(database, name, exportType, nil, nil, nil)
	if manifest == nil {
		t.Fatal("expected manifest to be created")
	}
}

func TestConnect_WithExportTypes(t *testing.T) {
	database := db.New()
	name := "test_module"
	exportType := typ.NewRecord().Build()
	exportTypes := map[string]typ.Type{
		"CustomType": typ.String,
	}

	manifest := Connect(database, name, exportType, exportTypes, nil, nil)
	if manifest == nil {
		t.Fatal("expected manifest to be created")
	}
}

func TestDisconnect_RemovesManifest(t *testing.T) {
	database := db.New()
	name := "test_module"
	manifest := io.NewManifest(name)
	database.Connect(name, manifest)

	Disconnect(database, name)
}

func TestExportFunctionSummaries_NilGraph(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.NewRecord().Build(), nil, nil)
}

func TestExportFunctionSummaries_EmptyEffects(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.NewRecord().Build(), nil, make(map[cfg.SymbolID]*constraint.FunctionRefinement))
}

func TestExportFunctionSummaries_NonRecordExportType(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.String, nil, nil)
}

func TestExportFieldNameFromSymbolName(t *testing.T) {
	tests := []struct {
		name      string
		fullName  string
		wantField string
		wantOK    bool
	}{
		{name: "direct field", fullName: "validate", wantField: "validate", wantOK: true},
		{name: "root field", fullName: "M.validate", wantField: "validate", wantOK: true},
		{name: "nested path rejected", fullName: "M.api.validate", wantField: "", wantOK: false},
		{name: "empty rejected", fullName: "", wantField: "", wantOK: false},
		{name: "trailing dot rejected", fullName: "M.", wantField: "", wantOK: false},
		{name: "leading dot rejected", fullName: ".x", wantField: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotField, gotOK := exportFieldNameFromSymbolName(tt.fullName)
			if gotOK != tt.wantOK {
				t.Fatalf("ok=%v, want %v", gotOK, tt.wantOK)
			}
			if gotField != tt.wantField {
				t.Fatalf("field=%q, want %q", gotField, tt.wantField)
			}
		})
	}
}
