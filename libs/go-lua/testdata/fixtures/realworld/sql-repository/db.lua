type DbType = "postgres" | "sqlite" | "mysql"
type QueryResult = {[string]: any}

type Database = {
    db_type: DbType,
    type: (self: Database) -> (DbType, string?),
    query: (self: Database, sql: string, params: {any}?) -> ({QueryResult}?, string?),
    execute: (self: Database, sql: string, params: {any}?) -> (boolean, string?),
}

local M = {}
M.Database = Database

M.type = {
    POSTGRES = "postgres",
    SQLITE = "sqlite",
    MYSQL = "mysql",
}

function M.mock(db_type: DbType): Database
    local db: Database = {
        db_type = db_type,
        type = function(self: Database): (DbType, string?)
            return self.db_type, nil
        end,
        query = function(self: Database, sql: string, params: {any}?): ({QueryResult}?, string?)
            return {{exists = true, count = 1}}, nil
        end,
        execute = function(self: Database, sql: string, params: {any}?): (boolean, string?)
            return true, nil
        end,
    }
    return db
end

return M
