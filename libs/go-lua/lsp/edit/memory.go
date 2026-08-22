package edit

import (
	"errors"
	"strings"
)

// MemoryApplier applies edits to in-memory content.
// Used for testing and preview.
type MemoryApplier struct {
	files map[string]string
}

// NewMemoryApplier creates an applier with the given file contents.
func NewMemoryApplier(files map[string]string) *MemoryApplier {
	copied := make(map[string]string, len(files))
	for k, v := range files {
		copied[k] = v
	}
	return &MemoryApplier{files: copied}
}

// Apply applies the workspace edit to the in-memory files.
func (a *MemoryApplier) Apply(edit *WorkspaceEdit) error {
	if edit == nil {
		return nil
	}

	if err := edit.Validate(); err != nil {
		return err
	}

	// Sort for safe application
	edit.Sort()

	for _, fileEdit := range edit.Files {
		content, ok := a.files[fileEdit.File]
		if !ok {
			return errors.New("edit: file not found: " + fileEdit.File)
		}

		newContent, err := applyEditsToContent(content, fileEdit.Edits)
		if err != nil {
			return err
		}

		a.files[fileEdit.File] = newContent
	}

	return nil
}

// Preview returns what files would look like after applying edits.
// Does not modify the actual stored content or the input edit.
func (a *MemoryApplier) Preview(edit *WorkspaceEdit) (map[string]string, error) {
	if edit == nil {
		return a.files, nil
	}

	if err := edit.Validate(); err != nil {
		return nil, err
	}

	// Make a copy to avoid mutating the input
	editCopy := &WorkspaceEdit{
		Files: make([]FileEdit, len(edit.Files)),
	}
	for i, f := range edit.Files {
		editCopy.Files[i] = FileEdit{
			File:  f.File,
			Edits: make([]TextEdit, len(f.Edits)),
		}
		copy(editCopy.Files[i].Edits, f.Edits)
	}
	editCopy.Sort()

	result := make(map[string]string, len(a.files))
	for k, v := range a.files {
		result[k] = v
	}

	for _, fileEdit := range editCopy.Files {
		content, ok := result[fileEdit.File]
		if !ok {
			return nil, errors.New("edit: file not found: " + fileEdit.File)
		}

		newContent, err := applyEditsToContent(content, fileEdit.Edits)
		if err != nil {
			return nil, err
		}

		result[fileEdit.File] = newContent
	}

	return result, nil
}

// Content returns the current content of a file.
func (a *MemoryApplier) Content(file string) string {
	return a.files[file]
}

// SetContent sets the content of a file.
func (a *MemoryApplier) SetContent(file, content string) {
	a.files[file] = content
}

// Files returns all file names.
func (a *MemoryApplier) Files() []string {
	result := make([]string, 0, len(a.files))
	for k := range a.files {
		result = append(result, k)
	}
	return result
}

// applyEditsToContent applies sorted edits to content.
// Edits must be sorted in reverse order (bottom-up, right-to-left).
func applyEditsToContent(content string, edits []TextEdit) (string, error) {
	lines := strings.Split(content, "\n")

	for _, edit := range edits {
		var err error
		lines, err = applyEdit(lines, edit)
		if err != nil {
			return "", err
		}
	}

	return strings.Join(lines, "\n"), nil
}

// applyEdit applies a single edit to lines.
func applyEdit(lines []string, edit TextEdit) ([]string, error) {
	startLine := edit.Span.StartLine - 1 // 0-indexed
	endLine := edit.Span.EndLine - 1
	startCol := edit.Span.StartCol - 1 // 0-indexed
	endCol := edit.Span.EndCol - 1

	if startLine < 0 || startLine >= len(lines) {
		return nil, errors.New("edit: start line out of range")
	}
	if endLine < 0 || endLine >= len(lines) {
		return nil, errors.New("edit: end line out of range")
	}

	// Handle single-line edit
	if startLine == endLine {
		line := lines[startLine]
		if startCol < 0 {
			startCol = 0
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		if startCol > len(line) {
			startCol = len(line)
		}

		newLine := line[:startCol] + edit.NewText + line[endCol:]
		newLines := strings.Split(newLine, "\n")

		result := make([]string, 0, len(lines)-1+len(newLines))
		result = append(result, lines[:startLine]...)
		result = append(result, newLines...)
		result = append(result, lines[startLine+1:]...)
		return result, nil
	}

	// Multi-line edit
	firstLine := lines[startLine]
	lastLine := lines[endLine]

	if startCol > len(firstLine) {
		startCol = len(firstLine)
	}
	if endCol > len(lastLine) {
		endCol = len(lastLine)
	}

	prefix := firstLine[:startCol]
	suffix := lastLine[endCol:]
	combined := prefix + edit.NewText + suffix
	newLines := strings.Split(combined, "\n")

	result := make([]string, 0, len(lines)-(endLine-startLine)+len(newLines)-1)
	result = append(result, lines[:startLine]...)
	result = append(result, newLines...)
	result = append(result, lines[endLine+1:]...)
	return result, nil
}

// LSPApplier wraps a callback to send workspace/applyEdit requests.
type LSPApplier struct {
	ApplyFunc func(*WorkspaceEdit) error
}

// Apply sends the edit via the callback.
func (a *LSPApplier) Apply(edit *WorkspaceEdit) error {
	if a.ApplyFunc == nil {
		return errors.New("edit: no apply function configured")
	}
	return a.ApplyFunc(edit)
}

// Preview is not supported for LSP applier.
func (a *LSPApplier) Preview(edit *WorkspaceEdit) (map[string]string, error) {
	return nil, errors.New("edit: preview not supported for LSP applier")
}
