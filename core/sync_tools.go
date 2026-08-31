package core

import (
	"context"
	"encoding/json"
	"time"

	"dsc/proto"
)

// syncMinInterval 聚合工具目录同步的最小间隔（节流）。模型每回合频繁取工具目录，
// 且 tool-lua-host 脚本热加载轮询为 2s，故 800ms 节流足够即时也不至于高频拉全量插件。
const syncMinInterval = 800 * time.Millisecond

// syncPluginTools 节流地重新拉取各 tool 插件的最新工具并同步到共享 registry。
// tool-lua-host 等脚本插件在运行中热加载新增/删除脚本工具后，宿主聚合工具目录
// （模型可直接调用的工具）随本同步更新，使新脚本工具无需重启即可被模型直接调用。
// 幂等：对工具集无变化的插件不产生任何 registry 变动。
func (m *Manager) syncPluginTools() {
	// 节流：取快照（不持锁做 gRPC），命中节流窗则跳过。
	m.mu.Lock()
	if time.Since(m.lastToolSync) < syncMinInterval {
		m.mu.Unlock()
		return
	}
	m.lastToolSync = time.Now()
	clients := make(map[string]proto.ToolServiceClient, len(m.toolClients))
	for name, c := range m.toolClients {
		clients[name] = c
	}
	m.mu.Unlock()

	for name, client := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		resp, err := client.ListTools(ctx, &proto.ListToolsRequest{})
		cancel()
		if err != nil {
			m.logger.Warn("sync plugin tools failed", "plugin", name, "error", err)
			continue
		}
		defs := make([]ToolDefinition, 0, len(resp.Tools))
		names := make([]string, 0, len(resp.Tools))
		for _, t := range resp.Tools {
			defs = append(defs, &RemoteTool{
				name:        t.Name,
				description: t.Description,
				schema:      json.RawMessage(t.ParametersJson),
				client:      client,
			})
			names = append(names, t.Name)
		}
		m.applyPluginTools(name, defs, names)
	}
}

// applyPluginTools 将某插件的最新工具集与共享 registry 对齐（新增/删除）。
// 以插件名维度 diff，替换 coreToolNames[name]；对工具集无变化的插件不做任何改动。
func (m *Manager) applyPluginTools(name string, defs []ToolDefinition, names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	old, _ := m.coreToolNames[name]
	oldSet := make(map[string]bool, len(old))
	for _, n := range old {
		oldSet[n] = true
	}
	newSet := make(map[string]bool, len(names))
	for _, n := range names {
		newSet[n] = true
	}

	// 无变化则跳过
	if len(old) == len(names) {
		same := true
		for _, n := range names {
			if !oldSet[n] {
				same = false
				break
			}
		}
		if same {
			return
		}
	}

	nameByDef := make(map[string]ToolDefinition, len(defs))
	for _, d := range defs {
		nameByDef[d.Name()] = d
	}

	svcID := m.toolServiceIDs[name]
	// 新增
	for _, n := range names {
		if oldSet[n] {
			continue
		}
		d, ok := nameByDef[n]
		if !ok {
			continue
		}
		if err := m.toolRegistry.Register(d); err != nil {
			m.logger.Warn("hot-sync register tool", "tool", n, "plugin", name, "error", err)
			continue
		}
		m.toolNameToServiceID[n] = svcID
		m.logger.Info("hot-synced new tool", "tool", n, "plugin", name)
	}
	// 删除
	for _, n := range old {
		if newSet[n] {
			continue
		}
		m.toolRegistry.Unregister(n)
		delete(m.toolNameToServiceID, n)
		m.logger.Info("hot-synced removed tool", "tool", n, "plugin", name)
	}

	m.coreToolNames[name] = names
}
