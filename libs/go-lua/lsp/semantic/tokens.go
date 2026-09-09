// Package semantic provides semantic token support for LSP.
package semantic

import (
	"sort"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

// TokenType represents semantic token types.
type TokenType uint32

const (
	TokNamespace TokenType = iota
	TokType
	TokClass
	TokEnum
	TokInterface
	TokStruct
	TokTypeParameter
	TokParameter
	TokVariable
	TokProperty
	TokEnumMember
	TokEvent
	TokFunction
	TokMethod
	TokMacro
	TokKeyword
	TokModifier
	TokComment
	TokString
	TokNumber
	TokRegexp
	TokOperator
)

// String returns the LSP token type name.
func (t TokenType) String() string {
	names := []string{
		"namespace", "type", "class", "enum", "interface",
		"struct", "typeParameter", "parameter", "variable",
		"property", "enumMember", "event", "function", "method",
		"macro", "keyword", "modifier", "comment", "string",
		"number", "regexp", "operator",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return "unknown"
}

// TokenModifier represents semantic token modifiers as a bitmask.
type TokenModifier uint32

const (
	ModDeclaration TokenModifier = 1 << iota
	ModDefinition
	ModReadonly
	ModStatic
	ModDeprecated
	ModAbstract
	ModAsync
	ModModification
	ModDocumentation
	ModDefaultLibrary
	// Custom modifiers for Lua
	ModPure    // function is pure (no side effects)
	ModEscapes // variable escapes its scope
	ModUnused  // variable is unused
	ModMutable // variable is mutated
)

// String returns modifier names.
func (m TokenModifier) String() string {
	var names []string
	if m&ModDeclaration != 0 {
		names = append(names, "declaration")
	}
	if m&ModDefinition != 0 {
		names = append(names, "definition")
	}
	if m&ModReadonly != 0 {
		names = append(names, "readonly")
	}
	if m&ModStatic != 0 {
		names = append(names, "static")
	}
	if m&ModDeprecated != 0 {
		names = append(names, "deprecated")
	}
	if m&ModAbstract != 0 {
		names = append(names, "abstract")
	}
	if m&ModAsync != 0 {
		names = append(names, "async")
	}
	if m&ModModification != 0 {
		names = append(names, "modification")
	}
	if m&ModDocumentation != 0 {
		names = append(names, "documentation")
	}
	if m&ModDefaultLibrary != 0 {
		names = append(names, "defaultLibrary")
	}
	if m&ModPure != 0 {
		names = append(names, "pure")
	}
	if m&ModEscapes != 0 {
		names = append(names, "escapes")
	}
	if m&ModUnused != 0 {
		names = append(names, "unused")
	}
	if m&ModMutable != 0 {
		names = append(names, "mutable")
	}
	if len(names) == 0 {
		return ""
	}
	result := names[0]
	for _, n := range names[1:] {
		result += "," + n
	}
	return result
}

// Token represents a single semantic token.
type Token struct {
	Span      diag.Span
	Type      TokenType
	Modifiers TokenModifier
}

// Legend describes available token types and modifiers.
type Legend struct {
	TokenTypes     []string
	TokenModifiers []string
}

// DefaultLegend returns the standard legend for our semantic tokens.
func DefaultLegend() *Legend {
	return &Legend{
		TokenTypes: []string{
			"namespace", "type", "class", "enum", "interface",
			"struct", "typeParameter", "parameter", "variable",
			"property", "enumMember", "event", "function", "method",
			"macro", "keyword", "modifier", "comment", "string",
			"number", "regexp", "operator",
		},
		TokenModifiers: []string{
			"declaration", "definition", "readonly", "static",
			"deprecated", "abstract", "async", "modification",
			"documentation", "defaultLibrary",
			"pure", "escapes", "unused", "mutable",
		},
	}
}

// Provider computes semantic tokens for a file.
type Provider struct {
	symbols *index.SymbolIndex
}

// NewProvider creates a semantic tokens provider.
func NewProvider(symbols *index.SymbolIndex) *Provider {
	return &Provider{symbols: symbols}
}

// TokensFull returns all semantic tokens for a file.
func (p *Provider) TokensFull(file string) []Token {
	if p.symbols == nil {
		return nil
	}

	syms := p.symbols.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}

	tokens := make([]Token, 0, len(syms)*2)

	for _, sym := range syms {
		// Token for definition
		tok := Token{
			Span:      sym.DefSpan,
			Type:      symbolKindToTokenType(sym.Kind),
			Modifiers: symbolToModifiers(sym, true),
		}
		tokens = append(tokens, tok)

		// Tokens for references
		refs := p.symbols.ReferencesTo(sym)
		for _, ref := range refs {
			refTok := Token{
				Span:      ref.UseSpan,
				Type:      symbolKindToTokenType(sym.Kind),
				Modifiers: symbolToModifiers(sym, false),
			}
			tokens = append(tokens, refTok)
		}
	}

	// Sort by position
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].Span.StartLine != tokens[j].Span.StartLine {
			return tokens[i].Span.StartLine < tokens[j].Span.StartLine
		}
		return tokens[i].Span.StartCol < tokens[j].Span.StartCol
	})

	return tokens
}

// TokensRange returns semantic tokens for a range.
func (p *Provider) TokensRange(file string, span diag.Span) []Token {
	all := p.TokensFull(file)
	if all == nil {
		return nil
	}

	var result []Token
	for _, tok := range all {
		if tokenInRange(tok, span) {
			result = append(result, tok)
		}
	}
	return result
}

// tokenInRange checks if a token overlaps with a range.
func tokenInRange(tok Token, span diag.Span) bool {
	// Token ends before range starts
	if tok.Span.EndLine < span.StartLine {
		return false
	}
	if tok.Span.EndLine == span.StartLine && tok.Span.EndCol < span.StartCol {
		return false
	}

	// Token starts after range ends
	if tok.Span.StartLine > span.EndLine {
		return false
	}
	if tok.Span.StartLine == span.EndLine && tok.Span.StartCol > span.EndCol {
		return false
	}

	return true
}

// symbolKindToTokenType maps symbol kinds to token types.
func symbolKindToTokenType(kind index.SymbolKind) TokenType {
	switch kind {
	case index.SymbolFunction:
		return TokFunction
	case index.SymbolMethod:
		return TokMethod
	case index.SymbolVariable:
		return TokVariable
	case index.SymbolParameter:
		return TokParameter
	case index.SymbolField:
		return TokProperty
	case index.SymbolType:
		return TokType
	default:
		return TokVariable
	}
}

// symbolToModifiers computes modifiers for a symbol.
func symbolToModifiers(sym *index.Symbol, isDefinition bool) TokenModifier {
	var mods TokenModifier

	if isDefinition {
		mods |= ModDefinition
	}

	if sym.Escapes {
		mods |= ModEscapes
	}

	return mods
}

// Encode encodes tokens to LSP delta format.
// Returns array of: [deltaLine, deltaStartChar, length, tokenType, tokenModifiers]
func Encode(tokens []Token) []uint32 {
	if len(tokens) == 0 {
		return nil
	}

	result := make([]uint32, 0, len(tokens)*5)
	prevLine := 0
	prevCol := 0

	for _, tok := range tokens {
		line := tok.Span.StartLine - 1 // 0-indexed
		col := tok.Span.StartCol - 1

		deltaLine := line - prevLine
		deltaCol := col
		if deltaLine == 0 {
			deltaCol = col - prevCol
		}

		length := tokenLength(tok)

		result = append(result,
			uint32(deltaLine),
			uint32(deltaCol),
			uint32(length),
			uint32(tok.Type),
			uint32(tok.Modifiers),
		)

		prevLine = line
		prevCol = col
	}

	return result
}

// tokenLength computes the length of a token.
// For multi-line tokens, returns EndCol as LSP semantic tokens
// are line-based. In practice, Lua identifiers never span lines.
func tokenLength(tok Token) int {
	if tok.Span.StartLine == tok.Span.EndLine {
		return tok.Span.EndCol - tok.Span.StartCol
	}
	return tok.Span.EndCol
}

// Decode decodes LSP delta format back to tokens.
func Decode(data []uint32) []Token {
	if len(data) == 0 || len(data)%5 != 0 {
		return nil
	}

	tokens := make([]Token, 0, len(data)/5)
	line := 0
	col := 0

	for i := 0; i < len(data); i += 5 {
		deltaLine := int(data[i])
		deltaCol := int(data[i+1])
		length := int(data[i+2])
		tokType := TokenType(data[i+3])
		mods := TokenModifier(data[i+4])

		line += deltaLine
		if deltaLine > 0 {
			col = deltaCol
		} else {
			col += deltaCol
		}

		tok := Token{
			Span: diag.Span{
				StartLine: line + 1,
				StartCol:  col + 1,
				EndLine:   line + 1,
				EndCol:    col + 1 + length,
			},
			Type:      tokType,
			Modifiers: mods,
		}
		tokens = append(tokens, tok)
	}

	return tokens
}
