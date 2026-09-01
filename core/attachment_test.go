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

// TestResolveImageRefExtensionIrrelevant 同内容改后缀不影响命中（去重不受扩展名
// 影响）：对同一引用追加假后缀仍按纯哈希读到同一文件。
func TestResolveImageRefExtensionIrrelevant(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 4, 5, 6}
	ref, err := SaveImageAttachment(jpg)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(ref, "dsc-img://")

	// 随便加个假后缀的旧式引用，仍应命中同一纯哈希文件
	url, err := ResolveImageRef("dsc-img://" + name + ".png")
	if err != nil {
		t.Fatalf("ref with fake extension should still resolve: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("resolved mime should come from bytes (jpeg), got %q", url)
	}
}

// TestResolveImageRefLegacyExtension 兼容早期版本：文件名带后缀的遗留附件
// （<sha256>.jpg）仍能被旧式引用解析。
func TestResolveImageRefLegacyExtension(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 7, 8, 9}
	sum := sha256SumHex(jpg)
	// 模拟旧版落盘文件名 <sha256>.jpg
	if err := os.WriteFile(filepath.Join(dir, sum+".jpg"), jpg, 0o644); err != nil {
		t.Fatal(err)
	}
	// 旧式引用 dsc-img://<sha256>.jpg 可解析
	url, err := ResolveImageRef("dsc-img://" + sum + ".jpg")
	if err != nil {
		t.Fatalf("legacy ref should resolve: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("legacy resolved mime should be jpeg, got %q", url)
	}
}

// TestResolveImageRefInlineAndErrors 已内联 data URL 原样返回；缺失/非法引用报错。
func TestResolveImageRefInlineAndErrors(t *testing.T) {
	inline := "data:image/jpeg;base64,QUJD"
	if got, _ := ResolveImageRef(inline); got != inline {
		t.Fatalf("inline data URL should pass through, got %q", got)
	}
	if _, err := ResolveImageRef("dsc-img://deadbeef"); err == nil {
		t.Fatal("missing attachment should error")
	}
	if _, err := ResolveImageRef("http://example.com/x.png"); err == nil {
		t.Fatal("non-attachment ref should error")
	}
}
