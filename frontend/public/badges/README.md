# 徽章素材放在这里

每张徽章一个文件，**文件名必须是成就的 id**：

```
frontend/public/badges/first-step.png
frontend/public/badges/hours-10.png
frontend/public/badges/streak-3.png
```

规格：

- 256 × 256 PNG，透明背景
- 画满整张画布（卡片会把它放进 44 × 44 的圆角方框里）
- 建议主体居中、四周留一点余量，缩小到 44px 之后边缘不会被切掉

**没有素材也没关系**：找不到图（或图加载失败）时，卡片会自动回退到内置的矢量奖章，
按稀有度（bronze / silver / gold）上色。所以可以先只加成就定义、之后再补图。

如果图是给浅色主题设计的，深色主题下可能会糊——建议在两种主题下都看一眼
（`tools/shoot.ps1 -Dark` 能直接出一套深色截图）。

当前内置的成就 id 与稀有度见 `internal/achievements/achievements.go` 的 `All`。
