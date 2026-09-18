package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dsc/core"
	openai "github.com/sashabaranov/go-openai"
)

// testPNGRef 生成一张 2×2 真实可解码 PNG 入库为持久附件引用（图像引用经
// ProjectImageRef 投影，截断/伪魔数字节无法通过解码，测试一律用真图）。
func testPNGRef(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(1, 1, color.RGBA{R: 10, G: 200, B: 40, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	t.Setenv("DSC_TEMP_DIR", t.TempDir())
	ref, err := core.SaveImageAttachment(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestImageContentPartsRef 内容寻址引用（dsc-img://）投影为文本 + image_url 块；
// 投影产物为 base64 data URL，未超限时字节透传零重编码。
func TestImageContentPartsRef(t *testing.T) {
	ref := testPNGRef(t)
	p := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	parts := p.fileContentBlocks("描述一下", []string{ref})

	if len(parts) != 2 {
		t.Fatalf("want text + image parts, got %d", len(parts))
	}
	if parts[0].Type != openai.ChatMessagePartTypeText || parts[0].Text != "描述一下" {
		t.Fatalf("first part should be text, got %+v", parts[0])
	}
	if parts[1].Type != openai.ChatMessagePartTypeImageURL {
		t.Fatalf("second part should be image_url, got %+v", parts[1])
	}
	if !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("projected url = %q", parts[1].ImageURL.URL)
	}
}

// TestToOpenAIMessagesVisionGating 视觉关闭时图像引用被跳过（仅留文本块）；视觉
// 开启时才构造含图像的完整多模态 content 数组。文本引用（dsc-txt）不受视觉门控。
func TestToOpenAIMessagesVisionGating(t *testing.T) {
	ref := testPNGRef(t)

	off := &OpenAIProvider{vision: false}
	msgs := off.toOpenAIMessages([]core.Message{core.Message{Role: "user", Content: "hi", Images: []string{ref}}})
	if len(msgs[0].MultiContent) != 1 || msgs[0].MultiContent[0].Text != "hi" {
		t.Fatalf("vision off should keep text block (image skipped), got %+v", msgs[0])
	}
	if msgs[0].Content != "" {
		t.Fatalf("vision off multimodal should not set Content, got %q", msgs[0].Content)
	}

	on := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	msgs = on.toOpenAIMessages([]core.Message{core.Message{Role: "user", Content: "hi", Images: []string{ref}}})
	if len(msgs[0].MultiContent) != 2 {
		t.Fatalf("vision on should build multimodal content, got %+v", msgs[0])
	}
}

// TestTextRefInjected 文本附件引用（dsc-txt://）即使视觉关闭也会作为文本块注入。
func TestTextRefInjected(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	ref, err := core.SaveTextAttachment([]byte("音乐插件状态：播放中"))
	if err != nil {
		t.Fatal(err)
	}
	off := &OpenAIProvider{vision: false}
	msgs := off.toOpenAIMessages([]core.Message{core.Message{Role: "user", Content: "看看这个", Images: []string{ref}}})
	if len(msgs[0].MultiContent) != 2 {
		t.Fatalf("text ref should inject as text block even without vision, got %+v", msgs[0])
	}
	if msgs[0].MultiContent[1].Text != "音乐插件状态：播放中" {
		t.Fatalf("text block content = %q", msgs[0].MultiContent[1].Text)
	}
}

// TestVisionEnabled 默认按模型能力自动判断（/models 上报 input_modalities）；
// 未上报放行；DSC_NO_VISION=1 强制关闭。
func TestVisionEnabled(t *testing.T) {
	t.Setenv("DSC_NO_VISION", "")

	// 服务端上报含 image → 自动启用
	srvImage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"Agentic-Turbo","input_modalities":["text","image"]}]}`))
	}))
	defer srvImage.Close()
	if !visionEnabled(srvImage.URL, "Agentic-Turbo", false) {
		t.Fatal("server reporting image modality should auto-enable")
	}

	// 服务端上报仅 text → 自动关闭
	srvText := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"Agentic-Turbo","input_modalities":["text"]}]}`))
	}))
	defer srvText.Close()
	if visionEnabled(srvText.URL, "Agentic-Turbo", false) {
		t.Fatal("server reporting text-only should auto-disable")
	}

	// 服务端未上报 → 默认放行
	srvUnknown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"Agentic-Turbo"}]}`))
	}))
	defer srvUnknown.Close()
	if !visionEnabled(srvUnknown.URL, "Agentic-Turbo", false) {
		t.Fatal("server not reporting modalities should default to allow")
	}

	// DSC_NO_VISION=1 强制关闭（逃生口）
	t.Setenv("DSC_NO_VISION", "1")
	if visionEnabled(srvImage.URL, "Agentic-Turbo", false) {
		t.Fatal("DSC_NO_VISION=1 should force disable")
	}
	t.Setenv("DSC_NO_VISION", "0")
	if !visionEnabled(srvImage.URL, "Agentic-Turbo", false) {
		t.Fatal("DSC_NO_VISION=0 should keep auto-detect")
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

// TestToOpenAIMessagesToolImages 工具消息图像引用：段末补发一条 user 图像消息；
// tool 消息本体回带 tool_call_id（OpenAI 协议关联要求）。
func TestToOpenAIMessagesToolImages(t *testing.T) {
	ref := testPNGRef(t)

	on := &OpenAIProvider{vision: true, filesAPI: false, fileCache: map[string]string{}}
	msgs := on.toOpenAIMessages([]core.Message{
		{Role: "assistant", ToolCalls: []core.ToolCall{{ID: "c1", Name: "computer_use_screen"}}},
		{Role: "tool", Content: "ok", ToolCallID: "c1", Images: []string{ref}},
		{Role: "user", Content: "看到什么了？"},
	})
	if len(msgs) != 4 {
		t.Fatalf("want assistant+tool+user(img)+user(text), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "assistant" || len(msgs[0].ToolCalls) != 1 {
		t.Fatalf("msgs[0] 应为 assistant tool_calls, got %+v", msgs[0])
	}
	if msgs[1].Role != "tool" || msgs[1].ToolCallID != "c1" || msgs[1].Content != "ok" {
		t.Fatalf("tool 消息应回带 tool_call_id, got %+v", msgs[1])
	}
	if msgs[2].Role != "user" || len(msgs[2].MultiContent) != 1 {
		t.Fatalf("段末应补发 user 图像消息, got %+v", msgs[2])
	}
	if msgs[2].MultiContent[0].ImageURL == nil {
		t.Fatalf("补发消息应为 image_url 块, got %+v", msgs[2].MultiContent[0])
	}
	if msgs[3].Content != "看到什么了？" {
		t.Fatalf("末条用户文本应在图像消息之后, got %+v", msgs[3])
	}
}

// TestToOpenAIMessagesToolImagesVisionOff 视觉关闭：不补发图像消息。
func TestToOpenAIMessagesToolImagesVisionOff(t *testing.T) {
	ref := testPNGRef(t)
	off := &OpenAIProvider{vision: false, filesAPI: false, fileCache: map[string]string{}}
	msgs := off.toOpenAIMessages([]core.Message{
		{Role: "tool", Content: "ok", ToolCallID: "c1", Images: []string{ref}},
	})
	if len(msgs) != 1 || msgs[0].ToolCallID != "c1" {
		t.Fatalf("视觉关闭应仅 tool 消息本体, got %+v", msgs)
	}
}
