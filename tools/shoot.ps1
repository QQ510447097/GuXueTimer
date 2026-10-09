<#
.SYNOPSIS
    启动应用、用 CDP 给界面截图并体检，然后关掉应用。

.DESCRIPTION
    这是"我真的看过界面长什么样"的执行入口。它做四件事：

      1. 用 GX_DEBUG_PORT 打开 WebView2 的远程调试端口（只在设了这个环境变量时开）
      2. 启动 bin\GuXueTimer.exe 并等端口就绪
      3. 调 tools/ui_probe.mjs 切视图、截图、断言布局（横向溢出 / 毛玻璃 / 主题）
      4. 关掉应用，把体检报告打出来

    截图落在 screenshots/ 下。窗口不一定要可见，也不会在你屏幕上反复弹窗。

.PARAMETER Views
    要截的视图，逗号分隔。默认四个都截。

.PARAMETER Size
    视口尺寸，形如 680x820。不传就用窗口自身的尺寸。
    想验证最小尺寸下的布局：-Size 460x620

.PARAMETER Dark
    先切到深色模式再截（通过页面里的主题选择器切，不改数据库）。

.EXAMPLE
    go build -tags production -ldflags="-w -s -H windowsgui" -o bin\GuXueTimer.exe .
    powershell -ExecutionPolicy Bypass -File tools\shoot.ps1
    powershell -ExecutionPolicy Bypass -File tools\shoot.ps1 -Size 460x620 -Views timer,records
#>
[CmdletBinding()]
param(
    [string]$Views = "timer,records,plans,achievements,personalize,settings",
    [string]$Size = "",
    [switch]$Dark,
    [switch]$Act,
    [switch]$RequireNoScroll,
    [string]$Out = "screenshots",
    [int]$Port = 9222,
    [int]$WaitSec = 25,
    [switch]$KeepOpen
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root 'bin\GuXueTimer.exe'
$probe = Join-Path $PSScriptRoot 'ui_probe.mjs'
$processName = 'GuXueTimer'

if (-not (Test-Path $exe)) {
    Write-Host "找不到 $exe —— 先构建" -ForegroundColor Red
    exit 2
}
if (-not (Test-Path $probe)) {
    Write-Host "找不到 $probe" -ForegroundColor Red
    exit 2
}

function Get-AppProcess { Get-Process -Name $processName -ErrorAction SilentlyContinue }

# 结束应用。**优先优雅退出**：用 --quit 让已有实例走它自己的退出路径，
# 那条路径会 NIM_DELETE 掉托盘图标。
#
# 为什么不直接 Stop-Process -Force：强杀不会删托盘图标，会在通知区域留下
# "残留图标"，要等鼠标扫过才被系统清掉。截图脚本一天要跑很多次，
# 每次强杀都在用户的通知区域里堆一个，最后看起来就像"托盘图标越堆越多"。
# 强杀只作为兜底（应用卡死时）。
function Stop-App {
    param([int]$Attempts = 3)
    if (-not (Get-AppProcess)) { return }

    if (Test-Path $exe) {
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

# 找 Node：PATH 里没有就用 DSH 自带的那份。
function Find-Node {
    $cmd = Get-Command node -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $bundled = 'C:\Users\GuDuXue\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\node\bin\node.exe'
    if (Test-Path $bundled) { return $bundled }
    return $null
}

$node = Find-Node
if (-not $node) {
    Write-Host '找不到 node，无法截图。装个 Node 或把 dsh 自带的加进 PATH。' -ForegroundColor Red
    exit 2
}

Stop-App

# 只有设了 GX_DEBUG_PORT，应用才会开调试端口。
$env:GX_DEBUG_PORT = "$Port"
Write-Host "启动应用（调试端口 $Port）..." -ForegroundColor Cyan
$proc = Start-Process -FilePath $exe -PassThru

# 等端口起来。
$ready = $false
$deadline = (Get-Date).AddSeconds($WaitSec)
while ((Get-Date) -lt $deadline) {
    try {
        $null = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/json/version" -UseBasicParsing -TimeoutSec 2
        $ready = $true
        break
    } catch {
        Start-Sleep -Milliseconds 500
    }
}

if (-not $ready) {
    Write-Host "调试端口 $Port 没起来。应用日志：" -ForegroundColor Red
    $log = Join-Path $env:LOCALAPPDATA 'GuXueTimer\app.log'
    if (Test-Path $log) { Get-Content -LiteralPath $log -Tail 15 -Encoding UTF8 | ForEach-Object { "  $_" } }
    Stop-App
    exit 1
}

# 端口起来了，但**不一定是我们启动的那个进程**在服务它。
#
# 应用是单实例的：如果还有别的实例活着，我们刚启动的这个会立刻退出、把窗口交给它，
# 而那个实例可能带着旧的调试端口与旧的前端状态——探针于是连到了"别人的窗口"上，
# 报出来的 activeView 与你请求的视图对不上（实测踩过一次，查了半天）。
# 这里直接把这件事变成一条明确的错误。
if ($proc -and -not (Get-Process -Id $proc.Id -ErrorAction SilentlyContinue)) {
    Write-Host "我们启动的实例立刻退出了（PID $($proc.Id)）——说明还有别的实例在跑，" -ForegroundColor Red
    Write-Host "调试端口是它开的，探针会连到那个窗口上。请先完全退出应用再重跑。" -ForegroundColor Red
    Stop-App
    exit 1
}

Write-Host '端口就绪，开始截图与体检...' -ForegroundColor Cyan

$nodeArgs = @($probe, '--port', "$Port", '--out', (Join-Path $root $Out), '--views', $Views)
$prefix = ''
if ($Dark) { $prefix = 'dark' }
if ($Size -match '^(\d+)x(\d+)$') {
    $nodeArgs += @('--width', $Matches[1], '--height', $Matches[2])
    if (-not $prefix) { $prefix = $Size }
}
if ($prefix) { $nodeArgs += @('--prefix', $prefix) }
# 深色：探测脚本往 <html> 上加 theme-dark 类即可，不动数据库里的主题设置。
if ($Dark) { $nodeArgs += '--dark' }
# 弹窗：截图前先做一次脚本化交互（点开归档确认框 / 切到本月），用来验证浮层里的文字对比度。
if ($Act) { $nodeArgs += '--act' }

$json = & $node @nodeArgs
$exit = $LASTEXITCODE

if (-not $KeepOpen) { Stop-App }

if ($exit -ne 0) {
    Write-Host '截图失败' -ForegroundColor Red
    exit $exit
}

$report = $json | ConvertFrom-Json
Write-Host "`n=== 界面体检 ===" -ForegroundColor Cyan

# 这个脚本同时是一道界面回归检查，不只是"看看图"：
#   - 每个视图都要能点到（导航项存在、点击后真的切过去了）
#   - 任何一屏都不许出现横向滚动条（窗口最小 460x620，内容区只有约 350px 宽）
#   - 每屏都应当有玻璃面板，否则说明视图没渲染出来
#   - -RequireNoScroll 时，页面内容不许超出一屏（用来验收"不用滚就能看全"）
$failures = New-Object System.Collections.Generic.List[string]

foreach ($v in $report.views) {
    $problems = New-Object System.Collections.Generic.List[string]
    if (-not $v.clicked) { $problems.Add('导航项点不到') }
    if ($v.clicked -and $v.activeView -ne $v.requested) { $problems.Add("点击后停在了 '$($v.activeView)'") }
    if ($v.overflowX) { $problems.Add("有横向滚动条（内容 $($v.scrollWidth) > 视口 $($v.innerWidth)）") }
    if ($v.panelCount -lt 1) { $problems.Add('一个 .gx-glass 面板都没有，视图可能没渲染') }
    if ($RequireNoScroll -and $v.contentOverflowY) {
        $problems.Add("页面内容超出一屏（$($v.contentScroll) > $($v.contentClient)）")
    }

    $ok = $problems.Count -eq 0
    $status = if ($ok) { 'PASS' } else { 'FAIL' }
    $color = if ($ok) { 'Green' } else { 'Red' }
    Write-Host "  [$status] 视图 $($v.requested) -> 实际 '$($v.activeView)'" -ForegroundColor $color
    Write-Host "         视口 $($v.innerWidth)x$($v.innerHeight) / 文档 $($v.scrollWidth)x$($v.scrollHeight)"
    # 页面内是嵌套滚动，文档级尺寸看不出"要不要滚"，所以单列一行内容区尺寸
    $scrollNote = if ($v.contentOverflowY) { '要滚动' } else { '一屏放得下' }
    Write-Host "         内容区 $($v.contentClient) 高 / 实际 $($v.contentScroll) 高（$scrollNote）"
    if ($v.barPoints -ge 0) { Write-Host "         柱状图数据点 $($v.barPoints) 根" }
    if ($v.bottom) { Write-Host "         底部区域模式 $($v.bottom)（面板 $($v.bottomBox)，期望 16:9）" }
    Write-Host "         主题 $($v.themeClass) / 玻璃面板 $($v.panelCount) 个 / backdrop-filter: $($v.backdropFilter)"
    if ($null -ne $v.acted) { Write-Host "         截图前交互: $($v.acted)" }
    foreach ($p in $problems) {
        Write-Host "         ! $p" -ForegroundColor Red
        $failures.Add("$($v.requested): $p")
    }
    Write-Host "         截图 $($v.file -replace [regex]::Escape($root + '\'), '')"
}

Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host "界面体检未通过（$($failures.Count) 项）" -ForegroundColor Red
    Write-Host "`n报告 JSON：" -ForegroundColor DarkGray
    $json
    exit 1
}
Write-Host "界面体检全部通过（$($report.views.Count) 屏）" -ForegroundColor Green
Write-Host "`n报告 JSON：" -ForegroundColor DarkGray
$json
exit 0
