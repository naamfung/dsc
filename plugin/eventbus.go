package plugin

import (
	"errors"
	"sync"
)

// 通用事件分发总线（对齐 DSH Cordis events.ts 的分发模式）：
// 监听器按事件名注册，宿主按分发模式调用。五种模式：
//
//	emit     同步顺序调用，忽略返回值（监听器错误仅记录，不影响调用方）
//	parallel 并发执行，聚合所有错误
//	serial   顺序执行，遇错误即停止
//	bail     顺序执行，首个非 nil 返回值即短路停止
//	waterfall 洋葱模型：监听器通过 next() 委托给链上后续监听器，不调 next 即 veto
//
// 与插件生命周期事件（Subscribe/publishEventLocked）正交：后者是状态机专用的
// 推送通道，此处是宿主内通用的事件扩展点，供工具流水线、请求拦截等使用。

// EventName 事件名（字符串，可扩展）。
type EventName string

// EventContext 事件分发时的上下文载荷。
type EventContext struct {
	Name EventName
	Data any
}

// Listener 普通监听器：返回值仅对 bail 模式有意义，其余模式忽略。
type Listener func(ctx EventContext) (any, error)

// WaterfallListener 洋葱监听器：调用 next 委托给链上后续监听器，
// 不调用 next 即中断（veto）。
type WaterfallListener func(ctx EventContext, next func(EventContext) error) error

type listenerEntry struct {
	order int
	fn    Listener
}

type waterfallEntry struct {
	order int
	fn    WaterfallListener
}

// EventBus 事件分发总线。
type EventBus struct {
	mu   sync.RWMutex
	next int
	emit map[EventName][]listenerEntry
	wf   map[EventName][]waterfallEntry
}

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus {
	return &EventBus{
		emit: make(map[EventName][]listenerEntry),
		wf:   make(map[EventName][]waterfallEntry),
	}
}

// On 注册普通监听器（按注册顺序执行），返回移除函数。
func (b *EventBus) On(name EventName, fn Listener) func() {
	b.mu.Lock()
	b.next++
	entry := listenerEntry{order: b.next, fn: fn}
	b.emit[name] = append(b.emit[name], entry)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		entries := b.emit[name]
		for i, e := range entries {
			if e.order == entry.order {
				b.emit[name] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// OnWaterfall 注册洋葱监听器（按注册顺序嵌套），返回移除函数。
func (b *EventBus) OnWaterfall(name EventName, fn WaterfallListener) func() {
	b.mu.Lock()
	b.next++
	entry := waterfallEntry{order: b.next, fn: fn}
	b.wf[name] = append(b.wf[name], entry)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		entries := b.wf[name]
		for i, e := range entries {
			if e.order == entry.order {
				b.wf[name] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// snapshotEmit 返回普通监听器快照（按注册顺序）。
func (b *EventBus) snapshotEmit(name EventName) []Listener {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := b.emit[name]
	out := make([]Listener, len(entries))
	for i, e := range entries {
		out[i] = e.fn
	}
	return out
}

// snapshotWaterfall 返回洋葱监听器快照（按注册顺序）。
func (b *EventBus) snapshotWaterfall(name EventName) []WaterfallListener {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := b.wf[name]
	out := make([]WaterfallListener, len(entries))
	for i, e := range entries {
		out[i] = e.fn
	}
	return out
}

// Emit 同步顺序调用所有监听器，忽略返回值；监听器错误仅记录不中断。
func (b *EventBus) Emit(name EventName, ctx EventContext) {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		_, _ = fn(ctx)
	}
}

// Parallel 并发调用所有监听器并聚合错误。
func (b *EventBus) Parallel(name EventName, ctx EventContext) error {
	ctx.Name = name
	fns := b.snapshotEmit(name)
	var wg sync.WaitGroup
	errCh := make(chan error, len(fns))
	for _, fn := range fns {
		wg.Add(1)
		go func(fn Listener) {
			defer wg.Done()
			if _, err := fn(ctx); err != nil {
				errCh <- err
			}
		}(fn)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Serial 顺序调用所有监听器，遇错误即停止并返回。
func (b *EventBus) Serial(name EventName, ctx EventContext) error {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		if _, err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Bail 顺序调用监听器，首个非 nil 返回值即短路停止并返回该值。
// 无监听器或全部返回 nil 时返回 nil。
func (b *EventBus) Bail(name EventName, ctx EventContext) (any, error) {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		if v, err := fn(ctx); v != nil || err != nil {
			return v, err
		}
	}
	return nil, nil
}

// Waterfall 按洋葱模型委托调用监听器链：从最外层监听器开始，每个监听器
// 通过 next 委托给链上后续监听器（含兜底 next）；监听器不调用 next 即
// 中断链（veto），其返回值为最终结果。无监听器时直接调用兜底 next。
func (b *EventBus) Waterfall(name EventName, ctx EventContext, next func(EventContext) error) error {
	ctx.Name = name
	listeners := b.snapshotWaterfall(name)
	if len(listeners) == 0 {
		return next(ctx)
	}
	// 从链尾向前包装：最内层是兜底 next
	var chain func(EventContext) error = next
	for i := len(listeners) - 1; i >= 0; i-- {
		fn := listeners[i]
		inner := chain
		chain = func(c EventContext) error { return fn(c, inner) }
	}
	return chain(ctx)
}
