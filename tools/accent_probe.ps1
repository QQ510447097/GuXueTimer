<#
.SYNOPSIS
    从外部改目标窗口的 accent 状态（SetWindowCompositionAttribute）。

.DESCRIPTION
    用途：定位"活动窗口的标题栏文字压着一块深色方块"这个问题的成因。

    Windows 10 上 Wails 的窗口毛玻璃走的是
        SetWindowCompositionAttribute(WCA_ACCENT_POLICY, ACCENT_ENABLE_BLURBEHIND)
    （AccentFlags / GradientColor 全为 0）。这个调用**任何进程**都能对别的窗口做，
    所以可以在不重新构建应用的情况下，逐个候选状态做对照：改一次、抓一次标题栏，
    看深色方块跟不跟着变。

    AccentState 取值：
      0  ACCENT_DISABLED                 关掉（对照基准：应当完全没有方块）
      3  ACCENT_ENABLE_BLURBEHIND        Wails 现在用的
      4  ACCENT_ENABLE_ACRYLICBLURBEHIND 亚克力，需要 GradientColor 带 alpha
      5  ACCENT_ENABLE_HOSTBACKDROP      宿主背景（Mica 一类）

.PARAMETER State
    AccentState，见上。

.PARAMETER Flags
    AccentFlags。已知有讲究：0x20|0x40|0x80|0x100 = 画四边；2 = 更强的模糊。

.PARAMETER Gradient
    GradientColor，ABGR 打包的 32 位整数（如 0xCC1A1A1A）。亚克力需要它的 alpha 非 0。

.PARAMETER ProcessName
    目标进程名，默认 GuXueTimer。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\accent_probe.ps1 -State 0
    powershell -ExecutionPolicy Bypass -File tools\accent_probe.ps1 -State 4 -Gradient 0xCC1A1A1A
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][int]$State,
    [int]$Flags = 0,
    [int]$Gradient = 0,
    [string]$ProcessName = 'GuXueTimer'
)

$ErrorActionPreference = 'Stop'

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class Accent
{
    [StructLayout(LayoutKind.Sequential)]
    public struct ACCENT_POLICY
    {
        public int AccentState;
        public int AccentFlags;
        public int GradientColor;
        public int AnimationId;
    }

    // WINDOWCOMPOSITIONATTRIBDATA { DWORD Attrib; PVOID pvData; SIZE_T cbData; }
    // SIZE_T 是 uintptr，写成 int 会让结构体在 64 位下错位。
    [StructLayout(LayoutKind.Sequential)]
    public struct WINDOWCOMPOSITIONATTRIBDATA
    {
        public int Attrib;
        public IntPtr PvData;
        public UIntPtr CbData;
    }

    [DllImport("user32.dll")]
    public static extern int SetWindowCompositionAttribute(IntPtr hwnd, ref WINDOWCOMPOSITIONATTRIBDATA data);

    /// WCA_ACCENT_POLICY = 19
    public const int WCA_ACCENT_POLICY = 19;

    public static int Apply(IntPtr hwnd, int state, int flags, int gradient)
    {
        var accent = new ACCENT_POLICY
        {
            AccentState = state,
            AccentFlags = flags,
            GradientColor = gradient,
            AnimationId = 0,
        };
        var data = new WINDOWCOMPOSITIONATTRIBDATA
        {
            Attrib = WCA_ACCENT_POLICY,
            PvData = Marshal.AllocHGlobal(Marshal.SizeOf(typeof(ACCENT_POLICY))),
            CbData = (UIntPtr)Marshal.SizeOf(typeof(ACCENT_POLICY)),
        };
        try
        {
            Marshal.StructureToPtr(accent, data.PvData, false);
            return SetWindowCompositionAttribute(hwnd, ref data);
        }
        finally
        {
            Marshal.FreeHGlobal(data.PvData);
        }
    }
}
'@

$proc = Get-Process -Name $ProcessName -ErrorAction SilentlyContinue |
    Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $proc) {
    Write-Host "找不到 $ProcessName 的可见主窗口" -ForegroundColor Red
    exit 2
}

$hwnd = $proc.MainWindowHandle
$ok = [Accent]::Apply($hwnd, $State, $Flags, $Gradient)
Write-Host "hwnd=$hwnd  AccentState=$State  AccentFlags=$Flags  GradientColor=0x$('{0:X8}' -f $Gradient)  返回=$ok"
