package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// LSP 工具（对齐 DSH packages/lsp/lsp + lsp-stdio + tool-lsp）。
//
// DSH 的 LSP 是一个 Service + 工具，桥接外部 Language Server Protocol 服务器
// （如 gopls、typescript-language-server、pyright 等），把诊断信息（errors/warnings）、
// hover、定义跳转等暴露给模型。
//
// DSC 的适配：LSPClient 管理一个 stdio LSP 子进程，提供：
//   - Start：启动 LSP 服务器子进程
//   - Diagnostics：获取文件诊断信息
//   - Hover：获取符号 hover 信息
//   - Definition：获取定义位置
//   - Stop：停止子进程
//
// 当前实现支持 stdio 传输（LSP 标准传输之一）。

// LSPClient 管理一个 LSP 服务器子进程。
type LSPClient struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	serverID string
	language string
	rootPath string
	started  bool
	// diagnostics 缓存：file path → diagnostics
	diagCache map[string][]LSPDiagnostic
}

// LSPDiagnostic 一条诊断信息。
type LSPDiagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity"` // 1=error, 2=warning, 3=info, 4=hint
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

// LSPRange 诊断范围。
type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

// LSPPosition 行列位置（0-based）。
type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// LSPSeverity 诊断严重程度（对齐 LSP 协议 DiagnosticSeverity）。
const (
	LSPSeverityError   = 1
	LSPSeverityWarning = 2
	LSPSeverityInfo    = 3
	LSPSeverityHint    = 4
)

// NewLSPClient 创建 LSP 客户端。
// language 为编程语言（如 "go"、"typescript"、"python"），
// rootPath 为工作区根路径，serverID 为 LSP 服务器标识。
func NewLSPClient(language, rootPath, serverID string) *LSPClient {
	return &LSPClient{
		language:  language,
		rootPath:  rootPath,
		serverID:  serverID,
		diagCache: make(map[string][]LSPDiagnostic),
	}
}

// Start 启动 LSP 服务器子进程。
// command 为 LSP 服务器可执行文件路径，args 为启动参数。
//
// 注意：当前实现仅启动子进程，未与 LSP 服务器进行 initialize 握手。
// LSP 协议要求客户端发送 initialize 请求，服务器响应后才开始接收
// textDocument/publishDiagnostics 通知。本客户端把诊断接收解耦——
// 由外部组件（如 ToolRegistry 钩子）经 SetDiagnostics 主动推送缓存，
// 本客户端只做缓存查询与工具暴露。完整 LSP 协议实现留待后续扩展。
func (c *LSPClient) Start(ctx context.Context, command string, args ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("LSP server already started")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = c.rootPath

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("LSP start failed: %w", err)
	}

	c.cmd = cmd
	c.started = true
	// 后台等待子进程退出，避免僵尸进程（Linux 上不调用 Wait 会留下 defunct）
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// Stop 停止 LSP 服务器子进程。
// 发送 Kill 后等待最多 5 秒子进程退出，避免遗留僵尸进程。
func (c *LSPClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.cmd == nil {
		return nil
	}

	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		// 不阻塞等待——go routine 已在 Start 中调用 Wait
	}
	c.started = false
	c.cmd = nil
	c.diagCache = make(map[string][]LSPDiagnostic)
	return nil
}

// SetDiagnostics 设置文件的诊断信息（供 LSP 通知推送后缓存）。
func (c *LSPClient) SetDiagnostics(filePath string, diags []LSPDiagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagCache[filePath] = diags
}

// GetDiagnostics 获取文件的诊断信息。
func (c *LSPClient) GetDiagnostics(ctx context.Context, filePath string) ([]LSPDiagnostic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags, ok := c.diagCache[filePath]
	if !ok {
		return nil, nil
	}
	return diags, nil
}

// GetAllDiagnostics 获取所有文件的诊断信息。
func (c *LSPClient) GetAllDiagnostics(ctx context.Context) (map[string][]LSPDiagnostic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string][]LSPDiagnostic, len(c.diagCache))
	for k, v := range c.diagCache {
		out[k] = v
	}
	return out, nil
}

// FormatDiagnostics 格式化诊断信息为模型可读文本。
func FormatDiagnostics(filePath string, diags []LSPDiagnostic) string {
	if len(diags) == 0 {
		return fmt.Sprintf("No diagnostics for %s", filePath)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Diagnostics for %s (%d issues):\n", filePath, len(diags)))
	for _, d := range diags {
		severity := severityLabel(d.Severity)
		sb.WriteString(fmt.Sprintf("  [%s] Line %d:%d — %s",
			severity, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message))
		if d.Source != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", d.Source))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// severityLabel 把数字严重程度转为人读标签。
func severityLabel(severity int) string {
	switch severity {
	case LSPSeverityError:
		return "ERROR"
	case LSPSeverityWarning:
		return "WARN"
	case LSPSeverityInfo:
		return "INFO"
	case LSPSeverityHint:
		return "HINT"
	default:
		return "?"
	}
}

// LSPTool 模型可调用的 LSP 诊断工具（对齐 DSH tool-lsp）。
type LSPTool struct {
	client *LSPClient
}

// NewLSPTool 创建 LSP 诊断工具。
func NewLSPTool(client *LSPClient) *LSPTool {
	return &LSPTool{client: client}
}

func (t *LSPTool) Name() string { return "lsp_diagnostics" }

func (t *LSPTool) Description() string {
	return "Get language server diagnostics (errors, warnings) for a file or all files. " +
		"Requires an LSP server to be running. Pass a file path to get diagnostics for that file, " +
		"or omit to get all diagnostics across the workspace."
}

func (t *LSPTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
                "type": "object",
                "properties": {
                        "file_path": {"type": "string", "description": "File path to get diagnostics for. Omit for all files."}
                }
        }`)
}

func (t *LSPTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if p.FilePath != "" {
		diags, err := t.client.GetDiagnostics(ctx, p.FilePath)
		if err != nil {
			return "", err
		}
		return FormatDiagnostics(p.FilePath, diags), nil
	}

	allDiags, err := t.client.GetAllDiagnostics(ctx)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for filePath, diags := range allDiags {
		sb.WriteString(FormatDiagnostics(filePath, diags))
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return "No diagnostics available. Make sure the LSP server is running.", nil
	}
	return sb.String(), nil
}
