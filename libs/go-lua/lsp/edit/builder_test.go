package edit

import (
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestBuilder_Replace(t *testing.T) {
	b := NewBuilder()
	b.Replace("test.lua", diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "hello")

	edit := b.Build()
	if len(edit.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(edit.Files))
	}
	if edit.Files[0].File != "test.lua" {
		t.Errorf("expected file test.lua, got %s", edit.Files[0].File)
	}
	if len(edit.Files[0].Edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edit.Files[0].Edits))
	}
	if edit.Files[0].Edits[0].NewText != "hello" {
		t.Errorf("expected 'hello', got %q", edit.Files[0].Edits[0].NewText)
	}
}

func TestBuilder_Insert(t *testing.T) {
	b := NewBuilder()
	b.Insert("test.lua", 5, 10, "inserted")

	edit := b.Build()
	e := edit.Files[0].Edits[0]

	if e.Span.StartLine != 5 || e.Span.StartCol != 10 {
		t.Errorf("wrong start position: (%d,%d)", e.Span.StartLine, e.Span.StartCol)
	}
	if e.Span.StartLine != e.Span.EndLine || e.Span.StartCol != e.Span.EndCol {
		t.Error("insert should have zero-width span")
	}
	if e.NewText != "inserted" {
		t.Errorf("expected 'inserted', got %q", e.NewText)
	}
}

func TestBuilder_InsertLine(t *testing.T) {
	b := NewBuilder()
	b.InsertLine("test.lua", 3, "new line content")

	edit := b.Build()
	e := edit.Files[0].Edits[0]

	if e.Span.StartLine != 3 {
		t.Errorf("expected line 3, got %d", e.Span.StartLine)
	}
	if e.NewText != "new line content\n" {
		t.Errorf("expected newline suffix, got %q", e.NewText)
	}
}

func TestBuilder_Delete(t *testing.T) {
	b := NewBuilder()
	span := diag.Span{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 15}
	b.Delete("test.lua", span)

	edit := b.Build()
	e := edit.Files[0].Edits[0]

	if e.NewText != "" {
		t.Errorf("delete should have empty NewText, got %q", e.NewText)
	}
	if e.Span != span {
		t.Errorf("span mismatch")
	}
}

func TestBuilder_DeleteLine(t *testing.T) {
	b := NewBuilder()
	b.DeleteLine("test.lua", 5)

	edit := b.Build()
	e := edit.Files[0].Edits[0]

	if e.Span.StartLine != 5 || e.Span.EndLine != 6 {
		t.Errorf("expected lines 5-6, got %d-%d", e.Span.StartLine, e.Span.EndLine)
	}
	if e.NewText != "" {
		t.Error("delete should have empty NewText")
	}
}

func TestBuilder_ReplaceAll(t *testing.T) {
	b := NewBuilder()
	edits := []TextEdit{
		{Span: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, NewText: "a"},
		{Span: diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 5}, NewText: "b"},
	}
	b.ReplaceAll("test.lua", edits)

	result := b.Build()
	if len(result.Files[0].Edits) != 2 {
		t.Errorf("expected 2 edits, got %d", len(result.Files[0].Edits))
	}
}

func TestBuilder_MultipleFiles(t *testing.T) {
	b := NewBuilder()
	b.Insert("a.lua", 1, 1, "a")
	b.Insert("b.lua", 1, 1, "b")
	b.Insert("a.lua", 2, 1, "aa")

	edit := b.Build()

	if len(edit.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(edit.Files))
	}

	aEdits := 0
	for _, f := range edit.Files {
		if f.File == "a.lua" {
			aEdits = len(f.Edits)
		}
	}
	if aEdits != 2 {
		t.Errorf("expected 2 edits in a.lua, got %d", aEdits)
	}
}

func TestBuilder_Chaining(t *testing.T) {
	edit := NewBuilder().
		Insert("test.lua", 1, 1, "a").
		Insert("test.lua", 2, 1, "b").
		Delete("test.lua", diag.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 5}).
		Build()

	if len(edit.Files[0].Edits) != 3 {
		t.Errorf("expected 3 edits, got %d", len(edit.Files[0].Edits))
	}
}

func TestBuilder_BuildSorted(t *testing.T) {
	b := NewBuilder()
	b.Insert("test.lua", 1, 1, "first")
	b.Insert("test.lua", 5, 1, "last")
	b.Insert("test.lua", 3, 1, "middle")

	edit := b.BuildSorted()
	edits := edit.Files[0].Edits

	// Should be reverse sorted (5, 3, 1)
	if edits[0].Span.StartLine != 5 {
		t.Errorf("first edit should be line 5, got %d", edits[0].Span.StartLine)
	}
	if edits[1].Span.StartLine != 3 {
		t.Errorf("second edit should be line 3, got %d", edits[1].Span.StartLine)
	}
	if edits[2].Span.StartLine != 1 {
		t.Errorf("third edit should be line 1, got %d", edits[2].Span.StartLine)
	}
}

func TestBuilder_Clear(t *testing.T) {
	b := NewBuilder()
	b.Insert("test.lua", 1, 1, "hello")

	if !b.HasEdits() {
		t.Error("should have edits")
	}

	b.Clear()

	if b.HasEdits() {
		t.Error("should not have edits after clear")
	}

	edit := b.Build()
	if !edit.IsEmpty() {
		t.Error("build after clear should be empty")
	}
}

func TestBuilder_HasEdits(t *testing.T) {
	b := NewBuilder()

	if b.HasEdits() {
		t.Error("new builder should not have edits")
	}

	b.Insert("test.lua", 1, 1, "x")

	if !b.HasEdits() {
		t.Error("should have edits after insert")
	}
}

func TestBuilder_FileCount(t *testing.T) {
	b := NewBuilder()

	if b.FileCount() != 0 {
		t.Error("new builder should have 0 files")
	}

	b.Insert("a.lua", 1, 1, "a")
	b.Insert("b.lua", 1, 1, "b")

	if b.FileCount() != 2 {
		t.Errorf("expected 2 files, got %d", b.FileCount())
	}
}

func TestBuilder_EmptyBuild(t *testing.T) {
	b := NewBuilder()
	edit := b.Build()

	if edit == nil {
		t.Error("build should not return nil")
	}
	if !edit.IsEmpty() {
		t.Error("empty build should be empty")
	}
}
