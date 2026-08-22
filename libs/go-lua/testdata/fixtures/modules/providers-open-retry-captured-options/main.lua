local test = require("test")
local providers = require("providers")

local captured_options = nil

providers._contract = {
    get = function(_contract_id)
        return {
            with_context = function(self, _context)
                return self
            end,
            with_options = function(self, opts)
                captured_options = opts
                return self
            end,
            open = function(self, binding_id)
                return { _binding_id = binding_id }, nil
            end,
        }, nil
    end,
}

local instance, err = providers.open("wippy.llm.provider:openai", {
    retry = { max_attempts = 3, initial_delay = 100 },
})

test.is_nil(err, "open should succeed")
assert(instance)
test.not_nil(captured_options, "captured options expected")
test.not_nil(captured_options.retry, "retry expected")

local attempts: number = captured_options.retry.max_attempts
local delay: number = captured_options.retry.initial_delay

return attempts, delay
