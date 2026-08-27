// Package toolclient 插件侧聚合 Tool 服务适配器（宿主互通机制 4）：
// 插件进程经 ToolService.SetInterconnect 传入的 serviceID + broker 连接宿主
// 聚合 Tool 服务，经宿主 ExecuteTool 流水线转发到任意工具插件（含策略/钩子），
// 使「宿主内工具」（如 tool-lua-host 的 dsc.tool.call）可以调用其他工具插件。
package toolclient

import (
	"context"
	"fmt"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Client 聚合 Tool 服务客户端。
type Client struct {
	c    proto.ToolServiceClient
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
		return nil, fmt.Errorf("toolclient: dial tool service %d: %w", id, err)
	}
	return &Client{c: proto.NewToolServiceClient(conn), conn: conn}, nil
}

// Close 关闭底层连接。
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// ListTools 返回宿主聚合工具目录。
func (c *Client) ListTools(ctx context.Context) ([]*proto.Tool, error) {
	if c == nil || c.c == nil {
		return nil, fmt.Errorf("toolclient: not connected")
	}
	resp, err := c.c.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// ExecuteTool 经宿主流水线执行任意工具插件；返回结果文本与错误。
func (c *Client) ExecuteTool(ctx context.Context, name, argumentsJSON string) (string, error) {
	if c == nil || c.c == nil {
		return "", fmt.Errorf("toolclient: not connected")
	}
	resp, err := c.c.ExecuteTool(ctx, &proto.ExecuteToolRequest{
		ToolName:      name,
		ArgumentsJson: argumentsJSON,
	})
	if err != nil {
		return "", err
	}
	if resp.GetError() != "" {
		return "", fmt.Errorf("%s", resp.GetError())
	}
	return resp.GetContent(), nil
}
