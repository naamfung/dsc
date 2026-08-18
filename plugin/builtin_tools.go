package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

var WorkspaceRoot = "./workspace" // 可改為環境變數或配置

// safePath 檢查並返回安全的路徑（防止路徑遍歷和符號鏈接繞過）
// 此函數為通用輔助函數，具體工具插件可根據需要實現自己的安全路徑檢查邏輯。
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
