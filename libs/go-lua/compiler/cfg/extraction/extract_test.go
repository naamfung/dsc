package extraction

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func TestExtractFieldPath(t *testing.T) {
	tests := []struct {
		name       string
		expr       *ast.AttrGetExpr
		wantBase   string
		wantFields []string
	}{
		{
			name: "single field",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "x"},
				Key:    &ast.StringExpr{Value: "y"},
			},
			wantBase:   "x",
			wantFields: []string{"y"},
		},
		{
			name: "nested fields",
			expr: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
			wantBase:   "a",
			wantFields: []string{"b", "c"},
		},
		{
			name: "non-string key",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "x"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
			wantBase:   "",
			wantFields: nil,
		},
		{
			name: "non-ident object",
			expr: &ast.AttrGetExpr{
				Object: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
				Key:    &ast.StringExpr{Value: "y"},
			},
			wantBase:   "",
			wantFields: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, fields := ExtractFieldPath(tt.expr)
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if len(fields) != len(tt.wantFields) {
				t.Errorf("fields = %v, want %v", fields, tt.wantFields)
			}
		})
	}
}

func TestExtractCondition(t *testing.T) {
	tests := []struct {
		name      string
		expr      ast.Expr
		wantVar   string
		wantCheck basecfg.CondCheck
	}{
		{
			name:      "simple ident",
			expr:      &ast.IdentExpr{Value: "x"},
			wantVar:   "x",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckTruthy},
		},
		{
			name: "nil check ==",
			expr: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs:      &ast.IdentExpr{Value: "y"},
				Rhs:      &ast.NilExpr{},
			},
			wantVar:   "y",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckNil},
		},
		{
			name: "nil check ~=",
			expr: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs:      &ast.IdentExpr{Value: "z"},
				Rhs:      &ast.NilExpr{},
			},
			wantVar:   "z",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckNotNil},
		},
		{
			name: "nil on left ==",
			expr: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs:      &ast.NilExpr{},
				Rhs:      &ast.IdentExpr{Value: "w"},
			},
			wantVar:   "w",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckNil},
		},
		{
			name: "nil on left ~=",
			expr: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs:      &ast.NilExpr{},
				Rhs:      &ast.IdentExpr{Value: "v"},
			},
			wantVar:   "v",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckNotNil},
		},
		{
			name:      "not operator",
			expr:      &ast.UnaryNotOpExpr{Expr: &ast.IdentExpr{Value: "a"}},
			wantVar:   "a",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckFalsy},
		},
		{
			name:      "not with non-path",
			expr:      &ast.UnaryNotOpExpr{Expr: &ast.NumberExpr{Value: "1"}},
			wantVar:   "",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckFalsy},
		},
		{
			name: "attr get truthy",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "obj"},
				Key:    &ast.StringExpr{Value: "field"},
			},
			wantVar:   "obj.field",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckTruthy},
		},
		{
			name: "type check",
			expr: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
				Rhs: &ast.StringExpr{Value: "string"},
			},
			wantVar:   "x",
			wantCheck: basecfg.CondCheck{Kind: basecfg.CheckTypeEqual, TypeName: "string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			varName, check := ExtractCondition(tt.expr)
			if varName != tt.wantVar {
				t.Errorf("varName = %q, want %q", varName, tt.wantVar)
			}
			if check.Kind != tt.wantCheck.Kind || check.TypeName != tt.wantCheck.TypeName {
				t.Errorf("check = %+v, want %+v", check, tt.wantCheck)
			}
		})
	}
}

func TestExtractTypeCheckInfo(t *testing.T) {
	tests := []struct {
		name     string
		expr     *ast.RelationalOpExpr
		wantPath string
		wantType string
		wantNot  bool
		wantOK   bool
	}{
		{
			name: "type on left ==",
			expr: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
				Rhs: &ast.StringExpr{Value: "table"},
			},
			wantPath: "x",
			wantType: "table",
			wantNot:  false,
			wantOK:   true,
		},
		{
			name: "type on right ==",
			expr: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs:      &ast.StringExpr{Value: "function"},
				Rhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "f"}},
				},
			},
			wantPath: "f",
			wantType: "function",
			wantNot:  false,
			wantOK:   true,
		},
		{
			name: "type ~= (negated)",
			expr: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "y"}},
				},
				Rhs: &ast.StringExpr{Value: "nil"},
			},
			wantPath: "y",
			wantType: "nil",
			wantNot:  true,
			wantOK:   true,
		},
		{
			name: "invalid operator",
			expr: &ast.RelationalOpExpr{
				Operator: "<",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
				Rhs: &ast.StringExpr{Value: "number"},
			},
			wantPath: "",
			wantType: "",
			wantNot:  false,
			wantOK:   false,
		},
		{
			name:     "nil expression",
			expr:     nil,
			wantPath: "",
			wantType: "",
			wantNot:  false,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, typeName, isNot, ok := ExtractTypeCheckInfo(tt.expr)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if typeName != tt.wantType {
				t.Errorf("typeName = %q, want %q", typeName, tt.wantType)
			}
			if isNot != tt.wantNot {
				t.Errorf("isNot = %v, want %v", isNot, tt.wantNot)
			}
		})
	}
}

func TestIsTypeofCall(t *testing.T) {
	tests := []struct {
		name string
		call *ast.FuncCallExpr
		want bool
	}{
		{
			name: "valid type call",
			call: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "type"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
			want: true,
		},
		{
			name: "wrong function name",
			call: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "typeof"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
			want: false,
		},
		{
			name: "method call",
			call: &ast.FuncCallExpr{
				Receiver: &ast.IdentExpr{Value: "obj"},
				Method:   "type",
				Args:     []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
			want: false,
		},
		{
			name: "no args",
			call: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "type"},
				Args: []ast.Expr{},
			},
			want: false,
		},
		{
			name: "multiple args",
			call: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "type"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "x"}, &ast.IdentExpr{Value: "y"}},
			},
			want: false,
		},
		{
			name: "nil call",
			call: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTypeofCall(tt.call)
			if got != tt.want {
				t.Errorf("IsTypeofCall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCalleeName(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "simple call",
			expr: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "print"},
			},
			want: "print",
		},
		{
			name: "method call",
			expr: &ast.FuncCallExpr{
				Receiver: &ast.IdentExpr{Value: "obj"},
				Method:   "foo",
			},
			want: "",
		},
		{
			name: "complex callee",
			expr: &ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "mod"},
					Key:    &ast.StringExpr{Value: "func"},
				},
			},
			want: "",
		},
		{
			name: "not a call",
			expr: &ast.IdentExpr{Value: "x"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCalleeName(tt.expr)
			if got != tt.want {
				t.Errorf("ExtractCalleeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathFromExpr(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "simple ident",
			expr: &ast.IdentExpr{Value: "x"},
			want: "x",
		},
		{
			name: "field access",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "obj"},
				Key:    &ast.StringExpr{Value: "field"},
			},
			want: "obj.field",
		},
		{
			name: "nested field",
			expr: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "a"},
					Key:    &ast.StringExpr{Value: "b"},
				},
				Key: &ast.StringExpr{Value: "c"},
			},
			want: "a.b.c",
		},
		{
			name: "numeric index",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "arr"},
				Key:    &ast.NumberExpr{Value: "1"},
			},
			want: "arr[1]",
		},
		{
			name: "string key with special chars",
			expr: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "obj"},
				Key:    &ast.StringExpr{Value: "key-with-dash"},
			},
			want: `obj["key-with-dash"]`,
		},
		{
			name: "non-path expr",
			expr: &ast.NumberExpr{Value: "42"},
			want: "",
		},
		{
			name: "call expr",
			expr: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PathFromExpr(tt.expr)
			if got != tt.want {
				t.Errorf("PathFromExpr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssignedOuterNamesInBlock(t *testing.T) {
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "y"}, &ast.IdentExpr{Value: "z"}},
			Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}, &ast.NumberExpr{Value: "3"}},
		},
	}

	names := AssignedOuterNamesInBlock(stmts)

	if len(names) != 3 {
		t.Errorf("Expected 3 names, got %d: %v", len(names), names)
	}
	if names[0] != "x" || names[1] != "y" || names[2] != "z" {
		t.Errorf("Names should be sorted [x, y, z], got %v", names)
	}
}

func TestAssignedOuterNamesInBlock_Empty(t *testing.T) {
	names := AssignedOuterNamesInBlock(nil)
	if names != nil {
		t.Error("Empty block should return nil")
	}

	names = AssignedOuterNamesInBlock([]ast.Stmt{})
	if names != nil {
		t.Error("Empty block should return nil")
	}
}

func TestAssignedOuterNamesInBlock_Nested(t *testing.T) {
	stmts := []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.TrueExpr{},
			Then: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.IdentExpr{Value: "a"}},
					Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
				},
			},
			Else: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.IdentExpr{Value: "b"}},
					Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
				},
			},
		},
		&ast.WhileStmt{
			Condition: &ast.TrueExpr{},
			Stmts: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.IdentExpr{Value: "c"}},
					Rhs: []ast.Expr{&ast.NumberExpr{Value: "3"}},
				},
			},
		},
	}

	names := AssignedOuterNamesInBlock(stmts)

	if len(names) != 3 {
		t.Errorf("Expected 3 names from nested stmts, got %d: %v", len(names), names)
	}
}

func TestAssignedOuterNamesInBlock_FuncDef(t *testing.T) {
	stmts := []ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{
				Func: &ast.IdentExpr{Value: "myFunc"},
			},
			Func: &ast.FunctionExpr{},
		},
	}

	names := AssignedOuterNamesInBlock(stmts)

	if len(names) != 1 || names[0] != "myFunc" {
		t.Errorf("Expected [myFunc], got %v", names)
	}
}

func TestAssignedOuterIdentsInBlock(t *testing.T) {
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{
				&ast.IdentExpr{Value: "y"},
				&ast.IdentExpr{Value: "z"},
			},
			Rhs: []ast.Expr{
				&ast.NumberExpr{Value: "2"},
				&ast.NumberExpr{Value: "3"},
			},
		},
	}

	idents := AssignedOuterIdentsInBlock(stmts)

	if len(idents) != 3 {
		t.Errorf("Expected 3 idents, got %d", len(idents))
	}
	if idents[0].Value != "x" || idents[1].Value != "y" || idents[2].Value != "z" {
		t.Errorf("Idents not sorted correctly: %v", idents)
	}
}

func TestAssignedOuterIdentsInBlock_Dedup(t *testing.T) {
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
		},
	}

	idents := AssignedOuterIdentsInBlock(stmts)

	if len(idents) != 1 {
		t.Errorf("Expected 1 ident (deduplicated), got %d", len(idents))
	}
}

func TestIsIdentName(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"x", true},
		{"_foo", true},
		{"foo123", true},
		{"FooBar", true},
		{"_", true},
		{"_123", true},
		{"", false},
		{"123", false},
		{"123abc", false},
		{"foo-bar", false},
		{"foo.bar", false},
		{"foo bar", false},
	}

	for _, tt := range tests {
		got := pathkey.IsIdentName(tt.s)
		if got != tt.want {
			t.Errorf("IsIdentName(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		s       string
		wantVal string
		wantOK  bool
	}{
		{"123", "123", true},
		{"0", "0", true},
		{"999999", "999999", true},
		{"", "", false},
		{"12.3", "", false},
		{"abc", "", false},
		{"-1", "", false},
		{"1e5", "", false},
	}

	for _, tt := range tests {
		val, ok := ParseInt(tt.s)
		if ok != tt.wantOK {
			t.Errorf("ParseInt(%q) ok = %v, want %v", tt.s, ok, tt.wantOK)
		}
		if val != tt.wantVal {
			t.Errorf("ParseInt(%q) val = %q, want %q", tt.s, val, tt.wantVal)
		}
	}
}

func TestEscapePathKey(t *testing.T) {
	tests := []struct {
		s    string
		want string
	}{
		{"simple", "simple"},
		{`with"quote`, `with\"quote`},
		{`with\backslash`, `with\\backslash`},
		{`both"and\`, `both\"and\\`},
		{"", ""},
	}

	for _, tt := range tests {
		got := EscapePathKey(tt.s)
		if got != tt.want {
			t.Errorf("EscapePathKey(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestIdentName(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "ident",
			expr: &ast.IdentExpr{Value: "x"},
			want: "x",
		},
		{
			name: "number",
			expr: &ast.NumberExpr{Value: "42"},
			want: "",
		},
		{
			name: "nil expr",
			expr: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IdentName(tt.expr)
			if got != tt.want {
				t.Errorf("IdentName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractRootName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"x", "x"},
		{"x.y", "x"},
		{"x.y.z", "x"},
		{"x[1]", "x"},
		{"obj[\"key\"]", "obj"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExtractRootName(tt.path)
		if got != tt.want {
			t.Errorf("ExtractRootName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
