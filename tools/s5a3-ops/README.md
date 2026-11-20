# tools/s5a3-ops · S5A3 服务端管理工具

一个 Windows 桌面运维工具（Python + tkinter），用来管理本服的 S5A3 服务端：部署/更新、数据库、登录器服务端（档案/更新包）、GM 后台（部署/Token/状态/索引）。

## 页面

| 页 | 能做什么 |
|---|---|
| 游戏服务端 | 连接检测、一键部署、更新服务端、替换 PVF、查看状态、端口/频道校验 |
| 数据库管理 | 导入初始库、替换数据库、备份、恢复 |
| 登录器服务端 | 上传登录器、检查/生成通讯文件（profile）、检查/发布更新包、打开后台 |
| GM 工具 | 部署 GM（compose）、重置管理 Token、后台状态、重建索引、打开后台 |

危险操作（替换数据库/PVF、一键部署）要输入 `CONFIRM`；GM 页的几个常规操作是「确定执行」按钮。

## 配置文件（**不在仓库里**）

工具首次运行会在 exe 同目录生成 `s5a3_config.json`，里面存服务器地址、root 口令（Windows DPAPI 加密，仅本机本用户可解）、各项路径。**不要提交这个文件，也不要拷给别人。**

换机器用时密码解不开是正常现象，重新输入一次 root 口令保存即可。

## 构建

需要 Python 3.12 + `paramiko` + `pyinstaller`：

```powershell
pip install paramiko pyinstaller
powershell -File build.ps1
```

产物在 `dist\S5A3服务端管理.exe`（约 23 MB，单文件、无控制台）。`version_info.txt` 控制 exe 的版本资源，改版本时同步改 `s5a3_deploy_tool.py` 里的 `TOOL_VERSION`。

## 自检

```powershell
.\dist\S5A3服务端管理.exe --selftest    # 期望输出 selftest ok
```

## 说明

- 这个工具只是“操作入口”，真正的部署逻辑在服务器上：`ops/` 目录（compose 部署、Token 重置、索引重建）；
- 工具里 GM 页调用的就是 `ops/` 那套等价命令；
- 工具本身不保存任何 Token，GM Token 只显示一次并复制到剪贴板。
