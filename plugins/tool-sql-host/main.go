package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	dsc "dsc-sdk"
	"dsc/core"
	"tool-sql-host/internal/bindings"
	"tool-sql-host/internal/host"
)

// dspDirs 插件目录列表（相对宿主 ExecDir；.dsp 文件直接置于目录内）。
// 两处目录等价：插件内置目录承载随宿主分发的示例插件，顶层 dsp/ 承载运行中被
// 创建出来的插件，均会被扫描与热加载。
var dspDirs = []string{"./dsp", "./plugins/tool-sql-host/dsp"}

// hostHolder 持有当前插件宿主（SetInterconnect 后创建，ToolProvider/Hook 共享读取）。
// 用 RWMutex 保护，避免插件热加载轮询与宿主 ListTools/ExecuteTool 并发访问竞态。
type hostHolder struct {
	mu   sync.RWMutex
	host *host.Host
}

func (h *hostHolder) set(hh *host.Host) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.host != nil {
		h.host.Stop()
	}
	h.host = hh
}

func (h *hostHolder) get() *host.Host {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.host
}

var holder = &hostHolder{}

func main() {
	// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService / PluginHookService /
	// PluginMetadata 与 go-core 组装。插件内脚本经 dsc.register_tool 注册的工具为运行时
	// 动态集合，故用 sdk.ToolProvider 每次求值；插件钩子经 sdk.Hook 转发到宿主流水线。
	sdk := dsc.New(dsc.Config{
		Name:    "tool-sql-host",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
		// 声明提供 "sql-host" 能力：插件经 dsc.register_tool 动态注册工具，且其
		// 代码/元数据/状态同处一个 .dsp（SQLite）文件。其他插件若需在 SQL 载体上
		// 扩展或调用插件工具，可经 Requires 声明依赖。
		// 对齐 DSH/Cordis 的 provide + inject 能力边界模型。
		Provides: map[string]string{"sql-host": "true"},
	})

	// 互通握手：宿主把聚合 LLM / 聚合 Tool / 插件通知服务挂到本插件 client broker
	// 后回调。此处经 ic 拿到宿主能力客户端（SDK 已 Dial 完毕），创建插件宿主并同步
	// 加载 .dsp，确保握手返回时宿主 ListTools 能取到全部工具。
	sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
		// 约束：插件创造（新增/替换 .dsp）仅在创造模式（creation）下允许。非创造模式
		// 仍加载启动时已存在的插件（运行已创建的工具），但禁用热加载轮询与 pack_dsp
		// 工具——期间写入的新插件不会生效，需重启宿主后才作为"已有插件"加载。
		creation := creationMode()

		services := &bindings.Services{LLM: ic.LLM(), Tool: ic.Tool()}
		if n := ic.Notifier(); n != nil { // 未互联时保持 nil，脚本据此得到明确的「未注入」错误
			services.Notify = n
		}
		h := host.New(append([]string(nil), dspDirs...), services, creation, func(format string, args ...any) {
			fmt.Printf("[tool-sql-host] "+format+"\n", args...)
		})
		// 同步加载插件（创造模式下含热加载轮询），确保握手返回时宿主 ListTools 能取到全部工具
		if err := h.Start(); err != nil {
			return err
		}
		holder.set(h)
		fmt.Printf("[tool-sql-host] interconnect ready: llm=%v tool=%v notify=%v, mode=%q creation=%v, plugin dirs=%v\n",
			ic.LLM() != nil, ic.Tool() != nil, ic.Notifier() != nil, os.Getenv("DSC_MODE"), creation, dspDirs)
		return nil
	})

	// 动态工具：插件注册的工具每次 ListTools/ExecuteTool 求值（插件可热加载增删）。
	// 头部注入静态只读工具 list_dsp_plugins；创造模式下再加打包工具 pack_dsp。
	sdk.ToolProvider(func() []dsc.Tool {
		h := holder.get()
		out := baseTools(creationMode())
		if h == nil {
			return out
		}
		for _, pt := range h.ListTools() {
			name := pt.Name
			out = append(out, dsc.Tool{
				Name:        pt.Name,
				Description: pt.Description,
				Schema:      json.RawMessage(pt.ParametersJson),
				Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
					return h.ExecuteTool(name, args)
				},
			})
		}
		return out
	})

	// 钩子（互通机制 3）：插件经 dsc.hook 注册的 BeforeTool/AfterTool/OnEvent
	// 在此响应宿主调用并转发到对应插件 VM。
	sdk.Hook(dsc.Hook{
		BeforeTool: func(ctx context.Context, toolName, argumentsJSON string) (string, error) {
			h := holder.get()
			if h == nil {
				return argumentsJSON, nil
			}
			before, _, _ := h.HookSnapshots()
			if len(before) == 0 {
				return argumentsJSON, nil
			}
			var args any
			if argumentsJSON != "" {
				_ = json.Unmarshal([]byte(argumentsJSON), &args)
			}
			results := h.RunHooks("before_tool", before, toolName, args)
			for _, r := range results {
				vals, _ := r.([]any)
				if len(vals) == 0 {
					continue
				}
				if veto, ok := vals[0].(bool); ok && veto {
					errMsg := ""
					if len(vals) > 1 {
						errMsg, _ = vals[1].(string)
					}
					return argumentsJSON, fmt.Errorf("%s", errMsg)
				}
				if len(vals) > 2 {
					if newArgs, ok := vals[2].(map[string]any); ok {
						if b, err := json.Marshal(newArgs); err == nil {
							return string(b), nil
						}
					}
				}
			}
			return argumentsJSON, nil
		},
		AfterTool: func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (string, string) {
			h := holder.get()
			if h == nil {
				return result, toolErr
			}
			_, after, _ := h.HookSnapshots()
			if len(after) == 0 {
				return result, toolErr
			}
			var args any
			if argumentsJSON != "" {
				_ = json.Unmarshal([]byte(argumentsJSON), &args)
			}
			results := h.RunHooks("after_tool", after, toolName, args, result, toolErr)
			for _, r := range results {
				vals, _ := r.([]any)
				if len(vals) == 0 {
					continue
				}
				newResult, newErr := result, toolErr
				if v, ok := vals[0].(string); ok {
					newResult = v
				}
				if len(vals) > 1 {
					if v, ok := vals[1].(string); ok {
						newErr = v
					}
				}
				return newResult, newErr
			}
			return result, toolErr
		},
		OnEvent: func(ctx context.Context, eventType, dataJSON string) (string, error) {
			h := holder.get()
			if h == nil {
				return "", nil
			}
			_, _, onEvent := h.HookSnapshots()
			if len(onEvent) == 0 {
				return "", nil
			}
			var data any
			if dataJSON != "" {
				_ = json.Unmarshal([]byte(dataJSON), &data)
			}
			h.RunHooks("on_event", onEvent, eventType, data)
			return "", nil
		},
	})

	sdk.Serve()
}

// creationMode 判断当前是否创造模式（未设置 DSC_MODE 的旧宿主默认允许创造）。
func creationMode() bool {
	mode := os.Getenv("DSC_MODE")
	return mode == "" || mode == core.ModeCreation
}
