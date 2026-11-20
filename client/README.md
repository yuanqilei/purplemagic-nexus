# client · 客户端

登录器（PurpleMagic）各版本发行信息。**二进制不放仓库**（避免仓库膨胀），统一发布到 GitHub Releases；
仓库只保留本清单 `versions.json`。下载地址形如
`https://github.com/yuanqilei/xianren/releases/download/<版本>/PurpleMagic-<版本>.exe`。

## 机器可读清单：`versions.json`

```json
{
  "schema": 1,
  "channel": "stable",
  "latest": "1.0.16",
  "versions": [
    { "version": "1.0.16", "file": "PurpleMagic-1.0.16.exe",
      "size": 23832064, "sha256": "42e91426…", "released": "2026-11-19", "notes": "…" }
  ]
}
```

用途：登录器「检查更新」脚本、更新包发布工具、回滚脚本直接读这个文件即可，
不用再解析 README。字段约定：

| 字段 | 说明 |
|---|---|
| `schema` | 清单格式版本（当前 1） |
| `channel` | `stable` / `beta`，多通道时新增文件而不是改字段 |
| `latest` | 最新版本号（等于 `versions[0].version`） |
| `versions[].version` | 版本号，与文件名一致 |
| `versions[].file` | GitHub Release 资产文件名 |
| `versions[].size` / `sha256` | 下载后必须校验 |
| `versions[].released` | 发布日期（YYYY-MM-DD） |
| `versions[].notes` | 一句话更新说明 |

## 当前版本

| 文件 | 版本 | 大小 | SHA256 |
|---|---|---|---|
| `PurpleMagic-1.0.16.exe` | 1.0.16 | 23,832,064 B | `42e91426be92d2e9bf615f7a7316bed036af72f4d622358213e696fad69dbdbc` |

## 怎么用

单个便携 exe，**放进客户端目录**（和 `DNF.exe`、`Script.pvf` 同一层），双击即可：

- 自动识别所在目录/`client/` 子目录，不需要手动选路径；
- 首次启动会按服务端档案校验 `DNF.exe` / `Script.pvf` 的哈希，不一致会提示版本不符；
- 账号密码、客户端目录等个人设置存在 `%LOCALAPPDATA%\DNF90Launcher`，不写客户端目录。

## 加新版本

1. 新 exe 命名 `PurpleMagic-<版本>.exe`，计算大小和 SHA256；
2. 上传到对应 GitHub Release，确认 `url` 可下载且摘要一致；
3. 更新 `versions.json` 与本文件，再提交清单。旧版本由独立备份管理。

> 本目录只存清单；发布前需确认新版本的 GitHub Release 资产已上传。

## 相关

- 登录器源码在 `launcher/`（.NET + Flutter）；
- 服务端更新包（客户端整包热更新）走 GM 后台的「更新包」发布，不在本目录。
