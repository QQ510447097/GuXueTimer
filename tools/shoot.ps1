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
    想验证最小尺寸下的布局：-Size 560x680

.PARAMETER Dark
    先切到深色模式再截（通过页面里的主题选择器切，不改数据库）。

.EXAMPLE
    go build -tags production -ldflags="-w -s -H windowsgui" -o bin\GuXueTimer.exe .
    powershell -ExecutionPolicy Bypass -File tools\shoot.ps1
    powershell -ExecutionPolicy Bypass -File tools\shoot.ps1 -Size 560x680 -Views timer,records
#>
[CmdletBinding()]
param(
    [string]$Views = "timer,records,plans,settings",
    [string]$Size = "",
    [switch]$Dark,
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
#   - 任何一屏都不许出现横向滚动条（窗口最小 560x680，内容区只有约 448px 宽）
#   - 每屏都应当有玻璃面板，否则说明视图没渲染出来
$failures = New-Object System.Collections.Generic.List[string]

foreach ($v in $report.views) {
    $problems = New-Object System.Collections.Generic.List[string]
    if (-not $v.clicked) { $problems.Add('导航项点不到') }
    if ($v.clicked -and $v.activeView -ne $v.requested) { $problems.Add("点击后停在了 '$($v.activeView)'") }
    if ($v.overflowX) { $problems.Add("有横向滚动条（内容 $($v.scrollWidth) > 视口 $($v.innerWidth)）") }
    if ($v.panelCount -lt 1) { $problems.Add('一个 .gx-glass 面板都没有，视图可能没渲染') }

    $ok = $problems.Count -eq 0
    $status = if ($ok) { 'PASS' } else { 'FAIL' }
    $color = if ($ok) { 'Green' } else { 'Red' }
    Write-Host "  [$status] 视图 $($v.requested) -> 实际 '$($v.activeView)'" -ForegroundColor $color
    Write-Host "         视口 $($v.innerWidth)x$($v.innerHeight) / 内容 $($v.scrollWidth)x$($v.scrollHeight)"
    Write-Host "         主题 $($v.themeClass) / 玻璃面板 $($v.panelCount) 个 / backdrop-filter: $($v.backdropFilter)"
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
