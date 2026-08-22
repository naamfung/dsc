package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestMethod(t *testing.T) {
	methodFn := typ.Func().Param("x", typ.Integer).Returns(typ.String).Build()
	metatable := typ.NewRecord().Field("add", methodFn).Build()
	rec := typ.NewRecord().Field("value", typ.Number).Metatable(metatable).Build()
	recNoMeta := typ.NewRecord().Field("value", typ.Number).Build()
	iface := typ.NewInterface("Iface", []typ.Method{
		{Name: "add", Type: methodFn},
	})

	tests := []struct {
		name       string
		t          typ.Type
		methodName string
		found      bool
	}{
		{"nil type", nil, "x", false},
		{"record with metatable method", rec, "add", true},
		{"interface method", iface, "add", true},
		{"record missing method", rec, "sub", false},
		{"record without metatable", recNoMeta, "add", false},
		{"function type", typ.Func().Build(), "call", false},
		{"builtin table marker", typ.NewInterface("table", nil), "anything", true},
		{"any type", typ.Any, "anything", true},
		{"unknown type", typ.Unknown, "anything", true},
		{"never type", typ.Never, "anything", true},
		{"ref type", typ.NewRef("mod", "Type"), "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := Method(tt.t, tt.methodName)
			if ok != tt.found {
				t.Errorf("expected found=%v, got %v", tt.found, ok)
			}
		})
	}
}

func TestMethodUnion(t *testing.T) {
	methodFn := typ.Func().Returns(typ.String).Build()
	meta1 := typ.NewRecord().Field("show", methodFn).Build()
	meta2 := typ.NewRecord().Field("show", methodFn).Build()
	rec1 := typ.NewRecord().Metatable(meta1).Build()
	rec2 := typ.NewRecord().Metatable(meta2).Build()
	union := typ.NewUnion(rec1, rec2)

	t.Run("common method in union", func(t *testing.T) {
		_, ok := Method(union, "show")
		if !ok {
			t.Error("expected to find common method in union")
		}
	})

	t.Run("empty union", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}

		result, ok := Method(emptyUnion, "x")
		if ok || result != nil {
			t.Error("expected nil result for empty union")
		}
	})

	t.Run("union with different method signatures", func(t *testing.T) {
		differentMeta := typ.NewRecord().Field("show", typ.Func().Returns(typ.Integer).Build()).Build()
		rec3 := typ.NewRecord().Metatable(differentMeta).Build()
		mixedUnion := typ.NewUnion(rec1, rec3)

		_, ok := Method(mixedUnion, "show")
		if ok {
			t.Error("expected not to find method with different signatures")
		}
	})
}

func TestMethodIntersection(t *testing.T) {
	methodFn := typ.Func().Returns(typ.String).Build()
	meta := typ.NewRecord().Field("foo", methodFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()
	inter := typ.NewIntersection(rec, typ.String)

	t.Run("method from record in intersection", func(t *testing.T) {
		_, ok := Method(inter, "foo")
		if !ok {
			t.Error("expected to find method from intersection member")
		}
	})
}

func TestMethodOptional(t *testing.T) {
	methodFn := typ.Func().Returns(typ.String).Build()
	meta := typ.NewRecord().Field("bar", methodFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()
	opt := typ.NewOptional(rec)

	t.Run("method on optional", func(t *testing.T) {
		_, ok := Method(opt, "bar")
		if !ok {
			t.Error("expected to find method through optional")
		}
	})
}

func TestMethodAlias(t *testing.T) {
	methodFn := typ.Func().Returns(typ.Number).Build()
	meta := typ.NewRecord().Field("compute", methodFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()
	alias := typ.NewAlias("MyRec", rec)

	t.Run("method through alias", func(t *testing.T) {
		_, ok := Method(alias, "compute")
		if !ok {
			t.Error("expected to find method through alias")
		}
	})
}

func TestMethodOpenRecordFallback(t *testing.T) {
	rec := typ.NewRecord().SetOpen(true).Build()

	mt, ok := Method(rec, "unknown_method")
	if !ok {
		t.Fatal("expected open record to allow unknown method")
	}
	if unwrap.Function(mt) == nil {
		t.Fatalf("expected open record fallback to be callable, got %v", mt)
	}
}

func TestMethodInstantiated(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	iface := typ.NewInterface("Box", []typ.Method{
		{Name: "get", Type: typ.Func().Param("self", typ.Self).Returns(tp).Build()},
	})
	gen := typ.NewGeneric("Box", []*typ.TypeParam{tp}, iface)
	inst := typ.Instantiate(gen, typ.String)

	mt, ok := Method(inst, "get")
	if !ok {
		t.Fatal("expected method on instantiated type")
	}

	fn, ok := mt.(*typ.Function)
	if !ok {
		t.Fatalf("expected function type, got %T", mt)
	}

	if len(fn.Returns) != 1 || fn.Returns[0] != typ.String {
		t.Fatalf("expected string return, got %v", fn.Returns)
	}
}

func TestHasMethod(t *testing.T) {
	methodFn := typ.Func().Build()
	meta := typ.NewRecord().Field("test", methodFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()

	tests := []struct {
		name   string
		t      typ.Type
		method string
		expect bool
	}{
		{"has method", rec, "test", true},
		{"no method", rec, "other", false},
		{"nil type", nil, "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasMethod(tt.t, tt.method) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

func TestFieldOrMethod(t *testing.T) {
	methodFn := typ.Func().Returns(typ.String).Build()
	meta := typ.NewRecord().Field("method", methodFn).Build()
	rec := typ.NewRecord().Field("field", typ.Number).Metatable(meta).Build()

	tests := []struct {
		name   string
		t      typ.Type
		lookup string
		found  bool
	}{
		{"find field", rec, "field", true},
		{"find method", rec, "method", true},
		{"find neither", rec, "missing", false},
		{"nil type", nil, "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := FieldOrMethod(tt.t, tt.lookup)
			if ok != tt.found {
				t.Errorf("expected found=%v, got %v", tt.found, ok)
			}
		})
	}
}

func TestCallable(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.Integer).Build()

	tests := []struct {
		name   string
		t      typ.Type
		expect bool
	}{
		{"function type", fn, true},
		{"nil type", nil, false},
		{"string type", typ.String, false},
		{"record type", typ.NewRecord().Build(), false},
		{"optional function", typ.NewOptional(fn), true},
		{"alias to function", typ.NewAlias("F", fn), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := Callable(tt.t)
			if ok != tt.expect {
				t.Errorf("expected callable=%v, got %v", tt.expect, ok)
			}

			if tt.expect && result == nil {
				t.Error("expected non-nil function type")
			}
		})
	}
}

func TestCallableUnion(t *testing.T) {
	fn1 := typ.Func().Returns(typ.String).Build()
	fn2 := typ.Func().Returns(typ.Integer).Build()

	t.Run("union of functions", func(t *testing.T) {
		union := typ.NewUnion(fn1, fn2)

		_, ok := Callable(union)
		if !ok {
			t.Error("expected union of functions to be callable")
		}
	})

	t.Run("union with non-function", func(t *testing.T) {
		union := typ.NewUnion(fn1, typ.String)

		_, ok := Callable(union)
		if ok {
			t.Error("expected union with non-function to not be callable")
		}
	})

	t.Run("empty union", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}

		result, ok := Callable(emptyUnion)
		if ok || result != nil {
			t.Error("expected empty union to not be callable")
		}
	})
}

func TestCallableIntersection(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	rec := typ.NewRecord().Build()
	inter := typ.NewIntersection(fn, rec)

	t.Run("intersection with function", func(t *testing.T) {
		_, ok := Callable(inter)
		if !ok {
			t.Error("expected intersection with function to be callable")
		}
	})

	t.Run("intersection without function", func(t *testing.T) {
		noFn := typ.NewIntersection(typ.String, typ.Integer)

		_, ok := Callable(noFn)
		if ok {
			t.Error("expected intersection without function to not be callable")
		}
	})
}

// TestCallableWithMetaCall tests that records with __call metamethod are callable.
// Lua pattern: setmetatable({}, {__call = function(self, x) return x * 2 end})
// The resulting table should be callable like a function.
func TestCallableWithMetaCall(t *testing.T) {
	callFn := typ.Func().Param("self", typ.Any).Param("x", typ.Number).Returns(typ.Number).Build()
	meta := typ.NewRecord().Field("__call", callFn).Build()
	callable := typ.NewRecord().Field("value", typ.Number).Metatable(meta).Build()

	t.Run("record with __call is callable", func(t *testing.T) {
		result, ok := Callable(callable)
		if !ok {
			t.Error("record with __call metamethod should be callable")
		}

		if result == nil {
			t.Error("should return the __call function")
		}
	})

	t.Run("record without __call is not callable", func(t *testing.T) {
		plainMeta := typ.NewRecord().Field("__index", typ.Any).Build()
		notCallable := typ.NewRecord().Metatable(plainMeta).Build()

		_, ok := Callable(notCallable)
		if ok {
			t.Error("record without __call should not be callable")
		}
	})

	t.Run("optional record with __call", func(t *testing.T) {
		opt := typ.NewOptional(callable)

		result, ok := Callable(opt)
		if !ok {
			t.Error("optional of callable record should be callable")
		}

		if result == nil {
			t.Error("should return the __call function")
		}
	})
}

func TestGetMetamethod(t *testing.T) {
	indexFn := typ.Func().Returns(typ.Any).Build()
	meta := typ.NewRecord().Field("__index", indexFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()
	recNoMeta := typ.NewRecord().Build()

	tests := []struct {
		name  string
		t     typ.Type
		mm    string
		found bool
	}{
		{"record with metamethod", rec, "__index", true},
		{"record missing metamethod", rec, "__newindex", false},
		{"record no metatable", recNoMeta, "__index", false},
		{"nil type", nil, "__index", false},
		{"optional with metamethod", typ.NewOptional(rec), "__index", true},
		{"alias with metamethod", typ.NewAlias("R", rec), "__index", true},
		{"primitive type", typ.String, "__tostring", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := GetMetamethod(tt.t, tt.mm)
			if ok != tt.found {
				t.Errorf("expected found=%v, got %v", tt.found, ok)
			}
		})
	}
}

func TestHasMetamethod(t *testing.T) {
	callFn := typ.Func().Returns(typ.String).Build()
	meta := typ.NewRecord().Field("__call", callFn).Build()
	rec := typ.NewRecord().Metatable(meta).Build()

	tests := []struct {
		name   string
		t      typ.Type
		mm     string
		expect bool
	}{
		{"has metamethod", rec, "__call", true},
		{"no metamethod", rec, "__len", false},
		{"nil type", nil, "__call", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasMetamethod(tt.t, tt.mm) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}

// =============================================================================
// Module Pattern Tests (Common Lua Patterns)
// =============================================================================

// TestModule_CallableModule tests module tables that can be called directly.
//
// Lua pattern:
//
//	local json = require("json")
//	local result = json(data)  -- Module is callable via __call
func TestModule_CallableModule(t *testing.T) {
	// Module with __call for direct invocation
	encodeFn := typ.Func().Param("data", typ.Any).Returns(typ.String).Build()
	meta := typ.NewRecord().Field("__call", encodeFn).Build()
	jsonModule := typ.NewRecord().
		Field("encode", typ.Func().Param("data", typ.Any).Returns(typ.String).Build()).
		Field("decode", typ.Func().Param("s", typ.String).Returns(typ.Any).Build()).
		Metatable(meta).
		Build()

	t.Run("module is callable", func(t *testing.T) {
		result, ok := Callable(jsonModule)
		if !ok {
			t.Error("module with __call should be callable")
		}

		if result == nil {
			t.Error("should return __call function")
		}
	})

	t.Run("module fields accessible", func(t *testing.T) {
		encType, ok := Field(jsonModule, "encode")
		if !ok {
			t.Error("should find 'encode' field")
		}

		if encType == nil {
			t.Error("encode should not be nil")
		}
	})

	t.Run("module methods via metatable", func(t *testing.T) {
		// Some modules put methods in metatable
		validateFn := typ.Func().Param("data", typ.Any).Returns(typ.Boolean).Build()
		metaWithMethods := typ.NewRecord().
			Field("__call", encodeFn).
			Field("validate", validateFn).
			Build()
		moduleWithMetaMethods := typ.NewRecord().
			Metatable(metaWithMethods).
			Build()

		_, ok := Method(moduleWithMetaMethods, "validate")
		if !ok {
			t.Error("should find method in metatable")
		}
	})
}

// TestModule_ConstructorPattern tests the common Lua class/factory pattern.
//
// Lua pattern:
//
//	local Point = {}
//	Point.__index = Point
//	function Point.new(x, y)
//	  return setmetatable({x=x, y=y}, Point)
//	end
func TestModule_ConstructorPattern(t *testing.T) {
	// Instance type that new() returns
	instanceMeta := typ.NewRecord().Field("__index", typ.Any).Build()
	instance := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Metatable(instanceMeta).
		Build()

	// Factory function
	newFn := typ.Func().
		Param("x", typ.Number).
		Param("y", typ.Number).
		Returns(instance).
		Build()

	// Module/class table
	pointModule := typ.NewRecord().
		Field("new", newFn).
		Field("__index", typ.Any).
		Build()

	t.Run("module has constructor", func(t *testing.T) {
		newType, ok := Field(pointModule, "new")
		if !ok {
			t.Error("should find 'new' constructor")
		}

		fn, ok := Callable(newType)
		if !ok || fn == nil {
			t.Error("new should be callable")
		}
	})

	t.Run("constructor returns instance", func(t *testing.T) {
		newType, _ := Field(pointModule, "new")

		fn, _ := Callable(newType)
		if fn == nil {
			t.Fatal("constructor should be callable")
		}

		if len(fn.Returns) == 0 {
			t.Fatal("constructor should have return type")
		}

		retType := fn.Returns[0]
		_, hasX := Field(retType, "x")
		_, hasY := Field(retType, "y")

		if !hasX || !hasY {
			t.Error("constructor should return instance with x, y fields")
		}
	})
}

// TestModule_ExportTable tests the return-table pattern.
//
// Lua pattern:
//
//	local function private() end
//	local function public1() end
//	local function public2() end
//	return {
//	  public1 = public1,
//	  public2 = public2,
//	}
func TestModule_ExportTable(t *testing.T) {
	exportTable := typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Field("sub", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Field("VERSION", typ.String).
		Build()

	t.Run("exported functions accessible", func(t *testing.T) {
		addType, ok := Field(exportTable, "add")
		if !ok {
			t.Error("should find exported 'add'")
		}

		_, isCallable := Callable(addType)
		if !isCallable {
			t.Error("add should be callable")
		}
	})

	t.Run("exported constants accessible", func(t *testing.T) {
		verType, ok := Field(exportTable, "VERSION")
		if !ok {
			t.Error("should find exported 'VERSION'")
		}

		if verType != typ.String {
			t.Errorf("VERSION should be string, got %v", verType)
		}
	})

	t.Run("private not exported", func(t *testing.T) {
		_, ok := Field(exportTable, "private")
		if ok {
			t.Error("should not find private function")
		}
	})
}

// TestModule_InheritanceChain tests class-like inheritance.
//
// Lua pattern:
//
//	local Animal = {}
//	Animal.__index = Animal
//	function Animal:speak() end
//
//	local Dog = setmetatable({}, {__index = Animal})
//	Dog.__index = Dog
//	function Dog:bark() end
func TestModule_InheritanceChain(t *testing.T) {
	// Base class
	speakFn := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	animalMeta := typ.NewRecord().
		Field("__index", typ.Any).
		Field("speak", speakFn).
		Build()
	animal := typ.NewRecord().
		Field("name", typ.String).
		Metatable(animalMeta).
		Build()

	// Derived class - its metatable has __index pointing to Animal
	barkFn := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	dogMetaInner := typ.NewRecord().Field("__index", animal).Build()
	dogMeta := typ.NewRecord().
		Field("__index", typ.Any).
		Field("bark", barkFn).
		Metatable(dogMetaInner).
		Build()
	dog := typ.NewRecord().
		Field("breed", typ.String).
		Metatable(dogMeta).
		Build()

	t.Run("own method found", func(t *testing.T) {
		_, ok := Method(dog, "bark")
		if !ok {
			t.Error("should find Dog's own 'bark' method")
		}
	})

	t.Run("inherited method via __index chain", func(t *testing.T) {
		result, ok := Method(dog, "speak")
		if !ok {
			t.Error("expected to find inherited 'speak' method via __index chain")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}

// TestModule_NamespacedFunctions tests deeply nested module access.
//
// Lua pattern:
//
//	local lib = require("biglib")
//	lib.util.string.trim(s)
func TestModule_NamespacedFunctions(t *testing.T) {
	trimFn := typ.Func().Param("s", typ.String).Returns(typ.String).Build()
	stringUtil := typ.NewRecord().Field("trim", trimFn).Build()
	util := typ.NewRecord().Field("string", stringUtil).Build()
	lib := typ.NewRecord().Field("util", util).Build()

	t.Run("nested field access", func(t *testing.T) {
		// lib.util
		utilType, ok := Field(lib, "util")
		if !ok {
			t.Fatal("should find lib.util")
		}

		// lib.util.string
		stringType, ok := Field(utilType, "string")
		if !ok {
			t.Fatal("should find lib.util.string")
		}

		// lib.util.string.trim
		trimType, ok := Field(stringType, "trim")
		if !ok {
			t.Fatal("should find lib.util.string.trim")
		}

		_, isCallable := Callable(trimType)
		if !isCallable {
			t.Error("trim should be callable")
		}
	})
}
