package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression: reader-style builder with metatable, clone, and optional filters.
func TestWippyReaderPattern(t *testing.T) {
	source := `
		local reader = {}

		type Filters = {
			email?: string,
			id?: string,
			status?: string,
			ids?: {string},
		}

		type ReaderState = {
			_filters: Filters,
			_limit: number,
			_offset: number,
			_order_by: string,
			_order_dir: string,
			_include_password_hash: boolean,
		}

		local reader_mt: metatable<ReaderState> = { __index = reader }

		function reader.new(): ReaderState
			local filters: Filters = {}
			local state: ReaderState = {
				_filters = filters,
				_limit = 50,
				_offset = 0,
				_order_by = "created_at",
				_order_dir = "DESC",
				_include_password_hash = false,
			}
			return setmetatable(state, reader_mt)
		end

		function reader.with_email(self: ReaderState, email: string): ReaderState
			local copy: ReaderState = reader._clone(self)
			copy._filters.email = email
			return copy
		end

		function reader.with_ids(self: ReaderState, ...: string): ReaderState
			local copy: ReaderState = reader._clone(self)
			copy._filters.ids = {...}
			return copy
		end

		function reader.limit(self: ReaderState, limit: number, offset: number?): ReaderState
			local copy: ReaderState = reader._clone(self)
			copy._limit = limit
			copy._offset = offset or 0
			return copy
		end

		function reader.ids_count(self: ReaderState): number
			if self._filters.ids and #self._filters.ids > 0 then
				return #self._filters.ids
			end
			return 0
		end

		function reader.exists(self: ReaderState): boolean
			local count = reader.ids_count(self)
			return count > 0
		end

		function reader._clone(self: ReaderState): ReaderState
			local copy: ReaderState = {
				_filters = {
					email = self._filters.email,
					id = self._filters.id,
					status = self._filters.status,
					ids = self._filters.ids,
				},
				_limit = self._limit,
				_offset = self._offset,
				_order_by = self._order_by,
				_order_dir = self._order_dir,
				_include_password_hash = self._include_password_hash,
			}
			return setmetatable(copy, reader_mt)
		end

		return reader
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for wippy reader pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
