package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256SumHex 计算字节的十六进制 sha256（测试辅助）。
func sha256SumHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestAttachmentDirDefault 未配置 DSC_ATTACHMENT_DIR 时，缺省落到可执行文件
// 所在目录的 attachments/（对齐 sessions/ 等目录旧例）。
func TestAttachmentDirDefault(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", "")
	dir := AttachmentDir()
	exeDir, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(exeDir), "attachments")
	if dir != want {
		t.Fatalf("AttachmentDir() = %q, want %q", dir, want)
	}
}

// TestSaveAndResolveImageAttachment 验证内容寻址附件库：保存返回纯哈希引用、
// 同内容去重、解析回带正确 MIME 的 data URL。
func TestSaveAndResolveImageAttachment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	// 用真实 PNG 魔数作为图片字节，验证 MIME 由字节嗅探得出
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3}
	ref, err := SaveImageAttachment(png)
	if err != nil {
		t.Fatalf("SaveImageAttachment: %v", err)
	}
	// 引用 = dsc-img://<纯哈希>，不带后缀
	if !strings.HasPrefix(ref, "dsc-img://") || strings.Contains(ref, ".") {
		t.Fatalf("ref = %q, want dsc-img://<sha256> without extension", ref)
	}
	name := strings.TrimPrefix(ref, "dsc-img://")
	// 附件文件名为纯哈希（无后缀）
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("attachment file not written as pure hash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, name+".jpg")); err == nil {
		t.Fatalf("attachment should not be named with extension")
	}

	// 同内容再次保存 → 同一引用（内容寻址去重）
	ref2, _ := SaveImageAttachment(png)
	if ref2 != ref {
		t.Fatalf("dedup failed: %q != %q", ref2, ref)
	}

	// 解析回 data URL，MIME 由字节嗅探（PNG 魔数 → image/png）
	url, err := ResolveImageRef(ref)
	if err != nil {
		t.Fatalf("ResolveImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q, want data:image/png;base64,", url)
	}
}

// TestSaveAttachmentRewritesCorruptFile 坏文件（大小与数据不符的残留）不被内容寻址
// 去重复用：保存时会识别大小不符、删除并重写为完整内容，解析回完整 data URL。
func TestSaveAttachmentRewritesCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3}
	sum := sha256SumHex(png)
	path := filepath.Join(dir, sum)

	// 预置一个内容相同哈希但大小不符的坏文件（模拟写入中断残留的部分文件）
	if err := os.WriteFile(path, png[:4], 0o644); err != nil {
		t.Fatal(err)
	}

	// 保存同内容 → 应识别坏文件并重写为完整内容，而非直接复用
	ref, err := SaveImageAttachment(png)
	if err != nil {
		t.Fatalf("SaveImageAttachment: %v", err)
	}
	if got := strings.TrimPrefix(ref, "dsc-img://"); got != sum {
		t.Fatalf("ref = %q, want hash %s", ref, sum)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if string(data) != string(png) {
		t.Fatalf("corrupt file not rewritten: got %d bytes, want %d", len(data), len(png))
	}
	url, err := ResolveImageRef(ref)
	if err != nil {
		t.Fatalf("ResolveImageRef after rewrite: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q, want complete png data url", url)
	}
}

// TestResolveImageRefRejectsSuffixedRef 引用只认纯哈希：带后缀的旧式引用
// （dsc-img://<sha256>.png）一律拒绝，不保留后缀回退兼容分支。
func TestResolveImageRefRejectsSuffixedRef(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 4, 5, 6}
	ref, err := SaveImageAttachment(jpg)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(ref, "dsc-img://")

	if _, err := ResolveImageRef("dsc-img://" + name + ".png"); err == nil {
		t.Fatal("suffixed ref should be rejected")
	}
	// 纯哈希引用照常解析，MIME 由字节嗅探（JPEG 魔数 → image/jpeg）
	url, err := ResolveImageRef(ref)
	if err != nil {
		t.Fatalf("ResolveImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("resolved mime should come from bytes (jpeg), got %q", url)
	}
}

// TestResolveImageRefRejectsInlineAndErrors 内联 data URL 不再是合法引用形态
// （会话历史只存内容寻址引用，data URL 与其它前缀一律拒绝）；缺失/非法引用报错。
func TestResolveImageRefRejectsInlineAndErrors(t *testing.T) {
	if _, err := ResolveImageRef("data:image/jpeg;base64,QUJD"); err == nil {
		t.Fatal("inline data URL should be rejected (refs only)")
	}
	if _, err := ResolveImageRef("dsc-img://deadbeef"); err == nil {
		t.Fatal("missing attachment should error")
	}
	if _, err := ResolveImageRef("http://example.com/x.png"); err == nil {
		t.Fatal("non-attachment ref should error")
	}
}
