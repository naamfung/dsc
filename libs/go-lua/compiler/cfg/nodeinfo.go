// Package cfg provides CFG construction utilities.
package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// ExtractAssignTarget extracts assignment target info from an expression.
func ExtractAssignTarget(expr ast.Expr) AssignTarget {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return AssignTarget{Kind: TargetIdent, Name: e.Value, Expr: expr}
	case *ast.AttrGetExpr:
		baseName, fields := extraction.ExtractFieldPath(e)
		if baseName != "" && len(fields) > 0 {
			return AssignTarget{
				Kind:      TargetField,
				BaseName:  baseName,
				FieldPath: fields,
				Expr:      expr,
			}
		}
		return AssignTarget{Kind: TargetIndex, Base: e.Object, Key: e.Key, Expr: expr}
	default:
		return AssignTarget{Kind: TargetIndex, Expr: expr}
	}
}

// BuildCallInfo creates a CallInfo from a FuncCallExpr, extracting all needed info.
func BuildCallInfo(call *ast.FuncCallExpr, isStmt bool) *CallInfo {
	if call == nil {
		return nil
	}
	info := &CallInfo{
		Call:         call,
		Callee:       call.Func,
		Args:         call.Args,
		Method:       call.Method,
		Receiver:     call.Receiver,
		IsStmt:       isStmt,
		CalleeName:   extraction.IdentName(call.Func),
		ReceiverName: extraction.IdentName(call.Receiver),
	}
	if info.CalleeName == "" && info.Method != "" {
		if isStaticReceiver(call.Receiver) {
			info.CalleeName = info.Method
		}
	}
	if info.CalleeName == "" {
		info.CalleeName = staticFieldCalleeName(call.Func)
	}
	info.ArgNames = extractIdentNames(call.Args)

	ExtractTypeCheckPattern(info)

	return info
}

// ExtractTypeCheckPattern detects Type:is(x) or TypeName(x) patterns.
func ExtractTypeCheckPattern(info *CallInfo) {
	if info == nil {
		return
	}

	if info.Method == "is" && info.ReceiverName != "" && len(info.Args) > 0 {
		info.IsTypeCheck = true
		info.TypeCheckName = info.ReceiverName

		return
	}

	if info.Method == "" && info.Receiver == nil && info.CalleeName != "" && len(info.Args) > 0 {
		info.IsTypeCheck = true
		info.TypeCheckName = info.CalleeName
	}
}

// ExtractSourceCalls pre-extracts CallInfo for each source expression that is a call.
func ExtractSourceCalls(exprs []ast.Expr) []*CallInfo {
	if len(exprs) == 0 {
		return nil
	}

	var calls []*CallInfo

	for i, expr := range exprs {
		if call, ok := expr.(*ast.FuncCallExpr); ok {
			if calls == nil {
				calls = make([]*CallInfo, len(exprs))
			}
			calls[i] = BuildCallInfo(call, false)
		}
	}

	return calls
}

// extractIdentNames returns identifier names aligned to expr indexes.
//
// It lazily allocates the output slice only when at least one expression is
// an identifier; callers get nil when no identifiers are present.
func extractIdentNames(exprs []ast.Expr) []string {
	if len(exprs) == 0 {
		return nil
	}

	var names []string

	for i, expr := range exprs {
		name := extraction.IdentName(expr)
		if name == "" {
			continue
		}
		if names == nil {
			names = make([]string, len(exprs))
		}
		names[i] = name
	}

	return names
}

func isStaticReceiver(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value != ""
	case *ast.AttrGetExpr:
		base, fields := extractStaticFieldPath(e)
		return base != "" && len(fields) > 0
	default:
		return false
	}
}

func staticFieldCalleeName(expr ast.Expr) string {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return ""
	}
	_, fields := extractStaticFieldPath(attr)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func extractStaticFieldPath(expr *ast.AttrGetExpr) (string, []string) {
	var path []string
	current := expr

	for {
		key := ast.KeyName(current.Key)
		if key == "" || !pathkey.IsIdentName(key) {
			return "", nil
		}

		path = append([]string{key}, path...)

		switch obj := current.Object.(type) {
		case *ast.IdentExpr:
			return obj.Value, path
		case *ast.AttrGetExpr:
			current = obj
		default:
			return "", nil
		}
	}
}
