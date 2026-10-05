/**
 * 计时页装饰卡片用的语录。
 *
 * 选的都是公有领域的古典句子与名句：一个是版权干净，另一个是这些话本身就适合
 * "坐下来做点事"这个场景。带出处，是因为出处让一句话更有分量——也能让人顺着去找。
 *
 * 加新句子的规则：
 *   - text 控制在 30 个字以内。卡片是装饰，句子太长会变成一段正文。
 *   - source 写体裁与篇名（《荀子·劝学》），不写"某位网友说"。
 *   - 不确定出处的宁可不写，也不要编一个——这张卡片每天都在用户眼前。
 */
export interface Quote {
	text: string;
	source: string;
}

export const quotes: Quote[] = [
	{ text: "不积跬步，无以至千里。", source: "《荀子·劝学》" },
	{ text: "锲而不舍，金石可镂。", source: "《荀子·劝学》" },
	{ text: "吾生也有涯，而知也无涯。", source: "《庄子·养生主》" },
	{ text: "学而不思则罔，思而不学则殆。", source: "《论语·为政》" },
	{ text: "温故而知新，可以为师矣。", source: "《论语·为政》" },
	{ text: "知之者不如好之者，好之者不如乐之者。", source: "《论语·雍也》" },
	{ text: "三人行，必有我师焉。", source: "《论语·述而》" },
	{ text: "工欲善其事，必先利其器。", source: "《论语·卫灵公》" },
	{ text: "逝者如斯夫，不舍昼夜。", source: "《论语·子罕》" },
	{ text: "博学之，审问之，慎思之，明辨之，笃行之。", source: "《礼记·中庸》" },
	{ text: "凡事预则立，不预则废。", source: "《礼记·中庸》" },
	{ text: "玉不琢，不成器；人不学，不知道。", source: "《礼记·学记》" },
	{ text: "学然后知不足，教然后知困。", source: "《礼记·学记》" },
	{ text: "独学而无友，则孤陋而寡闻。", source: "《礼记·学记》" },
	{ text: "天行健，君子以自强不息。", source: "《周易·乾》" },
	{ text: "地势坤，君子以厚德载物。", source: "《周易·坤》" },
	{ text: "千里之行，始于足下。", source: "《老子》" },
	{ text: "合抱之木，生于毫末。", source: "《老子》" },
	{ text: "知人者智，自知者明。", source: "《老子》" },
	{ text: "胜人者有力，自胜者强。", source: "《老子》" },
	{ text: "路漫漫其修远兮，吾将上下而求索。", source: "屈原《离骚》" },
	{ text: "尺有所短，寸有所长。", source: "《楚辞·卜居》" },
	{ text: "流水不腐，户枢不蠹。", source: "《吕氏春秋·尽数》" },
	{ text: "读书破万卷，下笔如有神。", source: "杜甫《奉赠韦左丞丈二十二韵》" },
	{ text: "会当凌绝顶，一览众山小。", source: "杜甫《望岳》" },
	{ text: "长风破浪会有时，直挂云帆济沧海。", source: "李白《行路难》" },
	{ text: "天生我材必有用。", source: "李白《将进酒》" },
	{ text: "业精于勤，荒于嬉。", source: "韩愈《进学解》" },
	{ text: "非学无以广才，非志无以成学。", source: "诸葛亮《诫子书》" },
	{ text: "静以修身，俭以养德。", source: "诸葛亮《诫子书》" },
	{ text: "少壮不努力，老大徒伤悲。", source: "汉乐府《长歌行》" },
	{ text: "盛年不重来，一日难再晨。", source: "陶渊明《杂诗》" },
	{ text: "读书百遍，而义自见。", source: "《三国志·魏书》" },
	{ text: "博观而约取，厚积而薄发。", source: "苏轼《稼说送张琥》" },
	{ text: "古之立大事者，不惟有超世之才，亦必有坚忍不拔之志。", source: "苏轼《晁错论》" },
	{ text: "纸上得来终觉浅，绝知此事要躬行。", source: "陆游《冬夜读书示子聿》" },
	{ text: "问渠那得清如许？为有源头活水来。", source: "朱熹《观书有感》" },
	{ text: "天下事有难易乎？为之，则难者亦易矣。", source: "彭端淑《为学》" },
	{ text: "苟有恒，何必三更眠五更起。", source: "胡居仁《居业录》" },
	{ text: "宝剑锋从磨砺出，梅花香自苦寒来。", source: "《警世贤文》" },
];

/** 按日期挑一条：同一天进来永远是同一句，换一天自己就换了。 */
export function quoteOfDay(dayKey: string): Quote {
	const stamp = Date.parse(`${dayKey}T00:00:00`);
	const seed = Number.isNaN(stamp) ? 0 : Math.floor(stamp / 86400000);
	return quotes[((seed % quotes.length) + quotes.length) % quotes.length];
}

/**
 * 按设置挑出该显示哪一句。
 *
 * index 的语义与后端一致：0 = 每日一句，大于 0 = 固定第 index 条（1 起算）。
 * 下标越界时回落到每日一句——用户可能在别处改过语录条数。
 */
export function pickQuote(dayKey: string, index: number): Quote {
	if (index <= 0 || index > quotes.length) return quoteOfDay(dayKey);
	return quotes[index - 1];
}
