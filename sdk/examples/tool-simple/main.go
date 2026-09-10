// tool-simple 是最小的工具插件示例：注册一个文件操作工具并启动服务。
// 构建：go build -o mytool.exe .  → 把 mytool.exe 放进宿主 plugins/tool-mytool/。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dsc-sdk"
)

func main() {
	sdk := dsc.New(dsc.Config{
		Name:    "file-op",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
	})

	sdk.Tool(dsc.Tool{
		Name:        "file_op",
		Description: "File operations tool for reading and writing files.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "enum": ["read", "write"], "description": "The command to run."},
				"path": {"type": "string", "description": "Absolute path to the file."},
				"content": {"type": "string", "description": "Content to write (required for write)."}
			},
			"required": ["command", "path"]
		}`),
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var req struct {
				Command string `json:"command"`
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &req); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			switch req.Command {
			case "read":
				b, err := os.ReadFile(req.Path)
				if err != nil {
					return "", err
				}
				return string(b), nil
			case "write":
				abs, err := filepath.Abs(req.Path)
				if err != nil {
					return "", err
				}
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return "", err
				}
				if err := os.WriteFile(abs, []byte(req.Content), 0o644); err != nil {
					return "", err
				}
				return "File written successfully.", nil
			default:
				return "", fmt.Errorf("unsupported command: %s", req.Command)
			}
		},
	})

	sdk.Serve()
}
