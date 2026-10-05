<script setup lang="ts">
import { computed } from "vue";
import GlassPanel from "../components/GlassPanel.vue";
import IconMedal from "../components/IconMedal.vue";
import StatTile from "../components/StatTile.vue";
import type { AchievementDTO } from "../../bindings/goxuetimer/models";
import { formatDurationCN } from "../composables/format";
import { useAchievements } from "../composables/useAchievements";

/**
 * 成就页。
 *
 * 徽章完全由历史记录推导（后端从 sessions 表算），所以这一页没有"刷新"按钮，
 * 也不需要本地缓存：记录一变（records:changed）就重拉一次。
 *
 * 徽章图走 `frontend/public/badges/<id>.png`；没有素材时回退到内置的矢量奖章，
 * 所以现在这一页在没有美术资源的情况下也是完整的。加新成就的做法见 README。
 */
const { summary, loading, error, refresh } = useAchievements();
void refresh();

const items = computed<AchievementDTO[]>(() => summary.value.items ?? []);

/** 进度百分比。后端已经把 progress 夹在 target 之内，这里只防一个个位数。 */
function percent(item: AchievementDTO): number {
	if (item.target <= 0) return 0;
	return Math.min(100, Math.round((item.progress / item.target) * 100));
}

function progressText(item: AchievementDTO): string {
	return `${item.progress} / ${item.target} ${item.unit}`;
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
	// 事件里的 img 已经没用了，直接藏掉，让下面的奖章露出来。
	(event.target as HTMLImageElement).style.display = "none";
}
function hasArt(id: string): boolean {
	return !brokenArt.has(id);
}
</script>

<template>
	<section class="achievements-view gx-col">
		<GlassPanel padding="16px 18px">
			<div class="head">
				<div class="head-text">
					<span class="head-title">
						已解锁 {{ summary.unlocked }} / {{ summary.total }}
					</span>
					<span class="head-sub gx-faint">
						累计专注 {{ formatDurationCN(summary.totalSec) }} ·
						{{ summary.sessions }} 次 · {{ summary.activeDays }} 天有记录 ·
						单次最长 {{ formatDurationCN(summary.longestSec) }}
					</span>
				</div>
				<div class="tiles">
					<StatTile label="当前连续" :value="`${summary.currentStreak} 天`" accent />
					<StatTile label="最长连续" :value="`${summary.longestStreak} 天`" />
				</div>
			</div>

			<p v-if="summary.lateNightCount > 0 || summary.earlyMorningCount > 0"
				class="extra gx-faint">
				<span v-if="summary.earlyMorningCount > 0">早起开始过 {{ summary.earlyMorningCount }} 次</span>
				<span v-if="summary.earlyMorningCount > 0 && summary.lateNightCount > 0"> · </span>
				<span v-if="summary.lateNightCount > 0">深夜开始过 {{ summary.lateNightCount }} 次</span>
			</p>
		</GlassPanel>

		<div v-if="error" class="error">{{ error }}</div>

		<div class="grid">
			<GlassPanel
				v-for="item in items"
				:key="item.id"
				class="badge"
				:class="[`tier-${item.tier}`, { 'is-locked': !item.unlocked }]"
				padding="14px 16px"
			>
				<div class="badge-head">
					<div class="medal">
						<img
							v-if="hasArt(item.id)"
							class="art"
							:src="`/badges/${item.id}.png`"
							:alt="item.name"
							@error="(e: Event) => useFallback(item.id, e)"
						/>
						<IconMedal v-if="!hasArt(item.id)" :size="30" :unlocked="item.unlocked" />
					</div>
					<div class="badge-text">
						<span class="badge-name">{{ item.name }}</span>
						<span class="badge-state gx-faint">
							{{ item.unlocked ? "已解锁" : progressText(item) }}
						</span>
					</div>
					<span v-if="item.unlocked" class="done" aria-label="已解锁">✓</span>
				</div>

				<p class="badge-desc gx-muted">{{ item.description }}</p>

				<div class="track" :title="progressText(item)">
					<div class="fill" :style="{ width: `${percent(item)}%` }"></div>
				</div>
			</GlassPanel>
		</div>

		<GlassPanel v-if="loading && items.length === 0" padding="16px 18px">
			<p class="gx-muted">正在汇总历史记录…</p>
		</GlassPanel>
	</section>
</template>

<style scoped>
.achievements-view {
	gap: var(--gx-gap);
}

.head {
	display: flex;
	align-items: center;
	gap: 14px;
	flex-wrap: wrap;
	margin-top: 4px;
}

.head-text {
	display: flex;
	flex-direction: column;
	gap: 3px;
	flex: 1 1 auto;
	min-width: 0;
}

.head-title {
	font-size: 16px;
	font-weight: 650;
}

.head-sub {
	font-size: 12px;
	line-height: 1.6;
}

.tiles {
	display: flex;
	gap: 10px;
	flex: 0 0 auto;
}

.extra {
	margin-top: 12px;
	font-size: 12px;
}

.grid {
	display: grid;
	/* 自适应列数：窄窗口一列、宽窗口两列，不需要写媒体查询 */
	grid-template-columns: repeat(auto-fill, minmax(232px, 1fr));
	gap: 12px;
}

.badge {
	display: flex;
	flex-direction: column;
	gap: 9px;
	/* 徽章靠稀有度上色，所以这三条是 token 而不是散落的色值 */
	--badge-1: var(--gx-accent);
	--badge-2: var(--gx-accent-2);
}

.tier-bronze {
	--badge-1: #d08a5a;
	--badge-2: #e6b184;
}

.tier-silver {
	--badge-1: #8fa3bb;
	--badge-2: #c2d0e0;
}

.tier-gold {
	--badge-1: #d9a53c;
	--badge-2: #f0cd7a;
}

.badge-head {
	display: flex;
	align-items: center;
	gap: 10px;
	min-width: 0;
}

.medal {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 44px;
	height: 44px;
	flex: 0 0 auto;
	border-radius: 14px;
	background: linear-gradient(135deg, var(--badge-1), var(--badge-2));
	color: #fff;
	overflow: hidden;
}

/* 未解锁的徽章降饱和、去掉阴影：还能看清是什么，但不抢已解锁的注意力 */
.is-locked .medal {
	filter: grayscale(1);
	opacity: 0.4;
}

.art {
	display: block;
	width: 100%;
	height: 100%;
	object-fit: cover;
}

.badge-text {
	display: flex;
	flex-direction: column;
	gap: 1px;
	min-width: 0;
	flex: 1 1 auto;
}

.badge-name {
	font-size: 14.5px;
	font-weight: 650;
}

.badge-state {
	font-size: 11.5px;
}

.is-locked .badge-name {
	color: var(--gx-text-2);
}

.done {
	flex: 0 0 auto;
	color: var(--gx-ok);
	font-size: 15px;
	font-weight: 700;
}

.badge-desc {
	font-size: 12.5px;
	line-height: 1.65;
	/* 说明长度不一，给个下限让卡片高度接近，网格看起来才齐 */
	min-height: 42px;
}

.track {
	height: 6px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-track);
	overflow: hidden;
}

.fill {
	height: 100%;
	border-radius: var(--gx-r-pill);
	background: linear-gradient(90deg, var(--badge-1), var(--badge-2));
	transition: width var(--gx-dur) var(--gx-ease);
}

.is-locked .fill {
	opacity: 0.45;
}

.error {
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
