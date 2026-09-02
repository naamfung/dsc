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
	"sort"
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

// ApprovalReason 实现 ApprovalRequester（入口②工具声明需审批）：安装并活加载插件
// 即执行任意二进制，属高风险变更，须经用户审批。
func (t *installDscPluginTool) ApprovalReason(_ string) string {
	return "Installs and live-loads a plugin binary (arbitrary executable) into the DSC host."
}

func (t *installDscPluginTool) TimeoutMs() int { return 120000 } // 安装+live 加载插件可能较慢

func (t *installDscPluginTool) Description() string {
	return "Install a DSC plugin (our own binary plugin, a Go program built with the dsc-sdk and loaded via go-plugin/gRPC) into this DSC instance so it can be used. " +
		"Give name as the full plugin directory basename under plugins/ (e.g. tool-musicplayer), consistent with " +
		"load_dsc_plugin/unload_dsc_plugin/uninstall_dsc_plugin/upgrade_dsc_plugin: the directory will be plugins/<name>/ " +
		"and its executable <name>.exe. <type> is one of tool|llm|agent|policy|dsc and must match the plugin's own declared type. " +
		"Provide source as a directory already laid out as plugins/<name>/ (containing <name>.exe), or as a single built <name>.exe binary. " +
		"The tool validates naming, backs up config.yaml, live-loads the plugin to verify it starts (type/metadata must match), " +
		"and only then persists it to config.yaml so it survives restart. On failure nothing is persisted and the copied directory is removed. " +
		"A successful return (ok:true) is sufficient confirmation: the plugin is already live-loaded and its commands are immediately usable in this session — " +
		"they appear in your available tool list. Do NOT verify with the shell tool or inspect files on disk."
}

func (t *installDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"type":{"type":"string","enum":["tool","llm","agent","policy","dsc"],"description":"Plugin type (must match the plugin's own declared type)."},
"name":{"type":"string","description":"Full plugin directory basename (e.g. tool-musicplayer), consistent with load/unload/uninstall/upgrade; directory will be plugins/<name>/."},
"source":{"type":"string","description":"Path to a directory laid out as plugins/<name>/ (containing <name>.exe) or to a single <name>.exe binary."},
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
		"name":    p.Name,
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
	// name 即完整目录基名（如 tool-musicplayer），与 load/unload/uninstall/upgrade
	// 一致，直接作为 plugins/ 下的目录基名；type 仅用于条目声明与 live 加载元数据校验。
	dirBase := name
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

// ApprovalReason 实现 ApprovalRequester：卸载并删除插件配置，属不可逆高影响变更，须经用户审批。
func (t *uninstallDscPluginTool) ApprovalReason(_ string) string {
	return "Uninstalls a DSC plugin and removes its config entry (irreversible)."
}

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
	return "List the DSC plugins for this instance (name, type, enabled, state). This is the authoritative single source: it merges three sources — plugins declared in config.yaml, plugins currently loaded at runtime (state=loaded), and plugins present on disk under plugins/ but not configured (state=orphan). Use this to discover plugins and their state; do NOT enumerate the plugins/ directory yourself with the shell tool, and never try to invoke a plugin binary directly — plugins are used exclusively through their tools."
}

func (t *listDscPluginsTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (t *listDscPluginsTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	m := t.m
	entries, err := m.listDscPluginConfig()
	if err != nil {
		return "", err
	}

	// 运行态：当前已加载的插件（活 client 进程）→ 类型映射
	m.mu.RLock()
	loadedNames := make(map[string]bool, len(m.clients))
	for n := range m.clients {
		loadedNames[n] = true
	}
	typeBy := make(map[string]string, len(m.typeMap))
	for n, typ := range m.typeMap {
		typeBy[n] = typ
	}
	m.mu.RUnlock()

	// 磁盘孤儿候选：plugins/<name>/<name><ext> 存在但未在 config 声明
	diskNames := m.listPluginDiskNames()

	type row struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		State   string `json:"state"` // loaded=运行期已加载; configured=config 声明未加载; orphan=磁盘孤儿/未配
		Enabled bool   `json:"enabled"`
	}

	// 合并三方（config + 运行态 + 磁盘），按名升序稳定输出
	confByName := make(map[string]PluginEntry, len(entries))
	names := make(map[string]bool, len(entries)+len(loadedNames)+len(diskNames))
	for _, e := range entries {
		confByName[e.Name] = e
		names[e.Name] = true
	}
	for n := range loadedNames {
		names[n] = true
	}
	for _, n := range diskNames {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	rows := make([]row, 0, len(sorted))
	for _, name := range sorted {
		conf, inConf := confByName[name]
		r := row{Name: name}
		switch {
		case loadedNames[name]:
			r.State = "loaded"
			r.Type = typeBy[name]
			if r.Type == "" {
				r.Type = conf.Type
			}
			r.Enabled = true
		case inConf:
			r.State = "configured"
			r.Type = conf.Type
			r.Enabled = conf.Enabled
		default:
			r.State = "orphan"
			r.Type = inferPluginTypeFromDir(name)
			r.Enabled = false
		}
		rows = append(rows, r)
	}
	b, _ := json.Marshal(map[string]any{
		"plugins": rows,
		"count":   len(rows),
		"note":    "state: loaded=运行期已加载; configured=config 声明但未加载; orphan=磁盘存在但未配置",
	})
	return string(b), nil
}

// 插件二进制命名约定（严格）：
//   目录 plugins/<type>-<name>/ 内的可执行文件主名须与目录名一致（类型前缀不可省略），
//   即 <type>-<name>；若带版本号（升级部署按 -v<semver> 后缀），版本号须符合
//   语义版本约定。目录内存在合规二进制则该目录是一个插件（孤儿/未配候选）——
//   判定依据是「文件名称」，而非仅目录名。
// 例如：tool-novelforge/tool-novelforge.exe 合规；tool-novelforge-v2.3.1.exe 合规；
//      novelforge.exe（省略 tool- 前缀）与目录名不符，不合规，系旧残留。

// pluginVersionSuffixRe 匹配 -v<semver> 版本后缀（形如 2 / 2.3 / 2.3.1）。
var pluginVersionSuffixRe = regexp.MustCompile(`^v[0-9]+(?:\.[0-9]+){0,2}(?:-[0-9A-Za-z.-]+)?$`)

// pluginBinaryStemValid 判断可执行文件主名（去扩展名后的 stem）对给定目录名是否合规
// （严格）：等于目录名，或等于目录名带 -v<semver> 版本后缀。
func pluginBinaryStemValid(dir, stem string) bool {
	if stem == dir {
		return true
	}
	if i := strings.LastIndex(stem, "-v"); i > 0 {
		pre, ver := stem[:i], stem[i+1:]
		if pre == dir && pluginVersionSuffixRe.MatchString(ver) {
			return true
		}
	}
	return false
}

// diskPluginBinary 在插件目录内查找首个符合命名约定的可执行文件路径；无则返回空串。
// 返回的路径统一用正斜杠（filepath.ToSlash），避免 Windows 反斜杠误导模型——
// 宿主 SHELL 是标准 POSIX，内置工具结果一律以正斜杠对外。
func (m *Manager) diskPluginBinary(name string) string {
	dir := filepath.Join(m.pluginsRoot(), name)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		if runtime.GOOS == "windows" {
			if !strings.EqualFold(filepath.Ext(fn), ".exe") {
				continue
			}
		} else {
			fi, err := e.Info()
			if err != nil || fi.Mode().Perm()&0111 == 0 {
				continue
			}
		}
		stem := strings.TrimSuffix(fn, binExt())
		if pluginBinaryStemValid(name, stem) {
			return filepath.ToSlash(filepath.Join(dir, fn))
		}
	}
	return ""
}

// listPluginDiskNames 扫描 pluginsRoot 下的目录名，仅将「目录内存在合规执行文件」的
// 目录计为孤儿/未配候选——校验文件名称，而非仅凭目录名。
func (m *Manager) listPluginDiskNames() []string {
	ents, err := os.ReadDir(m.pluginsRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !dscPluginNameRe.MatchString(name) {
			continue
		}
		if m.diskPluginBinary(name) == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// inferPluginTypeFromDir 从目录名 <type>-<name> 推断插件类型；无法识别返回空串。
func inferPluginTypeFromDir(base string) string {
	for _, p := range []string{"tool-", "llm-", "agent-", "policy-", "dsc-"} {
		if strings.HasPrefix(base, p) {
			return strings.TrimSuffix(p, "-")
		}
	}
	return ""
}

func (t *listDscPluginsTool) ExecuteWithView(ctx context.Context, args json.RawMessage, result string) (string, string, error) {
	var r struct {
		Plugins []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			State   string `json:"state"`
			Enabled bool   `json:"enabled"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return result, "", nil
	}
	rows := make([]ViewRow, 0, len(r.Plugins))
	for _, p := range r.Plugins {
		rows = append(rows, ViewRow{
			"name":    p.Name,
			"type":    p.Type,
			"state":   stateLabel(p.State),
			"enabled": fmt.Sprint(p.Enabled),
		})
	}
	v, verr := json.Marshal(ToolView{
		Kind:    "table",
		Title:   "DscPlugins",
		Columns: []ViewColumn{{Key: "name", Title: "插件"}, {Key: "type", Title: "类型"}, {Key: "state", Title: "状态"}, {Key: "enabled", Title: "启用"}},
		Rows:    rows,
	})
	if verr != nil {
		return result, "", nil
	}
	return result, string(v), nil
}

// stateLabel 把运行态/状态 token 转为人读中文标签。
func stateLabel(s string) string {
	switch s {
	case "loaded":
		return "已加载"
	case "configured":
		return "未加载"
	case "orphan":
		return "未配置"
	default:
		return s
	}
}

// ============================================================
// upgrade_dsc_plugin
// ============================================================

type upgradeDscPluginTool struct{ m *Manager }

func (t *upgradeDscPluginTool) Name() string { return "upgrade_dsc_plugin" }

// ApprovalReason 实现 ApprovalRequester：部署新二进制并热更替运行进程，须经用户审批。
func (t *upgradeDscPluginTool) ApprovalReason(_ string) string {
	return "Deploys a new plugin binary and hot-replaces the running process."
}

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

// ApprovalReason 实现 ApprovalRequester：加载插件二进制（执行任意代码）并上线注册，须经用户审批。
func (t *loadDscPluginTool) ApprovalReason(_ string) string {
	return "Loads a DSC plugin binary (arbitrary executable) and registers it live."
}

func (t *loadDscPluginTool) TimeoutMs() int { return 120000 } // 加载插件进程可能较慢

func (t *loadDscPluginTool) Description() string {
	return "Activate an existing DSC plugin that is already on disk under the plugins/ directory but not yet loaded (e.g. it exists but is not in the running config), so its tools become usable by the model this session. " +
		"Give the plugin id as the directory name under plugins/ (e.g. tool-musicplayer). The binary is resolved automatically " +
		"(plugins/<name>/<name>.exe, honoring versioned binaries), and type defaults to tool. " +
		"This does NOT modify config.yaml (the change lasts only for the current process; after a restart you can reload it again). " +
		"A successful return (ok:true, status=loaded) is sufficient confirmation: the plugin is active and its commands are immediately usable this session — " +
		"they appear in your available tool list. Do NOT verify with the shell tool or inspect files on disk. " +
		"Discover plugins and their state with list_dsc_plugins (it merges config, runtime, and disk orphans)."
}

func (t *loadDscPluginTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"插件 id，即 plugins/ 下的目录名（如 tool-musicplayer），须匹配 [A-Za-z0-9_-]。"},
"type":{"type":"string","enum":["tool","llm","agent","policy","dsc"],"description":"插件类型，默认 tool。"},
"persist":{"type":"boolean","description":"是否写回 config.yaml 使重启后仍加载。默认 false：仅当前进程生效、不改配置；true：持久化（先备份 config）。"}},
"required":["name"],"additionalProperties":false}`)
}

func (t *loadDscPluginTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Persist bool   `json:"persist"`
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

	// 二进制一律按命名约定自动解析（plugins/<name>/<name>.exe，含版本化），不向模型
	// 暴露插件文件路径，也不接受模型传入路径。
	pluginDir := filepath.Join(t.m.pluginsRoot(), p.Name)
	if st, err := os.Stat(pluginDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("插件目录 %s 不存在（插件须已置于 plugins/ 目录下）", pluginDir)
	}
	bin := ResolveLatestBinary(filepath.Join(pluginDir, p.Name+binExt()))
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
		return t.result(p.Name, p.Type, "already-active", false, p.Persist)
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
	return t.result(p.Name, p.Type, "loaded", true, p.Persist)
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

func (t *loadDscPluginTool) result(name, typ, status string, loaded, persist bool) (string, error) {
	note := "插件已载入本进程，其工具立即可用（出现在你的工具列表中），无需磁盘验证"
	if persist {
		note += "；已写入 config.yaml，重启后仍自动加载"
	} else {
		note += "；未改配置（重启后失效，可再次 load）"
	}
	b, err := json.Marshal(map[string]any{
		"ok":      true,
		"name":    name,
		"type":    typ,
		"status":  status,
		"loaded":  loaded,
		"persist": persist,
		"note":    note,
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

// ApprovalReason 实现 ApprovalRequester：卸载插件（停止其进程、撤销注册），须经用户审批。
func (t *unloadDscPluginTool) ApprovalReason(_ string) string {
	return "Unloads a DSC plugin (stops its process and unregisters it)."
}

func (t *unloadDscPluginTool) TimeoutMs() int { return 120000 }

func (t *unloadDscPluginTool) Description() string {
	return "Unload a DSC plugin that is currently loaded (stops it in this process and removes its tools). " +
		"Give the plugin id as the plugins/ directory name (e.g. tool-musicplayer). " +
		"Set persist=true to also remove its entry from config.yaml so it no longer loads on restart (config is backed up first); " +
		"files under plugins/ are always kept. This complements uninstall_dsc_plugin (which removes config and optionally the directory). " +
		"A successful return (ok:true) is sufficient confirmation: the plugin's commands are removed from your available tool list. Do NOT verify with the shell tool."
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
