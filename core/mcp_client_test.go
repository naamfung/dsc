package core

import (
	"testing"
)

func TestMCPPublicName(t *testing.T) {
	cases := []struct {
		serverName, rawName, want string
	}{
		{"myserver", "get_weather", "mcp__myserver__get_weather"},
		{"fs", "read_file", "mcp__fs__read_file"},
		{"db", "query", "mcp__db__query"},
	}
	for _, c := range cases {
		if got := mcpPublicName(c.serverName, c.rawName); got != c.want {
			t.Errorf("mcpPublicName(%q, %q) = %q, want %q", c.serverName, c.rawName, got, c.want)
		}
	}
}

func TestMCPPublicNameNormalization(t *testing.T) {
	// 含非法字符的 rawName 应被规范化
	got := mcpPublicName("srv", "tool.name")
	if got != "mcp__srv__tool_name" {
		t.Errorf("mcpPublicName with dot = %q, want mcp__srv__tool_name", got)
	}
}

func TestMCPPublicNameTruncation(t *testing.T) {
	// 超长名应截断到 64 字符
	longName := "mcp__srv__" + string(make([]byte, 100))
	for i := range longName {
		if longName[i] == 0 {
			longName = longName[:i] + "a" + longName[i+1:]
		}
	}
	longName = "mcp__srv__" + repeatStr("a", 100)
	got := mcpPublicName("srv", repeatStr("a", 100))
	if len(got) > 64 {
		t.Errorf("mcpPublicName should be truncated to 64 chars, got %d", len(got))
	}
}

func TestMCPServerNameValid(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"myserver", true},
		{"fs-tools", true},
		{"db_v2", true},
		{"", false},
		{"a", true},
		{repeatStr("a", 33), false}, // too long
		{"bad name", false},         // space
		{"bad.name", false},         // dot
	}
	for _, c := range cases {
		if got := isMCPServerNameValid(c.name); got != c.valid {
			t.Errorf("isMCPServerNameValid(%q) = %v, want %v", c.name, got, c.valid)
		}
	}
}

func TestMCPNormalizeName(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello_world", "hello_world"},
		{"hello.world", "hello_world"},
		{"hello world", "hello_world"},
		{"hello/world", "hello_world"},
	}
	for _, c := range cases {
		if got := mcpNormalizeName(c.input); got != c.want {
			t.Errorf("mcpNormalizeName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
