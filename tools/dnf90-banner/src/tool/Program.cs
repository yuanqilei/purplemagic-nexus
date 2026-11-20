// =============================================================================
//  Program.cs —— 命令行入口
// =============================================================================
//  用法：
//      DnfBannerTool scan                       扫描并打印命中
//      DnfBannerTool patch  --text "新文字"      扫到就替换（最多等 30 秒）
//      DnfBannerTool watch  --interval 50        持续轮询
//  可选参数：
//      --process DNF                            指定进程名（默认 DNF）
// =============================================================================

using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

namespace DnfBannerTool;

internal static class Program
{
    /// <summary>默认替换文本。</summary>
    public const string DefaultText = "公益90欢迎您的到来~";

    /// <summary>锚点：横幅里的域名，首字符 'p' 在 UTF-16 中文里出现概率低。</summary>
    public const string Anchor = "po.mansus.cc";

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr OpenProcess(uint access, bool inherit, int pid);

    [DllImport("kernel32.dll")]
    private static extern bool CloseHandle(IntPtr handle);

    private const uint ProcessQueryInformation = 0x0400;
    private const uint ProcessVmRead = 0x0010;
    private const uint ProcessVmWrite = 0x0020;
    private const uint ProcessVmOperation = 0x0008;

    private static int Main(string[] args)
    {
        Console.OutputEncoding = Encoding.UTF8;

        string command = args.Length > 0 ? args[0].ToLowerInvariant() : "scan";
        string processName = "DNF";
        string text = DefaultText;
        int interval = 50;

        for (int i = 1; i < args.Length; i++)
        {
            switch (args[i])
            {
                case "--process": processName = args[++i]; break;
                case "--text": text = args[++i]; break;
                case "--interval": interval = int.Parse(args[++i]); break;
            }
        }

        if (command is not ("scan" or "patch" or "watch"))
            return Usage();

        Console.WriteLine("==============================================");
        Console.WriteLine(" DNF90 横幅文字工具 (.NET 10)");
        Console.WriteLine($" 命令 : {command}");
        Console.WriteLine($" 替换 : {text}");
        Console.WriteLine("==============================================");

        Process? proc = Process.GetProcessesByName(processName).FirstOrDefault();
        if (proc == null)
        {
            Console.WriteLine($"[错误] 找不到进程 {processName}.exe");
            return 2;
        }

        IntPtr handle = OpenProcess(
            ProcessQueryInformation | ProcessVmRead | ProcessVmWrite | ProcessVmOperation,
            false, proc.Id);

        if (handle == IntPtr.Zero)
        {
            Console.WriteLine($"[错误] OpenProcess 失败，错误码 {Marshal.GetLastWin32Error()}");
            Console.WriteLine("       请以管理员身份运行。");
            return 3;
        }

        Console.WriteLine($"目标 : {proc.ProcessName}.exe  PID={proc.Id}");

        try
        {
            var engine = new BannerEngine(handle, proc, text);

            return command switch
            {
                "scan" => engine.ScanOnce() > 0 ? 0 : 1,
                "patch" => engine.PatchUntilFound(30) ? 0 : 1,
                "watch" => engine.Watch(interval),
                _ => Usage()
            };
        }
        finally
        {
            CloseHandle(handle);
        }
    }

    private static int Usage()
    {
        Console.WriteLine("用法:");
        Console.WriteLine("  DnfBannerTool scan                        扫描并列出命中");
        Console.WriteLine("  DnfBannerTool patch  [--text 文本]         扫到就替换（最多等 30 秒）");
        Console.WriteLine("  DnfBannerTool watch  [--interval 50]       持续轮询");
        Console.WriteLine("  --process 进程名                           默认 DNF");
        return 1;
    }
}
