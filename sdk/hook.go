package dsc

import (
	"context"

	"dsc/proto"
)

// Hook 插件钩子（语义化封装宿主 proto.PluginHookService）：
// 宿主在工具流水线执行前调用 BeforeTool（可否决/改写参数），执行后调用
// AfterTool（可改写结果/错误），并把宿主事件经 OnEvent 同步分发。
// 任一字段均可省略（nil 表示不参与该环节）。
//
// OnEvent 对齐 DSH Cordis ctx.on() 设计：单一事件钩子接口，分发模式由事件名
// 决定（宿主侧 eventDispatchMode 映射表），listener 不感知模式。
//
// 多进程适配（与 DSH 单进程的关键差异）：DSH 在同进程内可把 next 闭包传给
// listener；DSC 插件在独立进程，next 无法跨进程传递。故 DSC 的 OnEvent 不含
// next 参数——宿主侧 dispatchEventToPlugins 已实现洋葱模型：按顺序调用每个
// 插件，把上一个插件返回的 result_json 透传给下一个。插件侧只需返回改写后的
// result 或 error，宿主负责串联。
//   - emit 模式事件（如 agent/status）：返回值被忽略；listener 仅产生副作用
//   - waterfall 模式事件（如 agent/pre-step）：resultJSON 非空表示改写后的载荷
//     透传给下游插件；err 非空 veto 整条链
type Hook struct {
	// BeforeTool 返回改写后的参数 JSON；err 非 nil 视为否决（阻止工具执行，
	// 错误文本会反馈给调用方）。
	BeforeTool func(ctx context.Context, toolName, argumentsJSON string) (rewrittenJSON string, err error)
	// AfterTool 返回改写后的结果与错误文本。argumentsJSON 为本次调用的原始
	// 参数（供按参数改写结果/判断），与宿主 AfterTool 语义一致。
	AfterTool func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (newResult, newErr string)
	// OnEvent 唯一的事件钩子（对齐 DSH ctx.on()）。
	//
	// eventType 为事件名（如 "agent/pre-step"、"agent/status"）。
	// dataJSON 为事件载荷 JSON（waterfall 模式下为上游插件改写后的载荷）。
	//
	// 返回 (resultJSON, err)：
	//   - emit 模式：返回值被宿主忽略；listener 仅产生副作用（如播放音效）
	//   - waterfall 模式：resultJSON 非空表示改写后的载荷透传给下游插件；
	//     err 非空 veto 整条链（停止后续插件与兜底 next）
	//
	// 通知型 listener（如 notify、approval 审计）写法：返回 ("", nil)。
	// 拦截型 listener（如 billion-context）写法：根据 eventType 改写 resultJSON 返回。
	OnEvent func(ctx context.Context, eventType, dataJSON string) (resultJSON string, err error)
	// ContextFn 贡献 system prompt 片段（对齐 DSH ctx.systemPrompt.section）。
	// 任何插件类型（tool/llm/agent/policy/dsc）均可实现。宿主聚合所有插件
	// 的贡献，拼接到 agent 的 system prompt。返回空串表示无贡献。
	// 每次调用求值（动态内容如技能索引安装后即时反映）。
	ContextFn func() string
}

// hookServiceServer 实现宿主 PluginHookService 的适配层。
type hookServiceServer struct {
	proto.UnimplementedPluginHookServiceServer
	hook *Hook
}

func (s *hookServiceServer) BeforeTool(ctx context.Context, req *proto.BeforeToolRequest) (*proto.BeforeToolResponse, error) {
	resp := &proto.BeforeToolResponse{Veto: false, ArgumentsJson: req.GetArgumentsJson()}
	if s.hook == nil || s.hook.BeforeTool == nil {
		return resp, nil
	}
	rewritten, err := s.hook.BeforeTool(ctx, req.GetToolName(), req.GetArgumentsJson())
	if err != nil {
		return &proto.BeforeToolResponse{Veto: true, Error: err.Error(), ArgumentsJson: req.GetArgumentsJson()}, nil
	}
	if rewritten != "" {
		resp.ArgumentsJson = rewritten // 空串 = 保持原样（宿主语义）
	}
	return resp, nil
}

func (s *hookServiceServer) AfterTool(ctx context.Context, req *proto.AfterToolRequest) (*proto.AfterToolResponse, error) {
	resp := &proto.AfterToolResponse{Result: req.GetResult(), Error: req.GetError()}
	if s.hook == nil || s.hook.AfterTool == nil {
		return resp, nil
	}
	newResult, newErr := s.hook.AfterTool(ctx, req.GetToolName(), req.GetArgumentsJson(), req.GetResult(), req.GetError())
	resp.Result = newResult
	resp.Error = newErr
	return resp, nil
}

func (s *hookServiceServer) OnEvent(ctx context.Context, req *proto.OnEventRequest) (*proto.OnEventResponse, error) {
	if s.hook == nil || s.hook.OnEvent == nil {
		// 未实现 OnEvent 的插件：返回空响应，宿主 emit 模式下相当于 no-op
		return &proto.OnEventResponse{}, nil
	}
	// 插件只返回 (resultJSON, err)，宿主侧 dispatchEventToPlugins 负责按顺序
	// 串联每个插件，把上一个的 result 透传给下一个（实现洋葱模型）。
	// 插件侧无需感知 next——next 的语义在宿主侧由调用顺序隐式表达。
	result, err := s.hook.OnEvent(ctx, req.GetName(), req.GetDataJson())
	resp := &proto.OnEventResponse{}
	if result != "" {
		resp.ResultJson = result
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// ListContext 贡献 system prompt 片段（对齐 DSH ctx.systemPrompt.section）。
// 任何插件类型均可实现。未实现 ContextFn 时返回空串（无贡献）。
func (s *hookServiceServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	if s.hook == nil || s.hook.ContextFn == nil {
		return &proto.ListContextResponse{}, nil
	}
	return &proto.ListContextResponse{Content: s.hook.ContextFn()}, nil
}
