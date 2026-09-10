package session

import (
	"context"
	"fmt"
	"sync"
)

// 会话持久化抽象缝（对齐 DSH/Cordis 的 ctx.sessionPersistence 服务）。
//
// DSH 的 SessionPersistence 是一个抽象 Service，后端（如 session-persistence-jsonl）
// 实现 create / open / stat / list / flush。每个会话经 SessionHandle 操作：
// append（追加事件）、read（读取事件）、flush（强制落盘）、close（释放所有权）。
//
// DSC 的适配：DSC 当前已有一个内联的 Store（session/store.go），直接操作 JSONL 文件。
// 本抽象缝把「持久化策略」从「JSONL 文件操作」中解耦——
//   - SessionPersistence 接口：定义 create / open / stat / list / flush 契约
//   - JSONLPersistence：默认实现，封装既有 Store 的 JSONL 逻辑
//   - 未来可扩展 SQLite / 远程存储等后端，只需实现接口
//
// 调用方（Store）经接口操作，不关心具体后端。

// SessionPersistenceSnapshot 会话的轻量级快照（对齐 DSH SessionPersistenceSnapshot）。
// 不读取完整事件日志，仅返回元数据。
type SessionPersistenceSnapshot struct {
	// ID 会话标识。
	ID string `json:"id"`
	// EventCount 事件总数。
	EventCount int `json:"event_count"`
	// LastModified 最后修改时间（Unix 毫秒）。
	LastModified int64 `json:"last_modified"`
	// Preview 首条 user 消息的截断预览。
	Preview string `json:"preview"`
	// Revision 不可透明版本标记（用于 stat 变更检测）。
	Revision string `json:"revision"`
}

// SessionPersistence 会话持久化接口（对齐 DSH SessionPersistence 抽象 Service）。
// 后端实现此接口，调用方经接口操作会话。
type SessionPersistence interface {
	// Create 创建新会话，返回可写的 SessionHandle。
	Create(ctx context.Context, id string) (SessionHandle, error)
	// Open 打开已有会话，返回 SessionHandle。
	// access 为 "read" 或 "write"；write 模式获得单写者所有权。
	Open(ctx context.Context, id string, access string) (SessionHandle, error)
	// Stat 返回会话的轻量级快照（不读取完整日志）。
	Stat(ctx context.Context, id string) (*SessionPersistenceSnapshot, error)
	// List 列出所有会话的快照。
	List(ctx context.Context) ([]SessionPersistenceSnapshot, error)
	// Flush 强制所有 write 模式的 SessionHandle 落盘。
	Flush(ctx context.Context) error
}

// SessionHandle 会话操作句柄（对齐 DSH SessionHandle）。
// 从 Create / Open 获得，经此追加事件、读取事件、关闭释放所有权。
type SessionHandle interface {
	// Append 追加一个事件到会话日志（best-effort，可能缓冲到 Flush 时落盘）。
	Append(ctx context.Context, event *Event) error
	// Read 读取会话日志中的事件（从 offset 开始，最多 limit 条）。
	Read(ctx context.Context, offset int, limit int) ([]*Event, error)
	// Flush 强制把缓冲的事件落盘。
	Flush(ctx context.Context) error
	// Close 关闭句柄，释放所有权。未 Flush 的缓冲事件会被强制落盘。
	Close(ctx context.Context) error
	// ID 返回会话 ID。
	ID() string
}

// JSONLPersistence 默认 JSONL 持久化后端（对齐 DSH session-persistence-jsonl）。
// 封装既有 Store 的 JSONL 文件操作。
type JSONLPersistence struct {
	store *Store
	mu    sync.Mutex
	// openHandles 跟踪当前打开的 write 句柄（单写者所有权检查）
	openHandles map[string]bool
}

// NewJSONLPersistence 创建 JSONL 持久化后端。
func NewJSONLPersistence(store *Store) *JSONLPersistence {
	return &JSONLPersistence{
		store:       store,
		openHandles: make(map[string]bool),
	}
}

// Create 创建新会话。
func (p *JSONLPersistence) Create(ctx context.Context, id string) (SessionHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openHandles[id] {
		return nil, fmt.Errorf("session %q is already owned by another write handle", id)
	}
	p.openHandles[id] = true
	return &jsonlHandle{store: p.store, id: id, writeMode: true, parent: p}, nil
}

// Open 打开已有会话。
func (p *JSONLPersistence) Open(ctx context.Context, id string, access string) (SessionHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	writeMode := access == "write"
	if writeMode {
		if p.openHandles[id] {
			return nil, fmt.Errorf("session %q is already owned by another write handle", id)
		}
		p.openHandles[id] = true
	}
	return &jsonlHandle{store: p.store, id: id, writeMode: writeMode, parent: p}, nil
}

// Stat 返回会话快照。
func (p *JSONLPersistence) Stat(ctx context.Context, id string) (*SessionPersistenceSnapshot, error) {
	infos, err := p.store.List()
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if info.ID == id {
			return &SessionPersistenceSnapshot{
				ID:           id,
				EventCount:   info.Events,
				LastModified: info.LastTime.UnixMilli(),
				Preview:      info.Preview,
				Revision:     fmt.Sprintf("%d-%d", info.LastTime.UnixMilli(), info.Events),
			}, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", id)
}

// List 列出所有会话快照。
func (p *JSONLPersistence) List(ctx context.Context) ([]SessionPersistenceSnapshot, error) {
	infos, err := p.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]SessionPersistenceSnapshot, 0, len(infos))
	for _, info := range infos {
		out = append(out, SessionPersistenceSnapshot{
			ID:           info.ID,
			EventCount:   info.Events,
			LastModified: info.LastTime.UnixMilli(),
			Preview:      info.Preview,
			Revision:     fmt.Sprintf("%d-%d", info.LastTime.UnixMilli(), info.Events),
		})
	}
	return out, nil
}

// Flush 强制所有 write 句柄落盘（JSONL 后端每次 Append 直接写文件，此为 no-op）。
func (p *JSONLPersistence) Flush(ctx context.Context) error {
	return nil // JSONL 后端是同步写入的，无需额外 flush
}

// jsonlHandle JSONL 后端的 SessionHandle 实现。
type jsonlHandle struct {
	store     *Store
	id        string
	writeMode bool
	parent    *JSONLPersistence // 反向引用：Close 时经它释放 openHandles[id]
	mu        sync.Mutex
	closed    bool
}

func (h *jsonlHandle) Append(ctx context.Context, event *Event) error {
	if !h.writeMode {
		return fmt.Errorf("session handle for %q is read-only", h.id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return fmt.Errorf("session handle for %q is closed", h.id)
	}
	sess, err := h.store.Ensure(h.id)
	if err != nil {
		return err
	}
	sess.Append(event.Type, event.Data, event.Surface)
	return h.store.Save(sess)
}

func (h *jsonlHandle) Read(ctx context.Context, offset int, limit int) ([]*Event, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("session handle for %q is closed", h.id)
	}
	sess, err := h.store.Load(h.id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return []*Event{}, nil
	}
	events := sess.Events()
	if offset >= len(events) {
		return []*Event{}, nil
	}
	end := offset + limit
	if end > len(events) {
		end = len(events)
	}
	return events[offset:end], nil
}

func (h *jsonlHandle) Flush(ctx context.Context) error {
	return nil // JSONL 后端同步写入，无需 flush
}

// Close 关闭句柄。write 模式句柄经 parent 释放 openHandles[id]，
// 否则后续对同一 session 的 Create/Open(write) 会因「already owned」永久失败。
// 多次 Close 是幂等的（已关闭直接返回 nil）。
func (h *jsonlHandle) Close(ctx context.Context) error {
	h.mu.Lock()
	wasClosed := h.closed
	h.closed = true
	h.mu.Unlock()
	if wasClosed {
		return nil
	}
	if h.writeMode && h.parent != nil {
		h.parent.mu.Lock()
		delete(h.parent.openHandles, h.id)
		h.parent.mu.Unlock()
	}
	return nil
}

func (h *jsonlHandle) ID() string {
	return h.id
}
