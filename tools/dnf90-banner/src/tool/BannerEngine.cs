// =============================================================================
//  BannerEngine.cs —— 横幅字符串的定位与替换引擎
// =============================================================================
//  定位流程：
//      1) 枚举目标进程中"已提交 + 可写 + 私有（堆）"的内存页；
//      2) 分块读入，先找锚点 "po.mansus.cc" 的 UTF-16 字节；
//      3) 命中后回读锚点前 64 字节做前缀校验：
//            前 60 字节 == 整串前缀  -> 替换 84 字节
//            前 44 字节 == 正文前缀  -> 替换 68 字节
//      4) VirtualProtectEx 放开写权限，WriteProcessMemory 写入等长内容。
//
//  所有替换都是"等长"的：新文字不足时补空格，保证绝不越界写。
// =============================================================================

using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

namespace DnfBannerTool;

internal sealed class BannerEngine
{
    // ---- Win32 常量 ----
    private const uint MemCommit = 0x1000;
    private const uint PageNoAccess = 0x01;
    private const uint PageGuard = 0x100;
    private const uint PageReadWrite = 0x04;
    private const uint PageWriteCopy = 0x08;
    private const uint PageExecuteReadWrite = 0x40;
    private const uint PageExecuteWriteCopy = 0x80;
    private const uint MemPrivate = 0x20000;

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool ReadProcessMemory(IntPtr h, IntPtr addr, byte[] buf, IntPtr size, out IntPtr read);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool WriteProcessMemory(IntPtr h, IntPtr addr, byte[] buf, IntPtr size, out IntPtr written);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr VirtualQueryEx(IntPtr h, IntPtr addr, out Mbi64 mbi, IntPtr len);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool VirtualProtectEx(IntPtr h, IntPtr addr, IntPtr size, uint newProtect, out uint oldProtect);

    [StructLayout(LayoutKind.Sequential)]
    private struct Mbi64
    {
        public ulong BaseAddress;
        public ulong AllocationBase;
        public uint AllocationProtect;
        public uint Alignment1;
        public ulong RegionSize;
        public uint State;
        public uint Protect;
        public uint Type;
        public uint Alignment2;
    }

    // ---- 运行参数 ----
    private readonly IntPtr _handle;
    private readonly Process _process;

    private readonly byte[] _anchor;

    private readonly byte[] _fullPreFullWidth;   // 整串前缀（全角标点）
    private readonly byte[] _fullPreAscii;       // 整串前缀（半角标点）
    private readonly byte[] _bodyPreFullWidth;   // 正文前缀（全角标点）
    private readonly byte[] _bodyPreAscii;       // 正文前缀（半角标点）

    private readonly byte[] _fullReplacement;    // 84 字节 = 42 个 UTF-16 字符
    private readonly byte[] _bodyReplacement;    // 68 字节 = 34 个 UTF-16 字符

    public BannerEngine(IntPtr handle, Process process, string text)
    {
        _handle = handle;
        _process = process;

        _anchor = Encoding.Unicode.GetBytes(Program.Anchor);

        _fullPreFullWidth = Encoding.Unicode.GetBytes("【90轻装上阵】开源免费，请勿付费购买。谨防收费开服。来源：");
        _fullPreAscii = Encoding.Unicode.GetBytes("【90轻装上阵】开源免费,请勿付费购买。谨防收费开服。来源:");
        _bodyPreFullWidth = Encoding.Unicode.GetBytes("开源免费，请勿付费购买。谨防收费开服。来源：");
        _bodyPreAscii = Encoding.Unicode.GetBytes("开源免费,请勿付费购买。谨防收费开服。来源:");

        _fullReplacement = Fit(text, 84);
        _bodyReplacement = Fit(StripBrackets(text), 68);
    }

    // -------------------------------------------------------------------------
    //  对外接口
    // -------------------------------------------------------------------------

    /// <summary>扫描一轮，返回命中数量。</summary>
    public int ScanOnce()
    {
        int hits = 0;
        foreach (var (addr, size) in EnumerateRegions())
            hits += ScanRegion(addr, size, patch: false);

        Console.WriteLine($"[扫描] 命中 {hits} 处");
        return hits;
    }

    /// <summary>反复扫描直到替换成功（横幅只显示几秒时用）。</summary>
    public bool PatchUntilFound(int timeoutSeconds)
    {
        Console.WriteLine($"[提示] 请在 {timeoutSeconds} 秒内进入赛利亚房间，让横幅弹出来…");
        DateTime deadline = DateTime.Now.AddSeconds(timeoutSeconds);

        while (DateTime.Now < deadline)
        {
            int hits = 0;
            foreach (var (addr, size) in EnumerateRegions())
                hits += ScanRegion(addr, size, patch: true);

            if (hits > 0)
            {
                Console.WriteLine($"[完成] 已替换 {hits} 处");
                return true;
            }
            Thread.Sleep(20);
        }

        Console.WriteLine("[超时] 未找到横幅字符串：可能是图片，或文字不在内存中。");
        return false;
    }

    /// <summary>持续轮询替换。</summary>
    public int Watch(int intervalMs)
    {
        Console.WriteLine($"[监视] 每 {intervalMs} ms 扫描一次，Ctrl+C 结束…");
        while (true)
        {
            foreach (var (addr, size) in EnumerateRegions())
                ScanRegion(addr, size, patch: true);

            Thread.Sleep(intervalMs);
        }
    }

    // -------------------------------------------------------------------------
    //  区域枚举
    // -------------------------------------------------------------------------

    /// <summary>返回所有"可写 + 私有 + 已提交"的内存段（即堆）。</summary>
    private IEnumerable<(ulong addr, ulong size)> EnumerateRegions()
    {
        ulong addr = 0x10000;
        const ulong max = 0x7FFF0000;

        while (addr < max)
        {
            if (VirtualQueryEx(_handle, (IntPtr)(long)addr, out Mbi64 mbi,
                    (IntPtr)Marshal.SizeOf<Mbi64>()) == IntPtr.Zero)
                yield break;

            ulong next = mbi.BaseAddress + mbi.RegionSize;

            bool readable = mbi.State == MemCommit
                            && (mbi.Protect & PageNoAccess) == 0
                            && (mbi.Protect & PageGuard) == 0;

            bool writable = (mbi.Protect & (PageReadWrite | PageWriteCopy |
                                            PageExecuteReadWrite | PageExecuteWriteCopy)) != 0;

            if (readable && writable && mbi.Type == MemPrivate && mbi.RegionSize > 256)
                yield return (mbi.BaseAddress, mbi.RegionSize);

            if (next <= addr) yield break;
            addr = next;
        }
    }

    // -------------------------------------------------------------------------
    //  单段扫描
    // -------------------------------------------------------------------------

    private int ScanRegion(ulong baseAddr, ulong size, bool patch)
    {
        const int chunk = 1 << 20;          // 每次读 1MB
        int overlap = _anchor.Length - 1;    // 跨块时保留的尾巴

        byte[] buf = new byte[chunk + 256];
        int hits = 0;
        int carry = 0;
        ulong offset = 0;

        while (offset < size)
        {
            int want = (int)Math.Min((ulong)chunk, size - offset);
            ReadProcessMemory(_handle, (IntPtr)(long)(baseAddr + offset), buf, (IntPtr)want, out IntPtr read);
            int got = (int)read;
            int total = carry + got;
            if (total <= 0) break;

            int pos = 0;
            while (true)
            {
                int idx = IndexOf(buf, total, _anchor, pos);
                if (idx < 0) break;

                ulong abs = baseAddr + offset - (ulong)carry + (ulong)idx;
                hits += HandleHit(abs, patch);
                pos = idx + 1;
            }

            if (got <= 0) break;

            if (overlap > 0 && total > overlap)
            {
                Array.Copy(buf, total - overlap, buf, 0, overlap);
                carry = overlap;
            }
            else carry = 0;

            offset += (ulong)got;
        }

        return hits;
    }

    /// <summary>校验锚点前面的前缀，决定替换整串还是正文。</summary>
    private int HandleHit(ulong anchorAddr, bool patch)
    {
        if (anchorAddr < 64) return 0;

        byte[] ctx = new byte[160];
        if (!ReadProcessMemory(_handle, (IntPtr)(long)(anchorAddr - 64), ctx, (IntPtr)ctx.Length, out IntPtr read))
            return 0;
        if ((int)read < 64 + _anchor.Length) return 0;

        if (Match(ctx, 4, _fullPreFullWidth) || Match(ctx, 4, _fullPreAscii))
        {
            ulong target = anchorAddr - 60;
            Console.WriteLine($"[命中] 整串 @0x{target:X}  模块={ModuleOf(target)}");
            if (patch && WriteRaw(target, _fullReplacement))
            {
                Console.WriteLine($"[替换] 整串 @0x{target:X} -> {Encoding.Unicode.GetString(_fullReplacement).TrimEnd()}");
                return 1;
            }
            return 0;
        }

        if (Match(ctx, 20, _bodyPreFullWidth) || Match(ctx, 20, _bodyPreAscii))
        {
            ulong target = anchorAddr - 44;
            Console.WriteLine($"[命中] 正文 @0x{target:X}  模块={ModuleOf(target)}");
            if (patch && WriteRaw(target, _bodyReplacement))
            {
                Console.WriteLine($"[替换] 正文 @0x{target:X} -> {Encoding.Unicode.GetString(_bodyReplacement).TrimEnd()}");
                return 1;
            }
            return 0;
        }

        return 0;
    }

    // -------------------------------------------------------------------------
    //  工具函数
    // -------------------------------------------------------------------------

    /// <summary>去掉首尾的【】，避免正文段落出现双括号。</summary>
    private static string StripBrackets(string s)
    {
        if (s.StartsWith("【", StringComparison.Ordinal)) s = s[1..];
        if (s.EndsWith("】", StringComparison.Ordinal)) s = s[..^1];
        return s;
    }

    /// <summary>压成固定长度的 UTF-16 缓冲，不足补空格。</summary>
    private static byte[] Fit(string text, int limit)
    {
        byte[] raw = Encoding.Unicode.GetBytes(text);
        byte[] buf = new byte[limit];
        int n = Math.Min(raw.Length, limit);
        Array.Copy(raw, buf, n);
        for (int i = n; i + 1 < limit; i += 2)
        {
            buf[i] = 0x20;
            buf[i + 1] = 0x00;
        }
        return buf;
    }

    private static bool Match(byte[] ctx, int offset, byte[] prefix)
    {
        if (offset + prefix.Length > ctx.Length) return false;
        for (int i = 0; i < prefix.Length; i++)
            if (ctx[offset + i] != prefix[i]) return false;
        return true;
    }

    /// <summary>写入等长字节（临时放开写保护）。</summary>
    private bool WriteRaw(ulong addr, byte[] data)
    {
        if (!VirtualProtectEx(_handle, (IntPtr)(long)addr, (IntPtr)data.Length, PageReadWrite, out uint old))
            return false;

        bool ok = WriteProcessMemory(_handle, (IntPtr)(long)addr, data, (IntPtr)data.Length, out IntPtr written);
        VirtualProtectEx(_handle, (IntPtr)(long)addr, (IntPtr)data.Length, old, out _);

        return ok && written.ToInt64() == data.Length;
    }

    private static int IndexOf(byte[] hay, int hayLen, byte[] needle, int start)
    {
        byte first = needle[0];
        int end = hayLen - needle.Length;
        for (int i = start; i <= end; i++)
        {
            if (hay[i] != first) continue;
            int j = 1;
            while (j < needle.Length && hay[i + j] == needle[j]) j++;
            if (j == needle.Length) return i;
        }
        return -1;
    }

    /// <summary>查地址属于哪个模块。</summary>
    private string ModuleOf(ulong addr)
    {
        try
        {
            foreach (ProcessModule m in _process.Modules)
            {
                ulong b = (ulong)m.BaseAddress.ToInt64();
                if (addr >= b && addr < b + (ulong)m.ModuleMemorySize)
                    return $"{m.ModuleName}+0x{addr - b:X}";
            }
        }
        catch { /* 受保护进程可能拒绝枚举模块 */ }
        return "?";
    }
}
