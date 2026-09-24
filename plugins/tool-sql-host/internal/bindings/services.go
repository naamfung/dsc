// Package bindings 提供 dsc.* 内建的服务侧基础设施：
//   - StateStore：插件自持 KV 的抽象（实现落盘在插件自身的 .dsp 里）
//   - HookRegistry：插件注册的宿主钩子（BeforeTool/AfterTool/OnEvent）
//
// 执行钩子/后台任务需要对应插件的 VM（非并发安全），因此由 host 层提供执行回调
// （Services.HookRun / SpawnJob），bindings 只负责注册与转发。
package bindings

import (
	"context"
	"sync"

	lua "github.com/wippyai/go-lua"
)

// Notifier 是宿主事件发布的抽象（生产实现为宿主聚合通知服务）。
// 抽成接口而非直接绑具体客户端类型，使宿主层/测试可注入替身。
type Notifier interface {
	Notify(ctx context.Context, name, dataJSON string) error
}

// StateStore 是插件自持状态的存取抽象，由 dsp.Plugin 实现（落在插件自身 .dsp 的
// dsp_state 表）。接口只暴露键值语义，路径不入参——插件在构造上就无法读写别的插件的状态。
type StateStore interface {
	StateGet(key string) (any, bool, error)
	StateSet(key string, v any) error
	StateDelete(key string) error
}

// SelfDB 是插件对自身 .dsp 的脚本侧访问能力（SQL 与静态内容），同样由 dsp.Plugin 实现。
// 底层为单连接，由 host 层按插件 VM 串行化。
type SelfDB interface {
	Query(stmt string, params []any) ([]map[string]any, error)
	Exec(stmt string, params []any) (int64, error)
	Tables() ([]string, error)
	Blob(name, kind string) ([]byte, bool, error)
}

// PluginInfo 是插件自述信息（注入 dsc.plugin），供脚本自省与日志标注。
type PluginInfo struct {
	Name     string
	Path     string
	ReadOnly bool
}

// HookHandler 插件注册的一个钩子 handler。
type HookHandler struct {
	Plugin string
	Fn     *lua.LFunction
}

// HookRegistry 插件注册的宿主钩子集合（按注册顺序执行）。
type HookRegistry struct {
	mu      sync.Mutex
	before  []HookHandler
	after   []HookHandler
	onEvent []HookHandler
}

// NewHookRegistry 创建钩子注册表。
func NewHookRegistry() *HookRegistry { return &HookRegistry{} }

// AddBefore 注册工具执行前钩子。
func (r *HookRegistry) AddBefore(h HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.before = append(r.before, h)
}

// AddAfter 注册工具执行后钩子。
func (r *HookRegistry) AddAfter(h HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after = append(r.after, h)
}

// AddOnEvent 注册宿主事件订阅钩子。
func (r *HookRegistry) AddOnEvent(h HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onEvent = append(r.onEvent, h)
}

// Drop 移除某插件注册的全部钩子（插件卸载时调用，避免残留 handler 指向已关闭的 VM）。
func (r *HookRegistry) Drop(plugin string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.before = dropByPlugin(r.before, plugin)
	r.after = dropByPlugin(r.after, plugin)
	r.onEvent = dropByPlugin(r.onEvent, plugin)
}

func dropByPlugin(in []HookHandler, plugin string) []HookHandler {
	out := make([]HookHandler, 0, len(in))
	for _, h := range in {
		if h.Plugin != plugin {
			out = append(out, h)
		}
	}
	return out
}

// Snapshots 返回三类钩子的快照（按注册顺序）。
func (r *HookRegistry) Snapshots() (before, after, onEvent []HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before = append(before, r.before...)
	after = append(after, r.after...)
	onEvent = append(onEvent, r.onEvent...)
	return
}
