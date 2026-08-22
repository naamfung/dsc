package bytecode

import (
	"errors"
	"testing"

	lua "github.com/wippyai/go-lua"
)

func TestDumpUndumpSimple(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `
		local x = 10
		local y = 20
		return x + y
	`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	proto := fn.Proto

	data, err := Dump(proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	if len(loaded.Code) != len(proto.Code) {
		t.Errorf("Code length mismatch: got %d, want %d", len(loaded.Code), len(proto.Code))
	}

	for i, c := range loaded.Code {
		if c != proto.Code[i] {
			t.Errorf("Code[%d] mismatch: got %d, want %d", i, c, proto.Code[i])
		}
	}

	if len(loaded.Constants) != len(proto.Constants) {
		t.Errorf("Constants length mismatch: got %d, want %d", len(loaded.Constants), len(proto.Constants))
	}
}

func TestDumpUndumpWithFunctions(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `
		function add(a, b)
			return a + b
		end

		function multiply(a, b)
			local result = a * b
			return result
		end

		return add(1, 2) + multiply(3, 4)
	`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	proto := fn.Proto

	data, err := Dump(proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	if len(loaded.FunctionPrototypes) != len(proto.FunctionPrototypes) {
		t.Errorf("Nested protos mismatch: got %d, want %d",
			len(loaded.FunctionPrototypes), len(proto.FunctionPrototypes))
	}
}

func TestDumpUndumpConstants(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `
		local s = "hello"
		local n = 3.14159
		local b = true
		local x = nil
		return s, n, b, x
	`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	proto := fn.Proto

	data, err := Dump(proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	for i, c := range loaded.Constants {
		if c.Type() != proto.Constants[i].Type() {
			t.Errorf("Constant[%d] type mismatch: got %v, want %v",
				i, c.Type(), proto.Constants[i].Type())
		}
		if c.String() != proto.Constants[i].String() {
			t.Errorf("Constant[%d] value mismatch: got %v, want %v",
				i, c.String(), proto.Constants[i].String())
		}
	}
}

func TestLoadAndExecute(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `
		local function fib(n)
			if n < 2 then return n end
			return fib(n-1) + fib(n-2)
		end
		return fib(10)
	`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	data, err := Dump(fn.Proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	L2 := lua.NewState()
	defer L2.Close()

	lfn := L2.NewFunctionFromProto(loaded)
	L2.Push(lfn)
	if err := L2.PCall(0, 1, nil); err != nil {
		t.Fatalf("PCall failed: %v", err)
	}

	result := L2.ToNumber(-1)
	if result != 55 {
		t.Errorf("Expected fib(10)=55, got %v", result)
	}
}

func TestInvalidBytecode(t *testing.T) {
	_, err := Undump([]byte{0, 0, 0, 0})
	if !errors.Is(err, ErrInvalidHeader) {
		t.Errorf("Expected ErrInvalidHeader, got %v", err)
	}

	_, err = Undump([]byte{0x43, 0x41, 0x55, 0x4C, 99})
	if !errors.Is(err, ErrVersionMismatch) {
		t.Errorf("Expected ErrVersionMismatch, got %v", err)
	}
}

func BenchmarkDump(b *testing.B) {
	L := lua.NewState()
	defer L.Close()

	script := `
		function process(data)
			local sum = 0
			for i = 1, #data do
				sum = sum + data[i]
			end
			return sum
		end
		return process
	`

	fn, _ := L.LoadString(script)
	proto := fn.Proto

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Dump(proto)
	}
}

func BenchmarkUndump(b *testing.B) {
	L := lua.NewState()
	defer L.Close()

	script := `
		function process(data)
			local sum = 0
			for i = 1, #data do
				sum = sum + data[i]
			end
			return sum
		end
		return process
	`

	fn, _ := L.LoadString(script)
	data, _ := Dump(fn.Proto)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Undump(data)
	}
}

func TestDumpUndumpWithTypeInfo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `return 1 + 2`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	proto := fn.Proto

	// Set some type info bytes
	typeInfo := []byte{0x4D, 0x41, 0x4E, 0x49, 0x02, 0x00, 0x00, 0x00, 0x00} // mock manifest header
	proto.SetTypeInfo(typeInfo)

	data, err := Dump(proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	loadedTypeInfo := loaded.GetTypeInfo()
	if len(loadedTypeInfo) != len(typeInfo) {
		t.Errorf("TypeInfo length mismatch: got %d, want %d", len(loadedTypeInfo), len(typeInfo))
	}

	for i, b := range loadedTypeInfo {
		if b != typeInfo[i] {
			t.Errorf("TypeInfo[%d] mismatch: got %d, want %d", i, b, typeInfo[i])
		}
	}
}

func TestDumpUndumpWithoutTypeInfo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	script := `return 1 + 2`

	fn, err := L.LoadString(script)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	proto := fn.Proto
	// No TypeInfo set

	data, err := Dump(proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}

	loaded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	if len(loaded.GetTypeInfo()) != 0 {
		t.Errorf("Expected empty TypeInfo, got %d bytes", len(loaded.GetTypeInfo()))
	}
}

func BenchmarkCompileVsUndump(b *testing.B) {
	script := `
		function factorial(n)
			if n <= 1 then return 1 end
			return n * factorial(n - 1)
		end

		function fibonacci(n)
			if n < 2 then return n end
			return fibonacci(n-1) + fibonacci(n-2)
		end

		return factorial(10) + fibonacci(10)
	`

	L := lua.NewState()
	fn, _ := L.LoadString(script)
	data, _ := Dump(fn.Proto)
	L.Close()

	b.Run("Compile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			L := lua.NewState()
			_, _ = L.LoadString(script)
			L.Close()
		}
	})

	b.Run("Undump", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = Undump(data)
		}
	})
}
