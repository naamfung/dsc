package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// scriptsDir LUA 脚本目录（相对宿主 ExecDir；脚本以 <dir>/<name>/main.lua 组织）。
const scriptsDir = "./plugins/tool-lua-host/scripts"

// ToolServiceServer 空壳工具插件：内部业务为空，启动后加载 LUA 脚本
// （脚本经 dsc.register_tool 注册工具，本服务汇总转发）。
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	broker *goplugin.GRPCBroker // 互通：llmclient/toolclient/notify Dial 宿主服务
	mu     sync.Mutex
	host   *host.Host // 脚本宿主（SetInterconnect 后创建）
}

// SetInterconnect 互通握手（机制 1/2/4）：宿主把挂载在本插件 client broker 上的
// 聚合 LLM / 聚合 Tool / 插件通知服务 ID 传入。宿主在调用本 RPC 前已 AcceptAndServe
// 完毕（ConnInfo 5s 内有效），此处立即 eager Dial 并创建脚本宿主。
func (s *ToolServiceServer) SetInterconnect(ctx context.Context, req *proto.InterconnectRequest) (*proto.InterconnectResponse, error) {
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
	dir := filepath.FromSlash(scriptsDir)
	s.host = host.New(dir, services, func(format string, args ...any) {
		fmt.Printf("[tool-lua-host] "+format+"\n", args...)
	})
	// 同步加载脚本（含类型检查/热加载轮询），确保握手返回时宿主 ListTools 能取到全部工具
	if err := s.host.Start(); err != nil {
		return nil, err
	}
	fmt.Printf("[tool-lua-host] interconnect ready: llm=%v tool=%v notify=%v, scripts dir=%s\n",
		llmC != nil, toolC != nil, notifier != nil, dir)
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
