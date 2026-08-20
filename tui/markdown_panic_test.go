package tui

import (
	"testing"
)

func TestRenderMarkdownNoPanic(t *testing.T) {
	inputs := []string{
		"你好！我在的。请问有什么我可以帮你的吗？",
		"我在的，有咩帮你？",
		"1. 用户发了好\n2. 需要回应\n回应策略",
		"你好，在吗",
		"行内 *强调* 与 **加粗** 和 `代码`",
		"带括号（中文）和英文 (paren) 的文本，含 —— 破折号",
		"混合待测试：**弯引號「」『』** _斜体_ [链接](https://x.com) ![图片](a) <tag>",
		"一个\n\n\n\n\n多空行\n\n结尾空行\n\n",
		"tab\t这里\n",
		"😀 emoji 与 emoji🎉符号混合 ✨",
		"> 引用块\n引用第二行\n\n- 列表项一\n- 列表项二\n1. 有序一\n2. 有序二",
		"# 标题\n## 二级\n下面是正文",
		"```go\nfunc main(){}\n```\n之后正文",
		"| a | b |\n|---|---|\n| 1 | 2 |",
		`百分号% 百分号%s 和 %d 格式符`,
		"斜杠/反斜杠\\和\n换行后\t制表",
		"Weird chars: \x00 \x7f ❤ 中文字长一段中文字长一段中文字长一段中文字长一段中文字长一段中文字长一段中文字长一段",
	}
	for i, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input[%d]=%q PANIC: %v", i, in, r)
				}
			}()
			_ = renderMarkdown(in, 76)
		}()
	}
}