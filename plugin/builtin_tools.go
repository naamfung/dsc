package plugin

import (
	"os"
	"path/filepath"
)

// WorkspaceRoot 統一工作空間根目錄（對齊 DSH ctx.sandboxPolicy 的單一根來源）：
// 宿主進程由 config.yaml 的 workspace_root 解析（見 main.go），
// 工具插件等子進程則讀取宿主注入的 DSC_WORKSPACE_ROOT 環境變量。
var WorkspaceRoot string

// WorkspaceProtectionEnabled 工作空間保護開關
var WorkspaceProtectionEnabled = true // 默認啟用工作空間機制保護，限制模型文件操作在 workspace 根目錄內

func init() {
	// 子進程：優先取宿主注入的 DSC_WORKSPACE_ROOT / DSC_WORKSPACE_PROTECTION_ENABLED，
	// 使宿主與各插件進程對工作空間根與保護狀態保持一致。
	if root := os.Getenv("DSC_WORKSPACE_ROOT"); root != "" {
		WorkspaceRoot = root
	}
	if v := os.Getenv("DSC_WORKSPACE_PROTECTION_ENABLED"); v == "0" {
		WorkspaceProtectionEnabled = false
	}
	if WorkspaceRoot == "" {
		// 無注入時，默認以啟動目錄為根（與宿主 resolveWorkspaceRoot 一致：
		// 在哪个目录启动，就以哪个目录为工作区）。
		if cwd, err := os.Getwd(); err == nil {
			WorkspaceRoot = cwd
		} else if exePath, err := os.Executable(); err == nil {
			// 無法獲取 cwd 時以可執行文件所在目錄為根（与宿主 Getwd 失败退化一致）
			WorkspaceRoot = filepath.Dir(exePath)
		} else {
			WorkspaceRoot = "."
		}
	}
}
