// Package refactor provides code refactoring operations for LSP.
package refactor

import (
	"errors"
	"unicode"

	"github.com/wippyai/go-lua/lsp"
	"github.com/wippyai/go-lua/lsp/edit"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

// Common errors.
var (
	ErrNoSymbol      = errors.New("refactor: no symbol at position")
	ErrNotRenameable = errors.New("refactor: symbol cannot be renamed")
	ErrInvalidName   = errors.New("refactor: invalid identifier name")
	ErrNameConflict  = errors.New("refactor: name conflicts with existing symbol")
)

// PrepareResult contains info about a symbol that can be renamed.
type PrepareResult struct {
	Span        diag.Span
	Placeholder string // current name
	Kind        index.SymbolKind
}

// RenameProvider handles rename operations.
type RenameProvider struct {
	symbols *index.SymbolIndex
}

// NewRenameProvider creates a rename provider.
func NewRenameProvider(symbols *index.SymbolIndex) *RenameProvider {
	return &RenameProvider{symbols: symbols}
}

// PrepareRename checks if rename is valid at position.
// Returns the symbol's current name and span, or error if not renameable.
func (p *RenameProvider) PrepareRename(file string, line, col int) (*PrepareResult, error) {
	if p.symbols == nil {
		return nil, ErrNoSymbol
	}

	sym := p.symbols.SymbolAt(file, line, col)
	if sym == nil {
		return nil, ErrNoSymbol
	}

	if !isRenameable(sym) {
		return nil, ErrNotRenameable
	}

	return &PrepareResult{
		Span:        sym.DefSpan,
		Placeholder: sym.Name,
		Kind:        sym.Kind,
	}, nil
}

// Rename creates a workspace edit to rename a symbol.
func (p *RenameProvider) Rename(file string, line, col int, newName string) (*edit.WorkspaceEdit, error) {
	if p.symbols == nil {
		return nil, ErrNoSymbol
	}

	if err := ValidateName(newName); err != nil {
		return nil, err
	}

	// Find the symbol at position
	sym := p.symbols.DefinitionOf(file, line, col)
	if sym == nil {
		return nil, ErrNoSymbol
	}

	if !isRenameable(sym) {
		return nil, ErrNotRenameable
	}

	// Check for conflicts
	if err := p.checkConflicts(sym, newName); err != nil {
		return nil, err
	}

	// Build edits for definition and all references
	builder := edit.NewBuilder()

	// Rename definition
	builder.Replace(sym.File, sym.DefSpan, newName)

	// Rename all references
	refs := p.symbols.ReferencesTo(sym)
	for _, ref := range refs {
		builder.Replace(ref.File, ref.UseSpan, newName)
	}

	return builder.Build(), nil
}

// isRenameable checks if a symbol can be renamed.
func isRenameable(sym *index.Symbol) bool {
	if sym == nil {
		return false
	}
	return !lsp.IsLuaBuiltin(sym.Name)
}

// checkConflicts checks if the new name would conflict with existing symbols.
func (p *RenameProvider) checkConflicts(sym *index.Symbol, newName string) error {
	// Check for symbols with same name in same file/scope
	existing := p.symbols.LookupByName(sym.File, newName)
	if existing != nil && existing != sym {
		// Same scope conflict
		if existing.Scope == sym.Scope {
			return ErrNameConflict
		}
	}

	return nil
}

// ValidateName checks if a name is a valid Lua identifier.
func ValidateName(name string) error {
	if name == "" {
		return ErrInvalidName
	}

	if !isValidLuaIdentifier(name) {
		return ErrInvalidName
	}

	if lsp.IsLuaKeyword(name) {
		return ErrInvalidName
	}

	return nil
}

// isValidLuaIdentifier checks if name is a valid Lua identifier.
func isValidLuaIdentifier(name string) bool {
	if len(name) == 0 {
		return false
	}

	// First character must be letter or underscore
	first := rune(name[0])
	if !unicode.IsLetter(first) && first != '_' {
		return false
	}

	// Rest must be letters, digits, or underscores
	for _, r := range name[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}

	return true
}
