package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// E2B 远程沙箱（对齐 DSH packages/e2b/e2b + fs-e2b + subprocess-e2b）。
//
// DSH 的 E2B 集成提供远程 Linux 沙箱——经 E2B API 创建沙箱实例，
// 在其中执行命令、读写文件，用于不可信代码的安全隔离执行。
//
// DSC 的适配：E2BSandbox 经 E2B REST API 管理远程沙箱实例，
// 提供 Execute（执行命令）/ ReadFile / WriteFile / Close 操作。
// 凭据经 DSC_E2B_API_KEY 环境变量注入。

// E2BSandbox E2B 远程沙箱客户端。
type E2BSandbox struct {
	mu        sync.Mutex
	apiKey    string
	baseURL   string
	client    *http.Client
	sandboxID string
}

// NewE2BSandbox 创建 E2B 沙箱客户端。
// apiKey 从 DSC_E2B_API_KEY 环境变量获取。
func NewE2BSandbox() *E2BSandbox {
	apiKey := osGetenv("DSC_E2B_API_KEY")
	return &E2BSandbox{
		apiKey:  apiKey,
		baseURL: "https://api.e2b.dev",
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// Create 创建新的沙箱实例。
func (e *E2BSandbox) Create(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.apiKey == "" {
		return fmt.Errorf("E2B: DSC_E2B_API_KEY not set")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/sandboxes", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("E2B create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("E2B create: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		SandboxID string `json:"sandboxID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("E2B create: parse response: %w", err)
	}

	e.sandboxID = result.SandboxID
	return nil
}

// Execute 在沙箱中执行命令。
func (e *E2BSandbox) Execute(ctx context.Context, command string) (string, error) {
	e.mu.Lock()
	sandboxID := e.sandboxID
	e.mu.Unlock()

	if sandboxID == "" {
		return "", fmt.Errorf("E2B: no sandbox created")
	}

	body := fmt.Sprintf(`{"cmd":"%s"}`, escapeJSON(command))
	req, err := http.NewRequestWithContext(ctx, "POST",
		e.baseURL+"/sandboxes/"+sandboxID+"/cmds", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("E2B execute: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("E2B execute: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output string `json:"stdout"`
		Error  string `json:"stderr"`
	}
	json.Unmarshal(respBody, &result)

	output := result.Output
	if result.Error != "" {
		output += "\n[stderr] " + result.Error
	}
	return output, nil
}

// ReadFile 从沙箱读取文件。
func (e *E2BSandbox) ReadFile(ctx context.Context, path string) (string, error) {
	e.mu.Lock()
	sandboxID := e.sandboxID
	e.mu.Unlock()

	if sandboxID == "" {
		return "", fmt.Errorf("E2B: no sandbox created")
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		e.baseURL+"/sandboxes/"+sandboxID+"/files?path="+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("E2B readFile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("E2B readFile: HTTP %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile 向沙箱写入文件。
func (e *E2BSandbox) WriteFile(ctx context.Context, path, content string) error {
	e.mu.Lock()
	sandboxID := e.sandboxID
	e.mu.Unlock()

	if sandboxID == "" {
		return fmt.Errorf("E2B: no sandbox created")
	}

	body := fmt.Sprintf(`{"path":"%s","content":"%s"}`, escapeJSON(path), escapeJSON(content))
	req, err := http.NewRequestWithContext(ctx, "POST",
		e.baseURL+"/sandboxes/"+sandboxID+"/files", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("E2B writeFile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("E2B writeFile: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Close 销毁沙箱实例。
func (e *E2BSandbox) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.sandboxID == "" {
		return nil
	}

	req, err := http.NewRequest("DELETE", e.baseURL+"/sandboxes/"+e.sandboxID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("E2B close: %w", err)
	}
	resp.Body.Close()

	e.sandboxID = ""
	return nil
}

// SandboxID 返回当前沙箱 ID。
func (e *E2BSandbox) SandboxID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sandboxID
}

// escapeJSON 转义 JSON 字符串中的特殊字符。
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// osGetenv 包装 os.Getenv（便于测试 mock）。
func osGetenv(key string) string {
	return os.Getenv(key)
}
