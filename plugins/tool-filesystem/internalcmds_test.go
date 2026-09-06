package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// runShell 以「空 PATH」构造 shell runner 并执行脚本，以此证明 internalcmds 是进程内
// 实现、不依赖外部可执行文件（在 Windows 且 PATH 被宿主过滤时仍可用）。
// 返回 stdout 与 Run 返回的 error（经 interp.IsExitStatus 可判退出码）。
func runShell(t *testing.T, dir, script string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	r, err := interp.New(
		interp.Env(expand.ListEnviron()), // 空环境：无 PATH
		interp.Dir(dir),
		interp.StdIO(strings.NewReader(""), &out, &errb),
		interp.ExecHandler(shellExecHandler),
	)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatalf("parse %q: %v", script, err)
	}
	err = r.Run(context.Background(), f)
	if errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return out.String(), err
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	if c, ok := interp.IsExitStatus(err); ok {
		return int(c)
	}
	t.Fatalf("unexpected error: %v", err)
	return -1
}

// TestInternalMkdirLsSetup 空 PATH 下 mkdir/ls 建目录与列出。
func TestInternalMkdirLsSetup(t *testing.T) {
	dir := t.TempDir()
	// 注意：tool-filesystem 报错信息假定 -r 为递归旗标，这里用 mkdir -p 建多级目录
	out, err := runShell(t, dir, "mkdir -p a/b/c && ls a && ls -a && ls -l a")
	if exitCode(t, err) != 0 {
		t.Fatalf("mkdir/ls 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "b") {
		t.Errorf("ls a 应列出 b: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "c")); err != nil {
		t.Errorf("多级目录创建失败: %v", err)
	}
}

// TestInternalFileFlow 空 PATH 下 touch/cat/echo 写读。
func TestInternalFileFlow(t *testing.T) {
	dir := t.TempDir()
	out, err := runShell(t, dir, "mkdir -p w && cd w && printf 'hello world\\n' > f.txt && cat f.txt && test -f f.txt")
	if exitCode(t, err) != 0 {
		t.Fatalf("touch/cat 失败: out=%q err=%v", out, err)
	}
	if got := strings.TrimRight(out, "\n"); got != "hello world" {
		t.Errorf("cat 输出异常: %q", got)
	}
}

// TestInternalGrepHeadTailWc 空 PATH 下 grep/head/tail/wc。
func TestInternalGrepHeadTailWc(t *testing.T) {
	dir := t.TempDir()
	out, err := runShell(t, dir,
		`mkdir -p g && cd g && printf 'line one\nline two\nFOO\n' > data.txt && grep -i -n foo data.txt && wc -l data.txt && head -n 1 data.txt && tail -n 1 data.txt`)
	if exitCode(t, err) != 0 {
		t.Fatalf("grep/head/tail/wc 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "3:FOO") {
		t.Errorf("grep -n 未命中第 3 行 FOO: %q", out)
	}
	if !strings.Contains(out, "3 data.txt") {
		t.Errorf("wc -l 应为 3 行: %q", out)
	}
	if !strings.Contains(out, "line one") || !strings.Contains(out, "FOO") {
		t.Errorf("head/tail 输出异常: %q", out)
	}
}

// TestInternalCpMvRm 空 PATH 下 cp/mv/rm。
func TestInternalCpMvRm(t *testing.T) {
	dir := t.TempDir()
	out, err := runShell(t, dir,
		`mkdir -p s && printf 'abc' > s/f.txt && cp s/f.txt copy.txt && mv copy.txt moved.txt && cp -r s s2 && test -f moved.txt && test -f s2/f.txt && rm -r s2 && test ! -e s2`)
	if exitCode(t, err) != 0 {
		t.Fatalf("cp/mv/rm 失败: out=%q err=%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "moved.txt")); err != nil {
		t.Errorf("mv 后 moved.txt 应存在: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "copy.txt")); err == nil {
		t.Error("mv 后 copy.txt 应已被移除")
	}
	if _, err := os.Stat(filepath.Join(dir, "s2")); err == nil {
		t.Error("rm -r 后 s2 应被删除")
	}
}

// TestPipeSupport 验证内部命令之间的管道（进程内）正常工作：
// 上游输出经 interp 管道流入下游，下游从 hc.Stdin 读取并过滤。
func TestPipeSupport(t *testing.T) {
	dir := t.TempDir()
	// internal | internal：printf(内建) 经管道喂给 grep(内部)
	out, err := runShell(t, dir, "printf 'dd.txt\\nmem.txt\\nother.txt\\n' | grep -i mem")
	if exitCode(t, err) != 0 {
		t.Fatalf("printf|grep 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "mem.txt") {
		t.Errorf("grep 应命中 mem.txt: %q", out)
	}
	if strings.Contains(out, "other.txt") {
		t.Errorf("grep 应过滤掉 other.txt: %q", out)
	}

	// internal | internal：cat(内部) 读文件经管道喂给 grep(内部)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\nfoo bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out2, err2 := runShell(t, dir, "cat a.txt | grep foo")
	if exitCode(t, err2) != 0 {
		t.Fatalf("cat|grep 失败: out=%q err=%v", out2, err2)
	}
	if !strings.Contains(out2, "foo bar") {
		t.Errorf("cat|grep 应输出 foo bar 行: %q", out2)
	}

	// 多段管道：internal | internal | internal
	out3, err3 := runShell(t, dir, "printf 'a\\nb\\na\\n' | grep a | wc -l")
	if exitCode(t, err3) != 0 {
		t.Fatalf("多段管道失败: out=%q err=%v", out3, err3)
	}
	if !strings.Contains(out3, "2") {
		t.Errorf("wc 应统计 2 行命中: %q", out3)
	}
}

// TestPosixHintForWindowsCommands 误用 Windows 专属命令时，报错应附带标准 POSIX 替代指引，
// 引导模型回到 POSIX 命令；同时确认未加兼容实现（管道直通不因此扩展）。
func TestPosixHintForWindowsCommands(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range []string{"dir", "findstr", "copy", "del", "move", "ipconfig", "where"} {
		if hint := posixHint(cmd); hint == "" || !strings.Contains(hint, "POSIX") {
			t.Errorf("命令 %q 应给出 POSIX 替代提示，got %q", cmd, hint)
			continue
		}
		// 通过真实 runner 确认执行失败并带上提示。
		_, err := runShell(t, dir, cmd)
		if err == nil {
			t.Errorf("命令 %q 不应成功（Windows 专属、未加兼容实现）", cmd)
		} else if !strings.Contains(err.Error(), "POSIX") {
			t.Errorf("命令 %q 的执行错误应包含 POSIX 引导，got %v", cmd, err)
		}
	}
	// 若未来误加入 dir 这类兼容实现，此测试会失败（dir 应仍报未找到并引导）。
	if _, ok := internalCommands["dir"]; ok {
		t.Error("不应为 dir 提供进程内兼容实现")
	}
}
