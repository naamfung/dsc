package lsp

import (
	"testing"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		cache   *index.DB
		symbols *index.SymbolIndex
	}{
		{
			name:    "with both cache and symbols",
			cache:   index.NewDB(),
			symbols: index.NewSymbolIndex(),
		},
		{
			name:    "with nil cache",
			cache:   nil,
			symbols: index.NewSymbolIndex(),
		},
		{
			name:    "with nil symbols",
			cache:   index.NewDB(),
			symbols: nil,
		},
		{
			name:    "with both nil",
			cache:   nil,
			symbols: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.cache, tt.symbols, nil)
			if svc == nil {
				t.Fatal("expected non-nil service")
			}
			if svc.Cache() != tt.cache {
				t.Error("cache mismatch")
			}
			if svc.Symbols() != tt.symbols {
				t.Error("symbols mismatch")
			}
		})
	}
}

func TestService_HoverAt(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Service
		file     string
		line     int
		col      int
		wantNil  bool
		validate func(*testing.T, *HoverResult)
	}{
		{
			name: "symbol found at position",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"test.lua",
					"foo",
					index.SymbolVariable,
					"string",
					diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 13},
					"",
				)
				if sym == nil {
					t.Fatal("failed to add definition")
				}
				return NewService(nil, idx, nil)
			},
			file:    "test.lua",
			line:    5,
			col:     11,
			wantNil: false,
			validate: func(t *testing.T, hr *HoverResult) {
				if hr.Symbol == nil {
					t.Error("expected non-nil symbol")
				}
				if hr.Symbol.Name != "foo" {
					t.Errorf("expected symbol name 'foo', got %s", hr.Symbol.Name)
				}
				if hr.Type != "string" {
					t.Errorf("expected type 'string', got %v", hr.Type)
				}
				if hr.Span.StartLine != 5 {
					t.Errorf("expected span start line 5, got %d", hr.Span.StartLine)
				}
			},
		},
		{
			name: "no symbol at position",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"test.lua",
					"foo",
					index.SymbolVariable,
					"string",
					diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 13},
					"",
				)
				return NewService(nil, idx, nil)
			},
			file:    "test.lua",
			line:    10,
			col:     1,
			wantNil: true,
		},
		{
			name: "nil symbols index",
			setup: func() *Service {
				return NewService(nil, nil, nil)
			},
			file:    "test.lua",
			line:    5,
			col:     11,
			wantNil: true,
		},
		{
			name: "symbol reference position",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"test.lua",
					"bar",
					index.SymbolFunction,
					"function",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4},
					"",
				)
				idx.AddReference(
					"test.lua",
					sym,
					diag.Span{StartLine: 10, StartCol: 5, EndLine: 10, EndCol: 8},
				)
				return NewService(nil, idx, nil)
			},
			file:    "test.lua",
			line:    10,
			col:     6,
			wantNil: false,
			validate: func(t *testing.T, hr *HoverResult) {
				if hr.Symbol == nil {
					t.Error("expected non-nil symbol")
				}
				if hr.Symbol.Name != "bar" {
					t.Errorf("expected symbol name 'bar', got %s", hr.Symbol.Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup()
			result := svc.HoverAt(tt.file, tt.line, tt.col)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestService_DefinitionAt(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Service
		file     string
		line     int
		col      int
		wantNil  bool
		validate func(*testing.T, *DefinitionResult)
	}{
		{
			name: "definition found",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"main.lua",
					"myVar",
					index.SymbolVariable,
					"number",
					diag.Span{StartLine: 3, StartCol: 5, EndLine: 3, EndCol: 10},
					"",
				)
				return NewService(nil, idx, nil)
			},
			file:    "main.lua",
			line:    3,
			col:     7,
			wantNil: false,
			validate: func(t *testing.T, dr *DefinitionResult) {
				if dr.File != "main.lua" {
					t.Errorf("expected file 'main.lua', got %s", dr.File)
				}
				if dr.Span.StartLine != 3 {
					t.Errorf("expected start line 3, got %d", dr.Span.StartLine)
				}
				if dr.Span.StartCol != 5 {
					t.Errorf("expected start col 5, got %d", dr.Span.StartCol)
				}
			},
		},
		{
			name: "definition from reference",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"lib.lua",
					"helper",
					index.SymbolFunction,
					"function",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7},
					"",
				)
				idx.AddReference(
					"main.lua",
					sym,
					diag.Span{StartLine: 20, StartCol: 10, EndLine: 20, EndCol: 16},
				)
				return NewService(nil, idx, nil)
			},
			file:    "main.lua",
			line:    20,
			col:     12,
			wantNil: false,
			validate: func(t *testing.T, dr *DefinitionResult) {
				if dr.File != "lib.lua" {
					t.Errorf("expected file 'lib.lua', got %s", dr.File)
				}
				if dr.Span.StartLine != 1 {
					t.Errorf("expected start line 1, got %d", dr.Span.StartLine)
				}
			},
		},
		{
			name: "no definition found",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				return NewService(nil, idx, nil)
			},
			file:    "test.lua",
			line:    5,
			col:     5,
			wantNil: true,
		},
		{
			name: "nil symbols index",
			setup: func() *Service {
				return NewService(nil, nil, nil)
			},
			file:    "test.lua",
			line:    5,
			col:     5,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup()
			result := svc.DefinitionAt(tt.file, tt.line, tt.col)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestService_ReferencesAt(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Service
		file        string
		line        int
		col         int
		includeDecl bool
		wantNil     bool
		wantCount   int
		validate    func(*testing.T, []ReferenceResult)
	}{
		{
			name: "references found with declaration",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"main.lua",
					"count",
					index.SymbolVariable,
					"number",
					diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 6},
					"",
				)
				idx.AddReference(
					"main.lua",
					sym,
					diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 15},
				)
				idx.AddReference(
					"main.lua",
					sym,
					diag.Span{StartLine: 8, StartCol: 5, EndLine: 8, EndCol: 10},
				)
				return NewService(nil, idx, nil)
			},
			file:        "main.lua",
			line:        2,
			col:         3,
			includeDecl: true,
			wantNil:     false,
			wantCount:   3,
			validate: func(t *testing.T, refs []ReferenceResult) {
				defCount := 0
				readCount := 0
				for _, ref := range refs {
					switch ref.Kind {
					case RefDefinition:
						defCount++
						if ref.Span.StartLine != 2 {
							t.Errorf("expected definition at line 2, got %d", ref.Span.StartLine)
						}
					case RefRead:
						readCount++
					}
				}
				if defCount != 1 {
					t.Errorf("expected 1 definition, got %d", defCount)
				}
				if readCount != 2 {
					t.Errorf("expected 2 read references, got %d", readCount)
				}
			},
		},
		{
			name: "references found without declaration",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"main.lua",
					"data",
					index.SymbolVariable,
					"table",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
					"",
				)
				idx.AddReference(
					"main.lua",
					sym,
					diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 5},
				)
				return NewService(nil, idx, nil)
			},
			file:        "main.lua",
			line:        1,
			col:         2,
			includeDecl: false,
			wantNil:     false,
			wantCount:   1,
			validate: func(t *testing.T, refs []ReferenceResult) {
				for _, ref := range refs {
					if ref.Kind == RefDefinition {
						t.Error("expected no definition in results")
					}
				}
			},
		},
		{
			name: "no references found",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"main.lua",
					"unused",
					index.SymbolVariable,
					"nil",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7},
					"",
				)
				return NewService(nil, idx, nil)
			},
			file:        "main.lua",
			line:        1,
			col:         3,
			includeDecl: false,
			wantNil:     false,
			wantCount:   0,
		},
		{
			name: "only declaration when includeDecl true and no refs",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"main.lua",
					"single",
					index.SymbolVariable,
					"string",
					diag.Span{StartLine: 7, StartCol: 1, EndLine: 7, EndCol: 7},
					"",
				)
				return NewService(nil, idx, nil)
			},
			file:        "main.lua",
			line:        7,
			col:         4,
			includeDecl: true,
			wantNil:     false,
			wantCount:   1,
			validate: func(t *testing.T, refs []ReferenceResult) {
				if refs[0].Kind != RefDefinition {
					t.Error("expected definition kind")
				}
			},
		},
		{
			name: "no symbol at position",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				return NewService(nil, idx, nil)
			},
			file:        "main.lua",
			line:        100,
			col:         50,
			includeDecl: true,
			wantNil:     true,
		},
		{
			name: "nil symbols index",
			setup: func() *Service {
				return NewService(nil, nil, nil)
			},
			file:        "test.lua",
			line:        1,
			col:         1,
			includeDecl: true,
			wantNil:     true,
		},
		{
			name: "references from reference position",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				sym := idx.AddDefinition(
					"def.lua",
					"global",
					index.SymbolVariable,
					"any",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7},
					"",
				)
				idx.AddReference(
					"use.lua",
					sym,
					diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 16},
				)
				idx.AddReference(
					"use.lua",
					sym,
					diag.Span{StartLine: 10, StartCol: 5, EndLine: 10, EndCol: 11},
				)
				return NewService(nil, idx, nil)
			},
			file:        "use.lua",
			line:        5,
			col:         12,
			includeDecl: true,
			wantNil:     false,
			wantCount:   3,
			validate: func(t *testing.T, refs []ReferenceResult) {
				defFile := ""
				for _, ref := range refs {
					if ref.Kind == RefDefinition {
						defFile = ref.File
					}
				}
				if defFile != "def.lua" {
					t.Errorf("expected definition file 'def.lua', got %s", defFile)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup()
			result := svc.ReferencesAt(tt.file, tt.line, tt.col, tt.includeDecl)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
				return
			}

			if result == nil && tt.wantCount > 0 {
				t.Fatal("expected non-nil result")
			}

			if len(result) != tt.wantCount {
				t.Errorf("expected %d references, got %d", tt.wantCount, len(result))
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestService_InvalidateFile(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() (*Service, *index.DB, *index.SymbolIndex)
		file     string
		validate func(*testing.T, *index.DB, *index.SymbolIndex)
	}{
		{
			name: "invalidates cache and symbols",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				idx := index.NewSymbolIndex()

				db.Set(index.Key{File: "test.lua", Func: "foo", Kind: "check"}, "value1", nil)
				db.Set(index.Key{File: "other.lua", Func: "bar", Kind: "check"}, "value2", nil)

				idx.AddDefinition(
					"test.lua",
					"var1",
					index.SymbolVariable,
					"string",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
					"",
				)
				idx.AddDefinition(
					"other.lua",
					"var2",
					index.SymbolVariable,
					"number",
					diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 5},
					"",
				)

				svc := NewService(db, idx, nil)
				return svc, db, idx
			},
			file: "test.lua",
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Has(index.Key{File: "test.lua", Func: "foo", Kind: "check"}) {
					t.Error("expected test.lua cache to be invalidated")
				}
				if !db.Has(index.Key{File: "other.lua", Func: "bar", Kind: "check"}) {
					t.Error("expected other.lua cache to remain valid")
				}

				if len(idx.SymbolsInFile("test.lua")) > 0 {
					t.Error("expected test.lua symbols to be invalidated")
				}
				if len(idx.SymbolsInFile("other.lua")) == 0 {
					t.Error("expected other.lua symbols to remain")
				}
			},
		},
		{
			name: "handles nil cache",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"test.lua",
					"var",
					index.SymbolVariable,
					"any",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4},
					"",
				)
				svc := NewService(nil, idx, nil)
				return svc, nil, idx
			},
			file: "test.lua",
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if len(idx.SymbolsInFile("test.lua")) > 0 {
					t.Error("expected symbols to be invalidated")
				}
			},
		},
		{
			name: "handles nil symbols",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				db.Set(index.Key{File: "test.lua", Func: "foo", Kind: "check"}, "value", nil)
				svc := NewService(db, nil, nil)
				return svc, db, nil
			},
			file: "test.lua",
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Has(index.Key{File: "test.lua", Func: "foo", Kind: "check"}) {
					t.Error("expected cache to be invalidated")
				}
			},
		},
		{
			name: "handles both nil",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				svc := NewService(nil, nil, nil)
				return svc, nil, nil
			},
			file: "test.lua",
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
			},
		},
		{
			name: "invalidates with dependents",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				idx := index.NewSymbolIndex()

				k1 := index.Key{File: "base.lua", Func: "", Kind: "types"}
				k2 := index.Key{File: "dep.lua", Func: "foo", Kind: "check"}

				db.Set(k1, "types", nil)
				db.Set(k2, "value", []index.Key{k1})

				svc := NewService(db, idx, nil)
				return svc, db, idx
			},
			file: "base.lua",
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Has(index.Key{File: "base.lua", Func: "", Kind: "types"}) {
					t.Error("expected base.lua to be invalidated")
				}
				if db.Has(index.Key{File: "dep.lua", Func: "foo", Kind: "check"}) {
					t.Error("expected dependent dep.lua to be invalidated")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db, idx := tt.setup()
			svc.InvalidateFile(tt.file)
			tt.validate(t, db, idx)
		})
	}
}

func TestService_Clear(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() (*Service, *index.DB, *index.SymbolIndex)
		validate func(*testing.T, *index.DB, *index.SymbolIndex)
	}{
		{
			name: "clears cache and symbols",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				idx := index.NewSymbolIndex()

				db.Set(index.Key{File: "a.lua", Func: "foo", Kind: "check"}, "v1", nil)
				db.Set(index.Key{File: "b.lua", Func: "bar", Kind: "infer"}, "v2", nil)

				idx.AddDefinition(
					"a.lua",
					"var1",
					index.SymbolVariable,
					"string",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
					"",
				)
				idx.AddDefinition(
					"b.lua",
					"var2",
					index.SymbolFunction,
					"function",
					diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 5},
					"",
				)

				svc := NewService(db, idx, nil)
				return svc, db, idx
			},
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Size() != 0 {
					t.Errorf("expected cache size 0, got %d", db.Size())
				}
				if len(idx.SymbolsInFile("a.lua")) > 0 {
					t.Error("expected a.lua symbols to be cleared")
				}
				if len(idx.SymbolsInFile("b.lua")) > 0 {
					t.Error("expected b.lua symbols to be cleared")
				}
			},
		},
		{
			name: "handles nil cache",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				idx := index.NewSymbolIndex()
				idx.AddDefinition(
					"test.lua",
					"var",
					index.SymbolVariable,
					"any",
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4},
					"",
				)
				svc := NewService(nil, idx, nil)
				return svc, nil, idx
			},
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if len(idx.SymbolsInFile("test.lua")) > 0 {
					t.Error("expected symbols to be cleared")
				}
			},
		},
		{
			name: "handles nil symbols",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				db.Set(index.Key{File: "test.lua", Func: "foo", Kind: "check"}, "value", nil)
				svc := NewService(db, nil, nil)
				return svc, db, nil
			},
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Size() != 0 {
					t.Errorf("expected cache size 0, got %d", db.Size())
				}
			},
		},
		{
			name: "handles both nil",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				svc := NewService(nil, nil, nil)
				return svc, nil, nil
			},
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
			},
		},
		{
			name: "clear bumps cache version",
			setup: func() (*Service, *index.DB, *index.SymbolIndex) {
				db := index.NewDB()
				db.Set(index.Key{File: "test.lua", Func: "foo", Kind: "check"}, "value", nil)
				svc := NewService(db, nil, nil)
				return svc, db, nil
			},
			validate: func(t *testing.T, db *index.DB, idx *index.SymbolIndex) {
				if db.Version() == 0 {
					t.Error("expected version to be bumped after clear")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db, idx := tt.setup()
			svc.Clear()
			tt.validate(t, db, idx)
		})
	}
}

func TestReferenceKind(t *testing.T) {
	tests := []struct {
		name string
		kind ReferenceKind
		want int
	}{
		{"RefRead", RefRead, 0},
		{"RefWrite", RefWrite, 1},
		{"RefDefinition", RefDefinition, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.kind) != tt.want {
				t.Errorf("expected %s to be %d, got %d", tt.name, tt.want, int(tt.kind))
			}
		})
	}
}

func TestHoverResult_Fields(t *testing.T) {
	span := diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10}
	sym := &index.Symbol{
		Name:    "test",
		Kind:    index.SymbolVariable,
		Type:    "string",
		DefSpan: span,
		File:    "test.lua",
	}

	hr := &HoverResult{
		Type:      "string",
		Signature: "local test: string",
		Symbol:    sym,
		Span:      span,
	}

	if hr.Type != "string" {
		t.Errorf("expected Type 'string', got %v", hr.Type)
	}
	if hr.Signature != "local test: string" {
		t.Errorf("expected Signature 'local test: string', got %s", hr.Signature)
	}
	if hr.Symbol != sym {
		t.Error("expected Symbol to match")
	}
	if hr.Span != span {
		t.Error("expected Span to match")
	}
}

func TestDefinitionResult_Fields(t *testing.T) {
	span := diag.Span{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 8}
	dr := &DefinitionResult{
		File: "main.lua",
		Span: span,
	}

	if dr.File != "main.lua" {
		t.Errorf("expected File 'main.lua', got %s", dr.File)
	}
	if dr.Span != span {
		t.Error("expected Span to match")
	}
}

func TestReferenceResult_Fields(t *testing.T) {
	span := diag.Span{StartLine: 5, StartCol: 3, EndLine: 5, EndCol: 7}
	rr := &ReferenceResult{
		File: "lib.lua",
		Span: span,
		Kind: RefRead,
	}

	if rr.File != "lib.lua" {
		t.Errorf("expected File 'lib.lua', got %s", rr.File)
	}
	if rr.Span != span {
		t.Error("expected Span to match")
	}
	if rr.Kind != RefRead {
		t.Errorf("expected Kind RefRead, got %v", rr.Kind)
	}
}

func TestService_DocumentSymbols(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *Service
		file      string
		wantNil   bool
		wantCount int
		validate  func(*testing.T, []*DocumentSymbol)
	}{
		{
			name: "returns symbols for file",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("main.lua", "myFunc", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 3}, "")
				idx.AddDefinition("main.lua", "myVar", index.SymbolVariable, nil,
					diag.Span{StartLine: 7, StartCol: 1, EndLine: 7, EndCol: 5}, "")
				return NewService(nil, idx, nil)
			},
			file:      "main.lua",
			wantNil:   false,
			wantCount: 2,
		},
		{
			name: "groups children under scope",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("main.lua", "myFunc", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 3}, "")
				idx.AddDefinition("main.lua", "param1", index.SymbolParameter, nil,
					diag.Span{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 21}, "myFunc")
				idx.AddDefinition("main.lua", "localVar", index.SymbolVariable, nil,
					diag.Span{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 13}, "myFunc")
				return NewService(nil, idx, nil)
			},
			file:      "main.lua",
			wantNil:   false,
			wantCount: 1,
			validate: func(t *testing.T, syms []*DocumentSymbol) {
				if len(syms[0].Children) != 2 {
					t.Errorf("expected 2 children, got %d", len(syms[0].Children))
				}
			},
		},
		{
			name: "nil symbols index",
			setup: func() *Service {
				return NewService(nil, nil, nil)
			},
			file:    "main.lua",
			wantNil: true,
		},
		{
			name: "empty file",
			setup: func() *Service {
				return NewService(nil, index.NewSymbolIndex(), nil)
			},
			file:    "empty.lua",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup()
			result := svc.DocumentSymbols(tt.file)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != tt.wantCount {
				t.Errorf("expected %d symbols, got %d", tt.wantCount, len(result))
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestService_WorkspaceSymbols(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *Service
		query     string
		wantNil   bool
		wantCount int
	}{
		{
			name: "finds matching symbols",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("a.lua", "processData", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 12}, "")
				idx.AddDefinition("b.lua", "processItems", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 13}, "")
				idx.AddDefinition("c.lua", "helperFunc", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 11}, "")
				return NewService(nil, idx, nil)
			},
			query:     "process",
			wantNil:   false,
			wantCount: 2,
		},
		{
			name: "case insensitive search",
			setup: func() *Service {
				idx := index.NewSymbolIndex()
				idx.AddDefinition("test.lua", "MyFunction", index.SymbolFunction, nil,
					diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 11}, "")
				return NewService(nil, idx, nil)
			},
			query:     "myfunction",
			wantNil:   false,
			wantCount: 1,
		},
		{
			name: "empty query returns nil",
			setup: func() *Service {
				return NewService(nil, index.NewSymbolIndex(), nil)
			},
			query:   "",
			wantNil: true,
		},
		{
			name: "nil symbols index",
			setup: func() *Service {
				return NewService(nil, nil, nil)
			},
			query:   "test",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup()
			result := svc.WorkspaceSymbols(tt.query)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != tt.wantCount {
				t.Errorf("expected %d results, got %d", tt.wantCount, len(result))
			}
		})
	}
}

func TestManifestRegistry(t *testing.T) {
	registry := NewManifestRegistry()

	m := io.NewManifest("mymodule")
	m.DefineType("Person", typ.String)
	registry.Register(m)

	if registry.Lookup("mymodule") == nil {
		t.Error("expected to find registered manifest")
	}
	if registry.Lookup("unknown") != nil {
		t.Error("expected nil for unknown manifest")
	}
}

func TestManifestRegistry_SearchSymbols(t *testing.T) {
	registry := NewManifestRegistry()

	m := io.NewManifest("utils")
	m.DefineType("StringUtils", typ.String)
	m.DefineType("NumberUtils", typ.Number)
	registry.Register(m)

	results := registry.SearchSymbols("String")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'String', got %d", len(results))
	}
}

func TestService_WorkspaceSymbols_WithManifests(t *testing.T) {
	svc := NewService(nil, index.NewSymbolIndex(), nil)

	svc.Symbols().AddDefinition("main.lua", "localFunc", index.SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}, "")

	m := io.NewManifest("lib")
	m.DefineType("Config", typ.String)
	svc.RegisterManifest(m)

	results := svc.WorkspaceSymbols("Func")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'Func', got %d", len(results))
	}

	results = svc.WorkspaceSymbols("Config")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'Config', got %d", len(results))
	}
}

func TestManifestRegistry_All(t *testing.T) {
	registry := NewManifestRegistry()

	registry.Register(io.NewManifest("a"))
	registry.Register(io.NewManifest("b"))

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("expected 2 manifests, got %d", len(all))
	}
}

func TestManifestRegistry_Clear(t *testing.T) {
	registry := NewManifestRegistry()

	registry.Register(io.NewManifest("test"))
	registry.Clear()

	if registry.Lookup("test") != nil {
		t.Error("expected nil after clear")
	}
}

func TestManifestRegistry_Remove(t *testing.T) {
	registry := NewManifestRegistry()

	registry.Register(io.NewManifest("a"))
	registry.Register(io.NewManifest("b"))

	registry.Remove("a")

	if registry.Lookup("a") != nil {
		t.Error("expected a to be removed")
	}
	if registry.Lookup("b") == nil {
		t.Error("expected b to remain")
	}
}

func TestManifestRegistry_FindType(t *testing.T) {
	registry := NewManifestRegistry()

	m := io.NewManifest("models")
	m.DefineType("User", typ.String)
	m.DefineType("Config", typ.Number)
	registry.Register(m)

	manifest, foundType := registry.FindType("User")
	if manifest == nil || foundType == nil {
		t.Error("expected to find User type")
	}

	manifest, foundType = registry.FindType("models.Config")
	if manifest == nil || foundType == nil {
		t.Error("expected to find models.Config type")
	}

	manifest, foundType = registry.FindType("Unknown")
	if manifest != nil || foundType != nil {
		t.Error("expected nil for unknown type")
	}
}

func TestFormatType(t *testing.T) {
	tests := []struct {
		name string
		typ  interface{}
		want string
	}{
		{"nil input", nil, "unknown"},
		{"nil type", typ.Nil, "nil"},
		{"boolean", typ.Boolean, "boolean"},
		{"number", typ.Number, "number"},
		{"integer", typ.Integer, "integer"},
		{"string", typ.String, "string"},
		{"any", typ.Any, "any"},
		{"unknown", typ.Unknown, "unknown"},
		{"never", typ.Never, "never"},
		{"array", typ.NewArray(typ.String), "string[]"},
		{"map", typ.NewMap(typ.String, typ.Number), "{ [string]: number }"},
		{"tuple", typ.NewTuple(typ.String, typ.Number), "[string, number]"},
		{"optional", typ.NewOptional(typ.String), "string?"},
		{"union", typ.NewUnion(typ.String, typ.Number), "number | string"},
		{"type param", &typ.TypeParam{Name: "T"}, "T"},
		{"alias", &typ.Alias{Name: "MyType"}, "MyType"},
		{"interface", &typ.Interface{Name: "IReader"}, "IReader"},
		{"literal string", typ.LiteralString("hello"), `"hello"`},
		{"literal number", typ.LiteralNumber(42), "42"},
		{"literal bool", typ.LiteralBool(true), "true"},
		{"record with fields", typ.NewRecord().Field("x", typ.Number).Build(), "{ x: number }"},
		{"function no params", typ.Func().Returns(typ.String).Build(), "() -> string"},
		{"function with params", typ.Func().Param("arg1", typ.String).Param("arg2", typ.Number).Returns(typ.Boolean).Build(), "(arg1: string, arg2: number) -> boolean"},
		{"function variadic", typ.Func().Param("arg1", typ.String).Variadic(typ.Number).Build(), "(arg1: string, ...number) -> void"},
		{"function multi return", typ.Func().Returns(typ.String, typ.Number).Build(), "() -> (string, number)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatType(tt.typ)
			if got != tt.want {
				t.Errorf("FormatType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestService_CallersOf(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("lib.lua", "helper", index.SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 3}, "")
	idx.AddDefinition("main.lua", "main", index.SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 3}, "")

	svc := NewService(nil, idx, nil)
	callerSpan := diag.Span{StartLine: 1, StartCol: 1, EndLine: 10, EndCol: 3}
	calleeSpan := diag.Span{StartLine: 1, StartCol: 1, EndLine: 5, EndCol: 3}
	callSpan := diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20}
	svc.CallGraph().AddCall("main.lua", "main", callerSpan, "lib.lua", "helper", calleeSpan, callSpan)

	callers := svc.CallersOf("lib.lua", 1, 1)
	if len(callers) != 1 {
		t.Errorf("expected 1 caller, got %d", len(callers))
	}
	if len(callers) > 0 && callers[0].CallerName != "main" {
		t.Errorf("expected caller 'main', got %s", callers[0].CallerName)
	}

	idx.AddDefinition("test.lua", "var", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	if svc.CallersOf("test.lua", 1, 1) != nil {
		t.Error("expected nil for non-function")
	}
}

func TestService_CalleesOf(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("main.lua", "process", index.SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 20, EndCol: 3}, "")

	svc := NewService(nil, idx, nil)
	callerSpan := diag.Span{StartLine: 1, StartCol: 1, EndLine: 20, EndCol: 3}
	svc.CallGraph().AddCall("main.lua", "process", callerSpan, "main.lua", "format",
		diag.Span{StartLine: 1}, diag.Span{StartLine: 5, StartCol: 10})
	svc.CallGraph().AddCall("main.lua", "process", callerSpan, "main.lua", "validate",
		diag.Span{StartLine: 1}, diag.Span{StartLine: 10, StartCol: 5})

	callees := svc.CalleesOf("main.lua", 1, 1)
	if len(callees) != 2 {
		t.Errorf("expected 2 callees, got %d", len(callees))
	}

	svc2 := NewService(nil, nil, nil)
	if svc2.CalleesOf("test.lua", 1, 1) != nil {
		t.Error("expected nil with nil symbols")
	}
}

func TestManifestRegistry_NilManifest(t *testing.T) {
	registry := NewManifestRegistry()
	registry.Register(nil)

	if len(registry.All()) != 0 {
		t.Error("registering nil should not add anything")
	}
}

func TestService_Manifests(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc.Manifests() == nil {
		t.Error("expected non-nil manifests registry")
	}
}

func TestService_CallersOf_NilSymbolsOrCallGraph(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc.CallersOf("test.lua", 1, 1) != nil {
		t.Error("expected nil with nil symbols")
	}

	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "foo", index.SymbolFunction, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}, "")
	svc2 := &Service{symbols: idx, callGraph: nil}
	if svc2.CallersOf("test.lua", 1, 1) != nil {
		t.Error("expected nil with nil callGraph")
	}
}

func TestService_CallersOf_NonFunction(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "myVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 6}, "")
	svc := NewService(nil, idx, nil)
	if svc.CallersOf("test.lua", 1, 3) != nil {
		t.Error("expected nil for non-function symbol")
	}
}

func TestService_CalleesOf_NonFunction(t *testing.T) {
	idx := index.NewSymbolIndex()
	idx.AddDefinition("test.lua", "myVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 6}, "")
	svc := NewService(nil, idx, nil)
	if svc.CalleesOf("test.lua", 1, 3) != nil {
		t.Error("expected nil for non-function symbol")
	}
}

func TestFormatType_RecordEmpty(t *testing.T) {
	r := typ.NewRecord().Build()
	got := FormatType(r)
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestFormatType_RecordManyFields(t *testing.T) {
	r := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Field("z", typ.Number).
		Field("w", typ.Number).
		Build()
	got := FormatType(r)
	// Fields are sorted alphabetically: w, x, y, z
	if got != "{ w: number, ... }" {
		t.Errorf("expected '{ w: number, ... }', got %q", got)
	}
}
