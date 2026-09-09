// Package diag provides diagnostic reporting for the type checker.
//
// Diagnostics are categorized by severity:
//   - Error: Type errors that prevent correctness (mismatches, undefined vars)
//   - Warning: Suspicious patterns that may indicate bugs (unreachable code)
//   - Hint: Suggestions for improvement (style, optimization)
//
// Each diagnostic includes position, code, message, and severity. Diagnostics
// render in Rust-style format with source context and optional ANSI colors.
//
// The Collector accumulates diagnostics during type checking with automatic
// deduplication. It supports snapshot/restore via Truncate for speculative
// checking where diagnostics may need to be rolled back.
//
// Usage:
//
//	collector := diag.NewCollector("example.lua")
//	collector.Add(node, diag.ErrTypeMismatch, "expected %s, got %s", want, got)
//
//	if collector.HasErrors() {
//	    source := diag.ParseSource(sourceCode)
//	    fmt.Print(collector.RenderAllColored(source))
//	}
package diag

import (
	"fmt"
	"strings"
	"sync"
)

// PositionHolder is implemented by AST nodes that have line information.
type PositionHolder interface {
	Line() int
}

// SpanHolder extends PositionHolder with full range information.
type SpanHolder interface {
	PositionHolder
	Column() int
	LastLine() int
	LastColumn() int
}

type diagKey struct {
	line   int
	column int
	format string
}

// Collector accumulates diagnostics during type checking.
// Thread-safe and supports deduplication by position and format string.
type Collector struct {
	mu     sync.Mutex
	errors []Diagnostic
	seen   map[diagKey]bool
	source string
}

// NewCollector creates a collector for the given source file.
func NewCollector(source string) *Collector {
	return &Collector{
		source: source,
		errors: make([]Diagnostic, 0),
		seen:   make(map[diagKey]bool),
	}
}

// Add records an error diagnostic.
func (c *Collector) Add(node PositionHolder, code Code, format string, args ...any) {
	if c == nil {
		return
	}

	c.add(node, code, SeverityError, format, args...)
}

// AddWarning records a warning diagnostic.
func (c *Collector) AddWarning(node PositionHolder, code Code, format string, args ...any) {
	if c == nil {
		return
	}

	c.add(node, code, SeverityWarning, format, args...)
}

// AddHint records a hint diagnostic.
func (c *Collector) AddHint(node PositionHolder, code Code, format string, args ...any) {
	if c == nil {
		return
	}

	c.add(node, code, SeverityHint, format, args...)
}

func (c *Collector) add(node PositionHolder, code Code, severity Severity, format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	line, column := 1, 1
	var span Span

	if node != nil {
		line = node.Line()
		if sh, ok := node.(SpanHolder); ok {
			column = sh.Column()
			span = Span{
				StartLine: line,
				StartCol:  column,
				EndLine:   sh.LastLine(),
				EndCol:    sh.LastColumn(),
			}
		}
	}

	key := diagKey{line: line, column: column, format: format}
	if c.seen[key] {
		return
	}
	c.seen[key] = true

	msg := fmt.Sprintf(format, args...)
	explanation, help := ContextualHelp(code, format, msg)

	c.errors = append(c.errors, Diagnostic{
		Position:    Position{File: c.source, Line: line, Column: column},
		Span:        span,
		Code:        code,
		Message:     msg,
		Format:      format,
		Severity:    severity,
		Explanation: explanation,
		Help:        help,
	})
}

// ContextualHelp returns explanation and help text based on error patterns.
func ContextualHelp(code Code, format, _ string) (explanation, help string) {
	info := code.Info()
	explanation = info.Explanation

	switch {
	case strings.Contains(format, "cannot concatenate"):
		explanation = "The .. operator requires string operands in strict mode."
		help = "Convert the value to string using tostring() before concatenation."

	case strings.Contains(format, "cannot compare"):
		explanation = "Relational operators (<, <=, >, >=) require operands of the same type."
		help = "Compare values of the same type, or convert them explicitly."

	case strings.Contains(format, "map key type"):
		explanation = "Map indexing requires a key of the declared key type."

	case strings.Contains(format, "array index must be"):
		explanation = "Arrays can only be indexed with numeric values."

	case strings.Contains(format, "arithmetic requires"):
		explanation = "Arithmetic operators (+, -, *, /, etc.) require numeric operands."

	case strings.Contains(format, "cannot call method on optional"):
		explanation = "The value may be nil. Methods cannot be called on nil values."
		help = "Add a nil check before calling the method."

	case strings.Contains(format, "cannot access field on optional"):
		explanation = "The value may be nil. Fields cannot be accessed on nil values."
		help = "Add a nil check before accessing the field."

	case strings.Contains(format, "not enough arguments"):
		explanation = "The function requires more arguments than provided."

	case strings.Contains(format, "too many arguments"):
		explanation = "The function was called with more arguments than it accepts."

	case strings.Contains(format, "cannot assign"):
		explanation = "The value type does not match the declared variable type."
		help = "Ensure the assigned value matches the declared type, or adjust the type annotation."

	case strings.Contains(format, "cannot return"):
		explanation = "The return value type does not match the declared return type."
		help = "Ensure the return value matches the declared return type."

	case strings.Contains(format, "does not exist on type"):
		explanation = "The field or method is not defined on this type."
		help = "Check the type definition for available fields and methods."

	case strings.Contains(format, "expected") && strings.Contains(format, "got"):
		explanation = "The argument type does not match the expected parameter type."
		help = "Pass a value of the correct type to the function."

	case strings.Contains(format, "expected function"):
		explanation = "Attempted to call a value that is not a function."
		help = "Ensure the value is a function before calling it."

	case strings.Contains(format, "cannot perform arithmetic"):
		explanation = "Arithmetic operators require numeric operands."
		help = "Convert the value to a number or check for nil before performing arithmetic."

	case strings.Contains(format, "no method") && strings.Contains(format, "missing on"):
		explanation = "The method exists on some union members but not all."
		help = "Narrow the type first using a type guard or conditional check."

	case strings.Contains(format, "no method"):
		explanation = "The method is not defined on this type."
		help = "Check the type definition for available methods."
	}

	return explanation, help
}

// All returns a copy of all diagnostics.
func (c *Collector) All() []Diagnostic {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Diagnostic, len(c.errors))
	copy(result, c.errors)

	return result
}

// HasErrors returns true if any error-severity diagnostics exist.
func (c *Collector) HasErrors() bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, err := range c.errors {
		if err.Severity == SeverityError {
			return true
		}
	}

	return false
}

// HasWarnings returns true if any warning-severity diagnostics exist.
func (c *Collector) HasWarnings() bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, err := range c.errors {
		if err.Severity == SeverityWarning {
			return true
		}
	}

	return false
}

// Len returns the number of diagnostics.
func (c *Collector) Len() int {
	if c == nil {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.errors)
}

// Truncate removes diagnostics added after count, rebuilding the dedup cache.
func (c *Collector) Truncate(count int) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if count < 0 {
		count = 0
	}

	if count >= len(c.errors) {
		return
	}

	c.errors = c.errors[:count]
	// Rebuild seen map from remaining errors using stored format
	c.seen = make(map[diagKey]bool)
	for _, diag := range c.errors {
		key := diagKey{line: diag.Position.Line, column: diag.Position.Column, format: diag.Format}
		c.seen[key] = true
	}
}

// Clear removes all diagnostics.
func (c *Collector) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = c.errors[:0]
	c.seen = make(map[diagKey]bool)
}

// Source returns the source file name.
func (c *Collector) Source() string {
	if c == nil {
		return ""
	}

	return c.source
}
