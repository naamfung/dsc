local contract = require("contract")

local providers = {
    _contract = contract,
}

function providers.open(provider_id, context_overrides)
    context_overrides = context_overrides or {}

    local provider_contract, err = providers._contract.get("provider")
    if err then
        return nil, err
    end

    local chain = provider_contract:with_context({})
    chain = chain:with_options({
        retry = context_overrides.retry,
    })

    return chain:open(provider_id)
end

return providers
