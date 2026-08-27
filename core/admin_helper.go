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
