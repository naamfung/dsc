package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[0;33m"
	colorBlue   = "\033[0;34m"
	colorNC     = "\033[0m"
)

func printColored(color, msg string) {
	fmt.Print(color + msg + colorNC)
}

func printInfo(msg string)    { printColored(colorBlue, msg) }
func printSuccess(msg string) { printColored(colorGreen, msg) }
func printWarning(msg string) { printColored(colorYellow, msg) }
func printError(msg string)   { printColored(colorRed, msg) }

// platform 定义目标平台
type platform struct {
	Name    string // 用户友好的名称（命令行参数用），也是发布目录名的一部分
	GOOS    string
	GOARCH  string
	Suffix  string // 可执行文件后缀（Windows 为 .exe）
	HostDir string // 宿主主程序在发布目录里的文件名（不含框架下的绝对路径）
}

// 目标平台集（对齐 AGENTS.md 七端）：freebsd-amd64 与 ghostbsd 共享同一二进制口径。
var platforms = []platform{
	{Name: "linux-amd64", GOOS: "linux", GOARCH: "amd64", HostDir: "dsc"},
	{Name: "linux-arm64", GOOS: "linux", GOARCH: "arm64", HostDir: "dsc"},
	{Name: "loong64", GOOS: "linux", GOARCH: "loong64", HostDir: "dsc"},
	{Name: "darwin-amd64", GOOS: "darwin", GOARCH: "amd64", HostDir: "dsc"},
	{Name: "darwin-arm64", GOOS: "darwin", GOARCH: "arm64", HostDir: "dsc"},
	{Name: "windows-amd64", GOOS: "windows", GOARCH: "amd64", HostDir: "dsc.exe", Suffix: ".exe"},
	{Name: "freebsd-amd64", GOOS: "freebsd", GOARCH: "amd64", HostDir: "dsc"},
}

// publicPlugins 发布于发布包内的对外插件（build.sh 同款目录名）；目录缺失时自动跳过。
var publicPlugins = []string{
	"agent-react-loop",
	"llm-openai",
	"llm-anthropic",
	"llm-ollama",
	"tool-filesystem",
	"tool-str-replace-editor",
	"tool-browser-use",
	"tool-lisp-eval",
	"tool-skill",
	"tool-memory-service",
	"dsc-notify",
	"tool-lua-host",
	"policy-fs-observation",
	"tool-ssh",
	"tool-musicplayer",
	"tool-harness-webui",
}

// internalPlugins 本机内部专用插件（如粤语、2FA、小说），默认不进入发布包；
// 经 --include-internal 显式才打包（避免随发布泄露内部工具）。目录缺失时自动跳过。
var internalPlugins = []string{
	"tool-jyutzyun",
	"tool-2fa-master",
	"tool-novelforge",
}

// audioPlugins 依赖音频库（oto）的核心插件，仅 darwin / windows 可 CGO_ENABLED=0 交叉到
// 纯 Go driver，其余平台（linux ALSA / freebsd 无 driver）交叉编译不过，发布时不打包。
var audioPlugins = map[string]bool{
	"dsc-notify":       true,
	"tool-musicplayer": true,
}

func main() {
	progName := filepath.Base(os.Args[0])

	// 切换到脚本所在目录，并据此定位仓库根（builder 的父目录）
	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	scriptDir := filepath.Dir(exePath)
	repoRoot := filepath.Dir(scriptDir)
	if err := os.Chdir(scriptDir); err != nil {
		fmt.Printf("切换目录失败: %v\n", err)
		os.Exit(1)
	}

	// 处理帮助命令
	if len(os.Args) >= 2 {
		arg := os.Args[1]
		if arg == "help" || arg == "--help" || arg == "-h" {
			printHelp(progName)
			os.Exit(0)
		}
	}

	// 处理 clean 命令
	if len(os.Args) >= 2 && (os.Args[1] == "clean" || os.Args[1] == "--clean" || os.Args[1] == "-clean") {
		clean(repoRoot)
		os.Exit(0)
	}

	// 处理 cross 命令（跨平台打包）
	if len(os.Args) >= 2 && (os.Args[1] == "cross" || os.Args[1] == "--cross" || os.Args[1] == "-cross") {
		crossBuild(repoRoot, os.Args[2:])
		os.Exit(0)
	}

	// 默认行为：打包当前平台
	printInfo(fmt.Sprintf("=== DSC 发布构建器 ===\n\n仓库根: %s\n", repoRoot))
	host := hostPlatform()
	if host == nil {
		printError(fmt.Sprintf("错误: 当前平台 %s/%s 不在定义的目标平台内\n", runtime.GOOS, runtime.GOARCH))
		os.Exit(1)
	}
	printInfo(fmt.Sprintf("当前平台: %s (%s/%s)\n\n", host.Name, host.GOOS, host.GOARCH))
	buildPlatform(repoRoot, *host, false)
	printSuccess(fmt.Sprintf("\n=== 打包完成: dist/dsc-for-%s ===\n", host.Name))
}

// hostPlatform 由当前 GOOS/GOARCH 匹配目标平台定义。
func hostPlatform() *platform {
	for i := range platforms {
		if platforms[i].GOOS == runtime.GOOS && platforms[i].GOARCH == runtime.GOARCH {
			return &platforms[i]
		}
	}
	return nil
}

func printHelp(progName string) {
	fmt.Printf(`DSC 发布构建器

用法:
  %s                    打包当前平台到 dist/dsc-for-<platform>/
  %s cross [选项]       打包所有或指定平台（无需 Docker）
  %s clean              清理输出目录 dist/（以及前端构建缓存）
  %s --help, -h         显示此帮助

选项（cross 命令）:
  --platforms, -p <列表>  指定要打包的平台，逗号分隔（例如：windows-amd64,loong64）
                          如果不指定，则打包所有平台
  --include-internal     把本机内部专用插件（tool-jyutzyun / tool-2fa-master /
                          tool-novelforge）一并纳入发布包
  --help, -h             显示此帮助

支持平台列表（cross）:
  linux-amd64       - Linux x86_64
  linux-arm64       - Linux ARM64
  loong64           - Loong64 龙芯64位
  darwin-amd64      - macOS Intel
  darwin-arm64      - macOS Apple Silicon
  windows-amd64     - Windows x86_64
  freebsd-amd64     - FreeBSD x86_64（与 GhostBSD 共用同一二进制）

每个平台生成 dist/dsc-for-<platform>/ 目录，布局与源码目录层级一致但无 .go 源码：
  dsc[.exe]               宿主主程序
  config/                 配置文件（config.example.yaml + presets/ 模板）
  plugins/<name>/<name>   各插件可执行文件
  skills/builtin,skills/installed   内置/已安装技能
  LICENSE, README.md      文档

注意：
  - dsc-notify / tool-musicplayer 依赖 oto 音频库，仅 darwin / windows 打包；
    其余平台这些插件目录不生成（发布包内不含其可执行文件）。
  - tool-harness-webui 插件需宿主平台有 bash + bun 先构建前端（go:embed），
    环境缺失时该插件被跳过。
  - 本机内部专用插件默认不打包，需 --include-internal 才纳入。
  - 本机存在 upx 时，对宿主与各插件二进制自动 UPX --best 压缩（未检测到则跳过）。

示例:
  %s cross                         # 打包所有平台
  %s cross --platforms windows-amd64,loong64
  %s cross --include-internal      # 连同内部插件一起打包
  %s                              # 仅打包当前平台
`,
		progName, progName, progName, progName,
		progName, progName, progName, progName)
}

// clean 清理所有发布产物（dist/ 与之前错位产生的 plugins/*/dist）
func clean(repoRoot string) {
	printInfo("=== DSC 发布清理 ===\n\n")
	removeAll(filepath.Join(repoRoot, "dist"))
	// 清理历史版本可能错位落到插件目录内的 dist（防御性）
	matches, _ := filepath.Glob(filepath.Join(repoRoot, "plugins", "*", "dist"))
	for _, m := range matches {
		removeAll(m)
	}
	fmt.Println("清理完成！")
}

// crossBuild 跨平台打包
func crossBuild(repoRoot string, args []string) {
	printInfo(fmt.Sprintf("=== DSC 跨平台打包（无需 Docker） ===\n仓库根: %s\n\n", repoRoot))

	buildAll := true
	includeInternal := false
	var selectedPlatforms []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printHelp(filepath.Base(os.Args[0]))
			return
		case "--include-internal":
			includeInternal = true
		case "--platforms", "-p":
			if i+1 < len(args) {
				buildAll = false
				selectedPlatforms = strings.Split(args[i+1], ",")
				i++
			} else {
				printError("错误: --platforms 需要指定平台列表\n")
				os.Exit(1)
			}
		default:
			printError(fmt.Sprintf("未知参数: %s\n", args[i]))
			printHelp(filepath.Base(os.Args[0]))
			os.Exit(1)
		}
	}

	// 确定目标平台，按 Name 去重
	var targets []platform
	if buildAll {
		targets = platforms
	} else {
		seen := map[string]bool{}
		for _, name := range selectedPlatforms {
			for _, p := range platforms {
				if p.Name == name && !seen[p.Name] {
					seen[p.Name] = true
					targets = append(targets, p)
					break
				}
			}
			if !seen[name] {
				printWarning(fmt.Sprintf("警告: 未知平台 '%s'，已跳过\n", name))
			}
		}
		if len(targets) == 0 {
			printError("错误: 没有有效的平台可供打包\n")
			os.Exit(1)
		}
	}

	fmt.Printf("Go 版本: %s\n", goVersion())
	fmt.Printf("Git Commit: %s\n", gitCommit(repoRoot))
	if includeInternal {
		fmt.Printf("内部专用插件: 纳入发布包\n")
	} else {
		fmt.Printf("内部专用插件: 默认排除（--include-internal 可加入）\n")
	}
	fmt.Println()

	successCount := 0
	for _, p := range targets {
		if buildPlatform(repoRoot, p, includeInternal) {
			successCount++
		}
	}

	printInfo("\n=== 全部完成 ===\n")
	fmt.Printf("成功打包 %d / %d 个平台\n", successCount, len(targets))
	if successCount > 0 {
		dirs, _ := filepath.Glob(filepath.Join(repoRoot, "dist", "dsc-for-*"))
		for _, d := range dirs {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				size := dirSize(d)
				fmt.Printf("  - %s (%d MB)\n", filepath.Base(d), size/1024/1024)
			}
		}
	}
}

// buildPlatform 打包单个平台，返回是否成功。
func buildPlatform(repoRoot string, p platform, includeInternal bool) bool {
	releaseDir := filepath.Join(repoRoot, "dist", "dsc-for-"+p.Name)
	printInfo(fmt.Sprintf("\n→ 打包 %s (%s/%s) ...\n", p.Name, p.GOOS, p.GOARCH))

	binExt := func(name string) string { return name + p.Suffix }

	// 先清空该平台发布目录，避免上次构建残留（如旧备份目录、旧二进制）混入
	removeAll(releaseDir)

	// 1. 准备发布目录骨架
	mustMkdirs(releaseDir,
		filepath.Join(releaseDir, "config", "presets"),
		filepath.Join(releaseDir, "plugins"),
		filepath.Join(releaseDir, "skills", "builtin"),
		filepath.Join(releaseDir, "skills", "installed"),
	)

	// 2. 拷贝运行时资源（config 模板、skills、文档）
	copyFileStrict(filepath.Join(repoRoot, "config", "config.example.yaml"), filepath.Join(releaseDir, "config", "config.example.yaml"))
	copyDir(filepath.Join(repoRoot, "config", "presets"), filepath.Join(releaseDir, "config", "presets"))
	copyDir(filepath.Join(repoRoot, "skills", "builtin"), filepath.Join(releaseDir, "skills", "builtin"))
	copyDir(filepath.Join(repoRoot, "skills", "installed"), filepath.Join(releaseDir, "skills", "installed"))
	copyFileStrict(filepath.Join(repoRoot, "LICENSE"), filepath.Join(releaseDir, "LICENSE"))
	copyFileStrict(filepath.Join(repoRoot, "README.md"), filepath.Join(releaseDir, "README.md"))

	// 3. 编译宿主主程序 → 发布目录根
	if !hasTool("upx") {
		printInfo("（未检测到 upx，跳过 UPX 压缩）\n")
	} else {
		printInfo("检测到 upx，构建产物将 UPX 压缩（--best）\n")
	}
	hostOut := filepath.Join(releaseDir, binExt("dsc"))
	if err := goBuild(repoRoot, hostOut, p); err != nil {
		printError(fmt.Sprintf("  ✗ 宿主编译失败: %v\n", err))
		return false
	}
	printSuccess(fmt.Sprintf("  ✓ 宿主: %s\n", relativeHost(repoRoot, hostOut)))
	pack(hostOut)

	// 4. 编译各插件 → plugins/<name>/<name>[.exe]
	plugins := append([]string{}, publicPlugins...)
	if includeInternal {
		plugins = append(plugins, internalPlugins...)
	}
	builtPlugins := 0
	skippedPlugins := []string{}
	for _, name := range plugins {
		pdir := filepath.Join(repoRoot, "plugins", name)
		if _, err := os.Stat(pdir); err != nil {
			continue
		}
		// 音频插件仅对 darwin / windows 打包（其余平台 CGO/ALSA 无法交叉编译）
		if p.GOOS != "darwin" && p.GOOS != "windows" && audioPlugins[name] {
			skippedPlugins = append(skippedPlugins, name)
			continue
		}
		out := filepath.Join(releaseDir, "plugins", name, binExt(name))
		if name == "tool-harness-webui" {
			if !buildWebUIAssets(repoRoot) {
				printWarning(fmt.Sprintf("  - 跳过 tool-harness-webui：需宿主平台 bash+bun 构建前端\n"))
				skippedPlugins = append(skippedPlugins, name)
				continue
			}
		}
		if err := goBuild(pdir, out, p); err != nil {
			printError(fmt.Sprintf("  ✗ 插件 %s 编译失败: %v\n", name, err))
			continue
		}
		printSuccess(fmt.Sprintf("  ✓ 插件: %s\n", name))
		pack(out)
		builtPlugins++
	}
	if len(skippedPlugins) > 0 {
		fmt.Printf("  （跳过平台不适用插件: %s）\n", strings.Join(skippedPlugins, ", "))
	}
	return true
}

// goBuild 以目标平台环境在 dir 目录交叉编译到 out。跨平台一律 CGO_ENABLED=0。
func goBuild(dir, out string, p platform) error {
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOOS="+p.GOOS,
		"GOARCH="+p.GOARCH,
		"CGO_ENABLED=0",
	)
	// 交错输出，便于在目标平台文件名上保留后缀
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// pack UPX 压缩单个二进制到 --best；未检测到 upx 或文件不存在/压缩失败时均保留原文件。
func pack(path string) {
	if !hasTool("upx") {
		return
	}
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return
	}
	cmd := exec.Command("upx", "--best", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		printWarning(fmt.Sprintf("  (warn: UPX 压缩失败，保留原始二进制: %s)\n", path))
	}
}

// buildWebUIAssets 在宿主平台用 bash+bun 构建 harness-webui 前端静态资源（go:embed 用）。
// 成功返回 true；bash/bun 缺失或构建失败返回 false（由调用方决定跳过该插件）。
var webuiBuilt bool

func buildWebUIAssets(repoRoot string) bool {
	if webuiBuilt {
		return true
	}
	if hasTool("bash") && hasTool("bun") {
		webuiDir := filepath.Join(repoRoot, "plugins", "tool-harness-webui")
		if _, err := os.Stat(filepath.Join(webuiDir, "webui", "package.json")); err != nil {
			return false
		}
		printInfo("  构建 harness-webui 前端 (bun)...\n")
		cmd := exec.Command("bash", "-lc", "(cd plugins/tool-harness-webui/webui && bun install >/dev/null 2>&1 || bun install >/dev/null; bunx svelte-kit sync >/dev/null 2>&1; bun run build)")
		cmd.Dir = repoRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			printWarning(fmt.Sprintf("  前端构建失败: %v\n", err))
			return false
		}
		webuiBuilt = true
		return true
	}
	return false
}

// ========== 工具函数 ==========

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func mustMkdirs(paths ...string) {
	for _, p := range paths {
		if err := os.MkdirAll(p, 0755); err != nil {
			printError(fmt.Sprintf("无法创建目录 %s: %v\n", p, err))
			os.Exit(1)
		}
	}
}

func removeAll(paths ...string) {
	for _, p := range paths {
		os.RemoveAll(p)
	}
}

// copyFileStrict 严格拷贝单个文件，源不存在则报错退出（发布必需的资源）。
func copyFileStrict(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		printError(fmt.Sprintf("缺少发布资源 %s: %v\n", src, err))
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		printError(fmt.Sprintf("无法创建目录 %s: %v\n", filepath.Dir(dst), err))
		os.Exit(1)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		printError(fmt.Sprintf("写入 %s 失败: %v\n", dst, err))
		os.Exit(1)
	}
}

// isBackupDir 判为备份性质目录：目录名含 "backup"（如 config-backups、preset-backups
// 等配置自愈产生的备份），发布时一律排除，不进入发布包。
func isBackupDir(name string) bool {
	return strings.Contains(strings.ToLower(name), "backup")
}

// copyDir 递归拷贝目录（含空目录结构），跳过任何备份性质目录；源不存在则忽略。
func copyDir(src, dst string) {
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return
	}
	_ = filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 跳过备份性质目录整棵，不复制其内容
		if fi.IsDir() && path != src && isBackupDir(fi.Name()) {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			_ = os.MkdirAll(target, 0755)
			return nil
		}
		_ = os.MkdirAll(filepath.Dir(target), 0755)
		return copyFile(path, target, fi)
	})
}

func copyFile(src, dst string, fi os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, _ error) error {
		if fi != nil && !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total
}

func relativeHost(repoRoot, p string) string {
	rel, err := filepath.Rel(repoRoot, p)
	if err != nil {
		return p
	}
	return rel
}

func goVersion() string {
	if out, err := exec.Command("go", "version").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

func gitCommit(repoRoot string) string {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return "unknown"
	}
	if out, err := exec.Command("git", "rev-parse", "--short=7", "HEAD").Output(); err != nil {
		return "unknown"
	} else {
		return strings.TrimSpace(string(out))
	}
}
