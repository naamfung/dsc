package lua

import (
	"sync"
	"sync/atomic"
)

// statePool holds reusable LState objects for better performance
var statePool = sync.Pool{
	New: func() any {
		return nil // We'll create states with specific options when needed
	},
}

// resetLState prepares a state for reuse by clearing its values but keeping the allocated structures
func resetLState(ls *LState) {
	// Clear registry but keep the underlying Array
	if ls.reg != nil {
		// Only clear up to top, not the entire array
		top := ls.reg.top
		ls.reg.top = 0
		for i := 0; i < top; i++ {
			ls.reg.array[i] = LNil
		}
	}

	// Reset upvalue cache
	ls.uvcache = nil

	// Reset call frames
	if ls.stack != nil {
		// Different stack implementations handled in FreeAll()
		ls.stack.SetSp(0)
	}

	// Reset state properties
	ls.currentFrame = nil
	ls.Parent = nil
	ls.Dead = false
	// Note: stop is NOT reset here - it stays set so IsClosed() returns true
	// stop is reset when the state is retrieved from the pool for reuse
	ls.Env = nil
	ls.G = nil
	ls.hasErrorFunc = false
	ls.wrapped = false
	ls.yieldState = yieldNone

	// Clear frame extensions to prevent stale continuations from being invoked
	ls.frameExt = nil
}

// Close returns the state to pool if appropriate.
func (ls *LState) Close() {
	atomic.AddInt32(&ls.stop, 1)

	// Don't pool if registry has grown beyond initial size
	shouldPool := ls.reg != nil && cap(ls.reg.array) <= ls.Options.RegistrySize+ls.Options.RegistryGrowStep

	if shouldPool {
		resetLState(ls)
		statePool.Put(ls)
	} else {
		ls.stack.FreeAll()
		ls.stack = nil
		ls.reg = nil
	}
}

// newLStateWithGlobal creates a thread that shares the parent's global/env.
func newLStateWithGlobal(options Options, G *Global, env *LTable) *LState {
	// Try to get a state from the pool
	pooledState := statePool.Get()

	if ls, ok := pooledState.(*LState); ok && ls != nil {
		// We got a pooled state, configure it for reuse
		ls.G = G
		ls.Env = env
		ls.Panic = panicWithTraceback
		ls.Options = options
		ls.mainLoop = mainLoop
		ls.stop = 0
		ls.yieldState = yieldNone
		ls.ctx = nil
		ls.ctxDone = nil

		// Registry was preserved but might need resetting if options changed
		if ls.reg != nil && cap(ls.reg.array) != options.RegistrySize {
			ls.reg = newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize)
		} else if ls.reg != nil {
			ls.reg.handler = ls
		}

		return ls
	}

	// No suitable pooled state available, create a new one
	ls := &LState{
		G:            G,
		Parent:       nil,
		Panic:        panicWithTraceback,
		Dead:         false,
		Options:      options,
		stop:         0,
		currentFrame: nil,
		wrapped:      false,
		uvcache:      nil,
		hasErrorFunc: false,
		mainLoop:     mainLoop,
		ctx:          nil,
	}

	if options.MinimizeStackMemory {
		ls.stack = newAutoGrowingCallFrameStack(options.CallStackSize)
	} else {
		ls.stack = newFixedCallFrameStack(options.CallStackSize)
	}

	ls.reg = newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize)
	ls.Env = env

	return ls
}
