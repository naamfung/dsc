// 本文件实现 `dsc help` / `dsc -h` / `dsc --help`：打印用法后退出 0。同时提供
// 「未知参数」检测，供 main 在进入主流程前 fail-loud 拒绝拼错的旗标/子命令。
//
// 为什么要有：此前帮助类参数全部落空——参数解析只认少数几个旗标，未识别的参数被
// 静默忽略，于是 `dsc -h` 会直接启动 TUI（看起来像「命令没反应」），`dsc hepl`
// 这种拼错也一样。用户无从知道有哪些旗标可用。
//
// 帮助文本必须与 main.go 中真实解析的旗标一一对应：新增/删除旗标时同步改这里，
// help_test.go 会校验「文本里出现的旗标都被解析、解析的旗标都出现在文本里」。
package main

import (
	"fmt"
	"os"
	"strings"
)

// helpText 是 `dsc help` 的完整输出。刻意手写而非自动生成：旗标说明需要讲清语义
// （尤其 -input/-headless 的差别、-log 的可选取值），自动生成只能列出名字。
const helpText = `DSC — 纯 Go 的 AI Agent 宿主（TUI + 插件化工具/模型/策略）

用法：
  dsc [选项]                启动 TUI
  dsc -input <文本>         非交互执行一轮后退出（stdin 为管道时多轮直到 EOF）
  dsc -headless -input <文本>  精简单发：仅跑一次任务后退出，不开管理端口/热重载/cron
  dsc setup                 交互式配置向导（只配 LLM 连接与默认提供商）
  dsc version               输出版本号（含 git 短哈希）
  dsc help | -h | --help    显示本帮助

选项：
  -mode <name>        模式名（取自 config/presets/<name>.yaml，默认 standard；
                      内置 minimal/standard/creation/ptc）
  -input <文本>       非 TUI 自动化入口；-input 后跟的值若以 - 开头视为缺失
  -headless           精简单发模式（须同时给 -input，否则用法错误退出 1）
  -with-user-grant    把需审批的动作视作已获授权（工具免问、沙箱升级自动放行）。
                      只对 -input / -headless 生效；TUI 有自己的审批界面，忽略并告警。
                      用于无人值守场景：否则装插件、写工作区外文件都会被直接拒绝（没有可问的人）
  -admin <地址>       管理 API 监听地址（默认 127.0.0.1:9999；远程管理需配 DSC_ADMIN_TOKEN）
  -debugger           开放 /debugger 观察路由（含完整会话历史，敏感，默认关闭）
  -log [<文件>]       日志：带文件名写文件，单独 -log 输出到屏幕；默认静默。
                      级别用 DSC_LOG_LEVEL=debug|info|warn|error（默认 info）
  -hooks <文件>       外部脚本钩子（hooks.json：lua 或原生可执行，BeforeTool/AfterTool 可 veto/改写）
  -patch <文件.yaml>  配置 overlay（可重复，按顺序合并；常用于启动时挂载 MCP 服务器）

环境变量（常用）：
  DSC_MODE / DSC_SANDBOX_POLICY / DSC_APPROVAL     运行模式、沙箱策略、审批策略
  DSC_CONTEXT_WINDOW                               上下文窗口（0=未设，按探测或 128K 兜底）
  DSC_HISTORY_INJECTION                            恢复历史注入：负值=不注入、0=未定义(不限制)、N=最近 N 条
  DSC_COMPACTION_BASIC_THRESHOLD                   压缩触发比例（默认 0.70；须低于宿主 80% 收口预算）
  DSC_COMPACTION_BASIC_LLM_TIMEOUT                 单轮摘要时间预算（秒，默认取宿主分发预算减 10s）
  DSC_EVENT_DISPATCH_TIMEOUT                       事件分发单轮预算（秒，默认 30）。本地引擎
                                                   （prefill 1~2K tok/s）要一轮压完摘要，把它与
                                                   上面两项一起放大（如 120 / 110）
  DSC_<PROVIDER>_API_KEY / _BASE_URL / _MODEL      LLM 连接（PROVIDER=OPENAI/ANTHROPIC/DEEPSEEK/...）

配置：<可执行目录>/config/config.yaml；会话与会话日志在 <可执行目录>/sessions/
退出码：0 正常；1 用法/运行错误；2 资源或初始化失败
`

// isHelpCommand 判断命令行是否请求帮助：
//   - 任何位置出现 -h / -help / --help / -? 即算（与多数 CLI 一致，`dsc -log -h` 也成立）
//   - 或第一个位置参数（非 flag 及其取值）为 "help"
//
// 判定规则与 isVersionCommand / isSetupCommand 保持一致：带值 flag 的取值不算位置参数。
func isHelpCommand(args []string) bool {
	for _, a := range args {
		switch a {
		case "-h", "-help", "--help", "-?":
			return true
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case valueFlag(a):
			i++ // 跳过 flag 的取值（若存在）
		case strings.HasPrefix(a, "-"):
			// 无值 flag（-headless / -debugger 等），跳过
		default:
			return a == "help"
		}
	}
	return false
}

// helpFlags 帮助文本里承诺可用的旗标（不含 help 自身与子命令）。供
// help_test.go 双向校验：解析的旗标都写进帮助、帮助里写的旗标都真被解析。
var helpFlags = []string{
	"-mode", "-input", "-headless", "-admin", "-debugger", "-log", "-hooks", "-patch",
}

// helpSubcommands 帮助文本里承诺可用的子命令。
var helpSubcommands = []string{"setup", "version", "help"}

// valueFlag 报告该旗标是否吃一个取值（用于跳过其后的位置参数）。
func valueFlag(a string) bool {
	switch a {
	case "-mode", "-input", "-admin", "-log", "-hooks", "-patch":
		return true
	}
	return false
}

// requiredValueFlag 报告该旗标的取值是否必填（-log 的取值可选：单独 -log 输出到屏幕）。
func requiredValueFlag(a string) bool {
	switch a {
	case "-mode", "-input", "-admin", "-hooks", "-patch":
		return true
	}
	return false
}

// knownFlag 报告该旗标是否被 main 的参数解析识别（含取值为可选的 -log）。
func knownFlag(a string) bool {
	switch a {
	case "-h", "-help", "--help", "-?", "-mode", "-input", "-headless",
		"-admin", "-debugger", "-log", "-hooks", "-patch", "-with-user-grant":
		return true
	}
	return false
}

// checkArgs 在进入主流程前校验命令行：未知旗标与未知位置参数一律 fail-loud。
//
// 为什么不能静默忽略：`dsc -h`、`dsc hepl`、`dsc -imput x` 这类拼写错误此前都会被
// 无声吞掉并直接启动 TUI——用户看到的是「键盘输入没反应」或「命令跑起来了但没做我
// 想做的事」，比报错难查得多。函数返回错误文本（已含用法提示）；nil 表示可用。
func checkArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case knownFlag(a):
			if !valueFlag(a) {
				continue
			}
			hasVal := i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
			if requiredValueFlag(a) && !hasVal {
				return fmt.Errorf("选项 %s 缺少取值\n（用 dsc help 查看用法）", a)
			}
			if hasVal {
				i++ // 跳过取值
			}
		case strings.HasPrefix(a, "-") && a != "-":
			return fmt.Errorf("未知选项: %s\n（用 dsc help 查看可用选项）", a)
		default:
			known := false
			for _, c := range helpSubcommands {
				if a == c {
					known = true
					break
				}
			}
			if !known {
				return fmt.Errorf("未知参数: %s\n（用 dsc help 查看用法；只跑一次任务请用 -input）", a)
			}
			// 子命令由各自的分支处理；继续扫描其后的旗标（`dsc version -bogus` 也要报错）
		}
	}
	return nil
}

// printHelp 打印帮助到 stdout 并退出 0（帮助属正常请求，不是错误）。
func printHelp() {
	fmt.Fprint(os.Stdout, helpText)
}
