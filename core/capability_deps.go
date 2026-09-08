package core

import (
	"sort"
	"strings"

	"dsc/proto/metadata"
)

// requiresCapabilityKeyPrefix 是声明式依赖在 PluginInfo.Capabilities 中的编码前缀
// （与 SDK 侧 dsc.requiresCapabilityKeyPrefix 同值，对齐同一 wire 编码约定）。
// 复用现有 capabilities map（避免改动 proto）：键形如 "requires/llm/supports_images"
// 或 "requires/tool/cron"，值固定为 "true"。
//
// 普通能力键（不带前缀，如 "supports_images": "true"、"cron": "true"）表示本插件
// **提供**了该能力——其他插件经 Requires 声明对此能力的依赖时，宿主扫描
// capabilities 找到首个声明该能力的插件并建立依赖关系。
//
// 此编码对齐 DSH/Cordis 的 provide + inject 模型——插件按能力边界声明依赖，
// 不按插件名引用（参见 vendor/cordis/src/registry.ts Plugin.Base.provide / inject）。
const requiresCapabilityKeyPrefix = "requires/"

// parseRequiresCapability 解析一个 capabilities 键，若是 requires/<type>/<cap> 形式
// 则返回 (type, cap, true)，否则返回 ("", "", false)。
func parseRequiresCapability(key string) (reqType, capability string, ok bool) {
	if len(key) <= len(requiresCapabilityKeyPrefix) {
		return "", "", false
	}
	if key[:len(requiresCapabilityKeyPrefix)] != requiresCapabilityKeyPrefix {
		return "", "", false
	}
	rest := key[len(requiresCapabilityKeyPrefix):]
	// 形如 "llm/supports_images" 或 "tool/cron"：按第一个 "/" 拆分
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			reqType = rest[:i]
			capability = rest[i+1:]
			if reqType != "" && capability != "" && !strings.Contains(capability, "/") {
				return reqType, capability, true
			}
			return "", "", false
		}
	}
	return "", "", false
}

// CapabilityRequirement 一条声明式能力依赖：本插件依赖由 Type 类型插件提供的
// Capability 能力。镜像 SDK 侧 dsc.CapabilityRequirement 结构体（避免循环引用）。
type CapabilityRequirement struct {
	Type       string
	Capability string
}

// DecodeRequires 从 PluginInfo.Capabilities 提取本插件声明的全部能力依赖。
// 无声明返回 nil。
func DecodeRequires(info *metadata.PluginInfo) []CapabilityRequirement {
	if info == nil || len(info.Capabilities) == 0 {
		return nil
	}
	var out []CapabilityRequirement
	for k := range info.Capabilities {
		if rt, rc, ok := parseRequiresCapability(k); ok {
			out = append(out, CapabilityRequirement{Type: rt, Capability: rc})
		}
	}
	// 稳定排序：避免 map 迭代顺序导致持久化结果不稳定（AGENTS.md 第6条）
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Capability < out[j].Capability
	})
	return out
}

// ResolvedDep 一条已解析的能力依赖：本插件依赖由 providerName 插件提供的
// capability 能力。供运行时态查询（m.resolvedDeps）与反应式重算（repairPendingLocked）。
type ResolvedDep struct {
	Type         string // "llm" | "tool" | "agent" | "policy" | "dsc"
	Capability   string // 能力名（不带 requires/ 前缀）
	ProviderName string // 已解析到的提供者插件名；空串表示尚未找到 provider
}

// resolveRequiredDeps 把本插件的声明式依赖（Requires）解析为 ResolvedDep 列表：
// 扫描已加载插件的 PluginInfo.Capabilities（不带前缀的普通能力键），找到首个
// 声明了所需能力的插件并记入 ProviderName；尚未找到 provider 的项 ProviderName 为空。
//
// 此函数需调用方已持有 m.mu（读访问 m.coreMetadata）。
func (m *Manager) resolveRequiredDeps(info *metadata.PluginInfo, selfName string) []ResolvedDep {
	requires := DecodeRequires(info)
	if len(requires) == 0 {
		return nil
	}
	out := make([]ResolvedDep, 0, len(requires))
	for _, r := range requires {
		dep := ResolvedDep{Type: r.Type, Capability: r.Capability}
		dep.ProviderName = m.findProviderByCapabilityLocked(r.Type, r.Capability, selfName)
		if dep.ProviderName != "" {
			m.logger.Info("resolved capability dependency",
				"plugin", selfName, "dep_type", r.Type,
				"capability", r.Capability, "provider", dep.ProviderName)
		} else {
			m.logger.Warn("capability requirement has no provider loaded yet",
				"plugin", selfName, "dep_type", r.Type,
				"capability", r.Capability)
		}
		out = append(out, dep)
	}
	return out
}

// depsSatisfiedLocked 报告给定的 ResolvedDep 列表是否全部已解析到 provider
// 且对应 provider 已加载可用。需调用方已持有 m.mu。
func (m *Manager) depsSatisfiedLocked(deps []ResolvedDep) bool {
	for _, d := range deps {
		if d.ProviderName == "" {
			return false
		}
		if !m.providerAvailableLocked(d.ProviderName) {
			return false
		}
	}
	return true
}

// providerAvailableLocked 报告名为 name 的插件当前是否已加载并可用作依赖来源
// （agent / llm / tool / policy / dsc 均计为可提供能力）。需调用方已持有 m.mu。
func (m *Manager) providerAvailableLocked(name string) bool {
	if _, ok := m.llmServiceIDs[name]; ok {
		return true
	}
	if _, ok := m.toolServiceIDs[name]; ok {
		return true
	}
	if _, ok := m.agents[name]; ok {
		return true
	}
	if _, ok := m.typeMap[name]; ok {
		return true
	}
	return false
}

// findProviderByCapabilityLocked 在已加载的插件元数据中查找首个声明了指定 capability
// 的插件（按名升序，稳定输出）。排除本插件自身，避免自引用。
//
// capability 是不带前缀的普通能力键（如 "supports_images"、"cron"），与目标插件
// PluginInfo.Capabilities 中的键名直接比对。
//
// 此函数需调用方已持有 m.mu（读访问 m.coreMetadata）。
func (m *Manager) findProviderByCapabilityLocked(providerType, capability, selfName string) string {
	if capability == "" {
		return ""
	}
	var names []string
	for name, info := range m.coreMetadata {
		if name == selfName {
			continue // 避免自引用
		}
		if info == nil || info.Type != providerType {
			continue
		}
		// 期望 capabilities 中存在一个不带 requires/ 前缀的普通能力键
		// （如 "supports_images": "true"）。requires/ 前缀的键表示该插件
		// 自身的依赖声明，不是它提供的能力。
		if v, ok := info.Capabilities[capability]; ok && v != "false" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// containsString 报告 slice 中是否含 s。
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
