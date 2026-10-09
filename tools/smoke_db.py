"""冒烟测试用的 SQLite 辅助脚本，由 tools/smoke_test.ps1 调用。

单独成一个文件而不是内联在 PowerShell 里：PowerShell 向原生命令传参时会吃掉
括号和引号，把 Python 代码写在命令行里并不可靠（`print(...)` 会变成语法错误）。

用法：
    python tools/smoke_db.py --check
    python tools/smoke_db.py inspect <db>
    python tools/smoke_db.py set-setting <db> <key> <value>
    python tools/smoke_db.py get-setting <db> <key>
    python tools/smoke_db.py seed-running <db> <interval-min>
    python tools/smoke_db.py seed-v1-db <db>
    python tools/smoke_db.py seed-plan-and-stale-running <db>
"""

import datetime
import sqlite3
import sys

# v1 的建表语句快照。
#
# 刻意在这里复制一份而不是从 Go 里读：这个场景要的正是"一个老版本留下的库"，
# 而 v1 是已经冻结的历史，不会再变。**不要**跟着 schema 的演进更新它——
# 它一变，这个迁移测试就失去意义了。
V1_SCHEMA = [
    """CREATE TABLE sessions (
        id           INTEGER PRIMARY KEY AUTOINCREMENT,
        started_at   TEXT    NOT NULL,
        ended_at     TEXT    NOT NULL,
        duration_sec INTEGER NOT NULL,
        note         TEXT    NOT NULL DEFAULT '',
        source       TEXT    NOT NULL DEFAULT 'timer',
        created_at   TEXT    NOT NULL
    )""",
    "CREATE INDEX idx_sessions_started_at ON sessions(started_at)",
    "CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    """CREATE TABLE timer_state (
        id              INTEGER PRIMARY KEY CHECK (id = 1),
        state           TEXT    NOT NULL DEFAULT 'idle',
        started_at      TEXT    NOT NULL DEFAULT '',
        last_resumed_at TEXT    NOT NULL DEFAULT '',
        accumulated_sec INTEGER NOT NULL DEFAULT 0,
        updated_at      TEXT    NOT NULL DEFAULT ''
    )""",
    "INSERT INTO timer_state(id, state) VALUES (1, 'idle')",
]

# 冒烟测试在 v1 库里留下的标记，迁移后必须还能找到。
V1_MARKER_NOTE = "v1-legacy-record"
V1_SESSION_SEC = 1800


def inspect(db_path: str) -> int:
    conn = sqlite3.connect(db_path)
    try:
        version = conn.execute("PRAGMA user_version").fetchone()[0]
        print(f"user_version={version}")

        tables = [
            row[0]
            for row in conn.execute(
                "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
            )
        ]
        print(f"tables={','.join(tables)}")

        # sessions 有没有 plan_id 列（v2 迁移是否真的加上了）。
        cols = [row[1] for row in conn.execute("PRAGMA table_info(sessions)")]
        print(f"sessions_has_plan_id={1 if 'plan_id' in cols else 0}")

        print(f"journal_mode={conn.execute('PRAGMA journal_mode').fetchone()[0]}")

        row = conn.execute("SELECT state FROM timer_state WHERE id = 1").fetchone()
        print(f"timer_state={row[0] if row else ''}")

        # 心跳有没有在跑，看这个数字有没有涨就知道。
        acc = conn.execute("SELECT accumulated_sec FROM timer_state WHERE id = 1").fetchone()
        print(f"timer_accumulated_sec={acc[0] if acc else ''}")

        plans = conn.execute("SELECT COUNT(*) FROM plans").fetchone()[0]
        print(f"plans={plans}")

        # 最新一条记录：迁移场景看它还在不在、归属对不对。
        latest = conn.execute(
            "SELECT duration_sec, note, source, plan_id FROM sessions "
            "ORDER BY id DESC LIMIT 1"
        ).fetchone()
        if latest:
            print(f"last_session_sec={latest[0]}")
            print(f"last_session_note={latest[1]}")
            print(f"last_session_source={latest[2]}")
            print(f"last_session_plan_id={latest[3] if latest[3] is not None else 'NULL'}")
        else:
            print("last_session_sec=")
            print("last_session_note=")
            print("last_session_source=")
            print("last_session_plan_id=")

        print(f"sessions={conn.execute('SELECT COUNT(*) FROM sessions').fetchone()[0]}")
    finally:
        conn.close()
    return 0


def _upsert_setting(conn: sqlite3.Connection, key: str, value: str) -> None:
    conn.execute(
        "INSERT INTO settings(key, value) VALUES(?, ?) "
        "ON CONFLICT(key) DO UPDATE SET value = excluded.value",
        (key, value),
    )


def seed_running(db_path: str, interval_min: int) -> int:
    """把计时状态播成"正在计时中的一次会话"。

    这样下次启动会走真实的崩溃恢复分支，随后心跳照常推进时长，
    用来验证"重启 → 恢复会话 → 心跳落库"这条链路。

    interval_min 参数留着是为了不改调用方签名：休息提醒已经整条删除，
    它现在只是被忽略（也不再往库里写提醒相关的键）。
    """
    conn = sqlite3.connect(db_path)
    try:
        now = datetime.datetime.now().astimezone().replace(microsecond=0).isoformat()
        conn.execute(
            "UPDATE timer_state SET state = 'running', started_at = ?, "
            "last_resumed_at = ?, accumulated_sec = 0 WHERE id = 1",
            (now, now),
        )
        conn.commit()

        row = conn.execute(
            "SELECT state, last_resumed_at FROM timer_state WHERE id = 1"
        ).fetchone()
        print(f"timer_state={row[0]},{row[1]}")
    finally:
        conn.close()
    return 0


def set_setting(db_path: str, key: str, value: str) -> int:
    conn = sqlite3.connect(db_path)
    try:
        _upsert_setting(conn, key, value)
        conn.commit()
        row = conn.execute("SELECT value FROM settings WHERE key = ?", (key,)).fetchone()
        print(f"{key}={row[0] if row else ''}")
    finally:
        conn.close()
    return 0


def get_setting(db_path: str, key: str) -> int:
    conn = sqlite3.connect(db_path)
    try:
        row = conn.execute("SELECT value FROM settings WHERE key = ?", (key,)).fetchone()
        print(f"{key}={row[0] if row else ''}")
    finally:
        conn.close()
    return 0


def seed_v1_db(db_path: str) -> int:
    """造一个 v1 时代的库（含一条旧记录与旧设置）。

    应用启动时应当把它迁移到 v2：旧记录一条不丢、plan_id 为 NULL、
    旧设置键还在。这是唯一一条能证明"升级不丢数据"的端到端路径。
    """
    conn = sqlite3.connect(db_path)
    try:
        for stmt in V1_SCHEMA:
            conn.execute(stmt)
        conn.execute("PRAGMA user_version = 1")

        started = "2026-05-01T09:00:00+08:00"
        conn.execute(
            "INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at)"
            " VALUES(?, ?, ?, ?, 'timer', ?)",
            (started, "2026-05-01T09:30:00+08:00", V1_SESSION_SEC, V1_MARKER_NOTE, started),
        )
        _upsert_setting(conn, "theme", "dark")
        conn.commit()

        version = conn.execute("PRAGMA user_version").fetchone()[0]
        count = conn.execute("SELECT COUNT(*) FROM sessions").fetchone()[0]
        print(f"user_version={version}")
        print(f"sessions={count}")
        print(f"note={V1_MARKER_NOTE}")
    finally:
        conn.close()
    return 0


def seed_plan_and_stale_running(db_path: str, stale_hours: int = 10) -> int:
    """预置"一个计划 + 一段 10 小时前中断的会话"。

    启动时恢复逻辑会判定它中断过久（超过 8 小时），于是自动结算并落库。
    这样不需要任何界面交互，就能验证
    "timer_state.plan_id -> 恢复 -> FinishSession -> sessions.plan_id"
    这整条数据通路。
    """
    conn = sqlite3.connect(db_path)
    try:
        now = datetime.datetime.now().astimezone().replace(microsecond=0)
        stale = (now - datetime.timedelta(hours=stale_hours)).isoformat()
        now_s = now.isoformat()

        # 直接删掉可能存在的同名计划，保证每次跑都是干净的。
        conn.execute("DELETE FROM plans WHERE name = '冒烟计划'")
        conn.execute(
            "INSERT INTO plans(name, color, sort_order, archived, created_at)"
            " VALUES('冒烟计划', '#3B82F6', 1, 0, ?)",
            (now_s,),
        )
        plan_id = conn.execute("SELECT id FROM plans ORDER BY id DESC LIMIT 1").fetchone()[0]

        _upsert_setting(conn, "currentPlanId", str(plan_id))

        conn.execute(
            "UPDATE timer_state SET state = 'running', started_at = ?, "
            "last_resumed_at = ?, accumulated_sec = 600, plan_id = ? WHERE id = 1",
            (stale, stale, plan_id),
        )
        conn.commit()

        state = conn.execute("SELECT state FROM timer_state WHERE id = 1").fetchone()[0]
        print(f"plan_id={plan_id}")
        print(f"timer_state={state}")
        print(f"last_resumed_at={stale}")
    finally:
        conn.close()
    return 0


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: smoke_db.py <--check|inspect|set-setting|get-setting|"
              "seed-running|seed-v1-db|seed-plan-and-stale-running> [db] [args]",
              file=sys.stderr)
        return 2

    command = argv[1]
    if command == "--check":
        # 走到这里说明 sqlite3 已经导入成功。
        print("ok")
        return 0

    if len(argv) < 3:
        print("missing db path", file=sys.stderr)
        return 2

    if command == "inspect":
        return inspect(argv[2])
    if command == "set-setting":
        if len(argv) < 5:
            print("set-setting needs <db> <key> <value>", file=sys.stderr)
            return 2
        return set_setting(argv[2], argv[3], argv[4])
    if command == "get-setting":
        if len(argv) < 4:
            print("get-setting needs <db> <key>", file=sys.stderr)
            return 2
        return get_setting(argv[2], argv[3])
    if command == "seed-running":
        interval = int(argv[3]) if len(argv) > 3 else 1
        return seed_running(argv[2], interval)
    if command == "seed-v1-db":
        return seed_v1_db(argv[2])
    if command == "seed-plan-and-stale-running":
        hours = int(argv[3]) if len(argv) > 3 else 10
        return seed_plan_and_stale_running(argv[2], hours)

    print(f"unknown command: {command}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
