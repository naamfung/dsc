package dsc

import (
	"context"

	"dsc/proto"
)

// Hook 插件钩子（语义化封装宿主 proto.PluginHookService）：
// 宿主在工具流水线执行前调用 BeforeTool（可否决/改写参数），执行后调用
// AfterTool（可改写结果/错误），并把宿主事件异步广播给 OnEvent。
// 任一字段均可省略（nil 表示不参与该环节）。
type Hook struct {
	// BeforeTool 返回改写后的参数 JSON；err 非 nil 视为否决（阻止工具执行，
	// 错误文本会反馈给调用方）。
	BeforeTool func(ctx context.Context, toolName, argumentsJSON string) (rewrittenJSON string, err error)
	// AfterTool 返回改写后的结果与错误文本。
	AfterTool func(ctx context.Context, toolName, result, toolErr string) (newResult, newErr string)
	// OnEvent 订阅宿主事件（异步广播；eventType 如 turn/start、tool/result 等）。
	OnEvent func(ctx context.Context, eventType, dataJSON string)
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
	newResult, newErr := s.hook.AfterTool(ctx, req.GetToolName(), req.GetResult(), req.GetError())
	resp.Result = newResult
	resp.Error = newErr
	return resp, nil
}

func (s *hookServiceServer) OnEvent(ctx context.Context, req *proto.OnEventRequest) (*proto.OnEventResponse, error) {
	if s.hook != nil && s.hook.OnEvent != nil {
		s.hook.OnEvent(ctx, req.GetName(), req.GetDataJson())
	}
	return &proto.OnEventResponse{}, nil
}
