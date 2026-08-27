package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"dsc/libs/vodka"
)

// adminAuth 管理 API 的 Token 认证中间件。
// 通过环境变量 DSC_ADMIN_TOKEN 配置认证 Token，未设置则不启用认证。
// 支持请求头 "Authorization: Bearer <token>" 或直接 "<token>"。
func adminAuth(c *vodka.Context) error {
	token := os.Getenv("DSC_ADMIN_TOKEN")
	if token == "" {
		return c.Next()
	}
	provided := c.Request.Header.Get("Authorization")
	if strings.HasPrefix(provided, "Bearer ") {
		provided = strings.TrimPrefix(provided, "Bearer ")
	}
	if provided != token {
		return vodka.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	return c.Next()
}

// StartAdmin 啟動管理 API HTTP 服務（基於 Vodka 框架）
func (m *Manager) StartAdmin(addr string) {
	e := vodka.New()

	admin := e.Group("/plugins")
	admin.Use(adminAuth)
	admin.Post("/load", m.handleLoad)
	admin.Post("/unload", m.handleUnload)
	admin.Post("/reload", m.handleReload)
	admin.Get("/list", m.handleList)
	admin.Get("/events", m.handleEvents)
	admin.Post("/metadata", m.handleMetadata)

	// DEBUGGER 观察渠道：读取 agent 插件当前运行时的调试快照（会话历史、token 用量、
	// turn 与 plan/goal 状态）。为自动化测试提供无障碍观察代理运行内部状态的端点。
	// 现有 /plugins/* 均为宿主侧（插件生命周期）观测，缺 agent 运行时内部视角。
	// 因快照含完整会话历史（隐私敏感），此路由仅在显式启用（-debugger）时开放。
	if m.config != nil && m.config.DebuggerEnabled {
		debugger := e.Group("/debugger")
		debugger.Use(adminAuth)
		debugger.Get("/agent", m.handleDebuggerAgent)
	}

	// 定时任务管理（认证与 /plugins 一致）
	crons := e.Group("/cron")
	crons.Use(adminAuth)
	crons.Get("/list", m.handleCronList)
	crons.Post("/add", m.handleCronAdd)
	crons.Post("/remove", m.handleCronRemove)
	crons.Post("/enable", m.handleCronEnable)

	go func() {
		m.logger.Info("admin api listening", "addr", addr)
		e.Server.Addr = addr
		if err := e.Server.ListenAndServe(); err != nil {
			m.logger.Error("admin api error", "error", err)
		}
	}()
}

// handleLoad 處理加載插件請求
func (m *Manager) handleLoad(c *vodka.Context) error {
	var entry PluginEntry
	if err := bindBody(c, &entry); err != nil {
		return err
	}

	if err := m.LoadPlugin(entry); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to load core: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "core loaded"})
}

// handleUnload 處理卸載插件請求
func (m *Manager) handleUnload(c *vodka.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := bindBody(c, &req); err != nil {
		return err
	}

	if err := m.UnloadPlugin(req.Name); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to unload core: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "core unloaded"})
}

// handleReload 處理重載插件請求
func (m *Manager) handleReload(c *vodka.Context) error {
	var entry PluginEntry
	if err := bindBody(c, &entry); err != nil {
		return err
	}

	// 先卸載
	if err := m.UnloadPlugin(entry.Name); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to unload core: %v", err))
	}

	// 再加載
	if err := m.LoadPlugin(entry); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to reload core: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "core reloaded"})
}

// handleList 處理列出插件請求
func (m *Manager) handleList(c *vodka.Context) error {
	plugins := m.ListPlugins()
	return c.JSON(vodka.Map{
		"status":  "success",
		"plugins": plugins,
	})
}

// handleEvents 以 Server-Sent Events 流推送插件的生命周期状态迁移事件。
// 订阅者连接后持续收到 "event: state" 帧，数据为 PluginEvent 的 JSON 编码。
// 用于观测插件状态机的实时迁移，替代轮询 /plugins/list。
func (m *Manager) handleEvents(c *vodka.Context) error {
	c.Response.Header().Set("Content-Type", "text/event-stream")
	c.Response.Header().Set("Cache-Control", "no-cache")
	c.Response.Header().Set("Connection", "keep-alive")

	flusher, ok := c.Response.Writer.(http.Flusher)
	if !ok {
		return vodka.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
	}

	ch, cancel := m.Subscribe()
	defer cancel()

	// 建立连接的握手注释帧，便于客户端确认流已就绪
	if _, err := fmt.Fprint(c.Response.Writer, ": connected\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue // 序列化失败的事件跳过，不影响连接
			}
			if _, err := fmt.Fprintf(c.Response.Writer, "event: state\ndata: %s\n\n", b); err != nil {
				return nil // 客户端断开
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return nil // 客户端断开，结束订阅
		}
	}
}

// handleMetadata 處理獲取插件元數據請求
func (m *Manager) handleMetadata(c *vodka.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := bindBody(c, &req); err != nil {
		return err
	}

	info, exists := m.GetPluginMetadata(req.Name)
	if !exists {
		return vodka.NewHTTPError(http.StatusNotFound, fmt.Sprintf("core '%s' not found", req.Name))
	}

	return c.JSON(vodka.Map{
		"status": "success",
		"metadata": map[string]string{
			"type":        info.Type,
			"name":        info.Name,
			"version":     info.Version,
			"api_version": info.ApiVersion,
		},
	})
}

// handleDebuggerAgent 转发 DEBUGGER 快照查询到 agent 插件：读取其当前运行时的
// 调试快照（会话历史含实时注入的消息、token 用量、turn 计数与 plan/goal 状态）。
// 用于自动化测试无障碍观察代理运行内部状态；未指定 agent 时使用主 agent。
func (m *Manager) handleDebuggerAgent(c *vodka.Context) error {
	name := c.Query("name")
	if name == "" {
		name = m.GetMainAgentName()
	}
	if name == "" {
		return vodka.NewHTTPError(http.StatusNotFound, "no agent configured")
	}

	agent, ok := m.GetAgent(name)
	if !ok {
		return vodka.NewHTTPError(http.StatusNotFound, fmt.Sprintf("agent '%s' not found", name))
	}

	snap, err := agent.DebugSnapshot(c.Request.Context())
	if err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("debug snapshot failed: %v", err))
	}

	return c.JSON(vodka.Map{
		"status": "success",
		"agent":  name,
		"snapshot": map[string]interface{}{
			"session_id":         snap.SessionID,
			"turn_count":         snap.TurnCount,
			"plan_active":        snap.PlanActive,
			"goal":               snap.Goal,
			"last_prompt_tokens": snap.LastPromptTokens,
			"messages":           snap.Messages,
		},
	})
}
