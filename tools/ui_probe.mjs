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

const DEFAULT_VIEWS = ["timer", "records", "plans", "personalize", "settings"];

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
		// 提醒间隔步进器的当前值（-1 表示这一屏没有它）。用来断言"43 -> 50 -> 60"。
		stepper: Number(document.querySelector('[data-stepper-value]')?.dataset.stepperValue ?? -1),
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
	// 切到「本月」（验证区间切换真的会重新取数——这里曾经因为 watcher 被卸载而失效）
	records: `(() => {
		const btn = document.querySelector('[data-range="month"]');
		if (!btn) return false;
		btn.click();
		return true;
	})()`,
	// 连点两下提醒间隔的加号：43 -> 50 -> 60。
	// 连点是有意为之：父组件的 prop 要下一个 tick 才回流，如果步进器按 prop 算，
	// 第二次点击会从旧值重算，结果停在 50 而不是 60——这条断言就是守这个的。
	settings: `(() => {
		const plus = document.querySelector('[data-step="1"]');
		if (!plus) return false;
		plus.click();
		plus.click();
		return true;
	})()`,
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
