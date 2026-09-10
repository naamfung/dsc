package diag

import "fmt"

// Severity indicates the importance level of a diagnostic.
//
// Severity levels affect compilation behavior and IDE presentation:
//   - Error: Type errors that indicate likely runtime failures. May block execution.
//   - Warning: Suspicious patterns that may indicate bugs but are not type errors.
//   - Hint: Suggestions for improvement that do not indicate problems.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityHint
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Position identifies a location in source code.
//
// Line and Column are 1-indexed to match editor conventions.
// File is the source file path for multi-file diagnostics.
type Position struct {
	File   string
	Line   int
	Column int
}

func (p Position) String() string {
	if p.File == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
}

// Span defines a range in source code for highlighting.
//
// All fields are 1-indexed. EndLine and EndCol may be 0 to indicate
// a point span (single character) or unknown extent.
type Span struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Valid returns true if the span has meaningful positions.
func (s Span) Valid() bool {
	return s.StartLine > 0 && s.StartCol > 0
}

// SingleLine returns true if the span does not cross line boundaries.
func (s Span) SingleLine() bool {
	return s.StartLine == s.EndLine || s.EndLine == 0
}

// Code categorizes diagnostics by error type.
//
// Each code has associated metadata (title, explanation) in codeInfos.
// Codes are formatted as E0000-E9999 for display.
type Code int

const (
	ErrTypeMismatch Code = iota
	ErrUndefined
	ErrNotCallable
	ErrWrongArity
	ErrNoField
	ErrNotIndexable
	ErrNoSynthesizer
	ErrNoHandler
	ErrReadonly
	ErrUnreachable
	ErrMissingReturn
	ErrDepthLimit
	ErrUnresolvedTypeRef
	ErrCircularReference
	ErrNonExhaustive
	ErrUseBeforeAssign
	ErrPreconditionViolation
	ErrDuplicateDeclaration
	ErrInvalidIndexType
	ErrInvalidOperand
	ErrUnsatConstraint
	ErrOptionalCall
	ErrNoMethod
)

// Name returns the formatted code identifier (e.g., E0001).
func (c Code) Name() string {
	return fmt.Sprintf("E%04d", c)
}

// CodeInfo holds metadata for a diagnostic code.
type CodeInfo struct {
	Title       string
	Explanation string
}

var codeInfos = map[Code]CodeInfo{
	ErrTypeMismatch:         {"type mismatch", "The type of a value does not match the expected type."},
	ErrUndefined:            {"undefined variable", "A variable or identifier was used before it was declared."},
	ErrNotCallable:          {"not callable", "Attempted to call a value that is not a function."},
	ErrWrongArity:           {"wrong number of arguments", "The function was called with an incorrect number of arguments."},
	ErrNoField:              {"field not found", "The accessed field does not exist on the type."},
	ErrNotIndexable:         {"not indexable", "Attempted to index a value that cannot be indexed."},
	ErrReadonly:             {"readonly violation", "Attempted to modify a readonly field or value."},
	ErrMissingReturn:        {"missing return", "A function with a declared return type does not return on all paths."},
	ErrNonExhaustive:        {"non-exhaustive match", "Not all possible cases are handled."},
	ErrUseBeforeAssign:      {"use before assignment", "A variable was used before it was definitely assigned a value."},
	ErrDuplicateDeclaration: {"duplicate declaration", "A name was declared more than once in the same scope."},
	ErrInvalidIndexType:     {"invalid index type", "The index type is not valid for this collection."},
	ErrInvalidOperand:       {"invalid operand", "The operand type is not valid for this operation."},
	ErrUnsatConstraint:      {"unsatisfiable constraint", "Numeric constraints on this path form a contradiction."},
	ErrOptionalCall:         {"optional call", "Attempted to call a method on an optional value without a nil check."},
	ErrNoMethod:             {"method not found", "The method does not exist on the type."},
}

// Info returns metadata for this code.
func (c Code) Info() CodeInfo {
	if info, ok := codeInfos[c]; ok {
		return info
	}
	return CodeInfo{Title: "error"}
}

// Diagnostic represents a type error, warning, or hint.
//
// A Diagnostic contains all information needed to display a rich error message:
//   - Position/Span: Where the error occurred for source highlighting
//   - Code: Categorizes the error for filtering and documentation lookup
//   - Message: The primary error description (formatted from Format + args)
//   - Explanation: Detailed description of why this is an error
//   - Help: Suggested fix or workaround
//   - Labels: Secondary source locations with annotations
//
// Diagnostics implement the error interface for use in error handling.
type Diagnostic struct {
	Position    Position
	Span        Span
	Code        Code
	Message     string
	Format      string
	Severity    Severity
	Explanation string
	Help        string
	Labels      []Label
}

// Label marks a secondary source location with an annotation message.
//
// Labels provide additional context for diagnostics, such as pointing to
// the declaration when an error involves both use and definition sites.
type Label struct {
	Span    Span
	Message string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
}

// Error implements the error interface.
func (d Diagnostic) Error() string {
	return d.String()
}
