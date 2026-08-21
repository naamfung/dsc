package plugin

import (
	"fmt"
	"os"
)

// 第 4 步「动态注入插件」的配置持久化：让 config.yaml 始终是运行态插件的唯一事实来源，
// 动态注入/卸载都同步写回，保证进程重启后保留运行期增删的插件。
// 此处为包级辅助函数，由 inject.go 与 manager.go 在持有 m.mu 的情况下调用（不在此加锁）。

// persistConfigPath 返回动态注入/卸载要写回的 config.yaml 路径：
// 优先使用 Manager.configPath（由 main 通过 SetConfigPath 注入），退回包级 ConfigPath。
func (m *Manager) persistConfigPath() string {
	if m.configPath != "" {
		return m.configPath
	}
	if ConfigPath != "" {
		return ConfigPath
	}
	return "./config/config.yaml"
}

// persistInjectionLocked 将注入的插件条目合并写回 config.yaml（同名覆盖，其余条目原样保留）。
// 需已持有 m.mu。
func (m *Manager) persistInjectionLocked(entry PluginEntry) error {
	path := m.persistConfigPath()
	cfg, err := LoadConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read config for injection: %w", err)
		}
		cfg = new(Config) // 配置文件不存在，以注入条目创建新配置
	}
	if cfg == nil {
		cfg = new(Config)
	}
	entry.Enabled = true
	upserted := false
	for i := range cfg.Plugins {
		if cfg.Plugins[i].Name == entry.Name {
			cfg.Plugins[i] = entry
			upserted = true
			break
		}
	}
	if !upserted {
		cfg.Plugins = append(cfg.Plugins, entry)
	}
	if err := SaveConfig(path, cfg); err != nil {
		return fmt.Errorf("save config after injection: %w", err)
	}
	m.logger.Info("injected plugin persisted", "name", entry.Name, "type", entry.Type, "config", path)
	return nil
}

// persistRemovalLocked 从 config.yaml 中移除指定插件条目。
// 条目不存在时视为幂等成功（支持重复卸载）。需已持有 m.mu。
func (m *Manager) persistRemovalLocked(name string) error {
	path := m.persistConfigPath()
	cfg, err := LoadConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config for removal: %w", err)
	}
	if cfg == nil {
		return nil
	}
	kept := cfg.Plugins[:0]
	removed := false
	for _, e := range cfg.Plugins {
		if e.Name == name {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return nil
	}
	cfg.Plugins = kept
	if err := SaveConfig(path, cfg); err != nil {
		return fmt.Errorf("save config after removal: %w", err)
	}
	m.logger.Info("removed plugin persisted", "name", name, "config", path)
	return nil
}