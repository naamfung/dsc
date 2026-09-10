package core

import (
	"fmt"
)

// 目标轮次驱动器（对齐 DSH goal-round-driver）。
//
// DSH 的 goal-round-driver 在模型自然停止（无工具调用）后检查：
//   - goal active + armed + 预算未耗尽 → 准入下一轮 goal-round 用户消息
//   - goalRounds 记录已准入的轮次数
//   - MaxGoalRounds 限制最大续行轮次
//
// DSC 当前已有 goalRoundDriver 函数（在 agent-react-loop/main.go 中内联实现），
// 但 goalRounds 恒为 0（v1 预算未接入）。本模块把轮次驱动逻辑抽为独立模块，
// 使其可测试、可复用、可扩展。

// GoalRoundConfig 目标轮次驱动器配置（对齐 DSH goal-round-driver 配置）。
type GoalRoundConfig struct {
	// MaxGoalRounds 最大续行轮次（0 = 不限制）。
	MaxGoalRounds int
	// GoalRoundPrompt 续行提示词模板（{round} 替换为当前轮次，{max} 替换为最大轮次）。
	GoalRoundPrompt string
}

// DefaultGoalRoundConfig 默认配置（对齐 DSH 默认值）。
func DefaultGoalRoundConfig() GoalRoundConfig {
	return GoalRoundConfig{
		MaxGoalRounds: 5,
		GoalRoundPrompt: "你之前设定了一个目标并正在执行中。请继续推进目标 {round}/{max}。" +
			"如果目标已完成，请用 complete_goal 工具标记完成；如果遇到阻塞，请用 update_goal 标记 blocked。",
	}
}

// GoalRoundDriver 目标轮次驱动器（对齐 DSH goal-round-driver）。
type GoalRoundDriver struct {
	config GoalRoundConfig
	rounds int  // 已准入轮次
	active bool // 是否激活
	armed  bool // 是否武装（恢复/fork 后停用）
}

// NewGoalRoundDriver 创建目标轮次驱动器。
func NewGoalRoundDriver(config GoalRoundConfig) *GoalRoundDriver {
	return &GoalRoundDriver{config: config}
}

// Activate 激活目标续行（模型调用 create_goal 时激活）。
func (d *GoalRoundDriver) Activate() {
	d.active = true
	d.armed = true
	d.rounds = 0
}

// Disarm 停用续行（恢复/fork 后停用，需显式 resume 重新武装）。
func (d *GoalRoundDriver) Disarm() {
	d.armed = false
}

// ShouldContinue 检查是否应继续下一轮目标续行。
// 条件：active + armed + 预算未耗尽。
func (d *GoalRoundDriver) ShouldContinue() bool {
	if !d.active || !d.armed {
		return false
	}
	if d.config.MaxGoalRounds > 0 && d.rounds >= d.config.MaxGoalRounds {
		return false
	}
	return true
}

// Advance 准入下一轮，返回续行提示词。
func (d *GoalRoundDriver) Advance() string {
	d.rounds++
	return d.formatPrompt()
}

// Complete 目标完成，停用续行。
func (d *GoalRoundDriver) Complete() {
	d.active = false
}

// Rounds 返回已准入轮次数。
func (d *GoalRoundDriver) Rounds() int {
	return d.rounds
}

// Active 返回是否激活。
func (d *GoalRoundDriver) Active() bool {
	return d.active
}

// formatPrompt 格式化续行提示词。
func (d *GoalRoundDriver) formatPrompt() string {
	prompt := d.config.GoalRoundPrompt
	prompt = replaceAllStr(prompt, "{round}", fmt.Sprintf("%d", d.rounds))
	maxStr := "∞"
	if d.config.MaxGoalRounds > 0 {
		maxStr = fmt.Sprintf("%d", d.config.MaxGoalRounds)
	}
	prompt = replaceAllStr(prompt, "{max}", maxStr)
	return prompt
}

// ReplaceAll replaces all occurrences of old with new in s.
func replaceAllStr(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}
