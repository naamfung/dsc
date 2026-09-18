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

// ExecDir 返回宿主可执行目录（DSC_EXEC_DIR；缺席回退当前进程 cwd）。
// 宿主在 spawn 插件子进程时注入 DSC_EXEC_DIR，插件据此定位与 exe 同级的
// 资源（fonts/、temp/、credentials/ 等），而非插件进程自身的 os.Getwd()
// （后者在测试场景下可能指向任意目录）。
func ExecDir() string {
	if d := strings.TrimSpace(os.Getenv("DSC_EXEC_DIR")); d != "" {
		return d
	}
	d, _ := os.Getwd()
	return d
}

// ContextWindow 返回宿主注入的有效上下文窗口大小（DSC_CONTEXT_WINDOW，token 数）。
// 缺席或非法返回 0（由调用方决定回退策略，如默认 131072）。
// 消除各插件散落的 `if v := os.Getenv("DSC_CONTEXT_WINDOW"); v != "" { ... }` 模式。
func ContextWindow() int {
	return envInt("DSC_CONTEXT_WINDOW")
}

func envInt(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	return v
}

func envBool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v != "" && v != "0"
}
