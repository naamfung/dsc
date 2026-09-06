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

// TestFallbackExternalStillPaths 未命中的命令仍回退外部 PATH（空 PATH 下应 127）。
func TestFallbackExternalStillPaths(t *testing.T) {
	dir := t.TempDir()
	_, err := runShell(t, dir, "definitely_not_a_real_cmd_xyz")
	if code := exitCode(t, err); code != 127 {
		t.Fatalf("未命中命令应回退 PATH 并返回 127，got %d", code)
	}
}
