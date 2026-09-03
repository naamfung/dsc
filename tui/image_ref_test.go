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

// TestTextRefCapFor 校验文本注入上限随上下文窗口容量换算，并收拢到 [min,max] 区间。
func TestTextRefCapFor(t *testing.T) {
	// 未知窗口 → 默认 1 MiB
	if got := textRefCapFor(0); got != defaultTextRefCapBytes {
		t.Fatalf("textRefCapFor(0) = %d, want %d", got, defaultTextRefCapBytes)
	}
	// 默认 128K 窗口（131072 token）→ cap = 131072*0.25*4 = 131072（约 128 KiB）
	want := int(float64(128*1024) * textRefBudgetFraction * textRefBytesPerToken)
	if got := textRefCapFor(128 * 1024); got != want {
		t.Fatalf("textRefCapFor(128k) = %d, want %d", got, want)
	}
	// 异常小窗口 → 抬到下限 minTextRefCapBytes
	if got := textRefCapFor(1024); got != minTextRefCapBytes {
		t.Fatalf("textRefCapFor(1k) = %d, want min %d", got, minTextRefCapBytes)
	}
	// 异常大窗口 → 收到上限 maxTextRefCapBytes
	if got := textRefCapFor(1 << 26); got != maxTextRefCapBytes {
		t.Fatalf("textRefCapFor(64M) = %d, want max %d", got, maxTextRefCapBytes)
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

// TestUnresolvedImageRefsFailure 校验 @图片 解析失败（已存在的图、缺失的图）会经
// resolveRefsDetailed 如实返回失败提示，供 TUI 提示用户而不静默丢图。
func TestUnresolvedImageRefsFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	// 使图片文件不可读（目录）→ saveImageAttachment 失败，当作解析失败暴露
	sub := filepath.Join(dir, "shot.png")
	_ = os.MkdirAll(sub, 0o755)
	t.Setenv("DSC_WORKSPACE_ROOT", dir)

	refs, fails := resolveRefsDetailed("@shot.png 看图")
	if len(refs) != 0 {
		t.Fatalf("图片解析失败时不应返回图片引用，got %v", refs)
	}
	if len(fails) != 1 || !strings.Contains(fails[0], "图片未解析") {
		t.Fatalf("应返回图片解析失败提示，got %v", fails)
	}

	// 缺失的图片 → 同样失败提示；缺失的非图片 → 不提示（普通 @ 文字引用不过度打扰）
	if _, fails := resolveRefsDetailed("@no_such.png"); len(fails) != 1 {
		t.Fatalf("缺失图片应失败提示，got %v", fails)
	}
	if _, fails := resolveRefsDetailed("@no_such.txt"); len(fails) != 0 {
		t.Fatalf("缺失非图片不应提示，got %v", fails)
	}
}
