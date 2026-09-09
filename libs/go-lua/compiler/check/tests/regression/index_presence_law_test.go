package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestIndexPresence_TruthinessGuard_RepeatedLiteralLookup(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {
			["root"] = {
				_topic = "hello",
				topic = function(self: Message): string
					return self._topic
				end,
			},
		}

		if messages["root"] then
			local topic: string = messages["root"]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_NilCheck_RepeatedLiteralLookup(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {
			["root"] = {
				_topic = "hello",
				topic = function(self: Message): string
					return self._topic
				end,
			},
		}

		if messages["root"] ~= nil then
			local topic: string = messages["root"]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_Assert_RepeatedLiteralLookup(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {
			["root"] = {
				_topic = "hello",
				topic = function(self: Message): string
					return self._topic
				end,
			},
		}

		assert(messages["root"])
		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_DominatingLiteralAssignment_MakesLookupDefinite(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		messages["root"] = {
			_topic = "installed",
			topic = function(self: Message): string
				return self._topic
			end,
		}

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_DominatingLiteralAssignment_SurvivesOtherKeys(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		messages["root"] = {
			_topic = "installed",
			topic = function(self: Message): string
				return self._topic
			end,
		}
		local other = "other"
		messages[other] = {
			_topic = "side",
			topic = function(self: Message): string
				return self._topic
			end,
		}

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_HybridFieldAndMap_LiteralFieldStaysExact(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local t = {}
		t.root = {
			_topic = "hybrid",
			topic = function(self: Message): string
				return self._topic
			end,
		}
		local key: string = "other"
		t[key] = {
			_topic = "mapped",
			topic = function(self: Message): string
				return self._topic
			end,
		}

		local topic: string = t["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_ConstResolvedStringKey_UsesStaticPathLaw(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {
			["root"] = {
				_topic = "hello",
				topic = function(self: Message): string
					return self._topic
				end,
			},
		}
		local key = "root"

		if messages[key] then
			local topic: string = messages[key]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_KeyOfPairsLoop_ProvesDynamicKeyPresence(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {
			["root"] = {
				_topic = "hello",
				topic = function(self: Message): string
					return self._topic
				end,
			},
			["other"] = {
				_topic = "world",
				topic = function(self: Message): string
					return self._topic
				end,
			},
		}

		for key, _ in pairs(messages) do
			local topic: string = messages[key]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_OverwriteWithNil_DirectLookupMustFail(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		messages["root"] = {
			_topic = "hello",
			topic = function(self: Message): string
				return self._topic
			end,
		}
		messages["root"] = nil

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected overwrite-with-nil direct lookup to require a nil check")
	}
}

func TestIndexPresence_OverwriteWithNil_GuardedLookupSucceeds(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		messages["root"] = {
			_topic = "hello",
			topic = function(self: Message): string
				return self._topic
			end,
		}
		messages["root"] = nil

		if messages["root"] then
			local topic: string = messages["root"]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_JoinedInstallOnBothBranches_IsDefiniteAfterJoin(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		local cond = true
		if cond then
			messages["root"] = {
				_topic = "a",
				topic = function(self: Message): string
					return self._topic
				end,
			}
		else
			messages["root"] = {
				_topic = "b",
				topic = function(self: Message): string
					return self._topic
				end,
			}
		end

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_NotPresentGuardThenInstall_IsDefiniteAfterJoin(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		if not messages["root"] then
			messages["root"] = {
				_topic = "installed",
				topic = function(self: Message): string
					return self._topic
				end,
			}
		end

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_JoinedInstallOnlyOnOneBranch_DirectLookupMustFail(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		local cond = true
		if cond then
			messages["root"] = {
				_topic = "a",
				topic = function(self: Message): string
					return self._topic
				end,
			}
		end

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected branch-local installation to require a nil check after join")
	}
}

func TestIndexPresence_JoinedInstallOnlyOnOneBranch_GuardedLookupSucceeds(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		local cond = true
		if cond then
			messages["root"] = {
				_topic = "a",
				topic = function(self: Message): string
					return self._topic
				end,
			}
		end

		if messages["root"] then
			local topic: string = messages["root"]:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIndexPresence_JoinedNilOnOneBranch_DirectLookupMustFail(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local messages: {[string]: Message} = {}
		messages["root"] = {
			_topic = "a",
			topic = function(self: Message): string
				return self._topic
			end,
		}
		local cond = true
		if cond then
			messages["root"] = nil
		end

		local topic: string = messages["root"]:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected niling a key on one branch to require a nil check after join")
	}
}
