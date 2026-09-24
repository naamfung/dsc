package dsc

import (
	"strings"
	"testing"
)

// TestQualifyToolName 钉死工具名的来源前缀约定：`<来源名>_<注册名>`。
// 前缀必须与来源名逐字相同——模型靠它去 `list_*_tools` 里对上号，任何改写都会断链。
func TestQualifyToolName(t *testing.T) {
	cases := []struct {
		unit, name, want string
	}{
		{"hello", "greet", "hello_greet"},
		{"example", "ping", "example_ping"},
		{"tool-sql-host", "note", "tool-sql-host_note"},
		{"a_b", "c", "a_b_c"},
	}
	for _, c := range cases {
		got, err := QualifyToolName(c.unit, c.name)
		if err != nil {
			t.Fatalf("QualifyToolName(%q, %q) 报错: %v", c.unit, c.name, err)
		}
		if got != c.want {
			t.Errorf("QualifyToolName(%q, %q) = %q, want %q", c.unit, c.name, got, c.want)
		}
	}
}

// TestQualifyToolNameRejects 覆盖非法输入：形态不合法一律报错而非就地改写
// （改写会让前缀与来源名不再逐字相同，模型的归属判断随之失效）。
func TestQualifyToolNameRejects(t *testing.T) {
	bad := []struct{ unit, name string }{
		{"", "greet"},          // 来源名缺省
		{"hello", ""},          // 注册名缺省
		{"my script", "greet"}, // 来源名带空格
		{"my.script", "greet"}, // 来源名带点
		{"脚本", "greet"},        // 来源名非 ASCII
		{"hello", "my tool"},   // 注册名带空格
		{"hello", "a.b"},       // 注册名带点
		{"hello", "工具"},        // 注册名非 ASCII
		// 合成结果超长（60 + 1 + 10 = 71 > ToolNameMaxLen）
		{strings.Repeat("u", 60), strings.Repeat("n", 10)},
	}
	for _, c := range bad {
		if _, err := QualifyToolName(c.unit, c.name); err == nil {
			t.Errorf("QualifyToolName(%q, %q) 应报错，却通过了", c.unit, c.name)
		}
	}
}

// TestQualifyToolNameLengthBoundary 覆盖长度上限的边界：正好 ToolNameMaxLen 通过，
// 多一字符被拒（上限存在的意义就是卡在这一字符上）。
func TestQualifyToolNameLengthBoundary(t *testing.T) {
	unit := "u"
	name := strings.Repeat("n", ToolNameMaxLen-len(unit)-1) // unit + "_" + name = ToolNameMaxLen
	if got, err := QualifyToolName(unit, name); err != nil || len(got) != ToolNameMaxLen {
		t.Fatalf("边界长度应通过：len=%d err=%v", len(got), err)
	}
	if _, err := QualifyToolName(unit, name+"n"); err == nil {
		t.Fatal("超出一字符应被拒")
	}
}
