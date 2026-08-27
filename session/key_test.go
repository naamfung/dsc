package session

import "testing"

// TestSessionKeyForProject 校验项目根路径 → 会话文件名的转换：
// 同项目同名、跨项目隔离，绝不落入硬编码 default.jsonl（空路径除外）。
func TestSessionKeyForProject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"windows abs", `C:\Users\Administrator\Desktop\DeepClean`, "C--Users-Administrator-Desktop-DeepClean"},
		{"git bash /mnt/c", `/mnt/c/Users/Administrator/Desktop/DeepClean`, "mnt-c-Users-Administrator-Desktop-DeepClean"},
		{"unix home", `/home/jor/DeepClean`, "home-jor-DeepClean"},
		{"unix root", `/`, "default"},
		{"empty", ``, "default"},
		{"with illegal chars", `C:\Users\a\b?c*d`, "C--Users-a-b-c-d"},
	}
	for _, c := range cases {
		if got := SessionKeyForProject(c.in); got != c.want {
			t.Errorf("SessionKeyForProject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
