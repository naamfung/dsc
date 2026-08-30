package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// 本文件提供 `dsc setup` 向导的配置写回辅助：以 yaml.Node 文档级操作更新
// config.yaml，只改动目标字段，保留文件其余内容与注释（与动态注入/卸载共用
// 同一套持久化约定，见 config_persist.go）。setup 是「启动前」命令，不持有
// Manager 实例，故这些函数独立于 Manager 方法、不依赖 m.mu。

// setMappingKeyValue 在 mapping 节点中设置/创建 key->value 标量对（含 tag），
// 值未变化时返回 false，避免无意义写回。
func setMappingKeyValue(m *yaml.Node, key, value, tag string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			if m.Content[i+1].Value == value && m.Content[i+1].Tag == tag {
				return false
			}
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
			return true
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
	return true
}

// findEntryInSeq 在 plugins 序列中返回 name 匹配的条目映射节点；不存在返回 nil。
func findEntryInSeq(seq *yaml.Node, name string) *yaml.Node {
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(item.Content); i += 2 {
			if item.Content[i].Value == "name" && item.Content[i+1].Value == name {
				return item
			}
		}
	}
	return nil
}

// findMappingField 返回 mapping 节点中指定键的值节点；不存在返回 nil。
func findMappingField(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// UpsertPluginEnv 更新 config.yaml plugins 序列中 name 条目的 env 映射：
// 已存在键更新、缺失键新增；条目本身不存在时按 name/type 与默认 binary_path
// 创建（并置 enabled）。文件其余内容与注释保留。返回是否发生变更。
func UpsertPluginEnv(path, name, setType string, env map[string]string) (bool, error) {
	doc, err := readConfigDocumentNode(path)
	if err != nil {
		return false, err
	}
	root := configMappingNode(doc)
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	seq := pluginsSequenceNode(root)

	entry := findEntryInSeq(seq, name)
	changed := false
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		changed = true
		setMappingKeyValue(entry, "name", name, "!!str")
		setMappingKeyValue(entry, "type", setType, "!!str")
		setMappingKeyValue(entry, "binary_path", "./plugins/"+name+"/"+name+".exe", "!!str")
		setMappingKeyValue(entry, "enabled", "true", "!!bool")
		seq.Content = append(seq.Content, entry)
	}
	envNode := findMappingField(entry, "env")
	if envNode == nil {
		envNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		entry.Content = append(entry.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "env"}, envNode)
		changed = true
	}
	for k, v := range env {
		if setMappingKeyValue(envNode, k, v, "!!str") {
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := saveConfigNode(path, doc); err != nil {
		return false, fmt.Errorf("setup persist config: %w", err)
	}
	return true, nil
}

// SetConfigStringField 设置 config.yaml 根映射的字符串字段（如 default_llm），
// 保留文件其余内容与注释。
func SetConfigStringField(path, key, value string) error {
	doc, err := readConfigDocumentNode(path)
	if err != nil {
		return fmt.Errorf("setup read config: %w", err)
	}
	root := configMappingNode(doc)
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	setRootField(root, key, value)
	if err := saveConfigNode(path, doc); err != nil {
		return fmt.Errorf("setup persist config: %w", err)
	}
	return nil
}

// SetPluginEnabled 设置 config.yaml plugins 序列中 name 条目的 enabled 字段
// （setup 向导停用未勾选提供商用）；条目不存在时返回 nil（幂等）。文件其余
// 内容与注释保留。
func SetPluginEnabled(path, name string, enabled bool) error {
	doc, err := readConfigDocumentNode(path)
	if err != nil {
		return fmt.Errorf("setup read config: %w", err)
	}
	root := configMappingNode(doc)
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	seq := findPluginsSequence(root)
	if seq == nil {
		return nil
	}
	entry := findEntryInSeq(seq, name)
	if entry == nil {
		return nil
	}
	tag := "!!bool"
	value := "false"
	if enabled {
		value = "true"
	}
	setMappingKeyValue(entry, "enabled", value, tag)
	return saveConfigNode(path, doc)
}

// LoadLLMPluginEnvs 读取 config.yaml 中 type=llm 条目的 (name -> env) 映射，
// 供 setup 向导显示当前值；文件缺失或无 LLM 条目时返回空映射（不报错）。
func LoadLLMPluginEnvs(path string) (map[string]map[string]string, error) {
	result := map[string]map[string]string{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return result, nil
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	for _, p := range cfg.Plugins {
		if p.Type == "llm" && p.Enabled {
			env := map[string]string{}
			for k, v := range p.Env {
				env[k] = v
			}
			result[p.Name] = env
		}
	}
	return result, nil
}
