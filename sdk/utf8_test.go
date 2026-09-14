package dsc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"dsc/proto"
	pbproto "google.golang.org/protobuf/proto"
)

// 工具结果含非法 UTF-8 时不得让 gRPC 响应 marshal 失败：proto string 字段强制
// 合法 UTF-8，原样透传会导致模型只收到 "grpc: error while marshaling: string
// field contains invalid UTF-8" 而丢失全部输出（真实案例：Windows 原生命令的
// OEM 码页输出）。SDK 层在 ExecuteTool 汇聚点统一净化。

func TestExecuteToolSanitizesInvalidUTF8Content(t *testing.T) {
	s := testSDK()
	s.Tool(Tool{
		Name: "dirty",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "ok \x80\xc4\xe3 bad", nil
		},
	})
	srv := &toolServiceServer{sdk: s}

	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "dirty", ArgumentsJson: `{}`})
	if err != nil {
		t.Fatalf("ExecuteTool 传输层错误: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("不应有错误: %q", resp.Error)
	}
	if !utf8.ValidString(resp.Content) {
		t.Fatalf("Content 必须为合法 UTF-8: %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "\uFFFD") {
		t.Fatalf("非法字节应替换为 U+FFFD: %q", resp.Content)
	}
	// 终极判据：响应必须能被 proto 序列化（gRPC 发送等价判据）
	if _, err := pbproto.Marshal(resp); err != nil {
		t.Fatalf("proto.Marshal 失败（gRPC 将拒发）: %v", err)
	}
}

func TestExecuteToolSanitizesInvalidUTF8ErrorAndView(t *testing.T) {
	s := testSDK()
	s.Tool(Tool{
		Name: "dirty-err",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", errors.New("failed: \xff\xfe detail")
		},
	})
	s.Tool(Tool{
		Name: "dirty-view",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "ok", nil
		},
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return json.RawMessage(`{"body":"` + "\xff" + `"}`), nil
		},
	})
	srv := &toolServiceServer{sdk: s}

	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "dirty-err", ArgumentsJson: `{}`})
	if err != nil {
		t.Fatalf("ExecuteTool 传输层错误: %v", err)
	}
	if !utf8.ValidString(resp.Error) || !strings.Contains(resp.Error, "\uFFFD") {
		t.Fatalf("Error 应净化为合法 UTF-8: %q", resp.Error)
	}

	resp, err = srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "dirty-view", ArgumentsJson: `{}`})
	if err != nil {
		t.Fatalf("ExecuteTool 传输层错误: %v", err)
	}
	if !utf8.ValidString(resp.ViewJson) {
		t.Fatalf("ViewJson 必须为合法 UTF-8: %q", resp.ViewJson)
	}
	if _, err := pbproto.Marshal(resp); err != nil {
		t.Fatalf("proto.Marshal 失败（gRPC 将拒发）: %v", err)
	}
}
