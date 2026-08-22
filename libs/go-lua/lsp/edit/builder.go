package edit

import "github.com/wippyai/go-lua/types/diag"

// Builder constructs workspace edits incrementally.
type Builder struct {
	edits map[string][]TextEdit
}

// NewBuilder creates a new edit builder.
func NewBuilder() *Builder {
	return &Builder{
		edits: make(map[string][]TextEdit),
	}
}

// Replace replaces text at the given span.
func (b *Builder) Replace(file string, span diag.Span, newText string) *Builder {
	b.edits[file] = append(b.edits[file], TextEdit{
		Span:    span,
		NewText: newText,
	})
	return b
}

// Insert inserts text at the given position.
func (b *Builder) Insert(file string, line, col int, text string) *Builder {
	span := diag.Span{
		StartLine: line,
		StartCol:  col,
		EndLine:   line,
		EndCol:    col,
	}
	return b.Replace(file, span, text)
}

// InsertLine inserts a complete line before the given line number.
func (b *Builder) InsertLine(file string, beforeLine int, text string) *Builder {
	return b.Insert(file, beforeLine, 1, text+"\n")
}

// Delete removes text at the given span.
func (b *Builder) Delete(file string, span diag.Span) *Builder {
	return b.Replace(file, span, "")
}

// DeleteLine removes an entire line.
func (b *Builder) DeleteLine(file string, line int) *Builder {
	span := diag.Span{
		StartLine: line,
		StartCol:  1,
		EndLine:   line + 1,
		EndCol:    1,
	}
	return b.Delete(file, span)
}

// ReplaceAll replaces all occurrences in a file.
func (b *Builder) ReplaceAll(file string, edits []TextEdit) *Builder {
	b.edits[file] = append(b.edits[file], edits...)
	return b
}

// Build creates the final WorkspaceEdit.
func (b *Builder) Build() *WorkspaceEdit {
	if len(b.edits) == 0 {
		return &WorkspaceEdit{}
	}

	result := &WorkspaceEdit{
		Files: make([]FileEdit, 0, len(b.edits)),
	}

	for file, edits := range b.edits {
		if len(edits) > 0 {
			result.Files = append(result.Files, FileEdit{
				File:  file,
				Edits: edits,
			})
		}
	}

	return result
}

// BuildSorted creates a sorted WorkspaceEdit ready for application.
func (b *Builder) BuildSorted() *WorkspaceEdit {
	result := b.Build()
	result.Sort()
	return result
}

// Clear resets the builder for reuse.
func (b *Builder) Clear() {
	b.edits = make(map[string][]TextEdit)
}

// HasEdits returns true if any edits have been added.
func (b *Builder) HasEdits() bool {
	for _, edits := range b.edits {
		if len(edits) > 0 {
			return true
		}
	}
	return false
}

// FileCount returns the number of files with edits.
func (b *Builder) FileCount() int {
	count := 0
	for _, edits := range b.edits {
		if len(edits) > 0 {
			count++
		}
	}
	return count
}
