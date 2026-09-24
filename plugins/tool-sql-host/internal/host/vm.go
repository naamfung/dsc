// Package host 实现 tool-sql-host 的插件宿主：
// 扫描插件目录加载 .dsp 插件（每个插件一个独立 LUA VM），把插件内脚本注册的工具
// 汇总成宿主工具表，支持目录轮询热加载。
//
// 与「目录 + 脚本文件」的载体不同，.dsp 插件的代码、元数据与运行期状态同处一个
// SQLite 库：插件经 dsc.store.* / dsc.sql.* 读写的是**自身文件**，即「程序即数据」。
package host

import (
	"fmt"
	"strings"
	"sync"

	"tool-sql-host/internal/bindings"
	"tool-sql-host/internal/checker"
	"tool-sql-host/internal/dsp"

	lua "github.com/wippyai/go-lua"
)

// Plugin 一个已加载的 .dsp 插件在宿主内的视图。
type Plugin struct {
	Name     string
	Path     string
	Artifact string // 产物哈希（元数据+代码）；状态写入不改变它，故写状态不会触发重载
	Entry    string
	Language string
	Dsp      *dsp.Plugin

	L     *lua.LState
	mu    sync.Mutex // VM 非并发安全：工具执行/重载串行化
	Tools []string   // 本插件注册的工具名
}

// ToolDef 插件注册的一个工具。
type ToolDef struct {
	Name        string
	Plugin      string
	Description string
	ParamsJSON  string
	Handler     *lua.LFunction
}

// 插件注册工具的前缀 = 插件名本身（合成规则见 dsc.QualifyToolName）：什么插件注册的
// 工具，就带什么前缀。插件一多，`mytool` 这样的裸名让模型无从判断归属；曾用载体代号
// 当前缀（sql_/dsp_）也不解决——它只说明「某个同技术载体的插件」，说不出是哪一个。
// 用插件名后，工具名与 list_dsp_plugins 列出的插件名逐字对应（如插件 hello 的 greet
// → hello_greet），归属一看即知。
//
// 注意：库内保留表（dsp_meta/dsp_blobs/dsp_state/dsp_log）仍用 dsp_ 前缀——那是同一个
// 库文件内部的命名空间，与下发给模型的工具名无关；脚本侧的 dsc.sql.* 内建也仍叫 sql，
// 它们命名的是真实的 SQL 操作，名副其实。

// loadPlugin 打开 .dsp 并用其入口脚本装配一个 LUA VM。
// 语法错误阻止加载；类型诊断仅告警（类型系统 pre-convergence，避免误杀）。
func (h *Host) loadPlugin(path string) (*Plugin, error) {
	d, err := dsp.Open(path)
	if err != nil {
		return nil, err
	}
	// 载体语言分派：先只支持 LUA。其余语言（含内嵌可执行二进制）在此显式拒绝，
	// 而非静默降级——写了 dsp_meta.language 的插件作者应当立刻知道它没被执行。
	if !strings.EqualFold(d.Language, dsp.LanguageLua) {
		return nil, fmt.Errorf("插件 %s 的载体语言 %q 暂不受支持（当前仅 %s）",
			d.Name, d.Language, dsp.LanguageLua)
	}
	raw, ok, err := d.Blob(d.Entry, dsp.KindScript)
	if err != nil {
		return nil, fmt.Errorf("读取入口 %s 失败: %w", d.Entry, err)
	}
	if !ok {
		return nil, fmt.Errorf("入口脚本 %s 不在 dsp_blobs（kind=script）中", d.Entry)
	}
	content := string(raw)

	diags, err := checker.Check(content, d.Name)
	if err != nil {
		return nil, fmt.Errorf("插件 %s 语法错误: %w", d.Name, err)
	}
	for _, diag := range diags {
		h.logf("插件 %s 类型诊断: %s", d.Name, diag)
	}

	L := lua.NewState()
	lua.OpenBase(L)
	lua.OpenString(L)
	lua.OpenTable(L)
	lua.OpenMath(L)
	L.SetTop(0) // 清理 Open 系列压入的库表，保证后续 GetTop/Get 语义从干净栈开始
	// 沙箱：go-lua 无 os/io 库，脚本无法直接访问宿主文件系统/进程；
	// 唯一的数据出入口是 dsc.* 内建（互通服务与自身 .dsp）。

	L.SetGlobal("__dsc_plugin", lua.LString(d.Name))

	p := &Plugin{
		Name: d.Name, Path: d.Path, Artifact: d.Artifact, Entry: d.Entry,
		Language: d.Language, Dsp: d, L: L,
	}
	// 每个插件一份 Services 副本：共享 LLM/Tool/Notify 客户端，
	// Store/Self/Info/Register 指向本插件自身（状态落盘在自身 .dsp）。
	svc := *h.services
	svc.Store = d
	svc.Self = d
	svc.Info = bindings.PluginInfo{Name: d.Name, Path: d.Path, ReadOnly: d.ReadOnly}
	svc.Register = func(_, toolName, desc, paramsJSON string, fn *lua.LFunction) error {
		return h.registerTool(p, toolName, desc, paramsJSON, fn)
	}
	bindings.Install(L, &svc)

	if err := L.DoString(content); err != nil {
		L.Close()
		return nil, fmt.Errorf("插件 %s 执行入口失败: %w", d.Name, err)
	}
	return p, nil
}
