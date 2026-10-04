"""冒烟测试用的 SQLite 辅助脚本，由 tools/smoke_test.ps1 调用。

单独成一个文件而不是内联在 PowerShell 里：PowerShell 向原生命令传参时会吃掉
括号和引号，把 Python 代码写在命令行里并不可靠（`print(...)` 会变成语法错误）。

用法：
    python tools/smoke_db.py --check
    python tools/smoke_db.py inspect <db>
    python tools/smoke_db.py disable-minimize-to-tray <db>
    python tools/smoke_db.py seed-running <db> <interval-min>
"""

import datetime
import sqlite3
import sys


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

        print(f"journal_mode={conn.execute('PRAGMA journal_mode').fetchone()[0]}")

        row = conn.execute("SELECT state FROM timer_state WHERE id = 1").fetchone()
        print(f"timer_state={row[0] if row else ''}")

        # 心跳有没有在跑，看这个数字有没有涨就知道。
        acc = conn.execute("SELECT accumulated_sec FROM timer_state WHERE id = 1").fetchone()
        print(f"timer_accumulated_sec={acc[0] if acc else ''}")

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

    这样下次启动会走真实的崩溃恢复分支，随后心跳照常推进；
    再配合 1 分钟的提醒间隔，就能在无人值守的情况下验证
    "计时 → 心跳 → 休息提醒 → 系统通知" 这整条链路。
    """
    conn = sqlite3.connect(db_path)
    try:
        now = datetime.datetime.now().astimezone().replace(microsecond=0).isoformat()
        conn.execute(
            "UPDATE timer_state SET state = 'running', started_at = ?, "
            "last_resumed_at = ?, accumulated_sec = 0 WHERE id = 1",
            (now, now),
        )
        _upsert_setting(conn, "breakReminderEnabled", "true")
        _upsert_setting(conn, "breakIntervalMin", str(interval_min))
        conn.commit()

        row = conn.execute(
            "SELECT state, last_resumed_at FROM timer_state WHERE id = 1"
        ).fetchone()
        print(f"timer_state={row[0]},{row[1]}")
        print(f"breakIntervalMin={interval_min}")
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


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: smoke_db.py <--check|inspect|set-setting|get-setting|"
              "seed-running> [db] [args]", file=sys.stderr)
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

    print(f"unknown command: {command}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
