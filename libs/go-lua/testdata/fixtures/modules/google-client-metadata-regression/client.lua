local json = require("json")
local http_client = require("http_client")
local output = require("output")

type StreamInput = {
    stream: any,
    metadata: table?,
}

local client = {
    _http_client = http_client
}

local function extract_response_metadata(response_body: any)
    if not response_body then
        return {}
    end

    local metadata = {}
    metadata.model_version = response_body.modelVersion
    metadata.response_id = response_body.responseId
    metadata.create_time = response_body.createTime

    return metadata
end

local function parse_error_response(http_response)
    local error_info = {
        status_code = http_response.status_code,
        message = "Google API error"
    }

    if http_response.body then
        local parsed, decode_err = json.decode(http_response.body)
        if not decode_err and parsed then
            error_info.metadata = extract_response_metadata(parsed)
        end
    end

    return error_info
end

function client.process_stream(stream_response: StreamInput, callbacks)
    return nil, "stream not used", {
        content = "",
        tool_calls = {},
        metadata = stream_response.metadata or {},
    }
end

function client.request(method, url, http_options)
    http_options = http_options or {}

    local response, err
    if method == "GET" then
        response, err = client._http_client.get(url, http_options)
    elseif method == "PUT" then
        response, err = client._http_client.put(url, http_options)
    elseif method == "PATCH" then
        response, err = client._http_client.patch(url, http_options)
    else
        response, err = client._http_client.post(url, http_options)
    end

    if not response then
        return nil, {
            status_code = 0,
            message = "Connection failed: " .. tostring(err)
        }
    end

    if response.status_code < 200 or response.status_code >= 300 then
        local parsed_error = parse_error_response(response)
        return nil, parsed_error
    end

    if http_options.stream and response.stream then
        return {
            stream = response.stream,
            status_code = response.status_code,
            headers = response.headers,
            metadata = extract_response_metadata(response)
        }
    end

    local parsed, parse_err = json.decode(response.body)
    if parse_err then
        local parse_error = {
            status_code = response.status_code,
            message = "Failed to parse Google response: " .. parse_err,
            metadata = {}
        }
        return nil, parse_error
    end

    parsed.metadata = extract_response_metadata(parsed)
    parsed.status_code = response.status_code

    return parsed
end

return client
