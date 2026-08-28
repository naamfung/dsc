// Package agentclient 插件侧主 agent 桥接客户端（宿主互通机制 1/2）：
// 插件进程经 ToolService.SetInterconnect 传入的 agent 桥接服务 ID + broker
// 连接宿主挂载的主 agent 服务，获得与主 agent 对话（RunStream）与实时注入
// 消息（InjectMessage）的能力。仅暴露这两种对话能力，不含生命周期管理，
// 避免插件能关闭/重载宿主主 agent。
package agentclient

import (
	"context"
	"fmt"
	"io"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Client 主 agent 桥接服务客户端（插件→宿主主 agent）。
type Client struct {
	c    proto.AgentServiceClient
	conn *grpc.ClientConn
}

// Dial 按 serviceID 建立连接（ID 由宿主经 ToolService.SetInterconnect 传入，
// 挂载在本插件 client 的 broker 上）；broker 为空或 id 为 0 返回 (nil, nil)。
func Dial(broker *plugin.GRPCBroker, id uint32) (*Client, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	conn, err := broker.Dial(id)
	if err != nil {
		return nil, fmt.Errorf("agentclient: dial agent bridge service %d: %w", id, err)
	}
	return &Client{c: proto.NewAgentServiceClient(conn), conn: conn}, nil
}

// Close 关闭底层连接。
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// StreamItem 一帧对话输出：把跨进程 RunStreamResponse 的语义收敛为插件可用的
// 精简形态（状态 + 输出/错误），屏蔽 proto 细节。
type StreamItem struct {
	Status string
	Output string
	Error  string
}

// RunStream 以与主 agent 内部一致的流式方式运行一轮对话，返回帧通道。
// 未连接时返回 nil 通道与错误。
func (c *Client) RunStream(ctx context.Context, input string) (<-chan *StreamItem, error) {
	if c == nil || c.c == nil {
		return nil, fmt.Errorf("agentclient: not connected")
	}
	stream, err := c.c.RunStream(ctx, &proto.RunRequest{Input: input})
	if err != nil {
		return nil, err
	}
	ch := make(chan *StreamItem)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &StreamItem{Status: "error", Error: err.Error()}
				return
			}
			ch <- &StreamItem{Status: resp.Status, Output: resp.Output, Error: resp.Error}
		}
	}()
	return ch, nil
}

// InjectMessage 把一条用户消息实时注入主 agent 当前会话（下一轮 LLM 迭代可见）。
func (c *Client) InjectMessage(ctx context.Context, content string) error {
	if c == nil || c.c == nil {
		return fmt.Errorf("agentclient: not connected")
	}
	_, err := c.c.InjectMessage(ctx, &proto.InjectMessageRequest{Content: content})
	return err
}
