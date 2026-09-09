package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestExportedSelfMethodStore_ConstructorReturnMatchesExportedType(t *testing.T) {
	mod := testutil.CheckAndExport(`
type Store = {
    cache: {[string]: string},
    get: (self: Store, key: string) -> string?,
    put: (self: Store, key: string, value: string) -> Store,
}

local Store = {}
Store.__index = Store

local M = {}
M.Store = Store

function M.new(): Store
    local self: Store = {
        cache = {},
        get = Store.get,
        put = Store.put,
    }
    setmetatable(self, Store)
    return self
end

function Store:get(key: string): string?
    return self.cache[key]
end

function Store:put(key: string, value: string): Store
    self.cache[key] = value
    return self
end

return M
`, "store")
	if mod.HasError() {
		t.Fatalf("module errors: %v", testutil.ErrorMessages(mod.Errors))
	}
	if mod.Manifest == nil {
		t.Fatal("expected manifest")
	}

	storeType, ok := mod.Manifest.LookupType("Store")
	if !ok || storeType == nil {
		t.Fatal("expected exported Store type")
	}

	newType, ok := mod.Manifest.LookupValue("new")
	if !ok || newType == nil {
		t.Fatal("expected exported new value type")
	}

	fn := unwrap.Function(newType)
	if fn == nil || len(fn.Returns) != 1 {
		t.Fatalf("expected constructor function type, got %s", typ.FormatShort(newType))
	}

	got := fn.Returns[0]
	if !subtype.IsSubtype(got, storeType) {
		t.Fatalf("constructor return %s is not subtype of exported Store %s", typ.FormatShort(got), typ.FormatShort(storeType))
	}
}
