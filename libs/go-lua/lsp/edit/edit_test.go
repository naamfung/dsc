package edit

import (
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestTextEdit_IsInsert(t *testing.T) {
	tests := []struct {
		name string
		edit TextEdit
		want bool
	}{
		{
			name: "insert at position",
			edit: TextEdit{
				Span:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 5},
				NewText: "hello",
			},
			want: true,
		},
		{
			name: "replace",
			edit: TextEdit{
				Span:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10},
				NewText: "hello",
			},
			want: false,
		},
		{
			name: "multiline not insert",
			edit: TextEdit{
				Span:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 2, EndCol: 5},
				NewText: "hello",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.edit.IsInsert(); got != tt.want {
				t.Errorf("IsInsert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTextEdit_IsDelete(t *testing.T) {
	tests := []struct {
		name string
		edit TextEdit
		want bool
	}{
		{
			name: "delete",
			edit: TextEdit{
				Span:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10},
				NewText: "",
			},
			want: true,
		},
		{
			name: "replace",
			edit: TextEdit{
				Span:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10},
				NewText: "hello",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.edit.IsDelete(); got != tt.want {
				t.Errorf("IsDelete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkspaceEdit_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		edit *WorkspaceEdit
		want bool
	}{
		{
			name: "nil",
			edit: nil,
			want: true,
		},
		{
			name: "empty files",
			edit: &WorkspaceEdit{},
			want: true,
		},
		{
			name: "files but no edits",
			edit: &WorkspaceEdit{Files: []FileEdit{{File: "test.lua"}}},
			want: true,
		},
		{
			name: "has edits",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{
					File:  "test.lua",
					Edits: []TextEdit{{NewText: "x"}},
				}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.edit.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkspaceEdit_Counts(t *testing.T) {
	edit := &WorkspaceEdit{
		Files: []FileEdit{
			{File: "a.lua", Edits: []TextEdit{{}, {}}},
			{File: "b.lua", Edits: []TextEdit{{}}},
		},
	}

	if got := edit.FileCount(); got != 2 {
		t.Errorf("FileCount() = %v, want 2", got)
	}

	if got := edit.EditCount(); got != 3 {
		t.Errorf("EditCount() = %v, want 3", got)
	}

	// Nil case
	var nilEdit *WorkspaceEdit
	if got := nilEdit.FileCount(); got != 0 {
		t.Errorf("nil FileCount() = %v, want 0", got)
	}
	if got := nilEdit.EditCount(); got != 0 {
		t.Errorf("nil EditCount() = %v, want 0", got)
	}
}

func TestWorkspaceEdit_Validate(t *testing.T) {
	tests := []struct {
		name    string
		edit    *WorkspaceEdit
		wantErr bool
	}{
		{
			name:    "nil",
			edit:    nil,
			wantErr: false,
		},
		{
			name:    "empty",
			edit:    &WorkspaceEdit{},
			wantErr: false,
		},
		{
			name: "valid single edit",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{
					File: "test.lua",
					Edits: []TextEdit{{
						Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
					}},
				}},
			},
			wantErr: false,
		},
		{
			name: "empty file path",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{File: "", Edits: []TextEdit{{}}}},
			},
			wantErr: true,
		},
		{
			name: "overlapping edits",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{
					File: "test.lua",
					Edits: []TextEdit{
						{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}},
						{Span: diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 15}},
					},
				}},
			},
			wantErr: true,
		},
		{
			name: "non-overlapping edits",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{
					File: "test.lua",
					Edits: []TextEdit{
						{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}},
						{Span: diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 15}},
					},
				}},
			},
			wantErr: false,
		},
		{
			name: "adjacent edits ok",
			edit: &WorkspaceEdit{
				Files: []FileEdit{{
					File: "test.lua",
					Edits: []TextEdit{
						{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}},
						{Span: diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10}},
					},
				}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.edit.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkspaceEdit_Sort(t *testing.T) {
	edit := &WorkspaceEdit{
		Files: []FileEdit{{
			File: "test.lua",
			Edits: []TextEdit{
				{Span: diag.Span{StartLine: 1, StartCol: 1}},
				{Span: diag.Span{StartLine: 3, StartCol: 5}},
				{Span: diag.Span{StartLine: 2, StartCol: 10}},
				{Span: diag.Span{StartLine: 2, StartCol: 5}},
			},
		}},
	}

	edit.Sort()

	// Should be sorted in reverse order
	edits := edit.Files[0].Edits
	expected := []struct{ line, col int }{
		{3, 5},
		{2, 10},
		{2, 5},
		{1, 1},
	}

	for i, exp := range expected {
		if edits[i].Span.StartLine != exp.line || edits[i].Span.StartCol != exp.col {
			t.Errorf("edits[%d] = (%d,%d), want (%d,%d)",
				i, edits[i].Span.StartLine, edits[i].Span.StartCol, exp.line, exp.col)
		}
	}

	// Nil should not panic
	var nilEdit *WorkspaceEdit
	nilEdit.Sort()
}

func TestWorkspaceEdit_Merge(t *testing.T) {
	edit1 := &WorkspaceEdit{
		Files: []FileEdit{
			{File: "a.lua", Edits: []TextEdit{{NewText: "1"}}},
		},
	}

	edit2 := &WorkspaceEdit{
		Files: []FileEdit{
			{File: "a.lua", Edits: []TextEdit{{NewText: "2"}}},
			{File: "b.lua", Edits: []TextEdit{{NewText: "3"}}},
		},
	}

	edit1.Merge(edit2)

	if len(edit1.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(edit1.Files))
	}

	// a.lua should have 2 edits
	for _, f := range edit1.Files {
		if f.File == "a.lua" && len(f.Edits) != 2 {
			t.Errorf("a.lua should have 2 edits, got %d", len(f.Edits))
		}
	}

	// Merge nil should be safe
	edit1.Merge(nil)
}

func TestSpansOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b diag.Span
		want bool
	}{
		{
			name: "same span",
			a:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10},
			b:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10},
			want: true,
		},
		{
			name: "partial overlap",
			a:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10},
			b:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 15},
			want: true,
		},
		{
			name: "no overlap - before",
			a:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
			b:    diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 15},
			want: false,
		},
		{
			name: "no overlap - after",
			a:    diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 15},
			b:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
			want: false,
		},
		{
			name: "adjacent - no overlap",
			a:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5},
			b:    diag.Span{StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 10},
			want: false,
		},
		{
			name: "multiline overlap",
			a:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 3, EndCol: 10},
			b:    diag.Span{StartLine: 2, StartCol: 5, EndLine: 4, EndCol: 5},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spansOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("spansOverlap() = %v, want %v", got, tt.want)
			}
		})
	}
}
