local registry = {}

function registry.get(_id)
    return nil, "not configured"
end

function registry.find(_query)
    return {}, nil
end

return registry
