// Package workflow 提供 DSH 风格的工作流 seam：模型编写的 JS 编排脚本，
// 经 goja 进程内执行，可扇出 subagent（agent() 钩子），返回脚本最终值。
//
// 对齐 DSH workflow 家族的核心契约：
//   - Start 同步完成足够校验（meta 块 + 脚本语法），运行创建前拒绝格式错误
//   - 执行失败以 stopReason=error 兑现，取消以 cancelled 兑现；result 不拒绝
//   - agent() 每次调用归属一个 subagent，计数 agentsStarted
//   - 事件仅供观察，携带 run id + meta，不暴露取消/释放权限
//
// 脚本为 async 模型（对齐 DSH）：agent() 返回 Promise，脚本需 await；
// parallel(thunks) 在并发上限内扇出 thunk，pipeline(items, ...stages) 无跨阶段
// 屏障地逐项跑 stage 链（stage 签名 (previous, item, index)，item 间并发）。
// 子 agent 真并发，受 MaxConcurrentAgents 并发上限与 MaxTotalAgents 总量上限
// 双约束。致命错误（INVALID_ARGUMENT / AGENT_CAP / ITEM_CAP 等）逸出组合器，
// 普通子 agent 失败与普通 stage 错误在脚本侧可见 null。
package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// 停止原因（对齐 DSH WorkflowStopReason）。
const (
	StopCompleted = "completed"
	StopCancelled = "cancelled"
	StopError     = "error"
)

// Meta 脚本身份块（对齐 DSH WorkflowMeta）。
type Meta struct {
	Name        string  `json:"name"`        // 短 kebab-case 名称（必填）
	Description string  `json:"description"` // 一句话描述（必填）
	WhenToUse   string  `json:"when_to_use,omitempty"`
	Phases      []Phase `json:"phases,omitempty"`
}

// Phase 脚本声明的阶段（进度词汇；phase() 调用按标题精确匹配）。
type Phase struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// Result 一次运行的结算结果（对齐 DSH WorkflowResult）。
type Result struct {
	Value         any    `json:"value,omitempty"` // 脚本返回的普通 JSON 数据；null 表示无返回
	StopReason    string `json:"stop_reason"`
	Error         string `json:"error,omitempty"`
	AgentsStarted int    `json:"agents_started"`
}

// RunError 工作流错误（对齐 DSH WorkflowError：code + fatal）。
type RunError struct {
	Code  string
	Fatal bool
	Err   error
}

func (e *RunError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

// 工作流错误码（对齐 DSH WorkflowError codes）。
const (
	ErrMetaInvalid          = "META_INVALID"
	ErrScriptParse          = "SCRIPT_PARSE"
	ErrInvalidArgument      = "INVALID_ARGUMENT"
	ErrAgentCap             = "AGENT_CAP"
	ErrItemCap              = "ITEM_CAP"
	ErrResultUnserializable = "RESULT_UNSERIALIZABLE"
	ErrCancelled            = "CANCELLED"
)

// AgentRunner 子 agent 执行器（宿主注入：Manager.RunSubagent）。
type AgentRunner interface {
	RunAgent(ctx context.Context, prompt string) (string, error)
}

// EventSink 工作流观测事件（nil 安全；只携带只读信息，不暴露取消权限）。
type EventSink interface {
	OnStart(id string, meta Meta)
	OnPhase(id, title string)
	OnLog(id, msg string)
	OnAgentStart(id string, seq int, label string)
	OnAgentEnd(id string, seq int, outcome string) // "completed" | "failed"
	OnEnd(id string, r Result)
}

// StartRequest 一次工作流启动请求。
type StartRequest struct {
	Meta   Meta
	Script string
	Args   any // 以全局变量 args 暴露给脚本
	// MaxTotalAgents 本次运行子 agent 总数上限；<=0 不设限。
	MaxTotalAgents int
	// MaxConcurrentAgents 并发的 agent() 上限；<=0 按可用 CPU 并行度解析。
	// parallel() 扇出的 thunk 受此上限约束（agent goroutine 排队等待）。
	MaxConcurrentAgents int
	// MaxItemsPerCall 一次 parallel() 调用接受的条目数上限；<=0 用默认 4096。
	MaxItemsPerCall int
	Runner          AgentRunner
	Events          EventSink
	Timeout         time.Duration // 单次运行上限；<=0 用默认 30 分钟
}

// defaultTimeout 单次工作流运行默认超时。
const defaultTimeout = 30 * time.Minute

// defaultMaxItemsPerCall 单次 parallel() 调用默认条目上限（对齐 DSH 4096）。
const defaultMaxItemsPerCall = 4096

// Run 持有方负责的一次工作流运行。result 不拒绝：
// 执行失败以 stopReason=error 兑现，取消以 cancelled 兑现。
type Run struct {
	ID     string
	Meta   Meta
	Result <-chan Result

	cancel context.CancelFunc
}

// Cancel 取消运行及其子 agent（脚本侧经 vm.Interrupt 中断）。
func (r *Run) Cancel() { r.cancel() }

// Start 启动一次运行。同步校验 meta 与脚本语法；非法请求返回错误，不产生运行。
// Runner 为 nil 时返回错误（无执行器无法运行）。
func Start(ctx context.Context, req StartRequest) (*Run, error) {
	if err := ValidateMeta(req.Meta); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Script) == "" {
		return nil, &RunError{Code: ErrScriptParse, Err: fmt.Errorf("empty script")}
	}
	if err := checkScriptSyntax(req.Script); err != nil {
		return nil, &RunError{Code: ErrScriptParse, Err: err}
	}
	if req.Runner == nil {
		return nil, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("runner is required")}
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &Run{
		ID:     newID(),
		Meta:   req.Meta,
		cancel: cancel,
	}
	ch := make(chan Result, 1)
	run.Result = ch
	go execute(runCtx, req, run.ID, timeout, ch)
	return run, nil
}

// ValidateMeta 校验 meta 块：name 必填且为 kebab-case，description 必填非空；
// phases 标题必填非空且不重复。
func ValidateMeta(m Meta) error {
	if strings.TrimSpace(m.Name) == "" {
		return &RunError{Code: ErrMetaInvalid, Err: fmt.Errorf("meta.name is required")}
	}
	if !kebabCase.MatchString(m.Name) {
		return &RunError{Code: ErrMetaInvalid, Err: fmt.Errorf("meta.name %q must be lower-kebab-case", m.Name)}
	}
	if strings.TrimSpace(m.Description) == "" {
		return &RunError{Code: ErrMetaInvalid, Err: fmt.Errorf("meta.description is required")}
	}
	seen := map[string]bool{}
	for _, p := range m.Phases {
		if strings.TrimSpace(p.Title) == "" {
			return &RunError{Code: ErrMetaInvalid, Err: fmt.Errorf("phase title is required")}
		}
		if seen[p.Title] {
			return &RunError{Code: ErrMetaInvalid, Err: fmt.Errorf("duplicate phase %q", p.Title)}
		}
		seen[p.Title] = true
	}
	return nil
}

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
