package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"dsc/core"
)

// 本文件实现 `dsc setup` 交互式配置向导：在终端以「数字选择 + 字段提示」的方式
// 快速配置 LLM 连接（基址/模型/API key）与默认提供商，让 dsc 能直接启动到可用
// 状态。相较 rex 的全屏箭头菜单，这里采用行式菜单（不进入 raw mode），Windows
// 终端下更稳定直观；写回经 core.UpsertPluginEnv/SetConfigStringField 以 yaml.Node
// 文档级操作完成，只改目标字段、保留注释（避免手动改配置文件的格式风险）。
//
// 提供商列表不写死：扫描 plugins 目录下的 llm-* 插件动态发现（新增 LLM 插件
// 无需改 setup 代码即可出现）；env 键名优先从 config.yaml 现有条目语义学习
// （已有 BASE_URL/HOST、MODEL、API_KEY/TOKEN 键则沿用），否则按插件名前缀推导
// 默认约定（<PREFIX>_BASE_URL / <PREFIX>_MODEL / <PREFIX>_API_KEY）。

// isSetupCommand 判断命令行是否为 `dsc setup`：第一个非 flag 参数（不以 - 开头）
// 为 "setup" 时进入向导；-input 等带值 flag 会被跳过（其值不算位置参数）。
func isSetupCommand(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-input" || a == "-admin" || a == "-mode" || a == "-log":
			i++ // 跳过 flag 的取值（若存在）
		case strings.HasPrefix(a, "-"):
			// 无值 flag（-headless / -debugger 等），跳过
		default:
			return a == "setup"
		}
	}
	return false
}

// setupProvider 描述一个可配置的 LLM 提供商：name 对应 config.yaml 的插件名，
// 各字段对应插件进程读取的 env 键（见 plugins/llm-* 的 os.Getenv 约定）。
// enabled 反映 config.yaml 中该条目的启用状态（未声明条目默认未启用）。
type setupProvider struct {
	name     string
	baseKey  string
	modelKey string
	apiKey   string // 空表示该提供商无 API key（如 ollama）
	enabled  bool
}

// setupEnv 记录向导会话中的待写 env 键值（provider name -> key -> value）。
type setupEnv map[string]map[string]string

// env 返回 provider 的 env 映射；不存在时创建。
func (s setupEnv) env(provider string) map[string]string {
	m, ok := s[provider]
	if !ok {
		m = map[string]string{}
		s[provider] = m
	}
	return m
}

// semanticKey 按语义在 env 中挑选匹配子串的键名；不存在返回 ""。
func semanticKey(env map[string]string, substrs ...string) string {
	for _, sub := range substrs {
		for k := range env {
			if strings.Contains(k, sub) {
				return k
			}
		}
	}
	return ""
}

// envKeyForProvider 决定提供商的 base/model/api 键名：
// 优先复用 config 现有条目语义匹配的键（如 OLLAMA_HOST 而非 *_BASE_URL、
// ANTHROPIC_API_KEY 等），保证旧配置键名不被改写；否则按插件名前缀推导默认。
func envKeyForProvider(pluginName string, existing map[string]string) (baseKey, modelKey, apiKey string) {
	prefix := strings.ToUpper(strings.TrimPrefix(pluginName, "llm-")) + "_"
	baseKey = semanticKey(existing, "BASE_URL", "HOST", "BASE")
	if baseKey == "" {
		baseKey = prefix + "BASE_URL"
	}
	modelKey = semanticKey(existing, "MODEL")
	if modelKey == "" {
		modelKey = prefix + "MODEL"
	}
	apiKey = semanticKey(existing, "API_KEY", "TOKEN", "SECRET", "KEY")
	if apiKey == "" {
		apiKey = prefix + "API_KEY"
	}
	return
}

// discoverLLMProviders 基于插件配置状态动态发现 LLM 提供商，而非写死列表：
//  1. 以 config.yaml 的 plugins 状态为事实来源——type=llm 条目（含 env 键约定、
//     enabled 状态）原样纳入；2) 扫描 plugins 目录的 llm-* 目录补充「已存在但
//     config 未声明」的新插件（供向导启用）。env 键名优先沿用 config 条目现键，
//     新插件按插件名前缀推导默认约定。
//
// 返回的 setupProvider 带 enabled 标记，供向导展示「已启用/未启用」状态。
func discoverLLMProviders(configPath, pluginsDir string) []setupProvider {
	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		cfg = nil
	}
	// config 中 type=llm 的条目（无论 enabled，均纳入以便管理）
	byName := map[string]*core.PluginEntry{}
	var declared []string
	if cfg != nil {
		for i := range cfg.Plugins {
			p := &cfg.Plugins[i]
			if p.Type != "llm" {
				continue
			}
			byName[p.Name] = p
			declared = append(declared, p.Name)
		}
	}
	// 目录扫描补充未声明的 llm-* 插件
	entries, err := os.ReadDir(pluginsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "llm-") {
				continue
			}
			if _, ok := byName[e.Name()]; !ok {
				byName[e.Name()] = &core.PluginEntry{Name: e.Name(), Type: "llm"}
				declared = append(declared, e.Name())
			}
		}
	}
	sort.Strings(declared)
	out := make([]setupProvider, 0, len(declared))
	for _, name := range declared {
		p := byName[name]
		baseKey, modelKey, apiKey := envKeyForProvider(name, p.Env)
		out = append(out, setupProvider{
			name:     name,
			baseKey:  baseKey,
			modelKey: modelKey,
			apiKey:   apiKey,
			enabled:  p.Enabled,
		})
	}
	return out
}

// setupAsk 打印提示并从 in 读一行；空回车返回 def，trim 后非空则返回输入。
func setupAsk(in *bufio.Scanner, w io.Writer, label, def string) string {
	if def != "" {
		fmt.Fprintf(w, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(w, "%s: ", label)
	}
	if !in.Scan() {
		return def
	}
	if v := strings.TrimSpace(in.Text()); v != "" {
		return v
	}
	return def
}

// setupConfirm 打印确认提示（y/N 默认 N）。
func setupConfirm(in *bufio.Scanner, w io.Writer, label string) bool {
	ans := setupAsk(in, w, label, "n")
	return ans == "y" || ans == "Y"
}

// setupSelect 打印编号菜单并让用户输入一个数字返回；非法输入重试；空输入返回 -1。
func setupSelect(in *bufio.Scanner, w io.Writer, label string, names []string) int {
	for {
		fmt.Fprintf(w, "\n%s\n", label)
		for i, name := range names {
			fmt.Fprintf(w, "  %d) %s\n", i+1, name)
		}
		fmt.Fprintf(w, "  0) 返回\n")
		fmt.Fprintf(w, "请选择 (0-%d): ", len(names))
		if !in.Scan() {
			return -1
		}
		n, err := strconv.Atoi(strings.TrimSpace(in.Text()))
		if err != nil {
			continue
		}
		if n == 0 {
			return -1
		}
		if n >= 1 && n <= len(names) {
			return n - 1
		}
	}
}

// setupProviderStatus 返回提供商的配置状态描述（用于菜单展示）。
func setupProviderStatus(env map[string]string, p setupProvider) string {
	var parts []string
	if !p.enabled {
		parts = append(parts, "未启用")
	}
	if v := env[p.baseKey]; v != "" {
		parts = append(parts, "基址="+v)
	}
	if v := env[p.modelKey]; v != "" {
		parts = append(parts, "模型="+v)
	}
	if p.apiKey != "" {
		if env[p.apiKey] != "" {
			parts = append(parts, "key=已设置")
		} else {
			parts = append(parts, "key=未设置")
		}
	}
	if len(parts) == 0 {
		return "未配置"
	}
	return strings.Join(parts, ", ")
}

// runSetupProviderEditor 编辑单个提供商的 base/model/key 三字段；对 config 未
// 声明的提供商（enabled=false 且无 base 配置）在编辑后询问是否启用。返回更新
// 后的提供商（enabled 可能被改为 true，供调用方覆盖数组条目）。
func runSetupProviderEditor(in *bufio.Scanner, w io.Writer, p setupProvider, env map[string]string) setupProvider {
	fmt.Fprintf(w, "\n配置 %s：\n", p.name)
	base := setupAsk(in, w, "基址 "+p.baseKey, env[p.baseKey])
	if base != "" {
		env[p.baseKey] = base
	}
	model := setupAsk(in, w, "模型 "+p.modelKey, env[p.modelKey])
	if model != "" {
		env[p.modelKey] = model
	}
	if p.apiKey != "" {
		cur := env[p.apiKey]
		label := "API key " + p.apiKey
		if cur != "" {
			label += " (当前已设置，回车保留，输入新值覆盖)"
		}
		key := setupAsk(in, w, label, "")
		if key != "" {
			env[p.apiKey] = key
		}
	}
	// 新声明条目：编辑后询问是否启用（已有条目不改变 enabled 状态）。
	if !p.enabled {
		if setupConfirm(in, w, "启用该提供商") {
			p.enabled = true
		}
	}
	fmt.Fprintln(w, "已更新（未保存，返回主菜单后选择“保存”生效）。")
	return p
}

// runSetup 执行交互式配置向导。configPath 为要写回的 config.yaml，pluginsDir
// 为插件目录（用于动态发现 LLM 提供商）。
func runSetup(in *bufio.Scanner, w io.Writer, configPath, pluginsDir string) int {
	providers := discoverLLMProviders(configPath, pluginsDir)
	if len(providers) == 0 {
		fmt.Fprintln(os.Stderr, "未在 "+pluginsDir+" 发现任何 llm-* 插件，无法配置 LLM。")
		return 1
	}

	// 会话内待写 env（初始为现有值，向导只改用户触碰的字段）
	pending := setupEnv{}
	existing, err := core.LoadLLMPluginEnvs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return 1
	}
	for name, m := range existing {
		env := pending.env(name)
		for k, v := range m {
			env[k] = v
		}
	}

	// 默认提供商：优先取 config 的 default_llm；未配置时参考 agent 的
	// depends_on.llm（依赖拓扑），再回退到首个提供商。
	currentDefault := ""
	if cfg, err := core.LoadConfig(configPath); err == nil {
		currentDefault = cfg.DefaultLLM
		if currentDefault == "" {
			for i := range cfg.Plugins {
				p := &cfg.Plugins[i]
				if p.Type == "agent" && p.DependsOn != nil && p.DependsOn.LLM != "" {
					currentDefault = p.DependsOn.LLM
					break
				}
			}
		}
	}

	fmt.Fprintln(w, "\nDSC 快速配置向导")
	fmt.Fprintf(w, "基于 config 状态发现 %d 个 LLM 插件。改动仅在“保存”后写入 %s\n", len(providers), configPath)

	for {
		// 主菜单
		menu := []string{"配置 LLM 提供商…", "设置默认提供商…"}
		for _, p := range providers {
			menu = append(menu, fmt.Sprintf("%s（%s）", p.name, setupProviderStatus(pending.env(p.name), p)))
		}
		menu = append(menu, "保存并退出", "取消")

		idx := setupSelect(in, w, "主菜单", menu)
		if idx == -1 {
			fmt.Fprintln(w, "已取消，未做任何更改。")
			return 0
		}
		switch {
		case idx == 0:
			// 子菜单：选择要编辑的提供商
			pnames := make([]string, len(providers))
			for i, p := range providers {
				pnames[i] = p.name
			}
			pi := setupSelect(in, w, "选择要配置的提供商", pnames)
			if pi == -1 {
				continue
			}
			providers[pi] = runSetupProviderEditor(in, w, providers[pi], pending.env(providers[pi].name))
		case idx == 1:
			// 设置默认提供商
			pnames := make([]string, len(providers))
			for i, p := range providers {
				pnames[i] = p.name
			}
			pi := setupSelect(in, w, "选择默认提供商", pnames)
			if pi == -1 {
				continue
			}
			currentDefault = providers[pi].name
			fmt.Fprintf(w, "默认提供商已设为 %s（未保存）。\n", currentDefault)
		case idx >= 2 && idx < 2+len(providers):
			pi := idx - 2
			providers[pi] = runSetupProviderEditor(in, w, providers[pi], pending.env(providers[pi].name))
		case idx == 2+len(providers):
			// 保存
			if !saveSetup(in, w, configPath, pending, providers, currentDefault) {
				continue
			}
			fmt.Fprintln(w, "\n配置完成。现在可以运行 dsc 启动。")
			return 0
		default:
			fmt.Fprintln(w, "已取消，未做任何更改。")
			return 0
		}
	}
}

// saveSetup 先展示摘要确认，再逐个写回 provider env（含 enabled）与 default_llm。
// 返回是否保存成功。
func saveSetup(in *bufio.Scanner, w io.Writer, configPath string, pending setupEnv, providers []setupProvider, defaultLLM string) bool {
	fmt.Fprintln(w, "\n即将保存以下配置到 "+configPath+":")
	provNames := make([]string, 0, len(pending))
	for name := range pending {
		provNames = append(provNames, name)
	}
	sort.Strings(provNames)
	for _, name := range provNames {
		env := pending[name]
		var parts []string
		enabled := false
		for _, p := range providers {
			if p.name == name {
				enabled = p.enabled
				break
			}
		}
		if !enabled {
			parts = append(parts, "未启用")
		}
		if v := env[semanticKey(env, "BASE_URL", "HOST", "BASE")]; v != "" {
			parts = append(parts, "基址="+v)
		}
		if v := env[semanticKey(env, "MODEL")]; v != "" {
			parts = append(parts, "模型="+v)
		}
		if k := semanticKey(env, "API_KEY", "TOKEN", "SECRET", "KEY"); k != "" {
			if env[k] != "" {
				parts = append(parts, "key=已设置")
			} else {
				parts = append(parts, "key=未设置")
			}
		}
		if len(parts) == 0 {
			parts = append(parts, "无连接配置")
		}
		fmt.Fprintf(w, "  - %s: %s\n", name, strings.Join(parts, ", "))
	}
	if defaultLLM != "" {
		fmt.Fprintf(w, "  - default_llm: %s\n", defaultLLM)
	}

	if !setupConfirm(in, w, "确认保存") {
		fmt.Fprintln(w, "已取消保存，未做任何更改。")
		return false
	}

	// 写回被用户改动过的提供商（env + enabled）。
	for _, name := range provNames {
		env := pending[name]
		changed, err := core.UpsertPluginEnv(configPath, name, "llm", env)
		if err != nil {
			fmt.Fprintf(os.Stderr, "写回 %s 失败: %v\n", name, err)
			return false
		}
		for _, p := range providers {
			if p.name != name {
				continue
			}
			if !p.enabled {
				if err := core.SetPluginEnabled(configPath, name, false); err != nil {
					fmt.Fprintf(os.Stderr, "停用 %s 失败: %v\n", name, err)
					return false
				}
				changed = true
			}
			break
		}
		if changed {
			fmt.Fprintf(w, "  ✓ 已更新 %s\n", name)
		}
	}
	if defaultLLM != "" {
		if err := core.SetConfigStringField(configPath, "default_llm", defaultLLM); err != nil {
			fmt.Fprintf(os.Stderr, "写回 default_llm 失败: %v\n", err)
			return false
		}
	}
	return true
}
