# 学习计时器（GoXueTimer）

一个 Windows 桌面端的学习计时工具。正计时、可暂停、可结束，数据落在本地
SQLite 文件里；关窗最小化到系统托盘，连续学习到设定时长会发通知提醒休息。

技术栈：**Go + Wails v3 + Vue 3 + Naive UI**。

## 功能

- **正计时**：开始 / 暂停 / 继续 / 结束并保存 / 放弃。暂停区间不计入时长。
- **崩溃可恢复**：计时状态每次心跳都落库，异常退出后按规则接回（见下文"恢复策略"）。
- **休眠防线**：系统休眠或合盖过夜不会被算成学习时间。
- **系统托盘**：托盘提示今日累计，右键菜单可直接开始/暂停/继续/结束；左键唤回主窗口。
- **关闭即隐藏**：点窗口 × 只把窗口藏进托盘，进程继续跑；托盘「退出」才真正结束。
- **休息提醒**：默认每连续学习 45 分钟发一次 Windows 通知（可调 1~480 分钟、可关闭），
  同时弹应用内提示作为兜底。
- **今日记录**：今天的会话列表可查看、可单条删除。
- **单实例**：重复启动只会把已有窗口叫回前台，不会出现两个进程抢数据库。

不做的事（v1 有意留白）：番茄钟、手动补录/编辑、跨日统计与图表、数据导出、
开机自启、每日目标、云同步、多科目标签。

## 需求环境

本机实测通过的版本：

| 组件 | 版本 |
| --- | --- |
| Wails CLI | `wails3` v3.0.0-beta.27 |
| Go | go1.26.1 windows/amd64 |
| Node.js / npm | 24.21.0 / 11.19.0 |
| WebView2 Runtime | 已随系统安装（`wails3 doctor` 会检查） |

用 `wails3 doctor` 一次性体检。**注意 CLI 叫 `wails3`，不是 `wails`**——
本机装的是 v3 beta，没有 v2 的 `wails` 命令。

## 常用命令

```bash
wails3 doctor                       # 环境体检

wails3 dev                          # 开发模式：前端热更新 + Go 改动自动重建

go build .                          # 只编译后端（比 wails3 build 快得多）
go vet ./internal/... .
go test ./internal/... .            # 全部单元测试与集成测试

wails3 task common:generate:bindings -f   # 改了 Go 的结构体/服务方法后必须重跑
wails3 build                        # 产出 bin/GoXueTimer.exe
```

### 三个容易踩的坑

1. **PowerShell 的执行策略会挡住 `npm.ps1`**。直接敲 `npm install` 会报
   "禁止运行脚本"。请走 `cmd /c "npm install"`，或者 `wails3 build` / `wails3 dev`
   内部的任务（它们自己用 cmd 执行，不受影响）。
2. **`go build ./...` 会失败**，报 `build/ios` 没有 `main` 函数。那是 Wails 模板
   生成的 iOS 交叉编译辅助包（`//go:build !ios` 的 `package main`，设计上要和主包
   一起链接），不是本项目的问题。用 `go build .`。
3. **别手敲 `wails3 generate bindings -ts`**。构建任务实际用的是
   `-clean=true -ts -i`，`-i` 让模型输出成**接口（纯类型）**；少了 `-i` 会生成
   **类**。前端是按接口写的（形如 `const payload: SettingsDTO = {...}`，不能 `new`），
   手敲错参数会得到一堆 `TS2693: only refers to a type, but is being used as a value`。
   要重新生成就统一用 `wails3 task common:generate:bindings -f`。

### 改了 Go 代码之后

`frontend/bindings/` 是**生成物**：绑定生成器默认 `-clean=true`，会先删目录再重建。
不要手改里面的任何文件，改了也会被覆盖。

`vite dev` 只负责前端热更新，不会重新生成绑定。改了 Go 的结构体或服务方法签名后，
必须手动跑一次 `wails3 task common:generate:bindings -f`（生产构建里这一步是自动的）。

所有跨边界的数据结构都定义在 `main` 包的 `dto.go` 里，时间戳一律是带本地时区偏移的
RFC3339 字符串。绑定生成器会把该包里被服务方法用到的类型平铺到
`frontend/bindings/goxuetimer/models.ts`，路径干净，不会因为 `internal/` 的层级变深。

## 数据存放位置

```
%LOCALAPPDATA%\GoXueTimer\
├── timer.db        # SQLite（WAL 模式，另有 -wal / -shm 两个附属文件）
├── app.log         # 运行日志
└── error.log       # 启动阶段的致命错误
```

路径由 `internal/apppath` 逐级降级确定：Wails 的 `PathDataHome` →
`%LOCALAPPDATA%` → `$XDG_DATA_HOME` → `os.UserConfigDir()`。

刻意**不用** `%APPDATA%`（漫游目录）：SQLite 文件不该被漫游配置同步。
也不放在 exe 同目录：装到 `Program Files` 之后那里没有写权限。

备份就是复制 `timer.db`（建议先退出应用，让 WAL 归并回主库文件）。

## 架构

```
main.go              装配 application.Options、窗口、关窗钩子、单实例、日志
app.go               backend：把各部件与它们的回调接线集中在一处
tray.go              托盘图标与菜单，跟随计时状态刷新
notifier.go          系统通知的"失败不致命"包装
dto.go               跨前后端的数据结构 + 时间戳/时长的格式化
timerservice.go      ┐
recordsservice.go    ├ 绑定给前端的三个服务（必须放在 main 包，绑定生成器以
settingsservice.go   ┘ main 包的服务注册为入口）
assets/tray.png      托盘图标（//go:embed）
internal/
  apppath/           数据目录解析（唯一依赖 Wails 运行时的内部包）
  store/             SQLite：连接、pragma、迁移、四张表的读写
  timer/             计时状态机、崩溃恢复、休眠钳制
  reminder/          休息提醒计数（镜像计时器的权威秒数）
  settings/          设置的默认值、合法性收敛与持久化
tools/
  gen_tray_icon.py   重新生成 assets/tray.png
  smoke_test.ps1     端到端冒烟测试（真实窗口/托盘/进程）
  smoke_db.py        smoke_test.ps1 用的 SQLite 读取助手
```

分层的关键约束：`internal/store` 只依赖标准库和 SQLite 驱动，因此测试可以开一个
临时文件库跑真实 SQL；`internal/timer` 与 `internal/reminder` 不依赖 Wails，
时间由注入的 `clock func() time.Time` 提供，所有时间相关用例都是确定性的、毫秒级跑完。

### 状态机

```
idle --Start--> running --Pause--> paused --Resume--> running --Stop--> idle
                     \-------------- Discard -------------/  (任意非 idle 态)
```

- 状态由后端独占，托盘、前端、心跳共用**同一个** `timer.Service` 实例。
- 每次变更都遵循同一条铁律：**先写库、后改内存**，写库失败就把内存回滚。
- 结束会话时用**单个事务**同时写记录和复位状态（`store.FinishSession`）。
  拆成两条语句会留下"记录已落库但状态没复位"的中间态，这时崩溃会导致重复结算。
- 短于 5 秒的会话视为误触，结束时不落库。

### 恢复策略

计时锚点每 30 秒心跳落库一次，所以库里始终是"截至最后一次心跳"的准确值。
重启时按离线时长 `gap = 现在 - 最后一次心跳` 分三种处理：

| gap | 处理 |
| --- | --- |
| ≤ 15 分钟 | 认为只是崩溃/重启，整段计入，继续计时 |
| 15 分钟 ~ 8 小时 | 只补入 15 分钟，其余不计入，继续计时，并提示用户 |
| > 8 小时 | 认为会话早已结束，按最后一次心跳结算并落库（`source='recovered'`） |

暂停态没有"离线时长"的概念，原样恢复。
持久化状态损坏时降级为未计时并提示，而不是让应用起不来。

### 休眠防线

心跳每次比较墙钟 Δ 与预期的 30 秒：Δ 超过 5 分钟就只计入 30 秒，并推一条提示。
没有这道防线的话，合盖过夜再打开会得到一个 8 小时的"学习记录"。
另外还注册了 `SystemWillSleep` / `SystemDidWake`，休眠时暂停、唤醒后恢复。

休息提醒用的是同一套判定，所以它也不会因为休眠而误触发。

## 测试

```bash
go test ./internal/... .                # 60 个用例，全部毫秒级完成
cd frontend && cmd /c "npm run build"   # vue-tsc 类型检查 + 生产构建

# 端到端冒烟测试：需要先 wails3 build
powershell -ExecutionPolicy Bypass -File tools\smoke_test.ps1
```

覆盖的关键场景：

- `internal/store`：迁移幂等、WAL 生效、跨零点会话归属开始日、设置 upsert、空日汇总不报错
- `internal/timer`：暂停区间不计入、时长精度无取整漂移、三种恢复分支、
  休眠钳制、时间回拨、写库失败时内存回滚、回调不死锁
- `internal/settings` / `internal/reminder`：脏数据回落默认值、阈值闭区间、
  暂停不累加、缩短间隔立刻触发、新会话从零重算、重开提醒时基准重对齐
- 主包集成测试：完整的开始→暂停→继续→结束流程落库、过短会话不落库、
  删除记录、设置往返与越界拒绝、心跳结算、心跳驱动提醒（含"事件里的提醒进度
  不能过期"这条不变量）、重启自动结算并广播事件、通知服务未就绪时安静降级

### 端到端冒烟测试

`go test` 覆盖不到真实窗口、托盘和进程行为，那些由 `tools/smoke_test.ps1` 驱动真实
exe 来验证（本机实测 20 项全通过）：

1. 主窗口出现，且前端成功调用后端（`app.log` 里会有锚点日志）
2. `timer.db` 落盘，schema 版本、三张表、WAL、初始状态都对
3. 关窗后进程仍存活、主窗口句柄归零 —— 关闭确实只是隐藏到托盘
4. 重复启动：第二个实例自行退出，第一个继续运行
5. 把 `minimizeToTray` 改成 false 后，关窗能真正退出进程
6. 预置一次"进行中"的会话后启动：走真实的崩溃恢复分支，并等心跳到点，
   确认 `已发出休息提醒 ... systemToast=true` —— 即计时 → 心跳 → 提醒 →
   Windows 通知这整条链路真的通了（这一步要等约 90 秒）

脚本要求数据目录干净，否则会拒绝运行（避免删掉真实记录），确认可以清空就加 `-Force`。
检查数据库内容需要 `python`（标准库 `sqlite3` 即可）；找不到就只跳过那几项。

两个和本机 PowerShell 配置有关的注意点：

- 系统禁止运行脚本（`about_Execution_Policies` 报 UnauthorizedAccess），
  所以要带 `-ExecutionPolicy Bypass`，或者直接 `powershell -File tools\...`。
- 脚本存为 **UTF-8 with BOM**。Windows PowerShell 5.1 会把无 BOM 的 UTF-8 文件
  按 ANSI/GBK 读，中文字符串会被撕碎、直接语法报错。改动这个脚本时别把 BOM 弄丢。

## 打包

```bash
wails3 build         # 便携版 exe：bin/GoXueTimer.exe
```

exe 的版本信息、产品名与清单来自 `build/config.yml` 的 `info:` 段。改完要跑
`wails3 task common:update:build-assets` 才会写进 `build/windows/info.json`
与 `.syso`（下次构建生效）。

**v1 不做安装包**：`makensis`（NSIS）本机没有安装，`wails3 package` 的 NSIS 分支会失败。
要做安装包是后续单独的一件事，需要先装 NSIS 或 MSIX 打包工具。

### 排查问题

- 应用"点了没反应"：看 `%LOCALAPPDATA%\GoXueTimer\app.log` 和 `error.log`。
  生产构建带 `-H windowsgui`，没有控制台窗口，日志只写到文件。
  日志里没有「前端已连接后端」这一行，就说明界面从未成功调用到 Go 侧。
- 只弹应用内提醒、没有 Windows 通知：通知服务在启动时往
  `HKCU\Software\Classes` 注册失败（受限账户或损坏的用户配置）。
  这种情况会记一条 warning 并降级——系统通知注册不上不该让应用打不开。
- 前端拿到了旧的数据结构：忘了跑 `wails3 task common:generate:bindings -f`。
- 托盘图标不显示：Windows 可能把它折叠进"隐藏的图标"区域了。
