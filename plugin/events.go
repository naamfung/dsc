package plugin

import (
	"time"
)

// PluginEvent 是一次插件生命周期的状态迁移事件。
// 对应 DSH 的 internal/status：状态机的每次推进都对外广播，供 Admin/TUI/持久化实时订阅，
// 取代原先仅能轮询 GetPluginState/ListPlugins 的观测方式。
type PluginEvent struct {
	Name  string      `json:"name"`            // 插件名
	Type  string      `json:"type,omitempty"`  // "llm" | "agent" | "tool" | "policy" | "dsc"
	From  PluginState `json:"from,omitempty"`  // 迁移前状态（空表示该插件首次进入状态机）
	To    PluginState `json:"to"`              // 迁移后状态
	Error string      `json:"error,omitempty"` // 迁移携带的错误信息（进入 failed 等状态时）
	Time  time.Time   `json:"time"`            // 事件发生时间
}

// Subscribe 订阅插件的生命周期事件流。
// 返回只读事件通道与取消函数；取消后不再接收后续事件。
// 事件由状态机在推进状态时非阻塞派发：订阅者积压太多时事件会被丢弃，避免阻塞状态机。
func (m *Manager) Subscribe() (<-chan PluginEvent, func()) {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	id := m.nextSubID
	m.nextSubID++
	ch := make(chan PluginEvent, 128)
	m.subscribers[id] = ch
	return ch, func() {
		m.eventsMu.Lock()
		delete(m.subscribers, id)
		m.eventsMu.Unlock()
	}
}

// publishEventLocked 向所有订阅者非阻塞派发事件（需已持有 m.mu）。
// 内部使用独立的 eventsMu 保护订阅表，避免与状态机锁相互纠缠。
func (m *Manager) publishEventLocked(ev PluginEvent) {
	m.eventsMu.RLock()
	subs := make([]chan PluginEvent, 0, len(m.subscribers))
	for _, ch := range m.subscribers {
		subs = append(subs, ch)
	}
	m.eventsMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default: // 订阅者积压，丢弃本事件以免阻塞状态机
		}
	}
}
