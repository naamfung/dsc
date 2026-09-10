package core

import (
	"context"
	"fmt"
	"sort"
)

// 动态注入插件的端到端能力。
//
// 相比仅拉起子进程，动态注入还补齐三个闭环环节：
//  1. 依赖判定：注入条目声明的能力依赖未满足时置为 PENDING 等待（而非直接失败/Active）；
//  2. 声明持久化：注入（含 PENDING）写回 config.yaml，进程重启后依旧保留；
//  3. 缺失修复与 agent 再激活：注入的 LLM/Tool 可能补足先前缺口，把等待中的 PENDING
//     provider 提升加载，并把因缺 LLM 而 PENDING 的 agent 重新注入 RegisterServices 并激活。
//
// 依赖模型对齐 DSH/Cordis 的能力边界声明：
//   - 插件经 PluginInfo.Capabilities 普通能力键（如 "supports_images": "true"）声明自己
//     **提供**哪些能力，对应 Cordis 的 provide；
//   - 插件经 PluginInfo.Capabilities 的 requires/<type>/<cap> 编码声明自己**依赖**哪些能力
//     （见 sdk.Config.Requires），对应 Cordis 的 inject；
//   - 宿主扫描已加载插件的能力键自动匹配依赖，无需用户在 config.yaml 手工指定 depends_on
//     按插件名引用——按插件名指定依赖的旧有机制已删除（见 AGENTS.md 第4条「禁止保留
//     Deprecated 代码」）。

// coreLoadedLocked 判断插件当前是否已加载到 Manager（agent 或 provider）。
func (m *Manager) coreLoadedLocked(name string) bool {
	if _, ok := m.agents[name]; ok {
		return true
	}
	if _, ok := m.typeMap[name]; ok {
		return true
	}
	return false
}

// deferPendingLocked 把依赖未满足的注入条目置为 PENDING 并记录待办（不拉起子进程），
// 同时把声明写回 config.yaml，等待后续注入补足依赖。
func (m *Manager) deferPendingLocked(entry PluginEntry) error {
	m.markPendingLocked(entry.Name, entry.Type, "capability dependency not satisfied")
	m.pendingEntries[entry.Name] = entry
	if err := m.persistInjectionLocked(entry); err != nil {
		m.logger.Warn("persist pending injection failed", "name", entry.Name, "error", err)
	}
	m.logger.Info("core deferred to pending (capability dependency not satisfied)", "name", entry.Name)
	return nil
}

// injectionEntryLocked 注入单个插件（需已持有 m.mu）：
//   - agent 作为 broker 提供者先拉起进程但不立即激活；能力依赖满足则直接激活，否则置 PENDING；
//   - llm/tool/policy 按类型加载，能力依赖未满足则进入 PENDING；
//   - persist=true 时将声明持久化写回 config.yaml（重启后仍加载），false 则仅当前进程生效；
//     随后触发 repairPendingLocked 修复其他等待中的插件。
//
// 能力依赖自动解析（与持久化解耦）：
//   - 自动解析的时机无限制——只要插件 PluginInfo 可读即可，不要求依赖已满足。
//   - 解析得到的 ResolvedDep 列表一律写入运行时态 m.resolvedDeps[entry.Name]，供运行时态
//     查询与反应式重算使用，**无论 persist 开关如何**。
//   - persist=true 时把 entry 写回 config.yaml（仅记插件条目本身，不再有 depends_on 字段——
//     能力依赖由插件二进制内的 sdk.Config.Requires 自描述，无需落盘）；
//     persist=false（孤儿插件经 load_dsc_plugin 默认载入）时**绝不写配置**。
func (m *Manager) injectionEntryLocked(entry PluginEntry, persist bool) error {
	switch entry.Type {
	case "agent":
		// agent 提供 broker；loadAgentAndGetBroker 拉起进程、注册 stop hook 并记录 agentEntries
		if _, _, err := m.loadAgentAndGetBroker(entry); err != nil {
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return err
		}
		if !m.agentDepsSatisfiedLocked(entry.Name) {
			m.markPendingLocked(entry.Name, "agent", "capability dependency not satisfied")
			m.pendingEntries[entry.Name] = entry
		} else {
			m.reactivateAgentLocked(entry.Name) // 依赖已满足则直接注入并激活
		}
	case "llm", "tool", "policy", "dsc":
		if m.broker == nil {
			return fmt.Errorf("broker not available, cannot inject core type %s", entry.Type)
		}
		if !m.providerDepsSatisfiedLocked(entry.Name) {
			return m.deferPendingLocked(entry)
		}
		if err := m.loadProviderDeclarativeLocked(entry); err != nil {
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return err
		}
	default:
		return fmt.Errorf("unsupported core type for injection: %s", entry.Type)
	}

	// 自动解析 + 可选持久化。
	m.autoResolveAndPersistDepsLocked(entry, persist)

	// 修复：本次注入可能补足了先前的缺口，提升等待中的 provider / 再激活 PENDING agent
	return m.repairPendingLocked()
}

// autoResolveAndPersistDepsLocked 在插件加载成功后做两件事（需已持有 m.mu）：
//  1. 能力依赖自动解析：从 m.coreMetadata[entry.Name] 读取 PluginInfo，解析其中的
//     requires/<type>/<cap> 编码并匹配已加载插件的能力键，得到 ResolvedDep 列表
//     （含已解析到的 provider 名；尚未找到 provider 的项 ProviderName 为空）。
//     解析结果一律存入 m.resolvedDeps[entry.Name]，供运行时态查询与反应式重算——
//     **无论 persist 开关如何**。
//  2. 可选持久化：persist=true 才把 entry 写回 config.yaml；
//     persist=false 时**绝不写配置**——保护孤儿插件等未持久化状态。
//
// 两条契约独立成立：自动解析的时机与是否持久化无关；解析得到的 ResolvedDep 在运行时态
// 始终可用，配置写入只受 persist 开关控制。
func (m *Manager) autoResolveAndPersistDepsLocked(entry PluginEntry, persist bool) {
	// 1. 能力依赖自动解析 + 运行时态存储
	if info, ok := m.coreMetadata[entry.Name]; ok {
		deps := m.resolveRequiredDeps(info, entry.Name)
		if deps != nil {
			m.resolvedDeps[entry.Name] = deps
		}
	}
	// 2. 可选持久化：persist=true 才写回 config.yaml（保证重启保留），false 仅本进程生效。
	if persist {
		if err := m.persistInjectionLocked(entry); err != nil {
			m.logger.Warn("persist injection failed", "name", entry.Name, "error", err)
		}
	}
}

// GetResolvedDeps 返回某插件在运行时态经能力依赖自动解析得到的 ResolvedDep 列表（只读副本）。
// 不区分插件是否经 config.yaml 持久化——孤儿插件（load_dsc_plugin persist=false 载入）
// 的解析结果也存于此，可在运行时态查询使用。无解析结果返回 nil。
//
// 供需要查询当前插件依赖关系的运行时路径使用（如未来可能的「依赖可视化」、agent
// 再激活、tool 互通路由等）。此为只读视图，调用方不应修改返回值。
func (m *Manager) GetResolvedDeps(name string) []ResolvedDep {
	m.mu.RLock()
	defer m.mu.RUnlock()
	deps, ok := m.resolvedDeps[name]
	if !ok || deps == nil {
		return nil
	}
	out := make([]ResolvedDep, len(deps))
	copy(out, deps)
	return out
}

// agentDepsSatisfiedLocked 报告名为 name 的 agent 的能力依赖是否全部满足。
// agent 的能力依赖来自其 PluginInfo 的 requires/<type>/<cap> 编码，经
// resolveRequiredDeps 解析后存于 m.resolvedDeps[name]。无依赖视为满足。
// 需调用方已持有 m.mu。
func (m *Manager) agentDepsSatisfiedLocked(name string) bool {
	deps, ok := m.resolvedDeps[name]
	if !ok {
		return true // 无声明依赖视为满足
	}
	return m.depsSatisfiedLocked(deps)
}

// providerDepsSatisfiedLocked 报告名为 name 的 provider（llm/tool/policy/dsc）的
// 能力依赖是否全部满足。语义同 agentDepsSatisfiedLocked，单独命名仅为可读性。
// 需调用方已持有 m.mu。
func (m *Manager) providerDepsSatisfiedLocked(name string) bool {
	deps, ok := m.resolvedDeps[name]
	if !ok {
		return true // 无声明依赖视为满足
	}
	return m.depsSatisfiedLocked(deps)
}

// repairPendingLocked 扫描所有 PENDING 插件并尽力提升（需已持有 m.mu）：
//   - provider(llm/tool/policy)：能力依赖就绪则按类型实际加载（Pending→Connecting→…→Active）；
//   - agent：能力依赖就绪则重新注入 RegisterServices 并置为 Active。
//
// 反复迭代直至无新增提升，以覆盖链式依赖。
//
// 反应式能力重算（对齐 DSH/Cordis 的 _refresh + notify）：
// 每轮扫描前，先对每个 PENDING entry 调用 resolveRequiredDeps 重算其 Requires 能力
// 是否被新加载的插件所提供——更新 m.resolvedDeps[name]，随后基于更新后的
// agentDepsSatisfiedLocked / providerDepsSatisfiedLocked 判定。
// 这与 DSH/Cordis 中 provider 加载触发依赖者 _refresh 的反应式语义一致。
func (m *Manager) repairPendingLocked() error {
	for {
		progressed := false
		// 收集 pendingEntries 的名并按名升序稳定排序后再迭代——
		// 不直接 range map（Go map 迭代序随机，AGENTS.md 第6条），避免提升顺序
		// 随进程随机漂移影响 m.llmOrder 追加顺序（虽不影响前缀缓存字节——
		// 工具目录经 AllToolsProto 独立按名排序，但路由 fallback 顺序仍应确定）。
		names := make([]string, 0, len(m.pendingEntries))
		for name := range m.pendingEntries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry := m.pendingEntries[name]
			// 仅对 provider 生效：非 agent 条目若已被其他路径加载，仅清除待办。
			// agent 必然已随 LoadFromConfig 拉起（作为 broker 提供者），“已加载”不代表已激活，
			// 必须继续走到 reactivateAgentLocked 做依赖注入，否则会漏掉 PENDING agent 的再激活。
			if entry.Type != "agent" && m.coreLoadedLocked(name) {
				delete(m.pendingEntries, name)
				progressed = true
				continue
			}
			// 反应式能力重算：新加载的插件可能提供了本 PENDING 插件 Requires 的能力，
			// 更新 m.resolvedDeps[name] 后再进入下方依赖判定。
			if info, ok := m.coreMetadata[name]; ok {
				if deps := m.resolveRequiredDeps(info, name); deps != nil {
					m.resolvedDeps[name] = deps
				}
			}
			// 按插件类型判定依赖是否就绪
			depsOK := false
			if entry.Type == "agent" {
				depsOK = m.agentDepsSatisfiedLocked(name)
			} else {
				depsOK = m.providerDepsSatisfiedLocked(name)
			}
			if !depsOK {
				continue // 依赖仍缺失，等待后续注入
			}
			switch entry.Type {
			case "llm":
				if err := m.loadProviderDeclarativeLocked(entry); err != nil {
					m.transitionLocked(name, StateFailed, err.Error())
					return fmt.Errorf("failed to promote core %s: %w", name, err)
				}
			case "tool", "policy":
				if m.broker == nil {
					return fmt.Errorf("broker not available, cannot promote core %s", name)
				}
				if err := m.loadPluginWithBroker(entry, m.broker); err != nil {
					m.transitionLocked(name, StateFailed, err.Error())
					return fmt.Errorf("failed to promote core %s: %w", name, err)
				}
			case "agent":
				// agent 已拉起，仅做依赖注入与激活。reactivateAgentLocked 在 LLM 依赖
				// 未就绪时是幂等空操作，此时必须保留待办以便后续注入再次尝试，不能仅因
				// “依赖判定通过”就移出 pendingEntries（否则激活失败后会永久丢失再激活机会）。
				m.reactivateAgentLocked(name)
				if !m.isPendingLocked(name) { // 真正激活（离开 PENDING）后才移出待办
					delete(m.pendingEntries, name)
					progressed = true
				}
				continue
			case "dsc":
				// dsc 类型无服务挂载，依赖就绪即视为已加载完成
			}
			delete(m.pendingEntries, name)
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return nil
}

// reactivateAgentLocked 若 agent 处于 PENDING 且其能力依赖已就绪（特别是 LLM 能力
// 已解析到具体 provider 且 provider 已加载），则重新注入 RegisterServices
// （LLM + 聚合 Tool 服务）并置为 Active（需已持有 m.mu）。
//
// agent 的 primary LLM 选择：从 m.resolvedDeps[name] 中找到首个 Type=="llm" 且
// ProviderName 非空的项，以其 ProviderName 作为 primary LLM。若有多条 llm 依赖
// （罕见），取按 Capability 名升序的首个（稳定选择）。
func (m *Manager) reactivateAgentLocked(name string) {
	st := m.states[name]
	if st == nil || st.State != StatePending {
		return
	}
	// 从运行时态解析结果中找 primary LLM
	deps, ok := m.resolvedDeps[name]
	if !ok {
		return
	}
	primaryLLM := pickPrimaryLLM(deps)
	if primaryLLM == "" {
		return // LLM 依赖尚未解析到 provider
	}
	// 多 provider 路由：确保聚合 LLM 服务已挂载（primary 更新为声明的 provider），
	// primary 就绪后才激活
	llmID, err := m.serveAggregateLLMLocked(primaryLLM)
	if err != nil {
		m.logger.Warn("aggregate llm service unavailable on reactivation", "error", err)
		return
	}
	if _, ok := m.llms[primaryLLM]; !ok {
		return // primary LLM 依赖仍未就绪
	}
	agent, ok := m.agents[name]
	if !ok {
		m.logger.Warn("cannot reactivate agent (no instance)", "name", name)
		return
	}
	// 若存在工具插件但聚合 Tool 服务尚未挂载（启动时无工具、后注入工具），补齐后再注入
	if len(m.toolServiceIDs) > 0 && m.agentToolServiceID == 0 {
		if id, err := m.serveAggregateToolLocked(); err == nil {
			m.agentToolServiceID = id
		} else {
			m.logger.Warn("aggregate tool service unavailable on reactivation", "error", err)
		}
	}
	if err := agent.RegisterServices(context.Background(), llmID, m.agentToolServiceID); err != nil {
		m.logger.Warn("agent reactivation failed", "name", name, "error", err)
		return
	}
	m.agentServiceIDs[name] = llmID
	m.agentLLMName = primaryLLM
	m.transitionLocked(name, StateActive, "")
	m.logger.Info("agent reactivated after capability dependency injection", "name", name, "llmID", llmID, "primaryLLM", primaryLLM)
}

// pickPrimaryLLM 从 ResolvedDep 列表中选首个 Type=="llm" 且 ProviderName 非空的项，
// 返回其 ProviderName。多条 llm 依赖时按 Capability 名升序取首个（稳定选择）。
// 无匹配返回空串。
func pickPrimaryLLM(deps []ResolvedDep) string {
	var candidates []ResolvedDep
	for _, d := range deps {
		if d.Type == "llm" && d.ProviderName != "" {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	// 按 Capability 名升序稳定排序（AGENTS.md 第6条：进模型请求前缀的有序产物须显式排序）
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j-1].Capability > candidates[j].Capability; j-- {
			candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
		}
	}
	return candidates[0].ProviderName
}
