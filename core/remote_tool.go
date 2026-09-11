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
//
// 通用 panic recover：插件 gRPC 客户端调用若 panic（如 nil client / 序列化失败），
// 转为错误返回而非崩溃宿主。插件 SDK 层（sdk/tool.go）已先行 recover 一次（Handler
// panic），本层兜底覆盖 gRPC 传输异常与未用 SDK 的插件。对齐「工具意外不中断会话」
// 的最终防线设计（与 core.executeToolBody 的 panic recover 互补）。
func (r *RemoteTool) ExecuteWithView(ctx context.Context, args json.RawMessage) (result string, viewJSON string, errRet error) {
	defer func() {
		if rv := recover(); rv != nil {
			errRet = fmt.Errorf("tool %s panicked (remote): %v", r.name, rv)
		}
	}()
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
