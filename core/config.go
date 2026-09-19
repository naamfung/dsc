package core

import (
        "os"

        "gopkg.in/yaml.v3"
)

// ConfigPath 默認配置文件路徑，基於程序可執行文件所在目錄
var ConfigPath string

// 内置模式名常量（对齐 DSH 的 agentPreset 命名）。
// 用户新增 preset（如 thin.yaml）不需在此添加常量——这些常量仅用于
// 代码中需要"按已知模式名分支"的位置（如 PTC 检测、creation 模式门控），
// 避免散落的字符串字面值。未知模式名经 SwitchMode 读 preset 文件
// 自然校验。
const (
        ModeMinimal  = "minimal"
        ModeStandard = "standard"
        ModeCreation = "creation"
        ModePTC      = "ptc"
)

func init() {
        ConfigPath = computeConfigPath(os.Executable())
}

// computeConfigPath 从可执行文件路径算出默认 config.yaml 路径。
// 经 PDir + PJoin 链路保证返回正斜杆（无反斜杆污染）——
// Windows 上 os.Executable() 返回反斜杆路径，PDir/PJoin 内部 toSlash
// 把反斜杆统一为正斜杆，使所有平台 ConfigPath 恒为 POSIX 风格。
// 抽出为独立函数便于在 Linux CI 上用 Windows 风格路径输入测试
// 归一化行为（TestComputeConfigPathWindowsBackslashNormalization）。
func computeConfigPath(exePath string, exeErr error) string {
        if exeErr != nil {
                return "./config/config.yaml"
        }
        execDir := PDir(exePath)
        return PJoin(execDir, "config", "config.yaml")
}

type Config struct {
        WorkspaceRoot string        `json:"workspace_root" yaml:"workspace_root"`
        Mode          string        `json:"mode" yaml:"mode"` // 默認模式：minimal 或 standard
        Plugins       []PluginEntry `json:"plugins" yaml:"plugins"`
        // 可保留舊的 LLM 字段便於過渡，但推薦統一使用 Plugins
        DefaultLLM string `json:"default_llm" yaml:"default_llm"`
        // ContextWindow 上下文窗口大小（token 数）；0 表示未配置，
        // 由宿主探测 LLAMACPP 的 /v1/models 獲取 n_ctx，仍失敗則用默認 128K×1024
        ContextWindow int `json:"context_window" yaml:"context_window"`
        // Persona "你是一個…助手" 身份句（預設可配，同 DSH 的 deployment persona）；空則用 DeepSeek 官方默認
        Persona string `json:"persona" yaml:"persona"`
        // PlanSection plan 模式激活时注入 system prompt 的部署方引导文案（同 DSH plan-mode 的 section）；
        // 空则用 react-loop 内置默认（DSH 示例文案）
        PlanSection string `json:"plan_section" yaml:"plan_section"`
        // SessionID 會話標識，用於 temp 目錄下的數據分離（如 browser-data/spill）
        SessionID string `json:"session_id" yaml:"session_id"`
        // HistoryInjection 历史注入持久化编码（/settings history 写回，启动时经
        // DSC_HISTORY_INJECTION 下发 agent）：-1 禁止（不注入历史）、0 未定义（默认，
        // agent 缺省不限制）、>0 启用并注入最近 N 条。
        HistoryInjection int `json:"history_injection" yaml:"history_injection"`
        // HotReload 是否启用版本化二进制自动热重载 watch：插件目录内出现版本高于当前运行的
        // <基名>-v<版本><ext> 文件时，自动经 HotReload 换成新进程（默认关闭）。
        HotReload bool `json:"hot_reload" yaml:"hot_reload"`
        // Compaction 显式声明压缩后端（对齐 DSH preset 的 compaction group）：
        //   ""             — 默认：agent 走内联 compactHistory（向后兼容）
        //   后端插件名      — 使用该插件的压缩接管（插件需声明 Provides compaction 能力：
        //                     dsc-system 基础压缩 / dsc-billion-context ACP 接管），
        //                     宿主 registerDscCoreLocked 验证能力声明；接管经
        //                     agent/pre-step 事件机制（后端主动压缩，agent 内联兜底）
        // 对齐 DSH：preset YAML 显式挂 compaction-basic 或其他后端。
        Compaction string `json:"compaction" yaml:"compaction"`
}

type PluginEntry struct {
        Name       string `json:"name" yaml:"name"`
        Type       string `json:"type" yaml:"type"` // "llm", "agent", "tool", "dsc"
        BinaryPath string `json:"binary_path" yaml:"binary_path"`
        // 可選：是否啟用、參數等
        Enabled bool `json:"enabled" yaml:"enabled" default:"true"`
        // 可選：傳遞給插件子進程的額外環境變量（合併宿主環境，插件值優先）
        Env map[string]string `json:"env" yaml:"env"`
        // Config 插件配置（对齐 DSH cordis.yml 的 config 字段）：
        // 供 -patch overlay 注入插件特定配置（如 MCP 客户端的 serverName/transport/
        // command/args/env）。宿主按 type 分发：tool/dsc 类型的 Config["mcp"] 含
        // MCP 客户端配置时，启动时自动连接。空则忽略。
        Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

// LoadConfig 從指定路徑加載配置
func LoadConfig(path string) (*Config, error) {
        data, err := os.ReadFile(path)
        if err != nil {
                return nil, err
        }
        var cfg Config
        if err := yaml.Unmarshal(data, &cfg); err != nil {
                return nil, err
        }
        return &cfg, nil
}

// SaveConfig 將配置保存到指定路徑
func SaveConfig(path string, cfg *Config) error {
        data, err := yaml.Marshal(cfg)
        if err != nil {
                return err
        }
        // 確保目錄存在
        dir := PDir(path)
        if err := os.MkdirAll(dir, 0755); err != nil {
                return err
        }
        return os.WriteFile(path, data, 0644)
}

// UpdateMode 更新模式狀態並保存到配置文件
func UpdateMode(mode string, configPath string) error {
        cfg, err := LoadConfig(configPath)
        if err != nil {
                // 如果配置文件不存在或加載失敗，創建一個新的配置
                cfg = &Config{
                        WorkspaceRoot: "",
                        Mode:          mode,
                        Plugins:       nil,
                        DefaultLLM:    "",
                        ContextWindow: 0,
                        Persona:       "",
                }
        } else {
                cfg.Mode = mode
        }
        return SaveConfig(configPath, cfg)
}
