package dsc

import (
	"fmt"

	"dsc/core/llmclient"
	"dsc/core/notify"
	"dsc/core/toolclient"
	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// AgentBroker 面向 agent 类型插件的宿主服务接入点：SDK 对 go-core GRPCBroker
// 的隔离封装，插件作者无需 import go-core（也无需在 go.mod 中 replace 到本
// 仓库定制的 libs/go-core）。
//
// agent 在 sdk.AgentBroker 回调中缓存本对象；待宿主经 RegisterServices /
// SetUserQuestionsService 下发 serviceID 后，用 Dial* 建立对应宿主服务的连接。
// 便捷方法（DialLLM/DialTool/DialNotify/DialUserQuestions）返回封装客户端；
// 需要自建 proto client 时用 Dial 拿原始 *grpc.ClientConn。
type AgentBroker struct {
	broker *plugin.GRPCBroker
}

// Dial 按宿主下发的 serviceID 建立 gRPC 连接（go-core broker.Dial 的封装）。
// 返回 *grpc.ClientConn，可自建任意 proto client。
func (b *AgentBroker) Dial(serviceID uint32) (*grpc.ClientConn, error) {
	if b == nil || b.broker == nil {
		return nil, fmt.Errorf("dsc-sdk: agent broker not available")
	}
	return b.broker.Dial(serviceID)
}

// DialLLM 建立宿主聚合 LLM 客户端（nil 接收者或 serviceID 为 0 时返回 (nil, nil)）。
func (b *AgentBroker) DialLLM(serviceID uint32) (*llmclient.Client, error) {
	if b == nil {
		return nil, nil
	}
	return llmDial(b.broker, serviceID)
}

// DialTool 建立宿主聚合 Tool 客户端（nil 接收者或 serviceID 为 0 时返回 (nil, nil)）。
func (b *AgentBroker) DialTool(serviceID uint32) (*toolclient.Client, error) {
	if b == nil {
		return nil, nil
	}
	return toolDial(b.broker, serviceID)
}

// DialNotify 建立宿主插件通知客户端（nil 接收者或 serviceID 为 0 时返回 (nil, nil)）。
func (b *AgentBroker) DialNotify(serviceID uint32) (*notify.Notifier, error) {
	if b == nil {
		return nil, nil
	}
	return notifyDial(b.broker, serviceID)
}

// DialUserQuestions 建立宿主用户评审（UserQuestionsService）客户端
// （serviceID 为 0 时返回 (nil, nil)）。
func (b *AgentBroker) DialUserQuestions(serviceID uint32) (proto.UserQuestionsServiceClient, error) {
	if b == nil || b.broker == nil || serviceID == 0 {
		return nil, nil
	}
	conn, err := b.broker.Dial(serviceID)
	if err != nil {
		return nil, fmt.Errorf("dsc-sdk: dial user-questions service: %w", err)
	}
	return proto.NewUserQuestionsServiceClient(conn), nil
}
