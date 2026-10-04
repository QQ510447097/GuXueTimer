<#
.SYNOPSIS
    托盘图标生命周期的端到端验证：反复隐藏/显示、退出、多实例下，本应用的图标必须始终只有一个。

.DESCRIPTION
    通知区域里应该始终只有一个属于本应用的图标。会出问题的地方有四类，这里逐条验证：

      1. 重复创建   反复显示/隐藏、反复启动第二个实例，图标数不能涨
      2. 正常退出   走应用自己的退出路径（托盘「退出」等价于 --quit）后，图标必须被删掉
      3. 强制结束   被强杀的进程来不及删图标，会留下"残留图标"——这是 Windows 的既有行为，
                    也正是"通知区域越堆越多"的来源
      4. 多实例     第二个实例应当自行退出，且不额外注册图标

    判定用**差值**而不是绝对值：通知区域里可能已经躺着别的残留条目（比如之前被强杀留下的），
    绝对值不可比；而且 explorer 会在有图标增删时顺手清掉陈旧条目，绝对数还会往下跳。
    脚本会把基线明确报出来，并对每一步断言"相对上一步应该怎么变"。

    依赖 tools/tray_icons.ps1 逐个读图标提示文字来识别归属（只数按钮个数不可靠：
    Windows 10 的折叠区工具条是按需填充的）。

.PARAMETER Exe
    应用可执行文件路径，默认 bin\GuXueTimer.exe。

.PARAMETER Rounds
    反复显示/隐藏的轮数，默认 5。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\tray_lifecycle_test.ps1
#>
[CmdletBinding()]
param(
    [string]$Exe = "",
    [int]$Rounds = 5
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
if (-not $Exe) { $Exe = Join-Path $root 'bin\GuXueTimer.exe' }
$Exe = (Resolve-Path $Exe).Path

$lister = Join-Path $PSScriptRoot 'tray_icons.ps1'
$procName = 'GuXueTimer'

$script:pass = 0
$script:fail = 0
$script:notes = New-Object System.Collections.Generic.List[string]

function Assert-That {
    param([bool]$Condition, [string]$Message)
    if ($Condition) {
        Write-Host "  PASS  $Message" -ForegroundColor Green
        $script:pass++
    } else {
        Write-Host "  FAIL  $Message" -ForegroundColor Red
        $script:fail++
        $script:notes.Add($Message)
    }
}

function Write-Note {
    param([string]$Message)
    Write-Host "        $Message" -ForegroundColor DarkGray
}

# 数出提示文字属于本应用的托盘条目个数。
function Get-TrayCount {
    $out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $lister -Quiet
    $m = [regex]::Match(($out -join ' '), 'mine=(\d+)')
    if (-not $m.Success) { throw "解析托盘计数失败：$out" }
    return [int]$m.Groups[1].Value
}

# 等到图标数满足条件，用于吸收"注册/删除略滞后于进程动作"的时序。
function Wait-TrayCount {
    param([scriptblock]$Predicate, [int]$TimeoutSec = 12)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $last = Get-TrayCount
    while ((Get-Date) -lt $deadline) {
        if (& $Predicate $last) { return $last }
        Start-Sleep -Milliseconds 400
        $last = Get-TrayCount
    }
    return $last
}

function Get-AppProcess { @(Get-Process -Name $procName -ErrorAction SilentlyContinue) }

# 读出日志文件在 From 字节之后新增的内容。
#
# 日志是追加写的 UTF-8，按字节偏移切片足够可靠；只在文件被重建（比 From 短）时
# 退化成读整个文件。
function Get-NewLogLines {
    param([string]$Path, [long]$From = 0)
    if (-not (Test-Path $Path)) { return @() }
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -le $From) { return @() }
    $slice = New-Object byte[] ($bytes.Length - $From)
    [Array]::Copy($bytes, $From, $slice, 0, $slice.Length)
    $text = [System.Text.Encoding]::UTF8.GetString($slice)
    return ($text -split "`r?`n" | Where-Object { $_.Trim() })
}

# 让已有实例走它自己的退出路径（--quit 与托盘菜单的「退出」是同一条路径）。
# 这条路径会 NIM_DELETE，所以不会在通知区域留下残留图标——测试自身也不该制造垃圾。
function Stop-AppGraceful {
    param([int]$TimeoutSec = 15)
    if ((Get-AppProcess).Count -eq 0) { return $true }
    Start-Process -FilePath $Exe -ArgumentList '--quit' | Out-Null
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if ((Get-AppProcess).Count -eq 0) { return $true }
        Start-Sleep -Milliseconds 300
    }
    return $false
}

function Stop-AppHard {
    Get-AppProcess | Stop-Process -Force -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt 20; $i++) {
        if ((Get-AppProcess).Count -eq 0) { return }
        Start-Sleep -Milliseconds 250
    }
}

# 启动并等到主窗口真的出现（"进程存在"不等于"托盘已注册"）。
function Start-App {
    param([int]$TimeoutSec = 25)
    Start-Process -FilePath $Exe | Out-Null
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $p = Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
        if ($p) { return $p }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

# 关闭主窗口（minimizeToTray=true 时只是隐藏，进程留下）。
function Close-MainWindow {
    $p = Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
    if (-not $p) { return $false }
    for ($i = 0; $i -lt 5; $i++) {
        $null = $p.CloseMainWindow()
        Start-Sleep -Milliseconds 500
        $still = Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
        if (-not $still) { return $true }
    }
    return $false
}

# 叫回主窗口：启动第二个实例即可（OnSecondInstanceLaunch 会把窗口显示出来）。
function Show-MainWindow {
    param([int]$TimeoutSec = 10)
    Start-Process -FilePath $Exe | Out-Null
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $p = Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
        if ($p) { return $true }
        Start-Sleep -Milliseconds 300
    }
    return $false
}

if (-not (Test-Path $Exe)) {
    Write-Host "找不到 $Exe，先构建" -ForegroundColor Red
    exit 2
}

Write-Host "托盘图标生命周期测试" -ForegroundColor Cyan
Write-Host "可执行文件：$Exe"
Write-Host "（注意：这个测试会真的启动/关闭应用）`n"

$null = Stop-AppGraceful
Start-Sleep -Seconds 1

# ---------------------------------------------------------------- 基线
Write-Host "[0] 基线"
$base = Get-TrayCount
Write-Note "本应用条目数 = $base"
Write-Note "其中可能包含宿主进程已退出的残留条目；下面一律看相对变化。"

# ---------------------------------------------------------------- 1. 启动
Write-Host "`n[1] 启动后应当出现本应用的图标"
$p = Start-App
Assert-That ($null -ne $p) '主窗口出现'
$afterStart = Wait-TrayCount { param($n) $n -ge 1 }
Write-Note "本应用条目数 = $afterStart（基线 $base）"
Assert-That ($afterStart -ge 1) '通知区域里存在本应用的图标'
Assert-That ((Get-AppProcess).Count -eq 1) '只有一个进程'

# ---------------------------------------------------------------- 2. 关窗隐藏
Write-Host "`n[2] 关窗隐藏到托盘后，图标数不变"
Assert-That (Close-MainWindow) '已向主窗口发出关闭请求且窗口已隐藏'
Assert-That ((Get-AppProcess).Count -eq 1) '进程仍然存活（隐藏到托盘）'
Start-Sleep -Seconds 1
$afterHide = Get-TrayCount
Write-Note "本应用条目数 = $afterHide"
Assert-That ($afterHide -eq $afterStart) "隐藏后条目数没变（$afterStart -> $afterHide）"

# ---------------------------------------------------------------- 3. 反复显示/隐藏
Write-Host "`n[3] 反复显示/隐藏 $Rounds 轮，图标数不能堆积"
$grew = $false
for ($i = 1; $i -le $Rounds; $i++) {
    $shown = Show-MainWindow
    $hidden = Close-MainWindow
    $n = Get-TrayCount
    Write-Note "第 $i 轮：显示=$shown 隐藏=$hidden 条目数=$n"
    if ($n -gt $afterHide) { $grew = $true }
    Start-Sleep -Milliseconds 600
}
$afterRounds = Get-TrayCount
Write-Note "本应用条目数 = $afterRounds"
Assert-That (-not $grew) "$Rounds 轮里条目数一次都没有超过 $afterHide"
Assert-That ($afterRounds -le $afterHide) "折腾完条目数没有堆积（$afterHide -> $afterRounds）"
Assert-That ((Get-AppProcess).Count -eq 1) '始终只有一个进程（单实例锁生效）'

# ---------------------------------------------------------------- 4. 从隐藏状态优雅退出
Write-Host "`n[4] 从托盘隐藏状态优雅退出后，图标必须被删除"
# 上一轮结束时窗口本来就是隐藏的，这里不再要求"关窗成功"，只确认状态。
if ((Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 }).Count -gt 0) {
    $null = Close-MainWindow
}
Assert-That ((Get-AppProcess).Count -eq 1) '隐藏状态下进程仍在'
$beforeQuit = Get-TrayCount
Write-Note "本应用条目数 = $beforeQuit"

# 判据用日志而不是图标个数。
#
# 为什么不数图标：explorer 的托盘工具条**删除**按钮是惰性的——图标已经 NIM_DELETE 了，
# 工具条上的条目还可能挂一会儿才消失，实测同一套操作时而 -1 时而 0，不能作为断言依据。
# 而"到底有没有真的删"这件事，应用在删的那一刻会打一条固定措辞的日志；
# 它出现就说明 NIM_DELETE 确实执行了，不出现就是没执行。
$logPath = Join-Path $env:LOCALAPPDATA 'GuXueTimer\app.log'
$logBefore = if (Test-Path $logPath) { (Get-Item $logPath).Length } else { 0 }

Assert-That (Stop-AppGraceful) '--quit 之后进程已退出（与托盘「退出」同一条路径）'
Start-Sleep -Seconds 1
$newLog = Get-NewLogLines -Path $logPath -From $logBefore
$tornDown = ($newLog | Where-Object { $_ -match '已摘掉托盘图标' }).Count -gt 0
Write-Note '退出期间的新日志：'
foreach ($line in $newLog) { Write-Note "  $line" }
Assert-That $tornDown '退出时执行了托盘清理（日志里有「已摘掉托盘图标」）'

$afterQuit = Get-TrayCount
Write-Note "本应用条目数 = $afterQuit（仅供参考，工具条删除是惰性的）"

# ---------------------------------------------------------------- 5. 强杀对照
Write-Host "`n[5] 对照：强制结束不会执行托盘清理（残留图标的来源）"
$p = Start-App
Assert-That ($null -ne $p) '再次启动'
$beforeKill = Wait-TrayCount { param($n) $n -ge 1 }
Write-Note "本应用条目数 = $beforeKill"

$logBefore = if (Test-Path $logPath) { (Get-Item $logPath).Length } else { 0 }
Stop-AppHard
Start-Sleep -Seconds 2
$newLog = Get-NewLogLines -Path $logPath -From $logBefore
$tornDown = ($newLog | Where-Object { $_ -match '已摘掉托盘图标' }).Count -gt 0
Assert-That (-not $tornDown) '强杀后没有托盘清理日志（图标就留在通知区域里了）'

$afterKill = Get-TrayCount
Write-Note "本应用条目数 = $afterKill"
Assert-That ($afterKill -ge $beforeKill) '强杀后条目仍然留在通知区域'
Write-Note '强杀（任务管理器「结束任务」、Stop-Process -Force）不会走 NIM_DELETE，'
Write-Note '这些残留条目会在鼠标扫过通知区域（或打开「隐藏的图标」面板）后被系统清掉。'
Write-Note '要优雅地关掉正在运行的实例（包括它藏在托盘里的时候），用：GuXueTimer.exe --quit'

Stop-AppHard

Write-Host ""
if ($script:fail -gt 0) {
    Write-Host "未通过 $($script:fail) 项，通过 $($script:pass) 项" -ForegroundColor Red
    foreach ($n in $script:notes) { Write-Host "  - $n" -ForegroundColor Red }
    exit 1
}
Write-Host "全部通过（$($script:pass) 项）" -ForegroundColor Green
