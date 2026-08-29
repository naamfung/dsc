package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"dsc/coderuntime"
	"dsc/proto"
)

// runCodeToolName PTC 呈现模式的 run_code presentation transport 名称（对齐 DSH：该名保留给
// presentation 层，普通业务工具不能注册或 shadow；native 模式不对模型暴露）。
const runCodeToolName = "run_code"

// runCodeTool 宿主内置 run_code 工具：执行一段 Lua 程序来组合多步工具调用
// （PTC code-runtime 的对外载体）。程序内每个可用工具以「同名 Lua 函数」呈现
// （由工具目录经 coderuntime.GenerateSDK 生成，见 Execute），外加全局 args
// 与原生 tool(name, args)。程序顶层 `return <value>` 即结果；运行失败
// （StopError/Cancelled）以结构化 Result JSON 作为数据返回，不硬失败。
type runCodeTool struct{ m *Manager }

func (t *runCodeTool) Name() string { return runCodeToolName }

func (t *runCodeTool) Description() string {
	return "Write and run a Lua program that composes multiple tool calls into a single step. " +
		"Globals: args (the JSON input object passed as `args`); each available tool is exposed " +
		"as a Lua function callable as <tool_name>{...} (e.g. read_file{path=...}); the raw " +
		"tool(name, args) binding is also available. The program's top-level `return <value>` is " +
		"the result. Use this to batch a sequence of operations that would otherwise need many " +
		"tool calls, or when the user explicitly asks to run code."
}

func (t *runCodeTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"source": {"type": "string", "description": "Lua program source. Globals: args, plus one function per available tool (called as <tool_name>{...}). The top-level 'return <value>' is the result."},
			"args": {"type": "object", "description": "Optional JSON input object exposed to the program as the 'args' global."},
			"timeout_ms": {"type": "integer", "description": "Optional per-run timeout in milliseconds; 0/omitted = unlimited."}
		},
		"required": ["source"]
	}`)
}

func (t *runCodeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Source    string `json:"source"`
		Args      any    `json:"args"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("run_code: invalid args: %w; 正确用法: 参数形如 {\"source\": \"local n = grep{pattern=\\\"TODO\\\"}; return n.count\", \"args\": {...}, \"timeout_ms\": 30000}", err)
	}
	if strings.TrimSpace(p.Source) == "" {
		return "", fmt.Errorf("run_code: source is required; 正确用法: {\"source\": \"return args.query\"}，其中 source 是 Lua 程序，顶层 return 即结果")
	}

	// 由工具目录生成 SDK，拼接在用户脚本之前执行。
	sdk := coderuntime.GenerateSDK(toolSpecsFromProto(t.m.AllToolsProto(), runCodeToolName))
	var timeout time.Duration
	if p.TimeoutMs > 0 {
		timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}
	bindings := map[string]any{}
	if p.Args != nil {
		bindings["args"] = p.Args
	}

	res := coderuntime.Run(ctx, coderuntime.Options{
		Script: sdk + "\n" + p.Source,
		Tool: func(rctx context.Context, name, argsJSON string) (string, error) {
			return t.m.ExecuteTool(rctx, name, json.RawMessage(argsJSON))
		},
		Bindings: bindings,
		Timeout:  timeout,
	})
	// 失败（StopError/StopCancelled）也作为结构化数据返回，模型可见而非硬中断。
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", fmt.Errorf("run_code: render result: %w", err)
	}
	return string(b), nil
}

// toolSpecsFromProto 把宿主工具目录（[]*proto.Tool）转成 SDK 用的 ToolSpec 列表，
// 按工具名排序（稳定、前缀缓存友好）；skip 指定的工具（如 run_code 自身）排除，
// 避免在同名 Lua 全局下造成嵌套/遮蔽歧义。
func toolSpecsFromProto(tools []*proto.Tool, skip string) []coderuntime.ToolSpec {
	out := make([]coderuntime.ToolSpec, 0, len(tools))
	for _, t := range tools {
		if t.Name == skip {
			continue
		}
		out = append(out, coderuntime.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			JSONSchema:  t.ParametersJson,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
