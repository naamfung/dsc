package plugin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// 动态注入/卸载的配置持久化：让 config.yaml 始终是运行态插件的唯一事实来源，
// 动态注入/卸载都同步写回，保证进程重启后保留运行期增删的插件。
// 此处为包级辅助函数，由 inject.go 与 manager.go 在持有 m.mu 的情况下调用（不在此加锁）。
//
// 写回采用 yaml.Node 文档级操作（而非整体序列化 Config 结构体）：
// 仅增改 plugins 序列，保留文件其余内容（注释、未声明字段的缺省语义），
// 避免把 WorkspaceProtectionEnabled 等零值字段补齐进 config.yaml。

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

// readConfigDocumentNode 读取 config.yaml 为 yaml 文档节点；文件不存在返回 (nil, nil)。
func readConfigDocumentNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// configMappingNode 返回文档的根映射节点；无文档时新建空根映射。
func configMappingNode(doc *yaml.Node) *yaml.Node {
	if doc != nil && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// findPluginsSequence 在根映射中查找 plugins 序列节点；不存在返回 nil。
func findPluginsSequence(root *yaml.Node) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "plugins" {
			return root.Content[i+1]
		}
	}
	return nil
}

// pluginsSequenceNode 定位 plugins 序列节点，缺失时创建并挂载到根映射。
func pluginsSequenceNode(root *yaml.Node) *yaml.Node {
	if seq := findPluginsSequence(root); seq != nil {
		return seq
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "plugins"}, seq)
	return seq
}

// entryToNode 将 PluginEntry 序列化为 yaml 映射节点，剔除空值键
// （depends_on 为 null、env 为空映射时不写入，保持写回内容干净）。
func entryToNode(e PluginEntry) (*yaml.Node, error) {
	data, err := yaml.Marshal(e)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, err
	}
	if len(n.Content) == 0 {
		return nil, fmt.Errorf("empty entry node for %s", e.Name)
	}
	m := n.Content[0]
	kept := m.Content[:0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		v := m.Content[i+1]
		if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
			continue
		}
		if (v.Kind == yaml.SequenceNode || v.Kind == yaml.MappingNode) && len(v.Content) == 0 {
			continue
		}
		kept = append(kept, m.Content[i], v)
	}
	m.Content = kept
	return m, nil
}

// entryNameFromNode 返回 plugins 序列中条目的 name 字段值；非映射或缺失返回空串。
func entryNameFromNode(n *yaml.Node) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == "name" {
			return n.Content[i+1].Value
		}
	}
	return ""
}

// saveConfigNode 以 2 空格缩进将 yaml 文档节点写回文件（保留注释等原始信息）。
func saveConfigNode(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// mutatePluginsSessionLocked 读取 config.yaml，定位（必要时创建）plugins 序列，
// 交由 mutate 闭包对序列做变换；仅当 changed=true 时写回文件并返回 true。
// 共享「读配置 → 定位 plugins 序列 → 变换 → 写回」流程（注入与移除两处复用）。
// createMissing 决定文档不存在时是否新建；否则直接返回未变化。需已持有 m.mu。
func (m *Manager) mutatePluginsSessionLocked(createMissing bool, mutate func(root, seq *yaml.Node) (changed bool, err error)) (bool, error) {
	path := m.persistConfigPath()
	doc, err := readConfigDocumentNode(path)
	if err != nil {
		return false, err
	}
	root := configMappingNode(doc)
	if doc == nil {
		if !createMissing {
			return false, nil
		}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	seq := pluginsSequenceNode(root)
	changed, err := mutate(root, seq)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	return true, saveConfigNode(path, doc)
}

// persistInjectionLocked 将注入的插件条目合并写回 config.yaml（同名覆盖，其余条目原样保留）。
// 动态注入默认启用条目（Enabled=true）。需已持有 m.mu。
func (m *Manager) persistInjectionLocked(entry PluginEntry) error {
	entry.Enabled = true
	_, err := m.mutatePluginsSessionLocked(true, func(root, seq *yaml.Node) (bool, error) {
		entryNode, err := entryToNode(entry)
		if err != nil {
			return false, err
		}
		for i, item := range seq.Content {
			if entryNameFromNode(item) == entry.Name {
				seq.Content[i] = entryNode
				return true, nil
			}
		}
		seq.Content = append(seq.Content, entryNode)
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("persist config for injection: %w", err)
	}
	m.logger.Info("injected plugin persisted", "name", entry.Name, "type", entry.Type, "config", m.persistConfigPath())
	return nil
}

// persistRemovalLocked 从 config.yaml 的 plugins 序列中移除指定插件条目。
// 文件缺失、无 plugins 序列或条目不存在时视为幂等成功（支持重复卸载）。需已持有 m.mu。
func (m *Manager) persistRemovalLocked(name string) error {
	changed, err := m.mutatePluginsSessionLocked(false, func(root, seq *yaml.Node) (bool, error) {
		kept := seq.Content[:0]
		removed := false
		for _, item := range seq.Content {
			if entryNameFromNode(item) == name {
				removed = true
				continue
			}
			kept = append(kept, item)
		}
		if !removed {
			return false, nil
		}
		seq.Content = kept
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("persist config for removal: %w", err)
	}
	if changed {
		m.logger.Info("removed plugin persisted", "name", name, "config", m.persistConfigPath())
	}
	return nil
}
