package dsc

// closable 是互通客户端的通用约束：所有宿主聚合/桥接客户端都带 Close() error。
type closable interface {
	Close() error
}

// dialInterconnect 统一「按 serviceID 连接宿主聚合/桥接服务」的四段重复分支
// （LLM / Tool / 通知 / Agent）。规则与既有语义保持一致：
//   - id 为 0 表示未提供，跳过；
//   - dial 成功则交给 assign 写入 Interconnect 对应字段；
//   - dial 失败不中断其他服务，仅记录首个 dialErr（宿主仅 Warn 不终止加载）。
func dialInterconnect[T closable](id uint32, dial func(uint32) (T, error), assign func(T), dialErr *error) {
	if id == 0 {
		return
	}
	c, err := dial(id)
	if err != nil {
		if *dialErr == nil {
			*dialErr = err
		}
		return
	}
	assign(c)
}
