package plugin

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveRelativeBinaryPath(t *testing.T) {
	// 模擬 execDir
	execDir := "/path/to/exec/dir"
	if runtime.GOOS == "windows" {
		execDir = "C:\\Agents\\dsc"
	}

	// 測試用例：相對路徑
	relativePaths := []string{
		"./plugins/llm-anthropic/llm-anthropic.exe",
		"plugins/llm-openai/llm-openai.exe",
		"./plugins/agent-react-loop/agent-react-loop.exe",
	}

	for _, relPath := range relativePaths {
		// 模擬 main.go 中的邏輯：
		// if !filepath.IsAbs(binaryPath) {
		//     binaryPath = filepath.Join(execDir, binaryPath)
		// }
		binaryPath := relPath
		if !filepath.IsAbs(binaryPath) {
			binaryPath = filepath.Join(execDir, binaryPath)
		}

		// 驗證結果是否為絕對路徑
		if !filepath.IsAbs(binaryPath) {
			t.Errorf("Expected absolute path, but got relative path: %s", binaryPath)
		}

		// 驗證結果是否以 execDir 開頭
		// filepath.Join 會規範化路徑，例如將 "/path/to/exec/dir/./plugins/..." 規範化為 "/path/to/exec/dir/plugins/..."
		execDirSlash := strings.ReplaceAll(execDir, "\\", "/")
		binaryPathSlash := strings.ReplaceAll(binaryPath, "\\", "/")
		if !strings.HasPrefix(binaryPathSlash, execDirSlash+"/") && binaryPathSlash != execDirSlash {
			t.Errorf("Expected path to start with %s/, but got: %s", execDirSlash, binaryPathSlash)
		}
	}

	// 測試用例：絕對路徑
	absPath := "/path/to/plugins/llm-anthropic/llm-anthropic.exe"
	if runtime.GOOS == "windows" {
		absPath = "C:\\path\\to\\plugins\\llm-anthropic\\llm-anthropic.exe"
	}

	// 模擬 main.go 中的邏輯：
	absBinaryPath := absPath
	if !filepath.IsAbs(absBinaryPath) {
		absBinaryPath = filepath.Join(execDir, absBinaryPath)
	}

	if !filepath.IsAbs(absBinaryPath) {
		t.Errorf("Expected absolute path to remain absolute, but got: %s", absBinaryPath)
	}
	if absBinaryPath != absPath {
		t.Errorf("Expected absolute path to remain unchanged, but got: %s", absBinaryPath)
	}
}
