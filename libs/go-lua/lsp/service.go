// Package lsp provides LSP query operations for the type system.
package lsp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// HoverResult contains information for hover display.
type HoverResult struct {
	Type      any // The type at the position (core.Type)
	Signature string
	Symbol    *index.Symbol
	Span      diag.Span
}

// DefinitionResult contains go-to-definition target.
type DefinitionResult struct {
	File string
	Span diag.Span
}

// ReferenceKind indicates the type of reference.
type ReferenceKind int

const (
	RefRead ReferenceKind = iota
	RefWrite
	RefDefinition
)

// ReferenceResult contains find-references results.
type ReferenceResult struct {
	File string
	Span diag.Span
	Kind ReferenceKind
}

// Service provides LSP query operations.
type Service struct {
	cache     *index.DB
	symbols   *index.SymbolIndex
	callGraph *index.CallGraph
	manifests *ManifestRegistry
}

// NewService creates a new LSP service with the given indexes.
func NewService(cache *index.DB, symbols *index.SymbolIndex, callGraph *index.CallGraph) *Service {
	if callGraph == nil {
		callGraph = index.NewCallGraph()
	}
	return &Service{
		cache:     cache,
		symbols:   symbols,
		callGraph: callGraph,
		manifests: NewManifestRegistry(),
	}
}

// ManifestRegistry stores type manifests for cross-module resolution.
type ManifestRegistry struct {
	mu     sync.RWMutex
	byPath map[string]*io.Manifest
}

// NewManifestRegistry creates an empty manifest registry.
func NewManifestRegistry() *ManifestRegistry {
	return &ManifestRegistry{
		byPath: make(map[string]*io.Manifest),
	}
}

// Register adds a manifest to the registry.
func (r *ManifestRegistry) Register(manifest *io.Manifest) {
	if manifest == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPath[manifest.Path] = manifest
}

// Lookup retrieves a manifest by module path.
func (r *ManifestRegistry) Lookup(path string) *io.Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byPath[path]
}

// All returns all registered manifests.
func (r *ManifestRegistry) All() []*io.Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*io.Manifest, 0, len(r.byPath))
	for _, m := range r.byPath {
		result = append(result, m)
	}
	return result
}

// Clear removes all manifests.
func (r *ManifestRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPath = make(map[string]*io.Manifest)
}

// Remove removes a manifest by path.
func (r *ManifestRegistry) Remove(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byPath, path)
}

// FindType searches for a type by name across all manifests.
// Supports both "TypeName" and "module.TypeName" formats.
func (r *ManifestRegistry) FindType(name string) (*io.Manifest, typ.Type) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for module.TypeName format
	if idx := strings.LastIndex(name, "."); idx > 0 {
		modulePath := name[:idx]
		typeName := name[idx+1:]
		if manifest, ok := r.byPath[modulePath]; ok {
			if t, found := manifest.Types[typeName]; found {
				return manifest, t
			}
		}
	}

	// Search all manifests for the type name
	for _, manifest := range r.byPath {
		if t, found := manifest.Types[name]; found {
			return manifest, t
		}
	}
	return nil, nil
}

// SearchSymbols searches for symbols across all manifests.
func (r *ManifestRegistry) SearchSymbols(query string) []*index.WorkspaceSymbol {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*index.WorkspaceSymbol
	queryLower := strings.ToLower(query)

	for _, manifest := range r.byPath {
		// Search types
		for name := range manifest.Types {
			if strings.Contains(strings.ToLower(name), queryLower) {
				results = append(results, &index.WorkspaceSymbol{
					Name: manifest.Path + "." + name,
					Kind: index.SymbolType,
					File: manifest.Path,
				})
			}
		}
	}

	return results
}

// Manifests returns the manifest registry.
func (s *Service) Manifests() *ManifestRegistry {
	return s.manifests
}

// RegisterManifest adds a manifest to the service.
func (s *Service) RegisterManifest(manifest *io.Manifest) {
	s.manifests.Register(manifest)
}

// CallGraph returns the call graph.
func (s *Service) CallGraph() *index.CallGraph {
	return s.callGraph
}

// Cache returns the cache database.
func (s *Service) Cache() *index.DB {
	return s.cache
}

// Symbols returns the symbol index.
func (s *Service) Symbols() *index.SymbolIndex {
	return s.symbols
}

// HoverAt returns type information at the given position.
func (s *Service) HoverAt(file string, line, col int) *HoverResult {
	if s.symbols == nil {
		return nil
	}

	sym := s.symbols.SymbolAt(file, line, col)
	if sym == nil {
		return nil
	}

	return &HoverResult{
		Type:      sym.Type,
		Signature: FormatType(sym.Type),
		Symbol:    sym,
		Span:      sym.DefSpan,
	}
}

// DefinitionAt returns the definition location for symbol at position.
func (s *Service) DefinitionAt(file string, line, col int) *DefinitionResult {
	if s.symbols == nil {
		return nil
	}

	sym := s.symbols.DefinitionOf(file, line, col)
	if sym == nil {
		return nil
	}

	return &DefinitionResult{
		File: sym.File,
		Span: sym.DefSpan,
	}
}

// ReferencesAt returns all references to the symbol at position.
func (s *Service) ReferencesAt(file string, line, col int, includeDecl bool) []ReferenceResult {
	if s.symbols == nil {
		return nil
	}

	sym := s.symbols.DefinitionOf(file, line, col)
	if sym == nil {
		return nil
	}

	var results []ReferenceResult

	// Include definition if requested
	if includeDecl {
		results = append(results, ReferenceResult{
			File: sym.File,
			Span: sym.DefSpan,
			Kind: RefDefinition,
		})
	}

	// Add all references
	refs := s.symbols.ReferencesTo(sym)
	for _, ref := range refs {
		results = append(results, ReferenceResult{
			File: ref.File,
			Span: ref.UseSpan,
			Kind: RefRead,
		})
	}

	return results
}

// InvalidateFile invalidates cache, symbols, and call graph for a file.
func (s *Service) InvalidateFile(file string) {
	if s.cache != nil {
		s.cache.InvalidateFileWithDependents(file)
	}
	if s.symbols != nil {
		s.symbols.InvalidateFile(file)
	}
	if s.callGraph != nil {
		s.callGraph.InvalidateFile(file)
	}
	if s.manifests != nil {
		s.manifests.Remove(file)
	}
}

// Clear clears all cached data.
func (s *Service) Clear() {
	if s.cache != nil {
		s.cache.Clear()
	}
	if s.symbols != nil {
		s.symbols.Clear()
	}
	if s.callGraph != nil {
		s.callGraph.Clear()
	}
	if s.manifests != nil {
		s.manifests.Clear()
	}
}

// CallersOf returns all callers of the function at the given position.
func (s *Service) CallersOf(file string, line, col int) []*index.CallEdge {
	if s.symbols == nil || s.callGraph == nil {
		return nil
	}

	sym := s.symbols.SymbolAt(file, line, col)
	if sym == nil || !isCallable(sym.Kind) {
		return nil
	}

	return s.callGraph.CallersOf(file, sym.Name)
}

// CalleesOf returns all functions called by the function at the given position.
func (s *Service) CalleesOf(file string, line, col int) []*index.CallEdge {
	if s.symbols == nil || s.callGraph == nil {
		return nil
	}

	sym := s.symbols.SymbolAt(file, line, col)
	if sym == nil || !isCallable(sym.Kind) {
		return nil
	}

	return s.callGraph.CalleesOf(file, sym.Name)
}

func isCallable(kind index.SymbolKind) bool {
	return kind == index.SymbolFunction || kind == index.SymbolMethod
}

// DocumentSymbol represents a symbol for document outline.
type DocumentSymbol struct {
	Name     string
	Kind     index.SymbolKind
	Span     diag.Span
	Children []*DocumentSymbol
}

// DocumentSymbols returns all symbols in a file for document outline.
func (s *Service) DocumentSymbols(file string) []*DocumentSymbol {
	if s.symbols == nil {
		return nil
	}

	syms := s.symbols.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}

	// Build tree by grouping children under their scope
	byScope := make(map[string][]*index.Symbol)
	var topLevel []*index.Symbol

	for _, sym := range syms {
		if sym.Scope == "" {
			topLevel = append(topLevel, sym)
		} else {
			byScope[sym.Scope] = append(byScope[sym.Scope], sym)
		}
	}

	result := make([]*DocumentSymbol, 0, len(topLevel))
	for _, sym := range topLevel {
		ds := &DocumentSymbol{
			Name: sym.Name,
			Kind: sym.Kind,
			Span: sym.DefSpan,
		}
		// Add children
		for _, child := range byScope[sym.Name] {
			ds.Children = append(ds.Children, &DocumentSymbol{
				Name: child.Name,
				Kind: child.Kind,
				Span: child.DefSpan,
			})
		}
		result = append(result, ds)
	}

	return result
}

// WorkspaceSymbols searches for symbols matching a query across all files.
func (s *Service) WorkspaceSymbols(query string) []*index.WorkspaceSymbol {
	if query == "" {
		return nil
	}

	var results []*index.WorkspaceSymbol

	// Search local symbols
	if s.symbols != nil {
		results = s.symbols.Search(query)
	}

	// Search manifest symbols
	if s.manifests != nil {
		manifestSyms := s.manifests.SearchSymbols(query)
		results = append(results, manifestSyms...)
	}

	return results
}

// FormatType formats a type for display in hover/completion.
func FormatType(t any) string {
	if t == nil {
		return "unknown"
	}

	// Handle typ.Type interface
	if tt, ok := t.(typ.Type); ok {
		return formatTypType(tt)
	}

	return "unknown"
}

func formatTypType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}

	// Check primitives by kind
	switch t.Kind() {
	case kind.Nil:
		return "nil"
	case kind.Boolean:
		return "boolean"
	case kind.Number:
		return "number"
	case kind.Integer:
		return "integer"
	case kind.String:
		return "string"
	case kind.Any:
		return "any"
	case kind.Unknown:
		return "unknown"
	case kind.Never:
		return "never"
	case kind.Self:
		return "self"
	}

	// Handle composite types
	switch v := t.(type) {
	case *typ.Function:
		return formatFunction(v)

	case *typ.Record:
		return formatRecord(v)

	case *typ.Array:
		return formatTypType(v.Element) + "[]"

	case *typ.Map:
		return fmt.Sprintf("{ [%s]: %s }", formatTypType(v.Key), formatTypType(v.Value))

	case *typ.Union:
		if len(v.Members) == 0 {
			return "never"
		}
		parts := make([]string, len(v.Members))
		for i, m := range v.Members {
			parts[i] = formatTypType(m)
		}
		return strings.Join(parts, " | ")

	case *typ.Optional:
		return formatTypType(v.Inner) + "?"

	case *typ.Tuple:
		parts := make([]string, len(v.Elements))
		for i, e := range v.Elements {
			parts[i] = formatTypType(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"

	case *typ.TypeParam:
		return v.Name

	case *typ.Literal:
		switch val := v.Value.(type) {
		case string:
			return fmt.Sprintf("%q", val)
		case bool:
			return strconv.FormatBool(val)
		default:
			return fmt.Sprintf("%v", v.Value)
		}

	case *typ.Alias:
		return v.Name

	case *typ.Interface:
		if v.Name != "" {
			return v.Name
		}
		return "interface{}"

	case *typ.Intersection:
		if len(v.Members) == 0 {
			return "unknown"
		}
		parts := make([]string, len(v.Members))
		for i, m := range v.Members {
			parts[i] = formatTypType(m)
		}
		return strings.Join(parts, " & ")

	case *typ.Sum:
		return v.Name

	case *typ.Generic:
		if len(v.TypeParams) == 0 {
			return v.Name
		}
		params := make([]string, len(v.TypeParams))
		for i, p := range v.TypeParams {
			params[i] = p.Name
		}
		return v.Name + "<" + strings.Join(params, ", ") + ">"

	case *typ.Instantiated:
		if v.Generic == nil {
			return "unknown"
		}
		args := make([]string, len(v.TypeArgs))
		for i, a := range v.TypeArgs {
			args[i] = formatTypType(a)
		}
		return v.Generic.Name + "<" + strings.Join(args, ", ") + ">"

	case *typ.Platform:
		return v.Name

	case *typ.Ref:
		if v.Module != "" {
			return v.Module + "." + v.Name
		}
		return v.Name

	case *typ.Meta:
		if v.Of == nil {
			return "type<unknown>"
		}
		return "type<" + formatTypType(v.Of) + ">"

	case *typ.TypeVar:
		return fmt.Sprintf("?%d", v.ID)

	default:
		return t.String()
	}
}

func formatFunction(f *typ.Function) string {
	if f == nil {
		return "function"
	}
	params := make([]string, 0, len(f.Params)+1)
	for _, p := range f.Params {
		params = append(params, p.Name+": "+formatTypType(p.Type))
	}
	if f.Variadic != nil {
		params = append(params, "..."+formatTypType(f.Variadic))
	}

	var returns string
	switch len(f.Returns) {
	case 0:
		returns = "void"
	case 1:
		returns = formatTypType(f.Returns[0])
	default:
		parts := make([]string, len(f.Returns))
		for i, r := range f.Returns {
			parts[i] = formatTypType(r)
		}
		returns = "(" + strings.Join(parts, ", ") + ")"
	}

	return "(" + strings.Join(params, ", ") + ") -> " + returns
}

func formatRecord(r *typ.Record) string {
	if r == nil || len(r.Fields) == 0 {
		return "{}"
	}
	if len(r.Fields) <= 3 {
		parts := make([]string, len(r.Fields))
		for i, f := range r.Fields {
			parts[i] = f.Name + ": " + formatTypType(f.Type)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
	return fmt.Sprintf("{ %s, ... }", r.Fields[0].Name+": "+formatTypType(r.Fields[0].Type))
}
