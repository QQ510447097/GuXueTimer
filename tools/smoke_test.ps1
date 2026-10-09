<#
.SYNOPSIS
    端到端冒烟测试：直接驱动 bin\GuXueTimer.exe。

.DESCRIPTION
    验证只能在真实 Windows 环境里成立的行为——真实窗口、托盘与注册表都无法用
    go test 覆盖：

      1. Wails 桥接真的通了（前端成功调用后端，app.log 里会留下锚点日志）
      2. 数据库落盘、schema 版本与表结构正确
      3. 关窗只是隐藏到托盘，进程继续存活
      4. 重复启动不会开出第二个进程
      5. 关掉"最小化到托盘"之后，关窗能真正退出
      6. 崩溃恢复（预置进行中的会话 -> 重启后接着计时并落库）
      7. 旧数据目录（GoXueTimer）会被一次性搬到 GuXueTimer
      8. v1 库会被迁移到 v2，旧记录一条不丢
      9. 计划能一路落到记录里（预置进行中的会话 -> 自动结算 -> sessions.plan_id）

    检查数据库内容需要 python（标准库 sqlite3 即可）。找不到 python 时
    只跳过这几项内容校验，其余照跑。

    脚本要求数据目录是干净的：如果 %LOCALAPPDATA%\GuXueTimer 已存在且非空，
    它会拒绝运行，避免删掉真实的学习记录。确认可以清空就加 -Force。

    注意：中文断言全部走 Test-LogContains / Get-LogTail，它们显式按 UTF-8 读。
    Windows PowerShell 5.1 默认按 ANSI 读文件，中文关键字会匹配不上。

.EXAMPLE
    wails3 build
    powershell -ExecutionPolicy Bypass -File tools\smoke_test.ps1 -Force
#>
[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root 'bin\GuXueTimer.exe'
$dataDir = Join-Path $env:LOCALAPPDATA 'GuXueTimer'
# 改名前的目录名：应用自己会把它搬过去，测试要能造出它、也要能清掉它。
$legacyDataDir = Join-Path $env:LOCALAPPDATA 'GoXueTimer'
$logPath = Join-Path $dataDir 'app.log'
$dbPath = Join-Path $dataDir 'timer.db'
$processName = 'GuXueTimer'
$script:dbHelper = Join-Path $PSScriptRoot 'smoke_db.py'

$script:failures = 0
$script:checks = 0
$script:python = $null

function Assert-That {
    param([bool]$Condition, [string]$Message)
    $script:checks++
    if ($Condition) {
        Write-Host "  PASS  $Message" -ForegroundColor Green
    } else {
        Write-Host "  FAIL  $Message" -ForegroundColor Red
        $script:failures++
    }
}

function Write-Skip {
    param([string]$Message)
    Write-Host "  SKIP  $Message" -ForegroundColor DarkGray
}

function Get-AppProcess {
    Get-Process -Name $processName -ErrorAction SilentlyContinue
}

# 结束应用。**优先优雅退出**：--quit 让已有实例走它自己的退出路径，那条路径会
# NIM_DELETE 掉托盘图标；强杀不会，会在通知区域留下"残留图标"（要等鼠标扫过才被
# 系统清掉）。这个脚本一天要跑很多次，全都强杀就等于一直往用户的通知区域里堆东西。
#
# 但有些场景**必须**用强杀：要模拟"进程异常消失"（崩溃恢复、单实例锁残留）时，
# 优雅退出会顺带把状态收拾干净，测不出想测的东西。所以强杀保留为显式选项。
#
# 调用方在收尾处必须自己断言没有残留——残留进程会一直占着 timer.db。
function Stop-App {
    param([int]$Attempts = 3, [switch]$Hard)

    if (-not (Get-AppProcess)) { return }

    if (-not $Hard) {
        try { Start-Process -FilePath $exe -ArgumentList '--quit' -ErrorAction Stop | Out-Null } catch { }
        for ($i = 0; $i -lt 16; $i++) {
            if (-not (Get-AppProcess)) { return }
            Start-Sleep -Milliseconds 300
        }
    }

    for ($i = 1; $i -le $Attempts; $i++) {
        $procs = @(Get-AppProcess)
        if ($procs.Count -eq 0) { return }
        foreach ($p in $procs) {
            try { Stop-Process -Id $p.Id -Force -ErrorAction Stop } catch { }
        }
        Start-Sleep -Milliseconds 700
    }
}

function Start-App {
    Start-Process -FilePath $exe -PassThru
}

# 等到主窗口真的出现，而不是只看进程在不在。
function Wait-MainWindow {
    param([int]$TimeoutSec = 20)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $proc = Get-AppProcess | Select-Object -First 1
        if ($proc -and $proc.MainWindowHandle -ne 0) {
            return $proc
        }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

# python 只用来读 SQLite；没有就跳过内容校验，不影响其他断言。
#
# 所有 Python 代码都走 tools/smoke_db.py，不内联在命令行里：
# PowerShell 向原生命令传参时会吃掉括号，`print(...)` 会变成语法错误。
function Find-Python {
    foreach ($candidate in @('python', 'python3', 'py')) {
        $cmd = Get-Command $candidate -ErrorAction SilentlyContinue
        if (-not $cmd) { continue }
        $out = & $cmd.Source $script:dbHelper --check 2>&1
        if ($LASTEXITCODE -eq 0 -and ($out -join '') -match 'ok') { return $cmd.Source }
    }
    return $null
}

# 日志是 Go 写的 UTF-8，必须显式按 UTF-8 读。
# Windows PowerShell 5.1 默认按 ANSI 读文件，中文关键字会匹配不上。
function Test-LogContains {
    param([string]$Pattern)
    if (-not (Test-Path $logPath)) { return $false }
    $text = Get-Content -LiteralPath $logPath -Raw -Encoding UTF8
    return $text -match [regex]::Escape($Pattern)
}

function Get-LogTail {
    param([int]$Count = 20)
    if (-not (Test-Path $logPath)) { return @('(没有 app.log)') }
    return @(Get-Content -LiteralPath $logPath -Tail $Count -Encoding UTF8)
}

# 返回一个 key -> value 的哈希表。
function Get-DbInfo {
    $info = @{}
    if (-not $script:python) { return $info }
    $lines = & $script:python $script:dbHelper inspect $dbPath 2>&1
    foreach ($line in $lines) {
        if ($line -match '^([^=]+)=(.*)$') { $info[$Matches[1]] = $Matches[2] }
    }
    return $info
}

Write-Host "`n=== 孤雪凝时 冒烟测试 ===" -ForegroundColor Cyan

# ---------------------------------------------------------------- 前置检查

if (-not (Test-Path $exe)) {
    Write-Host "找不到 $exe —— 先跑 wails3 build" -ForegroundColor Red
    exit 2
}

# 先结束上一轮可能残留的进程，再动数据目录。
#
# 顺序不能反：残留进程占着 timer.db 时，下面那个删除会静默失败，于是上一轮的
# 设置（比如 minimizeToTray=false）会渗进这一轮，后面场景就得出莫名其妙的结论。
$script:python = Find-Python
Stop-App

# 新旧两个目录都要清干净。旧目录留着的话，应用会在启动时把它搬进新目录，
# "干净状态"这个前提就不成立了。
foreach ($dir in @($dataDir, $legacyDataDir)) {
    if (-not ((Test-Path $dir) -and (Get-ChildItem -Force $dir | Measure-Object).Count -gt 0)) {
        continue
    }
    if (-not $Force) {
        Write-Host "数据目录已有内容：$dir" -ForegroundColor Yellow
        Write-Host "为免删掉真实学习记录，脚本就此停下。确认可以清空就加 -Force。" -ForegroundColor Yellow
        exit 2
    }
    Write-Host "清空数据目录（-Force）：$dir" -ForegroundColor Yellow
    for ($i = 1; $i -le 5; $i++) {
        Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
        if (-not (Test-Path $dir)) { break }
        Start-Sleep -Milliseconds 600
    }
    if (Test-Path $dir) {
        Write-Host "数据目录删不掉，多半还有进程占着：$dir" -ForegroundColor Red
        Write-Host "先手工结束 $processName 进程再重跑。" -ForegroundColor Red
        exit 2
    }
}

# ---------------------------------------------------------------- 1. 启动与桥接

Write-Host "`n[1] 启动与 Wails 桥接"
$proc = Start-App
$windowProc = Wait-MainWindow
Assert-That ($null -ne $windowProc) '主窗口出现'

$deadline = (Get-Date).AddSeconds(15)
while ((Get-Date) -lt $deadline) {
    if (Test-LogContains '前端已连接后端') { break }
    Start-Sleep -Milliseconds 300
}
$bridgeOk = Test-LogContains '前端已连接后端'
Assert-That $bridgeOk '前端成功调用后端（app.log 有锚点日志）'
if (-not $bridgeOk) {
    Write-Host '  --- app.log 尾部 ---' -ForegroundColor Yellow
    Get-LogTail 20 | ForEach-Object { "  $_" }
}

# ---------------------------------------------------------------- 2. 数据库

Write-Host "`n[2] 数据库落盘"
Assert-That (Test-Path $dbPath) 'timer.db 已创建'

if (-not $script:python) {
    Write-Skip '找不到 python，跳过数据库内容校验'
} elseif (Test-Path $dbPath) {
    $info = Get-DbInfo

    Assert-That ($info['user_version'] -eq '2') "schema 版本 = 2（实际 '$($info['user_version'])'）"

    foreach ($t in @('sessions', 'settings', 'timer_state', 'plans')) {
        Assert-That (($info['tables'] -split ',') -contains $t) "存在表 $t"
    }

    Assert-That ($info['sessions_has_plan_id'] -eq '1') 'sessions 有 plan_id 列'

    Assert-That ($info['journal_mode'].ToLower() -eq 'wal') "journal_mode = wal（实际 '$($info['journal_mode'])'）"

    Assert-That ($info['timer_state'] -eq 'idle') "初始计时状态 = idle（实际 '$($info['timer_state'])'）"
}

# ---------------------------------------------------------------- 3. 关窗隐藏

Write-Host "`n[3] 关窗最小化到托盘"

# CloseMainWindow 依赖 MainWindowHandle 有效。机器忙的时候可能取到还没刷新出来的 0，
# 那样它返回 false 且一个消息都不发——后面就会看到"进程活着但窗口没隐藏"这种
# 假失败。所以这里先重试把手柄拿到，再轮询等它清零。
$closeRequested = $false
for ($i = 1; $i -le 6 -and -not $closeRequested; $i++) {
    $live = Get-AppProcess | Select-Object -First 1
    if (-not $live) { break }
    if ($live.MainWindowHandle -ne 0) {
        $closeRequested = [bool]$live.CloseMainWindow()
    }
    if (-not $closeRequested) { Start-Sleep -Milliseconds 600 }
}
Assert-That $closeRequested '成功向主窗口发出关闭请求'

# Hide 是异步的；.NET 只把"可见"的顶层窗口算作主窗口，所以句柄清零就是隐藏成功的证据。
$after = Get-AppProcess | Select-Object -First 1
$hidden = $false
$deadline = (Get-Date).AddSeconds(10)
while ((Get-Date) -lt $deadline) {
    $after = Get-AppProcess | Select-Object -First 1
    if (-not $after) { break }
    if ($after.MainWindowHandle -eq 0) { $hidden = $true; break }
    Start-Sleep -Milliseconds 400
}
Assert-That ($null -ne $after) '关窗后进程仍然存活（隐藏到托盘）'
Assert-That $hidden '主窗口已隐藏'

# ---------------------------------------------------------------- 4. 单实例

Write-Host "`n[4] 单实例"
$firstPid = $after.Id
$second = Start-App
$exited = $second.WaitForExit(10000)
Assert-That $exited '第二个实例自行退出'
Assert-That ($null -ne (Get-Process -Id $firstPid -ErrorAction SilentlyContinue)) '第一个实例仍在运行'

# ---------------------------------------------------------------- 5. 关窗即退出

Write-Host "`n[5] 关闭「最小化到托盘」后关窗应真正退出"
Stop-App

if ($script:python) {
    $patched = (& $script:python $script:dbHelper set-setting $dbPath minimizeToTray false 2>&1) -join ''
    Assert-That ($patched -match 'minimizeToTray=false') "已把 minimizeToTray 改为 false（$patched）"

    $proc = Start-App
    $windowProc = Wait-MainWindow
    Assert-That ($null -ne $windowProc) '重新启动后主窗口出现'

    $live = Get-AppProcess | Select-Object -First 1
    if ($live) {
        $live.CloseMainWindow() | Out-Null
        Start-Sleep -Seconds 5
        $gone = $null -eq (Get-Process -Id $live.Id -ErrorAction SilentlyContinue)
        Assert-That $gone '关窗后进程真正退出（没有变成无窗口的僵尸进程）'
    }
} else {
    Write-Skip '找不到 python，无法改写设置，跳过该场景'
}

# ---------------------------------------------------------------- 6. 崩溃恢复

Write-Host "`n[6] 崩溃恢复（预置一次进行中的会话，重启后必须接着计时）"

if ($script:python) {
    Stop-App
    $seeded = (& $script:python $script:dbHelper seed-running $dbPath 1 2>&1) -join ' '
    Assert-That ($seeded -match 'timer_state=running') "已预置一次进行中的会话（$seeded）"

    $proc = Start-App
    $null = Wait-MainWindow

    # 恢复分支应该留下痕迹：会话被接着计时，而不是被丢弃。
    # 等一拍心跳让时长真的涨起来（心跳 30 秒一次）。
    $deadline = (Get-Date).AddSeconds(45)
    $restored = $false
    while ((Get-Date) -lt $deadline) {
        $state = (& $script:python $script:dbHelper inspect $dbPath 2>&1) -join ' '
        # 字段名是 timer_accumulated_sec；0 表示心跳还没落过账。
        if ($state -match 'timer_state=running' -and $state -match 'timer_accumulated_sec=[1-9]\d*') {
            $restored = $true
            break
        }
        Start-Sleep -Seconds 3
    }
    $state = (& $script:python $script:dbHelper inspect $dbPath 2>&1) -join ' '
    Assert-That ($state -match 'timer_state=running') '重启后会话被恢复为运行中'
    Assert-That $restored "心跳把时长结算进了库（$state）"
    if (-not $restored) {
        Write-Host '  --- app.log 尾部 ---' -ForegroundColor Yellow
        Get-LogTail 12 | ForEach-Object { "  $_" }
    }
    Stop-App
} else {
    Write-Skip '找不到 python，无法预置计时状态，跳过该场景'
}

# ---------------------------------------------------------------- 7. 旧目录搬迁

Write-Host "`n[7] 旧数据目录（GoXueTimer）一次性搬迁"

if ($script:python) {
    Stop-App

    # 造一个"改名之前运行过"的现场：把现在的数据目录整体挪回旧名字，
    # 并留下一枚标记，用来证明搬过去的是同一份数据，而不是新建了一个空库。
    $marked = (& $script:python $script:dbHelper set-setting $dbPath smokeMarker keep-me 2>&1) -join ''
    Assert-That ($marked -match 'smokeMarker=keep-me') "已在库里留下搬迁标记（$marked）"

    if (Test-Path $legacyDataDir) { Remove-Item -Recurse -Force $legacyDataDir -ErrorAction SilentlyContinue }
    Move-Item -LiteralPath $dataDir -Destination $legacyDataDir -ErrorAction Stop
    Assert-That ((Test-Path $legacyDataDir) -and -not (Test-Path $dataDir)) '已把数据目录挪回旧名 GoXueTimer'

    $proc = Start-App
    Assert-That ($null -ne (Wait-MainWindow)) '搬迁后应用仍能启动'
    Stop-App

    Assert-That (Test-Path $dataDir) '新目录 GuXueTimer 被重新建立'
    Assert-That (-not (Test-Path $legacyDataDir)) '旧目录被搬空后删掉了'

    $kept = (& $script:python $script:dbHelper get-setting $dbPath smokeMarker 2>&1) -join ''
    Assert-That ($kept -match 'smokeMarker=keep-me') "搬过去的是同一份数据库（$kept）"
} else {
    Write-Skip '找不到 python，无法留标记，跳过该场景'
}

# ---------------------------------------------------------------- 8. v1 库迁移

Write-Host "`n[8] v1 库升级到 v2（旧记录一条不丢）"

if ($script:python) {
    Stop-App
    # 从零造一个 v1 时代的库：v1 建表语句 + user_version=1 + 一条旧记录 + 旧设置。
    if (Test-Path $dataDir) { Remove-Item -Recurse -Force $dataDir -ErrorAction SilentlyContinue }
    New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

    $seeded = (& $script:python $script:dbHelper seed-v1-db $dbPath 2>&1) -join ' '
    Assert-That ($seeded -match 'user_version=1') "已造好 v1 库（$seeded）"

    # 启动一次，让应用自己做迁移。
    $proc = Start-App
    Assert-That ($null -ne (Wait-MainWindow)) 'v1 库上应用能正常启动'
    Stop-App

    $after = Get-DbInfo
    Assert-That ($after['user_version'] -eq '2') "迁移后 schema 版本 = 2（实际 '$($after['user_version'])'）"
    Assert-That ($after['sessions_has_plan_id'] -eq '1') '迁移后 sessions 有 plan_id 列'
    Assert-That (($after['tables'] -split ',') -contains 'plans') '迁移后有了 plans 表'

    # 旧记录必须还在，并且归为"未分类"。
    Assert-That ($after['last_session_sec'] -eq '1800') "旧记录时长保住了（实际 '$($after['last_session_sec'])'）"
    Assert-That ($after['last_session_note'] -match 'v1') "旧记录内容保住了（实际 '$($after['last_session_note'])'）"
    Assert-That ($after['last_session_plan_id'] -eq 'NULL') "旧记录归为未分类（实际 '$($after['last_session_plan_id'])'）"

    # 旧设置也必须还在（用户改过的主题不能被重置）。
    $theme = (& $script:python $script:dbHelper get-setting $dbPath theme 2>&1) -join ''
    Assert-That ($theme -match 'theme=dark') "旧设置保住了（$theme）"
} else {
    Write-Skip '找不到 python，无法造 v1 库，跳过该场景'
}

# ---------------------------------------------------------------- 9. 计划落到记录

Write-Host "`n[9] 计划一路落到记录里（timer_state -> 恢复 -> 自动结算 -> sessions.plan_id）"

if ($script:python) {
    Stop-App
    # 预置"一个计划 + 一段 10 小时前中断的会话"：启动时恢复逻辑会判定中断过久，
    # 自动结算并落库。整条通路不需要任何界面交互就能验证。
    $seeded = (& $script:python $script:dbHelper seed-plan-and-stale-running $dbPath 10 2>&1) -join ' '
    Assert-That ($seeded -match 'plan_id=\d+') "已预置计划与中断会话（$seeded）"
    $planId = [regex]::Match($seeded, 'plan_id=(\d+)').Groups[1].Value

    $proc = Start-App
    Assert-That ($null -ne (Wait-MainWindow)) '预置状态下应用能正常启动'
    # 自动结算发生在启动阶段，但落库要等恢复流程跑完，稍等一下再查。
    Start-Sleep -Seconds 3
    Stop-App

    $after = Get-DbInfo
    Assert-That ($after['last_session_sec'] -eq '600') "自动结算写入了记录（实际 '$($after['last_session_sec'])' 秒）"
    Assert-That ($after['last_session_source'] -eq 'recovered') "记录来源是 recovered（实际 '$($after['last_session_source'])'）"
    Assert-That ($after['last_session_plan_id'] -eq $planId) "记录归属到预置的计划 $planId（实际 '$($after['last_session_plan_id'])'）"
    Assert-That ($after['timer_state'] -eq 'idle') "结算后计时状态回到 idle（实际 '$($after['timer_state'])'）"
} else {
    Write-Skip '找不到 python，无法预置计划，跳过该场景'
}

Stop-App
Assert-That ((@(Get-AppProcess)).Count -eq 0) '收尾时没有残留进程（残留会占住 timer.db）'

Write-Host "`n测试数据留在：$dataDir" -ForegroundColor DarkGray
Write-Host '注意：跑完之后应用的数据目录里是一份**测试库**（最后两幕会把它替换成合成数据，' -ForegroundColor DarkGray
Write-Host '      主题也会被第 8 幕灌成 dark）。想恢复成一份好看的演示数据：' -ForegroundColor DarkGray
Write-Host '        python tools\seed_demo.py --clear' -ForegroundColor DarkGray

# ---------------------------------------------------------------- 汇总

Write-Host "`n=== 结果 ===" -ForegroundColor Cyan
if ($script:failures -eq 0) {
    Write-Host "全部通过（$($script:checks) 项）" -ForegroundColor Green
    exit 0
}
Write-Host "$($script:failures)/$($script:checks) 项失败" -ForegroundColor Red
Write-Host "日志：$logPath"
exit 1
