package lua

import (
	"fmt"
	"testing"
)

func TestLStringFormat(t *testing.T) {
	tests := []struct {
		input    LString
		format   string
		expected string
	}{
		{LString("42"), "%d", "42"},
		{LString("42"), "%i", "42"},
		{LString("hello"), "%s", "hello"},
		{LString("hello"), "%d", "hello"}, // non-numeric string with %d should stay as string
		{LString("3.14"), "%d", "3"},      // float string truncated to int
	}

	for _, tt := range tests {
		result := fmt.Sprintf(tt.format, tt.input)
		if result != tt.expected {
			t.Errorf("LString(%q) with format %q: expected %q, got %q",
				tt.input, tt.format, tt.expected, result)
		}
	}
}

func TestLValueType(t *testing.T) {
	tests := []struct {
		value    LValue
		expected LValueType
	}{
		{LNil, LTNil},
		{LTrue, LTBool},
		{LFalse, LTBool},
		{LNumber(1.5), LTNumber},
		{LInteger(42), LTInteger},
		{LString("hello"), LTString},
	}

	for _, tt := range tests {
		if tt.value.Type() != tt.expected {
			t.Errorf("Type of %v: expected %v, got %v",
				tt.value, tt.expected, tt.value.Type())
		}
	}
}

func TestLVIsFalse(t *testing.T) {
	tests := []struct {
		value    LValue
		expected bool
	}{
		{LNil, true},
		{LFalse, true},
		{LTrue, false},
		{LNumber(0), false},
		{LNumber(1), false},
		{LString(""), false},
		{LString("hello"), false},
	}

	for _, tt := range tests {
		if LVIsFalse(tt.value) != tt.expected {
			t.Errorf("LVIsFalse(%v): expected %v, got %v",
				tt.value, tt.expected, LVIsFalse(tt.value))
		}
	}
}

func TestLVAsBool(t *testing.T) {
	tests := []struct {
		value    LValue
		expected bool
	}{
		{LNil, false},
		{LFalse, false},
		{LTrue, true},
		{LNumber(0), true},
		{LNumber(1), true},
		{LString(""), true},
		{LString("hello"), true},
	}

	for _, tt := range tests {
		if LVAsBool(tt.value) != tt.expected {
			t.Errorf("LVAsBool(%v): expected %v, got %v",
				tt.value, tt.expected, LVAsBool(tt.value))
		}
	}
}

func TestLVAsString(t *testing.T) {
	tests := []struct {
		value    LValue
		expected string
	}{
		{LString("hello"), "hello"},
		{LNumber(42), "42"},
		{LNumber(3.14), "3.14"},
		{LInteger(100), "100"},
		{LNil, ""},
		{LTrue, ""},
		{LFalse, ""},
	}

	for _, tt := range tests {
		if LVAsString(tt.value) != tt.expected {
			t.Errorf("LVAsString(%v): expected %q, got %q",
				tt.value, tt.expected, LVAsString(tt.value))
		}
	}
}

func TestLVCanConvToString(t *testing.T) {
	tests := []struct {
		value    LValue
		expected bool
	}{
		{LString("hello"), true},
		{LNumber(42), true},
		{LInteger(100), true},
		{LNil, false},
		{LTrue, false},
		{LFalse, false},
	}

	for _, tt := range tests {
		if LVCanConvToString(tt.value) != tt.expected {
			t.Errorf("LVCanConvToString(%v): expected %v, got %v",
				tt.value, tt.expected, LVCanConvToString(tt.value))
		}
	}
}

func TestLVAsNumber(t *testing.T) {
	tests := []struct {
		value    LValue
		expected LNumber
	}{
		{LNumber(42), LNumber(42)},
		{LNumber(3.14), LNumber(3.14)},
		{LInteger(100), LNumber(100)},
		{LString("42"), LNumber(42)},
		{LString("3.14"), LNumber(3.14)},
		{LString("hello"), LNumber(0)},
		{LNil, LNumber(0)},
		{LTrue, LNumber(0)},
	}

	for _, tt := range tests {
		result := LVAsNumber(tt.value)
		if result != tt.expected {
			t.Errorf("LVAsNumber(%v): expected %v, got %v",
				tt.value, tt.expected, result)
		}
	}
}

func TestLNumberFormat(t *testing.T) {
	tests := []struct {
		input    LNumber
		format   string
		expected string
	}{
		{LNumber(42), "%d", "42"},
		{LNumber(42), "%i", "42"},
		{LNumber(3.14), "%f", "3.140000"},
		{LNumber(3.14), "%.2f", "3.14"},
		{LNumber(42), "%x", "2a"},
		{LNumber(42), "%s", "42"},
	}

	for _, tt := range tests {
		result := fmt.Sprintf(tt.format, tt.input)
		if result != tt.expected {
			t.Errorf("LNumber(%v) with format %q: expected %q, got %q",
				tt.input, tt.format, tt.expected, result)
		}
	}
}

func TestLValueTypeString(t *testing.T) {
	tests := []struct {
		vt       LValueType
		expected string
	}{
		{LTNil, "nil"},
		{LTBool, "boolean"},
		{LTNumber, "number"},
		{LTInteger, "number"},
		{LTString, "string"},
		{LTFunction, "function"},
		{LTUserData, "userdata"},
		{LTThread, "thread"},
		{LTTable, "table"},
		{LTChannel, "channel"},
		{LTType, "type"},
	}

	for _, tt := range tests {
		if tt.vt.String() != tt.expected {
			t.Errorf("LValueType(%d).String(): expected %q, got %q",
				tt.vt, tt.expected, tt.vt.String())
		}
	}
}
