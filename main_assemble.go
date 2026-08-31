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

// injectRuntimeEnv 向所有插件进程注入当前模式/工作根/沙箱档（主路径与「失败重试」共用）。
func injectRuntimeEnv(merged *core.Config, mode, workspaceRoot, sandboxPolicy string) {
	for i := range merged.Plugins {
		e := &merged.Plugins[i]
		if e.Env == nil {
			e.Env = map[string]string{}
		}
		e.Env["DSC_MODE"] = mode
		e.Env["DSC_WORKSPACE_ROOT"] = workspaceRoot
		e.Env["DSC_SANDBOX_POLICY"] = sandboxPolicy
	}
}
