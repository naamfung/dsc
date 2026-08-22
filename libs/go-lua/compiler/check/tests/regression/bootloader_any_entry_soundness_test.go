package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Soundness regression for the bootloader pattern:
// assigning `entry.id` to a `string` local must fail when `entry` is explicitly `any`.
func TestBootloader_ExplicitAnyEntry_AssignToStringIsError(t *testing.T) {
	source := `
		type BootloaderEntry = {
			id: string,
		}

		local function execute_bootloader(entry: any)
			local bootloader_id: string = entry.id
			return bootloader_id
		end

		local e: BootloaderEntry = { id = "boot:one" }
		execute_bootloader(e)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected error: cannot assign any to string")
	}

	found := false
	for _, d := range result.Errors {
		if strings.Contains(d.Message, "cannot assign any to string") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'cannot assign any to string', got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Control case: when entry is typed, assigning entry.id to string is valid.
func TestBootloader_TypedEntry_AssignToStringNoError(t *testing.T) {
	source := `
		type BootloaderEntry = {
			id: string,
		}

		local function execute_bootloader(entry: BootloaderEntry)
			local bootloader_id: string = entry.id
			return bootloader_id
		end

		local e: BootloaderEntry = { id = "boot:one" }
		local id: string = execute_bootloader(e)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for typed entry, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
