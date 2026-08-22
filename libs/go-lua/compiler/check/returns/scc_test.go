package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestComputeSymbolSCCs(t *testing.T) {
	t.Run("empty adj returns nil", func(t *testing.T) {
		result := ComputeSymbolSCCs(nil)
		if result != nil {
			t.Error("expected nil")
		}
	})

	t.Run("single node", func(t *testing.T) {
		adj := map[cfg.SymbolID][]cfg.SymbolID{
			1: {},
		}
		result := ComputeSymbolSCCs(adj)
		if len(result) != 1 {
			t.Errorf("expected 1 SCC, got %d", len(result))
		}
	})
}
