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

	version "github.com/hashicorp/go-version"
)

// 宿主内置「DSC 插件安装/卸载/列举」模型工具：
// 指 DSC 自身的插件（本机二进制 Go 插件，基于 go-plugin/gRPC 实现、经 dsc-sdk 构建），
// 区别于 Go 标准库的 plugin 包。让模型按统一命名约定自助安装/卸载（对标 install_skill）。
// 安全：严格命名 + 路径安全 + 写 config 前备份 + 「干跑(live 加载)校验成功才落盘」，
// 失败回滚（删除已拷贝目录、config 未被写入），避免模型搞坏配置导致启动失败。

var (
	dscPluginNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	dscPluginTypes  = map[string]bool{"tool": true, "llm": true, "agent": true, "policy": true, "dsc": true}
)

// dscPluginDirBase 返回按命名约定拼出的插件目录基名 <type>-<name>。
func dscPluginDirBase(ptype, name string) string { return ptype + "-" + name }

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

// removeDscPluginConfig 仅从 config.yaml 移除条目（幂等）。需自行避免嵌套 m.mu。
func (m *Manager) removeDscPluginConfig(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistRemovalLocked(name)
}

// 列出 config.yaml 中声明的插件（用于 list_dsc_plugins）。
func (m *Manager) listDscPluginConfig() ([]PluginEntry, error) {
	cfg, err := LoadConfig(m.persistConfigPath())
	if err != nil {
		return nil, fmt.Errorf("list plugins: load config: %w", err)
	}
	return cfg.Plugins, nil
}

// ============================================================
// install_dsc_plugin
// ============================================================

type installDscPluginTool struct{ m *Manager }

func (t *installDscPluginTool) Name() string { return "install_dsc_plugin" }

func (t *installDscPluginTool) TimeoutMs() int { return 120000 } // 安装+live 加载插件可能较慢

func (t *installDscPluginTool) Description() string {
	return "Install a DSC plugin (our own binary plugin, a Go program built with the dsc-sdk and loaded via go-plugin/gRPC) into this DSC instance so it can be used. " +
		"Follow the naming convention: plugin directory must be plugins/<type>-<name>/ and its executable " +
		"must be <type>-<name>.exe, where <type> is one of tool|llm|agent|policy|dsc and <name> uses only " +
		"[A-Za-z0-9_-]. Provide source as a directory already laid out as plugins/<type>-<name>/ (containing " +
		"<type>-<name>.exe), or as a single built <type>-<name>.exe binary. The tool validates naming, backs up " +
		"config.yaml, live-loads the plugin to verify it starts (type/metadata must match), and only then persists " +
		"it to config.yaml so it survives restart. On failure nothing is persisted and the copied directory is removed."
}

func (t *installDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"type":{"type":"string","enum":["tool","llm","agent","policy","dsc"],"description":"Plugin type (must match the plugin's own declared type)."},
"name":{"type":"string","description":"Plugin name using [A-Za-z0-9_-]; directory will be plugins/<type>-<name>/."},
"source":{"type":"string","description":"Path to a directory laid out as plugins/<type>-<name>/ (containing <type>-<name>.exe) or to a single <type>-<name>.exe binary."},
"enabled":{"type":"boolean","description":"Whether to mark the plugin enabled in config. Default true."}},
"required":["type","name","source"],"additionalProperties":false}`)
}

func (t *installDscPluginTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
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
		"name":    dscPluginDirBase(p.Type, p.Name),
		"type":    p.Type,
		"enabled": enabled,
		"note":    "已安装并热加载；已备份原配置，重启后仍生效；卸载可用 uninstall_dsc_plugin",
	})
	return string(res), nil
}

func (t *installDscPluginTool) install(ctx context.Context, ptype, name, source string, enabled bool) error {
	if !dscPluginTypes[ptype] {
		return fmt.Errorf("invalid type %q: must be tool|llm|agent|policy|dsc", ptype)
	}
	if !dscPluginNameRe.MatchString(name) || name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name %q: use [A-Za-z0-9_-] only (prevents path traversal)", name)
	}
	dirBase := dscPluginDirBase(ptype, name)
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

func (t *installDscPluginTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	v, verr := json.Marshal(ToolView{
		Kind:  "card",
		Title: "InstallDscPlugin",
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
// uninstall_dsc_plugin
// ============================================================

type uninstallDscPluginTool struct{ m *Manager }

func (t *uninstallDscPluginTool) Name() string { return "uninstall_dsc_plugin" }

func (t *uninstallDscPluginTool) Description() string {
	return "Uninstall a DSC plugin by its config entry name (the directory basename like tool-filesystem or dsc-notify). " +
		"Backs up config.yaml, removes the plugin from config so it won't load on restart, live-unloads it, " +
		"and optionally deletes its directory under plugins/. Pass delete_dir=true to also remove the files."
}

func (t *uninstallDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"Config entry name = directory basename (e.g. tool-filesystem, dsc-notify)."},
"delete_dir":{"type":"boolean","description":"Also delete plugins/<name>/ directory. Default false."}},
"required":["name"],"additionalProperties":false}`)
}

func (t *uninstallDscPluginTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name      string `json:"name"`
		DeleteDir bool   `json:"delete_dir"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if !dscPluginNameRe.MatchString(p.Name) || p.Name == "" || p.Name == "." || p.Name == ".." {
		return "", fmt.Errorf("invalid name %q", p.Name)
	}
	if _, err := t.m.backupConfig(); err != nil {
		return "", err
	}
	// live 卸载（含持久化移除）；若当前未加载则仅清除 config 条目
	if err := t.m.UnloadPlugin(p.Name); err != nil {
		if perr := t.m.removeDscPluginConfig(p.Name); perr != nil {
			return "", fmt.Errorf("卸载插件: %v; 且清 config 失败: %v", err, perr)
		}
	} else if err := t.m.removeDscPluginConfig(p.Name); err != nil {
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

func (t *uninstallDscPluginTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	v, verr := json.Marshal(ToolView{
		Kind:  "card",
		Title: "UninstallDscPlugin",
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
// list_dsc_plugins
// ============================================================

type listDscPluginsTool struct{ m *Manager }

func (t *listDscPluginsTool) Name() string { return "list_dsc_plugins" }

func (t *listDscPluginsTool) Description() string {
	return "List the DSC plugins declared in config.yaml for this DSC instance (name, type, enabled, binary_path)."
}

func (t *listDscPluginsTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (t *listDscPluginsTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	entries, err := t.m.listDscPluginConfig()
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

func (t *listDscPluginsTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
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
		Title:   "DscPlugins",
		Columns: []ViewColumn{{Key: "name", Title: "插件"}, {Key: "type", Title: "类型"}, {Key: "enabled", Title: "启用"}},
		Rows:    rows,
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// ============================================================
// upgrade_dsc_plugin
// ============================================================

type upgradeDscPluginTool struct{ m *Manager }

func (t *upgradeDscPluginTool) Name() string { return "upgrade_dsc_plugin" }

func (t *upgradeDscPluginTool) TimeoutMs() int { return 120000 } // 部署+热更替新进程可能较慢

func (t *upgradeDscPluginTool) Description() string {
	return "Upgrade an already-installed DSC plugin to a new version. " +
		"name is the existing plugin's directory basename (e.g. tool-filesystem); " +
		"version is the new semantic version (e.g. 2.3.1); " +
		"source is the new plugin binary (a single <name>.exe) or a directory laid out as plugins/<name>/ containing <name>.exe. " +
		"The tool deploys the new binary as <name>-v<version>.exe inside the plugin dir and hot-reloads the process to it; " +
		"if hot-reload fails, the new binary is kept so it takes effect on next restart. Works on top of the DSC versioned-binary hot-reload mechanism."
}

func (t *upgradeDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"Existing plugin name = directory basename (e.g. tool-filesystem)."},
"version":{"type":"string","description":"New semantic version, e.g. '2.3.1' or '2.4'."},
"source":{"type":"string","description":"Path to the new plugin binary (single <name>.exe) or to a directory laid out as plugins/<name>/ containing <name>.exe."}},
"required":["name","version","source"],"additionalProperties":false}`)
}

func (t *upgradeDscPluginTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.upgrade(p.Name, p.Version, p.Source)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(res)
	return string(b), nil
}

func (t *upgradeDscPluginTool) upgrade(name, versionStr, source string) (map[string]any, error) {
	if !dscPluginNameRe.MatchString(name) || name == "" || name == "." || name == ".." {
		return nil, fmt.Errorf("invalid name %q: use [A-Za-z0-9_-] only", name)
	}
	pluginDir := filepath.Join(t.m.pluginsRoot(), name)
	st, err := os.Stat(pluginDir)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("插件目录 %s 不存在（请先用 install_dsc_plugin 安装）", pluginDir)
	}

	ext := binExt()
	base := name
	// 校验并解析新版本号
	nv, err := version.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid version %q: must be semver like 2.3.1", versionStr)
	}
	if nv.LessThanOrEqual(version.Must(version.NewVersion("0.0.0"))) {
		return nil, fmt.Errorf("版本号必须大于 0.0.0")
	}
	newFile := filepath.Join(pluginDir, base+"-v"+versionStr+ext)
	if _, err := os.Stat(newFile); err == nil {
		return nil, fmt.Errorf("目标版本 %s 已存在（%s），无需重复升级", versionStr, newFile)
	}

	// 当前运行版本（基线或既有版本化二进制中最高者）
	curBinary := ResolveLatestBinary(filepath.Join(pluginDir, base+ext))
	curV := binaryVersion(curBinary)
	if !nv.GreaterThan(curV) {
		return nil, fmt.Errorf("版本 %s 不高于当前运行版本 %s，不是升级",
			nv.String(), curV.String())
	}

	// 解析源二进制（单文件或目录的约定执行文件）
	srcFile := ""
	si, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("source 不存在: %w", err)
	}
	if si.IsDir() {
		srcFile = filepath.Join(source, base+ext)
		if _, err := os.Stat(srcFile); err != nil {
			return nil, fmt.Errorf("目录 %s 缺少约定执行文件 %s", source, base+ext)
		}
	} else {
		srcFile = source
	}

	// 植入版本化二进制（不覆盖基础文件，供热重载识别更高版本）
	if err := copyFile(srcFile, newFile); err != nil {
		return nil, fmt.Errorf("植入版本化二进制 %s 失败: %w", newFile, err)
	}

	// 触发热更替：验证新进程，成功才交换；失败则保留新文件供重启兜底
	if err := t.m.HotReload(name, newFile); err != nil {
		// 不 return 错误：二进制已置入，重启后依版本规则生效
		return map[string]any{
			"ok":            true,
			"name":          name,
			"version":       versionStr,
			"deployed_file": filepath.ToSlash(newFile),
			"hot_reload":    false,
			"note":          "新版本二进制已置入 " + filepath.ToSlash(newFile) + "，但当下热更替未立即生效（" + err.Error() + "），重启 DSC 后生效",
		}, nil
	}
	return map[string]any{
		"ok":            true,
		"name":          name,
		"version":       versionStr,
		"deployed_file": filepath.ToSlash(newFile),
		"hot_reload":    true,
		"note":          "已升级并热更替为 " + name + "-v" + versionStr + ext + "；原版本文件保留可回退",
	}, nil
}

func (t *upgradeDscPluginTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	v, verr := json.Marshal(ToolView{
		Kind:  "card",
		Title: "UpgradeDscPlugin",
		Badge: &ViewBadge{Text: "upgraded", Tone: "green"},
		Fields: []ViewField{
			{Key: "name", Value: extractField(result, "name")},
			{Key: "version", Value: extractField(result, "version")},
			{Key: "deployed_file", Value: extractField(result, "deployed_file")},
			{Key: "hot_reload", Value: extractField(result, "hot_reload")},
		},
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// ============================================================
// load_dsc_plugin
// ============================================================

// loadDscPluginTool 运行期载入「已存在于 plugins/ 目录、但未在 config 声明」的插件，
// 使模型可直接使用其工具（本进程生效）。走宿主原生聚合 Tool RPC（经 ToolRegistry
// Execute → LoadPlugin），不经 HTTP，也不写 config.yaml。安全：只允许 pluginsRoot
// 之内的路径 + 严格命名；幂等：已加载则直接返回当前状态。
type loadDscPluginTool struct{ m *Manager }

func (t *loadDscPluginTool) Name() string { return "load_dsc_plugin" }

func (t *loadDscPluginTool) TimeoutMs() int { return 120000 } // 加载插件进程可能较慢

func (t *loadDscPluginTool) Description() string {
	return "Activate an existing DSC plugin that is already on disk under the plugins/ directory but not yet loaded (e.g. it exists but is not in the running config), so its tools become usable by the model this session. " +
		"Give the plugin id as the directory name under plugins/ (e.g. tool-musicplayer). The binary is resolved automatically " +
		"(plugins/<name>/<name>.exe, honoring versioned binaries) unless binary_path is given, and type defaults to tool. " +
		"This does NOT modify config.yaml (the change lasts only for the current process; after a restart you can reload it again). " +
		"Discover what is on disk by listing the plugins/ directory with the shell tool, or list configured plugins with list_dsc_plugins."
}

func (t *loadDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"插件 id，即 plugins/ 下的目录名（如 tool-musicplayer），须匹配 [A-Za-z0-9_-]。"},
"type":{"type":"string","enum":["tool","llm","agent","policy","dsc"],"description":"插件类型，默认 tool。"},
"binary_path":{"type":"string","description":"可选：显式指定插件可执行文件路径；缺省按约定 plugins/<name>/<name>.exe 解析。"},
"persist":{"type":"boolean","description":"是否写回 config.yaml 使重启后仍加载。默认 false：仅当前进程生效、不改配置；true：持久化（先备份 config）。"}},
"required":["name"],"additionalProperties":false}`)
}

func (t *loadDscPluginTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		BinaryPath string `json:"binary_path"`
		Persist    bool   `json:"persist"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if !dscPluginNameRe.MatchString(p.Name) || p.Name == "." || p.Name == ".." {
		return "", fmt.Errorf("invalid name %q: use [A-Za-z0-9_-] and match the plugins/ directory name", p.Name)
	}
	if p.Type == "" {
		p.Type = "tool"
	}
	if !dscPluginTypes[p.Type] {
		return "", fmt.Errorf("invalid type %q", p.Type)
	}

	bin := p.BinaryPath
	if bin == "" {
		pluginDir := filepath.Join(t.m.pluginsRoot(), p.Name)
		if st, err := os.Stat(pluginDir); err != nil || !st.IsDir() {
			return "", fmt.Errorf("插件目录 %s 不存在（插件须已置于 plugins/ 目录下）", pluginDir)
		}
		bin = ResolveLatestBinary(filepath.Join(pluginDir, p.Name+binExt()))
	}
	bin = filepath.Clean(bin)
	root := strings.ToLower(filepath.Clean(t.m.pluginsRoot()))
	if !strings.HasPrefix(strings.ToLower(bin), root) {
		return "", fmt.Errorf("拒绝加载 plugins/ 目录之外的可执行文件: %s", bin)
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("插件可执行文件不存在: %s", bin)
	}

	// 幂等：已在运行则直接返回当前状态（仍可按需落盘）
	t.m.mu.RLock()
	_, already := t.m.clients[p.Name]
	t.m.mu.RUnlock()
	if already {
		if p.Persist {
			if err := t.persistEntry(p.Name, p.Type, bin); err != nil {
				return "", err
			}
		}
		return t.result(p.Name, p.Type, bin, "already-active", false, p.Persist)
	}

	// 持久化需在改动前先备份 config；persist=false 则连备份都不做（不改配置）。
	if p.Persist {
		if _, err := t.m.backupConfig(); err != nil {
			return "", err
		}
	}
	entry := PluginEntry{Name: p.Name, Type: p.Type, BinaryPath: bin, Enabled: true}
	t.m.mu.Lock()
	err := t.m.injectionEntryLocked(entry, p.Persist)
	t.m.mu.Unlock()
	if err != nil {
		return "", fmt.Errorf("加载失败（可能已加载，或插件自身类型/元数据不匹配）: %w", err)
	}
	return t.result(p.Name, p.Type, bin, "loaded", true, p.Persist)
}

// persistEntry 把载入的插件条目显式写回 config.yaml（先备份），保证重启后仍加载。
func (t *loadDscPluginTool) persistEntry(name, typ, bin string) error {
	if _, err := t.m.backupConfig(); err != nil {
		return err
	}
	t.m.mu.Lock()
	err := t.m.persistInjectionLocked(PluginEntry{Name: name, Type: typ, BinaryPath: bin})
	t.m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("写回 config.yaml 失败: %w", err)
	}
	return nil
}

func (t *loadDscPluginTool) result(name, typ, bin, status string, loaded, persist bool) (string, error) {
	b, err := json.Marshal(map[string]any{
		"ok":      true,
		"name":    name,
		"type":    typ,
		"binary":  filepath.ToSlash(bin),
		"status":  status,
		"loaded":  loaded,
		"persist": persist,
		"note":    fmt.Sprintf("插件已载入本进程，其工具立即可用；persist=%v", persist),
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ============================================================
// unload_dsc_plugin
// ============================================================

// unloadDscPluginTool 运行期卸载已载入的插件（本进程内停止其服务并注销其工具）。
// persist=true 时同时从 config.yaml 移除条目（重启后不再加载）；目录文件保留。
type unloadDscPluginTool struct{ m *Manager }

func (t *unloadDscPluginTool) Name() string { return "unload_dsc_plugin" }

func (t *unloadDscPluginTool) TimeoutMs() int { return 120000 }

func (t *unloadDscPluginTool) Description() string {
	return "Unload a DSC plugin that is currently loaded (stops it in this process and removes its tools). " +
		"Give the plugin id as the plugins/ directory name (e.g. tool-musicplayer). " +
		"Set persist=true to also remove its entry from config.yaml so it no longer loads on restart (config is backed up first); " +
		"files under plugins/ are always kept. This complements uninstall_dsc_plugin (which removes config and optionally the directory)."
}

func (t *unloadDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"插件 id，即 plugins/ 下的目录名（如 tool-musicplayer）。"},
"persist":{"type":"boolean","description":"是否同时从 config.yaml 移除条目（重启后不再加载）。默认 false。"}},
"required":["name"],"additionalProperties":false}`)
}

func (t *unloadDscPluginTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name    string `json:"name"`
		Persist bool   `json:"persist"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if !dscPluginNameRe.MatchString(p.Name) || p.Name == "" || p.Name == "." || p.Name == ".." {
		return "", fmt.Errorf("invalid name %q", p.Name)
	}

	// persist=true 会在改动前移除 config 条目：先备份，与 load/install/uninstall 的
	// 「写 config 前必备份」约定保持一致，避免卸载写坏配置。
	if p.Persist {
		if _, err := t.m.backupConfig(); err != nil {
			return "", err
		}
	}

	t.m.mu.RLock()
	_, loaded := t.m.clients[p.Name]
	t.m.mu.RUnlock()
	if !loaded {
		if p.Persist {
			if err := t.m.removeDscPluginConfig(p.Name); err != nil {
				return "", err
			}
		}
		return t.unloadResult(p.Name, "not-loaded", p.Persist)
	}

	if err := t.m.UnloadPlugin(p.Name); err != nil {
		return "", fmt.Errorf("卸载失败: %w", err)
	}
	if p.Persist {
		if err := t.m.removeDscPluginConfig(p.Name); err != nil {
			return "", fmt.Errorf("卸载成功但清 config 失败: %w", err)
		}
	}
	return t.unloadResult(p.Name, "unloaded", p.Persist)
}

func (t *unloadDscPluginTool) unloadResult(name, status string, persist bool) (string, error) {
	b, err := json.Marshal(map[string]any{
		"ok":      true,
		"name":    name,
		"status":  status,
		"persist": persist,
		"note":    fmt.Sprintf("插件已卸载（persist=%v）；plugins/ 目录文件保留", persist),
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ============================================================
// 工具集合 + 拷贝辅助
// ============================================================

// dscPluginTools 返回宿主内置的 DSC 插件管理工具。
func (m *Manager) dscPluginTools() []ToolDefinition {
	return []ToolDefinition{
		&installDscPluginTool{m: m},
		&upgradeDscPluginTool{m: m},
		&uninstallDscPluginTool{m: m},
		&listDscPluginsTool{m: m},
		&loadDscPluginTool{m: m},
		&unloadDscPluginTool{m: m},
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
