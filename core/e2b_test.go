package core

import (
	"os"
	"testing"
)

func TestE2BNoApiKey(t *testing.T) {
	// 确保无 API key
	os.Unsetenv("DSC_E2B_API_KEY")

	sb := NewE2BSandbox()
	if sb.apiKey != "" {
		t.Error("apiKey should be empty when env not set")
	}
}

func TestE2BSandboxID(t *testing.T) {
	sb := &E2BSandbox{}
	if sb.SandboxID() != "" {
		t.Error("sandboxID should be empty initially")
	}
}

func TestEscapeJSON(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{`hello "world"`, `hello \"world\"`},
		{"hello\\world", "hello\\\\world"},
		{"line1\nline2", "line1\\nline2"},
		{"tab\there", "tab\\there"},
	}
	for _, c := range cases {
		if got := escapeJSON(c.input); got != c.want {
			t.Errorf("escapeJSON(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
