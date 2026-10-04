<#
.SYNOPSIS
    列出通知区域（系统托盘）里每个图标的提示文字，并数出属于本应用的有几个。

.DESCRIPTION
    为什么不能只数按钮个数：Windows 10 的"隐藏的图标"折叠面板里那个工具条是**按需填充**的，
    没打开过面板时 TB_BUTTONCOUNT 会给出一个偏小的数，于是会出现"启动后图标没增加、
    折腾几轮反而增加了"这种自相矛盾的读数。个数不可信。

    所以这里逐个读出按钮的文本（托盘按钮的文本就是它的 szTip 提示文字），
    按文字判断归属。"有几个是本应用的"才是有意义的指标。

    跨进程读文本用的是 MSDN 规定的那套：在 explorer 里 VirtualAllocEx 一块缓冲区，
    对它发 TB_GETBUTTONTEXTW 把字符串写进那块缓冲区，再用 ReadProcessMemory 取回来。

    两个必须知道的局限（都踩过）：

    1. 必须用 TB_GETBUTTONTEXT**W**（WM_USER+75）。WM_USER+45 是 A 变体，
       提示文字里全是中文，用 A 变体读回来是一串 ANSI 字节，按 UTF-16 解码就是乱码。
    2. 折叠面板里的条目**不代表当前真的显示着这么多图标**。面板没被打开过时，
       工具条上可能还留着宿主进程已经退出的陈旧条目，而 Windows 要等到鼠标扫过
       通知区域（或打开折叠面板）才会把死掉的图标真正抹掉。
       所以这个脚本适合看**相对变化**（启动前后、退出前后差了几个），
       不适合断言"屏幕上现在有几个"。

    这个脚本是只读的：不改设置、不点图标、不动鼠标。

.PARAMETER Match
    判定为本应用的提示文字前缀，默认取"孤雪学习计时器"。

.PARAMETER Quiet
    只输出一行机器可读的计数（mine=N），供脚本取用。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\tray_icons.ps1
    powershell -ExecutionPolicy Bypass -File tools\tray_icons.ps1 -Quiet
#>
[CmdletBinding()]
param(
    [string]$Match = "孤雪学习计时器",
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'

Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

public static class TrayLister
{
    [DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern IntPtr FindWindow(string lpClassName, string lpWindowName);

    [DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern IntPtr FindWindowEx(IntPtr hWndParent, IntPtr hWndChildAfter, string lpszClass, string lpszWindow);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    public static extern IntPtr SendMessage(IntPtr hWnd, uint Msg, IntPtr wParam, IntPtr lParam);

    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern IntPtr OpenProcess(uint dwDesiredAccess, bool bInheritHandle, uint dwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool CloseHandle(IntPtr hObject);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern IntPtr VirtualAllocEx(IntPtr hProcess, IntPtr lpAddress, IntPtr dwSize, uint flAllocationType, uint flProtect);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool VirtualFreeEx(IntPtr hProcess, IntPtr lpAddress, IntPtr dwSize, uint dwFreeType);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool ReadProcessMemory(IntPtr hProcess, IntPtr lpBaseAddress, byte[] lpBuffer, IntPtr nSize, out IntPtr lpNumberOfBytesRead);

    const uint TB_BUTTONCOUNT = 0x0400 + 24;
    // 必须用 W 变体（WM_USER+75）；WM_USER+45 是 A 变体，中文会变乱码。
    const uint TB_GETBUTTONTEXTW = 0x0400 + 75;
    const uint PROCESS_VM_OPERATION = 0x0008;
    const uint PROCESS_VM_READ = 0x0010;
    const uint PROCESS_VM_WRITE = 0x0020;
    const uint PROCESS_QUERY_INFORMATION = 0x0400;
    const uint MEM_COMMIT = 0x1000;
    const uint MEM_RESERVE = 0x2000;
    const uint MEM_RELEASE = 0x8000;
    const uint PAGE_READWRITE = 0x04;

    static IntPtr Visible()
    {
        IntPtr shell = FindWindow("Shell_TrayWnd", null);
        if (shell == IntPtr.Zero) return IntPtr.Zero;
        IntPtr notify = FindWindowEx(shell, IntPtr.Zero, "TrayNotifyWnd", null);
        if (notify == IntPtr.Zero) return IntPtr.Zero;
        IntPtr pager = FindWindowEx(notify, IntPtr.Zero, "SysPager", null);
        if (pager == IntPtr.Zero) return IntPtr.Zero;
        return FindWindowEx(pager, IntPtr.Zero, "ToolbarWindow32", null);
    }

    static IntPtr Overflow()
    {
        IntPtr overflow = FindWindow("NotifyIconOverflowWindow", null);
        if (overflow == IntPtr.Zero) return IntPtr.Zero;
        IntPtr toolbar = FindWindowEx(overflow, IntPtr.Zero, "ToolbarWindow32", null);
        if (toolbar != IntPtr.Zero) return toolbar;
        IntPtr mid = FindWindowEx(overflow, IntPtr.Zero, "SysPager", null);
        if (mid == IntPtr.Zero) return IntPtr.Zero;
        return FindWindowEx(mid, IntPtr.Zero, "ToolbarWindow32", null);
    }

    /// 读一个工具条上所有按钮的文本。整个工具条读不到时返回 null（而不是空数组）。
    static string[] ReadToolbar(IntPtr toolbar)
    {
        if (toolbar == IntPtr.Zero) return null;

        int count = (int)SendMessage(toolbar, TB_BUTTONCOUNT, IntPtr.Zero, IntPtr.Zero);
        if (count < 0) return null;
        if (count == 0) return new string[0];

        uint pid;
        GetWindowThreadProcessId(toolbar, out pid);
        if (pid == 0) return null;

        IntPtr proc = OpenProcess(PROCESS_VM_OPERATION | PROCESS_VM_READ | PROCESS_VM_WRITE | PROCESS_QUERY_INFORMATION, false, pid);
        if (proc == IntPtr.Zero) return null;

        IntPtr remote = IntPtr.Zero;
        try
        {
            const int bufSize = 4096;
            remote = VirtualAllocEx(proc, IntPtr.Zero, (IntPtr)bufSize, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
            if (remote == IntPtr.Zero) return null;

            var list = new List<string>();
            for (int i = 0; i < count; i++)
            {
                // 返回字符数（不含结尾 NUL）；-1 表示这个按钮没有文本。
                long len = (long)SendMessage(toolbar, TB_GETBUTTONTEXTW, (IntPtr)i, remote);
                if (len <= 0) { list.Add(""); continue; }

                int bytes = (int)Math.Min(len, 2047) * 2;
                byte[] local = new byte[bytes];
                IntPtr read;
                if (!ReadProcessMemory(proc, remote, local, (IntPtr)bytes, out read) || (long)read < 2)
                {
                    list.Add("");
                    continue;
                }
                list.Add(Encoding.Unicode.GetString(local, 0, (int)read).TrimEnd('\0'));
            }
            return list.ToArray();
        }
        finally
        {
            if (remote != IntPtr.Zero) VirtualFreeEx(proc, remote, IntPtr.Zero, MEM_RELEASE);
            CloseHandle(proc);
        }
    }

    /// 返回提示文字数组，区域名通过 out 参数一并给出。
    public static string[] Collect(out string[] areas)
    {
        var texts = new List<string>();
        var where = new List<string>();

        string[] names = new string[] { "可见区", "折叠区" };
        IntPtr[] bars = new IntPtr[] { Visible(), Overflow() };

        for (int i = 0; i < bars.Length; i++)
        {
            string[] got = ReadToolbar(bars[i]);
            if (got == null) { texts.Add("<工具条读不到>"); where.Add(names[i]); continue; }
            foreach (string t in got) { texts.Add(t); where.Add(names[i]); }
        }

        areas = where.ToArray();
        return texts.ToArray();
    }
}
'@

$areas = $null
$texts = [TrayLister]::Collect([ref]$areas)

$mine = 0
$other = 0
for ($i = 0; $i -lt $texts.Count; $i++) {
    $text = [string]$texts[$i]
    $isMine = $text.StartsWith($Match)
    if ($isMine) { $mine++ } elseif ($text -and $text -ne '<工具条读不到>') { $other++ }
    if ($Quiet) { continue }
    $mark = if ($isMine) { '  <<< 本应用' } else { '' }
    $shown = if ($text) { $text } else { '(无文本)' }
    Write-Host ("  [{0}] {1}{2}" -f $areas[$i], $shown, $mark)
}

if ($Quiet) {
    Write-Host "mine=$mine"
    exit 0
}

Write-Host ""
Write-Host "本应用图标数=$mine  其它=$other  （判定前缀：$Match）"
