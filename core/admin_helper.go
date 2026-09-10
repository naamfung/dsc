package core

import (
	"dsc/libs/vodka"
	"fmt"
)

// bindBody 把请求体 JSON 绑定到 out，供 ADMIN 各 handler 复用，
// 避免每个 handler 重复编写 "invalid json" 返回 400 的分支。
// 绑定失败时返回 400 HTTPError。
func bindBody(c *vodka.Context, out interface{}) error {
	if err := c.Bind(out); err != nil {
		return vodka.NewHTTPError(vodka.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}
	return nil
}

// wrapErr 把插件生命周期操作的内部错误统一包装为 500 HTTPError。
// op 为操作动词（如 "load"/"unload"/"reload"），错误消息统一
// "failed to <op> core: ..."，供 load/unload/reload 等 handler 复用，
// 避免重复编写 NewHTTPError(StatusInternalServerError, ...) 分支。
func wrapErr(op string, err error) error {
	return vodka.NewHTTPError(vodka.StatusInternalServerError, fmt.Sprintf("failed to %s core: %v", op, err))
}
