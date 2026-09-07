package core

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
)

// CapabilityTool 可选接口：宿主内部工具可报告自身声明的能力标签，供宿主按
// 「能力」而非「插件名」做前置门控（对齐 ApprovalRequester 可选接口风格）。
type CapabilityTool interface {
	// HasCapability 返回该工具是否声明了指定能力标签。
	HasCapability(cap string) bool
}

// RemoteTool 實現了 ToolDefinition，通過 gRPC 調用遠程工具
type RemoteTool struct {
	name         string
	description  string
	schema       json.RawMessage
	client       proto.ToolServiceClient
	capabilities []string
}

// HasCapability 报告该工具是否声明了指定能力标签（源自插件工具声明的 proto.Tool.capabilities。
// 空集合对任意能力返回 false，向后兼容无能力声明的旧工具）。
func (r *RemoteTool) HasCapability(cap string) bool {
	for _, c := range r.capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

func (r *RemoteTool) Name() string {
	return r.name
}

func (r *RemoteTool) Description() string {
	return r.description
}

func (r *RemoteTool) ParametersSchema() json.RawMessage {
	return r.schema
}

func (r *RemoteTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	result, _, err := r.ExecuteWithView(ctx, args)
	return result, err
}

// ExecuteWithView 与 Execute 语义相同，额外返回插件的 ViewJson（Tool.ViewFn 产物）。
// 宿主聚合路径据此透传插件视图，避免视图在聚合层被丢弃。
func (r *RemoteTool) ExecuteWithView(ctx context.Context, args json.RawMessage) (string, string, error) {
	resp, err := r.client.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName:      r.name,
		ArgumentsJson: string(args),
	})
	if err != nil {
		return "", "", err
	}
	if resp.Error != "" {
		return "", "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Content, resp.ViewJson, nil
}
