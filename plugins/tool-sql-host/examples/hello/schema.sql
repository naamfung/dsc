-- 插件自有表：打包期落库（运行期不允许 DDL，见 tool-sql-host 的 SQL 守卫）。
CREATE TABLE IF NOT EXISTS note (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    text TEXT NOT NULL,
    at   TEXT NOT NULL
);
