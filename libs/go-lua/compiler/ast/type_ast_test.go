package ast

import (
	"testing"
)

func TestTypeExprInterface(t *testing.T) {
	// Verify all type expressions implement TypeExpr
	var _ TypeExpr = &PrimitiveTypeExpr{}
	var _ TypeExpr = &OptionalTypeExpr{}
	var _ TypeExpr = &UnionTypeExpr{}
	var _ TypeExpr = &IntersectionTypeExpr{}
	var _ TypeExpr = &ArrayTypeExpr{}
	var _ TypeExpr = &MapTypeExpr{}
	var _ TypeExpr = &RecordTypeExpr{}
	var _ TypeExpr = &FunctionTypeExpr{}
	var _ TypeExpr = &TypeRefExpr{}
	var _ TypeExpr = &GenericTypeExpr{}
	var _ TypeExpr = &LiteralTypeExpr{}
	var _ TypeExpr = &MetaTypeExpr{}
	var _ TypeExpr = &SelfTypeExpr{}
	var _ TypeExpr = &TupleTypeExpr{}
	var _ TypeExpr = &TypeOfExpr{}
	var _ TypeExpr = &KeyOfExpr{}
	var _ TypeExpr = &IndexAccessExpr{}
	var _ TypeExpr = &ConditionalTypeExpr{}
}

func TestTypeOfExpr(t *testing.T) {
	// typeof(x) captures type of expression x
	expr := &IdentExpr{Value: "person"}
	typeOf := &TypeOfExpr{Expr: expr}
	typeOf.SetLine(5)

	if typeOf.Expr != expr {
		t.Error("Expr should match")
	}
	if typeOf.Line() != 5 {
		t.Error("Line should be 5")
	}
}

func TestTypeOfExprWithTableConstructor(t *testing.T) {
	// typeof({ name = "Alice" }) - Luau style
	tableExpr := &TableExpr{
		Fields: []*Field{
			{Key: &StringExpr{Value: "name"}, Value: &StringExpr{Value: "Alice"}},
		},
	}
	typeOf := &TypeOfExpr{Expr: tableExpr}

	if typeOf.Expr != tableExpr {
		t.Error("Expr should be table constructor")
	}
}

func TestTypeOfExprWithAttrGet(t *testing.T) {
	// typeof(module.config) - property access
	attrGet := &AttrGetExpr{
		Object: &IdentExpr{Value: "module"},
		Key:    &StringExpr{Value: "config"},
	}
	typeOf := &TypeOfExpr{Expr: attrGet}

	if typeOf.Expr != attrGet {
		t.Error("Expr should be attribute access")
	}
}

func TestStmtInterface(t *testing.T) {
	// Verify type-related statements implement Stmt
	var _ Stmt = &TypeDefStmt{}
	var _ Stmt = &InterfaceDefStmt{}
}

func TestExprInterface(t *testing.T) {
	// Verify type-related expressions implement Expr
	var _ Expr = &CastExpr{}
	var _ Expr = &NonNilAssertExpr{}
}

func TestPrimitiveTypeExpr(t *testing.T) {
	p := &PrimitiveTypeExpr{Name: "number"}
	p.SetLine(10)

	if p.Name != "number" {
		t.Error("Name should be 'number'")
	}
	if p.Line() != 10 {
		t.Error("Line should be 10")
	}
}

func TestOptionalTypeExpr(t *testing.T) {
	inner := &PrimitiveTypeExpr{Name: "string"}
	opt := &OptionalTypeExpr{Inner: inner}
	opt.SetLine(5)

	if opt.Inner != inner {
		t.Error("Inner should match")
	}
	if opt.Line() != 5 {
		t.Error("Line should be 5")
	}
}

func TestUnionTypeExpr(t *testing.T) {
	t1 := &PrimitiveTypeExpr{Name: "number"}
	t2 := &PrimitiveTypeExpr{Name: "string"}
	union := &UnionTypeExpr{Types: []TypeExpr{t1, t2}}

	if len(union.Types) != 2 {
		t.Error("Should have 2 types")
	}
}

func TestIntersectionTypeExpr(t *testing.T) {
	t1 := &TypeRefExpr{Path: []string{"Named"}}
	t2 := &TypeRefExpr{Path: []string{"Aged"}}
	inter := &IntersectionTypeExpr{Types: []TypeExpr{t1, t2}}

	if len(inter.Types) != 2 {
		t.Error("Should have 2 types")
	}
}

func TestArrayTypeExpr(t *testing.T) {
	elem := &PrimitiveTypeExpr{Name: "number"}
	arr := &ArrayTypeExpr{Element: elem, Readonly: true}

	if arr.Element != elem {
		t.Error("Element should match")
	}
	if !arr.Readonly {
		t.Error("Readonly should be true")
	}
}

func TestMapTypeExpr(t *testing.T) {
	key := &PrimitiveTypeExpr{Name: "string"}
	val := &PrimitiveTypeExpr{Name: "number"}
	m := &MapTypeExpr{Key: key, Value: val, Readonly: false}

	if m.Key != key {
		t.Error("Key should match")
	}
	if m.Value != val {
		t.Error("Value should match")
	}
	if m.Readonly {
		t.Error("Readonly should be false")
	}
}

func TestRecordTypeExpr(t *testing.T) {
	field1 := RecordFieldExpr{Name: "name", Type: &PrimitiveTypeExpr{Name: "string"}, Optional: false}
	field2 := RecordFieldExpr{Name: "age", Type: &PrimitiveTypeExpr{Name: "number"}, Optional: true}
	rec := &RecordTypeExpr{Fields: []RecordFieldExpr{field1, field2}}

	if len(rec.Fields) != 2 {
		t.Error("Should have 2 fields")
	}
	if rec.Fields[0].Name != "name" {
		t.Error("First field should be 'name'")
	}
	if !rec.Fields[1].Optional {
		t.Error("Second field should be optional")
	}
}

func TestFunctionTypeExpr(t *testing.T) {
	param := &PrimitiveTypeExpr{Name: "number"}
	ret := &PrimitiveTypeExpr{Name: "string"}
	fn := &FunctionTypeExpr{
		Params:  []FunctionParamExpr{{Type: param}},
		Returns: []TypeExpr{ret},
	}

	if len(fn.Params) != 1 {
		t.Error("Should have 1 param")
	}
	if len(fn.Returns) != 1 {
		t.Error("Should have 1 return")
	}
}

func TestFunctionTypeExprWithGenerics(t *testing.T) {
	tparam := TypeParamExpr{Name: "T", Constraint: nil}
	fn := &FunctionTypeExpr{
		TypeParams: []TypeParamExpr{tparam},
		Params:     []FunctionParamExpr{{Type: &TypeRefExpr{Path: []string{"T"}}}},
		Returns:    []TypeExpr{&TypeRefExpr{Path: []string{"T"}}},
	}

	if len(fn.TypeParams) != 1 {
		t.Error("Should have 1 type param")
	}
	if fn.TypeParams[0].Name != "T" {
		t.Error("Type param should be 'T'")
	}
}

func TestFunctionTypeExprVariadic(t *testing.T) {
	variadic := &PrimitiveTypeExpr{Name: "any"}
	fn := &FunctionTypeExpr{
		Params:   []FunctionParamExpr{},
		Variadic: variadic,
		Returns:  []TypeExpr{},
	}

	if fn.Variadic == nil {
		t.Error("Variadic should not be nil")
	}
}

func TestTypeRefExpr(t *testing.T) {
	ref := &TypeRefExpr{Path: []string{"http", "Request"}}
	ref.SetLine(20)

	if len(ref.Path) != 2 {
		t.Error("Path should have 2 parts")
	}
	if ref.Path[0] != "http" || ref.Path[1] != "Request" {
		t.Error("Path should be ['http', 'Request']")
	}
}

func TestGenericTypeExpr(t *testing.T) {
	base := &TypeRefExpr{Path: []string{"Map"}}
	arg1 := &PrimitiveTypeExpr{Name: "string"}
	arg2 := &PrimitiveTypeExpr{Name: "number"}
	gen := &GenericTypeExpr{Base: base, Args: []TypeExpr{arg1, arg2}}

	if gen.Base != base {
		t.Error("Base should match")
	}
	if len(gen.Args) != 2 {
		t.Error("Should have 2 args")
	}
}

func TestLiteralTypeExpr(t *testing.T) {
	lit1 := &LiteralTypeExpr{Value: "red"}
	lit2 := &LiteralTypeExpr{Value: 42.0}
	lit3 := &LiteralTypeExpr{Value: true}

	if lit1.Value != "red" {
		t.Error("String literal should be 'red'")
	}
	if lit2.Value != 42.0 {
		t.Error("Number literal should be 42.0")
	}
	if lit3.Value != true {
		t.Error("Bool literal should be true")
	}
}

func TestMetaTypeExpr(t *testing.T) {
	inner := &TypeRefExpr{Path: []string{"User"}}
	meta := &MetaTypeExpr{Inner: inner}

	if meta.Inner != inner {
		t.Error("Inner should match")
	}
}

func TestSelfTypeExpr(t *testing.T) {
	self := &SelfTypeExpr{}
	self.SetLine(1)

	if self.Line() != 1 {
		t.Error("Line should be 1")
	}
}

func TestTupleTypeExpr(t *testing.T) {
	t1 := &PrimitiveTypeExpr{Name: "number"}
	t2 := &PrimitiveTypeExpr{Name: "string"}
	tuple := &TupleTypeExpr{Elements: []TypeExpr{t1, t2}}

	if len(tuple.Elements) != 2 {
		t.Error("Should have 2 elements")
	}
}

func TestTypeDefStmt(t *testing.T) {
	def := &TypeDefStmt{
		Name: "User",
		TypeParams: []TypeParamExpr{
			{Name: "T", Constraint: nil},
		},
		Type: &RecordTypeExpr{
			Fields: []RecordFieldExpr{
				{Name: "name", Type: &PrimitiveTypeExpr{Name: "string"}},
			},
		},
	}
	def.SetLine(1)

	if def.Name != "User" {
		t.Error("Name should be 'User'")
	}
	if len(def.TypeParams) != 1 {
		t.Error("Should have 1 type param")
	}
}

func TestInterfaceDefStmt(t *testing.T) {
	iface := &InterfaceDefStmt{
		Name:    "Serializable",
		Extends: []*TypeRefExpr{{Path: []string{"Base"}}},
		Fields: []RecordFieldExpr{
			{Name: "id", Type: &PrimitiveTypeExpr{Name: "number"}},
		},
		Methods: []InterfaceMethodExpr{
			{
				Name: "serialize",
				Type: &FunctionTypeExpr{
					Params:  []FunctionParamExpr{{Type: &SelfTypeExpr{}}},
					Returns: []TypeExpr{&PrimitiveTypeExpr{Name: "string"}},
				},
			},
		},
	}
	iface.SetLine(10)

	if iface.Name != "Serializable" {
		t.Error("Name should be 'Serializable'")
	}
	if len(iface.Extends) != 1 {
		t.Error("Should have 1 extends")
	}
	if len(iface.Fields) != 1 {
		t.Error("Should have 1 field")
	}
	if len(iface.Methods) != 1 {
		t.Error("Should have 1 method")
	}
}

func TestCastExpr(t *testing.T) {
	expr := &IdentExpr{Value: "data"}
	typ := &TypeRefExpr{Path: []string{"User"}}
	cast := &CastExpr{Expr: expr, Type: typ}
	cast.SetLine(5)

	if cast.Expr != expr {
		t.Error("Expr should match")
	}
	if cast.Type != typ {
		t.Error("Type should match")
	}
}

func TestNonNilAssertExpr(t *testing.T) {
	expr := &IdentExpr{Value: "maybeNil"}
	assert := &NonNilAssertExpr{Expr: expr}
	assert.SetLine(5)

	if assert.Expr != expr {
		t.Error("Expr should match")
	}
}

func TestTypeAnnotation(t *testing.T) {
	annot := TypeAnnotation{
		Type: &PrimitiveTypeExpr{Name: "number"},
	}

	if annot.Type == nil {
		t.Error("Type should not be nil")
	}
}

func TestTypeParamExprWithConstraint(t *testing.T) {
	constraint := &TypeRefExpr{Path: []string{"Comparable"}}
	param := TypeParamExpr{Name: "T", Constraint: constraint}

	if param.Name != "T" {
		t.Error("Name should be 'T'")
	}
	if param.Constraint != constraint {
		t.Error("Constraint should match")
	}
}

func TestTypeExprPositionHolder(t *testing.T) {
	types := []TypeExpr{
		&PrimitiveTypeExpr{},
		&OptionalTypeExpr{},
		&UnionTypeExpr{},
		&IntersectionTypeExpr{},
		&ArrayTypeExpr{},
		&MapTypeExpr{},
		&RecordTypeExpr{},
		&FunctionTypeExpr{},
		&TypeRefExpr{},
		&GenericTypeExpr{},
		&LiteralTypeExpr{},
		&MetaTypeExpr{},
		&SelfTypeExpr{},
		&TupleTypeExpr{},
	}

	for i, typ := range types {
		typ.SetLine(i + 1)
		typ.SetLastLine(i + 10)

		if typ.Line() != i+1 {
			t.Errorf("type %d: Line() = %d, want %d", i, typ.Line(), i+1)
		}
		if typ.LastLine() != i+10 {
			t.Errorf("type %d: LastLine() = %d, want %d", i, typ.LastLine(), i+10)
		}
	}
}

func TestTypeDefStmtPositionHolder(t *testing.T) {
	def := &TypeDefStmt{Name: "Test"}
	def.SetLine(5)
	def.SetLastLine(10)

	if def.Line() != 5 {
		t.Errorf("Line() = %d, want 5", def.Line())
	}
	if def.LastLine() != 10 {
		t.Errorf("LastLine() = %d, want 10", def.LastLine())
	}
}

func TestInterfaceDefStmtPositionHolder(t *testing.T) {
	iface := &InterfaceDefStmt{Name: "Test"}
	iface.SetLine(15)
	iface.SetLastLine(25)

	if iface.Line() != 15 {
		t.Errorf("Line() = %d, want 15", iface.Line())
	}
	if iface.LastLine() != 25 {
		t.Errorf("LastLine() = %d, want 25", iface.LastLine())
	}
}

func TestCastExprPositionHolder(t *testing.T) {
	cast := &CastExpr{}
	cast.SetLine(1)
	cast.SetLastLine(2)

	if cast.Line() != 1 {
		t.Error("Line should be 1")
	}
	if cast.LastLine() != 2 {
		t.Error("LastLine should be 2")
	}
}

func TestNonNilAssertExprPositionHolder(t *testing.T) {
	assert := &NonNilAssertExpr{}
	assert.SetLine(3)
	assert.SetLastLine(4)

	if assert.Line() != 3 {
		t.Error("Line should be 3")
	}
	if assert.LastLine() != 4 {
		t.Error("LastLine should be 4")
	}
}

func TestRecordFieldExprFields(t *testing.T) {
	field := RecordFieldExpr{
		Name:     "email",
		Type:     &PrimitiveTypeExpr{Name: "string"},
		Optional: true,
	}

	if field.Name != "email" {
		t.Error("Name should be 'email'")
	}
	if field.Type == nil {
		t.Error("Type should not be nil")
	}
	if !field.Optional {
		t.Error("Optional should be true")
	}
}

func TestInterfaceMethodExprFields(t *testing.T) {
	method := InterfaceMethodExpr{
		Name: "toString",
		Type: &FunctionTypeExpr{
			Params:  []FunctionParamExpr{},
			Returns: []TypeExpr{&PrimitiveTypeExpr{Name: "string"}},
		},
	}

	if method.Name != "toString" {
		t.Error("Name should be 'toString'")
	}
	if method.Type == nil {
		t.Error("Type should not be nil")
	}
	if len(method.Type.Returns) != 1 {
		t.Error("Should have 1 return type")
	}
}

func TestComplexUnionType(t *testing.T) {
	// number | string | nil
	union := &UnionTypeExpr{
		Types: []TypeExpr{
			&PrimitiveTypeExpr{Name: "number"},
			&PrimitiveTypeExpr{Name: "string"},
			&PrimitiveTypeExpr{Name: "nil"},
		},
	}
	union.SetLine(1)

	if len(union.Types) != 3 {
		t.Error("Should have 3 union members")
	}
}

func TestNestedOptionalType(t *testing.T) {
	// {string}?
	opt := &OptionalTypeExpr{
		Inner: &ArrayTypeExpr{
			Element: &PrimitiveTypeExpr{Name: "string"},
		},
	}

	inner, ok := opt.Inner.(*ArrayTypeExpr)
	if !ok {
		t.Error("Inner should be ArrayTypeExpr")
	}
	if inner.Element.(*PrimitiveTypeExpr).Name != "string" {
		t.Error("Array element should be string")
	}
}

func TestComplexFunctionType(t *testing.T) {
	// <T, U extends Comparable>(arr: {T}, cmp: (T, T) -> number) -> {U}
	fn := &FunctionTypeExpr{
		TypeParams: []TypeParamExpr{
			{Name: "T", Constraint: nil},
			{Name: "U", Constraint: &TypeRefExpr{Path: []string{"Comparable"}}},
		},
		Params: []FunctionParamExpr{
			{Type: &ArrayTypeExpr{Element: &TypeRefExpr{Path: []string{"T"}}}},
			{Type: &FunctionTypeExpr{
				Params:  []FunctionParamExpr{{Type: &TypeRefExpr{Path: []string{"T"}}}, {Type: &TypeRefExpr{Path: []string{"T"}}}},
				Returns: []TypeExpr{&PrimitiveTypeExpr{Name: "number"}},
			}},
		},
		Returns: []TypeExpr{
			&ArrayTypeExpr{Element: &TypeRefExpr{Path: []string{"U"}}},
		},
	}

	if len(fn.TypeParams) != 2 {
		t.Error("Should have 2 type params")
	}
	if fn.TypeParams[1].Constraint == nil {
		t.Error("Second type param should have constraint")
	}
	if len(fn.Params) != 2 {
		t.Error("Should have 2 params")
	}
}

func TestTypeRefExprSinglePath(t *testing.T) {
	ref := &TypeRefExpr{Path: []string{"User"}}

	if len(ref.Path) != 1 {
		t.Error("Path should have 1 element")
	}
	if ref.Path[0] != "User" {
		t.Error("Path[0] should be 'User'")
	}
}

func TestGenericTypeExprMultipleArgs(t *testing.T) {
	// Result<User, Error>
	gen := &GenericTypeExpr{
		Base: &TypeRefExpr{Path: []string{"Result"}},
		Args: []TypeExpr{
			&TypeRefExpr{Path: []string{"User"}},
			&TypeRefExpr{Path: []string{"Error"}},
		},
	}

	if len(gen.Args) != 2 {
		t.Error("Should have 2 type args")
	}
}

func TestKeyOfExpr(t *testing.T) {
	// keyof Person
	inner := &TypeRefExpr{Path: []string{"Person"}}
	keyof := &KeyOfExpr{Inner: inner}
	keyof.SetLine(10)

	if keyof.Inner != inner {
		t.Error("Inner should match")
	}
	if keyof.Line() != 10 {
		t.Error("Line should be 10")
	}
}

func TestIndexAccessExpr(t *testing.T) {
	// Person["name"]
	obj := &TypeRefExpr{Path: []string{"Person"}}
	idx := &LiteralTypeExpr{Value: "name"}
	access := &IndexAccessExpr{Object: obj, Index: idx}
	access.SetLine(15)

	if access.Object != obj {
		t.Error("Object should match")
	}
	if access.Index != idx {
		t.Error("Index should match")
	}
	if access.Line() != 15 {
		t.Error("Line should be 15")
	}
}

func TestConditionalTypeExpr(t *testing.T) {
	// T extends string ? number : boolean
	check := &TypeRefExpr{Path: []string{"T"}}
	extends := &PrimitiveTypeExpr{Name: "string"}
	thenBr := &PrimitiveTypeExpr{Name: "number"}
	elseBr := &PrimitiveTypeExpr{Name: "boolean"}

	cond := &ConditionalTypeExpr{
		Check:   check,
		Extends: extends,
		Then:    thenBr,
		Else:    elseBr,
	}
	cond.SetLine(20)

	if cond.Check != check {
		t.Error("Check should match")
	}
	if cond.Extends != extends {
		t.Error("Extends should match")
	}
	if cond.Then != thenBr {
		t.Error("Then should match")
	}
	if cond.Else != elseBr {
		t.Error("Else should match")
	}
	if cond.Line() != 20 {
		t.Error("Line should be 20")
	}
}
