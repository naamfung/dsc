package plugin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// WorkspaceRoot 默認工作空間目錄，基於程序可執行文件所在目錄
var WorkspaceRoot string

// WorkspaceProtectionEnabled 工作空間保護開關
var WorkspaceProtectionEnabled = true // 默認啟用工作空間機制保護，限制模型文件操作在 ./workspace 目錄內

func init() {
	// 初始化 WorkspaceRoot 為基於程序可執行文件所在目錄的 workspace 目錄
	exePath, err := os.Executable()
	if err != nil {
		// 若無法獲取可執行文件路徑，則回退到相對路徑 ./workspace
		WorkspaceRoot = "./workspace"
	} else {
		execDir := filepath.Dir(exePath)
		WorkspaceRoot = filepath.Join(execDir, "workspace")
	}
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

// hasMultipleDrives 檢查路徑是否包含多個盤符（如 D:\...\E:\...），這在 Windows 上會導致「語法不正確」錯誤
func hasMultipleDrives(path string) bool {
	// 簡單檢查：統計路徑中類似 "X:\" 或 "X:/" 的模式數量
	count := 0
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if len(part) >= 2 && part[1] == ':' {
			c := part[0]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				count++
			}
		}
	}
	return count > 1
}

// normalize4windowsPath 将各种路径格式统一转换为 Windows 绝对路径（反斜杠分隔）
func normalize4windowsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	// 处理 Unix 风格：/D/... 或 /d/...
	re := regexp.MustCompile(`^/([a-zA-Z])[/\\](.*)`)
	if matches := re.FindStringSubmatch(path); matches != nil {
		drive := strings.ToUpper(matches[1])
		rest := filepath.FromSlash(matches[2])
		return filepath.Clean(drive + ":\\" + rest)
	}
	return filepath.Clean(path)
}

// executeShellCommand 使用 mvdan.cc/sh/v3/interp 执行 shell 命令并返回输出
func executeShellCommand(ctx context.Context, cmd string) (string, error) {
	var outBuf bytes.Buffer
	runner, err := interp.New(interp.StdIO(nil, &outBuf, nil))
	if err != nil {
		return "", err
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return "", err
	}
	if err := runner.Run(ctx, file); err != nil {
		return "", err
	}
	return strings.TrimSpace(outBuf.String()), nil
}

// getValidAbsolutePath 將路徑轉換為絕對路徑，並嘗試多種格式確保路徑格式正確
func getValidAbsolutePath(reqPath string) (string, error) {
	// 首先嘗試 normalize4windowsPath 統一轉換
	normalizedPath := normalize4windowsPath(reqPath)

	// 生成多種可能的絕對路徑變體
	variants := []string{}

	// 變體 1: 直接使用 filepath.Abs 對原始 reqPath 和 normalizedPath
	for _, p := range []string{reqPath, normalizedPath} {
		abs1, err := filepath.Abs(p)
		if err == nil && !hasMultipleDrives(abs1) {
			variants = append(variants, abs1)
		}
	}

	// 變體 2: 如果 reqPath 包含反斜槓 \，轉換為正斜槓 / 後再使用 filepath.Abs
	if strings.Contains(reqPath, "\\") {
		reqPathSlash := strings.ReplaceAll(reqPath, "\\", "/")
		abs2, err := filepath.Abs(reqPathSlash)
		if err == nil && !hasMultipleDrives(abs2) {
			variants = append(variants, abs2)
		}
	}

	// 變體 3: 如果 reqPath 是 Unix 絕對路徑（以 / 或 \ 開頭且沒有盤符）
	if isAbsPath(reqPath) && !strings.Contains(reqPath, ":") {
		cwd, err := os.Getwd()
		if err == nil {
			vol := filepath.VolumeName(cwd)
			cleanReqNoLeadSep := strings.TrimLeft(reqPath, "/\\")

			// 構建為 D:/Agents/novelforge/main.go 格式（正斜槓）
			absUnixStyle := ""
			if strings.HasSuffix(vol, ":") {
				absUnixStyle = vol + "/" + cleanReqNoLeadSep
			} else {
				// 對於 \\server\share 形式
				absUnixStyle = vol
				if !strings.HasSuffix(absUnixStyle, "/") && !strings.HasSuffix(absUnixStyle, "\\") {
					absUnixStyle = absUnixStyle + "/"
				}
				absUnixStyle = absUnixStyle + cleanReqNoLeadSep
			}
			if !hasMultipleDrives(absUnixStyle) {
				variants = append(variants, absUnixStyle)
			}

			// 構建為 D:\Agents\novelforge\main.go 格式（反斜槓）
			absWinStyle := ""
			if strings.HasSuffix(vol, ":") {
				absWinStyle = vol + string(os.PathSeparator) + cleanReqNoLeadSep
			} else {
				absWinStyle = vol
				if !strings.HasSuffix(absWinStyle, "/") && !strings.HasSuffix(absWinStyle, "\\") {
					absWinStyle = absWinStyle + string(os.PathSeparator)
				}
				absWinStyle = absWinStyle + cleanReqNoLeadSep
			}
			if !hasMultipleDrives(absWinStyle) {
				variants = append(variants, absWinStyle)
			}
		}
	}

	// 對每個變體，確保其為絕對路徑並過濾掉包含多個盤符的變體
	finalVariants := []string{}
	for _, v := range variants {
		absV, err := filepath.Abs(v)
		if err == nil {
			if !hasMultipleDrives(absV) {
				finalVariants = append(finalVariants, absV)
			}
		}
	}

	// 移除重複項
	uniqueVariants := []string{}
	seen := make(map[string]bool)
	for _, v := range finalVariants {
		if !seen[v] {
			seen[v] = true
			uniqueVariants = append(uniqueVariants, v)
		}
	}

	// 嘗試找到第一個格式正確且符號鏈接解析成功（或父目錄存在）的路徑
	for _, p := range uniqueVariants {
		realP, err := filepath.EvalSymlinks(p)
		if err == nil {
			return realP, nil
		}
		// 如果解析失敗，檢查父目錄是否存在且無語法錯誤
		parent := filepath.Dir(p)
		_, symlinksErr := filepath.EvalSymlinks(parent)
		if symlinksErr == nil {
			// 父目錄存在且格式正確，返回 p 的絕對路徑
			absP, _ := filepath.Abs(p)
			return absP, nil
		}
	}

	// 如果傳統 Go 路徑處理失敗，嘗試使用 mvdan.cc/sh/v3/interp 執行 shell 命令來驗證或轉換路徑
	ctx := context.Background()

	// 嘗試使用 shell 的 `test -e` 或 `ls -ld` 來驗證路徑
	shellCmds := []string{
		// 嘗試將路徑作為參數傳遞給 shell 並檢查
		`test -e "` + strings.ReplaceAll(normalizedPath, `"`, `\"`) + `" && echo "EXISTS" || echo "NOT_EXISTS"`,
		`[ -e "` + strings.ReplaceAll(normalizedPath, `"`, `\"`) + `" ] && echo "EXISTS" || echo "NOT_EXISTS"`,
	}

	for _, cmd := range shellCmds {
		output, err := executeShellCommand(ctx, cmd)
		if err == nil && strings.Contains(output, "EXISTS") {
			// shell 確認路徑存在，返回 normalizedPath 的絕對路徑
			absP, _ := filepath.Abs(normalizedPath)
			return absP, nil
		}
	}

	// 如果 shell 執行也失敗，嘗試使用 PowerShell 或 CMD 作為兜底
	// 這裡簡化處理：如果 normalizedPath 格式正確且無多個盤符，直接返回其絕對路徑
	if !hasMultipleDrives(normalizedPath) {
		absP, _ := filepath.Abs(normalizedPath)
		return absP, nil
	}

	if len(uniqueVariants) > 0 {
		return uniqueVariants[0], nil
	}

	return "", os.ErrNotExist
}

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）
// 此函數為通用輔助函數，具體工具插件可根據需要實現自己的安全路徑檢查邏輯。
func safePath(base, reqPath string) (string, error) {
	// 首先將 reqPath 轉換為絕對路徑，並嘗試多種格式確保路徑格式正確
	absReq, err := getValidAbsolutePath(reqPath)
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
		// 對於不作限制的情況，返回 absReq
		return absReq, nil
	}

	// 如果工作空間保護未啟用，則不作限制，直接返回 realReq（整個檔案系統都可以訪問）
	if !WorkspaceProtectionEnabled {
		return realReq, nil
	}

	// 工作空間保護已啟用，檢查路徑是否在 base (WorkspaceRoot) 下
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", err
	}

	// 檢查 realReq 是否在 realBase 下
	if !strings.HasPrefix(realReq, realBase+string(os.PathSeparator)) && realReq != realBase {
		return "", os.ErrPermission
	}
	return realReq, nil
}
