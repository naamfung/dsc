package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"dsc/proto"
)

// timeout 超时决策插件（第二实例，自 plugins/policy-timeout 迁入的核心插件
// 混合体驻留）：单工具的超时策略条目——空闲预算来源（env 可调）与超时触发的
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

// timeoutServer 活跃续命超时策略服务（dsc-system 驻留）：在 tool/execute 槽
// 为策略表内的工具裁决 TimeoutSpec（空闲预算 + 超时文案），宿主机械安装为
// 执行域（看门狗 + TouchActivity 活动通道）。无状态：超时是执行语义而非观察
// 语义，每次调用独立裁决，无需会话属主状态。
type timeoutServer struct {
	proto.UnimplementedPolicyServiceServer
}

func newTimeoutServer() *timeoutServer { return &timeoutServer{} }

// OnEvent 实现 proto.PolicyServiceServer：仅 tool/execute 槽参与裁决；
// 其他槽（pre/post）与表外工具一律放行（空裁决）。
func (s *timeoutServer) OnEvent(_ context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
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
