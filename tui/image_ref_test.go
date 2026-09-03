package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/core"
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

// TestResolveImageRefsPixPinSentence 复现真实用户输入：PixPin 截图（含下划线/连字符
// 的文件名）在句中引用、文件名在工作区内，应解析出一个附件。
func TestResolveImageRefsPixPinSentence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	png := filepath.Join(dir, "PixPin_2026-07-18_11-38-34.png")
	if err := os.WriteFile(png, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSC_WORKSPACE_ROOT", dir)

	input := "不用了，你帮我睇下这张图 @PixPin_2026-07-18_11-38-34.png , 描述下都有乜信息？"
	refs := ResolveImageRefs(input)
	if len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-img://") {
		t.Fatalf("句中 PixPin 图片引用（png 后有空格）= %v, want 1 dsc-img ref", refs)
	}

	// 关键变体：.png 后**紧跟**中文标点再接文字（无空格）——这是句中 @ 最常见的
	// 写法。若 splitRefTokens 不在此处截断 token，会吞掉后续文字导致查找失败。
	inputNoSpace := "不用了，你帮我睇下这张图 @PixPin_2026-07-18_11-38-34.png，描述下都有乜信息？"
	refs2 := ResolveImageRefs(inputNoSpace)
	if len(refs2) != 1 || !strings.HasPrefix(refs2[0], "dsc-img://") {
		t.Fatalf("句中 PixPin 图片引用（png 后紧跟标点）= %v, want 1 dsc-img ref（token 未在标点截断，吞掉后续文字）", refs2)
	}
}

// TestResolveFileRefsText 校验 @文本文件 与图像读取方式对齐，生成 dsc-txt:// 内容
// 寻址引用；二进制（含 NUL）/目录/缺失路径忽略保留文字。
func TestResolveFileRefsText(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("hello 世界"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(dir, "shot.png")
	os.WriteFile(png, []byte("fake"), 0o644)
	t.Setenv("DSC_WORKSPACE_ROOT", dir)

	// 文本文件 → dsc-txt:// 引用
	refs := ResolveFileRefs("看下 @note.txt 的内容")
	if len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-txt://") {
		t.Fatalf("ResolveFileRefs(text) = %v, want 1 dsc-txt ref", refs)
	}
	if got, err := core.ResolveTextRef(refs[0]); err != nil || got != "hello 世界" {
		t.Fatalf("ResolveTextRef = %q, err %v", got, err)
	}

	// 图片仍走 dsc-img://
	if refs := ResolveFileRefs("@shot.png"); len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-img://") {
		t.Fatalf("ResolveFileRefs(image) = %v, want 1 dsc-img ref", refs)
	}

	// 二进制（含 NUL）→ 忽略
	if refs := ResolveFileRefs("@blob.bin"); len(refs) != 0 {
		t.Fatalf("二进制文件不应生成引用，got %v", refs)
	}
	// 缺失路径 → 忽略
	if refs := ResolveFileRefs("@missing.txt"); len(refs) != 0 {
		t.Fatalf("缺失文件不应生成引用，got %v", refs)
	}
}
