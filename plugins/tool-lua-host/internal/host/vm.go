// Package host 实现 tool-lua-host 的脚本宿主：
// 扫描 scripts/ 目录加载 LUA 脚本（每个脚本一个独立 VM），把脚本注册的工具
// 汇总成宿主工具表，支持目录轮询热加载。
package host

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tool-lua-host/internal/bindings"
	"tool-lua-host/internal/checker"

	lua "github.com/wippyai/go-lua"
)

// Script 单个 LUA 脚本的 VM 状态。
type Script struct {
	Name  string
	Path  string
	L     *lua.LState
	mu    sync.Mutex // VM 非并发安全：工具执行/重载串行化
	Tools []string   // 本脚本注册的工具名
	hash  string     // main.lua 内容哈希（热加载检测）
}

// ToolDef 脚本注册的一个工具。
type ToolDef struct {
	Name        string
	Script      string
	Description string
	ParamsJSON  string
	Handler     *lua.LFunction
}

// loadScript 读取并加载一个脚本目录（<dir>/<name>/main.lua）。
// 语法错误阻止加载；类型诊断仅告警（类型系统 pre-convergence，避免误杀）。
func (h *Host) loadScript(name string) (*Script, error) {
	path := filepath.Join(h.dir, name, "main.lua")
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(src)

	// 语法门禁 + 类型诊断
	diags, err := checker.Check(content, name)
	if err != nil {
		return nil, fmt.Errorf("lua 脚本 %s 语法错误: %w", name, err)
	}
	for _, d := range diags {
		h.logf("lua 类型诊断 [%s]: %s", name, d)
	}

	L := lua.NewState()
	lua.OpenBase(L)
	lua.OpenString(L)
	lua.OpenTable(L)
	lua.OpenMath(L)
	L.SetTop(0) // 清理 Open 系列压入的库表，保证后续 GetTop/Get 语义从干净栈开始
	// 沙箱：go-lua 无 os/io 库，脚本无法访问宿主文件系统/进程

	// 标记脚本名（供 bindings.scriptName 取用）
	L.SetGlobal("__dsc_script", lua.LString(name))

	s := &Script{Name: name, Path: path, L: L}
	// 每个脚本一份 Services 副本：共享 LLM/Tool/Notify 客户端，Register 指向本脚本
	svc := *h.services
	svc.Register = func(script, toolName, desc, paramsJSON string, fn *lua.LFunction) error {
		return h.registerTool(s, toolName, desc, paramsJSON, fn)
	}
	bindings.Install(L, &svc)

	if err := L.DoString(content); err != nil {
		L.Close()
		return nil, fmt.Errorf("lua 脚本 %s 执行失败: %w", name, err)
	}
	s.hash = hashContent(content)
	return s, nil
}

func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
