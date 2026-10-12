> **声明：本项目全部内容均由 AI 生成。**

# 孤雪凝时

Windows 桌面端的学习计时器。建好计划，对着计划正计时，记录自动汇总成图表与成就。
离线运行，数据保存在本机。

[![版本](https://img.shields.io/github/v/release/QQ510447097/GuXueTimer?label=%E7%89%88%E6%9C%AC&color=3b82f6)](https://github.com/QQ510447097/GuXueTimer/releases)
[![平台](https://img.shields.io/badge/%E5%B9%B3%E5%8F%B0-Windows%2010%20%2F%2011-3b82f6)](https://github.com/QQ510447097/GuXueTimer)
[![技术栈](https://img.shields.io/badge/Go%20%C2%B7%20Wails%20v3%20%C2%B7%20Vue%203-00ADD8)](https://github.com/QQ510447097/GuXueTimer)
[![离线](https://img.shields.io/badge/%E7%A6%BB%E7%BA%BF-%E6%95%B0%E6%8D%AE%E5%9C%A8%E6%9C%AC%E6%9C%BA-success)](https://github.com/QQ510447097/GuXueTimer)
[![许可](https://img.shields.io/badge/%E8%AE%B8%E5%8F%AF-MIT-green)](LICENSE)

![计时页](docs/images/timer.webp)

## 功能

- **计划**：新建 / 改名 / 换色 / 排序 / 归档
- **正计时**：开始 / 暂停 / 继续 / 结束并保存 / 放弃
- **记录**：本周与本月柱状图、当月热力图，可按计划筛选
- **成就与等级**：单次 / 循环 / 每日三组，共九个成就
- **个性化**：浅色 / 深色 / 跟随系统，计时页底部可放一张图
- **托盘**：关闭窗口只是隐藏到托盘，计时继续

## 下载

到 [Releases](https://github.com/QQ510447097/GuXueTimer/releases) 下载：

| 文件 | 说明 |
|---|---|
| 便携版 zip | 解压后双击 `GuXueTimer.exe`，不需要管理员权限 |
| 安装包 setup.exe | 按用户安装，带开始菜单与卸载项 |

首次运行若出现「Windows 已保护你的电脑」，点「更多信息」→「仍要运行」。

## 系统要求

- Windows 10 / 11（64 位）
- 磁盘占用约 20 MB

## 数据

数据位置：`%LOCALAPPDATA%\GuXueTimer`

备份 = 复制该目录；删除软件不影响记录。

## 截图

| 记录 | 计划 |
|---|---|
| ![记录页](docs/images/records.webp) | ![计划页](docs/images/plans.webp) |

| 成就 | 深色主题 |
|---|---|
| ![成就页](docs/images/achievements.webp) | ![深色计时页](docs/images/timer-dark.webp) |

## 开发

技术栈：Go + Wails v3 + Vue 3。构建方式见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。

## 反馈

提 [Issue](https://github.com/QQ510447097/GuXueTimer/issues)，报 bug 请附上
`%LOCALAPPDATA%\GuXueTimer\app.log`。

## 许可

[MIT](LICENSE)
