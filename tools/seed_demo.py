"""往数据库里灌一份"像用了两个月"的演示数据，用来截图和肉眼验收界面。

为什么需要它：柱状图、月历热力图、按计划拆分这些界面，空数据下什么都看不出来。
没有像样的数据，就没法判断设计到底成不成立。

用法（**先退出应用**，否则 SQLite 文件被占用）：

    python tools/seed_demo.py                     # 默认 %LOCALAPPDATA%\\GuXueTimer\\timer.db
    python tools/seed_demo.py <db 路径>
    python tools/seed_demo.py --days 90           # 灌更长的历史
    python tools/seed_demo.py --clear             # 先清空记录与计划再灌

数据是确定性的（固定随机种子），所以每次生成的图案一样，截图之间可以对比。

注意：这份脚本按 v2 的表结构直接写库，只用于开发期造数据。
应用自己的写入路径在 internal/store 里，两者互不影响。
"""

import argparse
import datetime
import os
import random
import sqlite3
import sys

# 固定的种子：截图要能互相比较，每次跑出来的图案必须一样。
SEED = 20261005

# 计划名与配色取自后端的白名单（main 包的 planPalette），保证颜色都在可选范围内。
PLANS = [
    ("高等数学", "#3B82F6", 5),   # 权重 = 大致的时间占比
    ("英语单词", "#22D3EE", 3),
    ("数据结构", "#6366F1", 3),
    ("计算机网络", "#14B8A6", 2),
]

# 一天里可能的开始时刻（小时）与时长区间（分钟）。
START_HOURS = [8, 10, 14, 16, 19, 21]
DURATION_RANGES = [(20, 35), (35, 60), (60, 95)]


def default_db_path() -> str:
    base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
    return os.path.join(base, "GuXueTimer", "timer.db")


def check_schema(conn: sqlite3.Connection) -> None:
    version = conn.execute("PRAGMA user_version").fetchone()[0]
    if version != 2:
        raise SystemExit(
            f"这个库的 user_version = {version}，不是 v2。"
            "先启动一次应用让它完成迁移，再来灌数据。"
        )
    cols = [row[1] for row in conn.execute("PRAGMA table_info(sessions)")]
    if "plan_id" not in cols:
        raise SystemExit("sessions 里没有 plan_id 列，库结构与 v2 不符。")


def clear(conn: sqlite3.Connection) -> None:
    conn.execute("DELETE FROM sessions")
    conn.execute("DELETE FROM plans")
    conn.execute("UPDATE timer_state SET state = 'idle', started_at = '', "
                 "last_resumed_at = '', accumulated_sec = 0, plan_id = 0 WHERE id = 1")


def seed(conn: sqlite3.Connection, days: int) -> dict:
    rnd = random.Random(SEED)
    now = datetime.datetime.now().astimezone().replace(microsecond=0)

    # ---- 计划 ----
    plan_ids: list[int] = []
    weights: list[int] = []
    for order, (name, color, weight) in enumerate(PLANS, start=1):
        created = (now - datetime.timedelta(days=days)).isoformat()
        cur = conn.execute(
            "INSERT INTO plans(name, color, sort_order, archived, created_at)"
            " VALUES(?, ?, ?, 0, ?)",
            (name, color, order, created),
        )
        plan_ids.append(cur.lastrowid)
        weights.append(weight)
    # 让"计算机网络"处于归档状态：归档区也该有东西可看。
    conn.execute("UPDATE plans SET archived = 1 WHERE id = ?", (plan_ids[-1],))

    # ---- 记录 ----
    inserted = 0
    total_sec = 0
    for offset in range(days - 1, -1, -1):
        day = (now - datetime.timedelta(days=offset)).date()
        weekday = day.weekday()

        # 休息日：工作日约两成、周末约一半不学。图案有明显的疏密才像真实记录。
        if rnd.random() < (0.5 if weekday >= 5 else 0.18):
            continue

        sessions = rnd.choices([1, 2, 3, 4], weights=[3, 4, 3, 2])[0]
        hours = rnd.sample(START_HOURS, k=sessions)
        for hour in sorted(hours):
            low, high = rnd.choice(DURATION_RANGES)
            minutes = rnd.randint(low, high)
            plan_id = rnd.choices(plan_ids, weights=weights)[0]

            started = datetime.datetime.combine(day, datetime.time(hour, rnd.choice([0, 5, 10, 30])))
            started = started.replace(tzinfo=now.tzinfo)
            ended = started + datetime.timedelta(minutes=minutes)

            conn.execute(
                "INSERT INTO sessions(started_at, ended_at, duration_sec, note, source,"
                " created_at, plan_id) VALUES(?, ?, ?, '', 'timer', ?, ?)",
                (
                    started.isoformat(),
                    ended.isoformat(),
                    minutes * 60,
                    ended.isoformat(),
                    plan_id,
                ),
            )
            inserted += 1
            total_sec += minutes * 60

    # ---- 当前计划与设置 ----
    def upsert(key: str, value: str) -> None:
        conn.execute(
            "INSERT INTO settings(key, value) VALUES(?, ?)"
            " ON CONFLICT(key) DO UPDATE SET value = excluded.value",
            (key, value),
        )

    # 当前计划选一个在用的（最后那个已归档，不能当默认）。
    upsert("currentPlanId", str(plan_ids[0]))
    upsert("theme", "light")
    upsert("breakReminderEnabled", "true")
    upsert("breakIntervalMin", "45")
    upsert("minimizeToTray", "true")

    conn.execute("UPDATE timer_state SET state = 'idle', started_at = '', "
                 "last_resumed_at = '', accumulated_sec = 0, plan_id = 0 WHERE id = 1")
    conn.commit()

    return {"plans": len(PLANS), "sessions": inserted, "total_sec": total_sec}


def main() -> int:
    parser = argparse.ArgumentParser(description="给孤雪学习计时器灌演示数据")
    parser.add_argument("db", nargs="?", default=default_db_path(), help="timer.db 路径")
    parser.add_argument("--days", type=int, default=75, help="灌多少天的历史（默认 75）")
    parser.add_argument("--clear", action="store_true", help="先清空记录与计划")
    args = parser.parse_args()

    if not os.path.exists(args.db):
        print(f"找不到数据库：{args.db}", file=sys.stderr)
        print("先启动一次应用让它建库，再回来灌数据。", file=sys.stderr)
        return 2

    conn = sqlite3.connect(args.db)
    try:
        check_schema(conn)
        if args.clear:
            clear(conn)
            print("已清空原有记录与计划")
        stats = seed(conn, args.days)
    except sqlite3.OperationalError as exc:
        print(f"写库失败：{exc}", file=sys.stderr)
        print("应用正在运行吗？SQLite 同一时间只允许一个写入者，先退出应用。", file=sys.stderr)
        return 1
    finally:
        conn.close()

    hours = stats["total_sec"] / 3600
    print(f"计划 {stats['plans']} 个（其中 1 个归档）")
    print(f"记录 {stats['sessions']} 条，合计 {hours:.1f} 小时")
    print(f"库：{args.db}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
