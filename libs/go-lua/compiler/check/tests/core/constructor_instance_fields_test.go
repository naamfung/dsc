package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestConstructorInstanceFields tests that fields assigned to self in T.new()
// are visible on self in T:method() receivers.
//
// Pattern:
//
//	local T = {}
//	T.__index = T
//	function T.new(session_id, user_id)
//	  local self = setmetatable({}, T)
//	  self.session_id = session_id
//	  self.user_id = user_id
//	  return self
//	end
//	function T:method()
//	  self.session_id -- should be known
//	end
func TestConstructorInstanceFields(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "basic constructor field propagation",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string, user_id: string)
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    self.user_id = user_id
    return self
end

function session_writer:add_message(role: string, content: string)
    local sid: string = self.session_id
    local uid: string = self.user_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor with index notation",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string, user_id: string)
    local self = setmetatable({}, session_writer)
    self["session_id"] = session_id
    self["user_id"] = user_id
    return self
end

function session_writer:add_message(role: string, content: string)
    local sid: string = self.session_id
    local uid: string = self.user_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor fields in multiple methods",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string)
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    return self
end

function session_writer:get_session_id(): string
    return self.session_id
end

function session_writer:log_session()
    local sid = self.session_id
    local x: string = sid
    print(x)
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor with metatable in table literal",
			Code: `
local session_writer = {}

function session_writer.new(session_id: string)
    local self = setmetatable({}, { __index = session_writer })
    self.session_id = session_id
    return self
end

function session_writer:get_session_id(): string
    return self.session_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor field assigned from call result",
			Code: `
local function generate_id(): string
    return "id-123"
end

local session_writer = {}
session_writer.__index = session_writer

function session_writer.new()
    local self = setmetatable({}, session_writer)
    self.session_id = generate_id()
    return self
end

function session_writer:get_session_id(): string
    return self.session_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor with conditional field assignment",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string?)
    local self = setmetatable({}, session_writer)
    if session_id then
        self.session_id = session_id
    else
        self.session_id = "default"
    end
    return self
end

function session_writer:get_session_id(): string
    return self.session_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method chaining with constructor fields",
			Code: `
local Builder = {}
Builder.__index = Builder

function Builder.new()
    local self = setmetatable({}, Builder)
    self.value = 0
    return self
end

function Builder:add(n: number)
    self.value = self.value + n
    return self
end

function Builder:get(): number
    return self.value
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "constructor assigned as function expression",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

session_writer.new = function(session_id: string, user_id: string)
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    self.user_id = user_id
    return self
end

function session_writer:add_message(role: string, content: string)
    local sid: string = self.session_id
    local uid: string = self.user_id
end
`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestConstructorInstanceFields_NegativeCases tests cases that should produce errors.
func TestConstructorInstanceFields_NegativeCases(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "accessing field not assigned in constructor",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string)
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    return self
end

function session_writer:get_user_id(): string
    return self.user_id
end
`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "constructor with early return nil does not propagate fields",
			Code: `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string?)
    if not session_id then
        return nil
    end
    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    return self
end

function session_writer:get_session_id(): string
    return self.session_id
end
`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
