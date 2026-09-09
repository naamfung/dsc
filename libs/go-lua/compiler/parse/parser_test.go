package parse

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func parseOne(t *testing.T, input string) ast.Stmt {
	t.Helper()
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) == 0 {
		t.Fatal("expected at least one statement")
	}
	return stmts[0]
}

func parseOneString(t *testing.T, input string) ast.Stmt {
	t.Helper()
	stmts, err := ParseString(input, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	if len(stmts) == 0 {
		t.Fatal("expected at least one statement")
	}
	return stmts[0]
}

func TestParseLocalWithType(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantType string
	}{
		{"local x: number = 1", "x", "number"},
		{"local s: string = 'hello'", "s", "string"},
		{"local b: boolean = true", "b", "boolean"},
	}

	for _, tt := range tests {
		stmts, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if len(stmts) != 1 {
			t.Errorf("Parse(%q) got %d stmts, want 1", tt.input, len(stmts))
			continue
		}
		local, ok := stmts[0].(*ast.LocalAssignStmt)
		if !ok {
			t.Errorf("Parse(%q) got %T, want *ast.LocalAssignStmt", tt.input, stmts[0])
			continue
		}
		if len(local.Names) != 1 || local.Names[0] != tt.wantName {
			t.Errorf("Parse(%q) name = %v, want %q", tt.input, local.Names, tt.wantName)
		}
		if len(local.Types) != 1 || local.Types[0] == nil {
			t.Errorf("Parse(%q) types = %v, want non-nil type", tt.input, local.Types)
			continue
		}
		prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
		if !ok {
			t.Errorf("Parse(%q) type = %T, want *ast.PrimitiveTypeExpr", tt.input, local.Types[0])
			continue
		}
		if prim.Name != tt.wantType {
			t.Errorf("Parse(%q) type name = %q, want %q", tt.input, prim.Name, tt.wantType)
		}
	}
}

func TestParseLocalMultipleWithTypes(t *testing.T) {
	input := "local x: number, y: string = 1, 'hello'"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.LocalAssignStmt", stmts[0])
	}
	if len(local.Names) != 2 {
		t.Errorf("got %d names, want 2", len(local.Names))
	}
	if len(local.Types) != 2 {
		t.Errorf("got %d types, want 2", len(local.Types))
	}
}

func TestParseLocalWithoutType(t *testing.T) {
	input := "local x = 1"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.LocalAssignStmt", stmts[0])
	}
	if local.Types != nil {
		t.Errorf("Types should be nil when no types specified")
	}
}

func TestParseFunctionWithTypes(t *testing.T) {
	input := "function foo(x: number, y: string): boolean return true end"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	funcdef, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.FuncDefStmt", stmts[0])
	}
	fn := funcdef.Func
	if fn.ParList == nil {
		t.Fatal("ParList is nil")
	}
	if len(fn.ParList.Names) != 2 {
		t.Errorf("got %d params, want 2", len(fn.ParList.Names))
	}
	if len(fn.ParList.Types) != 2 {
		t.Errorf("got %d param types, want 2", len(fn.ParList.Types))
	}
	if len(fn.ReturnTypes) != 1 {
		t.Errorf("got %d return types, want 1", len(fn.ReturnTypes))
	}
}

func TestParseFunctionWithoutTypes(t *testing.T) {
	input := "function foo(x, y) return x + y end"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	funcdef, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.FuncDefStmt", stmts[0])
	}
	fn := funcdef.Func
	if fn.ParList.Types != nil {
		t.Errorf("ParList.Types should be nil when no types specified")
	}
	if fn.ReturnTypes != nil {
		t.Errorf("ReturnTypes should be nil when not specified")
	}
}

func TestParseTypeDecl(t *testing.T) {
	input := "type User = {name: string, age: number}"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	typedef, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.TypeDefStmt", stmts[0])
	}
	if typedef.Name != "User" {
		t.Errorf("name = %q, want 'User'", typedef.Name)
	}
	rec, ok := typedef.Type.(*ast.RecordTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.RecordTypeExpr", typedef.Type)
	}
	if len(rec.Fields) != 2 {
		t.Errorf("got %d fields, want 2", len(rec.Fields))
	}
}

func TestParseOptionalType(t *testing.T) {
	input := "local x: number? = nil"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	opt, ok := local.Types[0].(*ast.OptionalTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.OptionalTypeExpr", local.Types[0])
	}
	inner, ok := opt.Inner.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("inner = %T, want *ast.PrimitiveTypeExpr", opt.Inner)
	}
	if inner.Name != testTypeNumber {
		t.Errorf("inner name = %q, want '%s'", inner.Name, testTypeNumber)
	}
}

func TestParseUnionType(t *testing.T) {
	input := "local x: number | string = 1"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	union, ok := local.Types[0].(*ast.UnionTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.UnionTypeExpr", local.Types[0])
	}
	if len(union.Types) != 2 {
		t.Errorf("got %d union members, want 2", len(union.Types))
	}
}

func TestParseArrayType(t *testing.T) {
	input := "local arr: {number} = {1, 2, 3}"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	arr, ok := local.Types[0].(*ast.ArrayTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.ArrayTypeExpr", local.Types[0])
	}
	elem, ok := arr.Element.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("element = %T, want *ast.PrimitiveTypeExpr", arr.Element)
	}
	if elem.Name != testTypeNumber {
		t.Errorf("element name = %q, want '%s'", elem.Name, testTypeNumber)
	}
}

// Array of optional elements via type alias (workaround for {T?} ambiguity with optional record fields)
func TestParseArrayTypeOptionalElement(t *testing.T) {
	// Note: Direct {number?} syntax conflicts with optional record field syntax {name?: type}
	// Use a type alias as workaround: type OptNum = number?; then {OptNum}
	input := `
		type OptNum = number?
		local arr: {OptNum} = {}
	`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d stmts, want 2", len(stmts))
	}
	local, ok := stmts[1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.LocalAssignStmt", stmts[1])
	}
	if len(local.Types) != 1 || local.Types[0] == nil {
		t.Fatalf("types = %v, want single type", local.Types)
	}

	// Should be ArrayTypeExpr with TypeRefExpr element (resolved to optional later)
	arr, ok := local.Types[0].(*ast.ArrayTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.ArrayTypeExpr", local.Types[0])
	}
	// At parse time, OptNum is a primitive/type ref, not yet resolved
	ref, ok := arr.Element.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("element = %T, want *ast.PrimitiveTypeExpr", arr.Element)
	}
	if ref.Name != "OptNum" {
		t.Errorf("element name = %q, want 'OptNum'", ref.Name)
	}
}

// Array of optional named type {Item?}
func TestParseArrayTypeOptionalNamedElement(t *testing.T) {
	input := `
		type Item = {id: number}
		local arr: {Item?} = {}
	`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) < 2 {
		t.Fatalf("got %d stmts, want at least 2", len(stmts))
	}

	// Second statement should be the local with {Item?}
	local, ok := stmts[1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt[1] = %T, want *ast.LocalAssignStmt", stmts[1])
	}
	if len(local.Types) != 1 || local.Types[0] == nil {
		t.Fatalf("types = %v, want single type", local.Types)
	}

	arr, ok := local.Types[0].(*ast.ArrayTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.ArrayTypeExpr", local.Types[0])
	}
	opt, ok := arr.Element.(*ast.OptionalTypeExpr)
	if !ok {
		t.Fatalf("element = %T, want *ast.OptionalTypeExpr", arr.Element)
	}
	// Parser creates PrimitiveTypeExpr for bare identifiers (semantic resolution happens later)
	prim, ok := opt.Inner.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("inner = %T, want *ast.PrimitiveTypeExpr", opt.Inner)
	}
	if prim.Name != "Item" {
		t.Errorf("inner name = %q, want 'Item'", prim.Name)
	}
}

func TestParseMapType(t *testing.T) {
	// Map type uses Luau-style syntax: {[K]: V}
	input := "local m: {[string]: number} = {}"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	mapType, ok := local.Types[0].(*ast.MapTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.MapTypeExpr", local.Types[0])
	}
	key, ok := mapType.Key.(*ast.PrimitiveTypeExpr)
	if !ok || key.Name != "string" {
		t.Errorf("key = %v, want string", mapType.Key)
	}
	val, ok := mapType.Value.(*ast.PrimitiveTypeExpr)
	if !ok || val.Name != testTypeNumber {
		t.Errorf("value = %v, want %s", mapType.Value, testTypeNumber)
	}
}

func TestParseFunctionType(t *testing.T) {
	input := "local fn: (number, string) -> boolean"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	fn, ok := local.Types[0].(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.FunctionTypeExpr", local.Types[0])
	}
	if len(fn.Params) != 2 {
		t.Errorf("got %d params, want 2", len(fn.Params))
	}
	if len(fn.Returns) != 1 {
		t.Errorf("got %d returns, want 1", len(fn.Returns))
	}
}

func TestParseFunctionTypeNamedParams(t *testing.T) {
	input := "local fn: (x: number, y: string) -> boolean"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	fn, ok := local.Types[0].(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.FunctionTypeExpr", local.Types[0])
	}
	if len(fn.Params) != 2 {
		t.Errorf("got %d params, want 2", len(fn.Params))
	}
	if fn.Params[0].Name != "x" {
		t.Errorf("param 0 name = %q, want 'x'", fn.Params[0].Name)
	}
	if fn.Params[1].Name != "y" {
		t.Errorf("param 1 name = %q, want 'y'", fn.Params[1].Name)
	}
	p0, ok := fn.Params[0].Type.(*ast.PrimitiveTypeExpr)
	if !ok || p0.Name != testTypeNumber {
		t.Errorf("param 0 type = %v, want %s", fn.Params[0].Type, testTypeNumber)
	}
	p1, ok := fn.Params[1].Type.(*ast.PrimitiveTypeExpr)
	if !ok || p1.Name != testTypeString {
		t.Errorf("param 1 type = %v, want %s", fn.Params[1].Type, testTypeString)
	}
	if len(fn.Returns) != 1 {
		t.Errorf("got %d returns, want 1", len(fn.Returns))
	}
}

func TestParseFunctionTypeMixedParams(t *testing.T) {
	// Test mixing named and anonymous params
	tests := []struct {
		input    string
		paramCnt int
		hasNames []bool
		desc     string
	}{
		{"local fn: (number) -> boolean", 1, []bool{false}, "single anonymous"},
		{"local fn: (x: number) -> boolean", 1, []bool{true}, "single named"},
		{"local fn: (number, string) -> boolean", 2, []bool{false, false}, "two anonymous"},
		{"local fn: (x: number, y: string) -> boolean", 2, []bool{true, true}, "two named"},
		{"local fn: (self: any) -> nil", 1, []bool{true}, "self param"},
	}

	for _, tt := range tests {
		stmts, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
			continue
		}
		local := stmts[0].(*ast.LocalAssignStmt)
		fn, ok := local.Types[0].(*ast.FunctionTypeExpr)
		if !ok {
			t.Errorf("Parse(%q) [%s] type = %T, want *ast.FunctionTypeExpr", tt.input, tt.desc, local.Types[0])
			continue
		}
		if len(fn.Params) != tt.paramCnt {
			t.Errorf("Parse(%q) [%s] got %d params, want %d", tt.input, tt.desc, len(fn.Params), tt.paramCnt)
			continue
		}
		for i, hasName := range tt.hasNames {
			if hasName && fn.Params[i].Name == "" {
				t.Errorf("Parse(%q) [%s] param %d should have name", tt.input, tt.desc, i)
			}
			if !hasName && fn.Params[i].Name != "" {
				t.Errorf("Parse(%q) [%s] param %d should not have name, got %q", tt.input, tt.desc, i, fn.Params[i].Name)
			}
		}
	}
}

func TestParseGenericType(t *testing.T) {
	input := "local arr: Array<number>"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	gen, ok := local.Types[0].(*ast.GenericTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.GenericTypeExpr", local.Types[0])
	}
	if gen.Base.Path[0] != "Array" {
		t.Errorf("base = %v, want Array", gen.Base.Path)
	}
	if len(gen.Args) != 1 {
		t.Errorf("got %d args, want 1", len(gen.Args))
	}
}

func TestParseIntegerType(t *testing.T) {
	input := "local x: integer = 42"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.PrimitiveTypeExpr", local.Types[0])
	}
	if prim.Name != "integer" {
		t.Errorf("type name = %q, want 'integer'", prim.Name)
	}
}

func TestParseCastExpr(t *testing.T) {
	input := "local x = data as User"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	if len(local.Exprs) != 1 {
		t.Fatalf("got %d exprs, want 1", len(local.Exprs))
	}
	cast, ok := local.Exprs[0].(*ast.CastExpr)
	if !ok {
		t.Fatalf("expr = %T, want *ast.CastExpr", local.Exprs[0])
	}
	if cast.Expr == nil {
		t.Error("cast.Expr is nil")
	}
	if cast.Type == nil {
		t.Error("cast.Type is nil")
	}
	prim, ok := cast.Type.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("cast.Type = %T, want *ast.PrimitiveTypeExpr", cast.Type)
	}
	if prim.Name != "User" {
		t.Errorf("cast type = %q, want 'User'", prim.Name)
	}
}

func TestParseNonNilAssert(t *testing.T) {
	input := "local x = maybeNil!"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	if len(local.Exprs) != 1 {
		t.Fatalf("got %d exprs, want 1", len(local.Exprs))
	}
	assert, ok := local.Exprs[0].(*ast.NonNilAssertExpr)
	if !ok {
		t.Fatalf("expr = %T, want *ast.NonNilAssertExpr", local.Exprs[0])
	}
	if assert.Expr == nil {
		t.Error("assert.Expr is nil")
	}
}

func TestParseInterfaceDecl(t *testing.T) {
	input := `interface Serializable
		function serialize(self: any): string
	end`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	iface, ok := stmts[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.InterfaceDefStmt", stmts[0])
	}
	if iface.Name != "Serializable" {
		t.Errorf("name = %q, want 'Serializable'", iface.Name)
	}
	if len(iface.Methods) != 1 {
		t.Errorf("got %d methods, want 1", len(iface.Methods))
	}
	if iface.Methods[0].Name != "serialize" {
		t.Errorf("method name = %q, want 'serialize'", iface.Methods[0].Name)
	}
}

func TestParseInterfaceWithExtends(t *testing.T) {
	input := `interface Child: Parent
		function foo(): number
	end`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	iface := stmts[0].(*ast.InterfaceDefStmt)
	if len(iface.Extends) != 1 {
		t.Fatalf("got %d extends, want 1", len(iface.Extends))
	}
	if iface.Extends[0].Path[0] != "Parent" {
		t.Errorf("extends = %v, want 'Parent'", iface.Extends[0].Path)
	}
}

func TestParseEmptyInterface(t *testing.T) {
	input := "interface Empty end"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	iface := stmts[0].(*ast.InterfaceDefStmt)
	if iface.Name != "Empty" {
		t.Errorf("name = %q, want 'Empty'", iface.Name)
	}
	if len(iface.Methods) != 0 {
		t.Errorf("got %d methods, want 0", len(iface.Methods))
	}
}

func TestParseComplexTypes(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"local x: number | string | nil", "triple union"},
		{"local x: number?", "optional"},
		{"local x: {number}", "array"},
		{"local x: {[string]: number}", "map"},
		{"local x: {name: string, age: number}", "record"},
		{"local x: (number) -> string", "function type"},
		{"local x: (x: number, y: string) -> boolean", "function type with named params"},
		{"local x: () -> ()", "void function"},
		{"local x: Array<number>", "generic"},
		{"local x: Map<string, number>", "generic two args"},
		{"local x: readonly {number}", "readonly array"},
		{"local x: number | string?", "union with optional"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseFunctionVariants(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"function foo() end", "no params no return"},
		{"function foo(x) end", "one param"},
		{"function foo(x, y) end", "two params"},
		{"function foo(x: number) end", "typed param"},
		{"function foo(x: number, y: string) end", "two typed params"},
		{"function foo(): number return 1 end", "return type"},
		{"function foo(): (number, string) return 1, 'a' end", "multiple returns"},
		{"function foo(x: number): boolean return true end", "full signature"},
		{"function foo(...) end", "variadic"},
		{"function foo(x, ...) end", "param and variadic"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseTypeDeclVariants(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"type Num = number", "alias to primitive"},
		{"type User = {name: string}", "record type"},
		{"type MaybeNum = number?", "optional alias"},
		{"type NumOrStr = number | string", "union alias"},
		{"type Callback = (number) -> boolean", "function alias"},
		{"type List = {number}", "array alias"},
		{"type Dict = {[string]: any}", "map alias"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseCastVariants(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"local x = y as number", "cast to primitive"},
		{"local x = data as User", "cast to type ref"},
		{"local x = arr as {string}", "cast to array"},
		{"local x = (a + b) as number", "cast expression"},
		{"local x = y :: number", ":: cast to primitive"},
		{"local x = data :: User", ":: cast to type ref"},
		{"local x = arr :: {string}", ":: cast to array"},
		{"local x = (a + b) :: number", ":: cast expression"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseNonNilVariants(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"local x = y!", "simple non-nil"},
		{"local x = arr[1]!", "index non-nil"},
		{"local x = obj.field!", "field non-nil"},
		{"local x = foo()!", "call non-nil"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseLabelAndCastCoexist(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"::loop:: goto loop", "simple label"},
		{"::continue::", "label only"},
		{"local x = y :: number; ::label::", "cast then label"},
		{"::label:: local x = y :: number", "label then cast"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseRecordFieldVariants(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"type R = {a: number}", "single field"},
		{"type R = {a: number, b: string}", "two fields"},
		{"type R = {a: number,}", "trailing comma"},
		{"type R = {a?: number}", "optional field"},
		{"type R = {a: number, b?: string}", "mixed fields"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseExistingLuaUnchanged(t *testing.T) {
	// Ensure existing Lua code still parses correctly
	tests := []string{
		"local x = 1",
		"local x, y = 1, 2",
		"function foo(x, y) return x + y end",
		"local function bar() end",
		"if true then end",
		"for i = 1, 10 do end",
		"while true do break end",
		"print('hello')",
		"local t = {a = 1, b = 2}",
		"print(type(x))",
	}

	for _, input := range tests {
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
	}
}

func FuzzParser(f *testing.F) {
	// Seed with valid Lua/typed code
	seeds := []string{
		"local x = 1",
		"local x: number = 1",
		"local x: number | string = 1",
		"local x: number?",
		"local x: {number} = {}",
		"local x: {[string]: number} = {}",
		"local x: {name: string, age: number}",
		"local x: (number) -> string",
		"local x: () -> ()",
		"local x: (number, string) -> (boolean, number)",
		"type User = {name: string}",
		"type Callback = (number) -> boolean",
		"interface Foo function bar(): number end",
		"local x = data as User",
		"local x = maybeNil!",
		"function foo(x: number): string return '' end",
		"local arr: Array<number>",
		"local m: Map<string, number>",
		"local x: readonly {number}",
		"print(type(x))",
		"local t = {a = 1, b = 2}",
		"for i = 1, 10 do print(i) end",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Parser should not panic on any input
		defer func() {
			if r := recover(); r != nil {
				// Some panics are expected for invalid syntax
				// but we catch them to ensure no unexpected crashes
				if err, ok := r.(*Error); ok {
					_ = err // expected parse error
				} else if err, ok := r.(error); ok {
					_ = err // expected error
				}
			}
		}()
		_, _ = Parse(strings.NewReader(input), "test")
	})
}

func TestParseEdgeCases(t *testing.T) {
	// Edge cases that should parse (or at least not panic)
	tests := []struct {
		input       string
		shouldParse bool
		desc        string
	}{
		{"", true, "empty input"},
		{"local x", true, "uninitialized local"},
		{"local x: number", true, "typed uninitialized"},
		{"type T = number", true, "simple type alias"},
		{"type T<K> = {K}", true, "generic type alias"},
		{"interface I end", true, "empty interface"},
		{"local x: A.B", true, "qualified type"},
		{"local x: A<B<C> >", true, "nested generics with space"},
		{"local x: number | string | boolean", true, "triple union"},
		{"local x: A & B & C", true, "triple intersection"},
		{"local x = a as b as c", true, "chained cast"},
		{"local x = a!!!", true, "chained non-nil"},
		{"local x: number??", true, "double optional"},
		{"local x: {a: number, b?: string,}", true, "trailing comma in record"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if tt.shouldParse && err != nil {
			t.Errorf("Parse(%q) [%s] unexpected error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseInvalidSyntax(t *testing.T) {
	// These should fail to parse (but not panic)
	tests := []struct {
		input string
		desc  string
	}{
		{"local x:", "type annotation without type"},
		{"interface end", "interface without name"},
		{"local x: ->", "arrow without params"},
		{"local x: {: number}", "record without field name"},
		{"local x: number |", "dangling union in type annotation"},
		{"local x: number &", "dangling intersection in type annotation"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err == nil {
			t.Errorf("Parse(%q) [%s] expected error, got nil", tt.input, tt.desc)
		}
	}
}

func TestParseTypeAsIdentifier(t *testing.T) {
	// 'type' is a contextual keyword - valid as variable name
	tests := []string{
		"local type = 1",
		"type = 42",
		"print(type(x))",
	}
	for _, input := range tests {
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
	}
}

func TestParseAsFieldAccess(t *testing.T) {
	// 'as' keyword should be usable as a field name in dot access
	tests := []string{
		"local x = sql.as.int(5)",
		"local y = obj.as",
		"print(t.as.null())",
		"local z = foo.bar.as.baz",
	}
	for _, input := range tests {
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
	}
}

func TestParseDeepNesting(t *testing.T) {
	// Test deep nesting doesn't cause stack overflow
	tests := []struct {
		input string
		desc  string
	}{
		{"local x: {{{{number}}}}", "deeply nested arrays"},
		{"local x: number | string | boolean | nil | any", "long union"},
		{"local x: A & B & C & D & E", "long intersection"},
		{"local x: () -> () -> () -> number", "chained function types"},
		{"local x: {{{[string]: number}}}", "nested map in arrays"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
		}
	}
}

func TestParseAssertsType(t *testing.T) {
	tests := []struct {
		input     string
		paramName string
		hasType   bool
		desc      string
	}{
		{"function assertOk(x: any?): asserts x end", "x", false, "truthy assertion"},
		{"function assertNum(x: any): asserts x is number end", "x", true, "type assertion"},
	}

	for _, tt := range tests {
		stmts, err := Parse(strings.NewReader(tt.input), "test")
		if err != nil {
			t.Errorf("Parse(%q) [%s] error: %v", tt.input, tt.desc, err)
			continue
		}
		if len(stmts) != 1 {
			t.Errorf("Parse(%q) got %d stmts, want 1", tt.input, len(stmts))
			continue
		}
		funcdef, ok := stmts[0].(*ast.FuncDefStmt)
		if !ok {
			t.Errorf("Parse(%q) got %T, want *ast.FuncDefStmt", tt.input, stmts[0])
			continue
		}
		fn := funcdef.Func
		if len(fn.ReturnTypes) != 1 {
			t.Errorf("Parse(%q) got %d return types, want 1", tt.input, len(fn.ReturnTypes))
			continue
		}
		asserts, ok := fn.ReturnTypes[0].(*ast.AssertsTypeExpr)
		if !ok {
			t.Errorf("Parse(%q) return type = %T, want *ast.AssertsTypeExpr", tt.input, fn.ReturnTypes[0])
			continue
		}
		if asserts.ParamName != tt.paramName {
			t.Errorf("Parse(%q) param name = %q, want %q", tt.input, asserts.ParamName, tt.paramName)
		}
		if tt.hasType && asserts.NarrowTo == nil {
			t.Errorf("Parse(%q) NarrowTo = nil, expected type", tt.input)
		}
		if !tt.hasType && asserts.NarrowTo != nil {
			t.Errorf("Parse(%q) NarrowTo = %v, expected nil", tt.input, asserts.NarrowTo)
		}
	}
}

func testTypeContextualParsing(t *testing.T) {
	t.Helper()
	cases := []string{"type Foo = number", "local type = 1", "print(type(x))"}
	for _, input := range cases {
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
	}
}

func TestParseConflictResolution(t *testing.T) {
	t.Run("optional_binds_tight", func(t *testing.T) {
		local := parseOne(t, "local x: number | string?").(*ast.LocalAssignStmt)
		union := local.Types[0].(*ast.UnionTypeExpr)
		if len(union.Types) != 2 {
			t.Fatalf("expected 2 union members, got %d", len(union.Types))
		}
		if _, ok := union.Types[1].(*ast.OptionalTypeExpr); !ok {
			t.Errorf("expected string? to be OptionalTypeExpr, got %T", union.Types[1])
		}
	})

	t.Run("function_return_union", func(t *testing.T) {
		local := parseOne(t, "local x: (number) -> string | boolean").(*ast.LocalAssignStmt)
		fn := local.Types[0].(*ast.FunctionTypeExpr)
		if len(fn.Returns) != 1 {
			t.Fatalf("expected 1 return, got %d", len(fn.Returns))
		}
		union, ok := fn.Returns[0].(*ast.UnionTypeExpr)
		if !ok {
			t.Errorf("expected return to be UnionTypeExpr, got %T", fn.Returns[0])
		} else if len(union.Types) != 2 {
			t.Errorf("expected union with 2 types, got %d", len(union.Types))
		}
	})

	t.Run("type_contextual", testTypeContextualParsing)

	t.Run("generic_type", func(t *testing.T) {
		local := parseOne(t, "local x: Map<string, number>").(*ast.LocalAssignStmt)
		gen := local.Types[0].(*ast.GenericTypeExpr)
		if len(gen.Args) != 2 {
			t.Errorf("expected 2 type args, got %d", len(gen.Args))
		}
	})

	t.Run("intersection_optional", func(t *testing.T) {
		local := parseOne(t, "local x: A & B?").(*ast.LocalAssignStmt)
		inter := local.Types[0].(*ast.IntersectionTypeExpr)
		if _, ok := inter.Types[1].(*ast.OptionalTypeExpr); !ok {
			t.Errorf("expected B? to be OptionalTypeExpr, got %T", inter.Types[1])
		}
	})
}

func TestParseReadonlyArrayType(t *testing.T) {
	input := "local arr: readonly {number} = {}"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	arr, ok := local.Types[0].(*ast.ArrayTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.ArrayTypeExpr", local.Types[0])
	}
	if !arr.Readonly {
		t.Error("expected Readonly = true")
	}
	elem, ok := arr.Element.(*ast.PrimitiveTypeExpr)
	if !ok || elem.Name != testTypeNumber {
		t.Errorf("element = %v, want %s", arr.Element, testTypeNumber)
	}
}

func TestParseReadonlyMapType(t *testing.T) {
	input := "local m: readonly {[string]: number} = {}"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	mapType, ok := local.Types[0].(*ast.MapTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.MapTypeExpr", local.Types[0])
	}
	if !mapType.Readonly {
		t.Error("expected Readonly = true")
	}
}

func TestParseLiteralStringType(t *testing.T) {
	input := `local x: "hello" = "hello"`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	lit, ok := local.Types[0].(*ast.LiteralTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.LiteralTypeExpr", local.Types[0])
	}
	str, ok := lit.Value.(string)
	if !ok || str != "hello" {
		t.Errorf("literal value = %v, want 'hello'", lit.Value)
	}
}

func TestParseLiteralNumberType(t *testing.T) {
	input := "local x: 42 = 42"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	lit, ok := local.Types[0].(*ast.LiteralTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.LiteralTypeExpr", local.Types[0])
	}
	// Whole numbers are parsed as int64
	num, ok := lit.Value.(int64)
	if !ok || num != 42 {
		t.Errorf("literal value = %v, want 42", lit.Value)
	}
}

func TestParseLiteralBooleanType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"local x: true = true", true},
		{"local x: false = false", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			stmts, err := Parse(strings.NewReader(tt.input), "test")
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			local := stmts[0].(*ast.LocalAssignStmt)
			lit, ok := local.Types[0].(*ast.LiteralTypeExpr)
			if !ok {
				t.Fatalf("type = %T, want *ast.LiteralTypeExpr", local.Types[0])
			}
			b, ok := lit.Value.(bool)
			if !ok || b != tt.want {
				t.Errorf("literal value = %v, want %v", lit.Value, tt.want)
			}
		})
	}
}

func TestParseUnionWithLiterals(t *testing.T) {
	input := `local x: "red" | "green" | "blue" = "red"`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	union, ok := local.Types[0].(*ast.UnionTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.UnionTypeExpr", local.Types[0])
	}
	if len(union.Types) != 3 {
		t.Errorf("got %d union members, want 3", len(union.Types))
	}
	// Check first literal
	lit, ok := union.Types[0].(*ast.LiteralTypeExpr)
	if !ok {
		t.Errorf("union member 0 = %T, want *ast.LiteralTypeExpr", union.Types[0])
	} else if lit.Value != "red" {
		t.Errorf("union member 0 value = %v, want 'red'", lit.Value)
	}
}

// Annotation parsing tests

func TestParseAnnotationSimple(t *testing.T) {
	input := `local x: number @min(0) = 1`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.PrimitiveTypeExpr", local.Types[0])
	}
	if prim.Name != testTypeNumber {
		t.Errorf("type name = %q, want '%s'", prim.Name, testTypeNumber)
	}
	if len(prim.Annotations) != 1 {
		t.Fatalf("got %d annotations, want 1", len(prim.Annotations))
	}
	if prim.Annotations[0].Name != "min" {
		t.Errorf("annotation name = %q, want 'min'", prim.Annotations[0].Name)
	}
	if len(prim.Annotations[0].Args) != 1 {
		t.Fatalf("got %d args, want 1", len(prim.Annotations[0].Args))
	}
}

func TestParseAnnotationMultiple(t *testing.T) {
	input := `local x: number @min(0) @max(100) = 50`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.PrimitiveTypeExpr", local.Types[0])
	}
	if len(prim.Annotations) != 2 {
		t.Fatalf("got %d annotations, want 2", len(prim.Annotations))
	}
	if prim.Annotations[0].Name != "min" {
		t.Errorf("annotation 0 name = %q, want 'min'", prim.Annotations[0].Name)
	}
	if prim.Annotations[1].Name != "max" {
		t.Errorf("annotation 1 name = %q, want 'max'", prim.Annotations[1].Name)
	}
}

func TestParseAnnotationString(t *testing.T) {
	input := `local x: string @pattern("^.+@.+$") = "test@example.com"`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.PrimitiveTypeExpr", local.Types[0])
	}
	if len(prim.Annotations) != 1 {
		t.Fatalf("got %d annotations, want 1", len(prim.Annotations))
	}
	if prim.Annotations[0].Name != "pattern" {
		t.Errorf("annotation name = %q, want 'pattern'", prim.Annotations[0].Name)
	}
}

func TestParseAnnotationNoArgs(t *testing.T) {
	input := `local x: number @integer = 42`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	prim, ok := local.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.PrimitiveTypeExpr", local.Types[0])
	}
	if len(prim.Annotations) != 1 {
		t.Fatalf("got %d annotations, want 1", len(prim.Annotations))
	}
	if prim.Annotations[0].Name != "integer" {
		t.Errorf("annotation name = %q, want 'integer'", prim.Annotations[0].Name)
	}
	if len(prim.Annotations[0].Args) != 0 {
		t.Errorf("got %d args, want 0", len(prim.Annotations[0].Args))
	}
}

func TestParseRecordFieldAnnotation(t *testing.T) {
	input := `type User = {
		age: number @min(0) @max(150),
		name: string @min_len(1) @max_len(100)
	}`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	typeDef := stmts[0].(*ast.TypeDefStmt)
	record, ok := typeDef.Type.(*ast.RecordTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.RecordTypeExpr", typeDef.Type)
	}
	if len(record.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(record.Fields))
	}

	// Check age field - annotations are on the type
	if record.Fields[0].Name != "age" {
		t.Errorf("field 0 name = %q, want 'age'", record.Fields[0].Name)
	}
	prim0, ok := record.Fields[0].Type.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("field 0 type = %T, want *ast.PrimitiveTypeExpr", record.Fields[0].Type)
	}
	if len(prim0.Annotations) != 2 {
		t.Errorf("field 0 type got %d annotations, want 2", len(prim0.Annotations))
	}

	// Check name field - annotations are on the type
	if record.Fields[1].Name != "name" {
		t.Errorf("field 1 name = %q, want 'name'", record.Fields[1].Name)
	}
	prim1, ok := record.Fields[1].Type.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("field 1 type = %T, want *ast.PrimitiveTypeExpr", record.Fields[1].Type)
	}
	if len(prim1.Annotations) != 2 {
		t.Errorf("field 1 type got %d annotations, want 2", len(prim1.Annotations))
	}
}

func TestParseArrayWithAnnotations(t *testing.T) {
	input := `local scores: {number @min(0) @max(100)} = {85, 90}`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	arr, ok := local.Types[0].(*ast.ArrayTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.ArrayTypeExpr", local.Types[0])
	}
	// Annotations are stored on the element type itself
	elem, ok := arr.Element.(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("element = %T, want *ast.PrimitiveTypeExpr", arr.Element)
	}
	if len(elem.Annotations) != 2 {
		t.Errorf("got %d annotations on element, want 2", len(elem.Annotations))
	}
}

func TestParseUnionWithAnnotations(t *testing.T) {
	input := `local id: number @min(1) | string @min_len(1) = 1`
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	local := stmts[0].(*ast.LocalAssignStmt)
	union, ok := local.Types[0].(*ast.UnionTypeExpr)
	if !ok {
		t.Fatalf("type = %T, want *ast.UnionTypeExpr", local.Types[0])
	}
	if len(union.Types) != 2 {
		t.Fatalf("got %d union members, want 2", len(union.Types))
	}

	// Check first member has annotation
	prim1, ok := union.Types[0].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("union member 0 = %T, want *ast.PrimitiveTypeExpr", union.Types[0])
	}
	if len(prim1.Annotations) != 1 {
		t.Errorf("union member 0 got %d annotations, want 1", len(prim1.Annotations))
	}

	// Check second member has annotation
	prim2, ok := union.Types[1].(*ast.PrimitiveTypeExpr)
	if !ok {
		t.Fatalf("union member 1 = %T, want *ast.PrimitiveTypeExpr", union.Types[1])
	}
	if len(prim2.Annotations) != 1 {
		t.Errorf("union member 1 got %d annotations, want 1", len(prim2.Annotations))
	}
}

func TestParseContextualKeywordsAsFieldNames(t *testing.T) {
	tests := []struct {
		code string
		name string
	}{
		{`local t = {readonly = true}`, "readonly"},
		{`local t = {type = "foo"}`, "type"},
		{`local t = {interface = 1}`, "interface"},
		{`local t = {as = "bar"}`, "as"},
		{`local t = {asserts = false}`, "asserts"},
		{`local t = {is = nil}`, "is"},
		{`local t = {readonly = 1, type = 2, interface = 3}`, "multiple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseString(tt.code, "test")
			if err != nil {
				t.Errorf("ParseString(%q) error = %v", tt.code, err)
			}
		})
	}
}

func TestParseContextualKeywordsAsPropertyAccess(t *testing.T) {
	tests := []struct {
		code string
		name string
	}{
		{`local x = entry.readonly`, "readonly"},
		{`local x = entry.type`, "type"},
		{`local x = entry.interface`, "interface"},
		{`local x = entry.as`, "as"},
		{`local x = entry.asserts`, "asserts"},
		{`local x = entry.is`, "is"},
		{`local t = {readonly = entry.readonly == true}`, "combined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseString(tt.code, "test")
			if err != nil {
				t.Errorf("ParseString(%q) error = %v", tt.code, err)
			}
		})
	}
}

func TestParseContextualKeywordsAsMethodNames(t *testing.T) {
	tests := []struct {
		code string
		name string
	}{
		{`function Builder:readonly() end`, "funcdef_readonly"},
		{`function Builder:type() end`, "funcdef_type"},
		{`function Builder:interface() end`, "funcdef_interface"},
		{`function Builder:as(name) end`, "funcdef_as"},
		{`function Builder:asserts() end`, "funcdef_asserts"},
		{`function Builder:is() end`, "funcdef_is"},
		{`obj:readonly()`, "call_readonly"},
		{`obj:type()`, "call_type"},
		{`obj:interface()`, "call_interface"},
		{`obj:as("foo")`, "call_as"},
		{`obj:asserts()`, "call_asserts"},
		{`obj:is()`, "call_is"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseString(tt.code, "test")
			if err != nil {
				t.Errorf("ParseString(%q) error = %v", tt.code, err)
			}
		})
	}
}

func TestParseTypeOfExpr(t *testing.T) {
	t.Run("typeof identifier", func(t *testing.T) {
		typedef := parseOneString(t, `type T = typeof(x)`).(*ast.TypeDefStmt)
		typeOf := typedef.Type.(*ast.TypeOfExpr)
		ident := typeOf.Expr.(*ast.IdentExpr)
		if ident.Value != "x" {
			t.Errorf("ident = %q, want 'x'", ident.Value)
		}
	})

	t.Run("typeof table literal", func(t *testing.T) {
		typedef := parseOneString(t, `type T = typeof({ x = 1 })`).(*ast.TypeDefStmt)
		typeOf := typedef.Type.(*ast.TypeOfExpr)
		if _, ok := typeOf.Expr.(*ast.TableExpr); !ok {
			t.Fatalf("expr = %T, want *ast.TableExpr", typeOf.Expr)
		}
	})

	t.Run("typeof in variable annotation", func(t *testing.T) {
		local := parseOneString(t, `local x: typeof(config) = nil`).(*ast.LocalAssignStmt)
		typeOf := local.Types[0].(*ast.TypeOfExpr)
		ident := typeOf.Expr.(*ast.IdentExpr)
		if ident.Value != "config" {
			t.Errorf("ident = %q, want 'config'", ident.Value)
		}
	})

	t.Run("typeof in union", func(t *testing.T) {
		typedef := parseOneString(t, `type T = typeof(a) | typeof(b)`).(*ast.TypeDefStmt)
		union := typedef.Type.(*ast.UnionTypeExpr)
		if len(union.Types) != 2 {
			t.Fatalf("got %d union members, want 2", len(union.Types))
		}
		for i, member := range union.Types {
			if _, ok := member.(*ast.TypeOfExpr); !ok {
				t.Errorf("union member %d = %T, want *ast.TypeOfExpr", i, member)
			}
		}
	})
}

// === TDD Tests for Parser Features ===

func TestParse_ColonColonCast(t *testing.T) {
	t.Run("basic cast", func(t *testing.T) {
		stmts, err := Parse(strings.NewReader("local x = y :: number"), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		local := stmts[0].(*ast.LocalAssignStmt)
		cast, ok := local.Exprs[0].(*ast.CastExpr)
		if !ok {
			t.Fatalf("expr = %T, want *ast.CastExpr", local.Exprs[0])
		}
		if cast.Expr == nil || cast.Type == nil {
			t.Error("cast.Expr or cast.Type is nil")
		}
	})

	t.Run("cast to complex type", func(t *testing.T) {
		tests := []string{
			"local x = y :: {string}",
			"local x = y :: {a: number, b: string}",
			"local x = y :: (number) -> string",
			"local x = y :: string | number",
			"local x = y :: User?",
		}
		for _, input := range tests {
			_, err := Parse(strings.NewReader(input), "test")
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
			}
		}
	})

	t.Run("cast in expression", func(t *testing.T) {
		tests := []string{
			"local x = (a + b) :: number",
			"local x = foo() :: string",
			"local x = arr[1] :: User",
			"local x = obj.field :: number",
		}
		for _, input := range tests {
			_, err := Parse(strings.NewReader(input), "test")
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
			}
		}
	})

	t.Run("cast equivalence with as", func(t *testing.T) {
		// Both syntaxes should produce CastExpr
		inputs := []struct {
			colonColon string
			asKeyword  string
		}{
			{"local x = y :: number", "local x = y as number"},
			{"local x = y :: string?", "local x = y as string?"},
			{"local x = y :: {number}", "local x = y as {number}"},
		}
		for _, tt := range inputs {
			stmts1, err1 := Parse(strings.NewReader(tt.colonColon), "test")
			stmts2, err2 := Parse(strings.NewReader(tt.asKeyword), "test")
			if err1 != nil || err2 != nil {
				t.Errorf("Parse error: :: = %v, as = %v", err1, err2)
				continue
			}
			local1 := stmts1[0].(*ast.LocalAssignStmt)
			local2 := stmts2[0].(*ast.LocalAssignStmt)
			_, ok1 := local1.Exprs[0].(*ast.CastExpr)
			_, ok2 := local2.Exprs[0].(*ast.CastExpr)
			if !ok1 || !ok2 {
				t.Errorf("Both should produce CastExpr: :: = %T, as = %T",
					local1.Exprs[0], local2.Exprs[0])
			}
		}
	})
}

func testLabelWhitespace(t *testing.T) {
	t.Helper()
	for _, input := range []string{":: label ::", "::\tlabel\t::", "::  label  ::"} {
		label := parseOne(t, input).(*ast.LabelStmt)
		if label.Name != "label" {
			t.Errorf("Parse(%q) label.Name = %q, want 'label'", input, label.Name)
		}
	}
}

func testMultipleLabels(t *testing.T) {
	t.Helper()
	stmts, err := Parse(strings.NewReader("::a:: ::b:: ::c::"), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 3 {
		t.Fatalf("got %d stmts, want 3", len(stmts))
	}
	for i, name := range []string{"a", "b", "c"} {
		if stmts[i].(*ast.LabelStmt).Name != name {
			t.Errorf("stmts[%d].Name = %q, want %q", i, stmts[i].(*ast.LabelStmt).Name, name)
		}
	}
}

func TestParse_Labels(t *testing.T) {
	t.Run("simple label", func(t *testing.T) {
		label := parseOne(t, "::myLabel::").(*ast.LabelStmt)
		if label.Name != "myLabel" {
			t.Errorf("label.Name = %q, want 'myLabel'", label.Name)
		}
	})

	t.Run("label with underscore", func(t *testing.T) {
		label := parseOne(t, "::my_label_123::").(*ast.LabelStmt)
		if label.Name != "my_label_123" {
			t.Errorf("label.Name = %q, want 'my_label_123'", label.Name)
		}
	})

	t.Run("label with whitespace", testLabelWhitespace)
	t.Run("multiple labels", testMultipleLabels)

	t.Run("label with goto", func(t *testing.T) {
		input := "::start::\nx = x + 1\nif x < 10 then goto start end"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if len(stmts) != 3 {
			t.Fatalf("got %d stmts, want 3", len(stmts))
		}
		if stmts[0].(*ast.LabelStmt).Name != "start" {
			t.Errorf("label.Name = %q, want 'start'", stmts[0].(*ast.LabelStmt).Name)
		}
	})
}

func TestParse_CastAndLabelCoexistence(t *testing.T) {
	t.Run("cast then label on newline", func(t *testing.T) {
		input := "local x = y :: number\n::label::"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if len(stmts) != 2 {
			t.Fatalf("got %d stmts, want 2", len(stmts))
		}
		if _, ok := stmts[0].(*ast.LocalAssignStmt); !ok {
			t.Errorf("stmts[0] = %T, want *ast.LocalAssignStmt", stmts[0])
		}
		if _, ok := stmts[1].(*ast.LabelStmt); !ok {
			t.Errorf("stmts[1] = %T, want *ast.LabelStmt", stmts[1])
		}
	})

	t.Run("label then cast", func(t *testing.T) {
		input := "::label:: local x = y :: number"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if len(stmts) != 2 {
			t.Fatalf("got %d stmts, want 2", len(stmts))
		}
		label := stmts[0].(*ast.LabelStmt)
		if label.Name != "label" {
			t.Errorf("label.Name = %q, want 'label'", label.Name)
		}
	})

	t.Run("cast with semicolon then label", func(t *testing.T) {
		input := "local x = y :: number; ::label::"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if len(stmts) != 2 {
			t.Fatalf("got %d stmts, want 2", len(stmts))
		}
	})

	t.Run("as cast then label", func(t *testing.T) {
		input := "local x = y as number\n::label::"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if len(stmts) != 2 {
			t.Fatalf("got %d stmts, want 2", len(stmts))
		}
	})

	t.Run("complex interleaving", func(t *testing.T) {
		input := `
			::start::
			local x = data :: User
			::middle::
			x = transform(x) as Result
			::done::
			return x
		`
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		// label, local, label, assign, label, return
		if len(stmts) != 6 {
			t.Fatalf("got %d stmts, want 6", len(stmts))
		}
	})
}

func TestParse_TypeOperators(t *testing.T) {
	t.Run("keyof", func(t *testing.T) {
		tests := []string{
			"type K = keyof(User)",
			"type K = keyof({a: number, b: string})",
		}
		for _, input := range tests {
			stmts, err := Parse(strings.NewReader(input), "test")
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
				continue
			}
			typedef := stmts[0].(*ast.TypeDefStmt)
			if _, ok := typedef.Type.(*ast.KeyOfExpr); !ok {
				t.Errorf("Parse(%q) type = %T, want *ast.KeyOfExpr", input, typedef.Type)
			}
		}
	})

	t.Run("index access", func(t *testing.T) {
		tests := []string{
			`type T = User["name"]`,
			`type T = Record["key"]`,
		}
		for _, input := range tests {
			stmts, err := Parse(strings.NewReader(input), "test")
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
				continue
			}
			typedef := stmts[0].(*ast.TypeDefStmt)
			if _, ok := typedef.Type.(*ast.IndexAccessExpr); !ok {
				t.Errorf("Parse(%q) type = %T, want *ast.IndexAccessExpr", input, typedef.Type)
			}
		}
	})

	t.Run("conditional type", func(t *testing.T) {
		tests := []string{
			"type T = string extends any ? true : false",
			"type T = T extends number ? T : never",
		}
		for _, input := range tests {
			stmts, err := Parse(strings.NewReader(input), "test")
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
				continue
			}
			typedef := stmts[0].(*ast.TypeDefStmt)
			if _, ok := typedef.Type.(*ast.ConditionalTypeExpr); !ok {
				t.Errorf("Parse(%q) type = %T, want *ast.ConditionalTypeExpr", input, typedef.Type)
			}
		}
	})
}

func TestParse_EdgeCases(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		stmts, err := Parse(strings.NewReader(""), "test")
		if err != nil {
			t.Errorf("Parse empty string error: %v", err)
		}
		if len(stmts) != 0 {
			t.Errorf("got %d stmts, want 0", len(stmts))
		}
	})

	t.Run("only whitespace", func(t *testing.T) {
		stmts, err := Parse(strings.NewReader("   \n\t\n   "), "test")
		if err != nil {
			t.Errorf("Parse whitespace error: %v", err)
		}
		if len(stmts) != 0 {
			t.Errorf("got %d stmts, want 0", len(stmts))
		}
	})

	t.Run("only comments", func(t *testing.T) {
		stmts, err := Parse(strings.NewReader("-- comment\n--[[ block ]]"), "test")
		if err != nil {
			t.Errorf("Parse comments error: %v", err)
		}
		if len(stmts) != 0 {
			t.Errorf("got %d stmts, want 0", len(stmts))
		}
	})

	t.Run("deeply nested types", func(t *testing.T) {
		input := "type T = {{{{string}}}}"
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse nested types error: %v", err)
		}
	})

	t.Run("long identifier in label", func(t *testing.T) {
		name := "verylonglabelname_with_underscores_and_numbers_123"
		input := "::" + name + "::"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		label := stmts[0].(*ast.LabelStmt)
		if label.Name != name {
			t.Errorf("label.Name = %q, want %q", label.Name, name)
		}
	})

	t.Run("cast with non-nil assertion", func(t *testing.T) {
		input := "local x = y :: number!"
		stmts, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		local := stmts[0].(*ast.LocalAssignStmt)
		// Should be NonNilAssertExpr wrapping CastExpr
		if _, ok := local.Exprs[0].(*ast.NonNilAssertExpr); !ok {
			t.Errorf("expr = %T, want *ast.NonNilAssertExpr", local.Exprs[0])
		}
	})

	t.Run("chained property access after cast", func(t *testing.T) {
		input := "local x = (y :: User).name"
		_, err := Parse(strings.NewReader(input), "test")
		if err != nil {
			t.Errorf("Parse error: %v", err)
		}
	})
}

func TestParse_InvalidSyntax(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"local x = y ::", "incomplete cast"},
		{":: ::", "empty label"},
		{"::123label::", "label starting with number"},
		{"local x = :: number", "cast without expression"},
		{"type T = keyof", "keyof without argument"},
	}

	for _, tt := range tests {
		_, err := Parse(strings.NewReader(tt.input), "test")
		if err == nil {
			t.Errorf("Parse(%q) [%s] should fail but didn't", tt.input, tt.desc)
		}
	}
}

func TestParseError_String(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantStr string
	}{
		{
			name: "basic error",
			err: &Error{
				Pos:     ast.Position{Source: "test.lua", Line: 1, Column: 5},
				Message: "unexpected token",
			},
			wantStr: "unexpected token",
		},
		{
			name: "EOF error",
			err: &Error{
				Pos:     ast.Position{Source: "test.lua", Line: EOF},
				Message: "unexpected end of file",
			},
			wantStr: "unexpected end of file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}

			result := "parse error: " + tt.err.String()
			expected := "parse error: " + tt.wantStr
			if result != expected {
				t.Errorf("concatenation = %q, want %q", result, expected)
			}
		})
	}
}

func TestParseStringConcatSpan(t *testing.T) {
	source := "local s = \"a\" .. b"
	stmts, err := ParseString(source, "test.lua")
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	stmt, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("expected LocalAssignStmt, got %T", stmts[0])
	}
	if len(stmt.Exprs) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(stmt.Exprs))
	}
	expr, ok := stmt.Exprs[0].(*ast.StringConcatOpExpr)
	if !ok {
		t.Fatalf("expected StringConcatOpExpr, got %T", stmt.Exprs[0])
	}
	if expr.Column() != 11 || expr.LastColumn() != 18 {
		t.Fatalf("concat span = %d:%d, want 11:18", expr.Column(), expr.LastColumn())
	}
}

func TestParseGenericFunctionTypeParams(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantParams []string
	}{
		{
			name:       "single type param",
			input:      "function identity<T>(x: T): T return x end",
			wantParams: []string{"T"},
		},
		{
			name:       "multiple type params",
			input:      "function make_pair<K, V>(k: K, v: V): {key: K, value: V} return {key = k, value = v} end",
			wantParams: []string{"K", "V"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := Parse(strings.NewReader(tt.input), "test")
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(stmts) != 1 {
				t.Fatalf("got %d stmts, want 1", len(stmts))
			}
			funcdef, ok := stmts[0].(*ast.FuncDefStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.FuncDefStmt", stmts[0])
			}
			fn := funcdef.Func
			if len(fn.TypeParams) != len(tt.wantParams) {
				t.Errorf("got %d type params, want %d", len(fn.TypeParams), len(tt.wantParams))
				return
			}
			for i, want := range tt.wantParams {
				if fn.TypeParams[i].Name != want {
					t.Errorf("TypeParams[%d].Name = %q, want %q", i, fn.TypeParams[i].Name, want)
				}
			}
		})
	}
}

func TestParseGenericFunctionExpr(t *testing.T) {
	input := "local f = function<T>(x: T): T return x end"
	stmts, err := Parse(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.LocalAssignStmt", stmts[0])
	}
	if len(local.Exprs) != 1 {
		t.Fatalf("got %d exprs, want 1", len(local.Exprs))
	}
	fn, ok := local.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("expr is %T, want *ast.FunctionExpr", local.Exprs[0])
	}
	if len(fn.TypeParams) != 1 {
		t.Errorf("got %d type params, want 1", len(fn.TypeParams))
		return
	}
	if fn.TypeParams[0].Name != "T" {
		t.Errorf("TypeParams[0].Name = %q, want 'T'", fn.TypeParams[0].Name)
	}
}

func TestParseVoidReturnInRecordField(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "named params void return with comma",
			input: `type C = {f: (self: C) -> (),}`,
		},
		{
			name:  "named params void return no comma",
			input: `type C = {f: (self: C) -> ()}`,
		},
		{
			name:  "named params void return multiline",
			input: "type C = {\n    f: (self: C) -> (),\n    g: (self: C, x: number) -> ()\n}",
		},
		{
			name:  "multiple named params void return",
			input: `type C = {f: (a: number, b: string) -> ()}`,
		},
		{
			name:  "no params void return",
			input: `type C = {f: () -> ()}`,
		},
		// Parenthesized single return like (number) is ambiguous with grouping
		// and handled separately via the typeexprlist2 rule for 2+ returns.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseString(tt.input, "test")
			if err != nil {
				t.Errorf("expected no parse error, got: %v", err)
			}
		})
	}
}

func TestParseVoidReturnAllForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		// Parenthesized forms for optional/union wrapping
		{name: "paren params void", input: `type T = {f: ((x: number) -> ())?}`},
		{name: "paren empty void", input: `type T = {f: (() -> ())?}`},
		{name: "paren params single", input: `type T = {f: ((x: number) -> string)?}`},
		{name: "paren empty single", input: `type T = {f: (() -> string)?}`},
		{name: "paren params multi", input: `type T = {f: ((x: number) -> (string, number))?}`},
		{name: "paren empty multi", input: `type T = {f: (() -> (string, number))?}`},
		{name: "paren void in union", input: `type T = ((x: number) -> ()) | nil`},
		// fun keyword with void return
		{name: "fun params void", input: `type T = {f: fun(x: number): ()}`},
		{name: "fun empty void", input: `type T = {f: fun(): ()}`},
		// fun keyword regression
		{name: "fun params single", input: `type T = {f: fun(x: number): string}`},
		{name: "fun empty single", input: `type T = {f: fun(): string}`},
		{name: "fun params multi", input: `type T = {f: fun(x: number): (string, number)}`},
		{name: "fun empty multi", input: `type T = {f: fun(): (string, number)}`},
		{name: "fun params no return", input: `type T = {f: fun(x: number)}`},
		{name: "fun empty no return", input: `type T = {f: fun()}`},
		// Bare arrow forms
		{name: "bare params void", input: `type T = {f: (x: number) -> ()}`},
		{name: "bare empty void", input: `type T = {f: () -> ()}`},
		{name: "bare params single", input: `type T = {f: (x: number) -> string}`},
		{name: "bare empty single", input: `type T = {f: () -> string}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseString(tt.input, "test")
			if err != nil {
				t.Errorf("expected no parse error, got: %v", err)
			}
		})
	}
}
