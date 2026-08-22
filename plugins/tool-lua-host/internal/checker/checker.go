// Package checker 对 LUA 脚本做加载前静态校验：
//   - 语法门禁：parse.Parse 失败即返回错误（阻止加载）
//   - 类型诊断：走 go-lua 的 flow-sensitive 类型检查器（compiler/check），
//     产出诊断列表（类型系统 pre-convergence，调用方可选择阻止或仅警告）
package checker

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

// Check 对脚本源码做语法门禁 + 类型诊断。
// 返回 (诊断列表, 语法错误)；诊断仅含 error 级别（忽略 warning/info 噪音）。
func Check(src, name string) ([]string, error) {
	chunk, err := parse.Parse(strings.NewReader(src), name)
	if err != nil {
		return nil, fmt.Errorf("lua 语法错误: %w", err)
	}

	sess := check.NewChecker(db.New(), check.Deps{
		Types:       core.NewEngineWithStdlib(stdlib.EngineConfig()),
		Stdlib:      scope.NewWithBuiltins(),
		GlobalTypes: stdlib.Library(),
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}, hooks.All()...).CheckChunk(chunk, name)

	var diags []string
	for _, d := range sess.DiagnosticsSlice() {
		diags = append(diags, d.String())
	}
	return diags, nil
}
