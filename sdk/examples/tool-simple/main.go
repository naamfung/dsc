package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dsc-sdk/tool"
)

func main() {
	// 初始化插件元數據
	tool.InitMetadata("tool-file-op-example", "1.0.0", "1.0")

	// 註冊文件操作工具
	tool.RegisterTool(&tool.ToolDef{
		name:        "file_op",
		description: "File operations tool for reading and writing files.",
		schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"enum": ["read", "write"],
					"description": "The command to run. Allowed options are: read, write."
				},
				"path": {
					"type": "string",
					"description": "Absolute path to the file."
				},
				"content": {
					"type": "string",
					"description": "Content to write (required for write command)."
				}
			},
			"required": ["command", "path"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var req struct {
				Command string `json:"command"`
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &req); err != nil {
				return "", err
			}

			// 解析並轉換為絕對路徑
			absPath, err := filepath.Abs(req.Path)
			if err != nil {
				return "", err
			}

			switch req.Command {
			case "read":
				content, err := os.ReadFile(absPath)
				if err != nil {
					return "", err
				}
				return string(content), nil

			case "write":
				if req.Content == "" {
					return "", fmt.Errorf("content is required for write command")
				}
				// 確保目錄存在
				dir := filepath.Dir(absPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return "", err
				}
				if err := os.WriteFile(absPath, []byte(req.Content), 0644); err != nil {
					return "", err
				}
				return "File written successfully.", nil

			default:
				return "", fmt.Errorf("unsupported command: %s", req.Command)
			}
		},
	})

	// 啟動插件服務
	tool.Serve()
}
