package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var WorkspaceRoot = "./workspace" // 可改為環境變數或配置

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）
func safePath(base, reqPath string) (string, error) {
	// 1. 清理並獲取絕對路徑
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absReq, err := filepath.Abs(filepath.Join(absBase, reqPath))
	if err != nil {
		return "", err
	}
	// 2. 解析符號鏈接（關鍵！）
	realReq, err := filepath.EvalSymlinks(absReq)
	if err != nil {
		return "", err
	}
	// 3. 嚴格前綴匹配
	if !strings.HasPrefix(realReq, absBase+string(os.PathSeparator)) && realReq != absBase {
		return "", os.ErrPermission
	}
	return realReq, nil
}

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

	// 使用安全路徑檢查
	reqPath, err := safePath(WorkspaceRoot, args.Path)
	if err != nil {
		return "", err
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

	// 使用安全路徑檢查
	reqPath, err := safePath(WorkspaceRoot, args.Path)
	if err != nil {
		return "", err
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
