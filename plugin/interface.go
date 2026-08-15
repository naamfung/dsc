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

// Handshake 是宿主和插件间的握手配置
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "DSC_PLUGIN",
	MagicCookieValue: "dsc-plugin-2026",
}