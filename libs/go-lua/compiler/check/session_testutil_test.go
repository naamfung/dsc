package check

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/typ"
)

func setConstructorFieldsNextForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	if s.InterprocNext == nil {
		s.InterprocNext = store.NewInterprocState()
	}
	s.InterprocNext.ConstructorFields = fields
}

func setConstructorFieldsPrevForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	if s.InterprocPrev == nil {
		s.InterprocPrev = store.NewInterprocState()
	}
	s.InterprocPrev.ConstructorFields = fields
}
