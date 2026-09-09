package refactor

import (
	"errors"

	"github.com/wippyai/go-lua/lsp/edit"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
)

// Extract refactoring errors.
var (
	ErrEmptySelection   = errors.New("refactor: empty selection")
	ErrInvalidSelection = errors.New("refactor: invalid selection for extraction")
	ErrNoSymbols        = errors.New("refactor: no symbol index available")
)

// ExtractInfo describes what can be extracted from a selection.
type ExtractInfo struct {
	CanExtractVariable bool
	CanExtractFunction bool
	CapturedVariables  []string // variables referenced from outside the selection
	DefinedVariables   []string // variables defined within the selection
	UsedAfter          []string // variables defined in selection but used after
}

// ExtractProvider handles extraction refactorings.
type ExtractProvider struct {
	symbols *index.SymbolIndex
}

// NewExtractProvider creates an extract provider.
func NewExtractProvider(symbols *index.SymbolIndex) *ExtractProvider {
	return &ExtractProvider{symbols: symbols}
}

// AnalyzeSelection analyzes a selection for extraction possibilities.
func (p *ExtractProvider) AnalyzeSelection(file string, span diag.Span) *ExtractInfo {
	if !span.Valid() {
		return &ExtractInfo{}
	}

	info := &ExtractInfo{
		CanExtractVariable: true,
		CanExtractFunction: true,
	}

	if p.symbols == nil {
		return info
	}

	syms := p.symbols.SymbolsInFile(file)
	for _, sym := range syms {
		// Check if symbol is defined within selection
		if spanContainsSpan(span, sym.DefSpan) {
			info.DefinedVariables = append(info.DefinedVariables, sym.Name)
		}

		// Check if symbol is referenced within selection but defined outside
		if spanContainsSpan(span, sym.DefSpan) {
			continue
		}

		refs := p.symbols.ReferencesTo(sym)
		for _, ref := range refs {
			if ref.File == file && spanContainsSpan(span, ref.UseSpan) {
				info.CapturedVariables = append(info.CapturedVariables, sym.Name)
				break
			}
		}
	}

	// Check for variables defined in selection but used after
	for _, sym := range syms {
		if !spanContainsSpan(span, sym.DefSpan) {
			continue
		}
		refs := p.symbols.ReferencesTo(sym)
		for _, ref := range refs {
			if ref.File == file && ref.UseSpan.StartLine > span.EndLine {
				info.UsedAfter = append(info.UsedAfter, sym.Name)
				break
			}
			if ref.File == file && ref.UseSpan.StartLine == span.EndLine && ref.UseSpan.StartCol > span.EndCol {
				info.UsedAfter = append(info.UsedAfter, sym.Name)
				break
			}
		}
	}

	// Can't extract to variable if multiple statements or has side effects
	if len(info.DefinedVariables) > 0 {
		info.CanExtractVariable = false
	}

	return info
}

// ExtractVariable extracts an expression into a local variable.
// The exprText parameter should contain the text of the expression being extracted.
func (p *ExtractProvider) ExtractVariable(file string, span diag.Span, varName, exprText string) (*edit.WorkspaceEdit, error) {
	if !span.Valid() {
		return nil, ErrEmptySelection
	}

	if err := ValidateName(varName); err != nil {
		return nil, err
	}

	info := p.AnalyzeSelection(file, span)
	if !info.CanExtractVariable {
		return nil, ErrInvalidSelection
	}

	builder := edit.NewBuilder()

	// Insert variable declaration before the line containing the expression
	declaration := "local " + varName + " = " + exprText + "\n"
	builder.Insert(file, span.StartLine, 1, declaration)

	// Replace the original expression with the variable name
	builder.Replace(file, span, varName)

	return builder.Build(), nil
}

// ExtractFunction extracts a code block into a function.
// The codeText parameter should contain the text of the code being extracted.
func (p *ExtractProvider) ExtractFunction(file string, span diag.Span, funcName, codeText string) (*edit.WorkspaceEdit, error) {
	if !span.Valid() {
		return nil, ErrEmptySelection
	}

	if err := ValidateName(funcName); err != nil {
		return nil, err
	}

	info := p.AnalyzeSelection(file, span)

	builder := edit.NewBuilder()

	// Build parameter list from captured variables
	params := ""
	for i, v := range info.CapturedVariables {
		if i > 0 {
			params += ", "
		}
		params += v
	}

	// Build return statement for variables used after
	returns := ""
	if len(info.UsedAfter) > 0 {
		returns = "\treturn "
		for i, v := range info.UsedAfter {
			if i > 0 {
				returns += ", "
			}
			returns += v
		}
		returns += "\n"
	}

	// Indent the extracted code
	indentedCode := "\t" + codeText + "\n"

	// Insert function before the selection
	funcDecl := "local function " + funcName + "(" + params + ")\n" + indentedCode + returns + "end\n\n"
	builder.Insert(file, span.StartLine, 1, funcDecl)

	// Build the call expression
	callExpr := funcName + "(" + params + ")"
	if len(info.UsedAfter) > 0 {
		callExpr = "local "
		for i, v := range info.UsedAfter {
			if i > 0 {
				callExpr += ", "
			}
			callExpr += v
		}
		callExpr += " = " + funcName + "(" + params + ")"
	}

	// Replace the selection with the call
	builder.Replace(file, span, callExpr)

	return builder.Build(), nil
}

// spanContainsSpan checks if outer contains inner.
func spanContainsSpan(outer, inner diag.Span) bool {
	if !outer.Valid() || !inner.Valid() {
		return false
	}

	// inner starts before outer
	if inner.StartLine < outer.StartLine {
		return false
	}
	if inner.StartLine == outer.StartLine && inner.StartCol < outer.StartCol {
		return false
	}

	// inner ends after outer
	if inner.EndLine > outer.EndLine {
		return false
	}
	if inner.EndLine == outer.EndLine && inner.EndCol > outer.EndCol {
		return false
	}

	return true
}
