package core

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestConfigPath_NoBackslash_RuntimeInvariant 核心运行时不变量：
// core.ConfigPath（init() 时由 os.Executable() 经 PDir+PJoin 链路算出）
// 不得含反斜杆。
//
// Linux 上此测试 trivial pass（路径本就纯正斜杆）；Windows CI 上是
// 真正的回归守护——若 PDir/PJoin 的归一化退化（如改回 filepath.ToSlash
// 在 Linux 是 no-op），os.Executable() 返回的反斜杆路径会直接污染
// ConfigPath，此测试在 Windows 上会失败。
//
// 这是用户最关心的不变量：禁止反斜杆染污路径传递链路
// （ConfigPath → main.go → plugins → 模型可见路径）。
func TestConfigPath_NoBackslash_RuntimeInvariant(t *testing.T) {
	if strings.Contains(ConfigPath, "\\") {
		t.Errorf("core.ConfigPath 含反斜杆（违反 POSIX 路径契约）：%q", ConfigPath)
	}
}

// TestConfigPath_RecomputedFromRealOsExecutableMatches 从真实 os.Executable()
// 重算 ConfigPath，断言：
//  1. 重算结果与 init() 设置的 ConfigPath 一致（验证 init() 链路无副作用）
//  2. 重算结果不含反斜杆（验证 os.Executable() 返回的路径经 PDir+PJoin
//     后归一化正确——这是用户关心的"os.Executable() 算出的路径经 ConfigPath
//     链路后是否正确"）
//
// 关键：此测试用真实 os.Executable()，不 mock。Linux 上 os.Executable()
// 返回 /proc/self/exe 之类正斜杆路径，trivial pass；Windows 上返回
// C:\foo\bar\dsc.exe 形式，验证 PDir+PJoin 把反斜杆正确归一化。
func TestConfigPath_RecomputedFromRealOsExecutableMatches(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable() 不可用：%v（测试环境限制）", err)
	}
	recomputed := computeConfigPath(exePath, nil)

	// 不变量 1：与 init() 设置的 ConfigPath 一致
	if recomputed != ConfigPath {
		t.Errorf("重算 ConfigPath 与 init() 值不一致：\n  recomputed = %q\n  ConfigPath = %q",
			recomputed, ConfigPath)
	}

	// 不变量 2：不含反斜杆——这是核心契约
	if strings.Contains(recomputed, "\\") {
		t.Errorf("从 os.Executable()=%q 重算的 ConfigPath 含反斜杆：%q",
			exePath, recomputed)
	}
}

// TestConfigPath_SimulateWindowsExePath 在 Linux 上用 Windows 风格的
// os.Executable() 返回值（含反斜杆）模拟 Windows 场景，验证
// computeConfigPath 输出不含反斜杆。
//
// 这是"在 Linux 上验证 Windows 行为"的折中方案——无法让 os.Executable()
// 在 Linux 返回 Windows 路径，但可以直接把 Windows 风格字符串喂给
// computeConfigPath（与 init() 用同一个函数），验证归一化能力。
//
// 前提：computeConfigPath → PDir/PJoin → toSlash（strings.ReplaceAll）
// 链路在所有平台都把反斜杆转正斜杆。若 toSlash 退化回 filepath.ToSlash
// （Linux 上是 no-op），此测试会失败——已通过手动退化验证过。
//
// 注意：filepath.Dir/Join 在 Linux 上不理解反斜杆为分隔符，会把反斜杆
// 当文件名字符。这意味着此测试在 Linux 上验证的是"反斜杆被 toSlash
// 转为正斜杆"这一归一化能力，而非 filepath.Dir/Join 的 Windows 路径
// 解析能力（后者只能在 Windows CI 上验证）。
func TestConfigPath_SimulateWindowsExePath(t *testing.T) {
	cases := []struct {
		name    string
		exePath string // 模拟 Windows os.Executable() 返回值
	}{
		{"Windows drive path", `C:\Users\foo\dsc.exe`},
		{"Windows mixed separators", `C:\Users/foo\dsc.exe`},
		{"UNC path", `\\server\share\dsc.exe`},
		{"Relative path with backslashes", `.\build\dsc.exe`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeConfigPath(c.exePath, nil)
			if strings.Contains(got, "\\") {
				t.Errorf("computeConfigPath(%q) 输出含反斜杆：%q（应全部归一化为正斜杆）",
					c.exePath, got)
			}
		})
	}
}

// TestConfigPath_ExeExecutableErrorFallback os.Executable() 出错时
// 回退到相对路径，且相对路径不含反斜杆。
func TestConfigPath_ExeExecutableErrorFallback(t *testing.T) {
	got := computeConfigPath("", fmt.Errorf("executable not available"))
	if got != "./config/config.yaml" {
		t.Errorf("exeErr 非空时应回退到 ./config/config.yaml，got %q", got)
	}
	if strings.Contains(got, "\\") {
		t.Errorf("fallback 路径不应含反斜杆：%q", got)
	}
}
