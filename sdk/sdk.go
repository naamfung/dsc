// Package dsc 是 DSC 的公共插件开发 SDK（module dsc-sdk）。
//
// 独立开发者只需引入本包（及其底层 dsc/proto 依赖），声明式地注册工具、LLM、
// Agent、钩子与互通回调，即可产出宿主零改动即可加载的插件二进制。
// 宿主按目录名 <type>-<name> 发现插件（见 core/manager.go validatePluginDirectoryName），
// 因此构建产物应放到宿主 plugins/ 目录（或经 ADMIN API /plugins/load 动态注入）。
package dsc

import (
	"context"
	"fmt"
	"os"

	plugin "github.com/hashicorp/go-plugin"

	"dsc/core"
	"dsc/proto"
)

// Type 插件类型，与宿主发现规则（目录名 <type>-<name>）与 metadata.Type 对应。
type Type string

const (
	// TypeTool 工具插件：注册一个或多个工具（可附加钩子与互通回调）。
	TypeTool Type = "tool"
	// TypeLLM 大模型插件：实现 core.LLMProvider。
	TypeLLM Type = "llm"
	// TypeAgent 智能体插件：实现 core.Agent。
	TypeAgent Type = "agent"
	// TypePolicy 策略插件：注册宿主可桥接的策略服务（如文件系统观测
	// FsObservationPolicyService）；宿主经主连接直接取对应 proto 客户端。
	TypePolicy Type = "policy"
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
	cfg    Config
	tools  []*Tool
	llm    core.LLMProvider
	agent  core.Agent
	policy proto.FsObservationPolicyServiceServer
	hook   *Hook
	inter  InterconnectHandler
	// toolProvider 可选动态工具提供者：每次 ListTools/ExecuteTool 求值当前工具集，
	// 覆盖「运行时动态注册工具」的插件（如 tool-lua-host 的脚本工具）。非 nil 时
	// 优先于静态 tools，validate 允许二选一。
	toolProvider func() []Tool
	// agentBroker 可选的 agent 类型 gRPC server 初始化回调（注入宿主 broker 的
	// SDK 隔离封装 AgentBroker；实现经其 Dial 宿主挂载的 LLM/Tool/UserQuestions）。
	agentBroker func(b *AgentBroker) error
	onStart     func(context.Context) error
	onStop      func() error
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

// ToolProvider 注册动态工具提供者（仅 tool 类型插件）：每次宿主 ListTools/
// ExecuteTool 时调用 fn 求值当前工具集，适合工具由运行时决定（如脚本注册）的
// 插件；与 sdk.Tool 二选一（provider 非 nil 时优先）。fn 可返回空集（空壳工具
// 插件，仅承载钩子/HTTP 服务等）。
func (s *SDK) ToolProvider(fn func() []Tool) *SDK {
	s.toolProvider = fn
	return s
}

// snapshotTools 返回当前工具集：动态 provider 优先，否则静态注册的 tools。
func (s *SDK) snapshotTools() []Tool {
	if s.toolProvider != nil {
		ts := s.toolProvider()
		if ts == nil {
			return nil
		}
		return ts
	}
	out := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, *t)
	}
	return out
}

// LLM 注册大模型实现（仅 llm 类型插件；实现 core.LLMProvider）。
func (s *SDK) LLM(impl core.LLMProvider) *SDK {
	s.llm = impl
	return s
}

// Agent 注册智能体实现（仅 agent 类型插件；实现 core.Agent）。
func (s *SDK) Agent(impl core.Agent) *SDK {
	s.agent = impl
	return s
}

// Policy 注册策略服务实现（仅 policy 类型插件；实现 proto.FsObservationPolicyServiceServer，
// 宿主经主连接直接取对应 proto 客户端并桥接到工具流水线）。
func (s *SDK) Policy(impl proto.FsObservationPolicyServiceServer) *SDK {
	s.policy = impl
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

// AgentBroker 注册 agent 类型插件的 gRPC server 初始化回调：宿主 broker 就绪后、
// 标准 AgentServiceServer 注册前执行。回调收到 SDK 对 go-core broker 的隔离
// 封装 AgentBroker（插件无需 import go-core）；agent 实现通常缓存它，待
// RegisterServices 拿到 LLM/Tool/UserQuestions 服务 ID 后经 Dial* 建立连接
// （见 core/manager.go 的 RegisterServices 时序）。
func (s *SDK) AgentBroker(fn func(b *AgentBroker) error) *SDK {
	s.agentBroker = fn
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

// Serve 校验声明并启动 go-core gRPC 服务；正常情形永不返回（宿主拉起并管理生命周期）。
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
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: core.Handshake,
		Plugins:         plugins,
		GRPCServer:      plugin.DefaultGRPCServer,
	})

	// Serve 返回说明发生错误（宿主异常终止等）
	fmt.Fprintln(os.Stderr, "dsc-sdk: core serve exited unexpectedly")
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
		if len(s.tools) == 0 && s.toolProvider == nil {
			return fmt.Errorf("tool 类型插件需至少注册一个工具（sdk.Tool 或 sdk.ToolProvider）")
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
	case TypePolicy:
		if s.policy == nil {
			return fmt.Errorf("policy 类型插件必须注册策略服务（调用 sdk.Policy(...)）")
		}
	default:
		return fmt.Errorf("不支持的插件类型 %q（tool | llm | agent | policy）", s.cfg.Type)
	}
	return nil
}

// plugins 组装 go-core 注册表（key 与宿主侧客户端无关紧要，metadata 决定类型）。
// LLM/Agent 类型用 metaWrapper 让元数据以 sdk.Config 的 Name/Version 为准（对齐 tool
// 类型语义），避免实现内部 Name()/Version() 与注册信息两处维护不一致。
func (s *SDK) plugins() map[string]plugin.Plugin {
	switch s.cfg.Type {
	case TypeLLM:
		return map[string]plugin.Plugin{"llm": &core.LLMGRPCPlugin{
			Impl: &llmMetaWrapper{LLMProvider: s.llm, name: s.cfg.Name, version: s.cfg.Version},
		}}
	case TypeAgent:
		return map[string]plugin.Plugin{"agent": &agentGRPCPlugin{sdk: s}}
	case TypePolicy:
		return map[string]plugin.Plugin{"policy": &policyGRPCPlugin{sdk: s}}
	default:
		return map[string]plugin.Plugin{"tool": &toolGRPCPlugin{sdk: s}}
	}
}

// llmMetaWrapper 覆盖 LLMProvider 的 Name/Version：cfg 非空时以 cfg 为准，否则回落实现。
type llmMetaWrapper struct {
	core.LLMProvider
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
	core.Agent
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
