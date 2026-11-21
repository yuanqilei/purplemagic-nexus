// =============================================================================
//  ijl15.dll 代理 —— DNF90 客户端横幅文字替换
// =============================================================================
//  目标      : DNF.exe (32 位 / x86)
//  编译      : Visual Studio 2022 BuildTools (x86)，见 build.ps1
//  依赖      : 无（静态链接 CRT，仅用 kernel32 / psapi）
//
//  背景
//  -----------------------------------------------------------------------------
//  DNF.exe 和 Script.pvf 被启动器用 SHA-256 校验，改了会被还原；而 ijl15.dll
//  不在校验清单里，因此用同名代理 DLL 顶替它，在游戏进程内做内存替换。
//
//  DNF.exe 是按【序号】导入 ijl15.dll 的（实测 2 / 3 / 5），所以导出序号必须
//  与原版逐一对齐：1=ijlGetLibVersion 2=ijlInit 3=ijlFree
//                4=ijlRead        5=ijlWrite 6=ijlErrorStr
//
//  横幅特征（实测）
//  -----------------------------------------------------------------------------
//      * 运行时是 UTF-16LE 字符串，位于堆（MEM_PRIVATE）；
//      * 进赛利亚房间时生成，只显示约 5 秒；
//      * 原文用全角标点：
//        【90轻装上阵】开源免费，请勿付费购买。谨防收费开服。来源：po.mansus.cc
//      * 共 42 个字符 = 84 字节；其中正文段 34 个字符 = 68 字节。
//
//  实现要点
//  -----------------------------------------------------------------------------
//      1. 裸 jmp 转发：不产生序言/尾声，寄存器与栈原样交给原版，
//         因此不必关心 stdcall / cdecl；
//      2. 锚点定位："po.mansus.cc" 首字符 'p' 在 UTF-16 中文里很少见，
//         先用 memchr 快速找它，再校验前面 60 / 44 字节的前缀；
//      3. 等长替换：新文字不足时补空格，绝不越界；
//      4. 8 线程 × 1ms：把地址空间切 8 段并行扫描，把延迟压到一帧以内；
//      5. 稳定性：扫描包在 SEH 里（游戏随时可能释放堆），只读可写私有页，
//         写前再 VirtualQuery 复核一次。
// =============================================================================

#define _CRT_SECURE_NO_WARNINGS
#include <windows.h>
#include <psapi.h>
#include <cstdio>
#include <cstdarg>
#include <cstring>
#include <string>
#include <vector>
#include "native_redemption.h"

// =============================================================================
//  一、导出转发
// =============================================================================

// 原版 6 个函数的真实地址
static void* g_fn[6] = { nullptr };

// 裸跳转：直接 jmp 到原版，不碰栈
extern "C" __declspec(naked) void ijlGetLibVersion() { __asm { jmp dword ptr [g_fn + 0] } }
extern "C" __declspec(naked) void ijlInit() { __asm { jmp dword ptr [g_fn + 4] } }
extern "C" __declspec(naked) void ijlFree() { __asm { jmp dword ptr [g_fn + 8] } }
extern "C" __declspec(naked) void ijlRead() { __asm { jmp dword ptr [g_fn + 12] } }
extern "C" __declspec(naked) void ijlWrite() { __asm { jmp dword ptr [g_fn + 16] } }
extern "C" __declspec(naked) void ijlErrorStr() { __asm { jmp dword ptr [g_fn + 20] } }

// 原版加载失败时的兜底：返回 0，避免跳到空指针
extern "C" int StubFn() { return 0; }

// =============================================================================
//  二、基础设施
// =============================================================================

static HMODULE       g_self = nullptr;   // 本模块句柄
static uintptr_t     g_selfBase = 0;     // 本模块基址（扫描时跳过自己）
static size_t        g_selfSize = 0;
static FILE*         g_log = nullptr;    // 日志
static CRITICAL_SECTION g_cs;
static bool          g_csReady = false;
static bool          g_origLoaded = false;

static const int  kThreads = 8;          // 并行扫描线程数
static int        g_intervalMs = 1;      // 每轮扫描后的休眠

// 带毫秒时间戳的日志
static void Log(const char* fmt, ...)
{
    if (!g_log) return;
    SYSTEMTIME st; GetLocalTime(&st);
    if (g_csReady) EnterCriticalSection(&g_cs);
    fprintf(g_log, "[%02d:%02d:%02d.%03d] ", st.wHour, st.wMinute, st.wSecond, st.wMilliseconds);
    va_list ap; va_start(ap, fmt); vfprintf(g_log, fmt, ap); va_end(ap);
    fprintf(g_log, "\n"); fflush(g_log);
    if (g_csReady) LeaveCriticalSection(&g_cs);
}

// "本 DLL 所在目录 + 文件名"
static std::wstring ModuleDirPath(const wchar_t* name)
{
    wchar_t buf[MAX_PATH]{};
    GetModuleFileNameW(g_self, buf, MAX_PATH);
    wchar_t* slash = wcsrchr(buf, L'\\');
    if (slash) *(slash + 1) = L'\0'; else buf[0] = L'\0';
    return std::wstring(buf) + name;
}

static std::wstring Utf8ToWide(const std::string& s)
{
    if (s.empty()) return {};
    int n = MultiByteToWideChar(CP_UTF8, 0, s.c_str(), (int)s.size(), nullptr, 0);
    if (n <= 0) return {};
    std::wstring w(n, 0);
    MultiByteToWideChar(CP_UTF8, 0, s.c_str(), (int)s.size(), &w[0], n);
    return w;
}

static std::string WideToUtf8(const std::wstring& w)
{
    if (w.empty()) return {};
    int n = WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), nullptr, 0, nullptr, nullptr);
    if (n <= 0) return {};
    std::string s(n, 0);
    WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), &s[0], n, nullptr, nullptr);
    return s;
}

// 擦除内存中的临时明文特征（避免被自己的扫描线程扫到）
static void Wipe(std::wstring& w) { if (!w.empty()) SecureZeroMemory(&w[0], w.size() * sizeof(wchar_t)); }
static void Wipe(std::string& s) { if (!s.empty()) SecureZeroMemory(&s[0], s.size()); }

// =============================================================================
//  三、特征与替换文本
// =============================================================================
//  特征全部以 XOR 0xA5 的形式存放 —— 进程内存里不存在明文特征串，
//  扫描线程不会命中自己。
// =============================================================================

static const unsigned char kKey = 0xA5;

static std::vector<unsigned char> g_anchor;      // "po.mansus.cc"
static std::vector<unsigned char> g_fullPre[2];  // 整串前缀（全角 / 半角）
static std::vector<unsigned char> g_bodyPre[2];  // 正文前缀（全角 / 半角）

static std::wstring g_textFull;                  // 用户设定的替换文本
static std::wstring g_textBody;                  // 去掉【】后的正文版
static char g_bufFull[84];                       // 预生成：84 字节等长
static char g_bufBody[68];                       // 预生成：68 字节等长

static void BuildFitted(const std::wstring& text, char* out, size_t limit);

static std::vector<unsigned char> Obfuscate(const std::wstring& w)
{
    std::vector<unsigned char> v(w.size() * 2);
    memcpy(v.data(), w.data(), w.size() * 2);
    for (auto& b : v) b ^= kKey;
    return v;
}

static bool MatchAt(const unsigned char* p, const std::vector<unsigned char>& pat)
{
    for (size_t i = 0; i < pat.size(); ++i)
        if (p[i] != (unsigned char)(pat[i] ^ kKey)) return false;
    return true;
}

// 生成"固定长度 + 空格填充"的 UTF-16 字节
static void BuildFitted(const std::wstring& text, char* out, size_t limit)
{
    memset(out, 0, limit);
    size_t chars = text.size();
    if (chars * 2 > limit) chars = limit / 2;
    if (chars) memcpy(out, text.data(), chars * 2);
    for (size_t off = chars * 2; off + 2 <= limit; off += 2)
    {
        out[off] = ' ';      // 低字节
        out[off + 1] = 0;    // 高字节
    }
}

static void BuildPatterns(const std::wstring& replacement)
{
    g_anchor = Obfuscate(L"po.mansus.cc");

    // 整串前缀："【90轻装上阵】开源免费，请勿付费购买。谨防收费开服。来源："
    g_fullPre[0] = Obfuscate(L"\u301090\u8f7b\u88c5\u4e0a\u9635\u3011"
                             L"\u5f00\u6e90\u514d\u8d39\uFF0C\u8bf7\u52ff\u4ed8\u8d39\u8d2d\u4e70\u3002"
                             L"\u8c28\u9632\u6536\u8d39\u5f00\u670d\u3002\u6765\u6e90\uFF1A");
    g_fullPre[1] = Obfuscate(L"\u301090\u8f7b\u88c5\u4e0a\u9635\u3011"
                             L"\u5f00\u6e90\u514d\u8d39,\u8bf7\u52ff\u4ed8\u8d39\u8d2d\u4e70\u3002"
                             L"\u8c28\u9632\u6536\u8d39\u5f00\u670d\u3002\u6765\u6e90:");

    // 正文前缀："开源免费，请勿付费购买。谨防收费开服。来源："
    g_bodyPre[0] = Obfuscate(L"\u5f00\u6e90\u514d\u8d39\uFF0C\u8bf7\u52ff\u4ed8\u8d39\u8d2d\u4e70\u3002"
                             L"\u8c28\u9632\u6536\u8d39\u5f00\u670d\u3002\u6765\u6e90\uFF1A");
    g_bodyPre[1] = Obfuscate(L"\u5f00\u6e90\u514d\u8d39,\u8bf7\u52ff\u4ed8\u8d39\u8d2d\u4e70\u3002"
                             L"\u8c28\u9632\u6536\u8d39\u5f00\u670d\u3002\u6765\u6e90:");

    g_textFull = replacement;

    std::wstring body = replacement;
    if (!body.empty() && body.front() == L'\u3010') body.erase(body.begin());
    if (!body.empty() && body.back() == L'\u3011') body.pop_back();
    g_textBody = body;
    Wipe(body);

    BuildFitted(g_textFull, g_bufFull, sizeof(g_bufFull));
    BuildFitted(g_textBody, g_bufBody, sizeof(g_bufBody));
}

// 读取同目录 banner_cfg.txt（可选）
//   replace=替换文字
//   interval=扫描间隔毫秒
static void LoadConfig()
{
    std::wstring text = L"\u516c\u76ca90\u6b22\u8fce\u60a8\u7684\u5230\u6765~";

    std::wstring path = ModuleDirPath(L"banner_cfg.txt");
    if (FILE* f = _wfopen(path.c_str(), L"rb"))
    {
        fseek(f, 0, SEEK_END);
        long size = ftell(f);
        fseek(f, 0, SEEK_SET);

        std::string raw(size > 0 ? size : 0, 0);
        if (size > 0) fread(&raw[0], 1, size, f);
        fclose(f);

        if (raw.size() >= 3 && (unsigned char)raw[0] == 0xEF) raw.erase(0, 3);  // UTF-8 BOM

        size_t pos = 0;
        while (pos < raw.size())
        {
            size_t eol = raw.find('\n', pos);
            if (eol == std::string::npos) eol = raw.size();

            std::string line = raw.substr(pos, eol - pos);
            pos = eol + 1;

            while (!line.empty() && (line.back() == '\r' || line.back() == ' ')) line.pop_back();
            if (line.empty() || line[0] == '#') continue;

            size_t eq = line.find('=');
            if (eq == std::string::npos) continue;

            std::string key = line.substr(0, eq);
            std::string val = line.substr(eq + 1);
            while (!key.empty() && key.back() == ' ') key.pop_back();
            while (!val.empty() && val.front() == ' ') val.erase(val.begin());

            if (key == "replace") { Wipe(text); text = Utf8ToWide(val); }
            else if (key == "interval") g_intervalMs = atoi(val.c_str());

            Wipe(val);
        }
        Wipe(raw);
    }

    BuildPatterns(text);
    Log("text=%s interval=%d", WideToUtf8(g_textFull).c_str(), g_intervalMs);
}

// =============================================================================
//  四、扫描与替换
// =============================================================================

static int g_hits = 0;      // 已记录日志的命中数（限流）
static int g_patched = 0;   // 成功替换次数

static bool IsReadable(void* p)
{
    MEMORY_BASIC_INFORMATION mbi{};
    if (VirtualQuery(p, &mbi, sizeof(mbi)) == 0) return false;
    return mbi.State == MEM_COMMIT && !(mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD));
}

// 等长原地写入
static bool WriteSameSize(void* addr, const char* data, size_t len)
{
    if (!IsReadable(addr)) return false;

    DWORD oldProtect = 0;
    if (!VirtualProtect(addr, len, PAGE_READWRITE, &oldProtect)) return false;

    memcpy(addr, data, len);

    DWORD tmp = 0;
    VirtualProtect(addr, len, oldProtect, &tmp);
    return true;
}

// 命中锚点后的判定：先试整串，再试正文
static void OnAnchorHit(unsigned char* anchor)
{
    auto addr = (uintptr_t)anchor;

    for (int v = 0; v < 2; ++v)
    {
        // 整串：锚点前 60 字节是"【90轻装上阵】……来源："
        if (addr >= 60 && MatchAt((unsigned char*)(addr - 60), g_fullPre[v]))
        {
            if (WriteSameSize((void*)(addr - 60), g_bufFull, sizeof(g_bufFull)))
            {
                g_patched++;
                if (g_hits < 40) { g_hits++; Log("PATCH full @0x%08X", (unsigned)(addr - 60)); }
            }
            return;
        }

        // 正文：锚点前 44 字节是"开源免费……来源："
        if (addr >= 44 && MatchAt((unsigned char*)(addr - 44), g_bodyPre[v]))
        {
            if (WriteSameSize((void*)(addr - 44), g_bufBody, sizeof(g_bufBody)))
            {
                g_patched++;
                if (g_hits < 40) { g_hits++; Log("PATCH body @0x%08X", (unsigned)(addr - 44)); }
            }
            return;
        }
    }
}

// 单段扫描（纯 POD，便于 SEH 包裹）
static void ScanBlock(unsigned char* base, size_t size)
{
    const size_t n = g_anchor.size();
    if (size <= n + 60) return;

    const unsigned char first = (unsigned char)(g_anchor[0] ^ kKey);
    unsigned char* cur = base;
    unsigned char* end = base + size - n;

    while (cur <= end)
    {
        unsigned char* hit = (unsigned char*)memchr(cur, first, (size_t)(end - cur) + 1);
        if (!hit) break;
        if (MatchAt(hit, g_anchor)) OnAnchorHit(hit);
        cur = hit + 1;
    }
}

// 游戏随时可能释放堆，读到已释放页会 AV —— 这里兜住，只跳过不崩溃
static void ScanBlockSafe(unsigned char* base, size_t size)
{
    __try { ScanBlock(base, size); }
    __except (EXCEPTION_EXECUTE_HANDLER) { }
}

// 枚举 [lo, hi) 内的可写私有页并扫描
static void ScanRange(uintptr_t lo, uintptr_t hi)
{
    const DWORD kWriteable = PAGE_READWRITE | PAGE_WRITECOPY
                           | PAGE_EXECUTE_READWRITE | PAGE_EXECUTE_WRITECOPY;

    uintptr_t addr = lo;
    MEMORY_BASIC_INFORMATION mbi{};

    while (addr < hi)
    {
        if (VirtualQuery((LPCVOID)addr, &mbi, sizeof(mbi)) == 0) break;

        uintptr_t base = (uintptr_t)mbi.BaseAddress;
        uintptr_t next = base + mbi.RegionSize;

        const bool isSelf = (base < g_selfBase + g_selfSize) && (base + mbi.RegionSize > g_selfBase);
        const bool isHeap = mbi.Type == MEM_PRIVATE
                         && mbi.State == MEM_COMMIT
                         && (mbi.Protect & kWriteable)
                         && !(mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD));

        if (!isSelf && isHeap)
            ScanBlockSafe((unsigned char*)base, mbi.RegionSize);

        if (next <= addr) break;
        addr = next;
    }
}

// =============================================================================
//  五、线程与入口
// =============================================================================

struct ScanSlice { uintptr_t lo, hi; };

static DWORD WINAPI ScanSliceThread(LPVOID param)
{
    auto* slice = (ScanSlice*)param;
    for (;;)
    {
        ScanRange(slice->lo, slice->hi);
        Sleep(g_intervalMs > 0 ? g_intervalMs : 0);
    }
    return 0;
}

static DWORD WINAPI MainThread(LPVOID)
{
    InitializeCriticalSection(&g_cs);
    g_csReady = true;

    g_log = _wfopen(ModuleDirPath(L"ijl15_proxy.log").c_str(), L"w");
    Log("loaded, orig=%d fn0=0x%08X", (int)g_origLoaded, (unsigned)(uintptr_t)g_fn[0]);

    LoadConfig();
    StartNativeRedemption();

    SYSTEM_INFO si{};
    GetSystemInfo(&si);
    uintptr_t lo = (uintptr_t)si.lpMinimumApplicationAddress;
    uintptr_t hi = (uintptr_t)si.lpMaximumApplicationAddress;
    uintptr_t span = (hi - lo) / kThreads;

    auto* slices = new ScanSlice[kThreads];
    for (int i = 0; i < kThreads; ++i)
    {
        slices[i].lo = lo + span * i;
        slices[i].hi = (i == kThreads - 1) ? hi : lo + span * (i + 1);
        CreateThread(nullptr, 0, ScanSliceThread, &slices[i], 0, nullptr);
    }

    for (;;)
    {
        Sleep(30000);
        Log("alive, patched=%d", g_patched);
    }
    return 0;
}

// 加载原版 ijl15.dll 并解析 6 个函数
static void LoadOriginal()
{
    HMODULE h = LoadLibraryW(ModuleDirPath(L"ijl15_orig.dll").c_str());
    if (!h) h = LoadLibraryW(ModuleDirPath(L"orig\\ijl15.dll").c_str());
    if (!h) h = LoadLibraryA("ijl15_orig.dll");

    if (!h)
    {
        for (auto& f : g_fn) f = (void*)StubFn;
        return;
    }

    g_origLoaded = true;

    static const char* kNames[6] = {
        "ijlGetLibVersion", "ijlInit", "ijlFree", "ijlRead", "ijlWrite", "ijlErrorStr"
    };

    for (int i = 0; i < 6; ++i)
    {
        void* p = (void*)GetProcAddress(h, kNames[i]);
        g_fn[i] = p ? p : (void*)StubFn;
    }
}

BOOL APIENTRY DllMain(HMODULE module, DWORD reason, LPVOID)
{
    if (reason == DLL_PROCESS_ATTACH)
    {
        g_self = module;
        DisableThreadLibraryCalls(module);

        MODULEINFO mi{};
        if (GetModuleInformation(GetCurrentProcess(), module, &mi, sizeof(mi)))
        {
            g_selfBase = (uintptr_t)mi.lpBaseOfDll;
            g_selfSize = mi.SizeOfImage;
        }

        LoadOriginal();
        CreateThread(nullptr, 0, MainThread, nullptr, 0, nullptr);
    }
    return TRUE;
}
