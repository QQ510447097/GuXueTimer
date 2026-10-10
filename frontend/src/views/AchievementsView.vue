<script setup lang="ts">
import { computed } from "vue";
import GlassPanel from "../components/GlassPanel.vue";
import IconMedal from "../components/IconMedal.vue";
import wallBg from "../assets/achievement-wall.jpg";
import type { AchievementDTO } from "../../bindings/goxuetimer/models";
import { levelFromSec } from "../composables/level";
import { useAchievements } from "../composables/useAchievements";

/**
 * 成就页：上面一条等级，下面一面勋章墙。
 *
 * ## 现在是一面空墙
 *
 * 最早那三条成就（第一步 / 十小时 / 三日不辍）是占位，已经按设计清掉，
 * 正式那套还没定。所以 `items` 是空的，页面只在墙里画出槽位——**这是有意的状态**。
 * 往 internal/achievements 的 All 里加定义，徽章就会自动填进这些槽。
 *
 * ## 面积怎么分的
 *
 * 可用区约 430×725（默认窗口）：等级条固定 80 上下（约一成），墙吃掉剩下的（约九成）。
 * 墙**自己有内部滚动**，不让整页被撑出滚动条——界面体检里有一条"六屏一屏放得下"，
 * 整页滚动会让它挂；而且成就以后加到几十条也不用重排。
 *
 * ## 背景图
 *
 * 素材是 2:3（`frontend/src/assets/achievement-wall.jpg`，从 assets/ 那张 1024×1536
 * 缩到 860×1290 再压成 JPEG：源图 3.2 MB，会被 //go:embed 进 exe，直接塞进去白送 3 MB）。
 * 图本身细节很密、中上部还有一块很亮的霜花，所以文字和槽位必须靠一层遮罩压住才读得清：
 * 那层遮罩在 .wall-scrim，嫌重/嫌轻都调它。
 */
const { summary, loading, error, refresh } = useAchievements();
void refresh();

const items = computed<AchievementDTO[]>(() => summary.value.items ?? []);

/** 等级：临时曲线，见 composables/level.ts。用累计专注时长换算。 */
const level = computed(() => levelFromSec(summary.value.totalSec));

/*
 * 槽位数：与"4 列 × 6 行"对应，固定 24 个。
 *
 * 固定槽位（而不是只画已解锁的）是有意的：墙的疏密才稳定，看起来像"收藏格子"；
 * 定义超过 24 条时槽位跟着长，多出来的交给内部滚动。
 */
const SLOT_COUNT = 24;
interface Slot {
	key: string;
	item: AchievementDTO | null;
}
const slots = computed<Slot[]>(() => {
	const total = Math.max(SLOT_COUNT, items.value.length);
	return Array.from({ length: total }, (_, i) => ({
		key: items.value[i]?.id ?? `empty-${i}`,
		item: items.value[i] ?? null,
	}));
});

const unlockedCount = computed(() => items.value.filter((i) => i.unlocked).length);

/** 进度百分比。后端已经把 progress 夹在 target 之内，这里只防一个个位数。 */
function percent(item: AchievementDTO): number {
	if (item.target <= 0) return 0;
	return Math.min(100, Math.round((item.progress / item.target) * 100));
}

function progressText(item: AchievementDTO): string {
	return `${item.progress} / ${item.target} ${item.unit}`;
}

/** 槽位的悬停说明：已解锁说进度，没解锁说还差多少。 */
function slotTitle(item: AchievementDTO): string {
	const state = item.unlocked ? "已解锁" : progressText(item);
	return `${item.name} · ${state}\n${item.description}`;
}

/*
 * 素材加载失败时退回内置奖章。
 *
 * 用 Set 记住失败的 ID 而不是给每个 item 加字段：DTO 是后端给的结构，
 * 往里塞前端的显示状态迟早会有人误把它当成数据的一部分。
 */
const brokenArt = new Set<string>();
function useFallback(id: string, event: Event): void {
	brokenArt.add(id);
	(event.target as HTMLImageElement).style.display = "none";
}
function hasArt(id: string): boolean {
	return !brokenArt.has(id);
}
</script>

<template>
	<section class="achievements-view gx-col">
		<!-- 等级条：占一成左右，固定不滚动 -->
		<GlassPanel class="level-bar" padding="14px 16px">
			<div class="level-badge" :title="`Lv.${level.n} ${level.title}`">
				<span class="level-cap">LV</span>
				<span class="level-num">{{ level.n }}</span>
			</div>

			<div class="level-main">
				<div class="level-line">
					<span class="level-title">{{ level.title }}</span>
					<span class="gx-spacer"></span>
					<span class="level-xp gx-tabular gx-faint">
						{{ level.into }} / {{ level.need }} XP
					</span>
				</div>
				<div class="track">
					<div class="fill" :style="{ width: `${level.percent}%` }"></div>
				</div>
			</div>
		</GlassPanel>

		<!-- 勋章墙：背景图 + 24 个槽位，自己滚 -->
		<div class="wall gx-glass" :style="{ backgroundImage: `url(${wallBg})` }">
			<div class="wall-scrim" aria-hidden="true"></div>

			<div class="wall-head">
				<span class="wall-title">勋章墙</span>
				<span class="gx-spacer"></span>
				<span class="wall-count gx-tabular">{{ unlockedCount }} / {{ slots.length }}</span>
			</div>

			<div class="wall-grid gx-scroll">
				<div
					v-for="slot in slots"
					:key="slot.key"
					class="slot"
					:class="{
						'is-empty': !slot.item,
						'is-locked': slot.item && !slot.item.unlocked,
					}"
					:title="slot.item ? slotTitle(slot.item) : undefined"
				>
					<template v-if="slot.item">
						<div class="slot-medal">
							<img
								v-if="hasArt(slot.item.id)"
								class="art"
								:src="`/badges/${slot.item.id}.png`"
								:alt="slot.item.name"
								@error="(e: Event) => useFallback(slot.item!.id, e)"
							/>
							<IconMedal v-else :size="34" :unlocked="slot.item.unlocked" />
						</div>
						<span class="slot-name">{{ slot.item.name }}</span>
						<div class="slot-track" :title="progressText(slot.item)">
							<div class="slot-fill" :style="{ width: `${percent(slot.item)}%` }"></div>
						</div>
					</template>

					<!-- 空槽：画得很淡，只交代"这面墙有多少格" -->
					<template v-else>
						<div class="slot-medal is-hollow">
							<IconMedal :size="34" :unlocked="false" />
						</div>
						<span class="slot-name gx-faint">待定</span>
					</template>
				</div>
			</div>

			<p v-if="items.length === 0" class="wall-hint gx-faint">
				成就还没定义，这面墙先空着
			</p>
		</div>

		<div v-if="loading" class="note gx-faint">正在汇总历史记录…</div>
		<div v-if="error" class="error">{{ error }}</div>
	</section>
</template>

<style scoped>
.achievements-view {
	gap: var(--gx-gap);
	/*
	 * 撑满内容区：墙靠这一条才有"剩余高度"可分配。
	 * 探针测不出这种留白（scrollHeight 不会小于 clientHeight），所以"填满没有"看截图。
	 */
	min-height: 100%;
}

/* ---------------------------------------------------------------- 等级条 */

.level-bar {
	display: flex;
	align-items: center;
	gap: 12px;
	flex: 0 0 auto;
}

.level-badge {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	width: 48px;
	height: 48px;
	flex: 0 0 auto;
	border-radius: 15px;
	background: var(--gx-accent-grad);
	color: #fff;
	box-shadow: 0 8px 18px -8px rgba(59, 130, 246, 0.65);
}

.level-cap {
	font-size: 8.5px;
	letter-spacing: 0.14em;
	opacity: 0.85;
	line-height: 1;
}

.level-num {
	font-size: 20px;
	font-weight: 700;
	line-height: 1.15;
}

.level-main {
	display: flex;
	flex-direction: column;
	gap: 7px;
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
	height: 8px;
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

/* ---------------------------------------------------------------- 勋章墙 */

.wall {
	position: relative;
	display: flex;
	flex-direction: column;
	flex: 1 1 auto;
	/* 至少留出两行槽位的高度；再矮就让整页滚（最小窗口下本来就要滚） */
	min-height: 200px;
	padding: 0;
	overflow: hidden;
	background-size: cover;
	background-position: center 30%;
	background-repeat: no-repeat;
}

/*
 * 遮罩：这张背景图细节很密，中上部还有一块很亮的霜花，不压一层的话
 * 文字和勋章都读不清。渐变是"上深下浅"——顶部要放墙的标题与计数，压得最重。
 */
.wall-scrim {
	position: absolute;
	inset: 0;
	background: linear-gradient(
		180deg,
		rgba(6, 16, 32, 0.66) 0%,
		rgba(6, 16, 32, 0.42) 22%,
		rgba(6, 16, 32, 0.34) 60%,
		rgba(6, 16, 32, 0.5) 100%
	);
}

.wall-head {
	position: relative;
	display: flex;
	align-items: center;
	gap: 8px;
	padding: 12px 14px 8px;
	color: #fff;
}

.wall-title {
	font-size: 13.5px;
	font-weight: 650;
	letter-spacing: 0.02em;
}

.wall-count {
	font-size: 12px;
	opacity: 0.8;
}

/* 槽位区自己有滚动条：成就加到几十条也不会把整页撑开 */
.wall-grid {
	position: relative;
	display: grid;
	grid-template-columns: repeat(4, minmax(0, 1fr));
	gap: 8px;
	padding: 4px 12px 14px;
	overflow-y: auto;
	min-height: 0;
}

.slot {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 5px;
	padding: 9px 6px 8px;
	border-radius: var(--gx-r-md);
	background: rgba(10, 22, 42, 0.34);
	border: 1px solid rgba(255, 255, 255, 0.1);
	color: #fff;
	min-width: 0;
}

.slot.is-empty {
	background: rgba(10, 22, 42, 0.18);
	border-style: dashed;
	border-color: rgba(255, 255, 255, 0.13);
}

.slot.is-locked .slot-medal {
	filter: grayscale(1);
	opacity: 0.45;
}

.slot-medal {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 44px;
	height: 44px;
	border-radius: 14px;
	overflow: hidden;
	background: rgba(255, 255, 255, 0.09);
}

.slot-medal.is-hollow {
	background: rgba(255, 255, 255, 0.05);
	opacity: 0.4;
}

.art {
	display: block;
	width: 100%;
	height: 100%;
	object-fit: cover;
}

.slot-name {
	font-size: 11px;
	max-width: 100%;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.slot-track {
	width: 100%;
	height: 3px;
	border-radius: var(--gx-r-pill);
	background: rgba(255, 255, 255, 0.16);
	overflow: hidden;
}

.slot-fill {
	height: 100%;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-grad);
}

/* 空墙时的提示：贴在底部，不挡住槽位 */
.wall-hint {
	position: absolute;
	left: 0;
	right: 0;
	bottom: 7px;
	text-align: center;
	font-size: 11.5px;
	color: rgba(255, 255, 255, 0.72);
}

.note,
.error {
	font-size: 12.5px;
}

.error {
	color: var(--gx-danger);
}
</style>
