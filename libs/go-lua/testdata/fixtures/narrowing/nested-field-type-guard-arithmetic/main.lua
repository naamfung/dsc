type PayloadCarrier = {
    data: fun(self: PayloadCarrier): any,
}

local function bump(carrier: PayloadCarrier?)
    local data = carrier and carrier:data() or nil
    if type(data) ~= "table" or type(data.amount) ~= "number" then
        return nil
    end

    local next_amount = data.amount + 1
    local exact: number = data.amount
    return next_amount, exact
end

return bump
