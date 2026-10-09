/**
 * 纯格式化与错误文案工具。
 *
 * 刻意放在独立模块里、不依赖任何响应式状态：useTimer / useStats / useSettings
 * 都要用 errText，如果留在 useTimer 里就会形成 useTimer ↔ useStats 的循环导入。
 * 循环导入在 ES 模块下靠函数声明提升能跑，但会随打包顺序出问题。
 */

/** 把绑定抛出的异常转成可以直接展示的中文文案。 */
export function errText(err: unknown): string {
	return err instanceof Error ? err.message : String(err);
}

/**
 * 时钟文本：不足 1 小时用 MM:SS，之后用 H:MM:SS。
 * 等宽字体下两种长度都不会因为进位而抖动。
 */
export function formatClock(totalSec: number): string {
	const safe = Number.isFinite(totalSec) && totalSec > 0 ? Math.floor(totalSec) : 0;
	const h = Math.floor(safe / 3600);
	const m = Math.floor((safe % 3600) / 60);
	const s = safe % 60;
	const mm = String(m).padStart(2, "0");
	const ss = String(s).padStart(2, "0");
	return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

/** 面向用户的中文时长文案。0 秒刻意说成「0 分钟」，比「0 秒」更自然。 */
export function formatDurationCN(totalSec: number): string {
	const safe = Number.isFinite(totalSec) && totalSec > 0 ? Math.floor(totalSec) : 0;
	if (safe <= 0) return "0 分钟";
	if (safe < 60) return `${safe} 秒`;
	const h = Math.floor(safe / 3600);
	const m = Math.floor((safe % 3600) / 60);
	if (h > 0) {
		return m > 0 ? `${h} 小时 ${m} 分` : `${h} 小时`;
	}
	return `${m} 分钟`;
}

/** 倒计时文案：不足 1 分钟时说「若干秒」，比取整成 0 分更清楚。 */
export function formatCountdownCN(totalSec: number): string {
	const safe = Number.isFinite(totalSec) && totalSec > 0 ? Math.floor(totalSec) : 0;
	if (safe <= 0) return "现在";
	if (safe < 60) return `${safe} 秒`;
	const m = Math.floor(safe / 60);
	if (m < 60) return `${m} 分`;
	const h = Math.floor(m / 60);
	const rest = m % 60;
	return rest > 0 ? `${h} 小时 ${rest} 分` : `${h} 小时`;
}

/** RFC3339（带本地偏移）→ HH:MM:SS。解析不出来时原样返回，不假装成功。 */
export function formatClockTime(rfc3339: string): string {
	if (!rfc3339) return "--:--";
	const t = Date.parse(rfc3339);
	if (Number.isNaN(t)) return rfc3339;
	const d = new Date(t);
	const p = (n: number) => String(n).padStart(2, "0");
	return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/** 今日日期，用于标题栏。 */
export function formatTodayLabel(stamp: number): string {
	const d = new Date(stamp);
	const p = (n: number) => String(n).padStart(2, "0");
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** HH:MM:SS → HH:MM，给记录行的起止时刻用。解析失败时原样返回。 */
export function formatClockHM(rfc3339: string): string {
	const full = formatClockTime(rfc3339);
	return /^\d{2}:\d{2}:\d{2}$/.test(full) ? full.slice(0, 5) : full;
}

/*
 * 以下是视图层共用的纯日期工具。
 *
 * 放在这里而不是各自的组件里：热力图、柱状图、区间查询、记录查询必须用
 * **同一个** 日期键格式（YYYY-MM-DD，严格零填充），后端 resolveDate 也按这个
 * 布局严格解析（"2026-5-1" 会被拒绝）。四处各格式化一遍迟早会漂移。
 *
 * 全部按本地时区计算：后端用本地时区判定"今天"，前端不能改用 UTC，
 * 否则东八区的凌晨会把记录算到前一天。
 */

function pad2(n: number): string {
	return String(n).padStart(2, "0");
}

/** 本地日期键 YYYY-MM-DD。 */
export function dayKeyOf(stamp: number | Date): string {
	const d = stamp instanceof Date ? stamp : new Date(stamp);
	return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

/** 今天的日期键。 */
export function todayKey(): string {
	return dayKeyOf(new Date());
}

/** 日期键 → 本地零点时间戳；格式不对返回 NaN。 */
export function dayKeyStamp(key: string): number {
	const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
	if (!m) return NaN;
	return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])).getTime();
}

/** 日期键加减天数。交给 Date 处理跨月、跨年。 */
export function addDays(key: string, delta: number): string {
	const stamp = dayKeyStamp(key);
	if (Number.isNaN(stamp)) return key;
	const d = new Date(stamp);
	d.setDate(d.getDate() + delta);
	return dayKeyOf(d);
}

/**
 * 闭区间 [from, to] 的日期键序列。
 *
 * 带一个硬上限：非法输入时 dayKeyStamp 会返回 NaN，没有上限就是一个死循环。
 */
export function dayRange(from: string, to: string): string[] {
	const out: string[] = [];
	let cursor = from;
	for (let i = 0; i < 1000; i += 1) {
		if (Number.isNaN(dayKeyStamp(cursor)) || cursor > to) break;
		out.push(cursor);
		cursor = addDays(cursor, 1);
	}
	return out;
}

/** 日期键所属月份 YYYY-MM。 */
export function monthOf(key: string): string {
	return key.slice(0, 7);
}

/** 月份 YYYY-MM 加减。 */
export function shiftMonth(month: string, delta: number): string {
	const year = Number(month.slice(0, 4));
	const m = Number(month.slice(5, 7));
	if (!Number.isFinite(year) || !Number.isFinite(m)) return month;
	const d = new Date(year, m - 1 + delta, 1);
	return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}`;
}

/** 该月的天数（1..31）。借 Date 的"下月第 0 天"处理闰年。 */
export function daysInMonth(month: string): number {
	const year = Number(month.slice(0, 4));
	const m = Number(month.slice(5, 7));
	if (!Number.isFinite(year) || !Number.isFinite(m)) return 30;
	return new Date(year, m, 0).getDate();
}

/** 月份标题：2026 年 5 月。 */
export function monthLabel(month: string): string {
	const year = month.slice(0, 4);
	const m = Number(month.slice(5, 7));
	return `${year} 年 ${m} 月`;
}

const weekdayNames = ["日", "一", "二", "三", "四", "五", "六"];

/** 日期标题：5 月 1 日 周五。 */
export function dayLabel(key: string): string {
	const stamp = dayKeyStamp(key);
	if (Number.isNaN(stamp)) return key;
	const d = new Date(stamp);
	return `${d.getMonth() + 1} 月 ${d.getDate()} 日 周${weekdayNames[d.getDay()]}`;
}

/** 图表横轴用的短标签：5/1。 */
export function shortDayLabel(key: string): string {
	const m = Number(key.slice(5, 7));
	const d = Number(key.slice(8, 10));
	if (!Number.isFinite(m) || !Number.isFinite(d)) return key;
	return `${m}/${d}`;
}

/** 周一起算的星期下标：0 = 周一。 */
export function weekdayFromMonday(key: string): number {
	const stamp = dayKeyStamp(key);
	if (Number.isNaN(stamp)) return 0;
	return (new Date(stamp).getDay() + 6) % 7;
}

/** 该日期所在周的周一。日期键非法时原样返回。 */
export function weekStart(key: string): string {
	return addDays(key, -weekdayFromMonday(key));
}

// 休息提醒间隔的合法区间，与后端 internal/settings 的 Min/MaxBreakIntervalMin 一致。
export const MIN_BREAK_INTERVAL_MIN = 1;
export const MAX_BREAK_INTERVAL_MIN = 480;

/**
 * 提醒间隔的加减步进：落到**最近的整十**上。
 *
 *   43 → + → 50 → + → 60
 *   43 → − → 40 → − → 30
 *
 * 为什么不是固定的 ±10：间隔是个"大概多久提醒我一下"的数字，用户不会需要 43 这种精度。
 * 从 45 起步时，固定 ±10 会得到 35 / 55 这种同样不整的数，一直按下去永远落不到整十上；
 * 按"最近的整十"走，两下之内就落到 10 的倍数，越按越整齐。
 *
 * 下界是 1：减到 10 以下时不再按整十走（否则会直接跳到 0 或负数），
 * 停在 1 分钟。上界由后端把关，这里先夹住，免得界面显示一个存不进去的值。
 */
export function stepBy10(value: number, dir: 1 | -1): number {
	const v = Number.isFinite(value) ? Math.round(value) : MIN_BREAK_INTERVAL_MIN;
	if (dir > 0) {
		const next = Math.floor(v / 10) * 10 + 10;
		return Math.min(MAX_BREAK_INTERVAL_MIN, Math.max(MIN_BREAK_INTERVAL_MIN, next));
	}
	const next = Math.ceil(v / 10) * 10 - 10;
	return next < MIN_BREAK_INTERVAL_MIN ? MIN_BREAK_INTERVAL_MIN : next;
}
