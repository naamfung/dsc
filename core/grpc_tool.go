package core

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

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
	result, viewJSON, err := s.mgr.ExecuteToolWithView(ctx, req.ToolName, args)
	if err != nil {
		return &proto.ExecuteToolResponse{Error: err.Error()}, nil
	}
	return &proto.ExecuteToolResponse{Content: result, ViewJson: viewJSON}, nil
}

// ListTools 返回当前 presentation mode 下模型可直接调用的工具目录。
// 对齐 DSH presentation mode：native 只给业务工具（run_code transport 隐藏）；
// PTC 则把直接工具调用折叠为唯一 run_code，其描述承载程序内可调的工具清单。
func (s *ToolGRPCServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{Tools: s.mgr.AgentDirectTools()}, nil
}

// AllToolsProto 返回宿主业务工具目录（proto.Tool 列表），供子代理 LLM 请求与
// run_code 的 SDK 注入复用。run_code 属 PTC presentation transport，不在普通业务
// 工具目录中（对齐 DSH：run_code 名被保留、仅由 presentation 层暴露）。
func (m *Manager) AllToolsProto() []*proto.Tool {
	openaiTools := m.GetToolRegistry().ToOpenAITools()
	out := make([]*proto.Tool, 0, len(openaiTools))
	for _, t := range openaiTools {
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		if name == runCodeToolName {
			continue
		}
		desc, _ := fn["description"].(string)
		paramsJSON, _ := json.Marshal(fn["parameters"])
		out = append(out, &proto.Tool{
			Name:           name,
			Description:    desc,
			ParametersJson: string(paramsJSON),
		})
	}
	// 稳定排序：工具目录顺序必须确定。底层 ToOpenAITools 遍历共享 map，迭代序随机；
	// 若直接把该顺序交给模型（ListTools→agent tools、subagent LLM、run_code SDK、
	// PTC 描述内嵌清单），请求前缀随进程随机漂移、命中前缀缓存失效。此处统一按名升序，
	// 与 agent 端对工具目录的排序算法一致（顺序不影响功能，但必须确定）。
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// isPTC 当前是否为 PTC 呈现模式（短读锁）。
func (m *Manager) isPTC() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ptc
}

// AgentDirectTools 返回当前 presentation mode 下模型可直接调用的工具目录。
// 由聚合 ListTools 使用（gen agent 的 LLM tools 参数与 prompt 工具清单来源）。
// 对齐 DSH presentation：native 只暴露业务工具（run_code 隐藏）；PTC 折叠直接
// 调用为唯一 run_code，并把业务工具名作为 SDK 清单并入 run_code 描述。
func (m *Manager) AgentDirectTools() []*proto.Tool {
	m.syncPluginTools()         // 节流同步各 tool 插件最新工具（热加载的脚本工具立即可见）
	native := m.AllToolsProto() // 不含 run_code
	if !m.isPTC() {
		return native
	}
	def, ok := m.toolRegistry.Get(runCodeToolName)
	if !ok {
		return native
	}
	names := make([]string, 0, len(native))
	for _, t := range native {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	desc := def.Description() + "\n\nAvailable to call inside the program (SDK): " + strings.Join(names, ", ")
	return []*proto.Tool{{
		Name:           runCodeToolName,
		Description:    desc,
		ParametersJson: string(def.ParametersSchema()),
	}}
}

// ListContext 聚合所有工具插件贡献的上下文片段（如技能索引），供 agent 拼接到 system prompt
func (s *ToolGRPCServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	content, err := s.mgr.ListContext(ctx)
	if err != nil {
		return &proto.ListContextResponse{}, err
	}
	return &proto.ListContextResponse{Content: content}, nil
}
