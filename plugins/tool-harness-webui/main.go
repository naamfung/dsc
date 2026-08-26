package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"dsc-sdk"
)

//go:embed all:webui/dist
var distFS embed.FS

// harnessData 探路版宿主接线状态：仅有 agent 名与 admin 可达性。
// 完整对话交互需宿主侧新增对主 agent RunStream/InjectMessage 的桥接 RPC，属下一阶段。
type harnessData struct {
	mu       sync.RWMutex
	agent    string
	adminURL string
	ok       bool
	lastErr  string
}

var hd = &harnessData{adminURL: defaultAdminURL()}

func defaultAdminURL() string {
	if a := os.Getenv("DSC_ADMIN_ADDR"); a != "" {
		return "http://127.0.0.1" + a
	}
	return "http://127.0.0.1:9999"
}
func webuiAddr() string {
	if a := os.Getenv("HARNESS_WEBUI_ADDR"); a != "" {
		return a
	}
	return ":8899"
}

// ---------- HTTP 服务（探路版） ----------

// health 探查宿主 admin 可达性与 agent 名。
func handleHealth(w http.ResponseWriter, r *http.Request) {
	hd.mu.RLock()
	type snap struct {
		Ok      bool   `json:"ok"`
		Admin   string `json:"admin"`
		Agent   string `json:"agent"`
		Message string `json:"message"`
	}
	s := snap{Admin: hd.adminURL, Agent: hd.agent}
	hd.mu.RUnlock()
	if s.Agent == "" {
		s.Agent = "agent-react-loop"
	}
	// 代理一次 admin 快照确认可达
	base := hd.adminURL
	code, body, err := proxy(base, "/debugger/agent", nil)
	s.Message = fmt.Sprintf("admin http %d, err=%v", code, err)
	s.Ok = err == nil && code == 200
	_ = body
	writeJSON(w, s)
}

// proxy 把请求代理转发到宿主 admin API，剥离前缀 /api。
// 复用 admin 的 DSC_ADMIN_TOKEN 认证（与宿主进程同一环境变量）。
func proxy(base, targetPath string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, base+targetPath, body)
	if err != nil {
		return 0, nil, err
	}
	if tok := os.Getenv("DSC_ADMIN_TOKEN"); tok != "" {
		req.Header.Set("X-Admin-Token", tok)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// readAdminTokenFrom 读取 admin token 认证头（与 admin.go adminAuth 对齐）。
func readAdminTokenFrom(r *http.Request) string {
	return r.Header.Get("X-Admin-Token")
}

// handleDebugger 代理 /debugger/agent（需 query name）。
func handleDebugger(w http.ResponseWriter, r *http.Request) {
	target := "/debugger/agent"
	if n := r.URL.Query().Get("name"); n != "" {
		target += "?name=" + n
	}
	code, body, err := proxy(hd.adminURL, target, nil)
	respondProxy(w, code, body, err)
}

// handlePlugins 代理 /plugins/list。
func handlePlugins(w http.ResponseWriter, r *http.Request) {
	code, body, err := proxy(hd.adminURL, "/plugins/list", nil)
	respondProxy(w, code, body, err)
}

// handleChat 发送/接收消息。探路版：未接入宿主对话桥接，返回占位说明。
func handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "invalid body"})
		return
	}
	writeJSON(w, map[string]any{
		"reply":  fmt.Sprintf("（探路占位）收到消息「%s」。对话桥接宿主将在下一阶段接入。", req.Content),
		"bridge": "pending",
	})
}

func respondProxy(w http.ResponseWriter, code int, body []byte, err error) {
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/debugger", handleDebugger)
	mux.HandleFunc("/api/plugins", handlePlugins)
	mux.HandleFunc("/api/chat", handleChat)
	// 静态资源：嵌入的前端 dist
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "favicon.ico" {
			p = "index.html"
		}
		// 防目录穿越
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		data, err := distFS.ReadFile(path.Join("webui/dist", p))
		if err != nil {
			// SPA 回退
			data, err = distFS.ReadFile("webui/dist/index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		ct := "text/html; charset=utf-8"
		switch {
		case strings.HasSuffix(p, ".js"):
			ct = "application/javascript"
		case strings.HasSuffix(p, ".css"):
			ct = "text/css"
		case strings.HasSuffix(p, ".svg"):
			ct = "image/svg+xml"
		}
		w.Header().Set("Content-Type", ct)
		w.Write(data)
	})
	return mux
}

// startHTTP 启动独立 HTTP 服务。
func startHTTP() error {
	srv := &http.Server{
		Addr:    webuiAddr(),
		Handler: newMux(),
	}
	fmt.Printf("[tool-harness-webui] http listening on %s (admin proxy -> %s)\n", webuiAddr(), hd.adminURL)
	return srv.ListenAndServe()
}

func main() {
	// 预先探测一次 admin（失败不致命，前端会显示）
	go func() {
		time.Sleep(500 * time.Millisecond)
		code, _, err := proxy(hd.adminURL, "/debugger/agent", nil)
		hd.mu.Lock()
		hd.ok = err == nil && code == 200
		if err != nil {
			hd.lastErr = err.Error()
		}
		hd.mu.Unlock()
	}()

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService / PluginMetadata
	// 与 go-plugin 组装。本插件为空壳工具（无业务工具，仅承载独立 HTTP 服务），
	// 故用 sdk.ToolProvider 返回空集即可满足 SDK 校验。
	sdk := dsc.New(dsc.Config{Name: "tool-harness-webui", Version: "0.1.0", Type: dsc.TypeTool})

	// 互通握手：宿主挂载聚合服务后回调，后台启动独立 HTTP 服务
	// （不阻塞 gRPC 握手；host ListTools 时端口已监听）。
	sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
		// 探知宿主 agent 名（来自环境注入，与 agent-react-loop 一致）
		hd.mu.Lock()
		hd.agent = os.Getenv("DSC_AGENT_NAME")
		if hd.agent == "" {
			hd.agent = "agent-react-loop"
		}
		hd.mu.Unlock()
		go func() {
			if err := startHTTP(); err != nil {
				hd.mu.Lock()
				hd.lastErr = fmt.Sprintf("http: %v", err)
				hd.mu.Unlock()
				fmt.Printf("[tool-harness-webui] http server failed: %v\n", err)
			}
		}()
		return nil
	})

	// 空壳工具插件：不提供业务工具（若宿主需要可后续注入 webui_open 工具）
	sdk.ToolProvider(func() []dsc.Tool { return nil })

	sdk.Serve()
}
