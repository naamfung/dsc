// Package llmclient 插件侧 LLM 服务适配器（宿主互通机制）：
// 插件进程经 DSC_LLM_SERVICE_ID 环境变量（宿主 pluginEnv 注入）+ broker 连接
// 宿主聚合 LLM 服务，复用 llm-openai / llm-anthropic / llm-ollama 三个 LLM
// 插件的能力（含多 provider 聚合路由），无需自带 LLM client。
package llmclient

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"dsc/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// EnvServiceID DSC_LLM_SERVICE_ID：宿主注入插件进程的聚合 LLM 服务 ID。
const EnvServiceID = "DSC_LLM_SERVICE_ID"

// Client 聚合 LLM 服务客户端（面向插件的简化接口：非流式 Chat）。
type Client struct {
	c    proto.LLMServiceClient
	conn *grpc.ClientConn
}

// Dial 按 serviceID 建立连接（ID 由宿主经 ToolService.SetInterconnect 传入，
// 挂载在本插件 client 的 broker 上）；broker 为空或 id 为 0 返回 (nil, nil)。
func Dial(broker *goplugin.GRPCBroker, id uint32) (*Client, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	conn, err := broker.Dial(id)
	if err != nil {
		return nil, fmt.Errorf("llmclient: dial LLM service %d: %w", id, err)
	}
	return &Client{c: proto.NewLLMServiceClient(conn), conn: conn}, nil
}

// DialFromEnv 读 DSC_LLM_SERVICE_ID 并经 broker 建立连接；宿主未注入该环境
// 变量时返回 (nil, nil)（插件可自行降级）。
func DialFromEnv(broker *goplugin.GRPCBroker) (*Client, error) {
	v := os.Getenv(EnvServiceID)
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("llmclient: invalid %s %q: %w", EnvServiceID, v, err)
	}
	return Dial(broker, uint32(id))
}

// Close 关闭底层连接。
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Chat 执行一次非流式对话（经宿主聚合路由；maxTokens 0 = 服务端默认）。
// 注意：thinking 模式下 unary Chat 可能只返回思考块而文本为空，优先用 ChatStream。
func (c *Client) Chat(ctx context.Context, messages []*proto.Message, maxTokens int32) (*proto.ChatResponse, error) {
	return c.c.Chat(ctx, &proto.ChatRequest{Messages: messages, MaxTokens: maxTokens})
}

// ChatStream 执行一次流式对话（经宿主聚合路由），逐帧返回。
func (c *Client) ChatStream(ctx context.Context, messages []*proto.Message, maxTokens int32) (proto.LLMService_ChatStreamClient, error) {
	return c.c.ChatStream(ctx, &proto.ChatRequest{Messages: messages, MaxTokens: maxTokens})
}
