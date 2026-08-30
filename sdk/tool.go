package dsc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"dsc/core/agentclient"
	"dsc/core/llmclient"
	"dsc/core/notify"
	"dsc/core/toolclient"
	"dsc/proto"
	"dsc/proto/metadata"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Tool 定义一个工具：名称、描述、JSON Schema 参数与执行处理器。
// 所有字段导出，跨包可直接以字面量初始化。
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Handler     func(ctx context.Context, args json.RawMessage) (string, error)
	// ViewFn 可选：为本工具调用声明结构化视图 spec（JSON，见 dsc.CardView/dsc.TableView/dsc.PlainView）。
	// 入参为调用参数 args 与 Handler 成功返回的结果 result，插件据此构造视图；返回的 spec 经
	// ExecuteToolResponse.view_json 传给 TUI 统一渲染，使该工具的结果呈现为专用卡片而风格与其余
	// 工具一致；缺失/出错时 TUI 回退到通用展示。
	ViewFn func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error)
	// Context 可选：向宿主贡献的 system prompt 片段（ListContext），
	// 用于向模型说明本工具的使用约定（如沙箱边界）。
	// ContextFn 可选：动态上下文（每次 ListContext 调用时求值，优先于 Context），
	// 用于索引等随运行状态变化的内容（如技能索引安装后需立即反映）。
	Context   string
	ContextFn func() string
}

// toolGRPCPlugin 是 go-core 适配器：注册 ToolService + PluginMetadata +
// PluginHookService（钩子为可选注册，未设置时为空实现，宿主调用无副作用）。
type toolGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	sdk *SDK
}

func (p *toolGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	srv := &toolServiceServer{sdk: p.sdk, broker: broker}
	proto.RegisterToolServiceServer(s, srv)
	metadata.RegisterPluginMetadataServer(s, &metadataServer{cfg: p.sdk.cfg})
	proto.RegisterPluginHookServiceServer(s, &hookServiceServer{hook: p.sdk.hook})
	return nil
}

func (p *toolGRPCPlugin) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

// toolServiceServer 工具服务端：ExecuteTool/ListTools/ListContext/SetInterconnect。
type toolServiceServer struct {
	proto.UnimplementedToolServiceServer
	sdk    *SDK
	broker *plugin.GRPCBroker // 互通：Dial 宿主聚合服务（SetInterconnect 时使用）

	mu sync.Mutex
	ic *Interconnect
}

func (s *toolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	for _, t := range s.sdk.snapshotTools() {
		if t.Name == req.ToolName {
			res, err := t.Handler(ctx, json.RawMessage(req.ArgumentsJson))
			if err != nil {
				return &proto.ExecuteToolResponse{Error: err.Error()}, nil
			}
			resp := &proto.ExecuteToolResponse{Content: res}
			// 可选结构化视图：插件基于参数与结果声明显示 spec，TUI 统一渲染（缺失/出错时回退）
			if t.ViewFn != nil {
				if v, verr := t.ViewFn(ctx, json.RawMessage(req.ArgumentsJson), res); verr == nil && len(v) > 0 {
					resp.ViewJson = string(v)
				}
			}
			return resp, nil
		}
	}
	return &proto.ExecuteToolResponse{Error: fmt.Sprintf("tool not found: %s", req.ToolName)}, nil
}

func (s *toolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	var tools []*proto.Tool
	for _, t := range s.sdk.snapshotTools() {
		tools = append(tools, &proto.Tool{
			Name:           t.Name,
			Description:    t.Description,
			ParametersJson: string(t.Schema),
		})
	}
	return &proto.ListToolsResponse{Tools: tools}, nil
}

// ListContext 贡献 system prompt 片段：优先用动态 ContextFn（每调用求值），
// 否则拼接各工具的静态 Context（旧宿主可忽略）。
func (s *toolServiceServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	var b strings.Builder
	for _, t := range s.sdk.snapshotTools() {
		ctxText := t.Context
		if t.ContextFn != nil {
			ctxText = t.ContextFn()
		}
		if ctxText != "" {
			b.WriteString(ctxText)
			b.WriteString("\n")
		}
	}
	return &proto.ListContextResponse{Content: b.String()}, nil
}

// SetInterconnect 互通握手（机制 1/2/4）：宿主把挂载在本插件 client broker 上的
// 聚合 LLM / 聚合 Tool / 通知服务 ID 传入。此处立即 Dial 并调用插件注册的
// InterconnectHandler（若有），供其缓存客户端供后续工具执行使用。
// dial 失败不中断：ic 中对应客户端保持 nil（插件自行判空），错误一并返回
// 供宿主记录（宿主仅 Warn，不终止加载）。
func (s *toolServiceServer) SetInterconnect(ctx context.Context, req *proto.InterconnectRequest) (*proto.InterconnectResponse, error) {
	ic := &Interconnect{}
	var dialErr error
	if s.broker != nil {
		dialInterconnect(req.GetLlmServiceId(), func(id uint32) (*llmclient.Client, error) {
			return llmDial(s.broker, id)
		}, func(c *llmclient.Client) { ic.llm = c }, &dialErr)
		dialInterconnect(req.GetToolServiceId(), func(id uint32) (*toolclient.Client, error) {
			return toolDial(s.broker, id)
		}, func(c *toolclient.Client) { ic.tool = c }, &dialErr)
		dialInterconnect(req.GetNotifyServiceId(), func(id uint32) (*notify.Notifier, error) {
			return notifyDial(s.broker, id)
		}, func(n *notify.Notifier) { ic.ntf = n }, &dialErr)
		dialInterconnect(req.GetAgentServiceId(), func(id uint32) (*agentclient.Client, error) {
			return agentDial(s.broker, id)
		}, func(a *agentclient.Client) { ic.agent = a }, &dialErr)
	}
	s.mu.Lock()
	if old := s.ic; old != nil {
		_ = old.Close()
	}
	s.ic = ic
	s.mu.Unlock()

	if s.sdk.inter != nil {
		if err := s.sdk.inter(ctx, ic); err != nil {
			return &proto.InterconnectResponse{}, err
		}
	}
	return &proto.InterconnectResponse{}, dialErr
}
