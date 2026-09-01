package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveImageRefs 验证 @图片路径 被写入附件库并返回内容寻址引用；非图片、
// 缺失路径、普通文件引用保持忽略（不作为附件）。
func TestResolveImageRefs(t *testing.T) {
	dir := t.TempDir()
	attachDir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", attachDir)
	png := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(png, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSC_WORKSPACE_ROOT", dir)

	// 相对工作区路径的图片引用 → 1 个 dsc-img:// 引用
	refs := ResolveImageRefs("看看 @shot.png 这张图")
	if len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-img://") {
		t.Fatalf("ResolveImageRefs = %v, want one dsc-img ref", refs)
	}
	// 附件库中应有内容寻址文件
	name := strings.TrimPrefix(refs[0], "dsc-img://")
	if _, err := os.Stat(filepath.Join(attachDir, name)); err != nil {
		t.Fatalf("attachment file not written: %v", err)
	}

	// 绝对路径引用同样生效
	refs = ResolveImageRefs("@" + png)
	if len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-img://") {
		t.Fatalf("absolute path ResolveImageRefs = %v", refs)
	}

	// 普通文本文件引用不解析为图片（保持文字）
	if refs := ResolveImageRefs("@note.txt"); len(refs) != 0 {
		t.Fatalf("text file should not become image, got %v", refs)
	}

	// 不存在的路径忽略
	if refs := ResolveImageRefs("@missing.png"); len(refs) != 0 {
		t.Fatalf("missing path should be ignored, got %v", refs)
	}

	// 无 @ 的输入 → 无图片
	if refs := ResolveImageRefs("你好"); len(refs) != 0 {
		t.Fatalf("plain text should have no images, got %v", refs)
	}
}

// TestResolveImageRefsDedupAndMultipe 同一路径多次引用只附加一次；多张图都生效。
func TestResolveImageRefsDedupAndMultipe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	png := filepath.Join(dir, "a.png")
	jpg := filepath.Join(dir, "b.jpg")
	os.WriteFile(png, []byte("png"), 0o644)
	os.WriteFile(jpg, []byte("jpg"), 0o644)
	t.Setenv("DSC_WORKSPACE_ROOT", dir)

	refs := ResolveImageRefs("@a.png 与 @b.jpg 和再提一次 @a.png")
	if len(refs) != 2 {
		t.Fatalf("want 2 unique images (dedup), got %d: %v", len(refs), refs)
	}
	if !strings.HasPrefix(refs[0], "dsc-img://") || !strings.HasPrefix(refs[1], "dsc-img://") {
		t.Fatalf("refs should be dsc-img refs: %v", refs)
	}
}

// TestResolveRefPathWorkspaceAlias /workspace 虚拟根映射到工作区真实根。
func TestResolveRefPathWorkspaceAlias(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_WORKSPACE_ROOT", dir)
	got := resolveRefPath(dir, "/workspace/sub/x.png")
	if got != filepath.Join(dir, "sub", "x.png") {
		t.Fatalf("/workspace alias = %q, want %q", got, filepath.Join(dir, "sub", "x.png"))
	}
}
