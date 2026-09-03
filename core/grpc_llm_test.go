package core

import (
	"context"
	"testing"

	"dsc/proto/metadata"
)

// fakeVisionProvider 固定上报视觉能力的 LLMProvider（仅用于 GetInfo 能力测试）。
type fakeVisionProvider struct {
	vision bool
}

func (f *fakeVisionProvider) Chat(context.Context, []Message, []Tool, int) (*ChatResponse, error) {
	return nil, nil
}
func (f *fakeVisionProvider) ChatStream(context.Context, []Message, []Tool) (<-chan *ChatStreamResponse, error) {
	return nil, nil
}
func (f *fakeVisionProvider) Name(context.Context) string    { return "fake" }
func (f *fakeVisionProvider) Version(context.Context) string { return "1.0.0" }
func (f *fakeVisionProvider) HealthCheck(context.Context) error {
	return nil
}
func (f *fakeVisionProvider) VisionEnabled() bool { return f.vision }

// TestLLMMetadataCapabilities 校验 llmMetadataServer.GetInfo 把视觉能力经 capabilities
// 上报给宿主（TUI 据此提示「图片未随消息发送：当前模型不支持图像输入」）。
func TestLLMMetadataCapabilities(t *testing.T) {
	on := &llmMetadataServer{impl: &fakeVisionProvider{vision: true}}
	info, err := on.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Capabilities["supports_images"] != "true" {
		t.Fatalf("vision=true 时应上报 supports_images=true, got %q", info.Capabilities["supports_images"])
	}

	off := &llmMetadataServer{impl: &fakeVisionProvider{vision: false}}
	infoOff, err := off.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if infoOff.Capabilities["supports_images"] != "false" {
		t.Fatalf("vision=false 时应上报 supports_images=false, got %q", infoOff.Capabilities["supports_images"])
	}
}
