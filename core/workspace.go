package core

// 工作空间统一设定（虚拟根归并的唯一源头）。
//
// 模型可见的文件系统是「以工作空间根为 / 的 POSIX 虚拟根」，所有平台所有输入输出
// 统一正斜杆。模型书写的路径（相对路径、/workspace 前缀、裸 / 前缀、WSL 风格、
// Windows 盘符路径）到真实路径的归并映射原先散落各插件（tool-filesystem 的
// mapWorkspacePath、tool-str-replace-editor 的 normalizeWorkspacePath/makeAbsPath），
// 语义漂移且覆盖不全（报告案例：str_replace_editor 把 /docs/architecture.md 经
// filepath.Abs 落到进程 cwd 所在盘的盘根 D:/docs，而非 <workspace>/docs/architecture.md）。
//
// 现统一收敛至 core（宿主本身亦有虚拟根诉求，工具插件经 SDK 二次封装使用同一实现）：
//   - core.MapWorkspacePath：虚拟根归并映射（供 shell AST 字面量重写与插件路径入口）
//   - core.ResolveWorkspacePath：映射后解析为真实绝对路径（相对路径锚定工作空间根）
//
// 语义与内置 POSIX shell（mvdan/sh）的既有实现逐条对齐，规则如下（依序判定）：
//
//  1. WSL 风格路径 /mnt/<drive>/... → <drive>:/...（仅 Windows）：
//     Windows 上 DSC 的 shell 是 mvdan POSIX 解释器（非 WSL），无法访问真正的 /mnt/c/
//     挂载点；模型若以 WSL 路径习惯调用，统一转换为 Windows 盘符路径。Linux/macOS
//     上 /mnt/c/... 是合法 POSIX 路径（可能是真实挂载点），不得改写。
//
//  2. /workspace 前缀 → 工作空间根（所有平台）：模型常先 cd /workspace 探索；
//     前缀后必须是分隔符或结尾（/workspacefoo 不作别名）。反斜杆形态 \workspace 同样识别。
//
//  3. 裸 / 前缀 POSIX 绝对路径 → 工作空间根（仅 Windows，虚拟根语义）：Windows 上
//     "/" 并非真实的文件系统根——Go 的 filepath.IsAbs("/x") 为 false，内建工具把它当
//     工作区相对路径，而 PATH 外部命令（MSYS 等）却把 "/" 当当前盘符根，同一写法两套
//     语义；统一锚定工作空间根，要跨出工作区须显式用盘符路径。例外：/dev/null 保持
//     原样（mvdan DefaultOpenHandler 在 Windows 上特判重定向到 NUL 设备）；// 开头的
//     UNC 路径不改写。Linux/macOS 不启用：POSIX 系统上 / 是真实根。
//
// 不匹配任何规则的输入（相对路径、盘符路径等）原样返回；映射结果一律正斜杆形态。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WorkspaceRoot 統一工作空間根目錄（對齊 DSH ctx.sandboxPolicy 的單一根來源）：
// 宿主進程由 config.yaml 的 workspace_root 解析（見 main.go），
// 工具插件等子進程則讀取宿主注入的 DSC_WORKSPACE_ROOT 環境變量。
var WorkspaceRoot string

func init() {
	// 子進程：優先取宿主注入的 DSC_WORKSPACE_ROOT，使宿主與各插件進程對工作空間根保持一致。
	if root := os.Getenv("DSC_WORKSPACE_ROOT"); root != "" {
		WorkspaceRoot = root
	}
	if WorkspaceRoot == "" {
		// 無注入時，默認以啟動目錄為根（與宿主 resolveWorkspaceRoot 一致：
		// 在哪个目录启动，就以哪个目录为工作区）。
		if cwd, err := os.Getwd(); err == nil {
			WorkspaceRoot = cwd
		} else if exePath, err := os.Executable(); err == nil {
			// 無法獲取 cwd 時以可執行文件所在目錄為根（与宿主 Getwd 失败退化一致）
			WorkspaceRoot = filepath.Dir(exePath)
		} else {
			WorkspaceRoot = "."
		}
	}
}

// MapWorkspacePath 把模型书写的路径按「虚拟根 = 工作空间根」契约映射为真实路径。
// 读取统一根 WorkspaceRoot（宿主与插件进程同源），规则见包注释；不匹配任何规则
// 的输入原样返回。shell 的 AST 字面量重写与各插件路径入口统一走此函数，
// 插件不得各自实现映射（SDK 层有同名二次封装供第三方插件使用）。
func MapWorkspacePath(p string) string {
	return mapWorkspacePathFor(p, WorkspaceRoot, runtime.GOOS)
}

// ResolveWorkspacePath 把模型书写的路径映射（MapWorkspacePath）后解析为真实绝对路径：
//   - 映射后为绝对路径（盘符路径、POSIX 系统上的 / 绝对路径、已锚定工作空间根的
//     虚拟根路径）→ 直接 Abs 求净；
//   - 映射后仍为相对路径 → 以工作空间根为基准拼接（相对路径一律工作空间相对，
//     与内置 shell 的 cwd 语义一致），而非进程 cwd——插件进程 cwd 是 ExecDir，
//     以它为锚会把模型的工作空间相对路径落到安装目录。
func ResolveWorkspacePath(p string) (string, error) {
	mapped := MapWorkspacePath(p)
	if filepath.IsAbs(mapped) {
		return filepath.Abs(mapped)
	}
	return filepath.Abs(filepath.Join(WorkspaceRoot, mapped))
}

// mapWorkspacePathFor 是 MapWorkspacePath 的纯函数本体（root 与 goos 显式注入，
// 便于跨平台测试全部规则分支）。语义逐条对齐 tool-filesystem 既有实现并扩展
// 反斜杆形态识别：规则匹配在显式正斜杆副本上进行，映射结果一律正斜杆。
func mapWorkspacePathFor(p, root, goos string) string {
	// 规则匹配统一在正斜杆副本上进行（模型可能传入 \workspace、\x 等 Windows 风格）。
	// 注意不用 filepath.ToSlash：它在非 Windows 宿主上是空操作（反斜杆在 POSIX
	// 不是分隔符），纯函数必须显式替换才能宿主无关。
	slash := strings.ReplaceAll(p, "\\", "/")

	// 1. WSL 路径映射：/mnt/c/... → C:/...，/mnt/d/... → D:/...（仅 Windows）
	if goos == "windows" && strings.HasPrefix(slash, "/mnt/") {
		rest := slash[len("/mnt/"):]
		if len(rest) >= 2 && rest[1] == '/' {
			drive := string(rest[0])
			if drive >= "a" && drive <= "z" || drive >= "A" && drive <= "Z" {
				return strings.ToUpper(drive) + ":/" + rest[2:]
			}
		}
		// /mnt/c (no trailing slash) → C:/
		if len(rest) == 1 {
			drive := rest
			if drive >= "a" && drive <= "z" || drive >= "A" && drive <= "Z" {
				return strings.ToUpper(drive) + ":/"
			}
		}
	}

	if root == "" {
		return p
	}
	r := strings.TrimRight(strings.ReplaceAll(root, "\\", "/"), "/")

	// 2. /workspace 虚拟根映射（所有平台生效；先于裸 / 判定，避免 Windows 上
	//    /workspace/x 被下面的裸 / 规则吃掉而错映射）
	const prefix = "/workspace"
	if strings.HasPrefix(slash, prefix) {
		rest := slash[len(prefix):]
		// 边界检查：前綴後必須是分隔符或結尾，否則 /workspacefoo 不當作 /workspace 別名
		if rest == "" || strings.HasPrefix(rest, "/") {
			sub := strings.TrimLeft(rest, "/")
			if sub == "" {
				return r
			}
			return r + "/" + sub
		}
	}

	// 3. Windows 裸 POSIX 绝对路径 → 工作区根（仅 Windows，虚拟根语义）
	if goos == "windows" {
		if slash == "/" {
			return r
		}
		if strings.HasPrefix(slash, "/") && !strings.HasPrefix(slash, "//") && slash != "/dev/null" {
			return r + slash
		}
	}
	return p
}
