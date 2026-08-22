package plugin

import (
	"time"
)

// PluginState 宿主侧插件运行状态机。
//
// 状态命名尽量贴近 DSH(deepseek-harness) 的 Cordis Fiber 术语，保留语义对齐：
//   - active     ≈ ACTIVE     （已加载并提供服务/依赖已就绪）
//   - unloading  ≈ UNLOADING  （卸载中，disposer/清理执行）
//   - disposed   ≈ DISPOSED   （已卸载，终态，不可再启，除非重新加载）
//   - failed     ≈ FAILED     （加载或运行失败，可被重新加载走一遍流程）
//
// 进程式插件比 DSH 多出 spawn→connect→ready 三个更细的过程态，用于在
// “拉起子进程 → 握手建链 → 拿到业务对象”各阶段都能被观测。
type PluginState string

const (
	// StatePending 配置已声明但依赖未满足（如 DependsOn 的 LLM/Tool 尚未就绪），
	// 尚不拉起子进程（对应 DSH PENDING）。
	StatePending PluginState = "pending"
	// StateSpawned 进程已创建，尚未握手（无 DSH 直译）。
	StateSpawned PluginState = "spawned"
	// StateConnecting go-plugin 握手/建链中（DSH LOADING 的前半段）。
	StateConnecting PluginState = "connecting"
	// StateReady 业务对象已 Dispense 并注册到 Manager，依赖/健康检查尚未就绪。
	StateReady PluginState = "ready"
	// StateActive 依赖与健康检查就绪，可对外服务（对应 DSH ACTIVE）。
	StateActive PluginState = "active"
	// StateUnloading 卸载中，尝试优雅关闭（对应 DSH UNLOADING）。
	StateUnloading PluginState = "unloading"
	// StateDisposed 已停止/已卸载（对应 DSH DISPOSED，终态）。
	StateDisposed PluginState = "disposed"
	// StateFailed 加载或运行失败（对应 DSH FAILED；可被热重载重新走流程）。
	StateFailed PluginState = "failed"
)

// pluginStateTransitions 定义合法的状态迁移表：from -> {to: true}。
// 非法迁移会被记录告警，便于尽早暴露流程漏步。
var pluginStateTransitions = map[PluginState]map[PluginState]bool{
	StatePending: {
		StateSpawned:    true, // 依赖就绪，重新拉起
		StateConnecting: true, // 动态注入后依赖就绪，provider 直接进入握手
		StateActive:     true, // 已拉起的 PENDING agent，依赖就绪后直接激活
		StateFailed:     true,
		StateDisposed:   true,
	},
	StateSpawned: {
		StateConnecting: true,
		StateFailed:     true,
		StateDisposed:   true,
	},
	StateConnecting: {
		StateReady:    true,
		StateFailed:   true,
		StateDisposed: true,
	},
	StateReady: {
		StateActive:    true,
		StatePending:   true, // 已加载/已拉起进程，但依赖不足（如 LLM 未就绪），退回待办等待注入
		StateUnloading: true,
		StateFailed:    true,
		StateDisposed:  true,
	},
	StateActive: {
		StateUnloading: true,
		StateFailed:    true,
		StateDisposed:  true,
	},
	StateUnloading: {
		StateDisposed: true,
		StateFailed:   true,
	},
	// 终态：不再迁移
	StateDisposed: {},
	StateFailed:   {},
}

// validPluginTransition 判断 from->to 是否为合法迁移。
func validPluginTransition(from, to PluginState) bool {
	nexts, ok := pluginStateTransitions[from]
	if !ok {
		return false
	}
	return nexts[to]
}

// RuntimeState 某个插件的运行时状态快照。
// 由 Manager 在持有 mu 的情况下推进并维护，供 Admin/TUI 观测。
type RuntimeState struct {
	State     PluginState `json:"state"`
	Type      string      `json:"type"` // "llm" | "agent" | "tool" | "dsc" | "policy"
	UpdatedAt time.Time   `json:"updated_at"`
	LastError string      `json:"last_error,omitempty"`
}
