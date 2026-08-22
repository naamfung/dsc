type ChanInt = {__tag: "int"}
type ChanStr = {__tag: "str"}
type SelResult =
    {channel: ChanInt, value: {error: string}, ok: boolean} |
    {channel: ChanStr, value: {data: number}, ok: boolean}

function get_result(a: ChanInt, b: ChanStr): SelResult
    return {channel = a, value = {error = "oops"}, ok = true}
end

function f(ch1: ChanInt, ch2: ChanStr)
    local result = get_result(ch1, ch2)
    if result.channel == ch1 then
        local e: string = result.value.error
    end
end
