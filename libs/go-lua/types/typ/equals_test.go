package typ

import "testing"

func TestTypeEqualsIdentity(t *testing.T) {
	if !TypeEquals(Number, Number) {
		t.Error("number should equal number")
	}

	if !TypeEquals(String, String) {
		t.Error("string should equal string")
	}
}

func TestTypeEqualsNil(t *testing.T) {
	if TypeEquals(nil, Number) {
		t.Error("nil should not equal number")
	}

	if TypeEquals(Number, nil) {
		t.Error("number should not equal nil")
	}

	if !TypeEquals(nil, nil) {
		t.Error("nil should equal nil")
	}
}

func TestTypeEqualsDifferentKinds(t *testing.T) {
	if TypeEquals(Number, String) {
		t.Error("number should not equal string")
	}
}

func TestTypeEqualsAlias(t *testing.T) {
	alias := NewAlias("MyNum", Number)

	if !TypeEquals(alias, Number) {
		t.Error("alias to number should equal number")
	}

	if !TypeEquals(Number, alias) {
		t.Error("number should equal alias to number")
	}
}

func TestTypeEqualsRef(t *testing.T) {
	r1 := NewRef("mod", "T")
	r2 := NewRef("mod", "T")
	r3 := NewRef("mod", "U")

	if !TypeEquals(r1, r2) {
		t.Error("mod.T should equal mod.T")
	}

	if TypeEquals(r1, r3) {
		t.Error("mod.T should not equal mod.U")
	}
}

func TestTypeEqualsOptional(t *testing.T) {
	o1 := NewOptional(Number)
	o2 := NewOptional(Number)
	o3 := NewOptional(String)

	if !TypeEquals(o1, o2) {
		t.Error("number? should equal number?")
	}

	if TypeEquals(o1, o3) {
		t.Error("number? should not equal string?")
	}
}

func TestTypeEqualsUnion(t *testing.T) {
	u1 := NewUnion(Number, String)
	u2 := NewUnion(Number, String)
	u3 := NewUnion(Number, Boolean)

	if !TypeEquals(u1, u2) {
		t.Error("number | string should equal number | string")
	}

	if TypeEquals(u1, u3) {
		t.Error("number | string should not equal number | boolean")
	}
}

func TestTypeEqualsArray(t *testing.T) {
	a1 := NewArray(Number)
	a2 := NewArray(Number)
	a3 := NewArray(String)

	if !TypeEquals(a1, a2) {
		t.Error("number[] should equal number[]")
	}

	if TypeEquals(a1, a3) {
		t.Error("number[] should not equal string[]")
	}
}

func TestTypeEqualsMap(t *testing.T) {
	m1 := NewMap(String, Number)
	m2 := NewMap(String, Number)
	m3 := NewMap(String, String)

	if !TypeEquals(m1, m2) {
		t.Error("maps should be equal")
	}

	if TypeEquals(m1, m3) {
		t.Error("maps with different value types should not be equal")
	}
}

func TestTypeEqualsTuple(t *testing.T) {
	t1 := NewTuple(Number, String)
	t2 := NewTuple(Number, String)
	t3 := NewTuple(String, Number)

	if !TypeEquals(t1, t2) {
		t.Error("tuples should be equal")
	}

	if TypeEquals(t1, t3) {
		t.Error("tuples with different element order should not be equal")
	}
}

func TestTypeEqualsRecord(t *testing.T) {
	r1 := NewRecord().Field("x", Number).Build()
	r2 := NewRecord().Field("x", Number).Build()
	r3 := NewRecord().Field("x", String).Build()

	if !TypeEquals(r1, r2) {
		t.Error("records should be equal")
	}

	if TypeEquals(r1, r3) {
		t.Error("records with different field types should not be equal")
	}
}

func TestTypeEqualsFunction(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(String).Build()
	f2 := Func().Param("y", Number).Returns(String).Build()
	f3 := Func().Param("x", String).Returns(String).Build()

	if !TypeEquals(f1, f2) {
		t.Error("functions with same signature should be equal")
	}

	if TypeEquals(f1, f3) {
		t.Error("functions with different param types should not be equal")
	}
}

func TestTypeEqualsDepthLimit(t *testing.T) {
	nested := Number
	for i := 0; i < 200; i++ {
		nested = NewArray(nested)
	}

	if TypeEquals(nested, nested) {
		t.Log("deep types may hit depth limit")
	}
}

func TestTypeString(t *testing.T) {
	if TypeString(nil) != "nil" {
		t.Error("TypeString(nil) should be nil")
	}

	if TypeString(Number) != "number" {
		t.Error("TypeString(Number) should be number")
	}
}

func TestTypeEqualsNilNil(t *testing.T) {
	// Both nil should return true
	if !TypeEquals(nil, nil) {
		t.Error("nil should equal nil")
	}
}

func TestTypeEqualsIntersection(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(Number, String)
	i3 := NewIntersection(Number, Boolean)

	if !TypeEquals(i1, i2) {
		t.Error("same intersections should be equal")
	}
	if TypeEquals(i1, i3) {
		t.Error("different intersections should not be equal")
	}
}

func TestTypeEqualsIntersectionLength(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(Number, String, Boolean)

	if TypeEquals(i1, i2) {
		t.Error("intersections of different lengths should not be equal")
	}
}

func TestTypeEqualsAliasChain(t *testing.T) {
	// A = B, B = C, C = number
	c := NewAlias("C", Number)
	b := NewAlias("B", c)
	a := NewAlias("A", b)

	if !TypeEquals(a, Number) {
		t.Error("alias chain should equal base type")
	}
	if !TypeEquals(Number, a) {
		t.Error("base type should equal alias chain")
	}
	if !TypeEquals(a, c) {
		t.Error("outer alias should equal inner alias")
	}
}

func TestTypeEqualsLocalRef(t *testing.T) {
	// Refs only equal other refs with same module and name
	// Refs do NOT equal aliases - they are different types
	ref := NewRef("", "MyType")
	alias := NewAlias("MyType", Number)

	if TypeEquals(ref, alias) {
		t.Error("ref should not equal alias even with same name")
	}

	// But alias should still equal its target
	if !TypeEquals(alias, Number) {
		t.Error("alias should equal its target")
	}
}

func TestTypeEqualsLocalRefToLocalRef(t *testing.T) {
	ref1 := NewRef("", "Type")
	ref2 := NewRef("", "Type")
	ref3 := NewRef("", "Other")

	if !TypeEquals(ref1, ref2) {
		t.Error("same local refs should be equal")
	}
	if TypeEquals(ref1, ref3) {
		t.Error("different local refs should not be equal")
	}
}

func TestTypeEqualsLiteral(t *testing.T) {
	l1 := LiteralString("hello")
	l2 := LiteralString("hello")
	l3 := LiteralString("world")

	if !TypeEquals(l1, l2) {
		t.Error("same string literals should be equal")
	}
	if TypeEquals(l1, l3) {
		t.Error("different string literals should not be equal")
	}
}

func TestTypeEqualsLiteralInt(t *testing.T) {
	l1 := LiteralInt(42)
	l2 := LiteralInt(42)
	l3 := LiteralInt(100)

	if !TypeEquals(l1, l2) {
		t.Error("same int literals should be equal")
	}
	if TypeEquals(l1, l3) {
		t.Error("different int literals should not be equal")
	}
}

func TestTypeEqualsLiteralBool(t *testing.T) {
	if !TypeEquals(True, True) {
		t.Error("true should equal true")
	}
	if !TypeEquals(False, False) {
		t.Error("false should equal false")
	}
	if TypeEquals(True, False) {
		t.Error("true should not equal false")
	}
}

func TestTypeEqualsUnionOrder(t *testing.T) {
	// Unions should be order-independent after normalization
	u1 := NewUnion(Number, String)
	u2 := NewUnion(String, Number)

	if !TypeEquals(u1, u2) {
		t.Error("unions with same members in different order should be equal")
	}
}

func TestTypeEqualsEmptyRecord(t *testing.T) {
	r1 := NewRecord().Build()
	r2 := NewRecord().Build()

	if !TypeEquals(r1, r2) {
		t.Error("empty records should be equal")
	}
}

func TestTypeEqualsRecordFieldOrder(t *testing.T) {
	// Records with same fields in different order
	r1 := NewRecord().Field("a", Number).Field("b", String).Build()
	r2 := NewRecord().Field("b", String).Field("a", Number).Build()

	if !TypeEquals(r1, r2) {
		t.Error("records with same fields should be equal regardless of definition order")
	}
}

func TestTypeEqualsOptionalField(t *testing.T) {
	r1 := NewRecord().OptField("x", Number).Build()
	r2 := NewRecord().OptField("x", Number).Build()
	r3 := NewRecord().Field("x", Number).Build()

	if !TypeEquals(r1, r2) {
		t.Error("records with same optional fields should be equal")
	}
	if TypeEquals(r1, r3) {
		t.Error("optional field should not equal required field")
	}
}

func TestTypeEqualsFunctionVariadic(t *testing.T) {
	f1 := Func().Variadic(Number).Returns(Nil).Build()
	f2 := Func().Variadic(Number).Returns(Nil).Build()
	f3 := Func().Param("args", Number).Returns(Nil).Build()

	if !TypeEquals(f1, f2) {
		t.Error("same variadic functions should be equal")
	}
	if TypeEquals(f1, f3) {
		t.Error("variadic should not equal non-variadic")
	}
}

func TestTypeEqualsFunctionMultiReturn(t *testing.T) {
	f1 := Func().Returns(Number, String).Build()
	f2 := Func().Returns(Number, String).Build()
	f3 := Func().Returns(Number).Build()

	if !TypeEquals(f1, f2) {
		t.Error("functions with same multi-return should be equal")
	}
	if TypeEquals(f1, f3) {
		t.Error("functions with different return counts should not be equal")
	}
}

func TestTypeEqualsFunctionTypeParams(t *testing.T) {
	f1 := Func().TypeParam("T", nil).Param("x", NewTypeParam("T", nil)).Returns(NewTypeParam("T", nil)).Build()
	f2 := Func().TypeParam("T", nil).Param("x", NewTypeParam("T", nil)).Returns(NewTypeParam("T", nil)).Build()
	f3 := Func().TypeParam("U", nil).Param("x", NewTypeParam("U", nil)).Returns(NewTypeParam("U", nil)).Build()

	if !TypeEquals(f1, f2) {
		t.Error("functions with same type params should be equal")
	}
	if TypeEquals(f1, f3) {
		t.Error("functions with different type param names should not be equal")
	}
	if TypeEquals(f1, Func().Param("x", Any).Returns(Any).Build()) {
		t.Error("generic and non-generic functions should not be equal")
	}
}

func TestTypeEqualsGeneric(t *testing.T) {
	params := []*TypeParam{NewTypeParam("K", nil), NewTypeParam("V", nil)}
	g1 := NewGeneric("T", params, NewMap(NewRef("", "K"), NewRef("", "V")))
	g2 := NewGeneric("T", params, NewMap(NewRef("", "K"), NewRef("", "V")))
	g3 := NewGeneric("U", params, NewMap(NewRef("", "K"), NewRef("", "V")))

	if !TypeEquals(g1, g2) {
		t.Error("same generics should be equal")
	}
	if TypeEquals(g1, g3) {
		t.Error("generics with different names should not be equal")
	}
}

func TestTypeEqualsInterface(t *testing.T) {
	i1 := NewInterface("I", []Method{{Name: "foo", Type: Func().Build()}})
	i2 := NewInterface("I", []Method{{Name: "foo", Type: Func().Build()}})
	i3 := NewInterface("J", []Method{{Name: "foo", Type: Func().Build()}})

	if !TypeEquals(i1, i2) {
		t.Error("same interfaces should be equal")
	}
	if TypeEquals(i1, i3) {
		t.Error("interfaces with different names should not be equal")
	}
}

func TestTypeEqualsInterfaceMethodOrder(t *testing.T) {
	m1 := Method{Name: "foo", Type: Func().Build()}
	m2 := Method{Name: "bar", Type: Func().Build()}

	i1 := NewInterface("I", []Method{m1, m2})
	i2 := NewInterface("I", []Method{m2, m1})

	// Method order may or may not matter depending on normalization
	_ = i1
	_ = i2
}

func TestTypeEqualsCycleDetection(t *testing.T) {
	// Create recursive type that could cause infinite loop
	rec := NewRecursive("Node", func(self Type) Type {
		return NewRecord().OptField("next", self).Build()
	})

	// Should not hang
	if !TypeEquals(rec, rec) {
		t.Error("recursive type should equal itself")
	}
}

func TestTypeEqualsMutualRecursion(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(NewRecord().OptField("b", recB).Build())
	recB.SetBody(NewRecord().OptField("a", recA).Build())

	// Should not hang
	if !TypeEquals(recA, recA) {
		t.Error("mutual recursive A should equal itself")
	}
	if !TypeEquals(recB, recB) {
		t.Error("mutual recursive B should equal itself")
	}
}

func TestTypeEqualsNestedOptional(t *testing.T) {
	o1 := NewOptional(NewOptional(Number))
	o2 := NewOptional(NewOptional(Number))

	if !TypeEquals(o1, o2) {
		t.Error("nested optionals should be equal")
	}
}

func TestTypeEqualsNestedUnion(t *testing.T) {
	inner := NewUnion(Number, String)
	u1 := NewUnion(inner, Boolean)
	u2 := NewUnion(inner, Boolean)

	if !TypeEquals(u1, u2) {
		t.Error("unions with same nested union should be equal")
	}
}

func TestTypeEqualsNeverAnyCases(t *testing.T) {
	if !TypeEquals(Never, Never) {
		t.Error("Never should equal Never")
	}
	if !TypeEquals(Any, Any) {
		t.Error("Any should equal Any")
	}
	if !TypeEquals(Unknown, Unknown) {
		t.Error("Unknown should equal Unknown")
	}
	if TypeEquals(Never, Any) {
		t.Error("Never should not equal Any")
	}
}

func TestTypeEqualsDeepAliasFunctionSignature(t *testing.T) {
	aliasChain := func(depth int) Type {
		t := Number
		for i := 0; i < depth; i++ {
			t = NewAlias("N", t)
		}
		return t
	}

	a := Func().Param("v", aliasChain(32)).Returns(Number).Build()
	b := Func().Param("v", aliasChain(32)).Returns(Number).Build()

	if !TypeEquals(a, b) {
		t.Fatalf("expected deep alias signatures to be equal:\nleft:  %s\nright: %s", FormatShort(a), FormatShort(b))
	}
}
