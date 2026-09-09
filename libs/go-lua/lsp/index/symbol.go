package index

import (
	"strings"
	"sync"

	"github.com/wippyai/go-lua/types/diag"
)

// SymbolKind identifies the category of a symbol.
type SymbolKind int

const (
	SymbolVariable SymbolKind = iota
	SymbolFunction
	SymbolMethod
	SymbolType
	SymbolField
	SymbolParameter
)

// Symbol represents a named entity with position information.
type Symbol struct {
	Name    string
	Kind    SymbolKind
	Type    any // The resolved type (core.Type)
	DefSpan diag.Span
	File    string
	Scope   string // Containing function name or "" for module-level

	// Escape analysis
	Escapes      bool   // Whether the symbol escapes its scope
	EscapeReason string // Why the symbol escapes (returned, captured, stored, etc.)
}

// Reference records a usage of a symbol.
type Reference struct {
	Symbol  *Symbol
	UseSpan diag.Span
	File    string
}

// filePos is a key for position-based lookup.
type filePos struct {
	file string
	line int
}

// SymbolIndex provides position-based symbol lookup for LSP features.
type SymbolIndex struct {
	mu         sync.RWMutex
	byFile     map[string][]*Symbol          // File -> symbols defined in file
	byName     map[string]map[string]*Symbol // File -> Name -> Symbol
	byLine     map[filePos][]*Symbol         // Quick lookup by file:line
	references map[*Symbol][]*Reference      // Definition -> usages
	refsByLine map[filePos][]*Reference      // Quick reference lookup by file:line
}

// NewSymbolIndex creates a new empty symbol index.
func NewSymbolIndex() *SymbolIndex {
	return &SymbolIndex{
		byFile:     make(map[string][]*Symbol),
		byName:     make(map[string]map[string]*Symbol),
		byLine:     make(map[filePos][]*Symbol),
		references: make(map[*Symbol][]*Reference),
		refsByLine: make(map[filePos][]*Reference),
	}
}

// AddDefinition records a symbol definition.
func (idx *SymbolIndex) AddDefinition(file, name string, kind SymbolKind, typ any, span diag.Span, scope string) *Symbol {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	sym := &Symbol{
		Name:    name,
		Kind:    kind,
		Type:    typ,
		DefSpan: span,
		File:    file,
		Scope:   scope,
	}

	idx.byFile[file] = append(idx.byFile[file], sym)

	if idx.byName[file] == nil {
		idx.byName[file] = make(map[string]*Symbol)
	}
	idx.byName[file][name] = sym

	pos := filePos{file: file, line: span.StartLine}
	idx.byLine[pos] = append(idx.byLine[pos], sym)

	return sym
}

// AddReference records a usage of a symbol.
func (idx *SymbolIndex) AddReference(file string, symbol *Symbol, useSpan diag.Span) {
	if symbol == nil {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	ref := &Reference{
		Symbol:  symbol,
		UseSpan: useSpan,
		File:    file,
	}
	idx.references[symbol] = append(idx.references[symbol], ref)

	pos := filePos{file: file, line: useSpan.StartLine}
	idx.refsByLine[pos] = append(idx.refsByLine[pos], ref)
}

// SymbolAt finds the symbol at or containing the given position.
func (idx *SymbolIndex) SymbolAt(file string, line, col int) *Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	pos := filePos{file: file, line: line}

	// Check definitions on this line
	for _, sym := range idx.byLine[pos] {
		if spanContains(sym.DefSpan, line, col) {
			return sym
		}
	}

	// Check references on this line using indexed lookup
	for _, ref := range idx.refsByLine[pos] {
		if spanContains(ref.UseSpan, line, col) {
			return ref.Symbol
		}
	}

	return nil
}

// DefinitionOf returns the definition symbol for a position.
// If position is on a reference, returns the definition.
// If position is on a definition, returns that symbol.
func (idx *SymbolIndex) DefinitionOf(file string, line, col int) *Symbol {
	return idx.SymbolAt(file, line, col)
}

// ReferencesTo returns all references to a symbol.
func (idx *SymbolIndex) ReferencesTo(symbol *Symbol) []*Reference {
	if symbol == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	refs := idx.references[symbol]
	result := make([]*Reference, len(refs))
	copy(result, refs)
	return result
}

// LookupByName finds a symbol by name in a file.
func (idx *SymbolIndex) LookupByName(file, name string) *Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if names, ok := idx.byName[file]; ok {
		return names[name]
	}
	return nil
}

// SymbolsInFile returns all symbols defined in a file.
func (idx *SymbolIndex) SymbolsInFile(file string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	syms := idx.byFile[file]
	result := make([]*Symbol, len(syms))
	copy(result, syms)
	return result
}

// InvalidateFile removes all symbols and references for a file.
func (idx *SymbolIndex) InvalidateFile(file string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove symbols defined in this file
	symbols := idx.byFile[file]
	for _, sym := range symbols {
		delete(idx.references, sym)
	}
	delete(idx.byFile, file)
	delete(idx.byName, file)

	// Remove line index entries for this file
	for pos := range idx.byLine {
		if pos.file == file {
			delete(idx.byLine, pos)
		}
	}

	// Remove refsByLine entries for this file
	for pos := range idx.refsByLine {
		if pos.file == file {
			delete(idx.refsByLine, pos)
		}
	}

	// Remove references from this file to other symbols
	for sym, refs := range idx.references {
		var kept []*Reference
		for _, ref := range refs {
			if ref.File != file {
				kept = append(kept, ref)
			}
		}
		if len(kept) > 0 {
			idx.references[sym] = kept
		} else {
			delete(idx.references, sym)
		}
	}
}

// Clear removes all symbols and references.
func (idx *SymbolIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.byFile = make(map[string][]*Symbol)
	idx.byName = make(map[string]map[string]*Symbol)
	idx.byLine = make(map[filePos][]*Symbol)
	idx.references = make(map[*Symbol][]*Reference)
	idx.refsByLine = make(map[filePos][]*Reference)
}

// WorkspaceSymbol is a search result for workspace symbol queries.
type WorkspaceSymbol struct {
	Name string
	Kind SymbolKind
	File string
	Span diag.Span
}

// Search finds symbols matching a query string across all files.
func (idx *SymbolIndex) Search(query string) []*WorkspaceSymbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*WorkspaceSymbol
	queryLower := strings.ToLower(query)

	for file, names := range idx.byName {
		for name, sym := range names {
			if strings.Contains(strings.ToLower(name), queryLower) {
				results = append(results, &WorkspaceSymbol{
					Name: sym.Name,
					Kind: sym.Kind,
					File: file,
					Span: sym.DefSpan,
				})
			}
		}
	}

	return results
}

// MarkEscape marks a symbol as escaping with a reason.
func (idx *SymbolIndex) MarkEscape(file, name, reason string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if names, ok := idx.byName[file]; ok {
		if sym, ok := names[name]; ok {
			sym.Escapes = true
			sym.EscapeReason = reason
		}
	}
}

// spanContains checks if a span contains a position.
func spanContains(s diag.Span, line, col int) bool {
	if !s.Valid() {
		return false
	}

	if line < s.StartLine || line > s.EndLine {
		return false
	}

	if line == s.StartLine && col < s.StartCol {
		return false
	}

	// EndCol == 0 is a sentinel meaning "match to end of line"
	if line == s.EndLine && s.EndCol > 0 && col > s.EndCol {
		return false
	}

	return true
}
