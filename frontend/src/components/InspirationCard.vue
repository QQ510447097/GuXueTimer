<script setup lang="ts">
import type { Quote } from "../data/quotes";

/**
 * 装饰卡片。
 *
 * 计时页下方本来就有一块空白（时钟只占上半屏），放一张可定制的卡片比留白好看，
 * 也给了这个工具一点"人味"。它有两种形态：
 *
 *   - quote：一句带出处的话。默认每天自动换一条（"每日一句"），
 *     用户也可以点「换一句」固定下来，或写自己的句子。
 *   - image：用户自己选的一张图。没选时给的是"去选一张"的入口，而不是空白框——
 *     空框会让人以为是加载失败。
 *
 * 两种形态都用同一个组件，是为了让个性化页能把"用户即将看到的"原样预览出来。
 *
 * 注意 props 的类型写在 defineProps 里而不是导出成接口：<script setup> 里不允许
 * export（连类型也不行），要共享类型得另开一个 .ts 文件。这里只有本组件用，就地写。
 */
const props = withDefaults(
	defineProps<{
		mode: "quote" | "image";
		quote: Quote;
		/** 图片模式下可直接用的地址；空串表示还没选图。 */
		imageUrl: string;
		/** 是否显示「换一句」。个性化页的预览不需要这个按钮（那里有独立控件）。 */
		showShuffle?: boolean;
		/** 未选图时的提示语。 */
		emptyHint?: string;
	}>(),
	{
		showShuffle: false,
		emptyHint: "挑一张喜欢的图放在这里",
	},
);

const emit = defineEmits<{
	(e: "shuffle"): void;
	(e: "pick"): void;
}>();
</script>

<template>
	<div class="inspiration" :class="`is-${props.mode}`">
		<!-- 语录 -->
		<template v-if="props.mode === 'quote'">
			<div class="quote">
				<span class="mark" aria-hidden="true">“</span>
				<p class="text">{{ props.quote.text }}</p>
				<!-- 自定义文字没有出处，这时候只显示正文 -->
				<span v-if="props.quote.source" class="source">—— {{ props.quote.source }}</span>
			</div>
			<button
				v-if="props.showShuffle"
				type="button"
				class="action gx-ghost gx-pill gx-lift"
				@click="emit('shuffle')"
			>
				换一句
			</button>
		</template>

		<!-- 图片 -->
		<template v-else>
			<img v-if="props.imageUrl" class="photo" :src="props.imageUrl" alt="背景图" />
			<div v-else class="placeholder">
				<span class="hint gx-muted">{{ props.emptyHint }}</span>
				<button type="button" class="action gx-ghost gx-pill gx-lift" @click="emit('pick')">
					选一张图
				</button>
			</div>
		</template>
	</div>
</template>

<style scoped>
.inspiration {
	position: relative;
	display: flex;
	align-items: center;
	gap: 14px;
	min-height: 96px;
	padding: 16px 18px;
	border-radius: var(--gx-r-md);
	background: var(--gx-glass-2-bg);
	border: 1px solid var(--gx-glass-2-border);
	overflow: hidden;
}

.is-quote {
	/* 卡片会被撑高（计时页让它吸收剩余空间），语录要在里面垂直居中，
	   否则会贴在顶部、下面留一块空——那正是要解决的问题本身。 */
	align-items: center;
}

/* 引号做成大字号的装饰，压在文字左上角外面一点，不占布局 */
.mark {
	position: absolute;
	top: 2px;
	left: 8px;
	font-size: 44px;
	line-height: 1;
	color: var(--gx-accent);
	opacity: 0.22;
	pointer-events: none;
}

.quote {
	display: flex;
	flex-direction: column;
	gap: 6px;
	flex: 1 1 auto;
	min-width: 0;
	padding-left: 22px;
}

.text {
	font-size: 15.5px;
	line-height: 1.7;
	color: var(--gx-text);
	letter-spacing: 0.02em;
}

.source {
	font-size: 12px;
	color: var(--gx-text-3);
}

.action {
	flex: 0 0 auto;
	padding: 5px 12px;
	font-size: 12px;
	font-family: inherit;
	cursor: pointer;
}

/* 图片模式：卡片本身当封面，图铺满。
   高度交给 flex 的 stretch（.is-image 覆盖了基类的 align-items: center），
   不再写死 168px——卡片在计时页会被撑高，写死的话图下方就空一块。 */
.is-image {
	padding: 0;
	min-height: 168px;
	align-items: stretch;
}

.photo {
	display: block;
	width: 100%;
	min-height: 168px;
	object-fit: cover;
}

.placeholder {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	gap: 10px;
	width: 100%;
	min-height: 168px;
	border-radius: var(--gx-r-md);
	/* 虚线框表达"这里还能放点东西"，实线会读成一个已经画好的面板 */
	border: 1px dashed var(--gx-line);
}

.hint {
	font-size: 12.5px;
}
</style>
