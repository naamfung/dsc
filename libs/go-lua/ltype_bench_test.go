package lua

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// ---------------------------------------------------------------------------
// Primitive validation benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Number(b *testing.B) {
	L := NewState()
	defer L.Close()
	for i := 0; i < b.N; i++ {
		LTypeNumber.Validate(L, LNumber(42))
	}
}

func BenchmarkValidate_String(b *testing.B) {
	L := NewState()
	defer L.Close()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeString.Validate(L, LString("hello"))
	}
}

func BenchmarkValidate_Boolean(b *testing.B) {
	L := NewState()
	defer L.Close()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeBoolean.Validate(L, LTrue)
	}
}

func BenchmarkValidate_Integer(b *testing.B) {
	L := NewState()
	defer L.Close()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeInteger.Validate(L, LInteger(42))
	}
}

func BenchmarkValidate_IntegerFromNumber(b *testing.B) {
	L := NewState()
	defer L.Close()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeInteger.Validate(L, LNumber(42.0))
	}
}

func BenchmarkValidate_Any(b *testing.B) {
	L := NewState()
	defer L.Close()
	tbl := L.NewTable()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeAny.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Optional type benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_OptionalNumber_Hit(b *testing.B) {
	L := NewState()
	defer L.Close()
	optNum := &LType{inner: typ.NewOptional(typ.Number)}
	for b.ResetTimer(); b.N > 0; b.N-- {
		optNum.Validate(L, LNumber(42))
	}
}

func BenchmarkValidate_OptionalNumber_Nil(b *testing.B) {
	L := NewState()
	defer L.Close()
	optNum := &LType{inner: typ.NewOptional(typ.Number)}
	for b.ResetTimer(); b.N > 0; b.N-- {
		optNum.Validate(L, LNil)
	}
}

func BenchmarkValidate_OptionalTable_Hit(b *testing.B) {
	L := NewState()
	defer L.Close()
	optTable := &LType{inner: typ.NewOptional(typ.NewInterface("table", nil))}
	tbl := L.NewTable()
	for b.ResetTimer(); b.N > 0; b.N-- {
		optTable.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Record validation benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Record_Small(b *testing.B) {
	L := NewState()
	defer L.Close()
	rec := &LType{
		inner: typ.NewRecord().
			Field("x", typ.Number).
			Field("y", typ.Number).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("x", LNumber(1))
	tbl.RawSetString("y", LNumber(2))
	for b.ResetTimer(); b.N > 0; b.N-- {
		rec.Validate(L, tbl)
	}
}

func BenchmarkValidate_Record_Medium(b *testing.B) {
	L := NewState()
	defer L.Close()
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			OptField("icon", typ.String).
			OptField("meta", typ.NewInterface("table", nil)).
			OptField("content", typ.NewInterface("table", nil)).
			OptField("tags", typ.NewArray(typ.String)).
			OptField("actor", typ.String).
			Build(),
		name: "UpdateInput",
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LString("abc"))
	tbl.RawSetString("name", LString("test"))
	tbl.RawSetString("actor", LString("user1"))
	for b.ResetTimer(); b.N > 0; b.N-- {
		rec.Validate(L, tbl)
	}
}

func BenchmarkValidate_Record_Full(b *testing.B) {
	L := NewState()
	defer L.Close()
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			OptField("icon", typ.String).
			OptField("meta", typ.NewInterface("table", nil)).
			OptField("content", typ.NewInterface("table", nil)).
			OptField("tags", typ.NewArray(typ.String)).
			OptField("actor", typ.String).
			Build(),
		name: "UpdateInput",
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LString("abc"))
	tbl.RawSetString("name", LString("test"))
	tbl.RawSetString("icon", LString("icon.png"))
	meta := L.NewTable()
	meta.RawSetString("key", LString("val"))
	tbl.RawSetString("meta", meta)
	content := L.NewTable()
	content.RawSetString("body", LString("text"))
	tbl.RawSetString("content", content)
	tags := L.NewTable()
	tags.Append(LString("a"))
	tags.Append(LString("b"))
	tbl.RawSetString("tags", tags)
	tbl.RawSetString("actor", LString("user1"))
	for b.ResetTimer(); b.N > 0; b.N-- {
		rec.Validate(L, tbl)
	}
}

func BenchmarkValidate_Record_Nested(b *testing.B) {
	L := NewState()
	defer L.Close()
	addr := typ.NewRecord().
		Field("street", typ.String).
		Field("zip", typ.String).
		Build()
	person := &LType{
		inner: typ.NewRecord().
			Field("name", typ.String).
			Field("address", addr).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("name", LString("Alice"))
	a := L.NewTable()
	a.RawSetString("street", LString("Main St"))
	a.RawSetString("zip", LString("12345"))
	tbl.RawSetString("address", a)
	for b.ResetTimer(); b.N > 0; b.N-- {
		person.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Record :is() benchmark (error path allocation)
// ---------------------------------------------------------------------------

func BenchmarkIs_Record_Pass(b *testing.B) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			OptField("count", typ.Number).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LString("abc"))
	tbl.RawSetString("name", LString("test"))
	tbl.RawSetString("count", LNumber(5))
	isMethod := L.typeGetField(rec, "is")
	for b.ResetTimer(); b.N > 0; b.N-- {
		L.Push(isMethod)
		L.Push(tbl)
		L.Call(1, 2)
		L.Pop(2)
	}
}

func BenchmarkIs_Record_Fail(b *testing.B) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LNumber(123))
	tbl.RawSetString("name", LString("test"))
	isMethod := L.typeGetField(rec, "is")
	for b.ResetTimer(); b.N > 0; b.N-- {
		L.Push(isMethod)
		L.Push(tbl)
		L.Call(1, 2)
		L.Pop(2)
	}
}

// ---------------------------------------------------------------------------
// Array validation benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Array_10(b *testing.B) {
	L := NewState()
	defer L.Close()
	arrType := &LType{inner: typ.NewArray(typ.Number)}
	tbl := L.NewTable()
	for i := 0; i < 10; i++ {
		tbl.Append(LNumber(float64(i)))
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		arrType.Validate(L, tbl)
	}
}

func BenchmarkValidate_Array_100(b *testing.B) {
	L := NewState()
	defer L.Close()
	arrType := &LType{inner: typ.NewArray(typ.Number)}
	tbl := L.NewTable()
	for i := 0; i < 100; i++ {
		tbl.Append(LNumber(float64(i)))
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		arrType.Validate(L, tbl)
	}
}

func BenchmarkValidate_Array_1000(b *testing.B) {
	L := NewState()
	defer L.Close()
	arrType := &LType{inner: typ.NewArray(typ.Number)}
	tbl := L.NewTable()
	for i := 0; i < 1000; i++ {
		tbl.Append(LNumber(float64(i)))
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		arrType.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Union and Literal benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Union_2Members(b *testing.B) {
	L := NewState()
	defer L.Close()
	u := &LType{inner: typ.NewUnion(typ.Number, typ.String)}
	for b.ResetTimer(); b.N > 0; b.N-- {
		u.Validate(L, LString("hello"))
	}
}

func BenchmarkValidate_Union_5Literals(b *testing.B) {
	L := NewState()
	defer L.Close()
	u := &LType{
		inner: typ.NewUnion(
			typ.LiteralString("active"),
			typ.LiteralString("draft"),
			typ.LiteralString("archived"),
			typ.LiteralString("deleted"),
			typ.LiteralString("pending"),
		),
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		u.Validate(L, LString("pending"))
	}
}

func BenchmarkValidate_Literal_String(b *testing.B) {
	L := NewState()
	defer L.Close()
	lit := &LType{inner: typ.LiteralString("active")}
	for b.ResetTimer(); b.N > 0; b.N-- {
		lit.Validate(L, LString("active"))
	}
}

// ---------------------------------------------------------------------------
// Map validation benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Map_StringToNumber_10(b *testing.B) {
	L := NewState()
	defer L.Close()
	mapType := &LType{inner: typ.NewMap(typ.String, typ.Number)}
	tbl := L.NewTable()
	for i := 0; i < 10; i++ {
		tbl.RawSetString("key"+string(rune('a'+i)), LNumber(float64(i)))
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		mapType.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Annotated type benchmarks
// ---------------------------------------------------------------------------

func BenchmarkValidate_Annotated_MinMax(b *testing.B) {
	L := NewState()
	defer L.Close()
	ann := &LType{
		inner: typ.NewAnnotated(typ.Number, []typ.Annotation{
			{Name: "min", Arg: float64(0)},
			{Name: "max", Arg: float64(100)},
		}),
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		ann.Validate(L, LNumber(50))
	}
}

func BenchmarkValidate_Annotated_Pattern(b *testing.B) {
	L := NewState()
	defer L.Close()
	ann := &LType{
		inner: typ.NewAnnotated(typ.String, []typ.Annotation{
			{Name: "pattern", Arg: "^[a-z]+$"},
		}),
	}
	for b.ResetTimer(); b.N > 0; b.N-- {
		ann.Validate(L, LString("hello"))
	}
}

func BenchmarkValidate_Record_Annotated(b *testing.B) {
	L := NewState()
	defer L.Close()
	rec := &LType{
		inner: typ.NewRecord().
			AnnotatedField("name", typ.String, false, []typ.Annotation{
				{Name: "min_len", Arg: float64(1)},
				{Name: "max_len", Arg: float64(100)},
			}).
			AnnotatedField("age", typ.Number, false, []typ.Annotation{
				{Name: "min", Arg: float64(0)},
				{Name: "max", Arg: float64(150)},
			}).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("name", LString("Alice"))
	tbl.RawSetString("age", LNumber(30))
	for b.ResetTimer(); b.N > 0; b.N-- {
		rec.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Intersection benchmark
// ---------------------------------------------------------------------------

func BenchmarkValidate_Intersection(b *testing.B) {
	L := NewState()
	defer L.Close()
	recA := typ.NewRecord().Field("x", typ.Number).Build()
	recB := typ.NewRecord().Field("y", typ.String).Build()
	inter := &LType{inner: typ.NewIntersection(recA, recB)}
	tbl := L.NewTable()
	tbl.RawSetString("x", LNumber(1))
	tbl.RawSetString("y", LString("hello"))
	for b.ResetTimer(); b.N > 0; b.N-- {
		inter.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Recursive type benchmark
// ---------------------------------------------------------------------------

func BenchmarkValidate_Recursive_Depth3(b *testing.B) {
	L := NewState()
	defer L.Close()
	nodeType := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("value", typ.Number).
			OptField("next", self).
			Build()
	})
	rt := &LType{inner: nodeType}
	n3 := L.NewTable()
	n3.RawSetString("value", LNumber(3))
	n2 := L.NewTable()
	n2.RawSetString("value", LNumber(2))
	n2.RawSetString("next", n3)
	n1 := L.NewTable()
	n1.RawSetString("value", LNumber(1))
	n1.RawSetString("next", n2)
	for b.ResetTimer(); b.N > 0; b.N-- {
		rt.Validate(L, n1)
	}
}

// ---------------------------------------------------------------------------
// Ref resolution benchmark
// ---------------------------------------------------------------------------

func BenchmarkValidate_RefResolution(b *testing.B) {
	L := NewState()
	defer L.Close()
	resolver := &typeResolver{
		types: map[string]typ.Type{
			"Status": typ.NewUnion(typ.LiteralString("active"), typ.LiteralString("draft")),
		},
	}
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			OptField("status", typ.NewRef("", "Status")).
			Build(),
		resolver: resolver,
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LString("abc"))
	tbl.RawSetString("status", LString("active"))
	for b.ResetTimer(); b.N > 0; b.N-- {
		rec.Validate(L, tbl)
	}
}

// ---------------------------------------------------------------------------
// Failure path benchmarks (how expensive are errors?)
// ---------------------------------------------------------------------------

func BenchmarkValidate_Fail_TypeMismatch(b *testing.B) {
	L := NewState()
	defer L.Close()
	for b.ResetTimer(); b.N > 0; b.N-- {
		LTypeNumber.Validate(L, LString("bad"))
	}
}

func BenchmarkIs_Fail_TypeMismatch(b *testing.B) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)
	isMethod := L.typeGetField(LTypeNumber, "is")
	for b.ResetTimer(); b.N > 0; b.N-- {
		L.Push(isMethod)
		L.Push(LString("bad"))
		L.Call(1, 2)
		L.Pop(2)
	}
}

func BenchmarkIs_Fail_MissingField(b *testing.B) {
	L := NewState()
	defer L.Close()
	OpenErrors(L)
	rec := &LType{
		inner: typ.NewRecord().
			Field("id", typ.String).
			Field("name", typ.String).
			Build(),
	}
	tbl := L.NewTable()
	tbl.RawSetString("id", LString("abc"))
	isMethod := L.typeGetField(rec, "is")
	for b.ResetTimer(); b.N > 0; b.N-- {
		L.Push(isMethod)
		L.Push(tbl)
		L.Call(1, 2)
		L.Pop(2)
	}
}
