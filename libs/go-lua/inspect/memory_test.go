package inspect

import (
	"testing"

	lua "github.com/wippyai/go-lua"
)

func TestMemoryStatsEmpty(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	stats := GetMemoryStats(L)
	if stats.Total <= 0 {
		t.Error("Expected non-zero memory for empty state")
	}
}

func TestMemoryStatsWithLibs(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	statsWithLibs := GetMemoryStats(L)

	L2 := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L2.Close()

	statsNoLibs := GetMemoryStats(L2)

	if statsWithLibs.Total <= statsNoLibs.Total {
		t.Error("State with libs should use more memory than without")
	}
}

func TestMemoryStatsWithData(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	statsBefore := GetMemoryStats(L)

	script := `
		data = {}
		for i = 1, 1000 do
			data[i] = "string value " .. i
		end
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	statsAfter := GetMemoryStats(L)

	if statsAfter.Tables <= statsBefore.Tables {
		t.Error("Expected table memory to increase")
	}
	if statsAfter.Strings <= statsBefore.Strings {
		t.Error("Expected string memory to increase")
	}
	if statsAfter.StringBytes < 15000 {
		t.Errorf("Expected at least 15KB of string data, got %d", statsAfter.StringBytes)
	}
}

func TestMemoryStatsWithFunctions(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	statsBefore := GetMemoryStats(L)

	script := `
		function factorial(n)
			if n <= 1 then return 1 end
			return n * factorial(n - 1)
		end

		function fibonacci(n)
			if n < 2 then return n end
			return fibonacci(n-1) + fibonacci(n-2)
		end

		funcs = {factorial, fibonacci}
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	statsAfter := GetMemoryStats(L)

	if statsAfter.Functions <= statsBefore.Functions {
		t.Error("Expected function memory to increase")
	}
}

func TestGetMemorySize(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	size := GetMemorySize(L)
	if size <= 0 {
		t.Error("Expected positive memory size")
	}
}

func BenchmarkGetMemoryStats(b *testing.B) {
	L := lua.NewState()
	defer L.Close()

	script := `
		data = {}
		for i = 1, 100 do
			data[i] = {
				name = "item" .. i,
				value = i * 1.5,
				active = i % 2 == 0
			}
		end
	`
	_ = L.DoString(script)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GetMemoryStats(L)
	}
}
