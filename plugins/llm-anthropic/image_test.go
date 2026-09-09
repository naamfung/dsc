package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"dsc/core"
	"github.com/anthropics/anthropic-sdk-go"
)

// TestFileContentBlocksBase64 视觉开启、非 DeepSeek 端点时：文本 + 每图一个 base64 image 块。
func TestFileContentBlocksBase64(t *testing.T) {
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("hello"))
	blocks, usesFile := p.fileContentBlocks("描述一下", []string{url})

	if usesFile {
		t.Fatal("非 files_api 端点不应使用 file 源")
	}
	if len(blocks) != 2 {
		t.Fatalf("want text + image blocks, got %d", len(blocks))
	}
	if blocks[0].OfText == nil || blocks[0].OfText.Text != "描述一下" {
		t.Fatalf("first block should be text, got %+v", blocks[0])
	}
	img := blocks[1].OfImage
	if img == nil || img.Source.OfBase64 == nil {
		t.Fatalf("second block should be base64 image, got %+v", blocks[1])
	}
	if img.Source.OfBase64.Data != "aGVsbG8=" {
		t.Fatalf("unexpected base64 data: %q", img.Source.OfBase64.Data)
	}
}

// TestFileContentBlocksFileSource 大图 + DeepSeek 端点时上传失败会回退 base64；若上传
// 成功则使用 file 源并标记 needs beta 头。这里以文件缓存预置 file_id 模拟上传成功。
func TestFileContentBlocksFileSource(t *testing.T) {
	// 构造 >20MiB 的 data URL
	big := make([]byte, (maxInlineImageBytes+1)*2)
	url := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(big)

	p := &AnthropicProvider{
		vision:    true,
		filesAPI:  true,
		fileCache: map[string]string{url: "file-api-abc123"},
	}
	blocks, usesFile := p.fileContentBlocks("", []string{url})
	if !usesFile {
		t.Fatal("大图应使用 file 源")
	}
	img := blocks[0].OfImage
	if img == nil || img.Source.OfFile == nil || img.Source.OfFile.FileID != "file-api-abc123" {
		t.Fatalf("expected file source with file_id, got %+v", blocks[0])
	}
}

// TestFileContentBlocksRef 内容寻址引用（dsc-img://）被解析为 base64 image 块。
func TestFileContentBlocksRef(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 7, 8, 9}
	ref, err := core.SaveImageAttachment(jpg)
	if err != nil {
		t.Fatal(err)
	}
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	blocks, _ := p.fileContentBlocks("", []string{ref})
	if len(blocks) != 1 || blocks[0].OfImage == nil || blocks[0].OfImage.Source.OfBase64 == nil {
		t.Fatalf("ref should resolve to base64 image block, got %+v", blocks)
	}
}

// TestBuildMessageParamsImages 用户消息带图片且视觉开启时构造多模态块；关闭时纯文本。
func TestBuildMessageParamsImages(t *testing.T) {
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("x"))

	off := &AnthropicProvider{vision: false}
	params, beta := off.buildMessageParams([]core.Message{{Role: "user", Content: "hi", Images: []string{url}}}, nil, 0)
	if beta {
		t.Fatal("无图片不应带 beta 头")
	}
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d", len(params.Messages))
	}

	on := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	params, beta = on.buildMessageParams([]core.Message{{Role: "user", Content: "hi", Images: []string{url}}}, nil, 0)
	if beta {
		t.Fatal("base64 内联不应带 beta 头")
	}
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d", len(params.Messages))
	}
}

// TestSplitDataURL 解析 data URL 的 mime 与 base64 负载。
func TestSplitDataURL(t *testing.T) {
	mime, b64, ok := splitDataURL("data:image/webp;base64,QUJD")
	if !ok || mime != "image/webp" || b64 != "QUJD" {
		t.Fatalf("splitDataURL = (%q,%q,%v)", mime, b64, ok)
	}
	if _, _, ok := splitDataURL("plain"); ok {
		t.Fatal("非 data URL 应解析失败")
	}
	if dataURLSize("data:image/png;base64,"+strings.Repeat("A", 8)) != 6 {
		t.Fatal("dataURLSize 估算错误")
	}
}

// TestFileImageSourceMarshal 验证 file 源序列化为 {"type":"file","file_id":...}
// （DeepSeek anthropic 兼容 Files API 引用格式）。
func TestFileImageSourceMarshal(t *testing.T) {
	block := anthropic.NewImageBlock(anthropic.FileImageSourceParam{
		FileID:    "file-api-xyz",
		MediaType: anthropic.Base64ImageSourceMediaTypeImageJPEG,
	})
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"type":"file"`) || !strings.Contains(s, `"file_id":"file-api-xyz"`) {
		t.Fatalf("file source not marshaled correctly: %s", s)
	}
}
