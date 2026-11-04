package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// 本文件验「服务器主动通知 → 客户端自己刷新目录 → 叫回调」整条链。
//
// 为什么值得单独测：目录不是连上那一刻就固定了。服务器可以运行期加/减工具
// （notifications/tools/list_changed）、加资源、改提示词模板。不订阅这些通知，
// 新工具就永远看不见，除非用户手动重连——而用户并不知道自己需要重连。

// waitFor 轮询直到 cond 成立（或超时）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

func hasTool(c *Client, name string) bool {
	for _, tool := range c.Tools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// TestStdioToolListChanged 真实子进程：调用 add_tool 后服务器推 list_changed，
// 客户端必须自己重新拉 tools/list（不需要重连），并通知回调。
func TestStdioToolListChanged(t *testing.T) {
	c, err := NewStdioClient("helper", stdioHelperSpec(t), nil)
	if err != nil {
		t.Fatalf("new stdio client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	changes := make(chan CatalogChange, 8)
	c.SetCatalogChangeHandler(func(kind CatalogChange) { changes <- kind })

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	if hasTool(c, "late_tool") {
		t.Fatal("late_tool 不该一开始就存在")
	}
	if _, err := c.CallTool(ctx, "add_tool", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CallTool add_tool: %v", err)
	}

	select {
	case kind := <-changes:
		if kind != CatalogTools {
			t.Fatalf("回调类别 = %q, 期望 tools", kind)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("服务器推了 tools/list_changed，但客户端没有回调")
	}

	waitFor(t, "late_tool 进入工具表", func() bool { return hasTool(c, "late_tool") })
	if got := c.ToolCount(); got != 6 {
		t.Fatalf("工具数 = %d, 期望 6（新增一个）", got)
	}
}

// fakeNotifyServer 一台 streamable HTTP 假服务器：POST 处理请求，GET 专门推通知。
type fakeNotifyServer struct {
	mu        sync.Mutex
	tools     []string
	resources []string
	notify    chan string // 要推的通知方法名
}

func (f *fakeNotifyServer) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// 通知流：长连接，逐条把 notify 里的方法名写下来。
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("response writer is not a flusher")
				return
			}
			flusher.Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case method := <-f.notify:
					fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":%q}\n\n", method)
					flusher.Flush()
				}
			}
		}

		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var params map[string]any
		_ = json.Unmarshal(req.Params, &params)

		f.mu.Lock()
		defer f.mu.Unlock()
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			resp["result"] = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools":     map[string]any{"listChanged": true},
					"resources": map[string]any{"listChanged": true},
				},
			}
		case "tools/list":
			items := make([]any, 0, len(f.tools))
			for _, name := range f.tools {
				items = append(items, map[string]any{"name": name, "inputSchema": map[string]any{"type": "object"}})
			}
			resp["result"] = map[string]any{"tools": items}
		case "resources/list":
			items := make([]any, 0, len(f.resources))
			for _, uri := range f.resources {
				items = append(items, map[string]any{"uri": uri, "name": uri})
			}
			resp["result"] = map[string]any{"resources": items}
		case "tools/call":
			// 模拟「运行期新增工具/资源，然后通知」
			if name, _ := params["name"].(string); name == "grow" {
				f.tools = append(f.tools, "late_tool")
				f.resources = append(f.resources, "mem://late")
				select {
				case f.notify <- "notifications/tools/list_changed":
				default:
				}
				select {
				case f.notify <- "notifications/resources/list_changed":
				default:
				}
			}
			resp["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}
		default:
			resp["error"] = map[string]any{"code": -32601, "message": "not found"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

// TestHTTPNotificationsRefreshCatalog HTTP：GET 通知流上的 list_changed 同样要触发刷新。
func TestHTTPNotificationsRefreshCatalog(t *testing.T) {
	fake := &fakeNotifyServer{
		tools:     []string{"grow"},
		resources: []string{"mem://origin"},
		notify:    make(chan string, 8),
	}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	c := NewHTTPClient("notify", srv.URL+"/mcp", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var kinds []CatalogChange
	c.SetCatalogChangeHandler(func(kind CatalogChange) {
		mu.Lock()
		kinds = append(kinds, kind)
		mu.Unlock()
	})

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	if c.ToolCount() != 1 || c.ResourceCount() != 1 {
		t.Fatalf("初始目录 tools=%d resources=%d, 期望 1/1", c.ToolCount(), c.ResourceCount())
	}

	if _, err := c.CallTool(ctx, "grow", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CallTool grow: %v", err)
	}

	waitFor(t, "工具与资源都被刷新", func() bool {
		return c.ToolCount() == 2 && c.ResourceCount() == 2
	})
	waitFor(t, "两类通知都回调过", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(kinds) >= 2
	})
	mu.Lock()
	defer mu.Unlock()
	seen := map[CatalogChange]bool{}
	for _, k := range kinds {
		seen[k] = true
	}
	if !seen[CatalogTools] || !seen[CatalogResources] {
		t.Fatalf("回调类别 = %v, 期望含 tools 与 resources", kinds)
	}
}

// TestNotificationDoesNotBlockOnFullChannel 通知通道满了要丢，不能把发送方卡死。
func TestNotificationDoesNotBlockOnFullChannel(t *testing.T) {
	tr := &stdioTransport{notes: make(chan map[string]any, 1), logger: nopLogger{}}
	tr.notes <- map[string]any{"method": "first"}
	done := make(chan struct{})
	go func() {
		tr.publishNotification("notifications/tools/list_changed", map[string]any{"method": "x"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("通道满时 publishNotification 阻塞了")
	}
}

// TestDisconnectStopsNotificationLoop 断开后不再回调（避免已卸载的会话继续改目录）。
func TestDisconnectStopsNotificationLoop(t *testing.T) {
	tr := &fakeNotifyTransport{notes: make(chan map[string]any, 4)}
	c := NewClient("stopped", tr, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var called bool
	var mu sync.Mutex
	c.SetCatalogChangeHandler(func(CatalogChange) {
		mu.Lock()
		called = true
		mu.Unlock()
	})
	c.Disconnect()

	// 断开之后再推一条：不应再触发（通知 goroutine 已停）
	tr.notes <- map[string]any{"method": "notifications/tools/list_changed"}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Fatal("断开后仍触发了目录回调")
	}
}

// fakeNotifyTransport 一个最简单的 Transport，用固定结果满足 Connect，并暴露 notes 通道。
type fakeNotifyTransport struct {
	notes chan map[string]any
}

func (f *fakeNotifyTransport) RoundTrip(_ context.Context, req map[string]any) (map[string]any, error) {
	method, _ := req["method"].(string)
	if method == "notifications/initialized" {
		return map[string]any{}, nil
	}
	if method == "initialize" {
		return map[string]any{"result": map[string]any{
			"capabilities": map[string]any{"tools": map[string]any{}},
		}}, nil
	}
	key := "tools"
	if method == "resources/list" {
		key = "resources"
	}
	if method == "prompts/list" {
		key = "prompts"
	}
	return map[string]any{"result": map[string]any{key: []any{}}}, nil
}

func (f *fakeNotifyTransport) Reset(Logger) error                   { return nil }
func (f *fakeNotifyTransport) Close()                               {}
func (f *fakeNotifyTransport) Kind() string                         { return "fake" }
func (f *fakeNotifyTransport) Describe() string                     { return "fake" }
func (f *fakeNotifyTransport) Notifications() <-chan map[string]any { return f.notes }

var _ = strings.TrimSpace
