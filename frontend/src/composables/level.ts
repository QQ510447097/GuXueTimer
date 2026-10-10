/**
 * 等级换算：**一版临时的规则**，等正式的等级系统定下来就换掉。
 *
 * 现在这版刻意做得简单可解释：1 分钟专注 = 1 XP，升到下一级要
 * `60 × 当前等级` XP（也就是 60 / 120 / 180 / 240……），累计门槛是
 * 60 / 180 / 360 / 600 / 900 分钟。做成"越往后越难"而不是固定步长，
 * 是因为固定步长在几百小时之后会一路平推、等级数失去意义。
 *
 * 等级称号同样是占位。真要定的时候要一起考虑的是：
 *   - XP 只看时长，还是也看连续天数/成就数（那样"等级"才不只是时长的别名）
 *   - 升级要不要有"上限感"（比如满级之后的显示）
 *   - 换规则时老用户会不会掉级（最好只增不减，和成就判据同一条原则）
 */
export interface Level {
	/** 等级数，从 1 起。 */
	n: number;
	/** 称号（占位）。 */
	title: string;
	/** 当前等级内已攒的 XP。 */
	into: number;
	/** 升到下一级需要的 XP。 */
	need: number;
	/** 进度百分比，0~100。 */
	percent: number;
}

/** 每一级需要的 XP = 这个基数 × 等级。 */
const STEP_XP = 60;

const TITLES = ["初雪", "微光", "静心", "深耕", "长夜", "破晓", "极夜", "薪火"];
const TITLE_FALLBACK = "行者";

/** 一分钟专注 = 1 XP。 */
export function levelFromSec(totalSec: number): Level {
	const xp = Math.max(0, Math.floor(totalSec / 60));

	let n = 1;
	let need = STEP_XP;
	let spent = 0;
	// 攒够了就升级，门槛随等级抬高
	while (xp >= spent + need) {
		spent += need;
		n += 1;
		need = STEP_XP * n;
	}

	const into = xp - spent;
	return {
		n,
		title: TITLES[n - 1] ?? TITLE_FALLBACK,
		into,
		need,
		percent: need > 0 ? Math.min(100, Math.round((into / need) * 100)) : 0,
	};
}
