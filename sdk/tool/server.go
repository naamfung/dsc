package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// ToolDef 定義一個工具的元數據與執行邏輯
type ToolDef struct {
	name        string
	description string
	schema      json.RawMessage
	handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

func (t *ToolDef) Name() string {
	return t.name
}

func (t *ToolDef) Description() string {
	return t.description
}

func (t *ToolDef) ParametersSchema() json.RawMessage {
	return t.schema
}

func (t *ToolDef) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.handler(ctx, args)
}

// ToolRegistry 工具註冊表
type ToolRegistry struct {
	tools []*ToolDef
}

// Register 註冊一個工具
func (r *ToolRegistry) Register(t *ToolDef) {
	r.tools = append(r.tools, t)
}

// Tools 返回所有註冊的工具
func (r *ToolRegistry) Tools() []*ToolDef {
	return r.tools
}

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	registry *ToolRegistry
}

func NewToolServiceServer(registry *ToolRegistry) *ToolServiceServer {
	return &ToolServiceServer{
		registry: registry,
	}
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	for _, t := range s.registry.Tools() {
		if t.Name() == req.ToolName {
			res, err := t.Execute(ctx, json.RawMessage(req.ArgumentsJson))
			if err != nil {
				return &proto.ExecuteToolResponse{Error: err.Error()}, nil
			}
			return &proto.ExecuteToolResponse{Content: res}, nil
		}
	}
	return &proto.ExecuteToolResponse{Error: fmt.Sprintf("tool not found: %s", req.ToolName)}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	var tools []*proto.Tool
	for _, t := range s.registry.Tools() {
		tools = append(tools, &proto.Tool{
			Name:           t.Name(),
			Description:    t.Description(),
			ParametersJson: string(t.ParametersSchema()),
		})
	}
	return &proto.ListToolsResponse{Tools: tools}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	meta *PluginMetadata
}

type PluginMetadata struct {
	Type       string
	Name       string
	Version    string
	ApiVersion string
}

func NewMetadataServer(meta *PluginMetadata) *MetadataServer {
	return &MetadataServer{
		meta: meta,
	}
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       m.meta.Type,
		Name:       m.meta.Name,
		Version:    m.meta.Version,
		ApiVersion: m.meta.ApiVersion,
	}, nil
}

// ToolMetadataGRPCPlugin 是 gRPC 插件的適配器
type ToolMetadataGRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	ToolImpl     proto.ToolServiceServer
	MetadataImpl metadata.PluginMetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}
