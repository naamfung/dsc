package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
)

// versionedBinaryRe 匹配「<基名>-v<major>.<minor>[.<patch>[.<build>]]<扩展名>」形式的版本化二进制文件名。
// 基名本身可含连字符（如 llm-openai），故前缀采用贪婪匹配到最后一个「-v…」。
var versionedBinaryRe = regexp.MustCompile(`^(.+)-v(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?(\..+)?$`)

// parseVersionedFileName 从版本化文件名解析出「去版本后的基名」与语义版本。
// 非版本化文件名返回 ok=false。
func parseVersionedFileName(filename string) (base string, v *version.Version, ok bool) {
	m := versionedBinaryRe.FindStringSubmatch(filename)
	if m == nil {
		return "", nil, false
	}
	parts := []string{m[2], m[3]}
	if m[4] != "" {
		parts = append(parts, m[4])
	}
	vv, err := version.NewVersion(strings.Join(parts, "."))
	if err != nil {
		return "", nil, false
	}
	return m[1], vv, true
}

// binaryVersion 把已加载（或候选）的二进制路径解析为语义版本：
// 带「-vN」后缀则取其版本，未带后缀（基线 <基名><扩展名>）视为 0.0.0。
func binaryVersion(path string) *version.Version {
	_, v, ok := parseVersionedFileName(filepath.Base(path))
	if !ok {
		return version.Must(version.NewVersion("0.0.0"))
	}
	return v
}

// versionedCandidatesInDir 返回目录 dir 中、以 base 为基名的最高版本化二进制路径（存在则伴随版本号）。
// 目录缺失或不存在候选时返回空串。base 即目录基名（如 llm-openai），
// 与「目录基名 + -vN + 扩展名」的版本化文件约定对齐。
func versionedCandidatesInDir(dir, base string) (string, *version.Version) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	best := ""
	bestV := version.Must(version.NewVersion("0.0.0"))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fb, v, ok := parseVersionedFileName(e.Name())
		if !ok || fb != base {
			continue
		}
		if v.GreaterThan(bestV) {
			bestV = v
			best = filepath.Join(dir, e.Name())
		}
	}
	return best, bestV
}

// ResolveLatestBinary 版本感知的二进制解析：在 binaryPath 所在目录内查找
// 「<目录基名>-v<版本><扩展名>」中版本最高者；存在则返回之（Windows 下运行中的
// 同名二进制被占用、无法原地覆盖，只能以新版本文件启动新进程），否则回退返回 binaryPath 本身。
func ResolveLatestBinary(binaryPath string) string {
	if binaryPath == "" {
		return ""
	}
	dir := filepath.Dir(binaryPath)
	if dir == "" || dir == "." {
		return binaryPath
	}
	base := filepath.Base(dir)
	if best, _ := versionedCandidatesInDir(dir, base); best != "" {
		return best
	}
	return binaryPath
}
