package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Logger 最小的日志接口：*hclog.Logger 直接满足，SDK 因此不必依赖具体日志实现。
// 默认静默；调用方传自己的 logger 即可看到 stdio 的 stderr 与进程事件。
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Debug(string, ...any) {}
func (nopLogger) Info(string, ...any)  {}

// MCP 传输层：一次 JSON-RPC 往返背后的实际通道。
//
// 两种实现：
//   - httpTransport：streamable HTTP（MCP 规范 2025-03-26+，响应可为 JSON 或 SSE），
//     会话状态在 Mcp-Session-Id 头里；
//   - stdioTransport：把 MCP 服务器作为**子进程**跑，JSON-RPC 逐行写 stdin / 读 stdout，
//     会话状态就是那个进程本身（对齐 DSH mcp-client 的 StdioClientTransport）。
//
// 为什么要抽象这一层：MCPClient 的 initialize / tools-list / tools-call 与传输无关，
// 会话过期重连、工具注册、错误包装都在客户端；把「怎么把一行 JSON 送过去再取回一行」
// 隔开之后，两种传输各自的怪癖（HTTP 的 header 与 SSE、stdio 的管道与进程退出）不会
// 渗进协议逻辑。
type Transport interface {
	// RoundTrip 发送一个 JSON-RPC 消息并返回响应；无 id 的通知返回空 map。
	RoundTrip(ctx context.Context, req map[string]any) (map[string]any, error)
	// Reset 丢弃会话状态并准备下一次握手（HTTP 清会话 id；stdio 在子进程已退出时重启）。
	Reset(logger Logger) error
	// Close 释放通道（HTTP 无操作；stdio 关 stdin 并回收子进程）。
	Close()
	// Kind 传输类型标识：http / stdio。
	Kind() string
	// Describe 一行人类可读描述（状态行、日志、管理 API 用）。
	Describe() string
}

// Notifier 可选接口：传输层能收到服务器**主动**发来的通知（无 id 的 JSON-RPC）。
//
// 为什么放在传输层：通知走哪条通道完全取决于传输——stdio 是 stdout 上的另一行，
// streamable HTTP 是另一条 SSE 流（GET）。客户端只关心「有没有消息」，不关心它从哪来。
//
// 通道**永不关闭**：进程重启、HTTP 重握手都可能发生，订阅方的退出由它自己的 stop 决定
// （关闭会让发送方 panic 或让订阅方误以为永远结束）。满了就丢一条并记日志——通知是
// 提示而不是数据，绝不能让服务器因为客户端不读而阻塞在写上。
type Notifier interface {
	Notifications() <-chan map[string]any
}

// ============================================================
// streamable HTTP
// ============================================================

// ErrSessionExpired 表示服务端不再认识当前会话（streamable HTTP 的 404）。
// 调用方据此重新 initialize，而不是把「会话过期」当成工具调用失败。
var ErrSessionExpired = fmt.Errorf("mcp: session expired")

type httpTransport struct {
	endpoint   string
	httpClient *http.Client
	// streamClient 专供 GET 通知流：**不能带超时**——那是一条长连接，
	// 30s 的 Client.Timeout 会把它每半分钟掐断一次。
	streamClient *http.Client
	logger       Logger

	mu              sync.Mutex
	sessionID       string
	protocolVersion string
	notes           chan map[string]any
	streamStarted   bool
	streamCancel    context.CancelFunc
}

func newHTTPTransport(endpoint string) *httpTransport {
	return &httpTransport{
		endpoint:     endpoint,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		streamClient: &http.Client{},
		notes:        make(chan map[string]any, 16),
	}
}

func (t *httpTransport) Kind() string     { return "http" }
func (t *httpTransport) Describe() string { return t.endpoint }

// Notifications 实现 Notifier（服务器主动通知的通道，永不关闭）。
func (t *httpTransport) Notifications() <-chan map[string]any { return t.notes }

// Close 停掉通知流。HTTP 没有别的连接要关（POST 是短连接）。
func (t *httpTransport) Close() {
	t.mu.Lock()
	cancel := t.streamCancel
	t.streamCancel = nil
	t.streamStarted = false
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Reset 清掉会话：重连必须拿新会话，旧 id 在新连接上一定 404。
// 顺带停掉旧的通知流——它绑在旧会话上，留着只会一直 404 重试。
func (t *httpTransport) Reset(logger Logger) error {
	t.mu.Lock()
	t.sessionID = ""
	t.streamStarted = false
	cancel := t.streamCancel
	t.streamCancel = nil
	t.logger = logger
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// RoundTrip 发送一次 JSON-RPC 请求。
//
// HTTP 头设置兼容新旧两种 MCP HTTP 传输：
//   - 旧版 JSON-RPC over HTTP（Content-Type: application/json）
//   - 新版 streamable-http（MCP 规范 2025-03-26+，要求 Accept 同时含
//     application/json 和 text/event-stream）
//
// 新版服务器经 Accept 头检测客户端能力——缺 text/event-stream 会返回 HTTP 406
// "Not Acceptable"。设上两个类型后，旧版服务器忽略 Accept、新版服务器选择其中一种
// 响应（简单请求通常返回 application/json）。
func (t *httpTransport) RoundTrip(ctx context.Context, req map[string]any) (map[string]any, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	t.mu.Lock()
	sid, ver := t.sessionID, t.protocolVersion
	t.mu.Unlock()
	if sid != "" {
		httpReq.Header.Set("Mcp-Session-Id", sid)
	}
	if ver != "" {
		httpReq.Header.Set("MCP-Protocol-Version", ver)
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 服务端可能在任意响应上重新下发会话标识（initialize 一定有），有就更新。
	if newSID := resp.Header.Get("Mcp-Session-Id"); newSID != "" && newSID != sid {
		t.mu.Lock()
		t.sessionID = newSID
		t.mu.Unlock()
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch {
	case resp.StatusCode == http.StatusOK:
	// 正常响应，按内容类型解析（JSON 或 SSE）
	case resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent:
		// 通知（如 notifications/initialized）按规范回 202/204 且无正文
		return map[string]any{}, nil
	case resp.StatusCode == http.StatusNotFound && sid != "":
		// 会话已失效：交给调用方重新 initialize
		return nil, ErrSessionExpired
	default:
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// streamable-http 服务器可能以 text/event-stream（SSE）响应：
	// 每行 "data: <json>\n\n" 格式。
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/event-stream") {
		return ParseSSEResponse(respBody)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return result, nil
}

// setProtocolVersion 记住 initialize 协商出的版本（后续请求要带）。
func (t *httpTransport) setProtocolVersion(v string) {
	if v == "" {
		return
	}
	t.mu.Lock()
	t.protocolVersion = v
	t.mu.Unlock()
}

// beginNotifications 打开 GET SSE 流收服务器的主动通知（如 tools/list_changed）。
//
// 规范里这是「客户端 MAY」，实测多数实现（官方 everything / 各类 SDK 服务器）都支持：
// 打开一条 GET、服务器把通知逐条推下来。不支持的最多发一次 GET 拿到 404/405 就收手。
//
// 不开这条流也不是不能用——但只要服务器支持，开着就能让「运行期新增的工具」立刻可见，
// 而不是等下次重连。
func (t *httpTransport) beginNotifications(logger Logger) {
	t.mu.Lock()
	// 会话 id 不是必需的（无状态服务器根本不发），有就带上。
	if t.streamStarted {
		t.mu.Unlock()
		return
	}
	t.streamStarted = true
	if logger == nil {
		logger = nopLogger{}
	}
	t.logger = logger
	sid, ver := t.sessionID, t.protocolVersion
	ctx, cancel := context.WithCancel(context.Background())
	t.streamCancel = cancel
	t.mu.Unlock()

	go t.readNotificationStream(ctx, sid, ver, logger)
}

func (t *httpTransport) readNotificationStream(ctx context.Context, sid, ver string, logger Logger) {
	backoff := time.Second
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", t.endpoint, nil)
		if err != nil {
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
		}
		if ver != "" {
			req.Header.Set("MCP-Protocol-Version", ver)
		}

		resp, err := t.streamClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Debug("mcp notification stream request failed",
				"endpoint", t.endpoint, "error", err.Error())
			if !t.sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// 服务器不支持 GET 流：再试也是一样的结果，直接收手（别每 30 秒打一次日志）。
		if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			logger.Debug("mcp notification stream unsupported by server; continuing without it",
				"endpoint", t.endpoint, "status", resp.StatusCode)
			return
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			logger.Debug("mcp notification stream rejected; will retry",
				"endpoint", t.endpoint, "status", resp.StatusCode)
			if !t.sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
			// 返回了 JSON 之类：不是通知流，别把它当流读。
			resp.Body.Close()
			logger.Debug("mcp notification stream not an event stream; giving up", "content_type", ct)
			return
		}

		logger.Debug("mcp notification stream open", "endpoint", t.endpoint)
		backoff = time.Second
		t.consumeNotificationStream(ctx, resp.Body, logger)
		resp.Body.Close()
		if ctx.Err() != nil {
			return
		}
		// 流被服务器关掉（重启/超时）：退避后重连
		if !t.sleepOrDone(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// consumeNotificationStream 逐行读 SSE，把 data 里的 JSON 通知投进 notes。
//
// 只关心「无 id 且有 method」的消息：流上也可能混进响应（少数服务器这么做），
// 那类带 id 的应走 POST 通道，这里丢掉。
func (t *httpTransport) consumeNotificationStream(ctx context.Context, body io.Reader, logger Logger) {
	br := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if s := strings.TrimSpace(string(line)); s != "" && strings.HasPrefix(s, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
			var msg map[string]any
			if json.Unmarshal([]byte(payload), &msg) != nil {
				continue
			}
			if _, isRequest := msg["method"]; !isRequest {
				continue
			}
			if _, hasID := msg["id"]; hasID {
				continue
			}
			select {
			case t.notes <- msg:
			default:
				logger.Debug("mcp notification channel full; dropping one", "endpoint", t.endpoint)
			}
		}
		if err != nil {
			if ctx.Err() == nil && err != io.EOF {
				logger.Debug("mcp notification stream read error", "endpoint", t.endpoint, "error", err.Error())
			}
			return
		}
	}
}

// sleepOrDone 退避等待；ctx 取消时立刻返回 false（让重连循环退出）。
func (t *httpTransport) sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// ParseSSEResponse 从 SSE 响应体提取首个 data: 行的 JSON 负载。
// MCP streamable-http 传输的 SSE 格式：每行 "data: <json>\n\n"，
// 客户端取首个事件 data 字段解析为 JSON-RPC 响应。
func ParseSSEResponse(body []byte) (map[string]any, error) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData := strings.TrimPrefix(line, "data: ")
			var result map[string]any
			if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
				return nil, fmt.Errorf("invalid SSE data JSON: %w", err)
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("no data: line found in SSE response")
}

// ============================================================
// stdio（子进程）
// ============================================================

// StdioSpec 是启动一个 stdio MCP 服务器所需的全部信息。
//
// Command/Args 是命令行，Env 只会追加到**白名单基线**上（见 stdioEnv）——
// MCP 服务器是第三方代码，不能把宿主的整份环境（含各家 API key）灌给它。
type StdioSpec struct {
	Command string
	Args    []string
	Env     map[string]string
	CWD     string
}

// stdioShutdownGrace 关 stdin 之后等子进程自行退出的时间，超时才 Kill。
const stdioShutdownGrace = 3 * time.Second

// stdioBaseEnv 传给 MCP 子进程的环境变量白名单（大小写不敏感匹配）。
//
// 只给「能启动进程、能找到自己的运行时」所必需的那些：PATH 及其 Windows 扩展、
// 系统根、家目录、临时目录。**不给**任何 DSC_* / *_API_KEY —— 需要密钥的服务器
// 应由用户在配置里显式写进 env（那是明确的授权动作，不是隐式继承）。
var stdioBaseEnv = []string{
	"PATH", "PATHEXT", "SystemRoot", "SYSTEMDRIVE", "COMSPEC", "WINDIR",
	"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA",
	"TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "TZ",
	"SHELL", "TERM", "NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE",
	"PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMW6432",
	"NODE_PATH", "NODE_OPTIONS", "PYTHONPATH", "PYTHONIOENCODING",
	"JAVA_HOME", "GOROOT", "GOPATH", "CARGO_HOME", "RUSTUP_HOME",
}

// stdioEnv 组装子进程环境：白名单基线 + 调用方显式给的 env（后者覆盖前者）。
//
// 排序输出：同一份配置在不同机器上环境块字节一致，便于比对与排查（也避免
// Windows 上「环境块顺序不同 → 进程行为不同」这种难查的差异）。
func stdioEnv(extra map[string]string) []string {
	env := map[string]string{}
	for _, key := range stdioBaseEnv {
		if v, ok := os.LookupEnv(key); ok {
			env[strings.ToUpper(key)] = key + "=" + v
		}
	}
	for k, v := range extra {
		if k == "" {
			continue
		}
		env[strings.ToUpper(k)] = k + "=" + v
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		out = append(out, kv)
	}
	sort.Strings(out)
	return out
}

type stdioTransport struct {
	spec   StdioSpec
	logger Logger

	// mu 串行化往返，并保护下面这些字段的快照（stdin/lines/done/cmd）。
	// **进程退出路径不取它**：等待 goroutine 若为拿锁而阻塞，会让正在等待响应的
	// RoundTrip 永远等不到 done 关闭（自锁）。退出信息走 exitMu。
	mu    sync.Mutex
	stdin io.WriteCloser
	lines chan []byte
	done  chan struct{}
	cmd   *exec.Cmd

	// notes 服务器主动通知（无 id）的出口，实现 Notifier。**只建一次、永不关闭**：
	// Reset 会换进程但不会换它，订阅方因此不必重新订阅。
	notes chan map[string]any

	// pending 待响应的请求：id → 交付通道，由分发 goroutine 投递。
	//
	// 以前是「RoundTrip 自己读 stdout 等 id」——只在有请求在飞时才读，闲时的通知
	// （例如运行期新增工具）根本没人收，而且读缓冲塞满会把服务器的写操作堵死。
	pendingMu sync.Mutex
	pending   map[string]chan map[string]any

	exitMu  sync.Mutex
	closed  bool
	doneErr error
}

func newStdioTransport(spec StdioSpec, logger Logger) (*stdioTransport, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("stdio MCP server needs a command")
	}
	if logger == nil {
		logger = nopLogger{}
	}
	t := &stdioTransport{
		spec:    spec,
		logger:  logger,
		lines:   make(chan []byte, 32),
		done:    make(chan struct{}),
		notes:   make(chan map[string]any, 16),
		pending: map[string]chan map[string]any{},
	}
	if err := t.start(); err != nil {
		return nil, err
	}
	return t, nil
}

// Notifications 实现 Notifier。
func (t *stdioTransport) Notifications() <-chan map[string]any { return t.notes }

// start 拉起子进程并接上三条管道。
//
// stderr 必须持续消费：管道写满会把服务器卡死在 write 上（看起来像「工具调用无响应」）。
// 这里逐行转进宿主日志，前缀带服务器名，便于对上是谁在报错。
func (t *stdioTransport) start() error {
	cmd := exec.Command(t.spec.Command, t.spec.Args...)
	// Windows：隐藏子进程控制台窗口（否则每拉一个 MCP 服务器都弹一个黑框）。
	// 本包自带实现（proc_windows.go），不依赖 SDK 根——否则 core 反向依赖会成环。
	configureChildProcessAttrs(cmd)
	cmd.Dir = t.spec.CWD
	cmd.Env = stdioEnv(t.spec.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdio MCP server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdio MCP server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stdio MCP server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start stdio MCP server %q: %w", t.describeCmd(), err)
	}

	lines, done := t.lines, t.done
	t.mu.Lock()
	t.cmd, t.stdin = cmd, stdin
	t.mu.Unlock()

	t.logger.Info("mcp stdio server started", "command", t.describeCmd(), "pid", cmd.Process.Pid)

	go t.readStdout(stdout, lines, done)
	go t.drainStderr(stderr)
	go t.dispatch(lines, done)
	go func() {
		err := cmd.Wait()
		t.exitMu.Lock()
		if !t.closed {
			// 进程自己退出（崩溃 / 主动结束）：让等待中的往返立刻失败，而不是挂住
			t.doneErr = fmt.Errorf("stdio MCP server %q exited: %v", t.describeCmd(), err)
		}
		t.exitMu.Unlock()
		close(done)
	}()
	return nil
}

// readStdout 逐行读响应。用 ReadBytes 而不是 bufio.Scanner：工具结果可能上兆，
// Scanner 的 token 上限会把它截断成「invalid JSON」。
func (t *stdioTransport) readStdout(r io.Reader, lines chan []byte, done chan struct{}) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			select {
			case lines <- line:
			case <-done:
				return
			}
		}
		if err != nil {
			return // EOF 或读错误：由 wait goroutine 统一收口
		}
	}
}

func (t *stdioTransport) drainStderr(r io.Reader) {
	br := bufio.NewReaderSize(r, 16*1024)
	for {
		line, err := br.ReadBytes('\n')
		if s := strings.TrimSpace(string(line)); s != "" {
			t.logger.Debug("mcp stdio stderr", "server", t.spec.Command, "line", s)
		}
		if err != nil {
			return
		}
	}
}

// dispatch 常驻读 stdout 分发行：响应交给等待方，通知投进 notes，服务器发来的请求回 -32601。
//
// 之前这段逻辑住在 RoundTrip 里（只在有请求在飞时才读），两个后果：闲时的通知没人收；
// 读通道塞满后 readStdout 阻塞，服务器写 stdout 就被堵住（表现为「莫名其妙卡住」）。
func (t *stdioTransport) dispatch(lines chan []byte, done chan struct{}) {
	for {
		select {
		case <-done:
			t.failPending()
			return
		case line, ok := <-lines:
			if !ok {
				t.failPending()
				return
			}
			var msg map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(line), &msg); err != nil {
				t.logger.Debug("mcp stdio non-JSON line", "line", string(line))
				continue
			}
			if method, isRequest := msg["method"].(string); isRequest {
				if id, hasID := msg["id"]; hasID && id != nil {
					// 服务器→客户端请求：我们不声明 sampling/roots 能力，明确回 -32601。
					t.replyMethodNotFound(msg)
					continue
				}
				t.publishNotification(method, msg)
				continue
			}
			// 响应：交给等它的那一方；没人等（迟到响应）就丢弃。
			if id, hasID := msg["id"]; hasID {
				t.deliver(id, msg)
			}
		}
	}
}

func (t *stdioTransport) publishNotification(method string, msg map[string]any) {
	select {
	case t.notes <- msg:
	default:
		t.logger.Debug("mcp notification channel full; dropping one", "method", method, "server", t.spec.Command)
	}
}

func (t *stdioTransport) deliver(id any, msg map[string]any) {
	key := fmt.Sprintf("%v", id)
	t.pendingMu.Lock()
	ch, ok := t.pending[key]
	if ok {
		delete(t.pending, key)
	}
	t.pendingMu.Unlock()
	if !ok {
		return // 迟到的响应
	}
	select {
	case ch <- msg:
	default:
	}
}

// failPending 进程退出时唤醒所有等待方（投 nil，让 RoundTrip 返回退出原因）。
func (t *stdioTransport) failPending() {
	t.pendingMu.Lock()
	pending := t.pending
	t.pending = map[string]chan map[string]any{}
	t.pendingMu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- nil:
		default:
		}
	}
}

func (t *stdioTransport) Kind() string { return "stdio" }

func (t *stdioTransport) Describe() string { return "stdio: " + t.describeCmd() }

func (t *stdioTransport) describeCmd() string {
	parts := append([]string{t.spec.Command}, t.spec.Args...)
	return strings.Join(parts, " ")
}

// Reset：stdio 没有「会话」，只有「进程」。进程还活着就什么都不做；已经退出就重启
// 一个（对齐 HTTP 侧 404 重连的语义：下一次调用前恢复可用状态）。
func (t *stdioTransport) Reset(_ Logger) error {
	select {
	case <-t.done:
	default:
		return nil // 还活着
	}
	t.mu.Lock()
	t.lines = make(chan []byte, 32)
	t.done = make(chan struct{})
	t.stdin = nil
	t.cmd = nil
	t.mu.Unlock()
	t.exitMu.Lock()
	t.closed = false
	t.doneErr = nil
	t.exitMu.Unlock()
	return t.start()
}

// Close 关 stdin 让服务器正常退出，超时才强杀。
func (t *stdioTransport) Close() {
	t.exitMu.Lock()
	if t.closed {
		t.exitMu.Unlock()
		return
	}
	t.closed = true
	t.exitMu.Unlock()

	t.mu.Lock()
	stdin, cmd, done := t.stdin, t.cmd, t.done
	t.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close() // 多数 MCP 服务器见到 stdin EOF 就退出
	}
	if cmd == nil || cmd.Process == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(stdioShutdownGrace):
		_ = cmd.Process.Kill()
		<-done
	}
}

// RoundTrip 写一行 JSON-RPC，等分发 goroutine 把匹配 id 的响应交回来。
//
// 读循环已经搬到 dispatch（常驻），这里只负责登记等待、写请求、等结果。三条退出路径：
// ctx 取消、进程退出（done）、拿到响应。
func (t *stdioTransport) RoundTrip(ctx context.Context, req map[string]any) (map[string]any, error) {
	t.mu.Lock()
	stdin, done := t.stdin, t.done
	t.mu.Unlock()

	select {
	case <-done:
		return nil, t.exitErr()
	default:
	}
	if stdin == nil {
		return nil, fmt.Errorf("stdio MCP server is not running")
	}

	// 通知没有 id，服务器不会回应（MCP 规定 notifications/initialized 无响应）。
	wantID, hasID := req["id"]
	if !hasID {
		payload, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		if _, err := stdin.Write(append(payload, '\n')); err != nil {
			return nil, fmt.Errorf("write to stdio MCP server: %w", err)
		}
		return map[string]any{}, nil
	}

	// 先登记再写：响应可能比我们的登记更快到（本地进程管道），先写后登记就是竞态。
	key := fmt.Sprintf("%v", wantID)
	ch := make(chan map[string]any, 1)
	t.pendingMu.Lock()
	t.pending[key] = ch
	t.pendingMu.Unlock()
	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, key)
		t.pendingMu.Unlock()
	}()

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := stdin.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("write to stdio MCP server: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return nil, t.exitErr()
	case msg := <-ch:
		if msg == nil {
			return nil, t.exitErr()
		}
		return msg, nil
	}
}

// exitErr 取进程退出原因（不取 t.mu，避免与 RoundTrip 互锁）。
func (t *stdioTransport) exitErr() error {
	t.exitMu.Lock()
	defer t.exitMu.Unlock()
	if t.doneErr != nil {
		return t.doneErr
	}
	return fmt.Errorf("stdio MCP server %q is not running", t.describeCmd())
}

// replyMethodNotFound 回应服务器发起的请求：我们不让服务器反向调用客户端能力
// （sampling / roots / elicitation 都没声明），明确回 -32601 比静默挂起好。
func (t *stdioTransport) replyMethodNotFound(msg map[string]any) {
	id, _ := msg["id"]
	method, _ := msg["method"].(string)
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32601,
			"message": "method not supported by dsc client: " + method,
		},
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	t.mu.Lock()
	stdin := t.stdin
	t.mu.Unlock()
	if stdin == nil {
		return
	}
	_, _ = stdin.Write(append(payload, '\n'))
}
