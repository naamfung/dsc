// Package dsc 是 DSC 的公共插件开发 SDK（module dsc-sdk）。
//
// 独立开发者只需引入本包（及其底层 dsc/proto 依赖），声明式地注册工具、LLM、
// Agent、钩子与互通回调，即可产出宿主零改动即可加载的插件二进制。
// 宿主按目录名 <type>-<name> 发现插件（见 plugin/manager.go validatePluginDirectoryName），
// 因此构建产物应放到宿主 plugins/ 目录（或经 ADMIN API /plugins/load 动态注入）。
package dsc

import (
	"context"
	"fmt"
	"os"

	goplugin "github.com/hashicorp/go-plugin"

	"dsc/plugin"
)

// Type 插件类型，与宿主发现规则（目录名 <type>-<name>）与 metadata.Type 对应。
type Type string

const (
	// TypeTool 工具插件：注册一个或多个工具（可附加钩子与互通回调）。
	TypeTool Type = "tool"
	// TypeLLM 大模型插件：实现 plugin.LLMProvider。
	TypeLLM Type = "llm"
	// TypeAgent 智能体插件：实现 plugin.Agent。
	TypeAgent Type = "agent"
)

// Config 插件声明。Name 与宿主发现/注入的插件名一致（目录 plugins/<type>-<name>/）。
type Config struct {
	Name       string
	Version    string
	Type       Type
	APIVersion string // 默认 "1.0"；宿主校验必须在 [1.0, 2.0)
}

// SDK 声明式插件构建器：注册完成后调用 Serve 启动 gRPC 插件进程。
type SDK struct {
	cfg     Config
	tools   []*Tool
	llm     plugin.LLMProvider
	agent   plugin.Agent
	hook    *Hook
	inter   InterconnectHandler
	onStart func(context.Context) error
	onStop  func() error
}

// New 创建 SDK。Name/Type 必填，Version 建议填写。
func New(cfg Config) *SDK {
	if cfg.APIVersion == "" {
		cfg.APIVersion = "1.0"
	}
	return &SDK{cfg: cfg}
}

// Tool 注册一个工具（仅 tool 类型插件）。
func (s *SDK) Tool(t Tool) *SDK {
	s.tools = append(s.tools, &t)
	return s
}

// LLM 注册大模型实现（仅 llm 类型插件；实现 plugin.LLMProvider）。
func (s *SDK) LLM(impl plugin.LLMProvider) *SDK {
	s.llm = impl
	return s
}

// Agent 注册智能体实现（仅 agent 类型插件；实现 plugin.Agent）。
func (s *SDK) Agent(impl plugin.Agent) *SDK {
	s.agent = impl
	return s
}

// Hook 注册插件钩子（任何类型均可）：宿主在工具流水线执行前/后调用
// BeforeTool/AfterTool，并把宿主事件广播给 OnEvent。
func (s *SDK) Hook(h Hook) *SDK {
	s.hook = &h
	return s
}

// SetInterconnect 注册互通回调：宿主挂载聚合 LLM/Tool/Notify 服务后回调，
// 插件可经 ic.LLM()/ic.Tool()/ic.Notify() 调用宿主能力（独立插件间互不感知）。
func (s *SDK) SetInterconnect(handler InterconnectHandler) *SDK {
	s.inter = handler
	return s
}

// OnStart 注册进程内启动钩子（Serve 建立 gRPC 服务前执行一次）。
func (s *SDK) OnStart(fn func(context.Context) error) *SDK {
	s.onStart = fn
	return s
}

// OnStop 注册进程内停止钩子（进程退出前执行，供释放资源）。
func (s *SDK) OnStop(fn func() error) *SDK {
	s.onStop = fn
	return s
}

// Serve 校验声明并启动 go-plugin gRPC 服务；正常情形永不返回（宿主拉起并管理生命周期）。
func (s *SDK) Serve() {
	if err := s.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "dsc-sdk: %v\n", err)
		os.Exit(2)
	}
	ctx := context.Background()
	if s.onStart != nil {
		if err := s.onStart(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "dsc-sdk: on-start hook failed: %v\n", err)
			os.Exit(2)
		}
	}
	if s.onStop != nil {
		defer func() {
			if err := s.onStop(); err != nil {
				fmt.Fprintf(os.Stderr, "dsc-sdk: on-stop hook failed: %v\n", err)
			}
		}()
	}

	plugins := s.plugins()
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins:         plugins,
		GRPCServer:      goplugin.DefaultGRPCServer,
	})

	// Serve 返回说明发生错误（宿主异常终止等）
	fmt.Fprintln(os.Stderr, "dsc-sdk: plugin serve exited unexpectedly")
	os.Exit(1)
}

func (s *SDK) validate() error {
	if s.cfg.Name == "" {
		return fmt.Errorf("Config.Name 必填（宿主按目录名 <type>-<name> 发现插件）")
	}
	if s.cfg.Type == "" {
		return fmt.Errorf("Config.Type 必填（tool | llm | agent）")
	}
	switch s.cfg.Type {
	case TypeTool:
		if len(s.tools) == 0 {
			return fmt.Errorf("tool 类型插件至少注册一个工具（调用 sdk.Tool(...)）")
		}
		for i, t := range s.tools {
			if t.Name == "" {
				return fmt.Errorf("tools[%d].Name 不能为空", i)
			}
			if t.Handler == nil {
				return fmt.Errorf("tools[%d]（%s）未设置 Handler", i, t.Name)
			}
		}
	case TypeLLM:
		if s.llm == nil {
			return fmt.Errorf("llm 类型插件必须注册 LLMProvider（调用 sdk.LLM(...)）")
		}
	case TypeAgent:
		if s.agent == nil {
			return fmt.Errorf("agent 类型插件必须注册 Agent（调用 sdk.Agent(...)）")
		}
	default:
		return fmt.Errorf("不支持的插件类型 %q（tool | llm | agent）", s.cfg.Type)
	}
	return nil
}

// plugins 组装 go-plugin 注册表（key 与宿主侧客户端无关紧要，metadata 决定类型）。
// LLM/Agent 类型用 metaWrapper 让元数据以 sdk.Config 的 Name/Version 为准（对齐 tool
// 类型语义），避免实现内部 Name()/Version() 与注册信息两处维护不一致。
func (s *SDK) plugins() map[string]goplugin.Plugin {
	switch s.cfg.Type {
	case TypeLLM:
		return map[string]goplugin.Plugin{"llm": &plugin.LLMGRPCPlugin{
			Impl: &llmMetaWrapper{LLMProvider: s.llm, name: s.cfg.Name, version: s.cfg.Version},
		}}
	case TypeAgent:
		return map[string]goplugin.Plugin{"agent": &plugin.AgentGRPCPlugin{
			Impl: &agentMetaWrapper{Agent: s.agent, name: s.cfg.Name, version: s.cfg.Version},
		}}
	default:
		return map[string]goplugin.Plugin{"tool": &toolGRPCPlugin{sdk: s}}
	}
}

// llmMetaWrapper 覆盖 LLMProvider 的 Name/Version：cfg 非空时以 cfg 为准，否则回落实现。
type llmMetaWrapper struct {
	plugin.LLMProvider
	name, version string
}

func (w *llmMetaWrapper) Name(ctx context.Context) string {
	if w.name != "" {
		return w.name
	}
	return w.LLMProvider.Name(ctx)
}

func (w *llmMetaWrapper) Version(ctx context.Context) string {
	if w.version != "" {
		return w.version
	}
	return w.LLMProvider.Version(ctx)
}

// agentMetaWrapper 覆盖 Agent 的 Name/Version：cfg 非空时以 cfg 为准，否则回落实现。
type agentMetaWrapper struct {
	plugin.Agent
	name, version string
}

func (w *agentMetaWrapper) Name(ctx context.Context) string {
	if w.name != "" {
		return w.name
	}
	return w.Agent.Name(ctx)
}

func (w *agentMetaWrapper) Version(ctx context.Context) string {
	if w.version != "" {
		return w.version
	}
	return w.Agent.Version(ctx)
}
