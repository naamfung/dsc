package core

import (
        "fmt"
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
// capabilities 找到声明该能力的插件并建立依赖关系。
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
// 扫描已加载插件的 PluginInfo.Capabilities（不带前缀的普通能力键），找到声明了
// 所需能力的插件并记入 ProviderName；尚未找到 provider 的项 ProviderName 为空。
//
// 对齐 DSH/Cordis「同域唯一 provider」语义：同一 (Type, Capability) 在同一 scope
// 内只允许一个 provider。若找到多个 provider，报错（fail-loud）而非静默取首个——
// DSH 的 reflect.ts provide() 在同 scope 内第二个注册会 throw
// `service "X" has been registered at <fiber>`。DSC 通过 fail-loud 在解析阶段
// 即暴露配置歧义，要求用户在 config.yaml 中只启用一个或经显式选择消除歧义。
//
// LLM 类型的特殊处理：LLM 能力允许多 provider 并存（对齐 DSH 的 adapter registry
// 模型——多个 LLM adapter 可注册不同 provider route，由 agentDefaultModel 显式选择）。
// 此时 preferredLLM 参数（来自 config.yaml 的 default_llm）作为显式选择器：
// 若 preferredLLM 非空且它提供了所需的 LLM 能力，直接选中它；否则 fail-loud。
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
                provider, err := m.findProviderByCapabilityLocked(r.Type, r.Capability, selfName)
                if err != nil {
                        // fail-loud：多个 provider 冲突，记录错误但仍返回空 provider——
                        // 调用方（repairPendingLocked / LoadFromConfig）据此保持 PENDING，
                        // 并在日志中暴露配置歧义供用户修正。
                        dep.ProviderName = ""
                        m.logger.Error("capability provider conflict (fail-loud)",
                                "plugin", selfName, "dep_type", r.Type,
                                "capability", r.Capability, "error", err.Error())
                } else {
                        dep.ProviderName = provider
                        if provider != "" {
                                m.logger.Info("resolved capability dependency",
                                        "plugin", selfName, "dep_type", r.Type,
                                        "capability", r.Capability, "provider", provider)
                        } else {
                                m.logger.Warn("capability requirement has no provider loaded yet",
                                        "plugin", selfName, "dep_type", r.Type,
                                        "capability", r.Capability)
                        }
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

// findProviderByCapabilityLocked 在已加载的插件元数据中查找声明了指定 capability
// 的插件。排除本插件自身，避免自引用。
//
// 对齐 DSH/Cordis「同域唯一 provider」语义：
//   - 找到 0 个：返回空串，无错误
//   - 找到 1 个：返回该插件名，无错误
//   - 找到 >1 个：返回空串 + error（fail-loud）
//
// LLM 类型的特殊处理：允许多个 LLM 插件并存（对齐 DSH adapter registry 模型）。
// 此时不报错，而是优先选择 m.agentLLMName（若已由 config.yaml 的 default_llm
// 或 LoadFromConfig 设置）作为显式选择。若 preferred 未设置或未提供该能力，
// fall back 到按名升序的首个——但这会记 warn，提示用户应设 default_llm 消除歧义。
//
// 此函数需调用方已持有 m.mu（读访问 m.coreMetadata）。
func (m *Manager) findProviderByCapabilityLocked(providerType, capability, selfName string) (string, error) {
        if capability == "" {
                return "", nil
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
                if v, ok := info.Capabilities[capability]; ok && v != "false" {
                        names = append(names, name)
                }
        }
        sort.Strings(names)
        switch len(names) {
        case 0:
                return "", nil
        case 1:
                return names[0], nil
        default:
                // 多 provider 冲突
                if providerType == "llm" {
                        // LLM 类型：对齐 DSH adapter registry 模型——多 LLM provider
                        // 可并存，由 default_llm 显式选择。若 preferred 已设置且提供了
                        // 该能力，直接选中；否则按名升序取首个并记 warn。
                        if m.agentLLMName != "" && containsString(names, m.agentLLMName) {
                                return m.agentLLMName, nil
                        }
                        m.logger.Warn("multiple LLM providers found for capability, using first by name (set default_llm in config.yaml to disambiguate)",
                                "capability", capability,
                                "providers", strings.Join(names, ", "),
                                "selected", names[0])
                        return names[0], nil
                }
                // 非 LLM 类型：fail-loud，对齐 DSH 的同域唯一 provider 约束
                return "", fmt.Errorf("multiple providers found for capability %q (type=%s): %s — enable only one in config.yaml or use isolate scopes",
                        capability, providerType, strings.Join(names, ", "))
        }
}

// HasPluginRequiringCapability 报告是否有任何已加载的插件声明了
// requires/<providerType>/<capability> 能力依赖。供宿主决定是否启动
// 某些可选服务（如管理 API：只有 tool-harness-webui 等插件声明
// requires/dsc/admin 时才自动启动）。线程安全。
func (m *Manager) HasPluginRequiringCapability(providerType, capability string) bool {
        m.mu.RLock()
        defer m.mu.RUnlock()
        for _, info := range m.coreMetadata {
                if info == nil {
                        continue
                }
                requires := DecodeRequires(info)
                for _, r := range requires {
                        if r.Type == providerType && r.Capability == capability {
                                return true
                        }
                }
        }
        return false
}

// HasPluginProvidingCapability 报告是否有任何已加载的插件在 PluginInfo.Capabilities
// 中声明了指定能力（普通能力键，非 requires/ 前缀）。供宿主检测后端接管——
// 如 billion-context 声明 Provides: {"compaction": "true"}，宿主检测到后设
// DSC_ACP_ACTIVE=1 让 agent 跳过内联压缩。线程安全。
func (m *Manager) HasPluginProvidingCapability(capability string) bool {
        m.mu.RLock()
        defer m.mu.RUnlock()
        for _, info := range m.coreMetadata {
                if info == nil || len(info.Capabilities) == 0 {
                        continue
                }
                if v, ok := info.Capabilities[capability]; ok && v != "false" {
                        return true
                }
        }
        return false
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
