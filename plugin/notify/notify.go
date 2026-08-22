// Package notify 插件侧事件通知适配器（宿主互通机制 2，插件→宿主）：
// 插件进程经 DSC_NOTIFY_SERVICE_ID 环境变量（宿主 pluginEnv 注入）+ broker
// 连接宿主 PluginNotifyService，把事件（含后台任务完成通知）发布到宿主事件
// 总线，供 TUI 唤醒与其他插件订阅。
package notify

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"dsc/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// EnvServiceID DSC_NOTIFY_SERVICE_ID：宿主注入插件进程的通知服务 ID。
const EnvServiceID = "DSC_NOTIFY_SERVICE_ID"

// Notifier 宿主 PluginNotifyService 客户端。
type Notifier struct {
	c    proto.PluginNotifyServiceClient
	conn *grpc.ClientConn
}

// Dial 按 serviceID 建立连接（ID 由宿主经 ToolService.SetInterconnect 传入，
// 挂载在本插件 client 的 broker 上）；broker 为空或 id 为 0 返回 (nil, nil)。
func Dial(broker *goplugin.GRPCBroker, id uint32) (*Notifier, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	conn, err := broker.Dial(id)
	if err != nil {
		return nil, fmt.Errorf("notify: dial notify service %d: %w", id, err)
	}
	return &Notifier{c: proto.NewPluginNotifyServiceClient(conn), conn: conn}, nil
}

// DialFromEnv 读 DSC_NOTIFY_SERVICE_ID 并经 broker 建立连接；宿主未注入该
// 环境变量时返回 (nil, nil)。
func DialFromEnv(broker *goplugin.GRPCBroker) (*Notifier, error) {
	v := os.Getenv(EnvServiceID)
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("notify: invalid %s %q: %w", EnvServiceID, v, err)
	}
	return Dial(broker, uint32(id))
}

// Close 关闭底层连接。
func (n *Notifier) Close() error {
	if n == nil || n.conn == nil {
		return nil
	}
	return n.conn.Close()
}

// Notify 发布事件到宿主事件总线：name 为宿主事件名（"job/done" 或插件自定义），
// data 为 JSON 字符串（name 为 job/done 时须为任务快照）。
func (n *Notifier) Notify(ctx context.Context, name, dataJSON string) error {
	if n == nil || n.c == nil {
		return fmt.Errorf("notify: not connected")
	}
	_, err := n.c.Notify(ctx, &proto.NotifyRequest{Name: name, Data: dataJSON})
	return err
}
