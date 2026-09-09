local M = {}

type Response = {
    metadata: {
        response_id: string,
    },
}

function M.request(ok: boolean): (Response?, string?)
    if ok then
        return {
            metadata = {
                response_id = "resp-123",
            },
        }, nil
    end

    return nil, "failed"
end

return M
