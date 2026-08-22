// Package signature provides signature help for function calls.
package signature

import (
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/typ"
)

// ParameterInfo describes a function parameter.
type ParameterInfo struct {
	Label         string // "name" or "name: type"
	Documentation string
}

// SignatureInfo describes a function signature.
type SignatureInfo struct {
	Label         string          // full signature like "foo(a: number, b: string) -> boolean"
	Documentation string          // doc comment
	Parameters    []ParameterInfo // individual parameters
}

// Result contains signature help information.
type Result struct {
	Signatures      []SignatureInfo
	ActiveSignature int // 0-based index of active signature
	ActiveParameter int // 0-based index of active parameter
}

// Provider handles signature help requests.
type Provider struct {
	symbols   *index.SymbolIndex
	callGraph *index.CallGraph
}

// NewProvider creates a signature help provider.
func NewProvider(symbols *index.SymbolIndex, callGraph *index.CallGraph) *Provider {
	return &Provider{symbols: symbols, callGraph: callGraph}
}

// Help returns signature information at the cursor position.
// Returns nil if cursor is not inside a function call.
func (p *Provider) Help(file string, line, col int) *Result {
	if p.callGraph == nil {
		return nil
	}

	// Find the call at position
	callEdge := p.callGraph.CallAt(file, line, col)
	if callEdge == nil {
		return nil
	}

	// Look up the called function's symbol using callee file for disambiguation
	sym := p.findFunctionSymbol(callEdge.CalleeFile, callEdge.CalleeName)
	if sym == nil {
		return nil
	}

	// Get function type
	funcType, ok := sym.Type.(*typ.Function)
	if !ok {
		return nil
	}

	// Build signature info
	sig := buildSignatureInfo(sym.Name, funcType)

	// Determine active parameter based on cursor position in call
	activeParam := p.findActiveParameter(callEdge, line, col, funcType)

	return &Result{
		Signatures:      []SignatureInfo{sig},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}
}

// findFunctionSymbol searches for a function symbol by file and name.
func (p *Provider) findFunctionSymbol(file, name string) *index.Symbol {
	if p.symbols == nil {
		return nil
	}

	// Try exact file match first
	var fileVarCandidate *index.Symbol
	if file != "" {
		sym := p.symbols.LookupByName(file, name)
		if sym != nil {
			if sym.Kind == index.SymbolFunction || sym.Kind == index.SymbolMethod {
				return sym
			}
			if sym.Kind == index.SymbolVariable {
				if _, ok := sym.Type.(*typ.Function); ok {
					fileVarCandidate = sym
				}
			}
		}
	}

	// Fall back to global search
	var varCandidate *index.Symbol
	results := p.symbols.Search(name)
	for _, result := range results {
		if (result.Kind == index.SymbolFunction || result.Kind == index.SymbolMethod) && result.Name == name {
			return p.symbols.LookupByName(result.File, name)
		}
		if result.Kind == index.SymbolVariable && result.Name == name {
			sym := p.symbols.LookupByName(result.File, name)
			if sym != nil {
				if _, ok := sym.Type.(*typ.Function); ok {
					varCandidate = sym
				}
			}
		}
	}

	if fileVarCandidate != nil {
		return fileVarCandidate
	}
	if varCandidate != nil {
		return varCandidate
	}

	return nil
}

// findActiveParameter estimates which parameter the cursor is on.
// This is approximate without full AST argument tracking.
func (p *Provider) findActiveParameter(edge *index.CallEdge, line, col int, funcType *typ.Function) int {
	// Basic heuristic: use relative position in call span
	callSpan := edge.CallSpan

	// If only one param, always return 0
	paramCount := len(funcType.Params)
	if funcType.Variadic != nil {
		paramCount++
	}
	if paramCount <= 1 {
		return 0
	}

	// Rough estimate based on position within call
	if line > callSpan.StartLine {
		// Multi-line call, estimate based on line offset
		lineOffset := line - callSpan.StartLine
		if lineOffset >= paramCount {
			return paramCount - 1
		}
		return lineOffset
	}

	// Same line - estimate based on column position
	callWidth := callSpan.EndCol - callSpan.StartCol
	if callWidth <= 0 {
		return 0
	}

	cursorOffset := col - callSpan.StartCol
	if cursorOffset < 0 {
		return 0
	}

	// Divide call span by param count
	paramWidth := callWidth / paramCount
	if paramWidth <= 0 {
		return 0
	}

	paramIndex := cursorOffset / paramWidth
	if paramIndex >= paramCount {
		return paramCount - 1
	}
	return paramIndex
}

// buildSignatureInfo creates a SignatureInfo from a function type.
func buildSignatureInfo(name string, funcType *typ.Function) SignatureInfo {
	params := make([]ParameterInfo, 0, len(funcType.Params))

	for _, param := range funcType.Params {
		label := param.Name + ": " + typeString(param.Type)
		params = append(params, ParameterInfo{Label: label})
	}

	// Add variadic param if present
	if funcType.Variadic != nil {
		label := "...: " + typeString(funcType.Variadic)
		params = append(params, ParameterInfo{Label: label})
	}

	return SignatureInfo{
		Label:      formatFullSignature(name, funcType),
		Parameters: params,
	}
}

// formatFullSignature creates the full signature string.
func formatFullSignature(name string, funcType *typ.Function) string {
	return name + funcType.String()
}

// typeString returns a string representation of a type.
func typeString(t typ.Type) string {
	if t == nil {
		return "any"
	}
	return t.String()
}
