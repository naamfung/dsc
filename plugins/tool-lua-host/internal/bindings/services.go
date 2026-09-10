// Package bindings 提供 dsc.* 内建的服务侧基础设施：
//   - Store：插件进程内 KV（脚本间共享，存 Go any，JSON 可序列化）
//   - HookRegistry：脚本注册的宿主钩子（BeforeTool/AfterTool/OnEvent）
//
// 执行钩子/后台任务需要对应脚本的 VM（非并发安全），因此由 host 层提供
// 执行回调（Services.HookRun / SpawnJob），bindings 只负责注册。
package bindings

import (
	"sync"

	lua "github.com/wippyai/go-lua"
)

// Store 插件进程内 KV 存储（脚本间共享）。
type Store struct {
	mu   sync.Mutex
	data map[string]any
}

// NewStore 创建 KV 存储。
func NewStore() *Store { return &Store{data: make(map[string]any)} }

// Get 读取值；不存在返回 (nil, false)。
func (s *Store) Get(key string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok
}

// Set 写入值。
func (s *Store) Set(key string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = v
}

// Delete 删除键。
func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// HookHandler 脚本注册的一个钩子 handler。
type HookHandler struct {
	Script string
	Fn     *lua.LFunction
}

// HookRegistry 脚本注册的宿主钩子集合（按注册顺序执行）。
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

// Snapshots 返回三类钩子的快照（按注册顺序）。
func (r *HookRegistry) Snapshots() (before, after, onEvent []HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before = append(before, r.before...)
	after = append(after, r.after...)
	onEvent = append(onEvent, r.onEvent...)
	return
}
