<script setup lang="ts">
import { computed } from "vue";
import GlassPanel from "../components/GlassPanel.vue";
import brandMark from "../assets/brand-mark.png";
import hourlyXp from "../assets/hourly-xp.png";
import type { AchievementDTO } from "../../bindings/goxuetimer/models";
import { formatDurationCN } from "../composables/format";
import { useAchievements } from "../composables/useAchievements";

/**
 * 成就页：上面一条等级，下面三组成就。
 *
 * ## 等级与经验是"账"，不是推导结果
 *
 * 等级、累计经验、奖章数存在库里（见 internal/achievements/progress.go 的说明），
 * 只在**结束并保存**那一刻结算：同一小时在拿到奖章前后值多少经验不同，而规则
 * 以后还会调，所以已发放的值必须留下来。这一页只负责显示，不做任何换算。
 *
 * ## 三组
 *
 * - **单次**：够线就永远达成，只记达成时间，不给经验。
 * - **循环**：累计若干个"达标日"（当天满 1 小时）达成一次，可以反复达成；
 *   每次给经验并发一枚奖章，奖章会永久抬高每小时的收益。
 * - **每日**：周期为 1 天的循环；末尾那张「寸阴寸金」是机制说明卡，不是成就
 *   （它每小时都在结算，套不进"达成一次"的模型），所以它的图不走 /badges/，
 *   而是跟应用图标一样由 Vite 静态引入。
 *
 * ## 面积
 *
 * 可用区约 430×725：等级条固定（约 100），三组吃掉剩下的并**自己滚动**——
 * 整页不许被撑出滚动条（界面体检有一条"六屏一屏放得下"），而且以后加成就
 * 也不会把页面顶开。
 *
 * 成就图按 id 取 `frontend/public/badges/<id>.png`（丢文件进去就能换，不用改代码）；
 * 万一缺图或加载失败就退回应用图标，见 onArtError。
 */
const { summary, loading, error, refresh } = useAchievements();
void refresh();

const items = computed<AchievementDTO[]>(() => summary.value.items ?? []);
const level = computed(() => summary.value.level);

const onceItems = computed(() => items.value.filter((i) => i.kind === "once"));
const cycleItems = computed(() => items.value.filter((i) => i.kind === "cycle"));
const dailyItems = computed(() => items.value.filter((i) => i.kind === "daily"));

/** 经验一律以"十分之一经验"的整数传输，这里只负责显示成一位小数。 */
function fmtXp(tenths: number): string {
	return (tenths / 10).toFixed(1);
}

/** 进度百分比。分母为零时按 0 处理，避免 NaN 把进度条画没。 */
function percent(item: AchievementDTO): number {
	if (!item.target) return 0;
	return Math.max(0, Math.min(100, Math.round((item.progress / item.target) * 100)));
}

/** 成长卡：当前这一小时攒到哪了。 */
const carryPercent = computed(() => {
	const sec = level.value.carrySec ?? 0;
	return Math.max(0, Math.min(100, Math.round((sec / 3600) * 100)));
});

/** 等级条上的进度百分比。 */
const levelRatePercent = computed(() => {
	const need = level.value.needTenths || 0;
	if (!need) return 0;
	return Math.max(0, Math.min(100, Math.round((level.value.intoTenths / need) * 100)));
});

/**
 * 奖章四件套：固定展示四个位置（数量为 0 也占位，布局才不会随数据跳）。
 *
 * bonus 是每枚奖章让"每小时经验"多出来的部分，与 internal/achievements 的
 * 成长倍率（10 + W + 2M + 3Q + 12Y 个十分之一）一一对应：周 +0.1、月 +0.2、
 * 季 +0.3、年 +1.2。加成写在悬停提示里，页面上不铺一段解释。
 */
const MEDAL_ORDER = [
	{ kind: "bronze", name: "铜勋章", key: "week", bonus: "0.1" },
	{ kind: "silver", name: "银勋章", key: "month", bonus: "0.2" },
	{ kind: "gold", name: "金勋章", key: "quarter", bonus: "0.3" },
	{ kind: "ruby", name: "红宝石勋章", key: "year", bonus: "1.2" },
] as const;

const medalRow = computed(() => {
	const medals = level.value.medals;
	return MEDAL_ORDER.map((m) => ({
		kind: m.kind,
		name: m.name,
		bonus: m.bonus,
		count: (medals ? medals[m.key] : 0) ?? 0,
	}));
});

const MEDAL_NAMES: Record<string, string> = {
	bronze: "铜勋章",
	silver: "银勋章",
	gold: "金勋章",
	ruby: "红宝石勋章",
};

/** 日期只显示到天：达成时间的时分秒对"哪一天做到的"没有意义。 */
function dayOf(stamp: string): string {
	return stamp ? stamp.slice(0, 10) : "";
}

/** 每张卡的第二行：这一条现在是什么状态。 */
function stateText(item: AchievementDTO): string {
	switch (item.kind) {
		case "once":
			if (item.unlocked) {
				const at = dayOf(item.achievedAt);
				return at ? `达成于 ${at}` : "已达成";
			}
			return `还差 ${item.target - item.progress} ${item.unit}`;
		case "cycle":
			if (item.count > 0) {
				const at = dayOf(item.lastAt);
				return at ? `已达成 ${item.count} 次 · 上次 ${at}` : `已达成 ${item.count} 次`;
			}
			return `还差 ${item.target - item.progress} 天`;
		case "daily":
			if (item.todayDone) return `今天已完成 · 累计 ${item.count} 天`;
			return `今天还差 ${formatDurationCN(item.target - item.progress)}`;
		default:
			return "";
	}
}

/** 每张卡进度条下面的数字。 */
function progressText(item: AchievementDTO): string {
	switch (item.kind) {
		case "daily":
			return `${formatDurationCN(item.progress)} / ${formatDurationCN(item.target)}`;
		case "cycle":
			return `${item.progress} / ${item.target} 天`;
		default:
			return `${item.progress} / ${item.target} ${item.unit}`;
	}
}

/*
 * 成就图：先试 `/badges/<id>.png`，加载失败就换成应用图标。
 *
 * 失败时直接改这个 img 的 src（DOM 层面），并记进 Set 免得反复重试——
 * 不去依赖"Set 变了会重新渲染"，那种写法在 Vue 里不会触发更新。
 */
const brokenArt = new Set<string>();
function artFor(id: string): string {
	return brokenArt.has(id) ? brandMark : `/badges/${id}.png`;
}
function onArtError(id: string, event: Event): void {
	brokenArt.add(id);
	const img = event.target as HTMLImageElement;
	if (img.src !== brandMark) img.src = brandMark;
}

/** 奖章图加载失败就藏起来，别留一个破图标。 */
function onMedalError(event: Event): void {
	(event.target as HTMLImageElement).style.display = "none";
}
</script>

<template>
	<section class="achievements-view gx-col">
		<!-- 等级条：固定不滚动 -->
		<GlassPanel
			class="level-bar"
			padding="13px 15px"
			:data-level="level.level"
			:data-xp="level.xpTenths"
			:data-rate="level.rateTenths"
		>
			<div class="level-badge" :title="`等级 ${level.level}`">
				<span class="level-cap">LV</span>
				<span class="level-num">{{ level.level }}</span>
			</div>

			<div class="level-main">
				<div class="level-line">
					<span class="level-title">等级 {{ level.level }}</span>
					<span class="gx-spacer"></span>
					<span class="level-xp gx-tabular gx-faint">
						{{ fmtXp(level.intoTenths) }} / {{ fmtXp(level.needTenths) }} 经验
					</span>
				</div>

				<div class="track">
					<div class="fill" :style="{ width: `${levelRatePercent}%` }"></div>
				</div>

				<div class="level-foot">
					<span class="level-rate gx-faint">
						每小时 +{{ fmtXp(level.rateTenths) }} 经验
					</span>
					<span class="gx-spacer"></span>
					<span class="medal-row">
						<span
							v-for="medal in medalRow"
							:key="medal.kind"
							class="medal-chip"
							:class="{ 'is-zero': medal.count === 0 }"
							:title="`${medal.name} × ${medal.count} · 每枚让每小时多 ${medal.bonus} 经验`"
						>
							<img class="medal-img" :src="`/medals/${medal.kind}.png`" :alt="medal.name" @error="onMedalError" />
							<span class="gx-tabular">×{{ medal.count }}</span>
						</span>
					</span>
				</div>
			</div>
		</GlassPanel>

		<!-- 三组成就：自己滚动 -->
		<div class="groups gx-scroll">
			<section class="group">
				<h3 class="group-title">单次成就 <span class="gx-faint">{{ onceItems.length }}</span></h3>
				<div class="grid">
					<GlassPanel
						v-for="item in onceItems"
						:key="item.id"
						class="ach"
						:class="{ 'is-locked': !item.unlocked }"
						padding="9px 12px"
					>
						<div class="ach-head">
							<div class="ach-art">
								<img :src="artFor(item.id)" :alt="item.name" @error="(e: Event) => onArtError(item.id, e)" />
							</div>
							<div class="ach-text">
								<span class="ach-name">{{ item.name }}</span>
								<span class="ach-state gx-faint">{{ stateText(item) }}</span>
							</div>
						</div>
						<p class="ach-desc gx-muted" :title="item.description">{{ item.description }}</p>
						<div class="ach-foot">
							<div class="track">
								<div class="fill" :style="{ width: `${percent(item)}%` }"></div>
							</div>
							<span class="ach-progress gx-faint gx-tabular">{{ progressText(item) }}</span>
						</div>
					</GlassPanel>
				</div>
			</section>

			<section class="group">
				<h3 class="group-title">循环成就 <span class="gx-faint">{{ cycleItems.length }}</span></h3>
				<div class="grid">
					<GlassPanel
						v-for="item in cycleItems"
						:key="item.id"
						class="ach"
						:class="{ 'is-locked': item.count === 0 }"
						padding="9px 12px"
					>
						<div class="ach-head">
							<div class="ach-art">
								<img :src="artFor(item.id)" :alt="item.name" @error="(e: Event) => onArtError(item.id, e)" />
							</div>
							<div class="ach-text">
								<span class="ach-name">{{ item.name }}</span>
								<span class="ach-state gx-faint">{{ stateText(item) }}</span>
							</div>
							<img
								v-if="item.medal"
								class="ach-medal"
								:class="{ 'is-zero': item.count === 0 }"
								:src="`/medals/${item.medal}.png`"
								:alt="MEDAL_NAMES[item.medal]"
								:title="`${MEDAL_NAMES[item.medal]} · 已获得 ${item.count} 枚`"
								@error="onMedalError"
							/>
						</div>
						<p class="ach-desc gx-muted" :title="item.description">{{ item.description }}</p>
						<div class="ach-foot">
							<div class="track">
								<div class="fill" :style="{ width: `${percent(item)}%` }"></div>
							</div>
							<span class="ach-progress gx-faint gx-tabular">{{ progressText(item) }}</span>
						</div>
					</GlassPanel>
				</div>
			</section>

			<section class="group">
				<h3 class="group-title">每日 <span class="gx-faint">{{ dailyItems.length + 1 }}</span></h3>
				<div class="grid">
					<GlassPanel
						v-for="item in dailyItems"
						:key="item.id"
						class="ach"
						:class="{ 'is-locked': item.count === 0 }"
						padding="9px 12px"
					>
						<div class="ach-head">
							<div class="ach-art">
								<img :src="artFor(item.id)" :alt="item.name" @error="(e: Event) => onArtError(item.id, e)" />
							</div>
							<div class="ach-text">
								<span class="ach-name">{{ item.name }}</span>
								<span class="ach-state gx-faint">{{ stateText(item) }}</span>
							</div>
						</div>
						<p class="ach-desc gx-muted" :title="item.description">{{ item.description }}</p>
						<div class="ach-foot">
							<div class="track">
								<div class="fill" :style="{ width: `${percent(item)}%` }"></div>
							</div>
							<span class="ach-progress gx-faint gx-tabular">{{ progressText(item) }}</span>
						</div>
					</GlassPanel>

					<!-- 机制卡「寸阴寸金」：一小时的收益一句话说完，加成写在勋章的悬停里 -->
					<GlassPanel class="ach kind-growth" padding="9px 12px">
						<div class="ach-head">
							<div class="ach-art">
								<img :src="hourlyXp" alt="寸阴寸金" />
							</div>
							<div class="ach-text">
								<span class="ach-name">寸阴寸金</span>
								<span class="ach-state gx-faint">每小时 +{{ fmtXp(level.rateTenths) }} 经验</span>
							</div>
						</div>
						<p class="ach-desc gx-muted">每小时学习获得经验</p>
						<div class="ach-foot">
							<div class="track">
								<div class="fill" :style="{ width: `${carryPercent}%` }"></div>
							</div>
							<span class="ach-progress gx-faint gx-tabular">
								{{ formatDurationCN(level.carrySec) }} / 1 小时
							</span>
						</div>
					</GlassPanel>
				</div>
			</section>

			<!--
				提示放在内滚区里，不放在页面列里：放在外面会参与整页高度，
				首帧"正在汇总"那一瞬间就把页面顶出滚动条（界面体检会挂）。
			-->
			<div v-if="loading" class="note gx-faint">正在汇总历史记录…</div>
			<div v-if="error" class="error">{{ error }}</div>
		</div>
	</section>
</template>

<style scoped>
.achievements-view {
	gap: var(--gx-gap);
	/*
	 * **必须是 height 而不是 min-height**：外层 .content 是 flex 项、高度确定，
	 * 用 height:100% 才给这一列一个上界，里面的 .groups 才谈得上"剩余高度"并内滚。
	 * 写 min-height 的话这一列的高度仍由内容决定，内滚区永远撑开、整页跟着变高，
	 * 界面体检的"六屏一屏放得下"就会挂。
	 */
	height: 100%;
}

/* ---------------------------------------------------------------- 等级条 */

.level-bar {
	display: flex;
	align-items: center;
	gap: 10px;
	flex: 0 0 auto;
}

.level-badge {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	width: 50px;
	height: 50px;
	flex: 0 0 auto;
	border-radius: 15px;
	background: var(--gx-accent-grad);
	color: #fff;
	box-shadow: 0 8px 18px -9px rgba(59, 130, 246, 0.7);
}

.level-cap {
	font-size: 8.5px;
	letter-spacing: 0.14em;
	line-height: 1;
	opacity: 0.85;
}

.level-num {
	font-size: 21px;
	font-weight: 700;
	line-height: 1.15;
}

.level-main {
	display: flex;
	flex-direction: column;
	gap: 6px;
	flex: 1 1 auto;
	min-width: 0;
}

.level-line {
	display: flex;
	align-items: baseline;
	gap: 8px;
	min-width: 0;
}

.level-title {
	font-size: 14px;
	font-weight: 650;
}

.level-xp {
	font-size: 11.5px;
	white-space: nowrap;
}

.track {
	height: 7px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-track);
	overflow: hidden;
}

.fill {
	height: 100%;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-grad);
	transition: width var(--gx-dur) var(--gx-ease);
}

.level-foot {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.level-rate {
	font-size: 11px;
	white-space: nowrap;
}

.medal-row {
	display: flex;
	align-items: center;
	gap: 7px;
}

.medal-chip {
	display: flex;
	align-items: center;
	gap: 3px;
	font-size: 10.5px;
	color: var(--gx-text-2);
}

.medal-chip.is-zero {
	opacity: 0.45;
}

.medal-img {
	width: 18px;
	height: 18px;
	display: block;
}

/* ---------------------------------------------------------------- 三组 */

.groups {
	display: flex;
	flex-direction: column;
	gap: 10px;
	flex: 1 1 auto;
	min-height: 0;
	overflow-y: auto;
}

.group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.group-title {
	margin: 0;
	font-size: 12px;
	font-weight: 650;
	color: var(--gx-text-2);
	letter-spacing: 0.03em;
}

.grid {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: 7px;
}

.ach {
	display: flex;
	flex-direction: column;
	gap: 5px;
	min-width: 0;
}

.ach-head {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.ach-art {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 30px;
	height: 30px;
	flex: 0 0 auto;
	border-radius: 10px;
	overflow: hidden;
	background: var(--gx-accent-soft);
}

.ach-art img {
	display: block;
	width: 100%;
	height: 100%;
	object-fit: contain;
}

/* 没解锁的整卡降饱和，但文字保持可读 */
.is-locked .ach-art {
	filter: grayscale(1);
	opacity: 0.5;
}

.ach-text {
	display: flex;
	flex-direction: column;
	gap: 1px;
	min-width: 0;
	flex: 1 1 auto;
}

.ach-name {
	font-size: 12.5px;
	font-weight: 650;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.ach-state {
	font-size: 10.5px;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.ach-medal {
	width: 26px;
	height: 26px;
	flex: 0 0 auto;
}

.ach-medal.is-zero {
	filter: grayscale(1);
	opacity: 0.3;
}

.ach-desc {
	margin: 0;
	font-size: 11px;
	line-height: 1.5;
	/* 说明长短不一，截一行让每张卡等高，网格才齐 */
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.ach-foot {
	display: flex;
	align-items: center;
	gap: 7px;
	min-width: 0;
}

.ach-foot .track {
	flex: 1 1 auto;
	height: 5px;
}

.ach-progress {
	font-size: 10px;
	white-space: nowrap;
	flex: 0 0 auto;
}

.is-locked .fill {
	opacity: 0.45;
}

/* 成长卡不是成就，用一条虚线边跟旁边的成就区分开 */
.kind-growth {
	border-style: dashed;
}

.note,
.error {
	font-size: 12.5px;
}

.error {
	color: var(--gx-danger);
}
</style>
