package plugin

// stopHooks 对称清理 hook 的机制。
//
// 进程式插件由宿主 Kill 子进程即消亡，无法真正在插件进程内执行对称的 Start/Stop 回调。
// 作为向 DSH(进程内 Cordis) 语义的映射，将「可逆清理」收敛到宿主侧：插件加载成功时，
// 为其注册一组清理 hook（撤销工具注册、释放 serviceID、优雅关闭 agent 等）；
// 卸载/热重载时按注册顺序执行这些 hook，完成对称清理后再 Kill 子进程，取代各处
// 散落的"kill 即销毁"逻辑。

// addStopHookLocked 为插件注册一个对称清理 hook（需已持有 m.mu）。
// 多个 hook 按注册顺序执行；插件删除时随 m.stopHooks 一并清除。
func (m *Manager) addStopHookLocked(name string, fn func() error) {
	m.stopHooks[name] = append(m.stopHooks[name], fn)
}

// runStopHooksLocked 按注册顺序执行插件的全部清理 hook 并清除（需已持有 m.mu）。
// 单个 hook 失败仅告警，不阻止后续清理与进程终止，保证卸载始终能进行。
func (m *Manager) runStopHooksLocked(name string) {
	hooks := m.stopHooks[name]
	for _, fn := range hooks {
		if err := fn(); err != nil {
			m.logger.Warn("plugin stop hook failed", "name", name, "error", err)
		}
	}
	delete(m.stopHooks, name)
}

// unregisterPluginToolsLocked 撤销某 tool 插件注册的全部工具，并清理相关索引（需已持有 m.mu）。
// 供 tool 插件的 stopHook 调用，作为其卸载时的对称清理。
func (m *Manager) unregisterPluginToolsLocked(name string) {
	if toolNames, ok := m.pluginToolNames[name]; ok {
		for _, toolName := range toolNames {
			m.toolRegistry.Unregister(toolName)
			delete(m.toolNameToServiceID, toolName)
			m.logger.Info("unregistered tool", "tool", toolName, "plugin", name)
		}
	}
	delete(m.toolServiceIDs, name)
	delete(m.pluginToolNames, name)
	delete(m.toolClients, name)
}
