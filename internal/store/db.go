// Package store 封装 SQLite 持久化。
//
// 除了 SQLite 驱动本身，这个包只依赖标准库，因此测试可以直接开一个
// 临时文件数据库跑真实的 SQL。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	// driverName 是 modernc.org/sqlite 注册的驱动名（纯 Go，无需 CGO）。
	// 若换成 github.com/mattn/go-sqlite3，这里改成 "sqlite3" 即可。
	driverName = "sqlite"

	// schemaVersion 是当前 schema 版本。加迁移时，新版本号与对应的 schemaVn
	// 语句列表要一起加，并且**不要**回头改老的 schemaVn：
	// 全新库也会按顺序跑完所有版本，改了老的会让全新库与升级库产生分叉。
	schemaVersion = 2
)

// ErrNotFound 表示目标记录不存在。
var ErrNotFound = errors.New("记录不存在")

// DB 是应用唯一的数据访问入口。
//
// 桌面端只有一个进程，连接数固定为 1，以此彻底规避 SQLITE_BUSY，
// 也让 pragma（busy_timeout / foreign_keys 是连接级的）只设置一次就够。
type DB struct {
	sql  *sql.DB
	path string
}

// Path 返回数据库文件的绝对路径，供诊断与界面展示。
func (d *DB) Path() string { return d.path }

// Open 打开（必要时创建）数据库文件，应用 pragma 并执行迁移。
func Open(path string) (*DB, error) {
	// 故意不把路径写成 file: URI：Windows 盘符在 SQLite URI 里是个经典坑，
	// pragma 也改成开库后显式执行，这样换驱动不会连带出问题。
	sqlDB, err := sql.Open(driverName, filepath.ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	db := &DB{sql: sqlDB, path: path}
	if err := db.applyPragmas(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close 关闭底层连接池。
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

var pragmas = []string{
	// WAL 写入不阻塞读取，且是持久化在库文件里的。
	"PRAGMA journal_mode = WAL",
	// 单连接下基本用不到，但作为兜底。
	"PRAGMA busy_timeout = 5000",
	"PRAGMA foreign_keys = ON",
	// WAL + NORMAL 是桌面应用的常规组合：崩溃不丢已提交事务。
	"PRAGMA synchronous = NORMAL",
}

func (d *DB) applyPragmas() error {
	for _, stmt := range pragmas {
		if _, err := d.sql.Exec(stmt); err != nil {
			return fmt.Errorf("设置 %q 失败: %w", stmt, err)
		}
	}
	return nil
}

// schemaV1 是 v0 -> v1 的全部语句，在一个事务里按顺序执行。
var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at   TEXT    NOT NULL,
		ended_at     TEXT    NOT NULL,
		duration_sec INTEGER NOT NULL,
		note         TEXT    NOT NULL DEFAULT '',
		source       TEXT    NOT NULL DEFAULT 'timer',
		created_at   TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS timer_state (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		state           TEXT    NOT NULL DEFAULT 'idle',
		started_at      TEXT    NOT NULL DEFAULT '',
		last_resumed_at TEXT    NOT NULL DEFAULT '',
		accumulated_sec INTEGER NOT NULL DEFAULT 0,
		updated_at      TEXT    NOT NULL DEFAULT ''
	)`,
	// 单行表：迁移时就种下 idle 行，之后所有写入都是 UPDATE。
	`INSERT OR IGNORE INTO timer_state(id, state) VALUES (1, 'idle')`,
}

// schemaV2 引入"学习计划"，并让记录与计时状态都能归属到一个计划。
//
// 两点刻意的取舍：
//
//  1. sessions.plan_id **可空且不加外键**。SQLite 的 ALTER TABLE ADD COLUMN
//     允许加带 REFERENCES 的可空列，但这里故意不加：完整性由服务层保证
//     （有记录的计划拒绝删除、只能归档），换来的是"哪怕计划被删，学习记录
//     也一定写得进去"。时长比约束宝贵。
//  2. timer_state.plan_id 是 NOT NULL DEFAULT 0，0 表示未选；同样不加外键，
//     因为 ALTER TABLE 加带 FK 的 NOT NULL 列会被 SQLite 拒绝。
//
// 全新库也会跑完 v1 再跑这一段（所以新装用户同样会走一遍 ALTER TABLE），
// 这正是我们想要的：升级路径与全新安装路径永远一致。
var schemaV2 = []string{
	`CREATE TABLE IF NOT EXISTS plans (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT    NOT NULL,
		color      TEXT    NOT NULL DEFAULT '#3B82F6',
		sort_order INTEGER NOT NULL DEFAULT 0,
		archived   INTEGER NOT NULL DEFAULT 0,
		created_at TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_plans_archived ON plans(archived, sort_order)`,
	`ALTER TABLE sessions ADD COLUMN plan_id INTEGER`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_plan_id ON sessions(plan_id)`,
	`ALTER TABLE timer_state ADD COLUMN plan_id INTEGER NOT NULL DEFAULT 0`,
}

// migrate 依据 PRAGMA user_version 做前向迁移。重复执行是幂等的。
func (d *DB) migrate() error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("开启迁移事务失败: %w", err)
	}
	// 成功提交后这里的 Rollback 会返回 ErrTxDone，忽略即可。
	defer func() { _ = tx.Rollback() }()

	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取 schema 版本失败: %w", err)
	}
	if version >= schemaVersion {
		return tx.Commit()
	}

	if version < 1 {
		for i, stmt := range schemaV1 {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("迁移 v1 第 %d 条语句失败: %w", i+1, err)
			}
		}
		if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
			return fmt.Errorf("写入 schema 版本 1 失败: %w", err)
		}
	}

	if version < 2 {
		for i, stmt := range schemaV2 {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("迁移 v2 第 %d 条语句失败: %w", i+1, err)
			}
		}
		if _, err := tx.Exec("PRAGMA user_version = 2"); err != nil {
			return fmt.Errorf("写入 schema 版本 2 失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移事务失败: %w", err)
	}
	return nil
}

// SchemaVersion 返回当前库里的 schema 版本，供测试与诊断使用。
func (d *DB) SchemaVersion() (int, error) {
	var v int
	if err := d.sql.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("读取 schema 版本失败: %w", err)
	}
	return v, nil
}

// JournalMode 返回当前 journal 模式，供测试确认 WAL 生效。
func (d *DB) JournalMode() (string, error) {
	var mode string
	if err := d.sql.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return "", fmt.Errorf("读取 journal_mode 失败: %w", err)
	}
	return mode, nil
}
