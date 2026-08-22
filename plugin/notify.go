package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/jobs"
	"dsc/proto"
	"google.golang.org/grpc"
)

// 插件通知服务（互通机制 2，插件→宿主）：插件进程经 broker 连接本服务，
// 把事件（含后台任务完成通知）发布到宿主事件总线，供 TUI/其他插件订阅。
// 宿主在 broker 挂载并注入 serviceID（DSC_NOTIFY_SERVICE_ID）到插件进程 env。

// servePluginNotifyLocked 在 broker 上挂载 PluginNotifyService（需已持有 m.mu）。
func (m *Manager) servePluginNotifyLocked() (uint32, error) {
	if m.broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve plugin notify service")
	}
	serviceID := m.broker.NextId()
	go m.broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterPluginNotifyServiceServer(s, &pluginNotifyServer{m: m})
		return s
	})
	m.pluginNotifyServiceID = serviceID
	return serviceID, nil
}

// pluginNotifyServer PluginNotifyService 宿主侧实现。
type pluginNotifyServer struct {
	proto.UnimplementedPluginNotifyServiceServer
	m *Manager
}

// Notify 把插件事件发布到宿主事件总线：
//   - name == job/done 时特化：data 为任务快照 JSON，宿主解析为 JobSnapshot 后
//     发 JobDoneEvent（复用 TUI 完成通知唤醒体系）；
//   - 其余 name 为插件自定义事件，data 尝试 JSON 解析（失败按原样字符串）。
func (s *pluginNotifyServer) Notify(ctx context.Context, req *proto.NotifyRequest) (*proto.NotifyResponse, error) {
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
