package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"dsc-sdk"
	"dsc/proto"
)

// 工具流水线执行槽事件种类（与宿主 core 的 PolicyService 字符串约定一致，
// 对应 proto PolicyEvent.kind / PolicyDecision.action）。
const kindExecute = "tool/execute"

// budget 单工具的超时策略条目：空闲预算来源（env 可调）与超时触发的
// 模型可见文案。超时语义固定为「活跃续命」：执行方每次活动（shell 的每段
// 输出、subagent 的每个 LLM 帧与工具结果）经 TouchActivity 重置计时；
// 只有持续无活动达预算才触发。
type budget struct {
	env        string                       // 空闲预算环境变量（显式 0s 禁用该工具超时）
	defaultVal time.Duration                // 缺省预算
	message    func(d time.Duration) string // 超时触发的模型可见文案
}

// timeoutPolicies 超时策略表：哪些工具在执行时附带活跃续命执行域，预算与
// 文案全部在此（策略归插件，宿主不 crafting 任何领域文案）。未列出的工具
// 不设超时（空裁决 = allow 无执行域）。
var timeoutPolicies = map[string]budget{
	"shell": {
		env:        "DSC_SHELL_TIMEOUT",
		defaultVal: 10 * time.Minute,
		message: func(d time.Duration) string {
			return fmt.Sprintf("command idle timeout (no output for > %s)", d)
		},
	},
	"subagent": {
		env:        "DSC_SUBAGENT_IDLE_TIMEOUT",
		defaultVal: 10 * time.Minute,
		message: func(d time.Duration) string {
			return fmt.Sprintf("subagent idle timeout (no activity for %s; set DSC_SUBAGENT_IDLE_TIMEOUT to adjust)", d)
		},
	},
}

// idleBudget 当前生效的空闲预算：env 显式设置（含 0s 禁用）优先，否则缺省。
func (b budget) idleBudget() time.Duration {
	if s := os.Getenv(b.env); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return b.defaultVal
}

// policyServer 活跃续命超时策略：在 tool/execute 槽为策略表内的工具裁决
// TimeoutSpec（空闲预算 + 超时文案），宿主机械安装为执行域（看门狗 +
// TouchActivity 活动通道）。无状态：超时是执行语义而非观察语义，每次调用
// 独立裁决，无需会话属主状态。
type policyServer struct {
	proto.UnimplementedPolicyServiceServer
}

func newPolicyServer() *policyServer { return &policyServer{} }

// OnEvent 实现 proto.PolicyServiceServer：仅 tool/execute 槽参与裁决；
// 其他槽（pre/post）与表外工具一律放行（空裁决）。
func (s *policyServer) OnEvent(_ context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
	if ev.GetKind() != kindExecute {
		return &proto.PolicyDecision{}, nil
	}
	b, ok := timeoutPolicies[ev.GetTool()]
	if !ok {
		return &proto.PolicyDecision{}, nil
	}
	idle := b.idleBudget()
	if idle <= 0 {
		// env 显式 0s：该工具不设执行域（禁用超时）
		return &proto.PolicyDecision{}, nil
	}
	return &proto.PolicyDecision{
		Action: "allow",
		Timeout: &proto.TimeoutSpec{
			IdleMs:  int64(idle / time.Millisecond),
			Message: b.message(idle),
		},
	}, nil
}

// main 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 PolicyService 与
// PluginMetadata 的 go-core 组装。超时预算与文案全部在本插件——宿主只
// 机械安装执行域与转发活动信号（对齐「policy 插件持有策略，宿主只派发
// 与执行裁决」）。
func main() {
	sdk := dsc.New(dsc.Config{
		Name:    "timeout-policy",
		Version: "1.0.0",
		Type:    dsc.TypePolicy,
		// 声明提供 "timeout-policy" 能力：其他插件若需依赖此策略可经
		// Requires 声明，宿主据此按能力匹配（对齐 DSH/Cordis 的 provide+inject）。
		Provides: map[string]string{"timeout-policy": "true"},
	})
	sdk.Policy(newPolicyServer())
	sdk.Serve()
}
