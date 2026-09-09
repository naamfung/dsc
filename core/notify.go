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

// dispatchEventToPlugins 把宿主事件同步分发到所有插件（互通机制 3 OnEvent）：
// 插件可经此订阅宿主及其他插件（notify 发布）的事件，无需协调发布方。
//
// 分发模式由事件名决定（见 eventDispatchMode 映射表，对齐 DSH Cordis ctx.on()）：
//   - emit（默认）：同步顺序调用所有插件，忽略返回值；插件内部可自行 go func 派发副作用
//   - waterfall：洋葱模型，每个插件可调 next 委托下游，返回非空 result_json 改写载荷
//     透传给下游；返回非空 error 短路链视为 veto
//   - serial/bail：error 非空使链停止（bail 短路返回，serial 错误聚合）
//
// 同步语义：宿主等待所有插件返回后才继续（避免异步竞态）。
// 通知型插件（如 notify）若需异步副作用（如播放音效），自行在 handler 内 go func()。
//
// waterfall 模式下返回 (result, err)：result 为最终改写后的载荷（any 类型，调用方
// 自行类型断言）；err 非 nil 表示链被 veto（含插件返回的 error 文本）。
func (m *Manager) dispatchEventToPlugins(name EventName, data any) (any, error) {
        clients := m.hookClientsSnapshot()
        if len(clients) == 0 {
                return data, nil
        }
        dataJSON := ""
        if data != nil {
                if b, err := json.Marshal(data); err == nil {
                        dataJSON = string(b)
                }
        }
        mode := eventDispatchMode[name]
        if mode == "" {
                mode = "emit"
        }

        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        switch mode {
        case "waterfall":
                // 洋葱模型：从尾向前包装，每个插件可改写 result_json
                // 初始 result 为原始 dataJSON；每个插件收到的 dataJSON 是上游改写后的
                next := func() (string, error) { return dataJSON, nil }
                for i := len(clients) - 1; i >= 0; i-- {
                        c := clients[i]
                        if c == nil {
                                continue
                        }
                        innerNext := next
                        next = func() (string, error) {
                                resp, err := c.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
                                if err != nil {
                                        return "", err
                                }
                                if resp.GetError() != "" {
                                        return "", fmt.Errorf("%s", resp.GetError())
                                }
                                if r := resp.GetResultJson(); r != "" {
                                        dataJSON = r // 改写后的载荷透传给下游
                                }
                                return innerNext()
                        }
                }
                finalResult, err := next()
                if err != nil {
                        return nil, err
                }
                // 反序列化最终 result 为 any（调用方自行类型断言）
                var result any
                if finalResult != "" {
                        _ = json.Unmarshal([]byte(finalResult), &result)
                }
                return result, nil

        case "serial":
                // 顺序调用，遇 error 即停
                for _, c := range clients {
                        if c == nil {
                                continue
                        }
                        resp, err := c.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
                        if err != nil {
                                return nil, err
                        }
                        if resp.GetError() != "" {
                                return nil, fmt.Errorf("%s", resp.GetError())
                        }
                }
                return data, nil

        case "bail":
                // 首个非空 result 或 error 短路返回
                for _, c := range clients {
                        if c == nil {
                                continue
                        }
                        resp, err := c.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
                        if err != nil {
                                return nil, err
                        }
                        if resp.GetError() != "" {
                                return nil, fmt.Errorf("%s", resp.GetError())
                        }
                        if r := resp.GetResultJson(); r != "" {
                                var result any
                                _ = json.Unmarshal([]byte(r), &result)
                                return result, nil
                        }
                }
                return data, nil

        default: // "emit"
                // 同步顺序调用，忽略返回值（兼容现有通知型 listener）
                for _, c := range clients {
                        if c == nil {
                                continue
                        }
                        _, _ = c.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
                }
                return data, nil
        }
}
