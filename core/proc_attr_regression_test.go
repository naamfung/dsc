package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// 哨兵测试：全仓扫描运行时 os/exec 派生点，确保每一个都落在「Windows 隐藏控制台」
// 统一修正（00ed647）的防护网内——要么文件内直接调用 ConfigureChildProcessAttrs /
// hideChildConsole / setChildProcSysProcAttr 三者之一，要么明确登记在下方豁免清单
// 并注明理由。背景：Windows 上未隐藏的 console 子进程在宿主继承不到控制台时会新建
// 可见控制台窗口，TUI 中表现为终端一闪而过；若此后新增派生点忘记接入防护网，
// 本测试在 CI 就地拦截，防止「统一修正出现遗漏」复发。
//
// 新增派生点的正确接入方式：
//   - 宿主 core 包内：exec 之后立即 core.ConfigureChildProcessAttrs(cmd)；
//   - 插件内：SDK re-export dsc.ConfigureChildProcessAttrs(cmd)；
//   - 经 go-plugin 的插件子进程：无需处理，plugin/internal/cmdrunner 统一防护。
//
// 豁免须写明理由，方便后续复核。
func TestRuntimeSpawnSitesConsoleHidden(t *testing.T) {
	// 本测试文件位于 core/ 下，仓库根即其上一级。
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller 定位测试文件失败")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))

	// 目录豁免（相对仓库根，"/" 分隔）：非运行时链路或 vendored 第三方库。
	// libs/vodka、libs/anthropic-sdk-go：vendored 外部依赖，运行时不可达
	//（agenttoolset 的 bash/rg 派生无任何 DSC 代码引用；见全仓 import 排查）。
	// builder：开发者本机构建工具，始终运行在开发者终端内，非 TUI 运行时链路。
	skipDirs := map[string]bool{
		".git":                         true,
		"node_modules":                 true,
		"webui":                        true,
		"dist":                         true,
		"docs":                         true,
		"examples":                     true,
		"testdata":                     true,
		filepath.ToSlash("libs/vodka"): true,
		filepath.ToSlash("libs/anthropic-sdk-go"): true,
		filepath.ToSlash("builder"):               true,
	}

	// 文件豁免（相对仓库根，"/" 分隔）：派生点不直接接触 exec.Cmd 启动属性、
	// 但其启动路径已整体处于防护网内的位置，逐一注明理由。
	fileAllowlist := map[string]string{
		// core/manager.go 全部 11 处派生均将 *exec.Cmd 交予 plugin.NewClient(Cmd:)，
		// 本仓 fork 的 plugin/internal/cmdrunner.NewCmdRunner 统一 hideChildConsole
		//（单一 chokepoint，含热重载 5 处与未来调用点）。
		"core/manager.go": "go-plugin 插件子进程，经 cmdrunner 统一隐藏",
		// plugin/client.go 的 exec.Command("") 仅为 RunnerFunc 自定义路径下的 spec
		// 占位（只读其 Path/Env 做元数据，从不 Start）；真实插件进程一律经 NewCmdRunner。
		"plugin/client.go": "RunnerFunc 路径 spec 占位，从不 Start",
		// 测试辅助文件（文件名不带 _test.go 后缀但仅供测试使用）。
		"libs/sh/internal/testing.go": "测试 helper，非运行时链路",
	}

	// 防护网标记：文件内出现任一标记即视为已直接接入防护。
	guards := []string{
		"ConfigureChildProcessAttrs", // core / sdk re-export 命名
		"hideChildConsole",           // plugin/internal/cmdrunner、libs/sh/interp 命名
		"setChildProcSysProcAttr",    // core windows 平台内部命名
	}

	// 派生点特征：构造 *exec.Cmd 的全部形态（直接与别名导入）。
	spawnPatterns := []string{
		"exec.Command(",
		"exec.CommandContext(",
		"&exec.Cmd{",
	}

	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[relSlash] || skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		hasSpawn := false
		for _, p := range spawnPatterns {
			if strings.Contains(src, p) {
				hasSpawn = true
				break
			}
		}
		if !hasSpawn {
			return nil
		}
		guarded := false
		for _, g := range guards {
			if strings.Contains(src, g) {
				guarded = true
				break
			}
		}
		if guarded {
			return nil
		}
		if _, exempt := fileAllowlist[relSlash]; exempt {
			return nil
		}
		// 定位首个派生点行号，便于报错直达。
		line := 0
		for i, l := range strings.Split(src, "\n") {
			for _, p := range spawnPatterns {
				if strings.Contains(l, p) {
					line = i + 1
					break
				}
			}
			if line != 0 {
				break
			}
		}
		violations = append(violations,
			relSlash+":"+strconv.Itoa(line)+" 未接入隐藏控制台防护网（修复：exec 之后调用 ConfigureChildProcessAttrs；确属豁免则登记 fileAllowlist 并注明理由）")
		return nil
	})
	if err != nil {
		t.Fatalf("扫描仓库失败: %v", err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}
