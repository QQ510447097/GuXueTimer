/**
 * 用 CDP 驱动正在运行的应用：切视图、截图、断言布局。
 *
 * 为什么需要它：界面"好不好看"读代码判断不出来，只能看图。而 CDP 的
 * Page.captureScreenshot 是渲染进程直接出图——窗口不必可见，也不会在你屏幕上
 * 反复弹窗。顺带能拿到计算样式与几何信息，"有没有横向滚动条""毛玻璃真的生效了吗"
 * 这类问题就从"我觉得"变成可断言的。
 *
 * 零依赖：Node 18+ 自带全局 fetch，Node 22+ 自带全局 WebSocket。
 *
 * 用法（通常由 tools/shoot.ps1 调用）：
 *   node tools/ui_probe.mjs --port 9222 --out screenshots [--views timer,records]
 */

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const DEFAULT_VIEWS = ["timer", "records", "plans", "achievements", "personalize", "settings"];

function parseArgs(argv) {
	const out = { port: 9222, out: "screenshots", views: DEFAULT_VIEWS, width: 0, height: 0, prefix: "", dark: false, dialog: false };
	for (let i = 0; i < argv.length; i++) {
		const a = argv[i];
		if (a === "--port") out.port = Number(argv[++i]);
		else if (a === "--out") out.out = argv[++i];
		else if (a === "--views") out.views = String(argv[++i]).split(",").map((s) => s.trim()).filter(Boolean);
		else if (a === "--width") out.width = Number(argv[++i]);
		else if (a === "--height") out.height = Number(argv[++i]);
		else if (a === "--prefix") out.prefix = argv[++i];
		else if (a === "--dark") out.dark = true;
		else if (a === "--act") out.act = true;
	}
	return out;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/**
 * 等前端渲染出导航栏（或引导页）。
 *
 * 调试端口比 Vue 挂载更早可用，所以要轮询到界面真的出现为止。
 * 找不到就返回 false，由调用方报错——**不要**静默继续：抢跑会让每个视图都点空。
 */
async function waitForFrontend(cdp, timeoutMs = 20000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const ok = await cdp.eval(`!!document.querySelector('[data-view], .onboarding')`);
		if (ok) return true;
		await sleep(200);
	}
	return false;
}

/** 轮询 /json/list，直到拿到页面 target 的调试地址。 */
async function waitForTarget(port, timeoutMs = 30000) {
	const deadline = Date.now() + timeoutMs;
	let lastErr = "";
	while (Date.now() < deadline) {
		try {
			const res = await fetch(`http://127.0.0.1:${port}/json/list`);
			const list = await res.json();
			const page = list.find((t) => t.type === "page" && t.webSocketDebuggerUrl);
			if (page) return page;
			lastErr = `列表里没有 page target（${list.length} 项）`;
		} catch (err) {
			lastErr = err instanceof Error ? err.message : String(err);
		}
		await sleep(400);
	}
	throw new Error(`连不上调试端口 ${port}：${lastErr}`);
}

/** 极简 CDP 客户端：够用就好，不引依赖。 */
class Cdp {
	constructor(ws) {
		this.ws = ws;
		this.id = 0;
		this.pending = new Map();
		ws.addEventListener("message", (ev) => {
			const msg = JSON.parse(ev.data);
			if (msg.id && this.pending.has(msg.id)) {
				const { resolve, reject } = this.pending.get(msg.id);
				this.pending.delete(msg.id);
				if (msg.error) reject(new Error(`${msg.error.message} (${JSON.stringify(msg.error.data ?? "")})`));
				else resolve(msg.result);
			}
		});
	}

	send(method, params = {}) {
		const id = ++this.id;
		return new Promise((resolve, reject) => {
			this.pending.set(id, { resolve, reject });
			this.ws.send(JSON.stringify({ id, method, params }));
			setTimeout(() => {
				if (this.pending.has(id)) {
					this.pending.delete(id);
					reject(new Error(`${method} 超时`));
				}
			}, 15000);
		});
	}

	async eval(expression) {
		const res = await this.send("Runtime.evaluate", {
			expression,
			returnByValue: true,
			awaitPromise: true,
		});
		if (res.exceptionDetails) {
			throw new Error(`页面里执行失败: ${res.exceptionDetails.text ?? "unknown"}`);
		}
		return res.result.value;
	}

	async shoot(path) {
		const res = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false });
		writeFileSync(path, Buffer.from(res.data, "base64"));
		return path;
	}
}

/** 在页面里跑的体检：横向溢出、页面内滚动、毛玻璃、关键元素。 */
const PROBE = `(() => {
	const de = document.documentElement;
	const panel = document.querySelector('.gx-glass');
	const active = document.querySelector('[data-view][data-active="true"]');
	// 页面内容区的滚动量。文档级的 overflowY 永远是 false——滚动发生在
	// .content 这个嵌套滚动容器里，所以"这一屏要不要滚"只能这样测。
	const content = document.querySelector('main.content');
	// 压缩器会把 backdrop-filter 与 -webkit-backdrop-filter 合并成一个（实测只留了
	// 前缀版），所以两个属性都要读，否则每次体检都会误报成 "none"。
	let backdrop = '(没有找到 .gx-glass)';
	if (panel) {
		const cs = getComputedStyle(panel);
		backdrop = cs.backdropFilter && cs.backdropFilter !== 'none'
			? cs.backdropFilter
			: (cs.webkitBackdropFilter || 'none');
	}
	return {
		activeView: active ? active.dataset.view : '',
		innerWidth: window.innerWidth,
		innerHeight: window.innerHeight,
		scrollWidth: de.scrollWidth,
		scrollHeight: de.scrollHeight,
		overflowX: de.scrollWidth > window.innerWidth + 1,
		overflowY: de.scrollHeight > window.innerHeight + 1,
		contentScroll: content ? content.scrollHeight : -1,
		contentClient: content ? content.clientHeight : -1,
		contentOverflowY: content ? content.scrollHeight > content.clientHeight + 1 : false,
		barPoints: document.querySelectorAll('[data-points]').length
			? Number(document.querySelector('[data-points]').dataset.points)
			: -1,
		// 图表区间的标题（本周（…）/ 某年某月）。区间切换是否生效，靠它 + barPoints 一起判。
		periodLabel: document.querySelector('[data-period]')?.dataset.period ?? '',
		// 计时页最下方那块区域显示什么：none | image。
		// 个性化页的下拉与计时页的根节点都挂了这个属性，同一个选择器两边都能读。
		bottom: document.querySelector('[data-bottom]')?.dataset.bottom ?? '',
		// 那块面板的实际宽高。它没有固定比例（吸收剩余高度），这个数字用来看
		// "窗口一变它有没有跟着变"——单看截图只能"觉得差不多"。
		bottomBox: (() => {
			const el = document.querySelector('.card-panel');
			return el ? el.clientWidth + 'x' + el.clientHeight : '';
		})(),
		backdropFilter: backdrop,
		panelCount: document.querySelectorAll('.gx-glass').length,
		themeClass: document.querySelector('.theme-dark') ? 'dark' : 'light',
		bodyBg: getComputedStyle(document.body).backgroundColor,
		viewText: (document.querySelector('main')?.innerText ?? '').slice(0, 120),
	};
})()`;

/*
 * 截图前的脚本化交互（--act）。
 *
 * 用文字找按钮而不是类名：类名是样式细节，改一次样式这里就失效；
 * 而"归档""本月"这些字是产品文案，本来就该稳定。
 */
const PRE_CAPTURE = {
	// 点开第一个计划的「归档」确认弹窗（验证弹窗按钮的文字可见性）
	plans: `(() => {
		const btn = [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '归档');
		if (!btn) return false;
		btn.click();
		return true;
	})()`,
	/*
	 * 记录页：一次验三件事，并把状态还原。
	 *
	 *   1. 切「本月」→ 标题与柱子根数都要跟着变（这里曾经因为 watcher 被卸载而失效）
	 *   2. 切回「本周」→ 再点「←」翻到上一周：标题要变成"上周"，柱子仍是七根
	 *   3. 上一周时「→」应当可用，回到本周后应当置灰（未来没有记录）
	 *
	 * 结尾会点「→」回到本周：截图工具连的是真实数据目录，跑完把界面留在"上周"
	 * 会让下一次截图看起来莫名其妙。
	 */
	records: `(async () => {
		const wait = (ms) => new Promise((r) => setTimeout(r, ms));
		const label = () => document.querySelector('[data-period]')?.dataset.period ?? '';
		const bars = () => Number(document.querySelector('[data-points]')?.dataset.points ?? -1);
		const month = document.querySelector('[data-range="month"]');
		const week = document.querySelector('[data-range="week"]');
		const back = document.querySelector('[data-period-step="back"]');
		const fwd = document.querySelector('[data-period-step="forward"]');
		if (!month || !week || !back || !fwd) return 'no-controls';

		const atStart = label();
		month.click();
		await wait(500);
		const monthState = label() + ' / ' + bars() + ' 根';

		week.click();
		await wait(500);
		back.click();
		await wait(500);
		const prevState = label() + ' / ' + bars() + ' 根';
		const forwardUsable = fwd.disabled !== true;

		fwd.click();
		await wait(500);

		return \`起始 \${atStart}; 本月 \${monthState}; 上一周 \${prevState}（此时→可用 \${forwardUsable}）; 还原 \${label()}\`;
	})()`,
	// settings 的动作没了：那一页已经没有需要脚本化交互的控件
	// （休息提醒连同它的步进器一起删除了）。
	// 打开计划选择器，再点里面的「新建计划」，把内联表单截下来。
	// 分两步所以要 await：弹出层是异步渲染的，紧接着找按钮会找不到。
	timer: `(async () => {
		const trigger = document.querySelector('[title="切换当前计划"]');
		if (!trigger) return false;
		trigger.click();
		await new Promise((r) => setTimeout(r, 250));
		const item = [...document.querySelectorAll('button')].find((b) => b.textContent.includes('新建计划'));
		if (!item) return 'no-new-button';
		item.click();
		await new Promise((r) => setTimeout(r, 250));
		return true;
	})()`,
	/*
	 * 个性化页：一次验两件事，并返回可读结果。
	 *
	 *   1. 点「其他组件」（禁用占位）→ 底部区域的值**不能**变
	 *   2. 再选「图片」→ 值要变成 image
	 *
	 * 类名不去猜：选项用 [class*="select-option"] 模糊匹配（naive-ui 的选项类名
	 * 在各版本间变过），触发元素用组件上挂的 data-bottom-select 找。
	 * 找不到就如实返回 no-select / no-option，不静默通过。
	 *
	 * 收尾会把它改过的设置改回去：这一跑动的是**用户真实的设置**（截图工具连的
	 * 就是正式数据目录），跑一次就把人家的"底部区域"从留空改成图片，是那种
	 * "跑完才发现"的脏改动。
	 */
	personalize: `(async () => {
		const wait = (ms) => new Promise((r) => setTimeout(r, ms));
		const read = () => document.querySelector('[data-bottom]')?.dataset.bottom ?? '';
		const root = document.querySelector('[data-bottom-select]');
		if (!root) return 'no-select';
		const trigger = root.querySelector('[class*="selection"]') ?? root;
		const openAndFind = async (text) => {
			trigger.click();
			await wait(300);
			const opts = [...document.querySelectorAll('[class*="select-option"]')];
			if (opts.length === 0) return { opts, hit: null };
			return { opts, hit: opts.find((o) => o.textContent.includes(text)) ?? null };
		};

		const before = read();

		const first = await openAndFind('其他组件');
		if (!first.hit) return first.opts.length ? 'no-other-option' : 'no-option';
		const disabledFlag = first.hit.className.includes('disabled') ? 'disabled' : 'not-disabled';
		first.hit.click();
		await wait(300);
		const afterDisabled = read();

		const second = await openAndFind('图片');
		if (!second.hit) return 'no-image-option';
		second.hit.click();
		await wait(400);
		const afterImage = read();

		const back = await openAndFind(before === 'image' ? '图片' : '留空');
		if (back.hit) back.hit.click();
		await wait(300);
		const restored = read();

		return \`\${disabledFlag}: \${before}->\${afterDisabled}; 图片: \${afterImage}; 还原: \${restored}\`;
	})()`,
};

async function main() {
	const args = parseArgs(process.argv.slice(2));
	const target = await waitForTarget(args.port);
	const ws = new WebSocket(target.webSocketDebuggerUrl);
	await new Promise((resolve, reject) => {
		ws.addEventListener("open", resolve, { once: true });
		ws.addEventListener("error", (e) => reject(new Error(`WebSocket 连接失败: ${e.message ?? ""}`)), { once: true });
	});

	const cdp = new Cdp(ws);
	await cdp.send("Page.enable");
	await cdp.send("Runtime.enable");

	/*
	 * 等前端真的渲染出来再开始。
	 *
	 * 调试端口是 WebView2 一起来就能连的，比 Vue 挂载更早——抢在前面点击会点空
	 * （导航项还不存在），而探针只会如实记一笔 clicked=false。看起来却很像
	 * "点到了却切错页"（报告里的 activeView 是应用恢复出来的上一屏），
	 * 排查时会往应用代码上找，白费半天。实测踩过一次。
	 *
	 * 等的是"导航栏或引导页出现了"：没有计划时外壳会换成引导页，那种情况也该放行。
	 */
	const ready = await waitForFrontend(cdp);
	if (!ready) {
		throw new Error("等不到前端渲染（既没有导航栏也没有引导页），页面可能白屏");
	}

	if (args.width > 0 && args.height > 0) {
		// 用视口模拟代替"真的改窗口大小"：对 CSS 布局而言等价，而且完全确定。
		await cdp.send("Emulation.setDeviceMetricsOverride", {
			width: args.width,
			height: args.height,
			deviceScaleFactor: 1,
			mobile: false,
		});
		await sleep(300);
	}

	mkdirSync(args.out, { recursive: true });

	// 深色只是往 <html> 上挂一个类（主题变量都在 :root 上），
	// 这样截图不必去改数据库里的主题设置，也不会影响下一次启动。
	if (args.dark) {
		await cdp.eval(`document.documentElement.classList.add('theme-dark'), true`);
		await sleep(200);
	}

	const report = { target: target.url, size: [args.width, args.height], dark: args.dark, views: [] };

	for (const view of args.views) {
		// 点导航项切视图；点不到就记下来，不要静默跳过。
		const clicked = await cdp.eval(`(() => {
			const el = document.querySelector('[data-view="${view}"]');
			if (!el) return false;
			el.click();
			return true;
		})()`);

		await sleep(500);

		// 需要弹窗的场合（--dialog）：先做一次脚本化交互再截图。
		let acted = null;
		if (args.act && PRE_CAPTURE[view]) {
			acted = await cdp.eval(PRE_CAPTURE[view]);
			await sleep(450);
		}

		const probe = await cdp.eval(PROBE);
		// 名字里带上限定条件，方便同一个视图的多种尺寸/主题摆在一起对比。
		// 尺寸只出现一次：--prefix 可能本身就是尺寸（shoot.ps1 就是这么传的）。
		const tags = [];
		if (args.prefix && args.prefix !== `${args.width}x${args.height}`) tags.push(args.prefix);
		if (args.width > 0) tags.push(`${args.width}x${args.height}`);
		const name = ["view", ...tags, view].join("-") + ".png";
		const file = join(args.out, name);
		await cdp.shoot(file);

		report.views.push({ requested: view, clicked, acted, ...probe, file });
	}

	ws.close();
	process.stdout.write(JSON.stringify(report, null, 2) + "\n");
}

main().catch((err) => {
	process.stderr.write(`ui_probe 失败: ${err.message}\n`);
	process.exit(1);
});
