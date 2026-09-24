package dsc

import (
	"fmt"
	"regexp"
)

// ToolNameMaxLen 是下发给模型的工具名长度上限。
// 各家模型服务对工具名有共同的形态约束（`^[A-Za-z0-9_-]{1,64}$`），取其中最紧的一档，
// 避免超长名在某一端被拒而另一端能过——那类不一致最难排查。
const ToolNameMaxLen = 64

// toolNamePartRE 工具名（及其来源单元名）的合法形态：字母、数字、下划线、连字符。
// 工具名最终要下发给模型服务，形态必须干净；来源单元名（脚本名/插件名）还要被逐字
// 用作前缀，故同样受此约束。
var toolNamePartRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// QualifyToolName 把「来源单元名 + 单元内注册名」合成为最终工具名：`<unit>_<name>`。
//
// 脚本载体（tool-lua-host 的脚本、tool-sql-host 的 .dsp 插件）各自可注册多个工具，
// 工具名必须带**来源**：插件一多，模型看到 `mytool` 无从判断它出自谁，多个载体注册
// 同名工具时也无法归因。约定：
//
//   - 前缀是来源单元名**本身**（脚本名 / 插件名），不加载体代号（如 lua_/dsp_）——
//     载体代号只说明「它用什么技术做的」，不说明「它从哪来」，而后者才是模型要的。
//   - 前缀与来源单元名**逐字相同**，故模型可用 `list_*_tools` 列出的名字直接做前缀
//     匹配来定位归属；任何变形（替换字符、截断、大小写改写）都会让这层对应关系失效，
//     故 unit 不合法时直接报错，而不是就地改写成一个能用的名字。
//
// 两个部分都须匹配 `^[A-Za-z0-9_-]+$`，合成结果不得超过 ToolNameMaxLen。
func QualifyToolName(unit, name string) (string, error) {
	if !toolNamePartRE.MatchString(unit) {
		return "", fmt.Errorf("工具来源名 %q 非法：仅允许字母、数字、下划线、连字符（来源名要逐字用作工具名前缀，不作改写）", unit)
	}
	if !toolNamePartRE.MatchString(name) {
		return "", fmt.Errorf("工具注册名 %q 非法：仅允许字母、数字、下划线、连字符", name)
	}
	full := unit + "_" + name
	if len(full) > ToolNameMaxLen {
		return "", fmt.Errorf("工具名 %q 超过 %d 字符上限（来源名 %q 与注册名 %q 合成）",
			full, ToolNameMaxLen, unit, name)
	}
	return full, nil
}
