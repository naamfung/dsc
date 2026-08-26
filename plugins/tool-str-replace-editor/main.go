package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"dsc-sdk"
	"dsc/plugin"
	"dsc/proto"
	"github.com/aymanbagabas/go-udiff"
)

// editorState 维护编辑工具的观察状态（view 过的文件内容与版本），
// 供 str_replace/insert 校验文件未在观察后被外部修改（重写自旧的
// ToolServiceServer.observations，SDK 的 Tool.Handler 无 server 引用，
// 故把状态抽成独立结构并由 handler 闭包捕获）。
type editorState struct {
	observations map[string]*proto.FsObservation
	mu           sync.RWMutex
}

func (s *editorState) getObservation(filePath string) (*proto.FsObservation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obs, found := s.observations[filePath]
	return obs, found
}

func (s *editorState) updateObservation(filePath, state, version, lastContent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observations == nil {
		s.observations = make(map[string]*proto.FsObservation)
	}
	s.observations[filePath] = &proto.FsObservation{
		State:       state,
		Version:     version,
		LastContent: lastContent,
	}
}

// withinBase 判斷 real 路徑是否在 base 目錄（含 base 自身）之內
func withinBase(real, realBase string) bool {
	return strings.HasPrefix(real, realBase+string(os.PathSeparator)) || real == realBase
}

// isAbsPath 檢查路徑是否為絕對路徑（支持 Windows 盤符絕對路徑如 C:\ 或 C:/，以及 Unix 絕對路徑如 /xxx）
func isAbsPath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	// 檢查是否為 Windows 盤符絕對路徑（如 C:/xxx 或 C:\xxx）
	if len(path) >= 2 && path[1] == ':' {
		c := path[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	// 檢查是否為 Unix 絕對路徑或 Windows 無盤符根路徑（以 / 或 \ 開頭）
	if len(path) >= 1 && (path[0] == '/' || path[0] == '\\') {
		return true
	}
	return false
}

// makeAbsPath 將路徑轉換為絕對路徑，正確處理 Unix 絕對路徑和 Windows 盤符絕對路徑
func makeAbsPath(reqPath string) (string, error) {
	// 先使用 FromSlash 轉換斜槓，將 / 轉換為 \（在 Windows 上）
	cleanReq := filepath.FromSlash(reqPath)

	// 檢查是否為 Windows 盤符絕對路徑 (如 C:\ 或 C:/)
	if len(cleanReq) >= 2 && cleanReq[1] == ':' {
		c := cleanReq[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return filepath.Abs(cleanReq)
		}
	}

	// 檢查是否為 Unix 絕對路徑或 Windows 無盤符根路徑 (以 / 或 \ 開頭)
	// 在 Windows 上，以 \ 開頭的路徑被視為相對於當前驅動器的根目錄
	// 直接使用 filepath.Abs 即可正確處理這種情況（會映射為如 D:\Agents\novelforge\main.go）
	return filepath.Abs(cleanReq)
}

// resolveExistingAncestor 從 dir 開始向上逐級查找第一個存在的路徑並解析符號鏈接；
// 返回其真實路徑。所有層級都不存在時返回 false。
func resolveExistingAncestor(dir string) (string, bool) {
	for {
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return real, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）。
// base 若不存在會先創建；目標路徑或其父目錄不存在時也能正常解析（中間目錄可由調用方自行創建）。
func safePath(base, reqPath string) (string, error) {
	// 如果 reqPath 已經是絕對路徑（包括 Windows 盤符絕對路徑如 C:\ 或 C:/，以及 Unix 絕對路徑如 /xxx），則直接基於它進行安全校驗，絕不與 base 拼接
	if isAbsPath(reqPath) {
		absReq, err := makeAbsPath(reqPath)
		if err != nil {
			return "", err
		}
		realReq, err := filepath.EvalSymlinks(absReq)
		if err != nil {
			// 解析失敗，可能文件不存在，則檢查父目錄
			parent := filepath.Dir(absReq)
			_, symlinksErr := filepath.EvalSymlinks(parent)
			if symlinksErr != nil {
				return "", symlinksErr
			}
			// 對於絕對路徑，返回解析後的絕對路徑（不強制要求它在 base 下）
			return absReq, nil
		}
		return realReq, nil
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	// 工作目錄（base）可能尚未創建（首次使用時），先確保它存在，
	// 否則 EvalSymlinks 會報 “The system cannot find the file specified”
	if err := os.MkdirAll(absBase, 0755); err != nil {
		return "", err
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", err
	}

	// 構建絕對路徑並清理（去除 . .. 等）
	absReq, err := filepath.Abs(filepath.Join(realBase, reqPath))
	if err != nil {
		return "", err
	}
	absReq = filepath.Clean(absReq)

	// 詞法前綴檢查：確保在 base 目錄內（对齐 DSH：工具层不做独立策略开关，
	// 相对路径永远以 workspace 为根，防止 ../ 路径穿越；是否允许绝对路径写
	// workspace 之外由宿主 pipeline 的 sandbox 策略统一判定）。
	if !withinBase(absReq, realBase) {
		return "", os.ErrPermission
	}

	// 目標本身若已存在，解析其真實路徑並檢查仍在 base 內（防符號鏈接逃逸）
	if _, statErr := os.Lstat(absReq); statErr == nil {
		realReq, err := filepath.EvalSymlinks(absReq)
		if err != nil {
			return "", err
		}
		if !withinBase(realReq, realBase) {
			return "", os.ErrPermission
		}
		return realReq, nil
	}

	// 目標不存在：解析其最深的已存在祖先，確保真實路徑仍在 base 內
	realAncestor, ok := resolveExistingAncestor(filepath.Dir(absReq))
	if !ok {
		return "", os.ErrPermission
	}
	if !withinBase(realAncestor, realBase) {
		return "", os.ErrPermission
	}
	return absReq, nil
}

// normalizeWorkspacePath 剝離模型按工具描述傳入的 /workspace 前綴，
// 使 /workspace/test/fib.go 映射到 workspace 根目錄下的 test/fib.go
func normalizeWorkspacePath(p string) string {
	p = strings.TrimSpace(p)
	for _, prefix := range []string{"/workspace", `\workspace`} {
		if strings.HasPrefix(p, prefix) {
			return strings.TrimLeft(strings.TrimPrefix(p, prefix), `/\`)
		}
	}
	return p
}

type strReplaceEditorArgs struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	FileText   string `json:"file_text"`
	OldStr     string `json:"old_str"`
	NewStr     string `json:"new_str"`
	InsertLine int    `json:"insert_line"`
}

// computeHash 計算字符串的 sha256 hash
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func strReplaceEditorHandler(ctx context.Context, state *editorState, argsJSON json.RawMessage) (string, error) {
	var args strReplaceEditorArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	// 使用安全路徑檢查；workspace 根統一來自 plugin.WorkspaceRoot
	// （宿主按 config workspace_root 解析並經 DSC_WORKSPACE_ROOT 注入，對齊 DSH 單一策略歸屬）
	workspaceRoot := plugin.WorkspaceRoot

	// 模型按工具描述會傳入形如 /workspace/test/fib.go 的絕對路徑，
	// 需剝離 /workspace 前綴後再與 workspaceRoot 拼接，避免變成 workspace/workspace/...
	reqPath, err := safePath(workspaceRoot, normalizeWorkspacePath(args.Path))
	if err != nil {
		return "", err
	}
	// diff 标签用相对 workspace 的路径（对齐 REX 的 a/path b/path），
	// ToSlash 统一为正斜杠，避免 Windows 绝对路径标签含反斜杠
	relPath := filepath.ToSlash(normalizeWorkspacePath(args.Path))

	switch args.Command {
	case "view":
		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)
		version := computeHash(contentStr)
		// 更新觀測狀態
		state.updateObservation(reqPath, "present", version, contentStr)
		return contentStr, nil

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
		version := computeHash(args.FileText)
		// 更新觀測狀態
		state.updateObservation(reqPath, "present", version, args.FileText)
		return appendDiff("File created successfully.", relPath, "", args.FileText), nil

	case "str_replace":
		if args.OldStr == "" {
			return "", fmt.Errorf("old_str is required for str_replace command")
		}
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for str_replace command")
		}

		// 檢查觀測狀態
		obs, found := state.getObservation(reqPath)
		if !found || obs.State == "unseen" || obs.State == "absent" {
			return "", fmt.Errorf("str_replace failed: file has not been observed (viewed). Please use 'view' command first.")
		}

		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)

		// 驗證版本/內容是否匹配
		if obs.LastContent != "" && obs.LastContent != contentStr {
			return "", fmt.Errorf("str_replace failed: file content has changed since last observation. Please use 'view' to get the latest content.")
		}

		// 与 DSH 语义对齐：old_str 确实未匹配时，不返回完整文件内容，
		// 仅提示 old_str 未在文件中找到（避免大文件错误信息导致 TUI 花屏）
		if !strings.Contains(contentStr, args.OldStr) {
			return "", fmt.Errorf("str_replace failed: No replacement was performed, old_str %q did not appear verbatim in %s", args.OldStr, relPath)
		}
		newContentStr := strings.Replace(contentStr, args.OldStr, args.NewStr, 1)
		if err := os.WriteFile(reqPath, []byte(newContentStr), 0644); err != nil {
			return "", err
		}

		// 更新觀測狀態
		newVersion := computeHash(newContentStr)
		state.updateObservation(reqPath, "present", newVersion, newContentStr)

		return appendDiff("File replaced successfully.", relPath, contentStr, newContentStr), nil

	case "insert":
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for insert command")
		}
		if args.InsertLine <= 0 {
			return "", fmt.Errorf("insert_line must be a positive integer for insert command")
		}

		// 檢查觀測狀態
		obs, found := state.getObservation(reqPath)
		if !found || obs.State == "unseen" || obs.State == "absent" {
			return "", fmt.Errorf("insert failed: file has not been observed (viewed). Please use 'view' command first.")
		}

		content, err := os.ReadFile(reqPath)
		if err != nil {
			return "", err
		}
		contentStr := string(content)

		// 驗證版本/內容是否匹配
		if obs.LastContent != "" && obs.LastContent != contentStr {
			return "", fmt.Errorf("insert failed: file content has changed since last observation. Please use 'view' to get the latest content.")
		}

		lines := strings.Split(contentStr, "\n")
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

		// 更新觀測狀態
		newVersion := computeHash(newContent)
		state.updateObservation(reqPath, "present", newVersion, newContent)

		return appendDiff("File inserted successfully.", relPath, contentStr, newContent), nil

	default:
		return "", fmt.Errorf("unsupported command: %s", args.Command)
	}
}

// appendDiff 生成 old→new 的 unified diff（带 a/ b/ 文件头，对齐 REX 的 writer 预览）
// 并附加到写操作结果文本；无变化时原样返回。TUI 侧识别 diff 块并彩色渲染。
func appendDiff(msg, path, oldContent, newContent string) string {
	if oldContent == newContent {
		return msg
	}
	diff := udiff.Unified("a/"+path, "b/"+path, oldContent, newContent)
	return msg + "\n\n" + diff
}

func main() {
	// 观察状态由独立结构承载（SDK 的 Tool.Handler 无 server 引用，闭包捕获 state）
	state := &editorState{observations: make(map[string]*proto.FsObservation)}

	// 定义 str_replace_editor 工具。path 描述以真实工作区根路径为示例：
	// shell 等原生命令无法解析 /workspace 虚拟前缀，模型应优先使用真实路径；
	// /workspace 仍作为工作区根别名被本工具与 sandbox 接受（对齐 DSH 以真实
	// 路径呈现 workspace-write 根的设计，避免模型臆造不存在的虚拟路径而死循环）。
	workspaceRoot := plugin.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	examplePath := filepath.ToSlash(workspaceRoot) + "/file.py"
	pathDesc := "Absolute path to the file, e.g. " + examplePath +
		`. "/workspace" is also accepted as an alias for the workspace root (` +
		filepath.ToSlash(workspaceRoot) + `), but prefer the real path above for shell commands.`
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"enum": ["view", "create", "str_replace", "insert"],
				"description": "The commands to run. Allowed options are: view, create, str_replace, insert."
			},
			"path": {
				"type": "string",
				"description": ` + strconv.Quote(pathDesc) + `
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
	handler := func(ctx context.Context, args json.RawMessage) (string, error) {
		return strReplaceEditorHandler(ctx, state, args)
	}

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
	// PluginMetadata / PluginHookService 与 go-plugin 组装。
	sdk := dsc.New(dsc.Config{Name: "str_replace_editor", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: "str_replace_editor", Description: "Custom editor tool for viewing, creating, and editing files. Supports commands: view, create, str_replace, insert.", Schema: schema, Handler: handler})
	sdk.Serve()
}
