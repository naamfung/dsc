// Package extraction provides pure AST extraction functions for CFG construction.
package extraction

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// ExtractFieldPath extracts base name and field chain from an AttrGetExpr.
// For x.y.z it returns ("x", ["y", "z"]). Returns ("", nil) if not a static field path.
func ExtractFieldPath(expr *ast.AttrGetExpr) (string, []string) {
	var path []string

	current := expr

	for {
		var fieldName string
		switch key := current.Key.(type) {
		case *ast.StringExpr:
			if pathkey.IsIdentName(key.Value) {
				fieldName = key.Value
			}
		}

		if fieldName == "" {
			return "", nil
		}

		path = append([]string{fieldName}, path...)

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

// ExtractCondition extracts variable name and check type from a condition expression.
// Returns (varName, condCheck) where condCheck describes the type of check performed.
func ExtractCondition(expr ast.Expr) (varName string, check basecfg.CondCheck) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value, basecfg.CondCheck{Kind: basecfg.CheckTruthy}
	case *ast.AttrGetExpr:
		if path := PathFromExpr(e); path != "" {
			return path, basecfg.CondCheck{Kind: basecfg.CheckTruthy}
		}
	case *ast.RelationalOpExpr:
		if varName, typeName, isNot, ok := ExtractTypeCheckInfo(e); ok {
			if isNot {
				return varName, basecfg.CondCheck{Kind: basecfg.CheckTypeNot, TypeName: typeName}
			}

			return varName, basecfg.CondCheck{Kind: basecfg.CheckTypeEqual, TypeName: typeName}
		}

		if lhs := PathFromExpr(e.Lhs); lhs != "" {
			if _, ok := e.Rhs.(*ast.NilExpr); ok {
				switch e.Operator {
				case "==":
					return lhs, basecfg.CondCheck{Kind: basecfg.CheckNil}
				case "~=":
					return lhs, basecfg.CondCheck{Kind: basecfg.CheckNotNil}
				}
			}
		}

		if _, ok := e.Lhs.(*ast.NilExpr); ok {
			if rhs := PathFromExpr(e.Rhs); rhs != "" {
				switch e.Operator {
				case "==":
					return rhs, basecfg.CondCheck{Kind: basecfg.CheckNil}
				case "~=":
					return rhs, basecfg.CondCheck{Kind: basecfg.CheckNotNil}
				}
			}
		}
	case *ast.UnaryNotOpExpr:
		if path := PathFromExpr(e.Expr); path != "" {
			return path, basecfg.CondCheck{Kind: basecfg.CheckFalsy}
		}
		return "", basecfg.CondCheck{Kind: basecfg.CheckFalsy}
	}
	return "", basecfg.CondCheck{Kind: basecfg.CheckTruthy}
}

// ExtractTypeCheckInfo extracts type check info from type(x) == "typename" patterns.
func ExtractTypeCheckInfo(e *ast.RelationalOpExpr) (argPath, typeName string, isNot, ok bool) {
	if e == nil {
		return "", "", false, false
	}

	if call, ok := e.Lhs.(*ast.FuncCallExpr); ok {
		if IsTypeofCall(call) {
			argPath = PathFromExpr(call.Args[0])
		}

		if str, ok := e.Rhs.(*ast.StringExpr); ok {
			typeName = str.Value
		}
	} else if call, ok := e.Rhs.(*ast.FuncCallExpr); ok {
		if IsTypeofCall(call) {
			argPath = PathFromExpr(call.Args[0])
		}

		if str, ok := e.Lhs.(*ast.StringExpr); ok {
			typeName = str.Value
		}
	}

	if argPath == "" || typeName == "" {
		return "", "", false, false
	}

	if e.Operator != "==" && e.Operator != "~=" {
		return "", "", false, false
	}

	return argPath, typeName, e.Operator == "~=", true
}

// IsTypeofCall returns true if the call is type(x) with exactly one argument.
func IsTypeofCall(call *ast.FuncCallExpr) bool {
	if !IsSingleArgCall(call) {
		return false
	}

	ident, ok := call.Func.(*ast.IdentExpr)

	return ok && ident.Value == "type"
}

// IsSingleArgCall returns true if the call is a non-method call with exactly one argument.
func IsSingleArgCall(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}

	return len(call.Args) == 1
}

// ExtractCalleeName extracts the function name from a call expression.
// Returns "" if the callee is not a simple identifier.
func ExtractCalleeName(expr ast.Expr) string {
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		if ident, ok := call.Func.(*ast.IdentExpr); ok {
			return ident.Value
		}
	}

	return ""
}

// PathFromExpr builds a path string from an expression.
// For x.y.z returns "x.y.z", for x["key"] returns "x[\"key\"]".
// Returns "" if the expression is not a valid path.
func PathFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		base := PathFromExpr(e.Object)
		if base == "" {
			return ""
		}

		switch key := e.Key.(type) {
		case *ast.StringExpr:
			if pathkey.IsIdentName(key.Value) {
				return base + "." + key.Value
			}

			return base + "[\"" + EscapePathKey(key.Value) + "\"]"
		case *ast.NumberExpr:
			if idx, ok := ParseInt(key.Value); ok {
				return base + "[" + idx + "]"
			}
		}
	}

	return ""
}

// AssignedOuterNamesInBlock collects names of variables assigned (not declared) in a block.
// Used to identify loop-modified variables.
func AssignedOuterNamesInBlock(stmts []ast.Stmt) []string {
	if len(stmts) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	CollectAssignedOuterNames(stmts, seen)

	if len(seen) == 0 {
		return nil
	}

	names := make([]string, 0, len(seen))

	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// AssignedOuterIdentsInBlock collects ident expressions for variables assigned (not declared) in a block.
// Returns deduplicated idents sorted by name, suitable for binding lookup.
func AssignedOuterIdentsInBlock(stmts []ast.Stmt) []*ast.IdentExpr {
	if len(stmts) == 0 {
		return nil
	}

	seen := make(map[string]*ast.IdentExpr)
	collectAssignedOuterIdents(stmts, seen)

	if len(seen) == 0 {
		return nil
	}

	idents := make([]*ast.IdentExpr, 0, len(seen))

	for _, ident := range seen {
		idents = append(idents, ident)
	}

	sort.Slice(idents, func(i, j int) bool {
		return idents[i].Value < idents[j].Value
	})

	return idents
}

// collectAssignedOuterIdents recursively collects assigned (not local) variable idents.
func collectAssignedOuterIdents(stmts []ast.Stmt, seen map[string]*ast.IdentExpr) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.IdentExpr); ok {
					if _, exists := seen[ident.Value]; !exists {
						seen[ident.Value] = ident
					}
				}
			}
		case *ast.FuncDefStmt:
			if s.Name != nil && s.Name.Func != nil && s.Name.Method == "" {
				if ident, ok := s.Name.Func.(*ast.IdentExpr); ok {
					if _, exists := seen[ident.Value]; !exists {
						seen[ident.Value] = ident
					}
				}
			}

			if s.Name != nil && s.Name.Receiver != nil && s.Name.Method != "" {
				if ident, ok := s.Name.Receiver.(*ast.IdentExpr); ok {
					if _, exists := seen[ident.Value]; !exists {
						seen[ident.Value] = ident
					}
				}
			}
		case *ast.NumberForStmt:
			collectAssignedOuterIdents(s.Stmts, seen)
		case *ast.GenericForStmt:
			collectAssignedOuterIdents(s.Stmts, seen)
		case *ast.IfStmt:
			collectAssignedOuterIdents(s.Then, seen)
			collectAssignedOuterIdents(s.Else, seen)
		case *ast.WhileStmt:
			collectAssignedOuterIdents(s.Stmts, seen)
		case *ast.RepeatStmt:
			collectAssignedOuterIdents(s.Stmts, seen)
		case *ast.DoBlockStmt:
			collectAssignedOuterIdents(s.Stmts, seen)
		}
	}
}

// CollectAssignedOuterNames recursively collects assigned (not local) variable names.
func CollectAssignedOuterNames(stmts []ast.Stmt, seen map[string]struct{}) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.IdentExpr); ok {
					seen[ident.Value] = struct{}{}
				}
			}
		case *ast.FuncDefStmt:
			if s.Name != nil && s.Name.Func != nil && s.Name.Method == "" {
				if ident, ok := s.Name.Func.(*ast.IdentExpr); ok {
					seen[ident.Value] = struct{}{}
				}
			}

			if s.Name != nil && s.Name.Receiver != nil && s.Name.Method != "" {
				if ident, ok := s.Name.Receiver.(*ast.IdentExpr); ok {
					seen[ident.Value] = struct{}{}
				}
			}
		case *ast.NumberForStmt:
			CollectAssignedOuterNames(s.Stmts, seen)
		case *ast.GenericForStmt:
			CollectAssignedOuterNames(s.Stmts, seen)
		case *ast.IfStmt:
			CollectAssignedOuterNames(s.Then, seen)
			CollectAssignedOuterNames(s.Else, seen)
		case *ast.WhileStmt:
			CollectAssignedOuterNames(s.Stmts, seen)
		case *ast.RepeatStmt:
			CollectAssignedOuterNames(s.Stmts, seen)
		case *ast.DoBlockStmt:
			CollectAssignedOuterNames(s.Stmts, seen)
		}
	}
}

// ParseInt parses a simple integer string. Returns (value, true) on success.
func ParseInt(s string) (string, bool) {
	if s == "" {
		return "", false
	}

	for i := range len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return "", false
		}
	}

	return s, true
}

// EscapePathKey escapes backslashes and quotes in a path key.
func EscapePathKey(s string) string {
	out := make([]byte, 0, len(s))

	for i := range len(s) {
		ch := s[i]
		if ch == '\\' || ch == '"' {
			out = append(out, '\\')
		}

		out = append(out, ch)
	}

	return string(out)
}

// IdentName extracts identifier name from expression, or "" if not an identifier.
func IdentName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		return ident.Value
	}

	return ""
}

// ExtractRootName extracts the root identifier name from a path string.
func ExtractRootName(path string) string {
	for i := range len(path) {
		if path[i] == '.' || path[i] == '[' {
			return path[:i]
		}
	}

	return path
}
