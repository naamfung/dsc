package plugin

import (
	"context"

	plugin "github.com/hashicorp/go-plugin"
)

// DSCPlugin 是插件必须实现的业务接口
type DSCPlugin interface {
	// Name 返回插件名称
	Name(ctx context.Context) string
	// Version 返回插件版本（用于兼容性校验）
	Version(ctx context.Context) string
	// Execute 执行插件的核心逻辑
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// ExecuteRequest 执行请求
type ExecuteRequest struct {
	Input  string            `json:"input"`
	Params map[string]string `json:"params"`
}

// ExecuteResponse 执行响应
type ExecuteResponse struct {
	Output  string `json:"output"`
	Status  string `json:"status"` // "success", "error"
	Message string `json:"message,omitempty"`
}

// Agent 定义了核心循环的契约
type Agent interface {
	// Run 是 Agent 的主入口，负责执行整个循环
	Run(ctx context.Context, input string) (*AgentResult, error)
	// 可以添加其他方法，如 Name(), Version() 等
	Name(ctx context.Context) string
	Version(ctx context.Context) string
	// SetLLMServiceID 设置 LLM service ID
	SetLLMServiceID(ctx context.Context, id uint32) error
	// SetToolServiceID 设置 Tool service ID
	SetToolServiceID(ctx context.Context, id uint32) error
}

// AgentResult 是 Agent 执行后的结果
type AgentResult struct {
	Output string `json:"output"`
	Status string `json:"status"` // "success", "error"
}

// Handshake 是宿主和插件间的握手配置
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "DSC_PLUGIN",
	MagicCookieValue: "dsc-plugin-2026",
}