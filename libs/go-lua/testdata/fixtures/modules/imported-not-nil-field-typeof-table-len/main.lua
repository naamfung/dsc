local test = require("test")

type Response = {
    result: {
        data: {
            departments: {string}?,
        },
    },
}

local response: Response = {
    result = {
        data = {
            departments = { "engineering", "product" },
        },
    },
}

test.not_nil(response.result.data.departments, "departments required")
test.eq(type(response.result.data.departments), "table", "departments should be a table")

local count: number = #response.result.data.departments
local first: string = response.result.data.departments[1]

return count, first
