package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 宿主内置「Go 插件安装/卸载/列举」模型工具：
// 让模型按统一命名约定自助安装/卸载 Go 插件（对标 install_skill）。
// 安全：严格命名 + 路径安全 + 写 config 前备份 + 「干跑(live 加载)校验成功才落盘」，
// 失败回滚（删除已拷贝目录、config 未被写入），避免模型搞坏配置导致启动失败。

var (
	goPluginNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	goPluginTypes  = map[string]bool{"tool": true, "llm": true, "agent": true, "policy": true, "dsc": true}
)

// goPluginDirBase 返回按命名约定拼出的插件目录基名 <type>-<name>。
func goPluginDirBase(ptype, name string) string { return ptype + "-" + name }

// binExt 返回当前平台的插件可执行文件后缀。
func binExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// execName 按约定插件可执行文件名 = 目录基名 + 平台后缀。例如 tool-filesystem.exe。
func (m *Manager) execName(dirBase string) string { return dirBase + binExt() }

// pluginsRoot 返回插件根目录（ExecDir/plugins）。
func (m *Manager) pluginsRoot() string { return filepath.Join(m.config.ExecDir, "plugins") }

// backupConfig 在改动前把 config.yaml 备份到 config.yaml.<毫秒时间戳>.bak，返回备份路径。
func (m *Manager) backupConfig() (string, error) {
	path := m.persistConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("backup config: read %s: %w", path, err)
	}
	bak := fmt.Sprintf("%s.%d.bak", path, time.Now().UnixMilli())
	if err := os.WriteFile(bak, data, 0644); err != nil {
		return "", fmt.Errorf("backup config: write %s: %w", bak, err)
	}
	return bak, nil
}

// removeGoPluginConfig 仅从 config.yaml 移除条目（幂等）。需自行避免嵌套 m.mu。
func (m *Manager) removeGoPluginConfig(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistRemovalLocked(name)
}

// goPluginToolListPrefix 列出 config.yaml 中声明的插件（用于 list_go_plugins）。
func (m *Manager) listGoPluginConfig() ([]PluginEntry, error) {
	cfg, err := LoadConfig(m.persistConfigPath())
	if err != nil {
		return nil, fmt.Errorf("list plugins: load config: %w", err)
	}
	return cfg.Plugins, nil
}

// ============================================================
// install_go_plugin
// ============================================================

type installGoPluginTool struct{ m *Manager }

func (t *installGoPluginTool) Name() string { return "install_go_plugin" }

func (t *installGoPluginTool) TimeoutMs() int { return 120000 } // 安装+live 加载插件可能较慢

func (t *installGoPluginTool) Description() string {
	return "Install a Go (dsc-sdk) plugin into this DSC instance so it can be used. " +
		"Follow the naming convention: plugin directory must be plugins/<type>-<name>/ and its executable " +
		"must be <type>-<name>.exe, where <type> is one of tool|llm|agent|policy|dsc and <name> uses only " +
		"[A-Za-z0-9_-]. Provide source as a directory already laid out as plugins/<type>-<name>/ (containing " +
		"<type>-<name>.exe), or as a single built <type>-<name>.exe binary. The tool validates naming, backs up " +
		"config.yaml, live-loads the plugin to verify it starts (type/metadata must match), and only then persists " +
		"it to config.yaml so it survives restart. On failure nothing is persisted and the copied directory is removed."
}

func (t *installGoPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"type":{"type":"string","enum":["tool","llm","agent","policy","dsc"],"description":"Plugin type (must match the plugin's own declared type)."},
"name":{"type":"string","description":"Plugin name using [A-Za-z0-9_-]; directory will be plugins/<type>-<name>/."},
"source":{"type":"string","description":"Path to a directory laid out as plugins/<type>-<name>/ (containing <type>-<name>.exe) or to a single <type>-<name>.exe binary."},
"enabled":{"type":"boolean","description":"Whether to mark the plugin enabled in config. Default true."}},
"required":["type","name","source"],"additionalProperties":false}`)
}

func (t *installGoPluginTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Source  string `json:"source"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	if err := t.install(ctx, p.Type, p.Name, p.Source, enabled); err != nil {
		return "", err
	}
	res, _ := json.Marshal(map[string]any{
		"ok":      true,
		"name":    goPluginDirBase(p.Type, p.Name),
		"type":    p.Type,
		"enabled": enabled,
		"note":    "已安装并热加载；已备份原配置，重启后仍生效；卸载可用 uninstall_go_plugin",
	})
	return string(res), nil
}

func (t *installGoPluginTool) install(ctx context.Context, ptype, name, source string, enabled bool) error {
	if !goPluginTypes[ptype] {
		return fmt.Errorf("invalid type %q: must be tool|llm|agent|policy|dsc", ptype)
	}
	if !goPluginNameRe.MatchString(name) || name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name %q: use [A-Za-z0-9_-] only (prevents path traversal)", name)
	}
	dirBase := goPluginDirBase(ptype, name)
	pluginRoot := filepath.Join(t.m.pluginsRoot(), dirBase)

	// 拷贝来源到约定目录
	if err := copyPluginSource(source, pluginRoot, t.m.execName(dirBase)); err != nil {
		return err
	}

	// 备份 config（改动前）
	backup, err := t.m.backupConfig()
	if err != nil {
		_ = os.RemoveAll(pluginRoot)
		return err
	}

	// 干跑 = 真实 live 加载：类型/元数据校验一致才落盘；失败回滚
	entry := PluginEntry{Name: dirBase, Type: ptype, Enabled: enabled,
		BinaryPath: "./plugins/" + dirBase + "/" + t.m.execName(dirBase)}
	if err := t.m.LoadPlugin(entry); err != nil {
		_ = os.RemoveAll(pluginRoot) // 回滚：移除已拷贝目录（config 未写入）
		return fmt.Errorf("插件未能加载（已回滚，config 未改动；备份保留于 %s）: %w", backup, err)
	}
	return nil
}

func (t *installGoPluginTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	v, verr := json.Marshal(ToolView{
		Kind:  "card",
		Title: "InstallGoPlugin",
		Badge: &ViewBadge{Text: "installed", Tone: "green"},
		Fields: []ViewField{
			{Key: "name", Value: extractField(result, "name")},
			{Key: "note", Value: extractField(result, "note")},
		},
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// copyPluginSource 把 source（目录 or 二进制）落到 pluginRoot，并确保执行文件存在。
func copyPluginSource(source, pluginRoot, execFile string) error {
	fi, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("source 不存在: %w", err)
	}
	if fi.IsDir() {
		if err := copyDir(source, pluginRoot); err != nil {
			return fmt.Errorf("拷贝插件目录失败: %w", err)
		}
	} else {
		if err := os.MkdirAll(pluginRoot, 0755); err != nil {
			return err
		}
		if err := copyFile(source, filepath.Join(pluginRoot, execFile)); err != nil {
			return fmt.Errorf("拷贝插件二进制失败: %w", err)
		}
	}
	// 断言约定执行文件齐备（目录来源须含 <type>-<name><ext>）
	if st, err := os.Stat(filepath.Join(pluginRoot, execFile)); err != nil || st.IsDir() {
		return fmt.Errorf("插件目录 %s 缺少约定执行文件 %s（须含 <type>-<name><ext>）", pluginRoot, execFile)
	}
	return nil
}

// ============================================================
// uninstall_go_plugin
// ============================================================

type uninstallGoPluginTool struct{ m *Manager }

func (t *uninstallGoPluginTool) Name() string { return "uninstall_go_plugin" }

func (t *uninstallGoPluginTool) Description() string {
	return "Uninstall a Go plugin by its config entry name (the directory basename like tool-filesystem or dsc-notify). " +
		"Backs up config.yaml, removes the plugin from config so it won't load on restart, live-unloads it, " +
		"and optionally deletes its directory under plugins/. Pass delete_dir=true to also remove the files."
}

func (t *uninstallGoPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"Config entry name = directory basename (e.g. tool-filesystem, dsc-notify)."},
"delete_dir":{"type":"boolean","description":"Also delete plugins/<name>/ directory. Default false."}},
"required":["name"],"additionalProperties":false}`)
}

func (t *uninstallGoPluginTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name      string `json:"name"`
		DeleteDir bool   `json:"delete_dir"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if !goPluginNameRe.MatchString(p.Name) || p.Name == "" || p.Name == "." || p.Name == ".." {
		return "", fmt.Errorf("invalid name %q", p.Name)
	}
	if _, err := t.m.backupConfig(); err != nil {
		return "", err
	}
	// live 卸载（含持久化移除）；若当前未加载则仅清除 config 条目
	if err := t.m.UnloadPlugin(p.Name); err != nil {
		if perr := t.m.removeGoPluginConfig(p.Name); perr != nil {
			return "", fmt.Errorf("卸载插件: %v; 且清 config 失败: %v", err, perr)
		}
	} else if err := t.m.removeGoPluginConfig(p.Name); err != nil {
		return "", fmt.Errorf("清 config 失败: %w", err)
	}
	t.m.removeHookClient(p.Name)
	if p.DeleteDir {
		if err := os.RemoveAll(filepath.Join(t.m.pluginsRoot(), p.Name)); err != nil {
			return "", fmt.Errorf("删除插件目录失败: %w", err)
		}
	}
	res, _ := json.Marshal(map[string]any{
		"ok":         true,
		"name":       p.Name,
		"delete_dir": p.DeleteDir,
		"note":       "已从 config 移除并（若已加载）热卸载；重启后不再加载",
	})
	return string(res), nil
}

func (t *uninstallGoPluginTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	v, verr := json.Marshal(ToolView{
		Kind:  "card",
		Title: "UninstallGoPlugin",
		Badge: &ViewBadge{Text: "removed", Tone: "yellow"},
		Fields: []ViewField{
			{Key: "name", Value: extractField(result, "name")},
			{Key: "note", Value: extractField(result, "note")},
		},
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// removeHookClient 卸载时同步注销该插件的事件订阅 hook client（避免残留指向已杀进程）。
func (m *Manager) removeHookClient(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.toolHookClients[name]; ok {
		delete(m.toolHookClients, name)
	}
	out := m.toolHookOrder[:0]
	for _, n := range m.toolHookOrder {
		if n != name {
			out = append(out, n)
		}
	}
	m.toolHookOrder = out
}

// ============================================================
// list_go_plugins
// ============================================================

type listGoPluginsTool struct{ m *Manager }

func (t *listGoPluginsTool) Name() string { return "list_go_plugins" }

func (t *listGoPluginsTool) Description() string {
	return "List the Go plugins declared in config.yaml for this DSC instance (name, type, enabled, binary_path)."
}

func (t *listGoPluginsTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (t *listGoPluginsTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	entries, err := t.m.listGoPluginConfig()
	if err != nil {
		return "", err
	}
	type row struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Enabled    bool   `json:"enabled"`
		BinaryPath string `json:"binary_path,omitempty"`
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{Name: e.Name, Type: e.Type, Enabled: e.Enabled, BinaryPath: e.BinaryPath})
	}
	b, _ := json.Marshal(map[string]any{"plugins": rows, "count": len(rows)})
	return string(b), nil
}

func (t *listGoPluginsTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	var r struct {
		Plugins []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Enabled bool   `json:"enabled"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return result, "", nil
	}
	rows := make([]ViewRow, 0, len(r.Plugins))
	for _, p := range r.Plugins {
		rows = append(rows, ViewRow{"name": p.Name, "type": p.Type, "enabled": fmt.Sprint(p.Enabled)})
	}
	v, verr := json.Marshal(ToolView{
		Kind:    "table",
		Title:   "GoPlugins",
		Columns: []ViewColumn{{Key: "name", Title: "插件"}, {Key: "type", Title: "类型"}, {Key: "enabled", Title: "启用"}},
		Rows:    rows,
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// ============================================================
// 工具集合 + 拷贝辅助
// ============================================================

// goPluginTools 返回宿主内置的 Go 插件管理工具。
func (m *Manager) goPluginTools() []ToolDefinition {
	return []ToolDefinition{
		&installGoPluginTool{m: m},
		&uninstallGoPluginTool{m: m},
		&listGoPluginsTool{m: m},
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// extractField 从扁平 JSON 结果中取字段（供视图展示）。
func extractField(result, key string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%v", v), "0"), ".")
	}
	return ""
}
