local db = require("db")
local repository = require("repository")

local database = db.mock("postgres")

local exists, err = repository.table_exists(database)
if err then
    print("Error: " .. err)
end

local ok, init_err = repository.init(database)

local recorded, rec_err = repository.record(database, "001_init", "Initial schema")

local applied, app_err = repository.is_applied(database, "001_init")
if applied then
    print("Migration 001_init is applied")
end
