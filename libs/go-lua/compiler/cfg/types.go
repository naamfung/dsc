// Package cfg provides CFG construction utilities.
package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Point is an alias for basecfg.Point.
type Point = basecfg.Point

// CFG is an alias for basecfg.CFG.
type CFG = basecfg.CFG

// Node is an alias for basecfg.Node.
type Node = basecfg.Node

// NodeKind is an alias for basecfg.NodeKind.
type NodeKind = basecfg.NodeKind

// Node kind constants.
const (
	NodeEntry      = basecfg.NodeEntry
	NodeExit       = basecfg.NodeExit
	NodeAssign     = basecfg.NodeAssign
	NodeCall       = basecfg.NodeCall
	NodeBranch     = basecfg.NodeBranch
	NodeJoin       = basecfg.NodeJoin
	NodeReturn     = basecfg.NodeReturn
	NodeScopeEnter = basecfg.NodeScopeEnter
	NodeScopeExit  = basecfg.NodeScopeExit
	NodeTypeDef    = basecfg.NodeTypeDef
)

// NodeInfo is the interface implemented by all node info types.
type NodeInfo interface {
	Kind() basecfg.NodeKind
	nodeInfo() // seal interface
}

// Version is an alias for basecfg.Version.
type Version = basecfg.Version

// PhiOperand is an alias for basecfg.PhiOperand.
type PhiOperand = basecfg.PhiOperand

// PhiInfo is an alias for basecfg.PhiNode.
type PhiInfo = basecfg.PhiNode

// CondCheckKind is an alias for basecfg.CondCheckKind.
type CondCheckKind = basecfg.CondCheckKind

// CondCheck is an alias for basecfg.CondCheck.
type CondCheck = basecfg.CondCheck

// Condition check kind constants.
const (
	CheckNone      = basecfg.CheckNone
	CheckTruthy    = basecfg.CheckTruthy
	CheckFalsy     = basecfg.CheckFalsy
	CheckNil       = basecfg.CheckNil
	CheckNotNil    = basecfg.CheckNotNil
	CheckLimit     = basecfg.CheckLimit
	CheckTypeEqual = basecfg.CheckTypeEqual
	CheckTypeNot   = basecfg.CheckTypeNot
)

// TargetKind identifies the kind of assignment target.
type TargetKind uint8

// Target kind constants.
const (
	TargetIdent TargetKind = iota // Simple identifier: x
	TargetField                   // Field access: x.y
	TargetIndex                   // Index access: x[k]
)

// AssignTarget represents a single assignment target with pre-extracted info.
type AssignTarget struct {
	Kind TargetKind

	// For TargetIdent: the variable name and symbol
	Name   string           // Variable name
	Symbol basecfg.SymbolID // Unique symbol ID (0 if unresolved)

	// For TargetField: base object and field chain
	// For TargetIndex: base object if it is an identifier
	BaseName   string           // "x" in x.y or x[k]
	BaseSymbol basecfg.SymbolID // Symbol ID for base variable
	FieldPath  []string         // ["y", "z"] in x.y.z

	// For TargetIndex: the base expression and key
	Base ast.Expr // x in x[k]
	Key  ast.Expr // k in x[k]

	// Raw LHS expression for complex cases
	Expr ast.Expr
}

// AssignInfo captures pre-extracted data for assignment nodes.
type AssignInfo struct {
	IsLocal bool     // local x = ... vs x = ...
	Stmt    ast.Stmt // original AST statement (for position info)

	Targets []AssignTarget
	// Inline backing for the common single-target assignment case.
	// Keeps Targets behavior while avoiding a per-node heap allocation.
	singleTarget  [1]AssignTarget
	Sources       []ast.Expr
	SourceNames   []string           // Pre-extracted: identifier name or "" if not ident
	SourceSymbols []basecfg.SymbolID // Symbol IDs for source expressions (0 if not ident or unresolved)

	// Pre-extracted CallInfo for each source that is a function call (nil otherwise)
	SourceCalls []*CallInfo

	// Type annotations (for local assignments)
	TypeAnnotations []ast.TypeExpr
	// Inline backing for the common single-target annotation case.
	// Keeps TypeAnnotations behavior while avoiding a per-node heap allocation.
	singleTypeAnnotation [1]ast.TypeExpr

	// For generic for: iterator expressions
	IterExprs []ast.Expr

	// For numeric for
	NumericFor *NumericForInfo

	// SSA versions assigned by this node (one per target, may be zero if not yet computed)
	TargetVersions []Version
	// Inline backing for the common single-target assignment case.
	// Keeps TargetVersions behavior while avoiding a per-node heap allocation.
	singleTargetVersion [1]Version
}

func (*AssignInfo) nodeInfo() {}

// Kind returns the node kind for AssignInfo.
func (*AssignInfo) Kind() basecfg.NodeKind { return basecfg.NodeAssign }

// TargetAt returns the assignment target aligned with target index i.
func (info *AssignInfo) TargetAt(i int) (AssignTarget, bool) {
	if info == nil || i < 0 || i >= len(info.Targets) {
		return AssignTarget{}, false
	}
	return info.Targets[i], true
}

// FirstTarget returns the first assignment target when present.
func (info *AssignInfo) FirstTarget() (AssignTarget, bool) {
	return info.TargetAt(0)
}

// SourceCallAt returns the source call info aligned with source index i.
func (info *AssignInfo) SourceCallAt(i int) *CallInfo {
	if info == nil || i < 0 || i >= len(info.SourceCalls) {
		return nil
	}
	return info.SourceCalls[i]
}

// SourceAt returns the source expression aligned with source index i.
func (info *AssignInfo) SourceAt(i int) ast.Expr {
	if info == nil || i < 0 || i >= len(info.Sources) {
		return nil
	}
	return info.Sources[i]
}

// LastSource returns the last source expression, or nil when no sources exist.
func (info *AssignInfo) LastSource() ast.Expr {
	if info == nil || len(info.Sources) == 0 {
		return nil
	}
	return info.Sources[len(info.Sources)-1]
}

// TypeAnnotationAt returns the type annotation aligned with target index i.
func (info *AssignInfo) TypeAnnotationAt(i int) ast.TypeExpr {
	if info == nil || i < 0 || i >= len(info.TypeAnnotations) {
		return nil
	}
	return info.TypeAnnotations[i]
}

// CallForTarget resolves the source call producing target index i.
//
// Lua assignment rule: when there are more targets than sources, the final
// source expression may expand to multiple values. If that final source is a
// call expression, it supplies trailing target values.
func (info *AssignInfo) CallForTarget(i int) (*CallInfo, int) {
	if info == nil || i < 0 {
		return nil, 0
	}
	if call := info.SourceCallAt(i); call != nil {
		return call, 0
	}
	last := len(info.SourceCalls) - 1
	if last < 0 || i < last {
		return nil, 0
	}
	call := info.SourceCallAt(last)
	if call == nil {
		return nil, 0
	}
	return call, i - last
}

// SingleSourceCall returns the call info when this assignment has exactly one
// call expression source (directly at source index 0). Returns nil otherwise.
func (info *AssignInfo) SingleSourceCall() *CallInfo {
	if info == nil || len(info.SourceCalls) != 1 {
		return nil
	}
	return info.SourceCallAt(0)
}

// ExpandingSourceCall returns the trailing source call that expands across
// multiple targets, along with the first target index fed by that call.
//
// Lua assignment semantics only allow multi-return expansion from the last
// source expression. This helper encodes that rule and requires at least two
// targets to be populated by the expanding call.
func (info *AssignInfo) ExpandingSourceCall() (*CallInfo, int) {
	if info == nil || len(info.Targets) < 2 || len(info.Sources) == 0 {
		return nil, 0
	}
	lastSource := len(info.Sources) - 1
	call := info.SourceCallAt(lastSource)
	if call == nil {
		return nil, 0
	}
	expandedTargets := len(info.Targets) - lastSource
	if expandedTargets < 2 {
		return nil, 0
	}
	return call, lastSource
}

// HasSiblingCallExpansion reports whether assignment can produce sibling
// correlations from a single call expression expanded across multiple targets.
func (info *AssignInfo) HasSiblingCallExpansion() bool {
	call, _ := info.ExpandingSourceCall()
	return call != nil
}

// EachSource visits non-nil source expressions in source index order.
func (info *AssignInfo) EachSource(fn func(i int, src ast.Expr)) {
	if info == nil || fn == nil {
		return
	}
	for i, src := range info.Sources {
		if src != nil {
			fn(i, src)
		}
	}
}

// EachTarget visits targets in target index order.
func (info *AssignInfo) EachTarget(fn func(i int, target AssignTarget)) {
	if info == nil || fn == nil {
		return
	}
	for i := range info.Targets {
		if target, ok := info.TargetAt(i); ok {
			fn(i, target)
		}
	}
}

// EachTargetSource visits targets in target index order with their aligned
// source expression. Source may be nil when no source exists at that index.
//
// This preserves raw positional alignment only; it does not apply Lua
// multi-return expansion semantics for trailing targets.
func (info *AssignInfo) EachTargetSource(fn func(i int, target AssignTarget, src ast.Expr)) {
	if info == nil || fn == nil {
		return
	}
	sources := info.Sources
	for i, target := range info.Targets {
		var src ast.Expr
		if i < len(sources) {
			src = sources[i]
		}
		fn(i, target, src)
	}
}

// EachSourceCall visits non-nil source call infos in source index order.
func (info *AssignInfo) EachSourceCall(fn func(i int, call *CallInfo)) {
	if info == nil || fn == nil {
		return
	}
	for i, call := range info.SourceCalls {
		if call != nil {
			fn(i, call)
		}
	}
}

// NumericForInfo captures numeric for loop details.
type NumericForInfo struct {
	VarName string
	Init    ast.Expr
	Limit   ast.Expr
	Step    ast.Expr
}

// CallInfo captures pre-extracted data for call nodes.
type CallInfo struct {
	Call     *ast.FuncCallExpr // Full call expression (for span lookup)
	Callee   ast.Expr          // Function expression (for non-method calls)
	Args     []ast.Expr        // Call arguments
	Method   string            // "foo" if x:foo()
	Receiver ast.Expr          // x if x:foo()
	IsStmt   bool              // Statement vs expression

	// Pre-extracted for quick checks
	CalleeName     string             // "error", "assert", "setmetatable" etc
	CalleeSymbol   basecfg.SymbolID   // Symbol ID for callee (0 if unresolved or not ident)
	ArgNames       []string           // Pre-extracted: identifier name or "" if not ident
	ArgSymbols     []basecfg.SymbolID // Symbol IDs for arguments (0 if not ident or unresolved)
	ReceiverName   string             // Pre-extracted: identifier name or "" if not ident
	ReceiverSymbol basecfg.SymbolID   // Symbol ID for receiver (0 if unresolved)

	// Pre-extracted predicate pattern info (for Type:is(x) or TypeName(x))
	IsTypeCheck   bool            // True if this is Type:is(arg) or TypeName(arg) pattern
	TypeCheckName string          // Type name for type check (receiver name for :is, callee name otherwise)
	TypeCheckPath constraint.Path // Binding-based path for type check argument (identity + display)

	// Unified callee path for identity resolution.
	// For f() -> {Root: "f", Symbol: sym}
	// For obj.f() -> {Root: "obj", Symbol: objSym, Segments: [{Field, "f"}]}
	// For obj:f() -> receiver path only: {Root: "obj", Symbol: objSym}
	//   Method name is in CallInfo.Method field, not in CalleePath
	// For a.b.c() -> {Root: "a", Symbol: aSym, Segments: [{Field, "b"}, {Field, "c"}]}
	// For a.b:c() -> receiver path: {Root: "a", Symbol: aSym, Segments: [{Field, "b"}]}
	// Empty if callee cannot be statically resolved to a path.
	CalleePath constraint.Path
}

func (*CallInfo) nodeInfo() {}

// Kind returns the node kind for CallInfo.
func (*CallInfo) Kind() basecfg.NodeKind { return basecfg.NodeCall }

// ReturnInfo captures pre-extracted data for return nodes.
type ReturnInfo struct {
	Stmt        *ast.ReturnStmt
	Exprs       []ast.Expr
	Names       []string           // Pre-extracted: identifier name or "" if not ident
	Symbols     []basecfg.SymbolID // Symbol IDs for returned identifiers (0 if not ident or unresolved)
	SourceCalls []*CallInfo        // Call info for returned call expressions (parallel to Exprs; nil if not a call)
}

func (*ReturnInfo) nodeInfo() {}

// Kind returns the node kind for ReturnInfo.
func (*ReturnInfo) Kind() basecfg.NodeKind { return basecfg.NodeReturn }

// SourceCallAt returns the return-expression call info aligned with expression index i.
func (info *ReturnInfo) SourceCallAt(i int) *CallInfo {
	if info == nil || i < 0 || i >= len(info.SourceCalls) {
		return nil
	}
	return info.SourceCalls[i]
}

// EachSourceCall visits non-nil return-expression call infos in expression order.
func (info *ReturnInfo) EachSourceCall(fn func(i int, call *CallInfo)) {
	if info == nil || fn == nil {
		return
	}
	for i := range info.SourceCalls {
		if call := info.SourceCallAt(i); call != nil {
			fn(i, call)
		}
	}
}

// BranchInfo captures pre-extracted data for branch nodes.
type BranchInfo struct {
	CondVar    string    // Variable being tested (path string)
	CondSymbol SymbolID  // Symbol ID of condition variable root (0 if unresolved)
	CondCheck  CondCheck // Condition check type
	Condition  ast.Expr  // Full condition expression
}

func (*BranchInfo) nodeInfo() {}

// Kind returns the node kind for BranchInfo.
func (*BranchInfo) Kind() basecfg.NodeKind { return basecfg.NodeBranch }

// TypeDefInfo captures pre-extracted data for type definition nodes.
type TypeDefInfo struct {
	Name       string
	TypeParams []TypeParamInfo
	TypeExpr   ast.TypeExpr
}

func (*TypeDefInfo) nodeInfo() {}

// Kind returns the node kind for TypeDefInfo.
func (*TypeDefInfo) Kind() basecfg.NodeKind { return basecfg.NodeTypeDef }

// TypeParamInfo captures a type parameter definition.
type TypeParamInfo struct {
	Name       string
	Constraint ast.TypeExpr
}

// FuncDefTargetKind identifies how the function is being assigned.
type FuncDefTargetKind uint8

// Function definition target kind constants.
const (
	FuncDefGlobal FuncDefTargetKind = iota // function foo()
	FuncDefField                           // function T.foo()
	FuncDefMethod                          // function T:foo()
)

// FuncDefInfo captures pre-extracted data for function definition nodes.
type FuncDefInfo struct {
	TargetKind     FuncDefTargetKind
	Name           string           // Function name
	Symbol         basecfg.SymbolID // Symbol ID for function (0 if unresolved)
	Receiver       ast.Expr         // T in T:bar() or T.baz()
	ReceiverName   string           // Receiver identifier name if available
	ReceiverSymbol basecfg.SymbolID // Symbol ID for receiver (0 if unresolved)
	IsMethod       bool             // Uses : syntax
	FuncExpr       *ast.FunctionExpr

	// Unified target path for where the function is stored.
	// For function foo() -> {Root: "foo", Symbol: sym}
	// For local function foo() -> {Root: "foo", Symbol: sym}
	// For function M.foo() -> {Root: "M", Symbol: MSym, Segments: [{Field, "foo"}]}
	// For function M:foo() -> same as M.foo (method semantics separate via IsMethod)
	TargetPath constraint.Path
}

func (*FuncDefInfo) nodeInfo() {}

// Kind returns the node kind for FuncDefInfo.
func (*FuncDefInfo) Kind() basecfg.NodeKind { return basecfg.NodeAssign }

// NestedFunc records a nested function found during CFG build.
type NestedFunc struct {
	Point  Point
	Func   *ast.FunctionExpr
	Symbol basecfg.SymbolID // Symbol for the function literal (0 if not yet assigned)
}

// Compile-time interface checks.
var (
	_ NodeInfo = (*AssignInfo)(nil)
	_ NodeInfo = (*CallInfo)(nil)
	_ NodeInfo = (*ReturnInfo)(nil)
	_ NodeInfo = (*BranchInfo)(nil)
	_ NodeInfo = (*TypeDefInfo)(nil)
	_ NodeInfo = (*FuncDefInfo)(nil)
)
