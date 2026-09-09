package concurrent

import (
	"context"
	_ "embed"
	"errors"
	"sync"

	"github.com/jig/lisp/lib/call"
	"github.com/jig/lisp/types"
	. "github.com/jig/lisp/types"
)

//go:embed header-concurrent.lisp
var headerConcurrent string

func HeaderConcurrent() string { return headerConcurrent }

func Load(env types.EnvType) {
	call.CallOverrideFN(env, "atom", func(a MalType) (MalType, error) { return &Atom{Val: a}, nil })
	call.CallOverrideFN(env, "new-atom", func(a *Atom) (MalType, error) { return nil, errors.New("atom cannot be deserialized") })
	call.CallOverrideFN(env, "atom?", func(a MalType) (MalType, error) { return Q[*Atom](a), nil })
	call.CallOverrideFN(env, "swap!", swap_BANG)
	call.CallOverrideFN(env, "reset!", reset_BANG)
	call.Call(env, future_call)
	call.Call(env, future_cancel)
	call.CallOverrideFN(env, "future-cancelled?", func(f *Future) (bool, error) { return f.Cancelled, nil })
	call.CallOverrideFN(env, "future-done?", func(f *Future) (bool, error) { return f.Done, nil })
	call.CallOverrideFN(env, "future?", func(f MalType) (bool, error) { return Q[*Future](f), nil })
	call.Call(env, new_future_call)
}

func future_call(ctx context.Context, f MalFunc) (*Future, error) {
	return NewFuture(ctx, f), nil
}

func future_cancel(f *Future) (bool, error) {
	return f.Cancel(), nil
}

// Atom functions
func reset_BANG(atomRef, value MalType) (MalType, error) {
	if !Q[*Atom](atomRef) {
		return nil, errors.New("reset! called with non-atom")
	}
	atm := atomRef.(*Atom)
	atm.Mutex.Lock()
	defer atm.Mutex.Unlock()
	atm.Set(value)
	return value, nil
}

func swap_BANG(ctx context.Context, a ...MalType) (MalType, error) {
	if !Q[*Atom](a[0]) {
		return nil, errors.New("swap! called with non-atom")
	}
	atm := a[0].(*Atom)
	atm.Mutex.Lock()
	defer atm.Mutex.Unlock()
	args := []MalType{atm.Val}
	f := a[1]
	args = append(args, a[2:]...)
	res, e := Apply(ctx, f, args)
	if e != nil {
		return nil, e
	}
	atm.Set(res)
	return res, nil
}

// Atoms
type Atom struct {
	Mutex  sync.RWMutex
	Val    MalType
	Meta   MalType
	Cursor *Position
}

func (a *Atom) Type() string {
	return "atom"
}

func (a *Atom) Set(val MalType) MalType {
	a.Val = val
	return a
}

func (a *Atom) Deref(_ context.Context) (MalType, error) {
	a.Mutex.RLock()
	defer a.Mutex.RUnlock()
	return a.Val, nil
}

func (a *Atom) LispPrint(pr_str func(MalType, bool) string) string {
	return "«atom " + pr_str(a.Val, true) + "»"
}

// Future
type Future struct {
	ValChan    chan MalType
	ErrChan    chan error
	CancelFunc context.CancelFunc
	Done       bool
	Cancelled  bool

	Fn     MalFunc
	Meta   MalType
	Cursor *Position
}

func new_future_call(fn MalFunc) (*Future, error) {
	return nil, errors.New("atom cannot be deserialized")
}

func NewFuture(ctx context.Context, fn MalFunc) *Future {
	ctx, cancel := context.WithCancel(ctx)
	f := &Future{
		ValChan:    make(chan MalType, 1),
		ErrChan:    make(chan error, 1),
		CancelFunc: cancel,
		Fn:         fn,
	}
	go func() {
		defer func() { f.Done = true }()
		res, err := Apply(ctx, fn, nil)
		if err != nil {
			f.ErrChan <- err
			return
		}
		f.ValChan <- res
	}()

	return f
}

func (f *Future) Cancel() bool {
	if !f.Done {
		f.Cancelled = true
		f.Done = true
		f.CancelFunc()
	}
	return f.Cancelled
}

func (f *Future) Deref(ctx context.Context) (MalType, error) {
	select {
	case <-ctx.Done():
		return nil, errors.New("timeout while dereferencing future")
	case err := <-f.ErrChan:
		f.ErrChan <- err
		return nil, err
	case res := <-f.ValChan:
		f.ValChan <- res
		return res, nil
	}
}

func (f *Future) LispPrint(_Pr_str func(MalType, bool) string) string {
	return "«futur-call " + _Pr_str(f.Fn.Exp, true) + "»"
}

func (f *Future) Type() string {
	return "future-call"
}

func (f *Future) GetPosition() *Position {
	return f.Cursor
}
