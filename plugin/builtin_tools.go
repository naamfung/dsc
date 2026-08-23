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
		// 無注入時，基於程序可執行文件所在目錄的 workspace 目錄
		if exePath, err := os.Executable(); err == nil {
			WorkspaceRoot = filepath.Join(filepath.Dir(exePath), "workspace")
		} else {
			// 若無法獲取可執行文件路徑，則回退到相對路徑 ./workspace
			WorkspaceRoot = "./workspace"
		}
	}
}
