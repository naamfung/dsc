// Package edit provides text editing abstractions for LSP refactoring operations.
package edit

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/types/diag"
)

// TextEdit represents a single text modification.
type TextEdit struct {
	Span    diag.Span
	NewText string
}

// IsInsert returns true if this edit is a pure insertion (empty span).
func (e TextEdit) IsInsert() bool {
	return e.Span.StartLine == e.Span.EndLine &&
		e.Span.StartCol == e.Span.EndCol
}

// IsDelete returns true if this edit is a pure deletion (empty new text).
func (e TextEdit) IsDelete() bool {
	return e.NewText == ""
}

// FileEdit represents all edits to a single file.
type FileEdit struct {
	File  string
	Edits []TextEdit
}

// WorkspaceEdit represents edits across multiple files.
type WorkspaceEdit struct {
	Files []FileEdit
}

// IsEmpty returns true if no edits are present.
func (w *WorkspaceEdit) IsEmpty() bool {
	if w == nil {
		return true
	}
	for _, f := range w.Files {
		if len(f.Edits) > 0 {
			return false
		}
	}
	return true
}

// FileCount returns the number of files affected.
func (w *WorkspaceEdit) FileCount() int {
	if w == nil {
		return 0
	}
	return len(w.Files)
}

// EditCount returns the total number of edits.
func (w *WorkspaceEdit) EditCount() int {
	if w == nil {
		return 0
	}
	count := 0
	for _, f := range w.Files {
		count += len(f.Edits)
	}
	return count
}

// Validate checks edit consistency.
// Returns error if edits overlap or are invalid.
func (w *WorkspaceEdit) Validate() error {
	if w == nil {
		return nil
	}

	for _, file := range w.Files {
		if file.File == "" {
			return errors.New("edit: empty file path")
		}

		// Check for overlapping spans within same file
		for i := 0; i < len(file.Edits); i++ {
			for j := i + 1; j < len(file.Edits); j++ {
				if spansOverlap(file.Edits[i].Span, file.Edits[j].Span) {
					return errors.New("edit: overlapping edits in " + file.File)
				}
			}
		}
	}

	return nil
}

// spansOverlap returns true if two spans overlap.
func spansOverlap(a, b diag.Span) bool {
	// Check if a ends before b starts
	if a.EndLine < b.StartLine || (a.EndLine == b.StartLine && a.EndCol <= b.StartCol) {
		return false
	}
	// Check if b ends before a starts
	if b.EndLine < a.StartLine || (b.EndLine == a.StartLine && b.EndCol <= a.StartCol) {
		return false
	}
	return true
}

// Sort orders edits for safe application (bottom-up, right-to-left).
// This ensures earlier edits don't affect positions of later ones.
func (w *WorkspaceEdit) Sort() {
	if w == nil {
		return
	}

	for i := range w.Files {
		edits := w.Files[i].Edits
		sort.Slice(edits, func(a, b int) bool {
			ea, eb := edits[a], edits[b]
			// Sort by line descending, then column descending
			if ea.Span.StartLine != eb.Span.StartLine {
				return ea.Span.StartLine > eb.Span.StartLine
			}
			return ea.Span.StartCol > eb.Span.StartCol
		})
	}
}

// Merge combines another workspace edit into this one.
func (w *WorkspaceEdit) Merge(other *WorkspaceEdit) {
	if other == nil {
		return
	}

	fileMap := make(map[string]int)
	for i, f := range w.Files {
		fileMap[f.File] = i
	}

	for _, of := range other.Files {
		if idx, ok := fileMap[of.File]; ok {
			w.Files[idx].Edits = append(w.Files[idx].Edits, of.Edits...)
		} else {
			w.Files = append(w.Files, of)
			fileMap[of.File] = len(w.Files) - 1
		}
	}
}

// EditApplier applies workspace edits to actual content.
type EditApplier interface {
	// Apply applies the workspace edit.
	Apply(edit *WorkspaceEdit) error

	// Preview returns the result without applying.
	Preview(edit *WorkspaceEdit) (map[string]string, error)
}
