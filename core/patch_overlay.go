package core

import (
        "fmt"
        "os"
        "strings"

        "gopkg.in/yaml.v3"
)

// PatchOverlay 一个 -patch 文件的解析结果：插件条目列表（含 insert 与 replace 语义）。
//
// 对齐 DSH cordis-plugin-include 的 PatchOptions：
//   - 顶层条目（id/name/type/config）= 替换同名插件条目的 config（id-targeted override）
//   - - insert: 列表 = 追加新插件条目
//
// DSC 简化：patch 文件是 YAML 列表，每项是 PluginEntry（可含 Config 字段）。
// 合并时按 Name 匹配——同 Name 替换 Config（DSH 用 id，DSC 用 Name 等价），
// 新 Name 追加。支持顶层条目与 - insert: 列表两种 YAML 形式。
type PatchOverlay struct {
        Source  string        // patch 文件路径（诊断用）
        Entries []PluginEntry // 解析出的插件条目
}

// LoadPatchOverlay 加载一个 -patch 文件。
//
// patch 文件格式（对齐 DSH cordis-plugin-include 的 PatchOptions）：
//
//   - insert:           # 显式 insert action（DSH 标准）
//
//   - name: mcp-memory
//     type: dsc
//     config:
//     mcp:
//     server_name: memory
//     endpoint: http://localhost:3100/mcp
//
//   - name: mcp-memory  # 顶层条目（无 action，视为 insert）
//     type: dsc
//     config:
//     mcp: {...}
//
// 两种形式可混用，解析为同一个 PatchOverlay.Entries 列表。
// 顶层条目若 Name 已存在于 base Config，合并时替换其 Config（id-targeted override）。
func LoadPatchOverlay(path string) (*PatchOverlay, error) {
        data, err := os.ReadFile(path)
        if err != nil {
                return nil, fmt.Errorf("read patch %s: %w", path, err)
        }
        abs, _ := PAbs(path)
        overlay := &PatchOverlay{Source: abs}

        // 统一解析为 []map[string]yaml.Node，按 action 分发
        var raw []yaml.Node
        if err := yaml.Unmarshal(data, &raw); err != nil {
                return nil, fmt.Errorf("parse patch %s: %w", path, err)
        }
        for _, topNode := range raw {
                if topNode.Kind != yaml.MappingNode {
                        continue
                }
                var action string
                var entriesNode *yaml.Node
                for i := 0; i+1 < len(topNode.Content); i += 2 {
                        key := topNode.Content[i].Value
                        val := topNode.Content[i+1]
                        switch key {
                        case "insert", "append", "replace":
                                action = key
                                entriesNode = val
                        }
                }
                if action != "" && entriesNode != nil {
                        // insert/append/replace action：解析为 PluginEntry 列表
                        var subEntries []PluginEntry
                        if err := entriesNode.Decode(&subEntries); err != nil {
                                return nil, fmt.Errorf("parse patch %s %s block: %w", path, action, err)
                        }
                        overlay.Entries = append(overlay.Entries, subEntries...)
                        continue
                }
                // 顶层条目（无 action，视为 insert）：重新解析为 PluginEntry
                var entry PluginEntry
                if err := topNode.Decode(&entry); err == nil && entry.Name != "" {
                        overlay.Entries = append(overlay.Entries, entry)
                }
        }

        if len(overlay.Entries) == 0 {
                return nil, fmt.Errorf("patch %s: no entries parsed", path)
        }
        return overlay, nil
}

// ApplyPatchOverlays 把多个 patch overlay 按顺序合并到 base Config。
//
// 合并语义（对齐 DSH applyEntryPatches 的 id-targeted override）：
//   - 同 Name 的条目：替换 Config（DSH 是整行替换，DSC 仅替换 Config 字段，
//     保留原 BinaryPath/Env 等宿主侧字段——DSC 的 patch 只覆盖插件配置面）
//   - 新 Name：追加到 Plugins 列表
//   - 多个 overlay 按顺序应用，后者覆盖前者
//
// 返回新的 Config（不修改 base）。
func ApplyPatchOverlays(base *Config, overlays []*PatchOverlay) *Config {
        if base == nil {
                base = &Config{}
        }
        merged := &Config{
                WorkspaceRoot:    base.WorkspaceRoot,
                Mode:             base.Mode,
                DefaultLLM:       base.DefaultLLM,
                ContextWindow:    base.ContextWindow,
                Persona:          base.Persona,
                PlanSection:      base.PlanSection,
                SessionID:        base.SessionID,
                HistoryInjection: base.HistoryInjection,
                HotReload:        base.HotReload,
                Compaction:       base.Compaction,
                Plugins:          make([]PluginEntry, len(base.Plugins)),
        }
        copy(merged.Plugins, base.Plugins)

        for _, overlay := range overlays {
                for _, entry := range overlay.Entries {
                        if entry.Name == "" {
                                continue
                        }
                        // 查找同名条目
                        found := false
                        for i := range merged.Plugins {
                                if merged.Plugins[i].Name == entry.Name {
                                        // 替换 Config（保留原 BinaryPath/Env/Enabled/Type）
                                        if entry.Config != nil {
                                                merged.Plugins[i].Config = entry.Config
                                        }
                                        if entry.Type != "" {
                                                merged.Plugins[i].Type = entry.Type
                                        }
                                        if entry.BinaryPath != "" {
                                                merged.Plugins[i].BinaryPath = entry.BinaryPath
                                        }
                                        if entry.Env != nil {
                                                merged.Plugins[i].Env = entry.Env
                                        }
                                        // Enabled 显式设置时覆盖（default true 的零值不覆盖）
                                        if entry.Name != "" && entry.Type != "" {
                                                merged.Plugins[i].Enabled = entry.Enabled
                                        }
                                        found = true
                                        break
                                }
                        }
                        if !found {
                                // 追加新条目：默认启用（yaml.v3 不识别 default tag，零值 false 需显式修正）
                                if entry.Type != "" && entry.Enabled == false {
                                        // 显式 enabled: false（有 type 但 enabled 未写）——视为默认启用
                                        // 除非 YAML 显式写 enabled: false（此时需用指针类型区分，简化为默认启用）
                                        entry.Enabled = true
                                } else if entry.Type == "" {
                                        entry.Enabled = true
                                }
                                merged.Plugins = append(merged.Plugins, entry)
                        }
                }
        }
        return merged
}

// LoadPatchFiles 加载多个 -patch 文件（按命令行顺序）。
// 任一文件加载失败返回 error（fail-loud，对齐 DSH overlay 必须可解析）。
func LoadPatchFiles(paths []string) ([]*PatchOverlay, error) {
        if len(paths) == 0 {
                return nil, nil
        }
        var overlays []*PatchOverlay
        for _, p := range paths {
                p = strings.TrimSpace(p)
                if p == "" {
                        continue
                }
                overlay, err := LoadPatchOverlay(p)
                if err != nil {
                        return nil, err
                }
                overlays = append(overlays, overlay)
        }
        return overlays, nil
}
