package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dsc "dsc-sdk"
	"tool-sql-host/internal/dsp"
	"tool-sql-host/internal/host"
)

// listToolName 列出已加载 .dsp 插件的只读工具名。
const listToolName = "list_sql_plugins"

// packToolName 打包 .dsp 的工具名（仅创造模式暴露）。
const packToolName = "pack_dsp"

// baseTools 返回与插件加载状态无关的静态工具（list_sql_plugins，创造模式下加 pack_dsp）。
func baseTools(creation bool) []dsc.Tool {
	out := []dsc.Tool{listPluginsTool()}
	if creation {
		out = append(out, packDspTool())
	}
	return out
}

// listPluginsTool 列出当前已加载的 .dsp 插件（名称、路径、载体语言、注册工具、
// 自持状态键数、内容哈希），只读，供模型在创建/排查插件前先看清现状。
func listPluginsTool() dsc.Tool {
	return dsc.Tool{
		Name:        listToolName,
		Description: "List the .dsp plugins currently loaded by tool-sql-host: each is a single SQLite database file (suffix .dsp) that carries its own metadata, code and runtime state. Returns name, path, language, entry, registered tools, state key count, blob count and content hash. Read-only; loads nothing new.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			h := holder.get()
			if h == nil {
				return `{"dirs":[],"plugins":[],"count":0}`, nil
			}
			plugins := h.ListPlugins()
			b, err := json.Marshal(map[string]any{
				"dirs":    dspDirs,
				"plugins": plugins,
				"count":   len(plugins),
			})
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		ViewFn: func(_ context.Context, _ json.RawMessage, result string) (json.RawMessage, error) {
			var payload struct {
				Plugins []host.PluginInfo `json:"plugins"`
			}
			if err := json.Unmarshal([]byte(result), &payload); err != nil {
				return nil, err
			}
			rows := make([]dsc.ViewRow, 0, len(payload.Plugins))
			for _, p := range payload.Plugins {
				ro := "rw"
				if p.ReadOnly {
					ro = "ro"
				}
				rows = append(rows, dsc.ViewRow{
					"name":     p.Name,
					"language": p.Language,
					"mode":     ro,
					"tools":    fmt.Sprintf("%d", len(p.Tools)),
					"state":    fmt.Sprintf("%d", p.StateKeys),
					"path":     p.Path,
				})
			}
			return dsc.TableView("SQL 插件（.dsp）", &dsc.ViewBadge{Text: fmt.Sprintf("%d 个", len(rows))},
				[]dsc.ViewColumn{
					{Key: "name", Title: "插件"},
					{Key: "language", Title: "语言"},
					{Key: "mode", Title: "读写"},
					{Key: "tools", Title: "工具"},
					{Key: "state", Title: "状态键"},
					{Key: "path", Title: "路径"},
				}, rows), nil
		},
	}
}

// packDspTool 把源目录打包为一个 .dsp 文件（目录 → SQLite 库）。
// 仅创造模式暴露：它会写出新的插件载体，与「插件创造仅在创造模式允许」同一边界。
func packDspTool() dsc.Tool {
	return dsc.Tool{
		Name: packToolName,
		Description: "Pack a plugin source directory into a .dsp plugin file (a SQLite database). " +
			"Source layout: dsp.yaml (optional manifest: name/language/entry/description/version) plus the entry script (default main.lua); " +
			"other *.lua files are packed as loadable script blobs (dsc.dsp.require), and remaining files as readable assets (dsc.dsp.asset). " +
			"The output path MUST end with .dsp. Writing the output under the plugin dir (./dsp) makes tool-sql-host pick it up automatically within a couple of seconds.",
		Schema: json.RawMessage(`{"type":"object","properties":{
"source":{"type":"string","description":"插件源目录（含 dsp.yaml 与入口脚本），路径经 dsc 正斜杆约定。"},
"output":{"type":"string","description":"输出 .dsp 文件路径（后缀必须为 .dsp，如 ./dsp/my-plugin.dsp）。"},
"name":{"type":"string","description":"插件名（可选，缺省取源目录名或 dsp.yaml 的 name）。仅允许 [A-Za-z0-9_-]。"},
"language":{"type":"string","description":"载体语言（可选，缺省 lua；当前仅支持 lua）。"},
"entry":{"type":"string","description":"入口脚本（可选，缺省 main.lua）。"},
"description":{"type":"string","description":"插件描述（可选，写入 dsp_meta 供 list_sql_plugins 展示）。"},
"version":{"type":"string","description":"插件版本（可选）。"}
},"required":["source","output"],"additionalProperties":false}`),
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Source      string `json:"source"`
				Output      string `json:"output"`
				Name        string `json:"name"`
				Language    string `json:"language"`
				Entry       string `json:"entry"`
				Description string `json:"description"`
				Version     string `json:"version"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if in.Source == "" || in.Output == "" {
				return "", fmt.Errorf("source 与 output 均为必填")
			}
			// 路径隔离：本工具会写出文件，与 tool-filesystem 的沙箱口径一致，
			// 源目录与产物一律限定在宿主工作区内（不越出 workspace_root）。
			for _, p := range []string{in.Source, in.Output} {
				if err := insideWorkspace(p); err != nil {
					return "", err
				}
			}
			// 后缀强制校验在 dsp.Pack 内部（ValidatePath）执行，此处不重复实现。
			info, err := dsp.Pack(in.Source, in.Output, &dsp.Manifest{
				Name: in.Name, Language: in.Language, Entry: in.Entry,
				Description: in.Description, Version: in.Version,
			})
			if err != nil {
				return "", err
			}
			b, err := json.Marshal(info)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
}

// insideWorkspace 校验路径落在允许写入的根内。
// pack_dsp 会写出文件，故与 tool-filesystem 的沙箱口径一致：越界一律拒绝。
//
// 允许的根有两个：宿主工作区（DSC_WORKSPACE_ROOT / 启动目录，即沙箱边界）与插件
// 进程工作目录。后者是必要的——两者被显式配成不同路径时，相对路径的 dsp 目录仍属于
// 「就在手边的工作目录」，不该因配置差异而失效。
func insideWorkspace(path string) error {
	abs, err := dsc.PAbs(path)
	if err != nil {
		return fmt.Errorf("解析路径 %q 失败: %w", path, err)
	}
	roots := make([]string, 0, 2)
	if r := dsc.WorkspaceRoot(); r != "" {
		roots = append(roots, r)
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	for _, root := range roots {
		if withinRoot(root, abs) {
			return nil
		}
	}
	return fmt.Errorf("路径 %q 越出允许的工作目录 %v", path, roots)
}

// withinRoot 判断 abs 是否位于 root 之内（含 root 自身）。
func withinRoot(root, abs string) bool {
	rootAbs, err := dsc.PAbs(root)
	if err != nil {
		return false
	}
	rel, err := dsc.PRel(rootAbs, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, "../")
}
