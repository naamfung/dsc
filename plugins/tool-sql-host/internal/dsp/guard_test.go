package dsp

import (
	"strings"
	"testing"
)

// TestGuardSQLAllowed 覆盖守卫放行的语句：自有表的数据读写与只读查询都能过。
func TestGuardSQLAllowed(t *testing.T) {
	cases := []string{
		`SELECT * FROM note`,
		`SELECT id, text FROM note WHERE text = 'attach; DROP TABLE note'`, // 字面量内的分号/关键字不算
		`SELECT * FROM note -- ; DROP TABLE note`,
		`SELECT * FROM /* ATTACH 'x' */ note`,
		`INSERT INTO note(id, text) VALUES(?, ?)`,
		`UPDATE note SET text = 'a' WHERE id = 1`,
		`DELETE FROM note WHERE id = 1`,
		`REPLACE INTO note(id, text) VALUES(1, 'x')`,
		`WITH t AS (SELECT 1 AS a) SELECT a FROM t`,
		`VALUES (1)`,
		`PRAGMA table_info(note)`,
		`SELECT * FROM dsp_state`, // 读保留表允许
	}
	for _, c := range cases {
		if err := GuardSQL(c); err != nil {
			t.Errorf("GuardSQL(%q) 应放行，得到 %v", c, err)
		}
	}
}

// TestGuardSQLRejected 覆盖守卫拦截的语句：多语句、跨库挂载、DDL、写保留表、写型 PRAGMA。
func TestGuardSQLRejected(t *testing.T) {
	cases := []struct{ sql, want string }{
		{``, "空"},
		{`   `, "空"},
		{`;`, "空"},
		{`SELECT 1; SELECT 2`, "单条"},
		{`SELECT 1; DROP TABLE note`, "单条"},
		{`ATTACH DATABASE 'other.dsp' AS o`, "ATTACH"},
		{`DETACH DATABASE o`, "DETACH"},
		{`CREATE TABLE x(a TEXT)`, "CREATE"},
		// 触发器体的分号会被切成多段（不做 BEGIN…END 嵌套识别），仍被拒绝——
		// 保守拒绝优于放行；打包期会在 schema.sql 上显式报错而非静默产出坏结构。
		{`CREATE TRIGGER tr AFTER UPDATE ON note BEGIN UPDATE dsp_meta SET value = 'x'; END`, "单条"},
		{`DROP TABLE note`, "DROP"},
		{`ALTER TABLE note ADD COLUMN b TEXT`, "ALTER"},
		{`VACUUM`, "VACUUM"},
		{`UPDATE dsp_meta SET value = 'x' WHERE key = 'entry'`, "保留表"},
		{`INSERT INTO dsp_blobs(name) VALUES('a')`, "保留表"},
		{`DELETE FROM dsp_blobs`, "保留表"},
		{`WITH t AS (SELECT 1) DELETE FROM dsp_blobs`, "保留表"},
		{`PRAGMA journal_mode = WAL`, "PRAGMA"},
		{`EXPLAIN SELECT 1`, "关键字"},
	}
	for _, c := range cases {
		err := GuardSQL(c.sql)
		if err == nil {
			t.Errorf("GuardSQL(%q) 应拒绝", c.sql)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("GuardSQL(%q) 错误 = %q，期望含 %q", c.sql, err, c.want)
		}
	}
}

// TestSplitStatements 覆盖顶层分号切分：字符串/注释/方括号标识符内的分号不参与切分。
func TestSplitStatements(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{``, 0},
		{`;;`, 0},
		{`SELECT 1`, 1},
		{`SELECT 1;`, 1},
		{`SELECT 1; SELECT 2`, 2},
		{`CREATE TABLE t(a TEXT DEFAULT ';'); INSERT INTO t VALUES(';')`, 2},
		{`SELECT 'a'';b' FROM t; SELECT 2`, 2},
		{`SELECT [weird;name] FROM t`, 1},
		{`-- 注释里的 ; 不算
SELECT 1`, 1},
		{`SELECT 1 /* ; ; */; SELECT 2`, 2},
	}
	for _, c := range cases {
		if got := len(SplitStatements(c.sql)); got != c.want {
			t.Errorf("SplitStatements(%q) = %d 段，期望 %d：%v", c.sql, got, c.want, SplitStatements(c.sql))
		}
	}
}
