# 孤雪学习计时器（GuXueTimer）

一个 Windows 桌面端的学习计时工具。先建学习计划，再对着计划正计时；数据落在本地
SQLite 文件里，关窗最小化到系统托盘，连续学习到设定时长会发通知提醒休息。
记录页有近 N 天柱状图和月历热力图，能看出每天学了多久、各计划各占多少。

技术栈：**Go + Wails v3 + Vue 3 + Naive UI**。运行期完全离线，不发起任何网络请求。

## 功能

- **学习计划**：新建 / 改名 / 换色 / 排序 / 归档 / 删除。没有任何在用的计划时，
  应用只会显示"建第一个计划"的引导页——不选计划就没法开始计时。
- **正计时**：开始 / 暂停 / 继续 / 结束并保存 / 放弃。暂停区间不计入时长。
- **归属清晰**：每次会话都记在当时的计划上；计时进行中不允许切换或归档当前计划。
- **记录与统计**：近 7 / 14 / 30 天柱状图 + 当月热力图日历（按天深浅着色，
  点某天看当天明细），可按计划筛选；当天还能看到按计划分解的时长占比。
- **崩溃可恢复**：计时状态每次心跳都落库，异常退出后按规则接回（见"恢复策略"）。
- **休眠防线**：系统休眠或合盖过夜不会被算成学习时间。
- **系统托盘**：提示里带当前计划与今日累计，右键菜单可直接开始/暂停/继续/结束。
- **关闭即隐藏**：点窗口 × 只把窗口藏进托盘，进程继续跑；托盘「退出」才真正结束。
- **休息提醒**：默认每连续学习 45 分钟发一次 Windows 通知（可调 1~480 分钟、可关闭），
  同时弹应用内提示作为兜底。
- **单实例**：重复启动只会把已有窗口叫回前台，不会出现两个进程抢数据库。

不做的事（有意留白）：番茄钟、手动补录/编辑记录、每日目标、数据导出、开机自启、
云同步、把记录批量改挂到别的计划、安装包。

## 界面

蓝白配色 + 玻璃质感，浅色是默认（深色是蓝调深色，不是灰）。

- 设计令牌（颜色 / 圆角 / 动效）全在 `frontend/src/styles/global.css`，
  主题切换只切 `<html>` 上的 `.theme-dark` 一个类，组件里不出现写死的色值。
- 玻璃质感的原子类在 `frontend/src/styles/glass.css`：`.gx-glass`、`.gx-glass-2`、
  `.gx-pill`、`.gx-lift`、`.gx-accent-btn`、`.gx-ghost`。
- Naive UI 的配色与圆角覆盖在 `frontend/src/theme/naiveTheme.ts`。控件仍用 naive-ui
  （输入、开关、对话框、通知这些重写收益低），但"门面"——时钟、图表、日历、导航、
  记录行——都是自己写的 HTML/CSS。上一版之所以像企业后台，根源是灰底卡片和 1px 硬边框，
  那些正是自定义的部分。

一个实测出来的经验：**只靠 `backdrop-filter` 是做不出玻璃感的**。它模糊的是背后的内容，
而纯渐变属于低频信号，糊了和没糊肉眼几乎没差别。所以背景层特意保留了色斑的边界
（`filter: blur(40px)` 而不是更大），并铺了一层极淡的噪点（内联 SVG 的 `feTurbulence`）
提供高频细节——面板掠过色斑时才看得出"糊了一层"。

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
wails3 build                        # 产出 bin/GuXueTimer.exe
```

### 三个容易踩的坑

1. **PowerShell 的执行策略会挡住脚本**。直接敲 `npm install` 会报"禁止运行脚本"，
   `tools/` 下的 `.ps1` 也一样。前者走 `cmd /c "npm install"`（`wails3 build` / `wails3 dev`
   内部的任务自己用 cmd 执行，不受影响）；后者加 `-ExecutionPolicy Bypass`。
   另外 `frontend/.npmrc` 里的 `minimum-release-age` 会让 npm 每次都打一条
   "Unknown project config" 警告——那是给 pnpm/bun 用的供应链设置，npm 自己不支持，
   警告是预期的、可以忽略（留着是为了将来换包管理器时自动生效）。
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
RFC3339 字符串（刻意不用 `time.Time`：它会变成 JS 的 `Date`，再叠一层浏览器时区解释）。
绑定生成器会把该包里被服务方法用到的类型平铺到 `frontend/bindings/goxuetimer/models.ts`。

## 数据存放位置

```
%LOCALAPPDATA%\GuXueTimer\
├── timer.db        # SQLite（WAL 模式，另有 -wal / -shm 两个附属文件）
├── app.log         # 运行日志
└── error.log       # 启动阶段的致命错误
```

路径由 `internal/apppath` 逐级降级确定：Wails 的 `PathDataHome` →
`%LOCALAPPDATA%` → `$XDG_DATA_HOME` → `os.UserConfigDir()`。

刻意**不用** `%APPDATA%`（漫游目录）：SQLite 文件不该被漫游配置同步。
也不放在 exe 同目录：装到 `Program Files` 之后那里没有写权限。

目录名从改动前的 `GoXueTimer` 换成了 `GuXueTimer`，启动时会做一次性搬迁：
旧目录非空、新目录里还没有 `timer.db` 时，逐项搬过去（同名文件不覆盖），搬完删掉空壳。
判定用的是"新目录里没有库"而不是"新目录不存在"——新目录很可能已被上一次启动建成了空目录。

备份就是复制 `timer.db`（建议先退出应用，让 WAL 归并回主库文件）。

## 架构

```
main.go              装配 application.Options、窗口、关窗钩子、单实例、日志
app.go               backend：把各部件与它们的回调接线集中在一处
tray.go              托盘图标与菜单，跟随计时状态刷新
notifier.go          系统通知的"失败不致命"包装
dto.go               跨前后端的数据结构 + 时间戳/时长的格式化
timerservice.go      ┐
recordsservice.go    ├ 绑定给前端的四个服务（必须放在 main 包，绑定生成器以
plansservice.go      │ main 包的服务注册为入口）
settingsservice.go   ┘
assets/tray.png      托盘图标（//go:embed）
internal/
  apppath/           数据目录解析与旧目录搬迁（唯一依赖 Wails 运行时的内部包）
  store/             SQLite：连接、pragma、版本化迁移、四张表的读写
  timer/             计时状态机、崩溃恢复、休眠钳制
  reminder/          休息提醒计数（镜像计时器的权威秒数）
  settings/          设置的默认值、合法性收敛与持久化
tools/
  gen_icons.py       生成 build/appicon.png 与 assets/tray.png
  smoke_test.ps1     端到端冒烟测试（真实窗口/托盘/进程/数据库）
  smoke_db.py        smoke_test.ps1 用的 SQLite 辅助脚本
  seed_demo.py       灌一份"像用了两个月"的演示数据，用来截图与肉眼验收界面
  shoot.ps1          启动应用并用 CDP 截图 + 体检界面布局
  ui_probe.mjs       shoot.ps1 调用的 CDP 客户端（零依赖）
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
- 计时器本身**不认识"计划"**，只把 `PlanID` 当不透明值一路携带；"必须选一个计划"
  这类产品规则留在服务层。

### 数据模型与迁移

`schemaVersion = 2`。迁移按版本递进（`if version < 1` / `if version < 2`），
在单个事务里跑完，`PRAGMA user_version` 记录进度。

**全新库也会跑一遍 v1 → v2**（先建 v1 的表，再 `ALTER TABLE` 加列），
这样新装用户和升级用户的 schema 永远一致。因此有一条纪律：**不要回头改老的 `schemaVn`**，
只往后追加。

两处刻意的取舍：

- `sessions.plan_id` **可空且不加外键**。完整性由服务层保证（有记录的计划拒绝删除、
  只能归档），换来的是"哪怕计划被删，学习记录也一定写得进去"——时长比约束宝贵。
  计划真被删掉时记录会变成孤儿，界面按 `planId` 查不到就显示"未分类"，不会崩。
- `sessions.plan_id` 为 `NULL` 表示未分类。应用层统一用 `0` 表示，避免可空 `int64`
  一路传染到 DTO；读写边界各有一个转换函数。

### 恢复策略

计时锚点每 30 秒心跳落库一次，所以库里始终是"截至最后一次心跳"的准确值。
重启时按离线时长 `gap = 现在 - 最后一次心跳` 分三种处理：

| gap | 处理 |
| --- | --- |
| ≤ 15 分钟 | 认为只是崩溃/重启，整段计入，继续计时 |
| 15 分钟 ~ 8 小时 | 只补入 15 分钟，其余不计入，继续计时，并提示用户 |
| > 8 小时 | 认为会话早已结束，按最后一次心跳结算并落库（`source='recovered'`） |

暂停态没有"离线时长"的概念，原样恢复。持久化状态损坏时降级为未计时并提示，
而不是让应用起不来。自动结算出来的记录，归属的是**当时那次会话选的计划**，
不是"现在"的当前计划。

### 休眠防线

心跳每次比较墙钟 Δ 与预期的 30 秒：Δ 超过 5 分钟就只计入 30 秒，并推一条提示。
没有这道防线的话，合盖过夜再打开会得到一个 8 小时的"学习记录"。
另外还注册了 `SystemWillSleep` / `SystemDidWake`，休眠时暂停、唤醒后恢复。

休息提醒直接镜像计时器的权威秒数（而不是自己再算一遍墙钟），所以暂停与休眠
天然不会累加，两套判定也不会出现分歧。

## 测试

```bash
go test ./internal/... .                # 100 个用例，全部毫秒级完成
cd frontend && cmd /c "npm run build"   # vue-tsc 类型检查 + 生产构建

# 端到端冒烟测试：需要先构建 bin\GuXueTimer.exe
powershell -ExecutionPolicy Bypass -File tools\smoke_test.ps1

# 看界面长什么样（截图 + 布局体检，产物在 screenshots\）
powershell -ExecutionPolicy Bypass -File tools\shoot.ps1
powershell -ExecutionPolicy Bypass -File tools\shoot.ps1 -Size 560x680 -Dark
```

覆盖的关键场景：

- `internal/store`：**造一个真正的 v1 库再打开**（旧记录与旧设置一条不丢、
  `plan_id` 为 NULL）、迁移幂等、WAL 生效、跨零点会话归属开始日、
  计划增删改查与重排、按计划筛选、`DailyTotals` 区间与缺日不补零
- `internal/timer`：暂停区间不计入、时长精度无取整漂移、三种恢复分支、
  休眠钳制、时间回拨、写库失败时内存回滚、回调不死锁、
  计划在"开始→暂停→恢复→结束"与两条恢复分支上都不丢
- `internal/settings` / `internal/reminder`：脏数据回落默认值、阈值闭区间、
  暂停不累加、缩短间隔立刻触发、新会话从零重算、重开提醒时基准重对齐
- 主包集成测试：完整计时流程落库、计划服务的全套校验（重名、配色白名单、
  长度、控制字符）、归档/删除当前计划后自动纠正、有记录的计划拒绝删除、
  计时中拒绝切换计划、保存设置不会清掉当前计划、心跳驱动提醒
  （含"事件里的提醒进度不能过期"这条不变量）

### 端到端冒烟测试

`go test` 覆盖不到真实窗口、托盘、进程和注册表行为，那些由 `tools/smoke_test.ps1`
驱动真实 exe 来验证（本机实测 45 项全通过）：

1. 主窗口出现，且前端成功调用后端（`app.log` 里会有锚点日志）
2. `timer.db` 落盘：schema 版本 = 2、四张表、`sessions.plan_id` 列、WAL、初始状态
3. 关窗后进程仍存活、主窗口句柄归零 —— 关闭确实只是隐藏到托盘
4. 重复启动：第二个实例自行退出，第一个继续运行
5. 把 `minimizeToTray` 改成 false 后，关窗能真正退出进程
6. 预置一次"进行中"的会话后启动：走真实的崩溃恢复分支，并等心跳到点，
   确认 `已发出休息提醒 ... systemToast=true` —— 计时 → 心跳 → 提醒 →
   Windows 通知这整条链路真的通了（这一步要等约 90 秒）
7. 旧数据目录（`GoXueTimer`）被一次性搬进新目录，且搬的是同一份库
8. 造一个 v1 库再启动：升到 v2，旧记录与旧设置都在
9. 预置"计划 + 10 小时前中断的会话"再启动：恢复逻辑自动结算，
   落库的记录归属到那个计划 —— 不需要任何界面交互就验证了
   `timer_state.plan_id → 恢复 → FinishSession → sessions.plan_id` 这条完整通路

脚本要求数据目录干净，否则会拒绝运行（避免删掉真实记录），确认可以清空就加 `-Force`。
检查数据库内容需要 `python`（标准库 `sqlite3` 即可）；找不到就只跳过那几项。

两个和本机 PowerShell 配置有关的注意点：

- 系统禁止运行脚本（`about_Execution_Policies` 报 UnauthorizedAccess），
  所以要带 `-ExecutionPolicy Bypass`，或者直接 `powershell -File tools\...`。
- 脚本存为 **UTF-8 with BOM**。Windows PowerShell 5.1 会把无 BOM 的 UTF-8 文件
  按 ANSI/GBK 读，中文字符串会被撕碎、直接语法报错。改动这些脚本时别把 BOM 弄丢。
  （`tools/*.py` 不用管，Python 默认按 UTF-8 读源码。）

### 界面截图与布局体检

`tools/shoot.ps1` 做一件代码审查看不出来的事：**把界面真正渲染出来看一眼**。

它给应用开一个 WebView2 远程调试端口（只在设置了 `GX_DEBUG_PORT` 时开，发布构建不受影响），
然后用 `Page.captureScreenshot` 出图——渲染进程直接给图，**窗口不必可见，也不会在你屏幕上
反复弹窗**。顺带读回计算样式与几何信息，于是"有没有横向滚动条""毛玻璃到底生效没有"
从"我觉得"变成可断言的。

它同时是一道界面回归检查：每个视图都要能点到、点击后真的切过去、任何一屏都不许出现
横向滚动条、每屏都得有玻璃面板，否则以非零码退出。

```powershell
# 截图前先灌一份演示数据，否则图表都是空的，什么也看不出来（需要先退出应用）
python tools\seed_demo.py --clear

# 四个视图各截一张，并打印每屏的视口/内容尺寸、主题、玻璃面板数、backdrop-filter
powershell -ExecutionPolicy Bypass -File tools\shoot.ps1

# 验收最小尺寸下的布局：内容区只有约 448px 宽
powershell -ExecutionPolicy Bypass -File tools\shoot.ps1 -Size 560x680

# 深色（往 <html> 挂 theme-dark 类，不改数据库里的设置）
powershell -ExecutionPolicy Bypass -File tools\shoot.ps1 -Dark
```

产物在 `screenshots/`（已 gitignore）。`ui_probe.mjs` 靠导航项上的 `data-view` /
`data-active` 属性切视图并读状态，所以改动 `NavRail.vue` 时要保留这两个属性。

两个实测踩到的坑，留给下次：

- `backdrop-filter` **只写标准属性**，不要自己再补一份 `-webkit-`。压缩器会按目标浏览器
  自动加前缀，但手写双份会被它当成重复声明——实测结果是标准属性被删掉、只剩前缀版。
- 截图脚本本身不该有副作用。曾经因为空格键是全局快捷键，在记录页误按一下就开始了一次
  计时（数据库里多出一条 `paused` 状态），所以现在空格只在计时页生效。

## 打包

```bash
wails3 build         # 便携版 exe：bin/GuXueTimer.exe
```

exe 的版本信息、产品名与清单来自 `build/config.yml` 的 `info:` 段。改完要跑
`wails3 task common:update:build-assets` 才会写进 `build/windows/info.json`
与 `.syso`（下次构建生效）。图标来自 `build/appicon.png`（1024×1024），
跑 `wails3 task common:generate:icons` 生成 `build/windows/icon.ico`。

注意：`.NET` 的 `FileVersionInfo` 读不出本项目的版本字符串（资源用的是 `lang=0` 的
翻译块），但这不影响资源管理器——用 `VerQueryValue` 能正常读到 `ProductName` 等字段。

**不做安装包**：`makensis`（NSIS）本机没有安装，`wails3 package` 的 NSIS 分支会失败。
要做安装包是后续单独的一件事，需要先装 NSIS 或 MSIX 打包工具。

### 排查问题

- 应用"点了没反应"：看 `%LOCALAPPDATA%\GuXueTimer\app.log` 和 `error.log`。
  生产构建带 `-H windowsgui`，没有控制台窗口，日志只写到文件。
  日志里没有「前端已连接后端」这一行，就说明界面从未成功调用到 Go 侧。
- 只弹应用内提醒、没有 Windows 通知：通知服务在启动时往
  `HKCU\Software\Classes` 注册失败（受限账户或损坏的用户配置）。
  这种情况会记一条 warning 并降级——系统通知注册不上不该让应用打不开。
- 前端拿到了旧的数据结构：忘了跑 `wails3 task common:generate:bindings -f`。
- 界面像是旧版本：`bin\GuXueTimer.exe` 里的前端是构建时嵌进去的，
  改了前端要重新 `wails3 build`（只跑 `npm run build` 不会更新 exe）。
- 托盘图标不显示：Windows 可能把它折叠进"隐藏的图标"区域了。
