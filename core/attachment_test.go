package core

import (
	"strings"
	"testing"
)

// TestSaveAndResolveImageAttachment 验证内容寻址附件库：保存返回 dsc-img:// 引用、
// 同内容去重、解析回 data URL。
func TestSaveAndResolveImageAttachment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)

	data := []byte("fake-png-bytes")
	ref, err := SaveImageAttachment(data, "image/png")
	if err != nil {
		t.Fatalf("SaveImageAttachment: %v", err)
	}
	if !strings.HasPrefix(ref, "dsc-img://") || !strings.HasSuffix(ref, ".png") {
		t.Fatalf("ref = %q, want dsc-img://<sha256>.png", ref)
	}

	// 同内容再次保存 → 同一引用（内容寻址去重）
	ref2, err := SaveImageAttachment(data, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if ref2 != ref {
		t.Fatalf("dedup failed: %q != %q", ref2, ref)
	}

	// 解析回 data URL
	url, err := ResolveImageRef(ref)
	if err != nil {
		t.Fatalf("ResolveImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q, want data:image/png;base64,", url)
	}

	// 已内联的 data URL 原样返回（兼容旧历史）
	inline := "data:image/jpeg;base64,QUJD"
	if got, _ := ResolveImageRef(inline); got != inline {
		t.Fatalf("inline data URL should pass through, got %q", got)
	}

	// 不存在的引用 → 错误
	if _, err := ResolveImageRef("dsc-img://deadbeef.png"); err == nil {
		t.Fatal("missing attachment should error")
	}
	// 非法引用格式 → 错误
	if _, err := ResolveImageRef("http://example.com/x.png"); err == nil {
		t.Fatal("non-attachment ref should error")
	}
}
