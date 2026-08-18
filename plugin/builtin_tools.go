package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var WorkspaceRoot = "./workspace" // 可改為環境變數或配置

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）
func safePath(base, reqPath string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", err
	}
	// 構建絕對路徑
	absReq, err := filepath.Abs(filepath.Join(realBase, reqPath))
	if err != nil {
		return "", err
	}
	// 嘗試解析，若失敗則檢查父目錄
	realReq, err := filepath.EvalSymlinks(absReq)
	if err != nil {
		// 解析失敗，可能文件不存在，則檢查父目錄
		parent := filepath.Dir(absReq)
		realParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(realParent, realBase+string(os.PathSeparator)) && realParent != realBase {
			return "", os.ErrPermission
		}
		// 父目錄安全，則返回 absReq（未解析的路徑，但已在安全目錄下）
		return absReq, nil
	}
	// 解析成功，檢查前綴
	if !strings.HasPrefix(realReq, realBase+string(os.PathSeparator)) && realReq != realBase {
		return "", os.ErrPermission
	}
	return realReq, nil
}

// StrReplaceEditorTool 文件編輯工具（str_replace_editor）
type StrReplaceEditorTool struct{}

func (t *StrReplaceEditorTool) Name() string { return "str_replace_editor" }

func (t *StrReplaceEditorTool) Description() string {
	return "Custom editor tool for viewing, creating, and editing files. Supports commands: view, create, str_replace, insert."
}

func (t *StrReplaceEditorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"enum": ["view", "create", "str_replace", "insert"],
				"description": "The commands to run. Allowed options are: view, create, str_replace, insert."
			},
			"path": {
				"type": "string",
				"description": "Absolute path to the file, e.g. /workspace/file.py."
			},
			"file_text": {
				"type": "string",
				"description": "Required for 'create' command. The content of the file to be created."
			},
			"old_str": {
				"type": "string",
				"description": "Required for 'str_replace' command. The string in the file to replace."
			},
			"new_str": {
				"type": "string",
				"description": "Required for 'str_replace' and 'insert' commands. The new string to replace with or insert."
			},
			"insert_line": {
				"type": "integer",
				"description": "Required for 'insert' command. The 1-based line number where the new_str should be inserted."
			}
		},
		"required": ["command", "path"]
	}`)
}

type strReplaceEditorArgs struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	FileText   string `json:"file_text"`
	OldStr     string `json:"old_str"`
	NewStr     string `json:"new_str"`
	InsertLine int    `json:"insert_line"`
}

func (t *StrReplaceEditorTool) Execute(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	var args strReplaceEditorArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	// 使用安全路徑檢查
	reqPath, err := safePath(WorkspaceRoot, args.Path)
	if err != nil {
		return "", err
	}

	switch args.Command {
	case "view":
		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		return string(content), nil

	case "create":
		if args.FileText == "" {
			return "", fmt.Errorf("file_text is required for create command")
		}
		dir := filepath.Dir(reqPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(reqPath, []byte(args.FileText), 0644); err != nil {
			return "", err
		}
		return "File created successfully.", nil

	case "str_replace":
		if args.OldStr == "" {
			return "", fmt.Errorf("old_str is required for str_replace command")
		}
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for str_replace command")
		}
		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)
		if !strings.Contains(contentStr, args.OldStr) {
			return "", fmt.Errorf("str_replace failed: old_str not found in file. File content:\n%s", contentStr)
		}
		newContentStr := strings.Replace(contentStr, args.OldStr, args.NewStr, 1)
		if err := os.WriteFile(reqPath, []byte(newContentStr), 0644); err != nil {
			return "", err
		}
		return "File replaced successfully.", nil

	case "insert":
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for insert command")
		}
		if args.InsertLine <= 0 {
			return "", fmt.Errorf("insert_line must be a positive integer for insert command")
		}
		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(content), "\n")
		// insert_line is 1-based
		// If insert_line is greater than len(lines), append to the end
		var newLines []string
		if args.InsertLine > len(lines) {
			newLines = append(lines, args.NewStr)
		} else {
			// Insert before the line at index args.InsertLine-1
			before := lines[:args.InsertLine-1]
			after := lines[args.InsertLine-1:]
			newLines = append(before, append([]string{args.NewStr}, after...)...)
		}
		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(reqPath, []byte(newContent), 0644); err != nil {
			return "", err
		}
		return "File inserted successfully.", nil

	default:
		return "", fmt.Errorf("unsupported command: %s", args.Command)
	}
}
