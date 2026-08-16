package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const workspaceRoot = "./workspace" // 可改為環境變數或配置

// ReadFileTool 讀取文件
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Returns the file content as a string."
}

func (t *ReadFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the file (relative to workspace root)" }
		},
		"required": ["path"]
	}`)
}

func (t *ReadFileTool) Execute(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	// 獲取安全工作目錄的絕對路徑
	base, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	// 構建請求路徑（相對 base）
	reqPath := filepath.Join(base, args.Path)
	// 檢查是否在 base 內（防止 .. 繞過）
	rel, err := filepath.Rel(base, reqPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", os.ErrPermission
	}

	content, err := os.ReadFile(reqPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFileTool 寫入文件
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Overwrites existing file."
}

func (t *WriteFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the file (relative to workspace root)" },
			"content": { "type": "string", "description": "Content to write" }
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteFileTool) Execute(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	base, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	reqPath := filepath.Join(base, args.Path)
	rel, err := filepath.Rel(base, reqPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", os.ErrPermission
	}

	dir := filepath.Dir(reqPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(reqPath, []byte(args.Content), 0644); err != nil {
		return "", err
	}
	return "File written successfully", nil
}
