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
	// RunStream 以流式方式执行循环，返回增量输出通道；关闭表示结束
	RunStream(ctx context.Context, input string) (<-chan *RunStreamResponse, error)
	// 可以添加其他方法，如 Name(), Version() 等
	Name(ctx context.Context) string
	Version(ctx context.Context) string
	// RegisterServices 一次性注入 Agent 运行所需的依赖 serviceID（LLM 与 Tool）。
	// 取代原先的 SetLLMServiceID/SetToolServiceID 两段式握手，保证一次性原子下发。
	RegisterServices(ctx context.Context, llmServiceID, toolServiceID uint32) error
	// SwitchSession 切换当前会话（事件溯源 store 中按 id 加载并接管）。
	// 用于 TUI 多会话切换；目标会话不存在时返回错误。
	SwitchSession(ctx context.Context, sessionID string) error
	// SetPlanMode 设置当前会话的 plan 模式（log-only plan/mode 事件，fold 恢复）。
	// 用于 TUI /plan 命令；激活时在 system prompt 注入部署方配置的 section，
	// 供模型先探索与设计，再通过 exit_plan_mode 工具呈现完整计划并退出。
	SetPlanMode(ctx context.Context, active bool) error
	// SetHistoryInjection 设置当前会话的历史注入条数上限（log-only history/limit
	// 事件，fold 恢复）：count < 0 不限制（缺省）；0 不注入历史；> 0 只注入最近
	// count 条派生消息。用于 TUI /settings history 命令控制本地模型预填充长度。
	SetHistoryInjection(ctx context.Context, count int) error
	// SetUserQuestionsService 注入宿主挂载在 broker 上的 UserQuestionsService ID
	// （0 = 无通道）。agent 的工具（如 exit_plan_mode）据此向用户提问并等待回答。
	SetUserQuestionsService(ctx context.Context, serviceID uint32) error
	// Shutdown 优雅关闭 Agent（用于热加载前）
	Shutdown(ctx context.Context, force bool) error
	// InjectMessage 将一条用户消息实时注入到当前运行中的会话历史末端，
	// 使模型在下一次 LLM 迭代即可看到（无需停止或等待本轮完成）。
	InjectMessage(ctx context.Context, content string) error
	// DebugSnapshot 返回 agent 当前运行时的调试快照（会话历史、token 用量、
	// turn 与 plan/goal 状态）。供 ADMIN API DEBUGGER 端点与自动化测试观察。
	DebugSnapshot(ctx context.Context) (*AgentDebugSnapshot, error)
}

// AgentDebugMessage 调试快照中派生历史里的一条消息。
type AgentDebugMessage struct {
	Role    string `json:"role"` // "system" | "user" | "assistant" | "tool"
	Content string `json:"content"`
}

// AgentGoalDebugInfo 目标的调试信息（对齐 session.Goal 的可观察字段）。
type AgentGoalDebugInfo struct {
	Phase          string `json:"phase"`
	Revision       int    `json:"revision"`
	MaxRounds      int    `json:"max_rounds"`
	Activation     string `json:"activation"`
	Objective      string `json:"objective"`
	CompletedSteps int    `json:"completed_steps"`
}

// AgentDebugSnapshot 是 agent 运行时的调试快照，由 DebugSnapshot RPC 返回，
// 供 ADMIN API 的 DEBUGGER 端点与自动化测试读取 agent 内部运行状态。
type AgentDebugSnapshot struct {
	SessionID        string               `json:"session_id"`
	TurnCount        int                  `json:"turn_count"`
	PlanActive       bool                 `json:"plan_active"`
	Goal             *AgentGoalDebugInfo  `json:"goal,omitempty"`
	LastPromptTokens int32                `json:"last_prompt_tokens"`
	Messages         []*AgentDebugMessage `json:"messages"`
}

// AgentResult 是 Agent 执行后的结果
type AgentResult struct {
	Output string `json:"output"`
	Status string `json:"status"` // "success", "error"
}

// RunStreamResponse 是 Agent 流式运行过程中的一帧增量输出
type RunStreamResponse struct {
	Output string `json:"output"` // 增量输出（文本增量或工具调用提示）
	Status string `json:"status"` // "streaming" | "reasoning" | "tool" | "success" | "error"
	Error  string `json:"error,omitempty"`
	// Usage 是本轮（一次 RunStream）累计的 token 用量，仅在 success 帧上携带
	Usage *Usage `json:"usage,omitempty"`
	// Reasoning 是思考过程增量文本（status="reasoning" 帧携带）
	Reasoning string `json:"reasoning,omitempty"`
	// ToolName/ToolArgs 是工具帧携带的工具名与参数 JSON（调用帧与成功结果帧均携带；
	// 结果帧的 ToolArgs 供 TUI 更新待办面板等整表工具），供 TUI 以「● Verb(arg)」
	// 卡片形式渲染，替代简陋的 [调用工具: xxx] 提示。
	ToolName string `json:"tool_name,omitempty"`
	ToolArgs string `json:"tool_args,omitempty"`
	// ToolResult 是工具结果帧携带的结果内容，供 TUI 以「└」gutter 缩进展示。
	ToolResult string `json:"tool_result,omitempty"`
	// Turn/Step 是对齐 DSH 的轮/步编号：轮为一次受理输入的排空，步为一次模型请求
	// 及其引发的工具执行。随每帧携带，供 TUI 状态行实时显示。
	Turn int32 `json:"turn,omitempty"`
	Step int32 `json:"step,omitempty"`
}

// Usage 是一次 LLM 调用的 token 用量统计
// CacheReadInputTokens 为命中 prompt 缓存的输入 token（Anthropic cache_read / OpenAI cached_tokens），
// CacheCreationInputTokens 为写入缓存的输入 token（Anthropic cache_creation）。
type Usage struct {
	PromptTokens             int32 `json:"prompt_tokens"`
	CompletionTokens         int32 `json:"completion_tokens"`
	TotalTokens              int32 `json:"total_tokens"`
	CacheReadInputTokens     int32 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int32 `json:"cache_creation_input_tokens"`
}

// Handshake 是宿主和插件间的握手配置
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "DSC_PLUGIN",
	MagicCookieValue: "dsc-plugin-2026",
}
