package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"dsc-sdk"
	"tool-lua-host/internal/bindings"
	"tool-lua-host/internal/host"
)

// scriptsDirs LUA 脚本目录列表（相对宿主 ExecDir；脚本以 <dir>/<name>/main.lua 组织）。
// 两处目录等价：插件内置目录承载随宿主分发的示例脚本，顶层 scripts/ 承载模型在
// 创造模式中创建的插件，均会被扫描与热加载。
var scriptsDirs = []string{"./scripts", "./plugins/tool-lua-host/scripts"}

// hostHolder 持有当前脚本宿主（SetInterconnect 后创建，ToolProvider/Hook 共享读取）。
// 用 RWMutex 保护，避免脚本热加载轮询与宿主 ListTools/ExecuteTool 并发访问竞态。
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
	// PluginMetadata 与 go-core 组装。脚本经 dsc.register_tool 注册的工具为运行时
	// 动态集合，故用 sdk.ToolProvider 每次求值；脚本钩子经 sdk.Hook 转发到宿主流水线。
	sdk := dsc.New(dsc.Config{Name: "tool-lua-host", Version: "1.0.0", Type: dsc.TypeTool})

	// 互通握手（机制 1/2/4）：宿主把聚合 LLM / 聚合 Tool / 插件通知服务挂到本插件
	// client broker 后回调。此处经 ic 拿到宿主能力客户端（SDK 已 Dial 完毕），
	// 创建脚本宿主并同步加载脚本，确保握手返回时宿主 ListTools 能取到全部工具。
	sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
		// 约束：插件创造（新增/修改脚本）仅在创造模式（creation）下允许。
		// 非创造模式仍加载启动时已存在的脚本（运行已创建的工具），但禁用热加载
		// 轮询——期间写入的新脚本不会生效，需重启宿主后才作为"已有脚本"加载。
		mode := os.Getenv("DSC_MODE")
		creation := mode == "" || mode == "creation" // 未设置（旧宿主）默认允许创造

		services := &bindings.Services{LLM: ic.LLM(), Tool: ic.Tool(), Notify: ic.Notifier()}
		dirs := make([]string, 0, len(scriptsDirs))
		for _, d := range scriptsDirs {
			dirs = append(dirs, filepath.FromSlash(d))
		}
		h := host.New(dirs, services, creation, func(format string, args ...any) {
			fmt.Printf("[tool-lua-host] "+format+"\n", args...)
		})
		// 同步加载脚本（创造模式下含热加载轮询），确保握手返回时宿主 ListTools 能取到全部工具
		if err := h.Start(); err != nil {
			return err
		}
		holder.set(h)
		fmt.Printf("[tool-lua-host] interconnect ready: llm=%v tool=%v notify=%v, mode=%q creation=%v, scripts dirs=%v\n",
			ic.LLM() != nil, ic.Tool() != nil, ic.Notifier() != nil, mode, creation, dirs)
		return nil
	})

	// 动态工具：脚本注册的工具每次 ListTools/ExecuteTool 求值（脚本可热加载增删）。
	sdk.ToolProvider(func() []dsc.Tool {
		h := holder.get()
		if h == nil {
			return nil
		}
		var out []dsc.Tool
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

	// 钩子（互通机制 3）：脚本经 dsc.hook 注册的 BeforeTool/AfterTool/OnEvent
	// 在此响应宿主调用并转发到对应脚本 VM。
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
		OnEvent: func(ctx context.Context, eventType, dataJSON string) {
			h := holder.get()
			if h == nil {
				return
			}
			_, _, onEvent := h.HookSnapshots()
			if len(onEvent) == 0 {
				return
			}
			var data any
			if dataJSON != "" {
				_ = json.Unmarshal([]byte(dataJSON), &data)
			}
			h.RunHooks("on_event", onEvent, eventType, data)
		},
	})

	sdk.Serve()
}
