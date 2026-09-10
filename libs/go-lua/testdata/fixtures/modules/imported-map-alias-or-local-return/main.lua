local context = require("context")

function with_default(initial: context.Context?): context.Context
    local ctx = initial or context.empty()
    return ctx
end
