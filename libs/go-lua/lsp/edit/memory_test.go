package edit

import (
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestMemoryApplier_Apply_SingleLineReplace(t *testing.T) {
	files := map[string]string{
		"test.lua": "hello world",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Replace("test.lua", diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 6}, "goodbye").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "goodbye world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_Insert(t *testing.T) {
	files := map[string]string{
		"test.lua": "hello world",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Insert("test.lua", 1, 6, " beautiful").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "hello beautiful world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_Delete(t *testing.T) {
	files := map[string]string{
		"test.lua": "hello beautiful world",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Delete("test.lua", diag.Span{StartLine: 1, StartCol: 6, EndLine: 1, EndCol: 16}).
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_MultipleEdits(t *testing.T) {
	files := map[string]string{
		"test.lua": "local x = 1\nlocal y = 2\nlocal z = 3",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Replace("test.lua", diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, "a").
		Replace("test.lua", diag.Span{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 8}, "b").
		Replace("test.lua", diag.Span{StartLine: 3, StartCol: 7, EndLine: 3, EndCol: 8}, "c").
		BuildSorted()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "local a = 1\nlocal b = 2\nlocal c = 3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_MultilineReplace(t *testing.T) {
	files := map[string]string{
		"test.lua": "line1\nline2\nline3\nline4",
	}
	applier := NewMemoryApplier(files)

	// Replace lines 2-3 with a single line
	edit := NewBuilder().
		Replace("test.lua", diag.Span{
			StartLine: 2, StartCol: 1,
			EndLine: 3, EndCol: 6,
		}, "replaced").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "line1\nreplaced\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_InsertNewLines(t *testing.T) {
	files := map[string]string{
		"test.lua": "line1\nline2",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Insert("test.lua", 2, 1, "inserted\n").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "line1\ninserted\nline2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMemoryApplier_Apply_Nil(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{"test.lua": "content"})

	err := applier.Apply(nil)
	if err != nil {
		t.Errorf("Apply(nil) should not error: %v", err)
	}
}

func TestMemoryApplier_Apply_FileNotFound(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{})

	edit := NewBuilder().
		Insert("missing.lua", 1, 1, "text").
		Build()

	err := applier.Apply(edit)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestMemoryApplier_Apply_InvalidSpan(t *testing.T) {
	files := map[string]string{
		"test.lua": "short",
	}
	applier := NewMemoryApplier(files)

	// Line out of range
	edit := NewBuilder().
		Insert("test.lua", 100, 1, "text").
		Build()

	err := applier.Apply(edit)
	if err == nil {
		t.Error("expected error for invalid line")
	}
}

func TestMemoryApplier_Apply_ValidationError(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{"test.lua": "content"})

	// Create overlapping edits
	edit := &WorkspaceEdit{
		Files: []FileEdit{{
			File: "test.lua",
			Edits: []TextEdit{
				{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}},
				{Span: diag.Span{StartLine: 1, StartCol: 3, EndLine: 1, EndCol: 7}},
			},
		}},
	}

	err := applier.Apply(edit)
	if err == nil {
		t.Error("expected validation error for overlapping edits")
	}
}

func TestMemoryApplier_Preview(t *testing.T) {
	files := map[string]string{
		"test.lua": "original",
	}
	applier := NewMemoryApplier(files)

	edit := NewBuilder().
		Replace("test.lua", diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 9}, "modified").
		Build()

	preview, err := applier.Preview(edit)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}

	if preview["test.lua"] != "modified" {
		t.Errorf("preview got %q, want 'modified'", preview["test.lua"])
	}

	// Original should be unchanged
	if applier.Content("test.lua") != "original" {
		t.Error("original content should not change after preview")
	}
}

func TestMemoryApplier_Preview_Nil(t *testing.T) {
	files := map[string]string{"test.lua": "content"}
	applier := NewMemoryApplier(files)

	result, err := applier.Preview(nil)
	if err != nil {
		t.Fatalf("Preview(nil) error: %v", err)
	}
	if result["test.lua"] != "content" {
		t.Error("preview of nil should return original content")
	}
}

func TestMemoryApplier_SetContent(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{})

	applier.SetContent("new.lua", "new content")

	if applier.Content("new.lua") != "new content" {
		t.Error("SetContent did not work")
	}
}

func TestMemoryApplier_Files(t *testing.T) {
	files := map[string]string{
		"a.lua": "a",
		"b.lua": "b",
	}
	applier := NewMemoryApplier(files)

	names := applier.Files()
	if len(names) != 2 {
		t.Errorf("expected 2 files, got %d", len(names))
	}
}

func TestMemoryApplier_IsolatesInput(t *testing.T) {
	files := map[string]string{
		"test.lua": "original",
	}
	applier := NewMemoryApplier(files)

	// Modify original map
	files["test.lua"] = "modified externally"

	// Applier should still have original
	if applier.Content("test.lua") != "original" {
		t.Error("applier should isolate from original map")
	}
}

func TestLSPApplier_Apply(t *testing.T) {
	var received *WorkspaceEdit

	applier := &LSPApplier{
		ApplyFunc: func(edit *WorkspaceEdit) error {
			received = edit
			return nil
		},
	}

	edit := NewBuilder().Insert("test.lua", 1, 1, "x").Build()
	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if received == nil {
		t.Error("ApplyFunc should have been called")
	}
}

func TestLSPApplier_Apply_NoFunc(t *testing.T) {
	applier := &LSPApplier{}

	edit := NewBuilder().Insert("test.lua", 1, 1, "x").Build()
	err := applier.Apply(edit)
	if err == nil {
		t.Error("expected error when ApplyFunc is nil")
	}
}

func TestLSPApplier_Preview(t *testing.T) {
	applier := &LSPApplier{}

	_, err := applier.Preview(nil)
	if err == nil {
		t.Error("Preview should error for LSP applier")
	}
}

func TestApplyEdit_ColBeyondLineLength(t *testing.T) {
	files := map[string]string{
		"test.lua": "short",
	}
	applier := NewMemoryApplier(files)

	// Try to replace beyond line end - should handle gracefully
	edit := NewBuilder().
		Replace("test.lua", diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 100}, "replaced").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	if got != "replaced" {
		t.Errorf("got %q, want 'replaced'", got)
	}
}

func TestApplyEdit_NegativeCol(t *testing.T) {
	// This tests the boundary handling in applyEdit
	lines := []string{"hello"}
	edit := TextEdit{
		Span:    diag.Span{StartLine: 1, StartCol: 0, EndLine: 1, EndCol: 3},
		NewText: "x",
	}

	result, err := applyEdit(lines, edit)
	if err != nil {
		t.Fatalf("applyEdit error: %v", err)
	}

	if result[0] != "xllo" {
		t.Errorf("got %q, want 'xllo'", result[0])
	}
}

func TestMemoryApplier_Preview_ValidationError(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{"test.lua": "content"})

	// Create overlapping edits
	edit := &WorkspaceEdit{
		Files: []FileEdit{{
			File: "test.lua",
			Edits: []TextEdit{
				{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}},
				{Span: diag.Span{StartLine: 1, StartCol: 3, EndLine: 1, EndCol: 7}},
			},
		}},
	}

	_, err := applier.Preview(edit)
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestMemoryApplier_Preview_FileNotFound(t *testing.T) {
	applier := NewMemoryApplier(map[string]string{})

	edit := NewBuilder().Insert("missing.lua", 1, 1, "text").Build()

	_, err := applier.Preview(edit)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestApplyEdit_MultilineColBeyondEnd(t *testing.T) {
	files := map[string]string{
		"test.lua": "first line\nsecond line\nthird line",
	}
	applier := NewMemoryApplier(files)

	// Multi-line replace with cols beyond line ends
	edit := NewBuilder().
		Replace("test.lua", diag.Span{
			StartLine: 1, StartCol: 50,
			EndLine: 2, EndCol: 50,
		}, "X").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "first lineX\nthird line"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyEdit_SingleLineInsertNewlines(t *testing.T) {
	files := map[string]string{
		"test.lua": "hello",
	}
	applier := NewMemoryApplier(files)

	// Insert multiple lines at a position
	edit := NewBuilder().
		Insert("test.lua", 1, 3, "\ninserted\n").
		Build()

	err := applier.Apply(edit)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got := applier.Content("test.lua")
	want := "he\ninserted\nllo"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyEdit_EndLineOutOfRange(t *testing.T) {
	lines := []string{"line1", "line2"}
	edit := TextEdit{
		Span:    diag.Span{StartLine: 1, StartCol: 1, EndLine: 100, EndCol: 1},
		NewText: "x",
	}

	_, err := applyEdit(lines, edit)
	if err == nil {
		t.Error("expected error for end line out of range")
	}
}

func TestApplyEdit_StartLineOutOfRange(t *testing.T) {
	lines := []string{"line1"}
	edit := TextEdit{
		Span:    diag.Span{StartLine: 100, StartCol: 1, EndLine: 100, EndCol: 1},
		NewText: "x",
	}

	_, err := applyEdit(lines, edit)
	if err == nil {
		t.Error("expected error for start line out of range")
	}
}

func TestApplyEdit_SingleLineStartColBeyondEnd(t *testing.T) {
	lines := []string{"hi"}
	edit := TextEdit{
		Span:    diag.Span{StartLine: 1, StartCol: 100, EndLine: 1, EndCol: 100},
		NewText: "!",
	}

	result, err := applyEdit(lines, edit)
	if err != nil {
		t.Fatalf("applyEdit error: %v", err)
	}

	// Insert at end of line
	if result[0] != "hi!" {
		t.Errorf("got %q, want 'hi!'", result[0])
	}
}

func TestMemoryApplier_Preview_ApplyError(t *testing.T) {
	files := map[string]string{
		"test.lua": "line1",
	}
	applier := NewMemoryApplier(files)

	// Create edit that will fail during apply (line out of range)
	edit := &WorkspaceEdit{
		Files: []FileEdit{{
			File: "test.lua",
			Edits: []TextEdit{
				{Span: diag.Span{StartLine: 100, StartCol: 1, EndLine: 100, EndCol: 1}, NewText: "x"},
			},
		}},
	}

	_, err := applier.Preview(edit)
	if err == nil {
		t.Error("expected error from applyEditsToContent")
	}
}
