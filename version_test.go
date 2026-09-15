package main

import "testing"

func TestIsVersionCommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"裸 version", []string{"version"}, true},
		{"无参", nil, false},
		{"仅无值 flag", []string{"-headless"}, false},
		{"-input 取值后跟 version（取值不算位置参数）", []string{"-input", "hi", "version"}, true},
		{"version 不是首个位置参数", []string{"session", "version"}, false},
		{"无值 flag 后跟 version", []string{"-debugger", "version"}, true},
		{"带值 flag 的取值不算位置参数", []string{"-mode", "minimal", "version"}, true},
		{"setup 不是 version", []string{"setup"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isVersionCommand(c.args); got != c.want {
				t.Errorf("isVersionCommand(%q) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	// VERSION 文件为版本号单一来源；输出须为 v 前缀的规范化版本号，
	// 断言前缀而非具体值，避免每次迭代版本号都要同步改测试。
	v := versionString()
	if v == "" {
		t.Fatal("versionString() 为空：VERSION 文件嵌入内容缺失")
	}
	if len(v) < 2 || v[0] != 'v' {
		t.Errorf("versionString() = %q, 期望 v 前缀的规范化版本号", v)
	}
}
