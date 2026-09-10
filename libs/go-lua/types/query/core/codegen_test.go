package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestDispatchKindString(t *testing.T) {
	tests := []struct {
		kind   DispatchKind
		expect string
	}{
		{DispatchDynamic, "dynamic"},
		{DispatchMono, "mono"},
		{DispatchPoly, "poly"},
		{DispatchMega, "mega"},
		{DispatchKind(99), "dynamic"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if tt.kind.String() != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, tt.kind.String())
			}
		})
	}
}

func TestLayoutString(t *testing.T) {
	tests := []struct {
		layout Layout
		expect string
	}{
		{LayoutHash, "hash"},
		{LayoutFlat, "flat"},
		{LayoutStruct, "struct"},
		{Layout(99), "hash"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if tt.layout.String() != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, tt.layout.String())
			}
		})
	}
}

func TestGetDispatch(t *testing.T) {
	tests := []struct {
		name   string
		t      typ.Type
		expect DispatchKind
	}{
		{"nil type", nil, DispatchDynamic},
		{"concrete string", typ.String, DispatchMono},
		{"concrete integer", typ.Integer, DispatchMono},
		{"concrete record", typ.NewRecord().Build(), DispatchMono},
		{"any type", typ.Any, DispatchDynamic},
		{"never type", typ.Never, DispatchDynamic},
		{"unknown type", typ.Unknown, DispatchDynamic},
		{"interface type", typ.NewInterface("I", nil), DispatchDynamic},
		{"small union 2", typ.NewUnion(typ.String, typ.Integer), DispatchPoly},
		{"small union 4", typ.NewUnion(typ.String, typ.Integer, typ.Boolean, typ.Number), DispatchPoly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDispatch(tt.t)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestGetDispatchLargeUnion(t *testing.T) {
	members := []typ.Type{
		typ.String, typ.Integer, typ.Boolean, typ.Number,
		typ.NewRecord().Build(),
	}

	union := typ.NewUnion(members...)
	if u, ok := union.(*typ.Union); ok && len(u.Members) > 4 {
		result := GetDispatch(union)
		if result != DispatchMega {
			t.Errorf("expected Mega for large union, got %v", result)
		}
	}
}

func TestIsConcreteType(t *testing.T) {
	tests := []struct {
		name   string
		t      typ.Type
		expect bool
	}{
		{"nil type", nil, false},
		{"string", typ.String, true},
		{"integer", typ.Integer, true},
		{"record", typ.NewRecord().Build(), true},
		{"array", typ.NewArray(typ.String), true},
		{"function", typ.Func().Build(), true},
		{"any", typ.Any, false},
		{"never", typ.Never, false},
		{"unknown", typ.Unknown, false},
		{"union", typ.NewUnion(typ.String, typ.Integer), false},
		{"intersection", typ.NewIntersection(typ.NewRecord().Field("a", typ.String).Build(), typ.NewRecord().Field("b", typ.Integer).Build()), false},
		{"interface", typ.NewInterface("I", nil), false},
		{"typevar", typ.NewTypeVar(0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsConcreteType(tt.t) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestCanElideNilCheck(t *testing.T) {
	tests := []struct {
		name   string
		t      typ.Type
		expect bool
	}{
		{"nil type", nil, false},
		{"string", typ.String, true},
		{"integer", typ.Integer, true},
		{"optional", typ.NewOptional(typ.String), false},
		{"any", typ.Any, false},
		{"nil singleton", typ.Nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanElideNilCheck(tt.t) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestCanElideTypeCheck(t *testing.T) {
	tests := []struct {
		name   string
		actual typ.Type
		target typ.Type
		expect bool
	}{
		{"nil actual", nil, typ.String, false},
		{"nil target", typ.String, nil, false},
		{"both nil", nil, nil, false},
		{"same primitive string", typ.String, typ.String, true},
		{"same primitive integer", typ.Integer, typ.Integer, true},
		{"same primitive number", typ.Number, typ.Number, true},
		{"same primitive boolean", typ.Boolean, typ.Boolean, true},
		{"same nil type", typ.Nil, typ.Nil, true},
		{"different kinds", typ.String, typ.Integer, false},
		{"record types", typ.NewRecord().Build(), typ.NewRecord().Build(), false},
		{"array types", typ.NewArray(typ.String), typ.NewArray(typ.String), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanElideTypeCheck(tt.actual, tt.target) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestGetLayout(t *testing.T) {
	tests := []struct {
		name   string
		t      typ.Type
		expect Layout
	}{
		{"nil type", nil, LayoutHash},
		{"array", typ.NewArray(typ.String), LayoutFlat},
		{"record", typ.NewRecord().Build(), LayoutStruct},
		{"map", typ.NewMap(typ.String, typ.Integer), LayoutHash},
		{"string", typ.String, LayoutHash},
		{"tuple", typ.NewTuple(typ.String), LayoutHash},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLayout(tt.t)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}
