package core

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"dsc/libs/vodka"
)

// adminAuth 管理 API 的 Token 认证中间件。
// 通过环境变量 DSC_ADMIN_TOKEN 配置认证 Token，未设置则不启用认证。
// 支持请求头 "Authorization: Bearer <token>" 或直接 "<token>"。
// Token 比较使用常量时间比较，避免时序侧信道。
func adminAuth(c *vodka.Context) error {
	token := os.Getenv("DSC_ADMIN_TOKEN")
	if token == "" {
		return c.Next()
	}
	provided := c.Request.Header.Get("Authorization")
	if strings.HasPrefix(provided, "Bearer ") {
		provided = strings.TrimPrefix(provided, "Bearer ")
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		return vodka.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	return c.Next()
}

// StartAdmin 啟動管理 API HTTP 服務（基於 Vodka 框架）
func (m *Manager) StartAdmin(addr string) {
	e := vodka.New()

	// 反慢速攻擊：限制讀取請求頭與空閒連接超時。刻意不設 Read/WriteTimeout——
	// /plugins/events 係長連接 SSE 流（持續寫入），設 WriteTimeout 會中斷訂閱；
	// /plugins/load|unload|reload 要拉起/關閉插件進程，可能超過保守讀超時。
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.IdleTimeout = 60 * time.Second

	admin := e.Group("/plugins")
	admin.Use(adminAuth)
	admin.Post("/load", m.handleLoad)
	admin.Post("/unload", m.handleUnload)
	admin.Post("/reload", m.handleReload)
	admin.Get("/list", m.handleList)
	admin.Get("/events", m.handleEvents)
	admin.Post("/metadata", m.handleMetadata)
	// 领域事件 SSE（Agent/tool 回合事件）与运行时日志 SSE：观察宿主 EventBus
	// 的领域事件（agent/status、agent/error、tools/execute、tools/result 等）与
	// 宿主/插件日志。用于自动化测试与排障时无障碍观察运行时状态。
	admin.Get("/domain-events", m.handleDomainEvents)
	admin.Get("/logs", m.handleLogs)

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
		return wrapErr("load", err)
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
		return wrapErr("unload", err)
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
		return wrapErr("unload", err)
	}

	// 再加載
	if err := m.LoadPlugin(entry); err != nil {
		return wrapErr("reload", err)
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

// marshalDomainEvent 把一次领域事件包封为 SSE 载荷 JSON {"name","data"}。
// 供 /domain-events handler 使用，也是其单元测试的序列化入口。
func marshalDomainEvent(ctx EventContext) ([]byte, error) {
	return json.Marshal(map[string]any{"name": ctx.Name, "data": ctx.Data})
}

// handleDomainEvents 以 SSE 流推送宿主 EventBus 的领域事件（agent/status、
// agent/error、tools/execute、tools/result 等）。订阅者连接后，每次 Emit 都会收到
// "event: domain" 帧，data 为 {"name": <事件名>, "data": <载荷>} 的 JSON。
// 观察者通过 EventBus.OnAny 注册：回调仅做非阻塞入队，绝不阻塞事件发射路径，
// 慢消费者溢出即丢弃（观察丢失不中断主机行为）。
func (m *Manager) handleDomainEvents(c *vodka.Context) error {
	c.Response.Header().Set("Content-Type", "text/event-stream")
	c.Response.Header().Set("Cache-Control", "no-cache")
	c.Response.Header().Set("Connection", "keep-alive")

	flusher, ok := c.Response.Writer.(http.Flusher)
	if !ok {
		return vodka.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
	}
	if _, err := fmt.Fprint(c.Response.Writer, ": connected\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

	ch := make(chan EventContext, 256)
	remove := m.events.OnAny(func(ctx EventContext) (any, error) {
		select {
		case ch <- ctx:
		default: // 慢消费者溢出即丢弃，不阻塞事件发射路径
		}
		return nil, nil
	})
	defer remove()

	for {
		select {
		case ev := <-ch:
			b, err := marshalDomainEvent(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Response.Writer, "event: domain\ndata: %s\n\n", b); err != nil {
				return nil
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return nil
		}
	}
}

// handleLogs 以 SSE 流推送宿主/插件运行时日志。宿主 INFO 日志与经 go-plugin
// 转发上来的插件子进程 stderr（如 notify 插件日志）汇聚到注入的 LogFanout，
// 此处订阅并转发。每行一条 "event: log" 帧，data 为原始日志行（已含时间戳/级别）。
func (m *Manager) handleLogs(c *vodka.Context) error {
	if m.logFanout == nil {
		return vodka.NewHTTPError(http.StatusServiceUnavailable, "log fanout not injected (enable with -log)")
	}
	c.Response.Header().Set("Content-Type", "text/event-stream")
	c.Response.Header().Set("Cache-Control", "no-cache")
	c.Response.Header().Set("Connection", "keep-alive")

	flusher, ok := c.Response.Writer.(http.Flusher)
	if !ok {
		return vodka.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
	}
	if _, err := fmt.Fprint(c.Response.Writer, ": connected\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

	id, ch := m.logFanout.Subscribe()
	defer m.logFanout.Unsubscribe(id)

	for {
		select {
		case line := <-ch:
			msg, err := json.Marshal(string(line))
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Response.Writer, "event: log\ndata: %s\n\n", msg); err != nil {
				return nil
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return nil
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
