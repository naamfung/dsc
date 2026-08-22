local db = require("db")

type MigrationRecord = {
    id: string,
    applied_at: any,
    description: string?,
}

local M = {}

M.schemas = {
    postgres = [[CREATE TABLE IF NOT EXISTS _migrations (
        id VARCHAR(512) PRIMARY KEY,
        applied_at TIMESTAMP NOT NULL DEFAULT NOW(),
        description TEXT
    )]],
    sqlite = [[CREATE TABLE IF NOT EXISTS _migrations (
        id VARCHAR(512) PRIMARY KEY,
        applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
        description TEXT
    )]],
    mysql = [[CREATE TABLE IF NOT EXISTS _migrations (
        id VARCHAR(512) PRIMARY KEY,
        applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
        description TEXT
    )]],
}

M.table_exists_queries = {
    postgres = [[SELECT EXISTS (SELECT FROM pg_tables WHERE tablename = '_migrations')]],
    sqlite = [[SELECT COUNT(*) AS count FROM sqlite_master WHERE type='table' AND name='_migrations']],
    mysql = [[SELECT COUNT(*) AS count FROM information_schema.tables WHERE table_name = '_migrations']],
}

function M.table_exists(database: db.Database): (boolean?, string?)
    local db_type, err = database:type()
    if err then
        return nil, "Failed to determine database type: " .. tostring(err)
    end
    local check_query = M.table_exists_queries[db_type]
    if not check_query then
        return nil, "Unsupported database type: " .. db_type
    end
    local result, query_err = database:query(check_query)
    if query_err then
        return nil, "Query failed: " .. tostring(query_err)
    end
    if result and result[1] then
        return result[1].exists or (result[1].count and result[1].count > 0), nil
    end
    return false, nil
end

function M.init(database: db.Database): (boolean, string?)
    local exists, err = M.table_exists(database)
    if err then return false, err end
    if exists then return true, nil end

    local db_type, type_err = database:type()
    if type_err then return false, type_err end

    local schema = M.schemas[db_type]
    if not schema then
        return false, "Unsupported database type: " .. db_type
    end
    return database:execute(schema)
end

function M.record(database: db.Database, id: string, description: string?): (boolean, string?)
    return database:execute(
        "INSERT INTO _migrations (id, description) VALUES (?, ?)",
        {id, description or ""}
    )
end

function M.is_applied(database: db.Database, id: string): (boolean, string?)
    local result, err = database:query(
        "SELECT id FROM _migrations WHERE id = ?",
        {id}
    )
    if err then return false, err end
    return result ~= nil and #result > 0, nil
end

return M
