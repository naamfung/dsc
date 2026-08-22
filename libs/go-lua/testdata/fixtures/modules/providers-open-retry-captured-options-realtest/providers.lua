local registry = require("registry")
local contract = require("contract")

local providers = {
    _registry = registry,
    _contract = contract,
}

local CONTRACT_ID = "wippy.llm:provider"

function providers.open(provider_id, context_overrides)
    if not provider_id then
        return nil, "Provider ID is required"
    end

    context_overrides = context_overrides or {}

    local provider_entry, err = providers._registry.get(provider_id)
    if err then
        return nil, "Registry error: " .. tostring(err)
    end

    if not provider_entry then
        return nil, "Provider not found: " .. provider_id
    end

    if not provider_entry.meta or provider_entry.meta.type ~= "llm.provider" then
        return nil, "Entry is not a provider: " .. provider_id
    end

    if not provider_entry.data or not provider_entry.data.driver or not provider_entry.data.driver.id then
        return nil, "Provider missing driver configuration: " .. provider_id
    end

    local binding_id = provider_entry.data.driver.id
    local base_options = provider_entry.data.driver.options or {}

    local final_context = {}
    for k, v in pairs(base_options) do
        final_context[k] = v
    end
    for k, v in pairs(context_overrides) do
        final_context[k] = v
    end

    local call_options = {}
    if final_context.retry then
        call_options.retry = final_context.retry
        final_context.retry = nil
    end

    local provider_contract, err = providers._contract.get(CONTRACT_ID)
    if err then
        return nil, "Failed to get provider contract: " .. tostring(err)
    end

    local chain = provider_contract:with_context(final_context)
    if next(call_options) then
        chain = chain:with_options(call_options)
    end

    local instance, open_err = chain:open(tostring(binding_id))
    if open_err then
        return nil, "Failed to open provider binding: " .. tostring(open_err)
    end

    return instance
end

return providers
