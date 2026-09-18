package core

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"dsc/libs/vodka"
)

// CredentialStore 懒初始化：首次调用时按 ExecDir/credentials 创建。
// 盘活 core/credentials.go 死模块——经 admin API /credentials/* 暴露给外部工具。
func (m *Manager) ensureCredentialStore() *CredentialStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.credentialStore == nil {
		dir := PJoin(m.config.ExecDir, "credentials")
		m.credentialStore = NewCredentialStore(dir)
	}
	return m.credentialStore
}

// FeedbackStore 懒初始化：首次调用时按 ExecDir/feedback.jsonl 创建。
// 盘活 core/feedback.go 死模块——经 admin API /feedback/* 暴露给外部工具。
func (m *Manager) ensureFeedbackStore() *FeedbackStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.feedbackStore == nil {
		filePath := PJoin(m.config.ExecDir, "feedback.jsonl")
		m.feedbackStore = NewFeedbackStore(filePath)
	}
	return m.feedbackStore
}

// registerCredentialAdminRoutes 注册 /credentials/* admin 端点。
// 对齐 DSH packages/credentials/credentials-local 的 admin 暴露面：
//   - GET  /credentials/list              列出所有插件的凭据键（不含值）
//   - GET  /credentials/get?plugin=...&key=...  获取单条凭据值
//   - POST /credentials/set                设置单条凭据（持久化到文件）
//   - POST /credentials/delete             删除单条凭据
func (m *Manager) registerCredentialAdminRoutes(group *vodka.RouteGroup) {
	group.Get("/credentials/list", m.handleCredentialList)
	group.Get("/credentials/get", m.handleCredentialGet)
	group.Post("/credentials/set", m.handleCredentialSet)
	group.Post("/credentials/delete", m.handleCredentialDelete)
}

// registerFeedbackAdminRoutes 注册 /feedback/* admin 端点。
// 对齐 DSH packages/feedback/message-feedback 的 admin 暴露面：
//   - POST /feedback/add    添加反馈记录
//   - GET  /feedback/list   列出反馈（按 session 可选过滤）
//   - GET  /feedback/stats  统计信息
func (m *Manager) registerFeedbackAdminRoutes(group *vodka.RouteGroup) {
	group.Post("/feedback/add", m.handleFeedbackAdd)
	group.Get("/feedback/list", m.handleFeedbackList)
	group.Get("/feedback/stats", m.handleFeedbackStats)
}

// ---- credentials handlers ----

func (m *Manager) handleCredentialList(c *vodka.Context) error {
	cs := m.ensureCredentialStore()
	ctx := context.Background()
	keys, err := cs.ListKeys(ctx, "")
	if err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(map[string]any{"keys": keys})
}

func (m *Manager) handleCredentialGet(c *vodka.Context) error {
	pluginName := c.QueryParam("plugin")
	key := c.QueryParam("key")
	if pluginName == "" || key == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "plugin and key are required")
	}
	cs := m.ensureCredentialStore()
	ctx := context.Background()
	v, err := cs.Get(ctx, pluginName, key)
	if err != nil {
		return vodka.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(map[string]any{"plugin": pluginName, "key": key, "value": v})
}

func (m *Manager) handleCredentialSet(c *vodka.Context) error {
	var req struct {
		Plugin string `json:"plugin"`
		Key    string `json:"key"`
		Value  string `json:"value"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.Plugin == "" || req.Key == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "plugin and key are required")
	}
	cs := m.ensureCredentialStore()
	ctx := context.Background()
	if err := cs.Set(ctx, req.Plugin, req.Key, req.Value); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(map[string]any{"ok": true})
}

func (m *Manager) handleCredentialDelete(c *vodka.Context) error {
	var req struct {
		Plugin string `json:"plugin"`
		Key    string `json:"key"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.Plugin == "" || req.Key == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "plugin and key are required")
	}
	cs := m.ensureCredentialStore()
	ctx := context.Background()
	if err := cs.Delete(ctx, req.Plugin, req.Key); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(map[string]any{"ok": true})
}

// ---- feedback handlers ----

func (m *Manager) handleFeedbackAdd(c *vodka.Context) error {
	var req struct {
		SessionID string         `json:"session_id"`
		Turn      int            `json:"turn"`
		Rating    FeedbackRating `json:"rating"`
		Comment   string         `json:"comment"`
		Message   string         `json:"message"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if req.SessionID == "" {
		return vodka.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	fs := m.ensureFeedbackStore()
	ctx := context.Background()
	fb, err := fs.Add(ctx, req.SessionID, req.Turn, req.Rating, req.Comment, req.Message)
	if err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(fb)
}

func (m *Manager) handleFeedbackList(c *vodka.Context) error {
	fs := m.ensureFeedbackStore()
	ctx := context.Background()
	sessionID := c.QueryParam("session")
	var records []Feedback
	if sessionID != "" {
		records = fs.ListBySession(ctx, sessionID)
	} else {
		records = fs.List(ctx)
	}
	// limit 截断（避免大列表回灌）
	limitStr := c.QueryParam("limit")
	if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n < len(records) {
		records = records[:n]
	}
	return c.JSON(map[string]any{"records": records, "count": len(records)})
}

func (m *Manager) handleFeedbackStats(c *vodka.Context) error {
	fs := m.ensureFeedbackStore()
	ctx := context.Background()
	stats := fs.Stats(ctx)
	return c.JSON(stats)
}
