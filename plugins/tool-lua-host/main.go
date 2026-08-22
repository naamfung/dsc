package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"dsc/plugin"
	"dsc/plugin/llmclient"
	"dsc/plugin/notify"
	"dsc/plugin/toolclient"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"tool-lua-host/internal/bindings"
	"tool-lua-host/internal/host"
)

// scriptsDirs LUA 脚本目录列表（相对宿主 ExecDir；脚本以 <dir>/<name>/main.lua 组织）。
// 两处目录等价：插件内置目录承载随宿主分发的示例脚本，顶层 scripts/ 承载模型在
// 创造模式中创建的插件，均会被扫描与热加载。
var scriptsDirs = []string{"./scripts", "./plugins/tool-lua-host/scripts"}

// ToolServiceServer 空壳工具插件：内部业务为空，启动后加载 LUA 脚本
// （脚本经 dsc.register_tool 注册工具，本服务汇总转发）。
// 同时实现 PluginHookService（互通机制 3）：脚本经 dsc.hook 注册的
// BeforeTool/AfterTool/OnEvent 钩子在此处响应宿主调用。
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	proto.UnimplementedPluginHookServiceServer
	broker *goplugin.GRPCBroker // 互通：llmclient/toolclient/notify Dial 宿主服务
	mu     sync.Mutex
	host   *host.Host // 脚本宿主（SetInterconnect 后创建）
}

// SetInterconnect 互通握手（机制 1/2/4）：宿主把挂载在本插件 client broker 上的
// 聚合 LLM / 聚合 Tool / 插件通知服务 ID 传入。宿主在调用本 RPC 前已 AcceptAndServe
// 完毕（ConnInfo 5s 内有效），此处立即 eager Dial 并创建脚本宿主。
func (s *ToolServiceServer) SetInterconnect(ctx context.Context, req *proto.InterconnectRequest) (*proto.InterconnectResponse, error) {
	// 约束：插件创造（新增/修改脚本）仅在创造模式（creation）下允许。
	// 非创造模式仍加载启动时已存在的脚本（运行已创建的工具），但禁用热加载
	// 轮询——期间写入的新脚本不会生效，需重启宿主后才作为"已有脚本"加载。
	mode := os.Getenv("DSC_MODE")
	creation := mode == "" || mode == "creation" // 未设置（旧宿主）默认允许创造

	s.mu.Lock()
	defer s.mu.Unlock()

	var llmC *llmclient.Client
	var toolC *toolclient.Client
	var notifier *notify.Notifier
	if s.broker != nil {
		if id := req.GetLlmServiceId(); id != 0 {
			if c, err := llmclient.Dial(s.broker, id); err == nil {
				llmC = c
			}
		}
		if id := req.GetToolServiceId(); id != 0 {
			if c, err := toolclient.Dial(s.broker, id); err == nil {
				toolC = c
			}
		}
		if id := req.GetNotifyServiceId(); id != 0 {
			if n, err := notify.Dial(s.broker, id); err == nil {
				notifier = n
			}
		}
	}

	services := &bindings.Services{LLM: llmC, Tool: toolC, Notify: notifier}
	if s.host != nil {
		s.host.Stop()
	}
	dirs := make([]string, 0, len(scriptsDirs))
	for _, d := range scriptsDirs {
		dirs = append(dirs, filepath.FromSlash(d))
	}
	s.host = host.New(dirs, services, creation, func(format string, args ...any) {
		fmt.Printf("[tool-lua-host] "+format+"\n", args...)
	})
	// 同步加载脚本（创造模式下含热加载轮询），确保握手返回时宿主 ListTools 能取到全部工具
	if err := s.host.Start(); err != nil {
		return nil, err
	}
	fmt.Printf("[tool-lua-host] interconnect ready: llm=%v tool=%v notify=%v, mode=%q creation=%v, scripts dirs=%v\n",
		llmC != nil, toolC != nil, notifier != nil, mode, creation, dirs)
	return &proto.InterconnectResponse{}, nil
}

// hostSnapshot 返回当前脚本宿主（nil 安全）。
func (s *ToolServiceServer) hostSnapshot() *host.Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.host
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	if h := s.hostSnapshot(); h != nil {
		return &proto.ListToolsResponse{Tools: h.ListTools()}, nil
	}
	return &proto.ListToolsResponse{}, nil
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	h := s.hostSnapshot()
	if h == nil {
		return &proto.ExecuteToolResponse{Error: "tool-lua-host: not connected to host"}, nil
	}
	out, err := h.ExecuteTool(req.ToolName, json.RawMessage(req.ArgumentsJson))
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, nil
	}
	return &proto.ExecuteToolResponse{Content: out}, nil
}

func (s *ToolServiceServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{}, nil
}

// ==================== PluginHookService（互通机制 3） ====================

// BeforeTool 响应宿主工具执行前钩子：脚本 handler 返回 (veto, error, new_args)。
func (s *ToolServiceServer) BeforeTool(ctx context.Context, req *proto.BeforeToolRequest) (*proto.BeforeToolResponse, error) {
	h := s.hostSnapshot()
	if h == nil {
		return &proto.BeforeToolResponse{}, nil
	}
	before, _, _ := h.HookSnapshots()
	if len(before) == 0 {
		return &proto.BeforeToolResponse{}, nil
	}
	var args any
	if req.GetArgumentsJson() != "" {
		_ = json.Unmarshal([]byte(req.GetArgumentsJson()), &args)
	}
	results := h.RunHooks("before_tool", before, req.GetToolName(), args)
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
			return &proto.BeforeToolResponse{Veto: true, Error: errMsg}, nil
		}
		if len(vals) > 2 {
			if newArgs, ok := vals[2].(map[string]any); ok {
				if b, err := json.Marshal(newArgs); err == nil {
					return &proto.BeforeToolResponse{ArgumentsJson: string(b)}, nil
				}
			}
		}
	}
	return &proto.BeforeToolResponse{}, nil
}

// AfterTool 响应宿主工具执行后钩子：脚本 handler 返回 (new_result, new_error)。
func (s *ToolServiceServer) AfterTool(ctx context.Context, req *proto.AfterToolRequest) (*proto.AfterToolResponse, error) {
	h := s.hostSnapshot()
	if h == nil {
		return &proto.AfterToolResponse{}, nil
	}
	_, after, _ := h.HookSnapshots()
	if len(after) == 0 {
		return &proto.AfterToolResponse{}, nil
	}
	var args any
	if req.GetArgumentsJson() != "" {
		_ = json.Unmarshal([]byte(req.GetArgumentsJson()), &args)
	}
	results := h.RunHooks("after_tool", after, req.GetToolName(), args, req.GetResult(), req.GetError())
	for _, r := range results {
		vals, _ := r.([]any)
		if len(vals) == 0 {
			continue
		}
		resp := &proto.AfterToolResponse{}
		if newResult, ok := vals[0].(string); ok {
			resp.Result = newResult
		}
		if len(vals) > 1 {
			if newErr, ok := vals[1].(string); ok {
				resp.Error = newErr
			}
		}
		return resp, nil
	}
	return &proto.AfterToolResponse{}, nil
}

// OnEvent 响应宿主事件广播：脚本 handler fn(name, data) 无返回值。
func (s *ToolServiceServer) OnEvent(ctx context.Context, req *proto.OnEventRequest) (*proto.OnEventResponse, error) {
	h := s.hostSnapshot()
	if h == nil {
		return &proto.OnEventResponse{}, nil
	}
	_, _, onEvent := h.HookSnapshots()
	if len(onEvent) == 0 {
		return &proto.OnEventResponse{}, nil
	}
	var data any
	if req.GetDataJson() != "" {
		_ = json.Unmarshal([]byte(req.GetDataJson()), &data)
	}
	h.RunHooks("on_event", onEvent, req.GetName(), data)
	return &proto.OnEventResponse{}, nil
}

// MetadataServer 插件元数据。
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "tool-lua-host",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

// ToolMetadataGRPCPlugin gRPC 插件适配：注册 tool + metadata + 注入 broker。
type ToolMetadataGRPCPlugin struct {
	goplugin.Plugin
	ToolImpl     *ToolServiceServer
	MetadataImpl *MetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	if p.ToolImpl != nil {
		p.ToolImpl.broker = broker // 互通：供 llmclient/toolclient/notify Dial 宿主服务
	}
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	proto.RegisterPluginHookServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

func main() {
	server := &ToolServiceServer{}
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:     server,
				MetadataImpl: &MetadataServer{},
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
