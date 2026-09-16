package main

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"dsc/core"
	"github.com/anthropics/anthropic-sdk-go"
)

// testPNGBytes 生成一张 2×2 真实可解码 PNG（图像引用经 ProjectImageRef 投影，
// 截断/伪魔数字节无法通过解码，测试一律用真图）。
func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	var buf strings.Builder
	if err := png.Encode(&stringWriter{&buf}, img); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

// stringWriter 适配 io.Writer 的薄包装（避免直接 import bytes 的循环别名）。
type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// saveRef 把测试图字节入库为持久附件引用。
func saveRef(t *testing.T, data []byte) string {
	t.Helper()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	t.Setenv("DSC_TEMP_DIR", t.TempDir())
	ref, err := core.SaveImageAttachment(data)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestFileContentBlocksRef 内容寻址引用（dsc-img://）投影为文本 + base64 image 块；
// 投影产物 base64 与入库字节一致（未超限透传，零重编码）。
func TestFileContentBlocksRef(t *testing.T) {
	data := testPNGBytes(t)
	ref := saveRef(t, data)
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	blocks, usesFile := p.fileContentBlocks("描述一下", []string{ref})

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
	if img.Source.OfBase64.Data != base64.StdEncoding.EncodeToString(data) {
		t.Fatal("projected payload should equal stored bytes (passthrough)")
	}
}

// TestFileContentBlocksFileSource 投影产物超过内联阈值 + DeepSeek 端点时以 file 源
// 引用 file_id 并标记需带 beta 头（上传以文件缓存预置 file_id 模拟成功）。
func TestFileContentBlocksFileSource(t *testing.T) {
	data := testPNGBytes(t)
	ref := saveRef(t, data)
	url, err := core.ProjectImageRef(ref, core.DefaultProjectionMaxSide)
	if err != nil {
		t.Fatal(err)
	}

	origLimit := maxInlineImageBytes
	maxInlineImageBytes = 8 // 注入小阈值：投影产物必然超限
	t.Cleanup(func() { maxInlineImageBytes = origLimit })

	p := &AnthropicProvider{
		vision:    true,
		filesAPI:  true,
		fileCache: map[string]string{url: "file-api-abc123"},
	}
	blocks, usesFile := p.fileContentBlocks("", []string{ref})
	if !usesFile {
		t.Fatal("超限图应使用 file 源")
	}
	img := blocks[0].OfImage
	if img == nil || img.Source.OfFile == nil || img.Source.OfFile.FileID != "file-api-abc123" {
		t.Fatalf("expected file source with file_id, got %+v", blocks[0])
	}
}

// TestFileContentBlocksUnavailableRef 引用失效（截图过期/附件缺失）：降级为稳定
// 占位文本块，模型由此知道该处曾有图。
func TestFileContentBlocksUnavailableRef(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	t.Setenv("DSC_TEMP_DIR", t.TempDir())
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	blocks, _ := p.fileContentBlocks("", []string{"dsc-shot://deadbeef"})
	if len(blocks) != 1 || blocks[0].OfText == nil {
		t.Fatalf("unavailable ref should degrade to text block, got %+v", blocks)
	}
	if !strings.Contains(blocks[0].OfText.Text, "[image unavailable") || !strings.Contains(blocks[0].OfText.Text, "dsc-shot://deadbeef") {
		t.Fatalf("placeholder should carry the ref identity, got %q", blocks[0].OfText.Text)
	}
}

// TestBuildMessageParamsImages 用户消息带图像引用且视觉开启时构造多模态块；
// 关闭时纯文本。
func TestBuildMessageParamsImages(t *testing.T) {
	ref := saveRef(t, testPNGBytes(t))

	off := &AnthropicProvider{vision: false}
	params, beta := off.buildMessageParams([]core.Message{{Role: "user", Content: "hi", Images: []string{ref}}}, nil, 0)
	if beta {
		t.Fatal("无图片不应带 beta 头")
	}
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d", len(params.Messages))
	}

	on := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	params, beta = on.buildMessageParams([]core.Message{{Role: "user", Content: "hi", Images: []string{ref}}}, nil, 0)
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

// TestToolResultContentBlocks 工具结果图像引用：文本块 + 内嵌 base64 图像块（vision on）。
func TestToolResultContentBlocks(t *testing.T) {
	data := testPNGBytes(t)
	ref := saveRef(t, data)
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	blocks, usesFile := p.toolResultContentBlocks("截图完成", []string{ref})
	if usesFile {
		t.Fatal("内联小图不应使用 file 源")
	}
	if len(blocks) != 2 {
		t.Fatalf("want text + image blocks, got %d", len(blocks))
	}
	if blocks[0].OfText == nil || blocks[0].OfText.Text != "截图完成" {
		t.Fatalf("first block should be text, got %+v", blocks[0])
	}
	img := blocks[1].OfImage
	if img == nil || img.Source.OfBase64 == nil || img.Source.OfBase64.Data != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("second block should be base64 image of stored bytes, got %+v", blocks[1])
	}
}

// TestToolResultContentBlocksVisionOff 视觉关闭：工具结果仅保留文本块。
func TestToolResultContentBlocksVisionOff(t *testing.T) {
	ref := saveRef(t, testPNGBytes(t))
	p := &AnthropicProvider{vision: false, filesAPI: false, fileCache: map[string]string{}}
	blocks, _ := p.toolResultContentBlocks("截图完成", []string{ref})
	if len(blocks) != 1 || blocks[0].OfText == nil {
		t.Fatalf("视觉关闭应仅文本块, got %d", len(blocks))
	}
}

// TestBuildMessageParamsToolImages tool 消息带图像引用：图像内嵌 tool_result.content
// （Anthropic computer-use 规范形态），不产生独立图像块。
func TestBuildMessageParamsToolImages(t *testing.T) {
	ref := saveRef(t, testPNGBytes(t))
	p := &AnthropicProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	params, beta := p.buildMessageParams([]core.Message{
		{Role: "assistant", ToolCalls: []core.ToolCall{{ID: "call1", Name: "computer_use_screen"}}},
		{Role: "tool", Content: "ok", ToolCallID: "call1", Images: []string{ref}},
	}, nil, 0)
	if beta {
		t.Fatal("内联图像不应触发 beta 头")
	}
	if len(params.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(params.Messages))
	}
	foundToolResult, foundImage := false, false
	for _, b := range params.Messages[1].Content {
		if b.OfToolResult != nil {
			foundToolResult = true
			if b.OfToolResult.ToolUseID != "call1" {
				t.Fatalf("tool_use_id = %q, want call1", b.OfToolResult.ToolUseID)
			}
			if len(b.OfToolResult.Content) != 2 || b.OfToolResult.Content[1].OfImage == nil {
				t.Fatalf("tool_result.content 应含 text + image, got %d", len(b.OfToolResult.Content))
			}
		}
		if b.OfImage != nil {
			foundImage = true
		}
	}
	if !foundToolResult || foundImage {
		t.Fatalf("图像应内嵌 tool_result（foundToolResult=%v, foundImage=%v）", foundToolResult, foundImage)
	}
}
