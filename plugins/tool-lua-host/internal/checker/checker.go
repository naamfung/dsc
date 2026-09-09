// Package checker 对 LUA 脚本做加载前静态校验：
//   - 语法门禁：parse.Parse 失败即返回错误（阻止加载）
//   - 类型诊断：走 go-lua 的 flow-sensitive 类型检查器（compiler/check），
//     产出诊断列表（类型系统 pre-convergence，调用方可选择阻止或仅警告）
//
// dsc 内建的全局类型（dsc.llm/dsc.tool/dsc.notify/dsc.register_tool）已注册到
// 检查器的全局类型表，脚本中带类型注解的变量可被正确推断（消除 unknown 误报）。
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
	"github.com/wippyai/go-lua/types/typ"
)

// dscType dsc 内建命名空间的类型定义（与 bindings.Install 暴露的 LUA API 对应）。
func dscType() typ.Type {
	chatReq := typ.NewRecord().
		Field("system", typ.NewOptional(typ.String)).
		Field("user", typ.String).
		Field("max_tokens", typ.Any).
		Build()
	return typ.NewRecord().
		Field("llm", typ.NewRecord().
			Field("chat", typ.Func().Param("req", chatReq).Returns(typ.String).Build()).
			Build()).
		Field("tool", typ.NewRecord().
			Field("call", typ.Func().Param("name", typ.String).OptParam("args", typ.Any).Returns(typ.String).Build()).
			Field("list", typ.Func().Returns(typ.Any).Build()).
			Build()).
		Field("notify", typ.NewRecord().
			Field("emit", typ.Func().Param("name", typ.String).OptParam("data", typ.Any).Build()).
			Build()).
		Field("store", typ.NewRecord().
			Field("get", typ.Func().Param("key", typ.String).Returns(typ.Any).Build()).
			Field("set", typ.Func().Param("key", typ.String).Param("value", typ.Any).Build()).
			Field("delete", typ.Func().Param("key", typ.String).Build()).
			Build()).
		Field("hook", typ.NewRecord().
			Field("before_tool", typ.Func().Param("fn", typ.Any).Build()).
			Field("after_tool", typ.Func().Param("fn", typ.Any).Build()).
			Field("on_event", typ.Func().Param("fn", typ.Any).Build()).
			Build()).
		Field("job", typ.NewRecord().
			Field("spawn", typ.Func().Param("fn", typ.Any).Returns(typ.String).Build()).
			Field("status", typ.Func().Param("id", typ.String).Returns(typ.String).Build()).
			Field("list", typ.Func().Returns(typ.Any).Build()).
			Build()).
		Field("register_tool", typ.Func().
			Param("name", typ.String).
			Param("spec", typ.Any).
			Param("handler", typ.Any).
			Build()).
		Build()
}

// globalTypes 在标准库类型表基础上注册 dsc 内建全局类型。
func globalTypes() map[string]typ.Type {
	base := stdlib.Library()
	out := make(map[string]typ.Type, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["dsc"] = dscType()
	return out
}

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
		GlobalTypes: globalTypes(),
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
