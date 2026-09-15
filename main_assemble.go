package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	core "dsc/core"
)

// assembleMerged 把 config.yaml（llm/agent）+ preset（tool/policy/dsc）合并成
// 交给 Manager 声明式加载的插件集，并做两件关键事：
//  1. config.yaml 里启用的 tool/policy/dsc（含 install_dsc_plugin 安装的）也并入，
//     与 preset 按名去重、preset 优先（同名冲突取 preset，config 仅补 preset 没有的）——
//     使模型安装的插件也能跨重启生效；
//  2. 为主路径与「失败回退续启」共用，避免重复的合并逻辑。
func assembleMerged(llmEntries []core.PluginEntry, agentEntry *core.PluginEntry, mainCfg, presetCfg *core.Config, contextWindow int, headless bool, inputText string) *core.Config {
	merged := &core.Config{}
	merged.Plugins = append(merged.Plugins, llmEntries...)

	ag := agentEntry
	if ag == nil {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		ag = &core.PluginEntry{Name: "agent-react-loop", Type: "agent", Enabled: true,
			BinaryPath: filepath.ToSlash(filepath.Join("./plugins", "agent-react-loop", "agent-react-loop"+ext))}
	}
	agentEnv := map[string]string{"DSC_CONTEXT_WINDOW": strconv.Itoa(contextWindow)}
	if presetCfg != nil && presetCfg.Persona != "" {
		agentEnv["DSC_PRESET_PERSONA"] = presetCfg.Persona
	}
	if presetCfg != nil && presetCfg.PlanSection != "" {
		agentEnv["DSC_PLAN_SECTION"] = presetCfg.PlanSection
	}
	if (inputText != "" && !stdinIsRedirected()) || headless {
		agentEnv["DSC_SINGLE_TURN"] = "1"
	}
	for k, v := range ag.Env {
		agentEnv[k] = v
	}
	ag.Env = agentEnv
	merged.Plugins = append(merged.Plugins, *ag)

	// tool/policy/dsc：preset 是具体的预设、优先；config.yaml 只是补充——同名冲突取 preset，
	// config 仅并入 preset 没有的（install_dsc_plugin 写入 config.yaml 的插件因此仍能跨重启生效）。
	seen := map[string]bool{}
	for _, p := range merged.Plugins {
		seen[p.Name] = true
	}
	if presetCfg != nil {
		for _, e := range presetCfg.Plugins {
			if !e.Enabled || (e.Type != "tool" && e.Type != "policy" && e.Type != "dsc") {
				continue
			}
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			merged.Plugins = append(merged.Plugins, e)
		}
	}
	if mainCfg != nil {
		for _, e := range mainCfg.Plugins {
			if !e.Enabled || (e.Type != "tool" && e.Type != "policy" && e.Type != "dsc") {
				continue
			}
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			merged.Plugins = append(merged.Plugins, e)
		}
	}
	return merged
}

// assemblePluginSet 重试专用：从 config.yaml + preset 重新推导 llm/agent 条目并合并。
// 用于启动失败回退后，在主路径合并逻辑之外独立重建插件集（复用 assembleMerged）。
func assemblePluginSet(mainCfg, presetCfg *core.Config, contextWindow int, headless bool, inputText string) *core.Config {
	var llmEntries []core.PluginEntry
	if mainCfg != nil {
		for _, e := range mainCfg.Plugins {
			if e.Enabled && e.Type == "llm" {
				llmEntries = append(llmEntries, e)
			}
		}
	}
	if len(llmEntries) == 0 {
		llmName := os.Getenv("LLM_PROVIDER")
		if llmName == "" && mainCfg != nil {
			llmName = mainCfg.DefaultLLM
		}
		if llmName == "" {
			llmName = "openai"
		}
		llmEntries = []core.PluginEntry{{Name: llmName, Type: "llm", Enabled: true,
			BinaryPath: filepath.ToSlash(filepath.Join("./plugins", "llm-"+llmName, "llm-"+llmName+modelExt()))}}
	}
	var agentEntry *core.PluginEntry
	if mainCfg != nil {
		for i := range mainCfg.Plugins {
			e := mainCfg.Plugins[i]
			if e.Enabled && e.Type == "agent" {
				agentEntry = &e
				break
			}
		}
	}
	return assembleMerged(llmEntries, agentEntry, mainCfg, presetCfg, contextWindow, headless, inputText)
}

func modelExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// injectRuntimeEnv 向所有插件进程注入当前模式/工作根/沙箱档/审批策略（主路径与「失败重试」共用）。
func injectRuntimeEnv(merged *core.Config, mode, workspaceRoot, sandboxPolicy string) {
	// 审批策略 env（供 agent 渲染 approval:policy 上下文）：显式 DSC_APPROVAL 尊重之；
	// 未显式时注入「沙箱预设绑定」默认（对齐 DSH permission-presets：full→never、其餘→ask）。
	approvalPolicy := core.ApprovalPolicyName(core.SandboxApprovalDefault(core.ParseSandboxPolicy(os.Getenv("DSC_SANDBOX"))))
	if os.Getenv("DSC_APPROVAL") != "" {
		approvalPolicy = core.ApprovalPolicyName(core.DefaultApprovalPolicy())
	}
	for i := range merged.Plugins {
		e := &merged.Plugins[i]
		if e.Env == nil {
			e.Env = map[string]string{}
		}
		e.Env["DSC_MODE"] = mode
		e.Env["DSC_WORKSPACE_ROOT"] = workspaceRoot
		e.Env["DSC_SANDBOX_POLICY"] = sandboxPolicy
		e.Env["DSC_APPROVAL_POLICY"] = approvalPolicy
	}
}

// injectMaxOutputTokens 向 LLM 插件注入 DSC_MAX_OUTPUT_TOKENS（默认输出上限=上下文窗口值）。
// 仅当宿主探测命中 LLAMACPP 家族端点（/v1/models 返回 meta.n_ctx——LLAMACPP 特有字段）时
// 由调用方触发：anthropic 兼容口把 max_tokens 视为 required，字段缺席时服务端自填保守默认
// （laamaafung server-chat.cpp 为 4096），「不携带=等模型自然结束」在其上退化为服务端默认
// 截断；显式携带窗口值在 LLAMACPP 侧受上下文自然钳制（生成至 EOS 或窗口满），无害。
// 云端端点探测不命中、不注入——超模型输出上限的 max_tokens 会被 400 拒绝，维持不携带语义。
// 插件侧优先级：请求级参数（压缩等场景的窗口净余值）> 显式插件 env（ANTHROPIC_/
// OPENAI_MAX_OUTPUT_TOKENS）> 本注入值 > 不携带。
func injectMaxOutputTokens(merged *core.Config, maxTokens int) {
	if maxTokens <= 0 {
		return
	}
	for i := range merged.Plugins {
		e := &merged.Plugins[i]
		if e.Type != "llm" {
			continue
		}
		if e.Env == nil {
			e.Env = map[string]string{}
		}
		e.Env["DSC_MAX_OUTPUT_TOKENS"] = strconv.Itoa(maxTokens)
	}
}
