<#
.SYNOPSIS
    端到端冒烟测试：直接驱动 bin\GoXueTimer.exe。

.DESCRIPTION
    验证只能在真实 Windows 环境里成立的行为——真实窗口、托盘与注册表都无法用
    go test 覆盖：

      1. Wails 桥接真的通了（前端成功调用后端，app.log 里会留下锚点日志）
      2. 数据库落盘、schema 版本与表结构正确
      3. 关窗只是隐藏到托盘，进程继续存活
      4. 重复启动不会开出第二个进程
      5. 关掉"最小化到托盘"之后，关窗能真正退出

    检查数据库内容需要 python（标准库 sqlite3 即可）。找不到 python 时
    只跳过这几项内容校验，其余照跑。

    脚本要求数据目录是干净的：如果 %LOCALAPPDATA%\GoXueTimer 已存在且非空，
    它会拒绝运行，避免删掉真实的学习记录。确认可以清空就加 -Force。

.EXAMPLE
    wails3 build
    pwsh tools\smoke_test.ps1
#>
[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root 'bin\GoXueTimer.exe'
$dataDir = Join-Path $env:LOCALAPPDATA 'GoXueTimer'
$logPath = Join-Path $dataDir 'app.log'
$dbPath = Join-Path $dataDir 'timer.db'
$processName = 'GoXueTimer'
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

# 结束应用。多次重试：窗口隐藏到托盘之后，进程退出可能还要抢一下 SQLite 的文件句柄。
# 调用方在收尾处必须自己断言没有残留——残留进程会一直占着 timer.db。
function Stop-App {
    param([int]$Attempts = 3)
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

Write-Host "`n=== GoXueTimer 冒烟测试 ===" -ForegroundColor Cyan

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

if ((Test-Path $dataDir) -and (Get-ChildItem -Force $dataDir | Measure-Object).Count -gt 0) {
    if (-not $Force) {
        Write-Host "数据目录已有内容：$dataDir" -ForegroundColor Yellow
        Write-Host "为免删掉真实学习记录，脚本就此停下。确认可以清空就加 -Force。" -ForegroundColor Yellow
        exit 2
    }
    Write-Host "清空数据目录（-Force）：$dataDir" -ForegroundColor Yellow
    for ($i = 1; $i -le 5; $i++) {
        Remove-Item -Recurse -Force $dataDir -ErrorAction SilentlyContinue
        if (-not (Test-Path $dataDir)) { break }
        Start-Sleep -Milliseconds 600
    }
    if (Test-Path $dataDir) {
        Write-Host "数据目录删不掉，多半还有进程占着：$dataDir" -ForegroundColor Red
        Write-Host "先手工结束 GoXueTimer 进程再重跑。" -ForegroundColor Red
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

    Assert-That ($info['user_version'] -eq '1') "schema 版本 = 1（实际 '$($info['user_version'])'）"

    foreach ($t in @('sessions', 'settings', 'timer_state')) {
        Assert-That (($info['tables'] -split ',') -contains $t) "存在表 $t"
    }

    Assert-That ($info['journal_mode'].ToLower() -eq 'wal') "journal_mode = wal（实际 '$($info['journal_mode'])'）"

    Assert-That ($info['timer_state'] -eq 'idle') "初始计时状态 = idle（实际 '$($info['timer_state'])'）"
}

# ---------------------------------------------------------------- 3. 关窗隐藏

Write-Host "`n[3] 关窗最小化到托盘"
$live = Get-AppProcess | Select-Object -First 1
$live.CloseMainWindow() | Out-Null
Start-Sleep -Seconds 4
$after = Get-AppProcess | Select-Object -First 1
Assert-That ($null -ne $after) '关窗后进程仍然存活（隐藏到托盘）'
if ($after) {
    Assert-That ($after.MainWindowHandle -eq 0) '主窗口已隐藏'
}

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
    $patched = (& $script:python $script:dbHelper disable-minimize-to-tray $dbPath 2>&1) -join ''
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

# ---------------------------------------------------------------- 6. 提醒链路

Write-Host "`n[6] 崩溃恢复 + 休息提醒链路（这一步要等心跳，约 90 秒）"

if ($script:python) {
    Stop-App
    $seeded = (& $script:python $script:dbHelper seed-running $dbPath 1 2>&1) -join ' '
    Assert-That ($seeded -match 'timer_state=running') "已预置一次进行中的会话（$seeded）"

    $proc = Start-App
    $null = Wait-MainWindow

    # 提醒需要三拍心跳：第一拍只用来对齐基准，之后每拍 30 秒。
    $deadline = (Get-Date).AddSeconds(140)
    $reminded = $false
    while ((Get-Date) -lt $deadline) {
        if (Test-LogContains '已发出休息提醒') {
            $reminded = $true
            break
        }
        Start-Sleep -Seconds 3
    }
    Assert-That $reminded '休息提醒已触发（app.log 有「已发出休息提醒」）'
    if ($reminded) {
        $hit = (Get-LogTail 200 | Where-Object { $_ -match '已发出休息提醒' } | Select-Object -Last 1)
        Write-Host "        $hit" -ForegroundColor DarkGray
        Assert-That ($hit -match 'systemToast=true') '系统通知发送成功（未降级为仅应用内提醒）'
    }

    # 恢复分支也应该留下痕迹：会话被接着计时，而不是被丢弃。
    $state = (& $script:python $script:dbHelper inspect $dbPath 2>&1) -join ' '
    Assert-That ($state -match 'timer_state=running') '重启后会话被恢复为运行中'
} else {
    Write-Skip '找不到 python，无法预置计时状态，跳过该场景'
}

Stop-App
Assert-That ((@(Get-AppProcess)).Count -eq 0) '收尾时没有残留进程（残留会占住 timer.db）'

Write-Host "`n测试数据留在：$dataDir" -ForegroundColor DarkGray

# ---------------------------------------------------------------- 汇总

Write-Host "`n=== 结果 ===" -ForegroundColor Cyan
if ($script:failures -eq 0) {
    Write-Host "全部通过（$($script:checks) 项）" -ForegroundColor Green
    exit 0
}
Write-Host "$($script:failures)/$($script:checks) 项失败" -ForegroundColor Red
Write-Host "日志：$logPath"
exit 1
