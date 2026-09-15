package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
	"dsc/session"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// 本文件回归「LLM 调用全程留痕 + 回合闭合不变量」：
//  1. LLM 调用无论成败均落 llm/attempt（log-only）——排障无需审计代码；
//  2. 失败/取消路径由闭合不变量兜底，turn/end 恒以某 reason 收口，
//     日志中不存在悬空 turn/start / step/start。
//     （历史顽疾：finish_reason/LLM 报错在会话日志中零痕迹，Task 6/7/8 类
//     问题每次都需深入代码审计才能归因。）

// errMockLLM 每次调用都失败（模拟 provider 不可达/被拒）。
type errMockLLM struct {
	proto.LLMServiceClient
	err error
}

func (m *errMockLLM) Chat(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (*proto.ChatResponse, error) {
	return nil, m.err
}

// midFailStream 先产出一帧内容后报错（模拟流中途断开）。
type midFailStream struct {
	proto.LLMServiceClient
	recv int
}

func (m *midFailStream) ChatStream(ctx context.Context, in *proto.ChatRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[proto.ChatStreamResponse], error) {
	return m, nil
}

func (m *midFailStream) Recv() (*proto.ChatStreamResponse, error) {
	m.recv++
	if m.recv == 1 {
		return &proto.ChatStreamResponse{Content: "部分内容"}, nil
	}
	return nil, errors.New("connection reset by peer")
}

func (m *midFailStream) Header() (metadata.MD, error) { return nil, nil }
func (m *midFailStream) Trailer() metadata.MD         { return nil }
func (m *midFailStream) CloseSend() error             { return nil }
func (m *midFailStream) Context() context.Context     { return context.Background() }
func (m *midFailStream) SendMsg(any) error            { return nil }
func (m *midFailStream) RecvMsg(any) error            { return nil }

// collectAttempts 折叠会话中的 llm/attempt 事件。
func collectAttempts(t *testing.T, a *ReactLoopAgent) []*session.LLMAttemptData {
	t.Helper()
	var out []*session.LLMAttemptData
	for _, ev := range a.sess.Events() {
		if ev.Type == session.LLMAttempt {
			d, ok := ev.Data.(*session.LLMAttemptData)
			if !ok {
				t.Fatalf("llm/attempt data type %T", ev.Data)
			}
			out = append(out, d)
		}
	}
	return out
}

// assertTurnClosed 校验回合闭合不变量：turn/end 存在且 reason 指定，
// turn/start 与 step/start 均有配对收口。
func assertTurnClosed(t *testing.T, a *ReactLoopAgent, wantReason string) {
	t.Helper()
	var turnStarts, turnEnds, stepStarts, stepEnds int
	var endReason string
	for _, ev := range a.sess.Events() {
		switch ev.Type {
		case session.TurnStart:
			turnStarts++
		case session.TurnEnd:
			turnEnds++
			endReason = ev.Data.(*session.TurnData).Reason
		case session.StepStart:
			stepStarts++
		case session.StepEnd:
			stepEnds++
		}
	}
	if turnStarts != turnEnds {
		t.Fatalf("turn/start=%d 与 turn/end=%d 不配对（悬空回合）", turnStarts, turnEnds)
	}
	if stepStarts != stepEnds {
		t.Fatalf("step/start=%d 与 step/end=%d 不配对（悬空步骤）", stepStarts, stepEnds)
	}
	if endReason != wantReason {
		t.Fatalf("turn/end reason = %q, 期望 %q", endReason, wantReason)
	}
}

// TestLLMFailureRecordsAttemptAndClosesTurn 非流式失败：llm/attempt 携带错误
// 与稳定错误码；turn 以 error 收口（此前直接 return，turn/start 悬空）。
func TestLLMFailureRecordsAttemptAndClosesTurn(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	a.llmClient = &errMockLLM{err: errors.New("dial tcp: connection refused")}
	a.toolClient = &mockToolClient{}

	_, err := a.runLoop(context.Background(), "测试", nil, nil)
	if err == nil {
		t.Fatalf("runLoop 应返回 LLM 错误")
	}

	attempts := collectAttempts(t, a)
	if len(attempts) != 1 {
		t.Fatalf("llm/attempt 事件数 = %d, 期望 1", len(attempts))
	}
	d := attempts[0]
	if d.Error == "" || !strings.Contains(d.Error, "connection refused") {
		t.Fatalf("失败留痕未携带错误文本: %+v", d)
	}
	if d.Code != "network_error" {
		t.Fatalf("错误码 = %q, 期望 network_error", d.Code)
	}
	if d.Streaming {
		t.Fatalf("非流式路径 Streaming 应为 false")
	}
	if d.DurationMS < 0 {
		t.Fatalf("时长不应为负: %d", d.DurationMS)
	}
	assertTurnClosed(t, a, "error")
}

// TestStreamMidFailureRecordsPartialContent 流中途断开：留痕携带已收到的
// 部分内容长度（完整分片另见 assistant/chunk），turn 以 error 收口。
func TestStreamMidFailureRecordsPartialContent(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	a.llmClient = &midFailStream{}
	a.toolClient = &mockToolClient{}

	var frames []*core.RunStreamResponse
	emit := func(f *core.RunStreamResponse) { frames = append(frames, f) }
	_, err := a.runLoop(context.Background(), "测试", nil, emit)
	if err == nil {
		t.Fatalf("runLoop 应返回流错误")
	}

	attempts := collectAttempts(t, a)
	if len(attempts) != 1 {
		t.Fatalf("llm/attempt 事件数 = %d, 期望 1", len(attempts))
	}
	d := attempts[0]
	if !d.Streaming {
		t.Fatalf("流式路径 Streaming 应为 true")
	}
	if d.ContentChars != len("部分内容") {
		t.Fatalf("ContentChars = %d, 期望部分内容长度 %d", d.ContentChars, len("部分内容"))
	}
	if d.Code != "network_error" || !strings.Contains(d.Error, "connection reset") {
		t.Fatalf("失败留痕字段不符: %+v", d)
	}
	assertTurnClosed(t, a, "error")
}

// TestSuccessAttemptCarriesFinishReason 成功路径：留痕携带 finish_reason 与
// 流式用量（finish 分片帧内值）。
func TestSuccessAttemptCarriesFinishReason(t *testing.T) {
	a := newTestAgent(t)
	a.llmServiceID = 1
	a.toolServiceID = 1
	llm := &truncMockLLM{
		firstContent:  "部分回答",
		firstFinish:   "max_tokens",
		secondContent: "续写完成",
	}
	a.llmClient = llm
	a.toolClient = &mockToolClient{}

	res, err := a.runLoop(context.Background(), "测试", nil, func(*core.RunStreamResponse) {})
	if err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("结果状态 = %s", res.Status)
	}
	attempts := collectAttempts(t, a)
	if len(attempts) != 1 {
		t.Fatalf("llm/attempt 事件数 = %d, 期望 1", len(attempts))
	}
	d := attempts[0]
	if d.FinishReason != "max_tokens" || d.Error != "" || d.Code != "" {
		t.Fatalf("成功留痕字段不符: %+v", d)
	}
	assertTurnClosed(t, a, "completed")
}
