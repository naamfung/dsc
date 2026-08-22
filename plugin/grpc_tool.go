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
	// owner 隔离：调用方会话标识注入 ctx，job 工具按此授权
	if sid := req.GetSessionId(); sid != "" {
		ctx = WithCaller(ctx, sid)
	}
	var args json.RawMessage = []byte(req.ArgumentsJson)
	result, err := s.mgr.ExecuteTool(ctx, req.ToolName, args)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, nil
	}
	return &proto.ExecuteToolResponse{Content: result}, nil
}

func (s *ToolGRPCServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{Tools: s.mgr.AllToolsProto()}, nil
}

// AllToolsProto 返回宿主聚合工具目录（proto.Tool 列表），供子代理 LLM 请求等
// 复用（与主 agent 从 ListTools 拿到的一致）。
func (m *Manager) AllToolsProto() []*proto.Tool {
	openaiTools := m.GetToolRegistry().ToOpenAITools()
	out := make([]*proto.Tool, 0, len(openaiTools))
	for _, t := range openaiTools {
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		paramsJSON, _ := json.Marshal(fn["parameters"])
		out = append(out, &proto.Tool{
			Name:           name,
			Description:    desc,
			ParametersJson: string(paramsJSON),
		})
	}
	return out
}

// ListContext 聚合所有工具插件贡献的上下文片段（如技能索引），供 agent 拼接到 system prompt
func (s *ToolGRPCServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	content, err := s.mgr.ListContext(ctx)
	if err != nil {
		return &proto.ListContextResponse{}, err
	}
	return &proto.ListContextResponse{Content: content}, nil
}
