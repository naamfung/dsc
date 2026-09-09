package join

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func BenchmarkTypes_TwoTypes(b *testing.B) {
	t1 := typ.String
	t2 := typ.Number
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(t1, t2)
	}
}

func BenchmarkTypes_FiveTypes(b *testing.B) {
	types := []typ.Type{typ.String, typ.Number, typ.Boolean, typ.Integer, typ.Nil}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(types...)
	}
}

func BenchmarkTypes_AllSame(b *testing.B) {
	types := make([]typ.Type, 10)
	for i := range types {
		types[i] = typ.String
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(types...)
	}
}

func BenchmarkTypes_WithUnknown(b *testing.B) {
	types := []typ.Type{typ.String, typ.Unknown, typ.Number, typ.Unknown, typ.Boolean}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(types...)
	}
}

func BenchmarkCoalesceMaps_TwoMaps(b *testing.B) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	types := []typ.Type{m1, m2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CoalesceMaps(types)
	}
}

func BenchmarkCoalesceMaps_FiveMaps(b *testing.B) {
	types := []typ.Type{
		typ.NewMap(typ.String, typ.Number),
		typ.NewMap(typ.Integer, typ.Boolean),
		typ.NewMap(typ.Boolean, typ.String),
		typ.NewMap(typ.Number, typ.Integer),
		typ.NewMap(typ.String, typ.Boolean),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CoalesceMaps(types)
	}
}

func BenchmarkCoalesceRecordOpenness_Mixed(b *testing.B) {
	open := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	closed := typ.NewRecord().Field("y", typ.String).Build()
	types := []typ.Type{open, closed, open, closed}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CoalesceRecordOpenness(types)
	}
}

func BenchmarkCoalesceRecordMapComponents_TwoRecords(b *testing.B) {
	r1 := typ.NewRecord().Field("x", typ.Number).MapComponent(typ.String, typ.Number).Build()
	r2 := typ.NewRecord().Field("x", typ.Number).MapComponent(typ.String, typ.Boolean).Build()
	types := []typ.Type{r1, r2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CoalesceRecordMapComponents(types)
	}
}

func BenchmarkTypes_ComplexRecords(b *testing.B) {
	r1 := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		OptField("email", typ.String).
		Build()
	r2 := typ.NewRecord().
		Field("name", typ.String).
		Field("id", typ.Integer).
		OptField("phone", typ.String).
		Build()
	types := []typ.Type{r1, r2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(types...)
	}
}

func BenchmarkTypes_NestedUnions(b *testing.B) {
	inner1 := typ.NewUnion(typ.String, typ.Number)
	inner2 := typ.NewUnion(typ.Boolean, typ.Integer)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Types(inner1, inner2)
	}
}

func BenchmarkFilterUnknown_HalfUnknown(b *testing.B) {
	types := make([]typ.Type, 20)
	for i := range types {
		if i%2 == 0 {
			types[i] = typ.Unknown
		} else {
			types[i] = typ.String
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filterUnknown(types)
	}
}

func BenchmarkCoalesceEmptyRecordWithMap(b *testing.B) {
	m := typ.NewMap(typ.String, typ.Number)
	empty := typ.NewRecord().Build()
	types := []typ.Type{m, empty, typ.String, empty}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CoalesceEmptyRecordWithMap(types)
	}
}
