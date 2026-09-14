package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInternalLsClassifyAndSort 验证 ls 新增旗标（-F 分类符 / -t 按时间 / -S 按大小 /
// -r 逆序）与组合短选项（-laF 等）。
// 真实案例：模型习惯性 `ls -laF` 被拒（unsupported option: -laF），F 未实现所致。
func TestInternalLsClassifyAndSort(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.txt")
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(small, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 控制修改时间：small 比 big 新（-t 应把 small 排前）
	base := time.Now().Add(-time.Hour)
	if err := os.Chtimes(big, base, base); err != nil {
		t.Fatal(err)
	}

	// -F：目录追加 "/"
	out, err := runShell(t, dir, "ls -F")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls -F 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "sub/") {
		t.Errorf("ls -F 目录应带 / 分类符: %q", out)
	}

	// -laF 组合短选项：-l 长格式 + -F 分类符
	out, err = runShell(t, dir, "ls -laF")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls -laF 失败（组合短选项应支持）: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, " sub/\n") && !strings.HasSuffix(out, " sub/\n") {
		t.Errorf("ls -laF 应含 sub/ 条目: %q", out)
	}

	// -t：按修改时间新在前 → small(现在) 在 big(-1h) 之前
	out, err = runShell(t, dir, "ls -1 -t")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls -t 失败: out=%q err=%v", out, err)
	}
	if i := strings.Index(out, "small.txt"); i == -1 || strings.Index(out, "big.txt") == -1 || i > strings.Index(out, "big.txt") {
		t.Errorf("ls -t 应 small.txt 在 big.txt 前: %q", out)
	}

	// -r：逆序 → 名字序反转
	outAsc, err := runShell(t, dir, "ls -1")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls 失败: out=%q err=%v", outAsc, err)
	}
	outDesc, err := runShell(t, dir, "ls -1 -r")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls -r 失败: out=%q err=%v", outDesc, err)
	}
	asc := strings.Join(nonEmptyLines(outAsc), ",")
	desc := strings.Join(reverseSlice(nonEmptyLines(outDesc)), ",")
	if asc != desc {
		t.Errorf("ls -r 应为默认序的逆序: asc=%q desc=%q", asc, desc)
	}

	// -S：按大小大在前 → big 在 small 前
	out, err = runShell(t, dir, "ls -1 -S")
	if exitCode(t, err) != 0 {
		t.Fatalf("ls -S 失败: out=%q err=%v", out, err)
	}
	if i := strings.Index(out, "big.txt"); i == -1 || strings.Index(out, "small.txt") == -1 || i > strings.Index(out, "small.txt") {
		t.Errorf("ls -S 应 big.txt 在 small.txt 前: %q", out)
	}
}

// TestInternalTree 验证内建 tree：-L 层数限制、-I 排除、-d 仅目录、计数汇总。
// 进程内实现消除了 Windows 命中 C:\Windows\tree.com 的回退（不支持 -L/-I 且输出为
// OEM 码页字节，曾致整个工具结果 gRPC marshal 失败）。
func TestInternalTree(t *testing.T) {
	dir := t.TempDir()
	// 结构：a/{1.txt,2.txt}、b/、c/d/、skipme/x.txt
	for _, d := range []string{"a", "b", filepath.Join("c", "d"), "skipme"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join("a", "1.txt"), filepath.Join("a", "2.txt"), filepath.Join("skipme", "x.txt")} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 基础：框线字符 + 目录优先按名升序 + 汇总计数（5 目录 3 文件）
	out, err := runShell(t, dir, "tree")
	if exitCode(t, err) != 0 {
		t.Fatalf("tree 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "├── ") || !strings.Contains(out, "└── ") {
		t.Errorf("tree 应含框线前缀: %q", out)
	}
	if !strings.Contains(out, "5 directories") || !strings.Contains(out, "3 files") {
		t.Errorf("tree 汇总应 5 目录 3 文件: %q", out)
	}
	ia, ib := strings.Index(out, "├── a\n"), strings.Index(out, "├── b\n")
	ic, is := strings.Index(out, "├── c\n"), strings.Index(out, "└── skipme\n")
	if ia < 0 || ib < 0 || ic < 0 || is < 0 || !(ia < ib && ib < ic && ic < is) {
		t.Errorf("tree 目录应优先且按名升序（a<b<c<skipme）: %q", out)
	}

	// -L 1：只列直接子项、不展开
	out, err = runShell(t, dir, "tree -L 1")
	if exitCode(t, err) != 0 {
		t.Fatalf("tree -L 1 失败: out=%q err=%v", out, err)
	}
	if strings.Contains(out, "1.txt") {
		t.Errorf("tree -L 1 不应展开 a/ 下的 1.txt: %q", out)
	}
	if !strings.Contains(out, "4 directories") || !strings.Contains(out, "0 files") {
		t.Errorf("tree -L 1 汇总应 4 目录 0 文件: %q", out)
	}

	// -I：按 basename 排除（| 分隔多模式）→ 剩 a、b、c 三目录两文件
	out, err = runShell(t, dir, "tree -I 'skipme|d'")
	if exitCode(t, err) != 0 {
		t.Fatalf("tree -I 失败: out=%q err=%v", out, err)
	}
	if strings.Contains(out, "skipme") || strings.Contains(out, "x.txt") {
		t.Errorf("tree -I 应排除 skipme: %q", out)
	}
	if strings.Contains(out, "d\n") {
		t.Errorf("tree -I 应排除 c 下的 d: %q", out)
	}
	if !strings.Contains(out, "1.txt") {
		t.Errorf("tree -I 不应误伤 1.txt: %q", out)
	}
	if !strings.Contains(out, "3 directories") || !strings.Contains(out, "2 files") {
		t.Errorf("tree -I 汇总应 3 目录 2 文件: %q", out)
	}

	// -d：仅目录
	out, err = runShell(t, dir, "tree -d")
	if exitCode(t, err) != 0 {
		t.Fatalf("tree -d 失败: out=%q err=%v", out, err)
	}
	if strings.Contains(out, "1.txt") {
		t.Errorf("tree -d 不应列出文件: %q", out)
	}
	if !strings.Contains(out, "5 directories") || !strings.Contains(out, "0 files") {
		t.Errorf("tree -d 汇总应 5 目录 0 文件: %q", out)
	}

	// -L 3 组合（真实案例用法）：展开到 a/1.txt
	out, err = runShell(t, dir, "tree -L 3 -I 'skipme'")
	if exitCode(t, err) != 0 {
		t.Fatalf("tree -L 3 -I 失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "1.txt") {
		t.Errorf("tree -L 3 应展开到 a/1.txt: %q", out)
	}
}

// TestInternalWcStdinNoName 对齐 GNU wc：stdin 单文件时不回显名字（"-"），
// `find ... | wc -l` 只出计数。
func TestInternalWcStdinNoName(t *testing.T) {
	dir := t.TempDir()
	out, err := runShell(t, dir, "printf 'a\\nb\\n' | wc -l")
	if exitCode(t, err) != 0 {
		t.Fatalf("wc 失败: out=%q err=%v", out, err)
	}
	if strings.Contains(out, "-") {
		t.Errorf("stdin 计数不应带 '-' 名字: %q", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("应统计 2 行: %q", out)
	}

	// 文件参数仍回显名字
	out, err = runShell(t, dir, `printf 'a\nb\nc\n' > f.txt && wc -l f.txt`)
	if exitCode(t, err) != 0 {
		t.Fatalf("wc 文件失败: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "f.txt") {
		t.Errorf("文件计数应带名字: %q", out)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func reverseSlice(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
