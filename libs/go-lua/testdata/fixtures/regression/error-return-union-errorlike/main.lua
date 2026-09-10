type GenError = {
    message: string,
}

local id_source = {}

function id_source.v7(): (string, GenError?)
    return "id", nil
end

type ActiveSession = {
    pid: any,
}

local active_sessions = {} :: {[string]: ActiveSession}

local function create_session(payload_data)
    if not payload_data then
        return nil, "missing payload"
    end

    local session_id = payload_data.session_id
    if not session_id then
        local id, err = id_source.v7()
        if err then
            return nil, err
        end
        session_id = id
    end

    if not payload_data.start_token then
        return nil, "missing token"
    end

    return session_id, nil
end

local function use_session(payload_data)
    local created_session_id, err = create_session(payload_data)
    if err then
        return
    end

    local recovered_session_info = active_sessions[created_session_id]
    if recovered_session_info then
        return recovered_session_info.pid
    end
end

return use_session
