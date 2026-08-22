package pathkey

import (
	"strconv"

	"github.com/wippyai/go-lua/types/cfg"
)

// SymbolRoot returns the canonical symbol root "sym<id>".
func SymbolRoot(sym cfg.SymbolID) string {
	return "sym" + strconv.FormatUint(uint64(sym), 10)
}

// SymbolVersionRoot returns the canonical versioned symbol root "sym<id>@<version>".
func SymbolVersionRoot(sym cfg.SymbolID, versionID int) string {
	return SymbolRoot(sym) + "@" + strconv.Itoa(versionID)
}
