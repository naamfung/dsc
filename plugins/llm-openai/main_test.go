package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"dsc/core"
	openai "github.com/sashabaranov/go-openai"
)

// TestImageContentPartsInline 视觉开启、非 DeepSeek 端点时：文本 + 每图一个
// image_url 块（base64 data URL 原样内联）。
func TestImageContentPartsInline(t *testing.T) {
	p := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("hello"))
	parts := p.imageContentParts("描述一下", []string{url})

	if len(parts) != 2 {
		t.Fatalf("want text + image parts, got %d", len(parts))
	}
	if parts[0].Type != openai.ChatMessagePartTypeText || parts[0].Text != "描述一下" {
		t.Fatalf("first part should be text, got %+v", parts[0])
	}
	if parts[1].Type != openai.ChatMessagePartTypeImageURL || parts[1].ImageURL.URL != url {
		t.Fatalf("second part should be image_url, got %+v", parts[1])
	}
}

// TestImageContentPartsRef 内容寻址引用（dsc-img://）被解析为 image_url 块。
func TestImageContentPartsRef(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	ref, err := core.SaveImageAttachment([]byte("ref-bytes"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	p := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	parts := p.imageContentParts("", []string{ref})
	if len(parts) != 1 || parts[0].Type != openai.ChatMessagePartTypeImageURL {
		t.Fatalf("ref should resolve to image_url, got %+v", parts)
	}
	if !strings.HasPrefix(parts[0].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q", parts[0].ImageURL.URL)
	}
}

// TestToOpenAIMessagesVisionGating 视觉关闭时图片不进入 MultiContent，保持纯文本
// Content；视觉开启时才构造多模态 content 数组。
func TestToOpenAIMessagesVisionGating(t *testing.T) {
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("x"))

	off := &OpenAIProvider{vision: false}
	msgs := off.toOpenAIMessages([]core.Message{core.Message{Role: "user", Content: "hi", Images: []string{url}}})
	if len(msgs[0].MultiContent) != 0 || msgs[0].Content != "hi" {
		t.Fatalf("vision off should keep plain content, got %+v", msgs[0])
	}

	on := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	msgs = on.toOpenAIMessages([]core.Message{core.Message{Role: "user", Content: "hi", Images: []string{url}}})
	if len(msgs[0].MultiContent) != 2 {
		t.Fatalf("vision on should build multimodal content, got %+v", msgs[0])
	}
}

// TestVisionEnabled 模型名含 vision 自动开启；DSC_VISION 可强制覆盖。
func TestVisionEnabled(t *testing.T) {
	t.Setenv("DSC_VISION", "")
	if !visionEnabled("deepseek-v4-flash-vision-exp") {
		t.Fatal("vision model should auto-enable vision")
	}
	if visionEnabled("Agentic-Turbo-Coder") {
		t.Fatal("non-vision model should not auto-enable")
	}
	t.Setenv("DSC_VISION", "0")
	if visionEnabled("deepseek-v4-flash-vision-exp") {
		t.Fatal("DSC_VISION=0 should force disable")
	}
	t.Setenv("DSC_VISION", "1")
	if !visionEnabled("Agentic-Turbo-Coder") {
		t.Fatal("DSC_VISION=1 should force enable")
	}
}

// TestDataURLSize 估算解码字节数（不依赖真实 base64 解码）。
func TestDataURLSize(t *testing.T) {
	raw := make([]byte, 1000)
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	size := dataURLSize(url)
	if size < 900 || size > 1100 {
		t.Fatalf("dataURLSize = %d, want ~1000", size)
	}
	if dataURLSize("not-a-data-url") != 0 {
		t.Fatal("non data URL should size 0")
	}
}

// TestChatCompletionMessageFilePartMarshal 验证扩展的 file 内容块序列化为
// {"type":"file","file_id":...}（DeepSeek Files API 引用格式）。
func TestChatCompletionMessageFilePartMarshal(t *testing.T) {
	msg := openai.ChatCompletionMessage{
		Role: "user",
		MultiContent: []openai.ChatMessagePart{
			{Type: openai.ChatMessagePartTypeText, Text: "看图"},
			{Type: openai.ChatMessagePartTypeFile, File: &openai.ChatMessageFile{FileID: "file-api-abc"}},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"type":"file"`) || !strings.Contains(s, `"file_id":"file-api-abc"`) {
		t.Fatalf("file part not marshaled correctly: %s", s)
	}
}
