package plugin

import (
	"context"
	"fmt"

	"dsc/proto"
	"dsc/userquestions"
	"google.golang.org/grpc"
)

// 用户评审通道（对齐 DSH user-questions）：宿主在 broker 上挂载
// UserQuestionsService；agent 侧工具（如 exit_plan_mode）经 gRPC 调用 Ask，
// 宿主转发给注册的 UI provider（TUI）并阻塞等待用户回答。

// UserQuestionProvider 宿主侧 UI provider：接收提问请求，返回用户回答。
// 实现需感知 ctx 取消（用户放弃/调用中止时返回错误）。
type UserQuestionProvider func(ctx context.Context, req *userquestions.Request) (*userquestions.Answer, error)

// RegisterUserQuestionProvider 注册唯一的 UI provider（TUI 启动时调用；
// 已注册时返回错误，对齐 DSH DUPLICATE_PROVIDER）。
func (m *Manager) RegisterUserQuestionProvider(p UserQuestionProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.userQuestionProvider != nil {
		return &userquestions.Error{Code: "DUPLICATE_PROVIDER", Err: fmt.Errorf("a user-questions provider is already registered")}
	}
	m.userQuestionProvider = p
	return nil
}

// serveUserQuestionsLocked 在 broker 上挂载 UserQuestionsService 并返回 serviceID
// （需已持有 m.mu）。
func (m *Manager) serveUserQuestionsLocked() (uint32, error) {
	if m.broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve user-questions service")
	}
	serviceID := m.broker.NextId()
	go m.broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterUserQuestionsServiceServer(s, &userQuestionsGRPCServer{m: m})
		return s
	})
	m.userQuestionsServiceID = serviceID
	return serviceID, nil
}

// Ask 宿主侧 Ask 实现：校验请求 → 调 provider → 返回回答。
func (m *Manager) Ask(ctx context.Context, req *userquestions.Request) (*userquestions.Answer, error) {
	if err := userquestions.Validate(req); err != nil {
		return nil, err
	}
	m.mu.Lock()
	provider := m.userQuestionProvider
	m.mu.Unlock()
	if provider == nil {
		return nil, &userquestions.Error{Code: userquestions.ErrNoProvider, Err: fmt.Errorf("no user-questions provider is registered")}
	}
	return provider(ctx, req)
}

// userQuestionsGRPCServer 暴露给 agent 的 gRPC 服务端。
type userQuestionsGRPCServer struct {
	proto.UnimplementedUserQuestionsServiceServer
	m *Manager
}

func (s *userQuestionsGRPCServer) Ask(ctx context.Context, req *proto.AskRequest) (*proto.AskResponse, error) {
	r := protoAskToDomain(req)
	ans, err := s.m.Ask(ctx, r)
	if err != nil {
		return &proto.AskResponse{Error: errorCode(err), Message: err.Error()}, nil
	}
	resp := &proto.AskResponse{}
	for _, a := range ans.Answers {
		resp.Answers = append(resp.Answers, &proto.AskAnswer{
			Id: a.ID, Selected: a.Selected, Custom: a.Custom,
		})
	}
	return resp, nil
}

// protoAskToDomain 把 gRPC 请求转成领域类型。
func protoAskToDomain(req *proto.AskRequest) *userquestions.Request {
	out := &userquestions.Request{}
	for _, q := range req.GetQuestions() {
		dq := userquestions.Question{
			ID: q.Id, Question: q.Question, Detail: q.Detail, Header: q.Header,
			MultiSelect: q.MultiSelect,
		}
		for _, o := range q.GetOptions() {
			dq.Options = append(dq.Options, userquestions.Option{Label: o.Label, Description: o.Description})
		}
		if it := q.GetIntent(); it != nil {
			dq.Intent = &userquestions.Intent{Kind: it.Kind, Approve: it.Approve}
		}
		out.Questions = append(out.Questions, dq)
	}
	return out
}

// errorCode 从错误中提取稳定错误码（对齐 DSH UserQuestionError codes）。
// provider 应返回带 code 的 *userquestions.Error；其余按内部错误处理。
func errorCode(err error) string {
	if ue, ok := err.(*userquestions.Error); ok {
		return ue.Code
	}
	return "INTERNAL"
}
