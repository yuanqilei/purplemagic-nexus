# DNF90 横幅文字替换

把进入赛利亚房间时顶部弹出的横幅

```
【90轻装上阵】开源免费，请勿付费购买。谨防收费开服。来源：po.mansus.cc
```

替换成

```
公益90欢迎您的到来~
```

> 完整推导过程（每一步的证据与弯路）见 [docs/完整分析过程.md](docs/完整分析过程.md)

---

## 目录结构

```
DNF90横幅修改/
├─ README.md
├─ docs/
│  └─ 完整分析过程.md          从截图到成功的完整记录
├─ src/
│  ├─ proxy/                   C++ 代理 DLL（实际生效的方案）
│  │  ├─ ijl15_proxy.cpp       主源码（中文注释）
│  │  ├─ ijl15_proxy.def       导出序号定义
│  │  └─ build.ps1             一键编译
│  └─ tool/                    .NET 10 工具（验证 / 临时替换）
│     ├─ Program.cs            命令行入口
│     ├─ BannerEngine.cs       扫描与替换引擎
│     └─ DnfBannerTool.csproj
└─ dist/                       部署包
   ├─ ijl15.dll                编译好的代理
   ├─ banner_cfg.txt           配置
   ├─ 部署.bat                 一键部署到 E:\D90
   └─ 还原ijl15.bat            一键还原
```

---

## 使用

### 方式一：一键部署（推荐）

1. 使用 Visual Studio 2022 Build Tools 的 x86 C++ 工具链编译代理 DLL：

   ```powershell
   cd src\proxy
   .\build.ps1
   copy .\ijl15.dll ..\..\dist\ -Force
   ```

2. 双击 `dist\部署.bat`（默认部署到 `E:\D90`，也可拖拽指定目录）。

3. 启动游戏，进赛利亚房间。

### 方式二：.NET 10 工具（不装 DLL）

```bat
cd src\tool
dotnet build -c Release

bin\Release\net10.0\DnfBannerTool.exe scan
bin\Release\net10.0\DnfBannerTool.exe patch --text "公益90欢迎您的到来~"
bin\Release\net10.0\DnfBannerTool.exe watch --interval 50
```

---

## 配置

`banner_cfg.txt`：

```ini
# 替换文字
replace=公益90欢迎您的到来~

# 扫描间隔（毫秒），默认 1
interval=1
```

---

## 还原

双击 `dist\还原ijl15.bat`，或手动：

```powershell
Copy-Item E:\D90\ijl15.dll.bak E:\D90\ijl15.dll -Force
```

---

## 原理速览

1. **不改 `DNF.exe` / `Script.pvf`**——启动器对这两个文件有 SHA-256 校验，
   改了会被还原；`ijl15.dll` 不在校验清单里。
2. 用同名代理 `ijl15.dll` 顶替原版，原版改名 `ijl15_orig.dll`；
   代理在 `DllMain` 里加载原版，6 个导出按**原序号**裸 `jmp` 转发。
3. 横幅是进房间时生成的 UTF-16LE 堆字符串，只显示约 5 秒；
   代理用 8 个线程、每 1ms 扫一遍堆，用 `po.mansus.cc` 做锚点定位，
   命中后**等长替换**（不足补空格）。

---

## 为什么 DLL 只能用 C++ 编

| | |
|---|---|
| DNF.exe | 32 位（x86，PE machine = 0x14C） |
| .NET NativeAOT 支持 | x64 / arm64（**无 x86**） |
| 托管 DLL | 原生进程无法加载 |

所以常驻方案是 C++；.NET 10 部分做成独立进程工具，
用 `ReadProcessMemory / WriteProcessMemory` 完成同样的定位与替换。

---

## 注意

- 8 线程 1ms 会占用较多内存带宽；觉得卡就把 `interval` 调到 5～10。
- 若游戏闪退：先运行 `还原ijl15.bat`，再看 `E:\D90\ijl15_proxy.log` 尾部。
- 只改进程内存，不改任何游戏文件，不会触发文件完整性校验。
