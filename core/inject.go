package core

import (
	"context"
	"fmt"
)

// 动态注入插件的端到端能力。
//
// 相比仅拉起子进程，动态注入还补齐三个闭环环节：
//  1. 依赖判定：注入条目的声明式依赖未满足时置为 PENDING 等待（而非直接失败/Active）；
//  2. 声明持久化：注入（含 PENDING）写回 config.yaml，进程重启后依旧保留；
//  3. 缺失修复与 agent 再激活：注入的 LLM/Tool 可能补足先前缺口，把等待中的 PENDING
//     provider 提升加载，并把因缺 LLM 而 PENDING 的 agent 重新注入 RegisterServices 并激活。

// knownPluginNamesLocked 返回当前已知的所有插件名（已加载 + 待办中的），
// 用于把声明式依赖中指向「插件名」的引用与「具体工具名」区分开。
func (m *Manager) knownPluginNamesLocked() map[string]bool {
	set := make(map[string]bool, len(m.typeMap)+len(m.agents)+len(m.pendingEntries))
	for n := range m.typeMap {
		set[n] = true
	}
	for n := range m.agents {
		set[n] = true
	}
	for n := range m.pendingEntries {
		set[n] = true
	}
	return set
}

// depSatisfiedLocked 判断名为 name 的插件当前提供的依赖是否已就绪可用。
func (m *Manager) depSatisfiedLocked(name string) bool {
	if _, ok := m.llmServiceIDs[name]; ok {
		return true
	}
	if _, ok := m.toolServiceIDs[name]; ok {
		return true
	}
	if _, ok := m.agents[name]; ok {
		return true
	}
	return false
}

// entryDepsSatisfiedLocked 判断注入条目的声明式依赖是否全部满足；无声明依赖视为满足。
func (m *Manager) entryDepsSatisfiedLocked(entry PluginEntry) bool {
	for _, d := range declaredPluginDeps(entry, m.knownPluginNamesLocked()) {
		if !m.depSatisfiedLocked(d) {
			return false
		}
	}
	return true
}

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
	m.markPendingLocked(entry.Name, entry.Type, "dynamic dependency not satisfied")
	m.pendingEntries[entry.Name] = entry
	if err := m.persistInjectionLocked(entry); err != nil {
		m.logger.Warn("persist pending injection failed", "name", entry.Name, "error", err)
	}
	m.logger.Info("core deferred to pending (dependency not satisfied)", "name", entry.Name)
	return nil
}

// injectionEntryLocked 注入单个插件（需已持有 m.mu）：
//   - agent 作为 broker 提供者先拉起进程但不立即激活；依赖满足则直接激活，否则置 PENDING；
//   - llm/tool/policy 按类型加载，依赖未满足则进入 PENDING；
//   - 加载/挂起后持久化声明，并触发 repairPendingLocked 修复其他等待中的插件。
func (m *Manager) injectionEntryLocked(entry PluginEntry) error {
	switch entry.Type {
	case "agent":
		// agent 提供 broker；loadAgentAndGetBroker 拉起进程、注册 stop hook 并记录 agentEntries
		if _, _, err := m.loadAgentAndGetBroker(entry); err != nil {
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return err
		}
		if !m.entryDepsSatisfiedLocked(entry) {
			m.markPendingLocked(entry.Name, "agent", "dependency not satisfied")
			m.pendingEntries[entry.Name] = entry
		} else {
			m.reactivateAgentLocked(entry.Name) // 依赖已满足则直接注入并激活
		}
	case "llm", "tool", "policy", "dsc":
		if m.broker == nil {
			return fmt.Errorf("broker not available, cannot inject core type %s", entry.Type)
		}
		if !m.entryDepsSatisfiedLocked(entry) {
			return m.deferPendingLocked(entry)
		}
		if err := m.loadProviderDeclarativeLocked(entry); err != nil {
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return err
		}
	default:
		return fmt.Errorf("unsupported core type for injection: %s", entry.Type)
	}

	// 声明持久化：注入（含 PENDING）写回 config.yaml，保证重启保留
	if err := m.persistInjectionLocked(entry); err != nil {
		m.logger.Warn("persist injection failed", "name", entry.Name, "error", err)
	}

	// 修复：本次注入可能补足了先前的缺口，提升等待中的 provider / 再激活 PENDING agent
	return m.repairPendingLocked()
}

// repairPendingLocked 扫描所有 PENDING 插件并尽力提升（需已持有 m.mu）：
//   - provider(llm/tool/policy)：依赖就绪则按类型实际加载（Pending→Connecting→…→Active）；
//   - agent：DependsOn.LLM 就绪则重新注入 RegisterServices 并置为 Active。
//
// 反复迭代直至无新增提升，以覆盖链式依赖。
func (m *Manager) repairPendingLocked() error {
	for {
		progressed := false
		for name, entry := range m.pendingEntries {
			// 仅对 provider 生效：非 agent 条目若已被其他路径加载，仅清除待办。
			// agent 必然已随 LoadFromConfig 拉起（作为 broker 提供者），“已加载”不代表已激活，
			// 必须继续走到 reactivateAgentLocked 做依赖注入，否则会漏掉 PENDING agent 的再激活。
			if entry.Type != "agent" && m.coreLoadedLocked(name) {
				delete(m.pendingEntries, name)
				progressed = true
				continue
			}
			if !m.entryDepsSatisfiedLocked(entry) {
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

// reactivateAgentLocked 若 agent 处于 PENDING 且其 DependsOn.LLM 已就绪，则重新注入
// RegisterServices（LLM + 聚合 Tool 服务）并置为 Active（需已持有 m.mu）。
func (m *Manager) reactivateAgentLocked(name string) {
	st := m.states[name]
	if st == nil || st.State != StatePending {
		return
	}
	entry, ok := m.agentEntries[name]
	if !ok || entry.DependsOn == nil || entry.DependsOn.LLM == "" {
		return
	}
	// 多 provider 路由：确保聚合 LLM 服务已挂载（primary 更新为声明的 provider），
	// primary 就绪后才激活
	llmID, err := m.serveAggregateLLMLocked(entry.DependsOn.LLM)
	if err != nil {
		m.logger.Warn("aggregate llm service unavailable on reactivation", "error", err)
		return
	}
	if _, ok := m.llms[entry.DependsOn.LLM]; !ok {
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
	m.transitionLocked(name, StateActive, "")
	m.logger.Info("agent reactivated after dependency injection", "name", name, "llmID", llmID)
}
