package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Shared-self law: setmetatable-backed constructors produce instance values,
// not class-table values, and methods returning self preserve that instance type.
func TestMetatableSharedSelf_LocalConstructorAndMethods(t *testing.T) {
	source := `
		type Counter = {
			value: number,
			inc: (self: Counter) -> Counter,
			get: (self: Counter) -> number,
		}

		local Counter = {}
		Counter.__index = Counter

		function Counter.new(): Counter
			local self: Counter = {
				value = 0,
				inc = Counter.inc,
				get = Counter.get,
			}
			setmetatable(self, Counter)
			return self
		end

		function Counter:inc(): Counter
			self.value = self.value + 1
			return self
		end

		function Counter:get(): number
			return self.value
		end

		local c: Counter = Counter.new()
		local next_counter: Counter = c:inc()
		local value: number = next_counter:get()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Cross-module law: exported constructors must preserve the instance/self contract
// so methods returning self remain callable downstream.
func TestMetatableSharedSelf_CrossModuleConstructorExport(t *testing.T) {
	builderSource := `
		type Builder = {
			_name: string,
			rename: (self: Builder, name: string) -> Builder,
			name: (self: Builder) -> string,
		}

		local Builder = {}
		Builder.__index = Builder

		function Builder.new(name: string): Builder
			local self: Builder = {
				_name = name,
				rename = Builder.rename,
				name = Builder.name,
			}
			setmetatable(self, Builder)
			return self
		end

		function Builder:rename(name: string): Builder
			self._name = name
			return self
		end

		function Builder:name(): string
			return self._name
		end

		local M = {}
		M.new = Builder.new
		return M
	`

	builderModule := testutil.CheckAndExport(builderSource, "builder", testutil.WithStdlib())
	if builderModule.HasError() {
		t.Fatalf("builder module should export cleanly, got: %v", testutil.ErrorMessages(builderModule.Errors))
	}

	consumerSource := `
		local builder = require("builder")

		local b = builder.new("first")
		local renamed = b:rename("second")
		local name: string = renamed:name()
	`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("builder", builderModule),
	)
	if result.HasError() {
		t.Fatalf("expected downstream metatable-backed methods to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Receiver law: once an instance is known to be Session, metatable-backed
// methods must see the instance fields promised by that type.
func TestMetatableSharedSelf_MethodSeesInstanceFields(t *testing.T) {
	source := `
		type Session = {
			session_id: string,
			user_id: string,
			describe: (self: Session) -> string,
		}

		local Session = {}
		Session.__index = Session

		function Session:describe(): string
			return self.session_id .. ":" .. self.user_id
		end

		function Session.new(session_id: string, user_id: string): Session
			local self: Session = {
				session_id = session_id,
				user_id = user_id,
				describe = Session.describe,
			}
			setmetatable(self, Session)
			return self
		end

		local s: Session = Session.new("s1", "u1")
		local label: string = s:describe()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
