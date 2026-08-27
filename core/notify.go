package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dsc/jobs"
	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// 插件通知服务（互通机制 2，插件→宿主）：插件进程经 broker 连接本服务，
// 把事件（含后台任务完成通知）发布到宿主事件总线，供 TUI/其他插件订阅。
// 宿主在 broker 挂载并注入 serviceID（DSC_NOTIFY_SERVICE_ID）到插件进程 env。

// servePluginNotifyLocked 在 agent broker 上挂载 PluginNotifyService（需已持有 m.mu）。
func (m *Manager) servePluginNotifyLocked() (uint32, error) {
	id, err := m.servePluginNotifyOnBroker(m.broker)
	if err == nil {
		m.coreNotifyServiceID = id
	}
	return id, err
}

// servePluginNotifyOnBroker 在指定 broker 上挂载 PluginNotifyService；返回
// serviceID。互通机制 2 中，该服务须挂在本插件 client 的 broker 上（插件
// 进程经自身 broker.Dial 访问），而不仅是 agent broker。
func (m *Manager) servePluginNotifyOnBroker(broker *plugin.GRPCBroker) (uint32, error) {
	if broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve core notify service")
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterPluginNotifyServiceServer(s, &coreNotifyServer{m: m})
		return s
	})
	return serviceID, nil
}

// coreNotifyServer PluginNotifyService 宿主侧实现。
type coreNotifyServer struct {
	proto.UnimplementedPluginNotifyServiceServer
	m *Manager
}

// Notify 把插件事件发布到宿主事件总线：
//   - name == job/done 时特化：data 为任务快照 JSON，宿主解析为 JobSnapshot 后
//     发 JobDoneEvent（复用 TUI 完成通知唤醒体系）；
//   - 其余 name 为插件自定义事件，data 尝试 JSON 解析（失败按原样字符串）。
func (s *coreNotifyServer) Notify(ctx context.Context, req *proto.NotifyRequest) (*proto.NotifyResponse, error) {
	if req.GetName() == string(JobDoneEvent) {
		var snap jobs.JobSnapshot
		if err := json.Unmarshal([]byte(req.GetData()), &snap); err != nil {
			return nil, fmt.Errorf("notify: invalid job/done data: %w", err)
		}
		s.m.events.Emit(JobDoneEvent, EventContext{Data: snap})
		return &proto.NotifyResponse{}, nil
	}
	data := any(req.GetData())
	if req.GetData() != "" {
		var v any
		if err := json.Unmarshal([]byte(req.GetData()), &v); err == nil {
			data = v
		}
	}
	s.m.events.Emit(EventName(req.GetName()), EventContext{Data: data})
	return &proto.NotifyResponse{}, nil
}

// hookClientsSnapshot 返回按加载顺序的插件钩子客户端快照（读锁保护，防热加载竞态）。
func (m *Manager) hookClientsSnapshot() []proto.PluginHookServiceClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]proto.PluginHookServiceClient, 0, len(m.toolHookOrder))
	for _, n := range m.toolHookOrder {
		out = append(out, m.toolHookClients[n])
	}
	return out
}

// runPluginBeforeTool 调用所有插件 BeforeTool 钩子（按加载顺序）：任一 veto
// 阻止执行；参数可被改写（后续用新参数）。插件不可用（UNIMPLEMENTED 等）跳过。
func (m *Manager) runPluginBeforeTool(ctx context.Context, inv *ToolInvocation) error {
	for _, c := range m.hookClientsSnapshot() {
		if c == nil {
			continue
		}
		resp, err := c.BeforeTool(ctx, &proto.BeforeToolRequest{
			ToolName: inv.ToolName, ArgumentsJson: inv.ArgumentsJSON, CallId: inv.CallID,
		})
		if err != nil {
			continue // UNIMPLEMENTED/插件不可用容错
		}
		if resp.GetVeto() {
			if resp.GetError() != "" {
				return fmt.Errorf("core vetoed %s: %s", inv.ToolName, resp.GetError())
			}
			return fmt.Errorf("core vetoed %s", inv.ToolName)
		}
		if resp.GetArgumentsJson() != "" && resp.GetArgumentsJson() != inv.ArgumentsJSON {
			inv.ArgumentsJSON = resp.GetArgumentsJson()
		}
	}
	return nil
}

// runPluginAfterTool 调用所有插件 AfterTool 钩子：可改写结果/错误。
func (m *Manager) runPluginAfterTool(ctx context.Context, inv *ToolInvocation) {
	for _, c := range m.hookClientsSnapshot() {
		if c == nil {
			continue
		}
		resp, err := c.AfterTool(ctx, &proto.AfterToolRequest{
			ToolName: inv.ToolName, ArgumentsJson: inv.ArgumentsJSON, CallId: inv.CallID,
			Result: inv.Result, Error: errString(inv.Err),
		})
		if err != nil {
			continue
		}
		if resp.GetError() != "" {
			inv.Err = fmt.Errorf("%s", resp.GetError())
		} else if resp.GetResult() != "" {
			inv.Result = resp.GetResult()
			inv.Err = nil
		}
	}
}

// errString 错误转字符串（nil → 空）。
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// broadcastEventToPlugins 把宿主事件异步广播给所有插件（互通机制 3 OnEvent）：
// 插件可经此订阅宿主及其他插件（notify 发布）的事件，无需协调发布方。
func (m *Manager) broadcastEventToPlugins(name EventName, data any) {
	clients := m.hookClientsSnapshot()
	if len(clients) == 0 {
		return
	}
	dataJSON := ""
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			dataJSON = string(b)
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, c := range clients {
			if c == nil {
				continue
			}
			_, _ = c.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
		}
	}()
}
