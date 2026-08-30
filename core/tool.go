package core

import (
	"context"
	"encoding/json"
)

// ToolDefinition 是編程智能體可調用的工具接口
type ToolDefinition interface {
	// Name 工具唯一標識，如 "read_file"
	Name() string
	// Description 供 LLM 理解工具用途
	Description() string
	// ParametersSchema 返回 JSON Schema，定義參數結構
	ParametersSchema() json.RawMessage
	// Execute 執行工具，接收 JSON 參數，返回結果或錯誤
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// TimeoutProvider 可选接口：工具声明单次调用超时（毫秒，<=0 不设）。
// 对齐 DSH timeout-policy：声明 timeoutMs 的工具在执行时获得协作式截止时间，
// 截止时间先到时返回 TOOL_TIMEOUT。协作式 = 工具必须转发/感知 ctx 取消。
type TimeoutProvider interface {
	TimeoutMs() int
}

// ViewExecutor 可选接口：工具额外返回结构化视图 spec（ViewJson 字符串）。
// RemoteTool（插件工具）据此透传插件 Tool.ViewFn 的产物；宿主工具（run_code 等）
// 自行构造视图；由 ToolGRPCServer 统一填充 ExecuteToolResponse.ViewJson，使宿主
// 聚合路径（agent → ToolGRPCServer → TUI）与插件直连路径的视图传播一致。
type ViewExecutor interface {
	ExecuteWithView(ctx context.Context, args json.RawMessage) (result string, viewJSON string, err error)
}

// PluginToolCall 表示 LLM 發起的一次工具調用
type PluginToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult 工具執行結果
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	Error      string `json:"error,omitempty"`
}
