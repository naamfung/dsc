package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"dsc-sdk"
	"dsc/core"
	"github.com/aymanbagabas/go-udiff"
)

// withinBase 判斷 real 路徑是否在 base 目錄（含 base 自身）之內。
// Windows 文件系統大小寫不敏感，故忽略大小寫比較（對齊宿主 containsPath）。
// 路徑分隔符一律用 "/"（正斜杠）：safePath 的入参 real/realBase 来自 dsc.PAbs /
// dsc.PJoin / dsc.PClean / core.CanonicalPath，这些 P* 函数在 Windows 上也
// 返回正斜杠结果（与 AGENTS.md 规则 11「内部 POSIX shell 统一正斜杠」一致）。
// 不得使用 string(os.PathSeparator)——Windows 上是反斜杠，与 P* 返回的正斜杠
// 不匹配，会让 withinBase 永远返回 false，导致所有相对路径都被拒绝
// （报 "permission denied"，实测场景：模型传 G:/Dev/quark-go/README.md，
// 经 MapWorkspacePath + 相对路径归并为 README.md，safePath 走相对分支，
// withinBase 前缀检查失败 → os.ErrPermission）。
func withinBase(real, realBase string) bool {
	if runtime.GOOS == "windows" {
		real = strings.ToLower(real)
		realBase = strings.ToLower(realBase)
	}
	return strings.HasPrefix(real, realBase+"/") || real == realBase
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

// makeAbsPath 將路徑轉換為絕對路徑，正確處理 Unix 絕對路徑和 Windows 盤符絕對路徑。
// 模型書寫的虛擬根路徑（/workspace）已在入口經 core.MapWorkspacePath 映射為真實路徑；
// 裸 POSIX 路徑（/docs）保持真實根語義，此處純絕對化後在 Windows 上即為當前盤根。
func makeAbsPath(reqPath string) (string, error) {
	// 路徑統一保持正斜槓（dsc.PAbs 結果一律正斜槓，Windows 上 os.* 亦接受正斜槓）。
	cleanReq := reqPath

	// 檢查是否為 Windows 盤符絕對路徑 (如 C:\ 或 C:/)
	if len(cleanReq) >= 2 && cleanReq[1] == ':' {
		c := cleanReq[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return dsc.PAbs(cleanReq)
		}
	}

	// 檢查是否為 Unix 絕對路徑或 Windows 無盤符根路徑 (以 / 或 \ 開頭)
	// 在 Windows 上，以 \ 開頭的路徑被視為相對於當前驅動器的根目錄
	// 直接使用 dsc.PAbs 即可正確處理這種情況（結果為正斜槓絕對路徑）
	return dsc.PAbs(cleanReq)
}

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接/junction 繞過）。
// base 若不存在會先創建；目標路徑或其父目錄不存在時也能正常解析（中間目錄可由調用方自行創建）。
func safePath(base, reqPath string) (string, error) {
	// 如果 reqPath 已經是絕對路徑（包括 Windows 盤符絕對路徑如 C:\ 或 C:/，以及 Unix 絕對路徑如 /xxx），則直接基於它進行安全校驗，絕不與 base 拼接
	if isAbsPath(reqPath) {
		absReq, err := makeAbsPath(reqPath)
		if err != nil {
			return "", err
		}
		realReq, err := dsc.PEvalSymlinks(absReq)
		if err != nil {
			// 解析失敗，可能文件不存在，則檢查父目錄
			parent := dsc.PDir(absReq)
			_, symlinksErr := dsc.PEvalSymlinks(parent)
			if symlinksErr != nil {
				return "", symlinksErr
			}
			// 對於絕對路徑，返回解析後的絕對路徑（不強制要求它在 base 下；是否允許
			// 寫 workspace 之外由宿主 pipeline 的 sandbox 策略統一判定）
			return absReq, nil
		}
		return realReq, nil
	}

	absBase, err := dsc.PAbs(base)
	if err != nil {
		return "", err
	}
	// 工作目錄（base）可能尚未創建（首次使用時），先確保它存在，
	// 否則解析真實路徑會報 “The system cannot find the file specified”
	if err := dsc.MkdirAll(absBase); err != nil {
		return "", err
	}
	// 根也經 CanonicalPath 解析真實路徑（穿透 junction/符號鏈接）
	realBase, err := core.CanonicalPath(absBase)
	if err != nil {
		return "", err
	}

	// 構建絕對路徑並清理（去除 . .. 等）
	absReq, err := dsc.PAbs(dsc.PJoin(realBase, reqPath))
	if err != nil {
		return "", err
	}
	absReq = dsc.PClean(absReq)

	// 詞法前綴檢查：確保在 base 目錄內（对齐 DSH：工具层不做独立策略开关，
	// 相对路径永远以 workspace 为根，防止 ../ 路径穿越；是否允许绝对路径写
	// workspace 之外由宿主 pipeline 的 sandbox 策略统一判定）。
	if !withinBase(absReq, realBase) {
		return "", os.ErrPermission
	}

	// 把目標整條 canonical 化（穿透 junction/符號鏈接至真實落點），再校驗真實
	// 路徑仍在 base 內——防 workspace 內指向外部的 junction 寫穿沙箱（P0-3）。
	realReq, err := core.CanonicalPath(absReq)
	if err != nil {
		return "", err
	}
	if !withinBase(realReq, realBase) {
		return "", os.ErrPermission
	}
	return realReq, nil
}

// normalizeWorkspacePath 已删除：/workspace 前缀剥离（以及此前缺失的裸 / 锚定）
// 统一由 core.MapWorkspacePath 承担（本入口经 mappedPath 接入），避免两套归并
// 语义并存（AGENTS.md 重复逻辑必须抽取）。

type strReplaceEditorArgs struct {
	Command    string `json:"command"`
	Path       string `json:"path"`
	FileText   string `json:"file_text"`
	OldStr     string `json:"old_str"`
	NewStr     string `json:"new_str"`
	InsertLine int    `json:"insert_line"`
	ViewRange  []int  `json:"view_range"`
}

// slashErr 委托 core.SlashErr（公共实现，见 core/errslash.go）：把文件系统
// 错误里的原生路径归一为正斜杆（Windows 上 os.* 错误内嵌反斜杆路径，直接透传
// 给模型/用户时与其余正斜杆路径风格不一致）。本地不再保留第二份替换实现
// （对齐 AGENTS.md 重复逻辑必须抽取）。
func slashErr(err error) error {
	return core.SlashErr(err)
}

// readFileForEdit 读取待编辑文件内容。路径指向目录时给出明确提示：Windows 上
// os.ReadFile 读目录只报 "Incorrect function."（ERROR_INVALID_FUNCTION），模型
// 无从判断是路径填错还是文件损坏；提前判定并说明是目录，便于模型自行纠正。
func readFileForEdit(reqPath string) (string, error) {
	fi, err := os.Stat(reqPath)
	if err != nil {
		return "", slashErr(err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", filepath.ToSlash(reqPath))
	}
	content, err := dsc.ReadFile(reqPath)
	if err != nil {
		return "", slashErr(err)
	}
	return string(content), nil
}

// formatFileView 对齐 DSH formatFileView：返回带 cat -n 风格行号的内容，
// 支持 view_range 分段查看。1-based 行号，[-1] 表示到文件末尾。
func formatFileView(path, content string, viewRange []int) string {
	allLines := strings.Split(content, "\n")
	lines := allLines
	initialLine := 1
	prompt := fmt.Sprintf("Here's the content of %s with line numbers (which has a total of %d lines)", path, len(allLines))

	if len(viewRange) == 2 {
		initialLine = viewRange[0]
		finalLine := viewRange[1]
		if initialLine < 1 || initialLine > len(allLines) {
			return fmt.Sprintf("Invalid `view_range`: [%d, %d]. First element should be within [1, %d]", initialLine, finalLine, len(allLines))
		}
		if finalLine == -1 {
			lines = allLines[initialLine-1:]
			prompt += fmt.Sprintf(" with view_range=[%d, -1]", initialLine)
		} else {
			if finalLine > len(allLines) {
				finalLine = len(allLines)
			}
			if finalLine < initialLine {
				return fmt.Sprintf("Invalid `view_range`: [%d, %d]. Second element should be >= first", initialLine, finalLine)
			}
			lines = allLines[initialLine-1 : finalLine]
			prompt += fmt.Sprintf(" with view_range=[%d, %d]", initialLine, finalLine)
		}
	}

	var b strings.Builder
	b.WriteString(prompt + ":\n")
	for i, line := range lines {
		fmt.Fprintf(&b, "%6d  %s\n", initialLine+i, line)
	}
	return b.String()
}

// listDirectory 对齐 DSH listDirectory：列出目录下 2 层深度的文件/目录。
func listDirectory(reqPath, relPath string) string {
	var rows []string
	rows = append(rows, fmt.Sprintf("d\t%s", relPath))
	visitDir(reqPath, 1, &rows)
	sort.Strings(rows)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Here're the files and directories up to 2 levels deep in %s, excluding hidden items:\n", relPath))
	for _, row := range rows {
		b.WriteString(row + "\n")
	}
	return b.String()
}

func visitDir(dirPath string, depth int, rows *[]string) {
	if depth > 2 {
		return
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" {
			continue
		}
		typeChar := "f"
		if e.IsDir() {
			typeChar = "d"
		}
		fullPath := dsc.PJoin(dirPath, name)
		relPath := filepath.ToSlash(strings.TrimPrefix(fullPath, dsc.PClean(core.WorkspaceRoot)+"/"))
		*rows = append(*rows, fmt.Sprintf("%s\t%s", typeChar, relPath))
		if e.IsDir() {
			visitDir(fullPath, depth+1, rows)
		}
	}
}

// matchOffsets 对齐 DSH matchOffsets：返回 content 中 search 的所有偏移量。
func matchOffsets(content, search string) []int {
	var offsets []int
	offset := 0
	for {
		idx := strings.Index(content[offset:], search)
		if idx < 0 {
			break
		}
		offsets = append(offsets, offset+idx)
		offset += idx + len(search)
	}
	return offsets
}

// lineNumbersAt 对齐 DSH lineNumbersAt：返回偏移量对应的 1-based 行号。
func lineNumbersAt(content string, offsets []int) []string {
	var result []string
	for _, offset := range offsets {
		line := 1
		for i := 0; i < offset && i < len(content); i++ {
			if content[i] == '\n' {
				line++
			}
		}
		result = append(result, fmt.Sprintf("%d", line))
	}
	return result
}

func strReplaceEditorHandler(ctx context.Context, argsJSON json.RawMessage) (string, error) {
	var args strReplaceEditorArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}

	// 参数校验：command 必填（小模型可能传空对象 {}）
	if args.Command == "" {
		return "", fmt.Errorf("command is required (one of: view, create, str_replace, insert)")
	}

	// 使用安全路徑檢查；workspace 根統一來自 core.WorkspaceRoot
	// （宿主按 config workspace_root 解析並經 DSC_WORKSPACE_ROOT 注入，對齊 DSH 單一策略歸屬）
	workspaceRoot := core.WorkspaceRoot

	// 虛擬根歸并統一走 core.MapWorkspacePath（源頭在 core，SDK 供第三方插件復用，
	// 各插件不再自行轉換）：/workspace 前綴錨定工作區根；裸 POSIX 路徑（如
	// /docs/architecture.md）保持真實根語義（Windows 上經下方 isAbsPath→dsc.PAbs
	// 解析為當前盤根，與 Linux 把 /docs 解析到真實根行為一致），模型須以 /workspace
	// 前綴引用工作區文件。
	mappedPath := core.MapWorkspacePath(args.Path)
	// 映射後落在工作區根內的路徑轉為相對形式，與模型直接傳相對路徑共用 safePath
	// 的 join 分支（create 可自建缺失父目錄；view 缺失路徑報工作區內真實路徑錯誤，
	// 而非走絕對分支報盤根 lstat 錯誤）。工作區外路徑保持絕對形式由 abs 分支校驗。
	if filepath.IsAbs(mappedPath) {
		if rel, relErr := dsc.PRel(workspaceRoot, mappedPath); relErr == nil &&
			rel != ".." && !strings.HasPrefix(rel, "../") {
			mappedPath = rel
		}
	}
	reqPath, err := safePath(workspaceRoot, mappedPath)
	if err != nil {
		return "", err
	}
	// diff 标签用相对 workspace 的路径（对齐 REX 的 a/path b/path），
	// ToSlash 统一为正斜杆，避免 Windows 绝对路径标签含反斜杆；
	// 工作区外路径（模型显式盘符路径）保留全路径标签
	relPath := filepath.ToSlash(reqPath)
	if prefix := filepath.ToSlash(dsc.PClean(workspaceRoot)) + "/"; strings.HasPrefix(relPath, prefix) {
		relPath = relPath[len(prefix):]
	}

	switch args.Command {
	case "view":
		// 对齐 DSH：view 返回带行号的内容（cat -n 风格），支持 view_range 分段查看
		fi, err := os.Stat(reqPath)
		if err != nil {
			return "", slashErr(err)
		}
		if fi.IsDir() {
			// 对齐 DSH：view 目录时列出 2 层深度的文件/目录
			return listDirectory(reqPath, relPath), nil
		}
		contentStr, err := readFileForEdit(reqPath)
		if err != nil {
			return "", err
		}
		return formatFileView(relPath, contentStr, args.ViewRange), nil

	case "create":
		if args.FileText == "" {
			return "", fmt.Errorf("file_text is required for create command")
		}
		// 对齐 DSH：create 不能覆盖已存在的文件
		if _, err := os.Stat(reqPath); err == nil {
			return "", fmt.Errorf("File already exists at: %s. Cannot overwrite files using command `create`.", relPath)
		}
		dir := dsc.PDir(reqPath)
		if err := dsc.MkdirAll(dir); err != nil {
			return "", slashErr(err)
		}
		if err := dsc.WriteFile(reqPath, []byte(args.FileText)); err != nil {
			return "", slashErr(err)
		}
		return appendDiff("File created successfully.", relPath, "", args.FileText), nil

	case "str_replace":
		if args.OldStr == "" {
			return "", fmt.Errorf("old_str is required for str_replace command")
		}
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for str_replace command")
		}

		contentStr, err := readFileForEdit(reqPath)
		if err != nil {
			return "", err
		}

		// 对齐 DSH：检查 old_str 唯一性——多匹配报错
		offsets := matchOffsets(contentStr, args.OldStr)
		if len(offsets) == 0 {
			return "", fmt.Errorf("No replacement was performed. old_str did not appear verbatim in %s", relPath)
		}
		if len(offsets) > 1 {
			lineNums := lineNumbersAt(contentStr, offsets)
			return "", fmt.Errorf("No replacement was performed. Multiple occurrences of old_str in lines [%s]. Please ensure it is unique", strings.Join(lineNums, ", "))
		}
		// 单一匹配：替换（对齐 DSH：new_str 可省略=删除匹配，Go 的空串等价）
		newContentStr := contentStr[:offsets[0]] + args.NewStr + contentStr[offsets[0]+len(args.OldStr):]
		if err := dsc.WriteFile(reqPath, []byte(newContentStr)); err != nil {
			return "", slashErr(err)
		}

		return appendDiff("File replaced successfully.", relPath, contentStr, newContentStr), nil

	case "insert":
		if args.NewStr == "" {
			return "", fmt.Errorf("new_str is required for insert command")
		}
		// 对齐 DSH：insert_line 是 0-based AFTER 语义（0=文件开头，len(lines)=末尾）
		if args.InsertLine < 0 {
			return "", fmt.Errorf("Invalid `insert_line` parameter: %d. It should be within the range [0, line_count]", args.InsertLine)
		}

		contentStr, err := readFileForEdit(reqPath)
		if err != nil {
			return "", err
		}

		lines := strings.Split(contentStr, "\n")
		// 对齐 DSH：insert_line 是 0-based AFTER 语义
		// insertLine=0 → 在第一行前插入；insertLine=len(lines) → 在末尾追加
		if args.InsertLine > len(lines) {
			args.InsertLine = len(lines)
		}
		newLines := append(append(append([]string{}, lines[:args.InsertLine]...), args.NewStr), lines[args.InsertLine:]...)
		newContent := strings.Join(newLines, "\n")
		if err := dsc.WriteFile(reqPath, []byte(newContent)); err != nil {
			return "", slashErr(err)
		}

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

// strReplaceEditor 是工具 handler 的包级入口：业务处理后于唯一出口做错误
// 路径归一（core.SlashErr）。safePath 在路径不存在等失败分支直接返回原生错误
// （如 EvalSymlinks 父目录失败的 *os.PathError，内嵌反斜杆原生路径——历史
// 回归即漏在此），逐点补挂易漏，统一在此结构化归一（宿主聚合工具服务出口
// 另有文本级兜底）。
func strReplaceEditor(ctx context.Context, args json.RawMessage) (string, error) {
	res, err := strReplaceEditorHandler(ctx, args)
	return res, core.SlashErr(err)
}

func main() {
	// 工具自身零观察状态：读前改写/新鲜度裁决由 policy 插件（fs-observation-policy）
	// 统一承载，宿主在工具流水线 pre/post-execute 转发事件并由其裁决（对齐 DSH
	// 「工具只管执行、policy 持有策略与状态」）。

	// 定义 str_replace_editor 工具。path 描述以真实工作区根路径为示例：
	// shell 等原生命令无法解析 /workspace 虚拟前缀，模型应优先使用真实路径；
	// /workspace 仍作为工作区根别名被本工具与 sandbox 接受（对齐 DSH 以真实
	// 路径呈现 workspace-write 根的设计，避免模型臆造不存在的虚拟路径而死循环）。
	workspaceRoot := core.WorkspaceRoot
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
                                "description": "Optional string parameter of 'str_replace' command containing the new string (if omitted, no string will be added). Required string parameter of 'insert' command containing the string to insert."
                        },
                        "insert_line": {
                                "type": "integer",
                                "description": "Required integer parameter of 'insert' command. The new_str will be inserted AFTER the line insert_line of path. 0 means insert at the beginning of the file."
                        },
                        "view_range": {
                                "type": "array",
                                "items": {"type": "integer"},
                                "description": "Optional parameter of 'view' command when path points to a file. If omitted, the full file is shown. If provided, the file will be shown in the indicated line number range, e.g. [11, 12] will show lines 11 and 12. Indexing at 1 to start. Setting [start_line, -1] shows all lines from start_line to the end of the file."
                        },
                        "sandbox_permissions": {
                                "type": "string",
                                "enum": ["workspace-write", "danger-full-access"],
                                "description": "Optional sandbox escalation, ONLY for retrying a write that was denied by the sandbox. Do NOT set it on normal calls: requesting a mode that is not strictly wider than the current one is ignored and the call simply runs under the current mode. To genuinely widen (read-only → workspace-write/danger-full-access; workspace-write → danger-full-access), retry the exact operation once with this field plus 'justification' to request one-call user approval."
                        },
                        "justification": {
                                "type": "string",
                                "description": "Required together with 'sandbox_permissions': a one-sentence reason shown to the user in the approval prompt."
                        }
                },
                "required": ["command", "path"]
        }`)
	// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
	// PluginMetadata / PluginHookService 与 go-core 组装。
	sdk := dsc.New(dsc.Config{
		Name:    "str_replace_editor",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
		Provides: map[string]string{
			// 提供 "editor" 能力：含 str_replace_editor 文件编辑工具
			"editor": "true",
		},
	})
	sdk.Tool(dsc.Tool{Name: "str_replace_editor", Description: "Custom editor tool for viewing, creating, and editing files. Supports commands: view, create, str_replace, insert.", Schema: schema, Handler: strReplaceEditor})
	sdk.Serve()
}
