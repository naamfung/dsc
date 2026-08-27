package dsc

import (
	"os"
	"strconv"
	"strings"
)

// Env 宿主注入的进程上下文（插件只读）。宿主在拉起插件时统一注入
// DSC_* 环境变量（见 main.go / core/manager.go），插件在 OnStart 或
// 工具执行时读取，无需关心注入细节。
type Env struct {
	// Mode 当前模式：minimal | standard | creation（tool-lua-host 据此限制脚本创造）。
	Mode string
	// WorkspaceRoot 统一工作空间根（sandbox 边界；默认启动目录，可被绝对路径覆盖）。
	WorkspaceRoot string
	// SessionDir 会话持久化目录。
	SessionDir string
	// PresetPersona / PlanSection 预设与计划段上下文（agent 相关）。
	PresetPersona string
	PlanSection   string

	ContextWindow     int
	MaxIterations     int
	GoalMaxRounds     int
	SingleTurn        bool
	AllowParallelTodo bool
}

// ReadEnv 从进程环境变量读取宿主注入的上下文；缺失项取零值，不做报错。
func ReadEnv() Env {
	return Env{
		Mode:              os.Getenv("DSC_MODE"),
		WorkspaceRoot:     os.Getenv("DSC_WORKSPACE_ROOT"),
		SessionDir:        os.Getenv("DSC_SESSION_DIR"),
		PresetPersona:     os.Getenv("DSC_PRESET_PERSONA"),
		PlanSection:       os.Getenv("DSC_PLAN_SECTION"),
		ContextWindow:     envInt("DSC_CONTEXT_WINDOW"),
		MaxIterations:     envInt("DSC_MAX_ITERATIONS"),
		GoalMaxRounds:     envInt("DSC_GOAL_MAX_ROUNDS"),
		SingleTurn:        envBool("DSC_SINGLE_TURN"),
		AllowParallelTodo: envBool("DSC_TODO_ALLOW_PARALLEL"),
	}
}

func envInt(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	return v
}

func envBool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v != "" && v != "0"
}
