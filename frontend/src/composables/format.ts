/**
 * 纯格式化与错误文案工具。
 *
 * 刻意放在独立模块里、不依赖任何响应式状态：useTimer / useRecords / useSettings
 * 都要用 errText，如果留在 useTimer 里就会形成 useTimer ↔ useRecords 的循环导入。
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
