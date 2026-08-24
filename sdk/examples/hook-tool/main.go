// hook-tool 示例：工具 + 钩子 + 互通回调。
// 展示独立插件如何：
//  1. 注册工具（ExecuteTool/ListTools/ListContext 由 SDK 自动实现）；
//  2. 注册钩子（BeforeTool 否决/改写、AfterTool 改写结果、OnEvent 订阅）；
//  3. 经 SetInterconnect 拿到宿主聚合 LLM/Tool/Notify 服务（插件间互不感知）。
//
// 构建：go build -o hook-tool.exe .  → 放进宿主 plugins/tool-hook-tool/。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"dsc-sdk"
)

func main() {
	sdk := dsc.New(dsc.Config{Name: "hook-demo", Version: "1.0.0", Type: dsc.TypeTool})

	// 一个只读工具：回显输入。
	sdk.Tool(dsc.Tool{
		Name:        "echo",
		Description: "Echo the input text back to the caller.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		Context:     "echo 工具仅供演示；禁止用 echo 掩盖真实错误。",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var req struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &req); err != nil {
				return "", err
			}
			return "echo: " + req.Text, nil
		},
	})

	// 钩子：演示 veto、参数改写与事件订阅（独立插件即可参与宿主流水线，无需他人配合）。
	sdk.Hook(dsc.Hook{
		BeforeTool: func(ctx context.Context, toolName, argumentsJSON string) (string, error) {
			// 演示否决：禁止任何人执行「危险」的调用
			var args struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal([]byte(argumentsJSON), &args)
			if toolName == "echo" && args.Text == "danger" {
				return "", fmt.Errorf("echo 拒绝输入 danger")
			}
			return argumentsJSON, nil
		},
		AfterTool: func(ctx context.Context, toolName, result, toolErr string) (string, string) {
			return result + " [via hook-demo]", toolErr
		},
		OnEvent: func(ctx context.Context, eventType, dataJSON string) {
			// 宿主事件广播（异步）：turn/start、tool/result 等；仅观察，不阻塞。
			log.Printf("hook-demo: event %s: %s", eventType, dataJSON)
		},
	})

	// 互通：缓存宿主聚合服务客户端（后续工具执行时可调用其他插件的工具/LLM）。
	sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
		log.Printf("hook-demo: interconnect ready, llm=%v tool=%v", ic.LLM() != nil, ic.Tool() != nil)
		_ = ic // 示例不实际使用；真实插件在此缓存 ic 供 Handler 使用
		return nil
	})

	sdk.OnStart(func(ctx context.Context) error {
		env := dsc.ReadEnv()
		log.Printf("hook-demo: mode=%s workspace=%s", env.Mode, env.WorkspaceRoot)
		return nil
	})

	sdk.Serve()
}
