<#
.SYNOPSIS
    抓一张**含标题栏**的整窗截图。

.DESCRIPTION
    为什么需要它：CDP 的 Page.captureScreenshot 只能抓 webview 那一块，
    抓不到窗口的非客户区（标题栏、边框）；而早先试过的 CopyFromScreen 在
    DSH 的进程里返回全白（拿不到交互桌面的画面）。

    PrintWindow 则是让**目标窗口自己**把内容画到我们给的内存 DC 上，
    所以不需要窗口可见、也不受遮挡影响。Chromium/WebView2 这类合成窗口
    必须带 PW_RENDERFULLCONTENT(0x2)，否则拿到的是一片黑。

    用途：验收毛玻璃开启后标题栏文字是否正常（这是 CDP 看不到的地方）。

.PARAMETER ProcessName
    进程名，默认 GuXueTimer。

.PARAMETER Out
    输出 PNG 路径。

.PARAMETER CaptionOnly
    只截顶部标题栏那一条（高度由 -CaptionHeight 决定，默认 40px）。
    放大后看文字细节时用它。

.PARAMETER Activate
    抓图前先把窗口切到前台。标题栏的绘制与"是不是活动窗口"有关——
    实测过：非活动窗口抓出来是干净的，而用户看到的是活动窗口的样子。

.PARAMETER NoFullContent
    不带 PW_RENDERFULLCONTENT。用来对照：这个标志走的是"让窗口自己重画"，
    可能与屏幕上 DWM 合成出来的画面不同。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\window_shot.ps1 -Out screenshots\window.png
    powershell -ExecutionPolicy Bypass -File tools\window_shot.ps1 -Activate -CaptionOnly -Out screenshots\caption.png
#>
[CmdletBinding()]
param(
    [string]$ProcessName = 'GuXueTimer',
    [string]$Out = 'screenshots\window.png',
    [switch]$CaptionOnly,
    [int]$CaptionHeight = 40,
    [switch]$Activate,
    [switch]$NoFullContent
)

$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.Drawing

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class WindowShot
{
    [DllImport("user32.dll")]
    public static extern bool PrintWindow(IntPtr hwnd, IntPtr hdcBlt, uint nFlags);

    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);

    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern IntPtr GetForegroundWindow();

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }

    /// PW_RENDERFULLCONTENT：合成窗口（Chromium/WebView2）必须带这个标志，
    /// 否则 PrintWindow 画出来的是黑的。
    public const uint PW_RENDERFULLCONTENT = 0x00000002;
}
'@

function Get-AppProcess { Get-Process -Name $ProcessName -ErrorAction SilentlyContinue }

$proc = Get-AppProcess | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $proc) {
    Write-Host "找不到 $ProcessName 的可见主窗口（隐藏到托盘时先唤回窗口再截图）" -ForegroundColor Red
    exit 2
}

$hwnd = $proc.MainWindowHandle

if ($Activate) {
    [void][WindowShot]::SetForegroundWindow($hwnd)
    Start-Sleep -Milliseconds 600
    $fg = [WindowShot]::GetForegroundWindow()
    $isFg = ($fg -eq $hwnd)
    Write-Host "已切到前台: $isFg （前台句柄 $fg）"
}

$rect = New-Object WindowShot+RECT
if (-not [WindowShot]::GetWindowRect($hwnd, [ref]$rect)) {
    Write-Host "GetWindowRect 失败" -ForegroundColor Red
    exit 1
}

$width = $rect.Right - $rect.Left
$height = $rect.Bottom - $rect.Top
Write-Host "窗口: $width x $height （位置 $($rect.Left),$($rect.Top)）"

$bmp = New-Object System.Drawing.Bitmap -ArgumentList @($width, $height)
$gfx = [System.Drawing.Graphics]::FromImage($bmp)
$hdc = $gfx.GetHdc()
$ok = $false
$flags = if ($NoFullContent) { 0 } else { [WindowShot]::PW_RENDERFULLCONTENT }
try {
    $ok = [WindowShot]::PrintWindow($hwnd, $hdc, $flags)
} finally {
    $gfx.ReleaseHdc($hdc)
    $gfx.Dispose()
}

if (-not $ok) {
    Write-Host "PrintWindow 返回 false（窗口可能不响应 WM_PRINT）" -ForegroundColor Yellow
}

if ($CaptionOnly) {
    $h = [Math]::Min($CaptionHeight, $height)
    # -ArgumentList 不能省：写成 New-Object Bitmap($w, $h) 这种形式时，PowerShell 会把
    # 多个参数当成一个 Object[] 传进去，报 "Cannot convert System.Object[] to UInt32"。
    $crop = New-Object System.Drawing.Bitmap -ArgumentList @($width, $h)
    $cg = [System.Drawing.Graphics]::FromImage($crop)
    $srcRect = New-Object System.Drawing.Rectangle -ArgumentList @(0, 0, $width, $h)
    $cg.DrawImage($bmp, 0, 0, $srcRect, [System.Drawing.GraphicsUnit]::Pixel)
    $cg.Dispose()

    # 放大 3 倍：标题栏文字很小，不放大看不清抗锯齿与底色
    $scale = 3
    $big = New-Object System.Drawing.Bitmap -ArgumentList @(($width * $scale), ($h * $scale))
    $bg = [System.Drawing.Graphics]::FromImage($big)
    $bg.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::NearestNeighbor
    $bg.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::Half
    $bg.DrawImage($crop, 0, 0, $big.Width, $big.Height)
    $bg.Dispose()
    $crop.Dispose()
    $bmp.Dispose()
    $bmp = $big
    Write-Host "已裁剪标题栏并放大 ${scale} 倍: $($big.Width) x $($big.Height)"
}

$dir = Split-Path -Parent $Out
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()

Write-Host "已保存 $Out"
Get-Item $Out | Select-Object @{n='字节';e={$_.Length}} | Format-Table -AutoSize
