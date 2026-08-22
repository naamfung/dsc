// This package handles post-analysis diagnostic operations:
//   - Sorting functions by source position for deterministic pass execution
//   - Sorting diagnostics for stable output ordering
//   - Generating widening diagnostics when type inference doesn't converge
//
// Deterministic ordering is essential for reproducible builds and test stability.
// All sorting uses stable tie-breakers (graph ID, message content) to ensure
// identical output across runs.
package pipeline

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/diag"
)

// SortedResultFunctions returns functions sorted by source position, then graph ID.
func SortedResultFunctions(results map[*ast.FunctionExpr]*api.FuncResult) []*ast.FunctionExpr {
	if len(results) == 0 {
		return nil
	}

	fns := make([]*ast.FunctionExpr, 0, len(results))
	for fn := range results {
		fns = append(fns, fn)
	}

	sort.Slice(fns, func(i, j int) bool {
		a := fns[i]
		b := fns[j]

		if a == nil || b == nil {
			return a != nil
		}

		if a.Line() != b.Line() {
			return a.Line() < b.Line()
		}
		if a.Column() != b.Column() {
			return a.Column() < b.Column()
		}
		if a.LastLine() != b.LastLine() {
			return a.LastLine() < b.LastLine()
		}
		if a.LastColumn() != b.LastColumn() {
			return a.LastColumn() < b.LastColumn()
		}

		ra := results[a]
		rb := results[b]
		ida := uint64(0)
		idb := uint64(0)
		if ra != nil && ra.Graph != nil {
			ida = ra.Graph.ID()
		}
		if rb != nil && rb.Graph != nil {
			idb = rb.Graph.ID()
		}
		return ida < idb
	})

	return fns
}

// SortDiagnostics sorts diagnostics deterministically by source position.
func SortDiagnostics(diags []diag.Diagnostic) {
	if len(diags) < 2 {
		return
	}

	sort.SliceStable(diags, func(i, j int) bool {
		a := diags[i]
		b := diags[j]

		if a.Position.File != b.Position.File {
			return a.Position.File < b.Position.File
		}
		if a.Position.Line != b.Position.Line {
			return a.Position.Line < b.Position.Line
		}
		if a.Position.Column != b.Position.Column {
			return a.Position.Column < b.Position.Column
		}
		if a.Span.StartLine != b.Span.StartLine {
			return a.Span.StartLine < b.Span.StartLine
		}
		if a.Span.StartCol != b.Span.StartCol {
			return a.Span.StartCol < b.Span.StartCol
		}
		if a.Span.EndLine != b.Span.EndLine {
			return a.Span.EndLine < b.Span.EndLine
		}
		if a.Span.EndCol != b.Span.EndCol {
			return a.Span.EndCol < b.Span.EndCol
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		if a.Format != b.Format {
			return a.Format < b.Format
		}
		if a.Help != b.Help {
			return a.Help < b.Help
		}
		return a.Explanation < b.Explanation
	})
}

// WideningDiagnostics reports symbols that were widened to unknown during preflow inference.
func WideningDiagnostics(sourceName string, fn *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
	if result == nil || result.FlowInputs == nil || len(result.FlowInputs.WideningEvents) == 0 {
		return nil
	}

	seenSCC := make(map[int]bool)
	var diags []diag.Diagnostic
	for _, event := range result.FlowInputs.WideningEvents {
		if seenSCC[event.SCCIndex] {
			continue
		}
		seenSCC[event.SCCIndex] = true

		symName := ""
		if result.Graph != nil {
			symName = result.Graph.NameOf(event.Symbol)
		}
		if symName == "" {
			symName = "<unknown>"
		}

		sccSize := len(event.SCC)
		msg := fmt.Sprintf("type inference did not converge for '%s' (SCC size %d); widened to unknown", symName, sccSize)

		pos := diag.Position{File: sourceName}
		span := diag.Span{}
		if fn != nil {
			pos.Line = fn.Line()
			pos.Column = fn.Column()
			span = ast.SpanOf(fn)
		}

		diags = append(diags, diag.Diagnostic{
			Position: pos,
			Span:     span,
			Severity: diag.SeverityWarning,
			Message:  msg,
		})
	}

	return diags
}

// ResolveSymbolName provides a stable name for diagnostics when CFG data is available.
func ResolveSymbolName(graph *cfg.Graph, sym cfg.SymbolID) string {
	if graph == nil {
		return ""
	}
	return graph.NameOf(sym)
}
