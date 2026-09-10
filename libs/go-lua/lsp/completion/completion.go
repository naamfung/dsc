// Package completion provides code completion for LSP.
package completion

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/lsp"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// Kind represents completion item kind (matches LSP CompletionItemKind).
type Kind int

const (
	KindText Kind = iota + 1
	KindMethod
	KindFunction
	KindConstructor
	KindField
	KindVariable
	KindClass
	KindInterface
	KindModule
	KindProperty
	KindUnit
	KindValue
	KindEnum
	KindKeyword
	KindSnippet
	KindColor
	KindFile
	KindReference
	KindFolder
	KindEnumMember
	KindConstant
	KindStruct
	KindEvent
	KindOperator
	KindTypeParameter
)

// Item represents a completion suggestion.
type Item struct {
	Label         string   // display text
	Kind          Kind     // item kind
	Detail        string   // type signature
	Documentation string   // doc comment
	InsertText    string   // text to insert (may differ from label)
	IsSnippet     bool     // if InsertText contains $1, $2 placeholders
	SortText      string   // for ordering
	FilterText    string   // for filtering
	Deprecated    bool     // whether this item is deprecated
	Type          typ.Type // the type of the completed item
}

// TriggerKind describes how completion was triggered.
type TriggerKind int

const (
	TriggerInvoked TriggerKind = iota + 1
	TriggerCharacter
	TriggerIncomplete
)

// ContextKind describes what kind of completion is needed.
type ContextKind int

const (
	ContextUnknown    ContextKind = iota
	ContextIdentifier             // local variable, function name
	ContextMember                 // after "." - field/method access
	ContextKeyword                // language keywords
)

// Context describes the completion context.
type Context struct {
	File        string
	Line        int
	Col         int
	Trigger     TriggerKind
	TriggerChar string
	Prefix      string      // text before cursor on current "word"
	Kind        ContextKind // detected context kind
	// ReceiverType is the resolved type of the receiver for member access (after ".").
	ReceiverType typ.Type
	// LocalSymbols holds in-scope locals at the completion site.
	// When provided, identifier completions will prefer these symbols.
	LocalSymbols []*index.Symbol
}

// TypeFormatter formats a type for display.
type TypeFormatter func(any) string

// Provider handles completion requests.
type Provider struct {
	symbols    *index.SymbolIndex
	formatType TypeFormatter
}

// NewProvider creates a completion provider.
func NewProvider(symbols *index.SymbolIndex) *Provider {
	return &Provider{symbols: symbols}
}

// SetTypeFormatter sets a custom type formatter for completion details.
func (p *Provider) SetTypeFormatter(f TypeFormatter) {
	p.formatType = f
}

// Complete returns completion items for the given context.
func (p *Provider) Complete(ctx *Context) []Item {
	if ctx == nil {
		return nil
	}

	var items []Item

	switch ctx.Kind {
	case ContextKeyword:
		items = p.keywordCompletions(ctx.Prefix)
	case ContextMember:
		items = p.memberCompletions(ctx)
	default:
		items = p.identifierCompletions(ctx)
	}

	// Sort items by relevance
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortText != "" && items[j].SortText != "" {
			return items[i].SortText < items[j].SortText
		}
		return items[i].Label < items[j].Label
	})

	return items
}

// identifierCompletions returns completions for identifiers (variables, functions).
func (p *Provider) identifierCompletions(ctx *Context) []Item {
	var items []Item
	prefix := strings.ToLower(ctx.Prefix)
	seen := make(map[string]bool)

	addLocal := func(sym *index.Symbol) {
		if sym == nil || sym.Name == "" {
			return
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), prefix) {
			return
		}
		item := p.symbolToItem(sym)
		item.SortText = "0" + sym.Name // locals first
		items = append(items, item)
		seen[sym.Name] = true
	}

	// Add locals first (scope-aware, type-checked by LSP).
	for _, sym := range ctx.LocalSymbols {
		if seen[sym.Name] {
			continue
		}
		addLocal(sym)
	}

	// Get all symbols in the file
	if p.symbols != nil {
		syms := p.symbols.SymbolsInFile(ctx.File)
		for _, sym := range syms {
			if seen[sym.Name] {
				continue
			}
			if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), prefix) {
				continue
			}

			item := p.symbolToItem(sym)
			item.SortText = "a" + sym.Name // sort symbols first
			items = append(items, item)
			seen[sym.Name] = true
		}
	}

	// Add keywords if prefix matches
	for _, kw := range lsp.LuaKeywords {
		if prefix == "" || strings.HasPrefix(kw, prefix) {
			items = append(items, Item{
				Label:    kw,
				Kind:     KindKeyword,
				SortText: "z" + kw, // sort keywords last
			})
		}
	}

	return items
}

// memberCompletions returns completions for member access (after ".").
func (p *Provider) memberCompletions(ctx *Context) []Item {
	prefix := strings.ToLower(ctx.Prefix)
	if ctx.ReceiverType != nil {
		return p.memberItemsFromType(ctx.ReceiverType, prefix)
	}

	// Without receiver type, fall back to field symbols from the file.
	if p.symbols == nil {
		return nil
	}

	var items []Item
	syms := p.symbols.SymbolsInFile(ctx.File)
	for _, sym := range syms {
		if sym.Kind != index.SymbolField {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), prefix) {
			continue
		}
		items = append(items, p.symbolToItem(sym))
	}

	return items
}

func (p *Provider) memberItemsFromType(t typ.Type, prefix string) []Item {
	if t == nil {
		return nil
	}

	itemsByName := make(map[string]Item)
	visited := make(map[typ.Type]bool)
	lowerPrefix := strings.ToLower(prefix)

	addItem := func(name string, kind Kind, t typ.Type) {
		if name == "" {
			return
		}
		if lowerPrefix != "" && !strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
			return
		}
		item := Item{
			Label:      name,
			Kind:       kind,
			FilterText: name,
		}
		if t != nil {
			item.Type = t
			if p.formatType != nil {
				item.Detail = p.formatType(t)
			} else {
				item.Detail = t.String()
			}
		}
		if existing, ok := itemsByName[name]; ok {
			if existing.Kind == KindField && kind == KindMethod {
				itemsByName[name] = item
			} else if existing.Detail == "" && item.Detail != "" {
				itemsByName[name] = item
			}
			return
		}
		itemsByName[name] = item
	}

	var visit func(typ.Type)
	visit = func(t typ.Type) {
		if t == nil {
			return
		}
		if visited[t] {
			return
		}
		visited[t] = true
		switch tt := t.(type) {
		case *typ.Alias:
			visit(tt.Target)
		case *typ.Optional:
			visit(tt.Inner)
		case *typ.Record:
			for _, field := range tt.Fields {
				kind := KindField
				if _, ok := field.Type.(*typ.Function); ok {
					kind = KindMethod
				}
				addItem(field.Name, kind, field.Type)
			}
		case *typ.Interface:
			for _, method := range tt.Methods {
				addItem(method.Name, KindMethod, method.Type)
			}
		case *typ.Union:
			for _, member := range tt.Members {
				visit(member)
			}
		case *typ.Intersection:
			for _, member := range tt.Members {
				visit(member)
			}
		case *typ.Instantiated:
			if expanded := subst.ExpandInstantiated(tt); expanded != nil && expanded != t {
				visit(expanded)
			}
		case *typ.Recursive:
			visit(tt.Body)
		case *typ.Generic:
			visit(tt.Body)
		case *typ.Meta:
			visit(tt.Of)
		case *typ.FieldAccess:
			visit(tt.Base)
		case *typ.IndexAccess:
			visit(tt.Base)
		}
	}

	visit(t)

	if len(itemsByName) == 0 {
		return nil
	}

	items := make([]Item, 0, len(itemsByName))
	for _, item := range itemsByName {
		items = append(items, item)
	}

	return items
}

// ResolveMemberType resolves the type of a named member on the given receiver type.
// It returns nil if the member cannot be resolved.
func ResolveMemberType(t typ.Type, name string) typ.Type {
	if t == nil || name == "" {
		return nil
	}
	visited := make(map[typ.Type]bool)
	return resolveMemberType(t, name, visited)
}

func resolveMemberType(t typ.Type, name string, visited map[typ.Type]bool) typ.Type {
	if t == nil {
		return nil
	}
	if visited[t] {
		return nil
	}
	visited[t] = true

	switch tt := t.(type) {
	case *typ.Alias:
		return resolveMemberType(tt.Target, name, visited)
	case *typ.Optional:
		return resolveMemberType(tt.Inner, name, visited)
	case *typ.Record:
		for _, field := range tt.Fields {
			if field.Name == name {
				return field.Type
			}
		}
		return nil
	case *typ.Interface:
		for _, method := range tt.Methods {
			if method.Name == name {
				return method.Type
			}
		}
		return nil
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			if mt := resolveMemberType(member, name, visited); mt != nil {
				members = append(members, mt)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			if mt := resolveMemberType(member, name, visited); mt != nil {
				members = append(members, mt)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Instantiated:
		if expanded := subst.ExpandInstantiated(tt); expanded != nil && expanded != t {
			return resolveMemberType(expanded, name, visited)
		}
	case *typ.Recursive:
		return resolveMemberType(tt.Body, name, visited)
	case *typ.Generic:
		return resolveMemberType(tt.Body, name, visited)
	case *typ.Meta:
		return resolveMemberType(tt.Of, name, visited)
	case *typ.FieldAccess:
		return resolveMemberType(tt.Base, name, visited)
	case *typ.IndexAccess:
		return resolveMemberType(tt.Base, name, visited)
	}

	return nil
}

// keywordCompletions returns Lua keyword completions.
func (p *Provider) keywordCompletions(prefix string) []Item {
	var items []Item
	prefix = strings.ToLower(prefix)

	for _, kw := range lsp.LuaKeywords {
		if prefix == "" || strings.HasPrefix(kw, prefix) {
			items = append(items, Item{
				Label: kw,
				Kind:  KindKeyword,
			})
		}
	}

	return items
}

// symbolToItem converts a symbol to a completion item.
func (p *Provider) symbolToItem(sym *index.Symbol) Item {
	item := Item{
		Label:      sym.Name,
		Kind:       symbolKindToCompletionKind(sym.Kind),
		FilterText: sym.Name,
	}

	if sym.Type != nil {
		if t, ok := sym.Type.(typ.Type); ok {
			item.Type = t
			if p.formatType != nil {
				item.Detail = p.formatType(t)
			} else {
				item.Detail = t.String()
			}
		}
	}

	return item
}

// symbolKindToCompletionKind maps symbol kinds to completion kinds.
func symbolKindToCompletionKind(kind index.SymbolKind) Kind {
	switch kind {
	case index.SymbolFunction:
		return KindFunction
	case index.SymbolMethod:
		return KindMethod
	case index.SymbolVariable:
		return KindVariable
	case index.SymbolParameter:
		return KindVariable
	case index.SymbolField:
		return KindField
	case index.SymbolType:
		return KindClass
	default:
		return KindVariable
	}
}

// ResolveItem adds detail to a completion item (lazy loading).
func (p *Provider) ResolveItem(item *Item) *Item {
	if item == nil {
		return nil
	}
	// Currently all info is populated upfront
	return item
}
