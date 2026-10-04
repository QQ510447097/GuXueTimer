/*
 * Naive UI 的主题覆盖。
 *
 * 为什么不全自己写控件：对话框、输入框、开关这些东西重写一遍收益很低、风险很高
 * （焦点管理、键盘可达性、边界情况）。上一版之所以有"企业级 Element UI"的观感，
 * 根源其实是灰底卡片和 1px 硬边框，而不是控件本身——那些正是我们自己写 HTML 的部分。
 *
 * 所以这里做两件事：
 *   1. 把 naive-ui 的配色、圆角、字体对到蓝白玻璃这套令牌上
 *   2. 把浮层（卡片 / 弹窗 / 提示）的底色改成半透明，再由 glass.css 补 backdrop-filter
 *
 * 数值刻意与 styles/global.css 里的 CSS 变量保持一致：改一处必须改另一处。
 */

import type { GlobalThemeOverrides } from "naive-ui";

const FONT =
	'"Segoe UI", "Microsoft YaHei", "PingFang SC", system-ui, -apple-system, sans-serif';

export function buildThemeOverrides(dark: boolean): GlobalThemeOverrides {
	// 半透明底色：让玻璃面板与浮层能透出背后的渐变与色斑。
	const surface = dark ? "rgba(20, 32, 52, 0.72)" : "rgba(255, 255, 255, 0.72)";
	const surfaceSolid = dark ? "rgba(20, 32, 52, 0.92)" : "rgba(255, 255, 255, 0.94)";
	const border = dark ? "rgba(255, 255, 255, 0.10)" : "rgba(15, 30, 56, 0.08)";
	const accent = dark ? "#60A5FA" : "#3B82F6";
	const accentHover = dark ? "#7CB3FC" : "#5B9BFF";
	const accentPressed = dark ? "#4A93EE" : "#2E6FDD";
	const text = dark ? "#E8EFFA" : "#0F1E38";
	const text2 = dark ? "#9FB0CC" : "#5A6B88";
	const text3 = dark ? "#6C7E9C" : "#8C9AB4";
	const hoverBg = dark ? "rgba(255, 255, 255, 0.07)" : "rgba(15, 30, 56, 0.05)";
	const pressedBg = dark ? "rgba(255, 255, 255, 0.12)" : "rgba(15, 30, 56, 0.09)";
	const inputBg = dark ? "rgba(255, 255, 255, 0.06)" : "rgba(255, 255, 255, 0.55)";

	return {
		common: {
			fontFamily: FONT,
			fontSize: "14px",
			fontSizeMedium: "15px",
			fontSizeLarge: "16px",

			// 圆角比 naive-ui 默认大一档，和玻璃面板的 22px 才协调
			borderRadius: "14px",
			borderRadiusSmall: "10px",

			primaryColor: accent,
			primaryColorHover: accentHover,
			primaryColorPressed: accentPressed,
			primaryColorSuppl: accentHover,

			infoColor: accent,
			successColor: dark ? "#4ADE9F" : "#12A875",
			warningColor: dark ? "#F0C070" : "#E08C1A",
			errorColor: dark ? "#F08A9B" : "#DC4C64",

			textColorBase: text,
			textColor1: text,
			textColor2: text2,
			textColor3: text3,
			textColorDisabled: text3,

			// 页面底色交给 App.vue 的背景层，这里保持透明
			bodyColor: "transparent",
			baseColor: "transparent",
			cardColor: surface,
			modalColor: surface,
			popoverColor: surfaceSolid,
			tableColor: "transparent",
			tableHeaderColor: "transparent",
			inputColor: inputBg,
			actionColor: hoverBg,
			tagColor: hoverBg,
			avatarColor: hoverBg,

			borderColor: border,
			dividerColor: border,
			hoverColor: hoverBg,
			pressedColor: pressedBg,

			boxShadow1: dark
				? "0 8px 24px -10px rgba(0, 0, 0, 0.6)"
				: "0 8px 24px -12px rgba(37, 99, 235, 0.28)",
			boxShadow2: dark
				? "0 12px 32px -12px rgba(0, 0, 0, 0.7)"
				: "0 12px 32px -14px rgba(37, 99, 235, 0.32)",
			boxShadow3: dark
				? "0 16px 40px -14px rgba(0, 0, 0, 0.75)"
				: "0 16px 40px -16px rgba(37, 99, 235, 0.36)",
		},

		Button: {
			borderRadiusMedium: "999px",
			borderRadiusLarge: "999px",
			borderRadiusSmall: "999px",
			// 次要按钮默认是灰底，改成"玻璃"观感
			colorSecondary: hoverBg,
			colorSecondaryHover: pressedBg,
			colorTertiary: "transparent",
			colorTertiaryHover: hoverBg,
			textColorGhost: text2,
			fontWeight: "500",
		},

		Card: {
			borderRadius: "20px",
			color: surface,
			borderColor: border,
		},

		Dialog: {
			borderRadius: "20px",
			color: surfaceSolid,
			titleFontSize: "16px",
		},

		Popover: {
			borderRadius: "16px",
			color: surfaceSolid,
		},

		Notification: {
			borderRadius: "16px",
			color: surfaceSolid,
		},

		Message: {
			borderRadius: "999px",
			color: surfaceSolid,
		},

		Input: {
			borderRadius: "12px",
			color: inputBg,
			colorFocus: inputBg,
		},

		InputNumber: {
			borderRadius: "12px",
		},

		Select: {
			borderRadius: "12px",
		},

		Switch: {
			railColorActive: accent,
			// 玻璃底上默认的灰轨道太脏，压低对比
			railColor: dark ? "rgba(255,255,255,0.14)" : "rgba(15,30,56,0.12)",
		},

		Radio: {
			buttonBorderRadius: "999px",
			color: inputBg,
		},

		Tag: {
			borderRadius: "999px",
		},

		// 记录页已经不用表格了，但保留这层覆盖以防将来复用
		DataTable: {
			thColor: "transparent",
			tdColor: "transparent",
			thTextColor: text2,
			borderColor: border,
		},
	};
}
