package core

import (
	"fmt"

	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"dsc/proto"
)

// attachAgentAggregatesOnBroker 在指定 broker 上挂载主 agent 所需的聚合服务
// （聚合 LLM、聚合 Tool、UserQuestions），返回各自 serviceID。
//
// 关键语义：go-plugin broker 的 (serviceID, addr) 连接信息是 per-connection 的——
// AcceptAndServe 仅把该元组沿「当前 broker 绑定的连接」通告给唯一对端插件进程；
// 跨连接（另一插件进程）用同一 serviceID Dial 会因无通告而超时。因此无论初次加载
// 还是热重载，聚合服务都必须挂载到「目标 agent 自己的 broker」上，并以本次返回的
// 新 serviceID 注入该 agent——**不能沿用先前连接上的旧 id**。这正是 P1-2（热重载后
// 新 agent 复用旧 serviceID 连不上聚合服务而失联）的修复锚点。
//
// 注意：那些 id 的「连接信息」为一次性发送且有 connTimeout 窗口，故挂载后调用方
// 须随即以返回的 id 调 RegisterServices / SetUserQuestionsService，让 agent 尽快 Dial。
//
// 本函数只完成「挂载与服务」，不写 m.* 聚合 id 字段；字段回写由调用方在持锁交换
// 新 broker 时进行，避免与 m.mu 同步错位。
func (m *Manager) attachAgentAggregatesOnBroker(broker *plugin.GRPCBroker, hasLLM bool, primary string, attachTool bool) (llmID, toolID, uqID uint32) {
	if broker == nil {
		m.logger.Warn("aggregate attach skipped: broker not available")
		return
	}
	if hasLLM && primary != "" {
		if id, err := m.serveAggregateLLMOnBroker(broker, primary); err == nil {
			llmID = id
		} else {
			m.logger.Warn("aggregate llm service unavailable", "error", err)
		}
	}
	if attachTool {
		if id, err := m.serveAggregateToolOnBroker(broker); err == nil {
			toolID = id
		} else {
			m.logger.Warn("aggregate tool service unavailable", "error", err)
		}
	}
	if id, err := m.serveUserQuestionsOnBroker(broker); err == nil {
		uqID = id
	} else {
		m.logger.Warn("user-questions service unavailable", "error", err)
	}
	return
}

// serveUserQuestionsOnBroker 在指定 broker 上挂载 UserQuestionsService 并返回
// serviceID（不写 m.userQuestionsServiceID）。供 attachAgentAggregatesOnBroker
// 与热重载在目标 agent 自己的 broker 上挂载使用。
func (m *Manager) serveUserQuestionsOnBroker(broker *plugin.GRPCBroker) (uint32, error) {
	if broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve user-questions service")
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterUserQuestionsServiceServer(s, &userQuestionsGRPCServer{m: m})
		return s
	})
	return serviceID, nil
}
