package plugin

import (
	"context"
	"encoding/json"

	"dsc/proto"
)

type ToolGRPCServer struct {
	proto.UnimplementedToolServiceServer
	mgr *Manager
}

// NewToolGRPCServer 创建 ToolGRPCServer 实例
func NewToolGRPCServer(mgr *Manager) *ToolGRPCServer {
	return &ToolGRPCServer{mgr: mgr}
}

func (s *ToolGRPCServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	var args json.RawMessage = []byte(req.ArgumentsJson)
	result, err := s.mgr.ExecuteTool(ctx, req.ToolName, args)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, nil
	}
	return &proto.ExecuteToolResponse{Content: result}, nil
}

func (s *ToolGRPCServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	tools := s.mgr.GetToolRegistry().ToOpenAITools()
	var protoTools []*proto.Tool
	for _, t := range tools {
		fn := t["function"].(map[string]interface{})
		paramsJSON, _ := json.Marshal(fn["parameters"])
		protoTools = append(protoTools, &proto.Tool{
			Name:           fn["name"].(string),
			Description:    fn["description"].(string),
			ParametersJson: string(paramsJSON),
		})
	}
	return &proto.ListToolsResponse{Tools: protoTools}, nil
}

// ListContext 聚合所有工具插件贡献的上下文片段（如技能索引），供 agent 拼接到 system prompt
func (s *ToolGRPCServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	content, err := s.mgr.ListContext(ctx)
	if err != nil {
		return &proto.ListContextResponse{}, err
	}
	return &proto.ListContextResponse{Content: content}, nil
}
