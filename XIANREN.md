# xianren · PurpleMagic 私有分支

本仓库是本服 GM 后台与玩家网关的私有开发分支，源码来自上游
[`Cec1c/PurpleMagic`](https://github.com/Cec1c/PurpleMagic)，用于后续自行开发 GM 功能。

## 上游基线

| 项 | 值 |
|---|---|
| 上游仓库 | `https://github.com/Cec1c/PurpleMagic` |
| 基线提交 | `868ba3d`（feat(gm): speed up dungeon quota reads…） |
| 本仓库分支 | `main`（从 `s5a3-gm` 迁入，含完整历史 94 个提交） |

## 本分支相对上游的差异

| 提交 | 内容 | 原因 |
|---|---|---|
| `1ea9f4a` | `deploy/selfhost/Dockerfile`：加 `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`；删掉 runtime 阶段的 `apt-get install ca-certificates curl` | 服务器访问 `proxy.golang.org` 不通；服务器时钟为 2026 年导致 Debian 源签名校验失败（NO_PUBKEY），且静态 Go 程序不需要这两个包 |

> 从上游合并更新时，这两处改动会被覆盖，需要重新打补丁，否则 `docker compose build` 会失败。

## 线上部署

| 项 | 值 |
|---|---|
| 服务器 | `<SERVER_IP>`，代码在 `/opt/purplemagic-src` |
| 编排 | `deploy/selfhost/compose.yaml`（gateway / gm / edge 三个容器，host 网络） |
| 数据与配置 | `/opt/purplemagic/data2`（**不在仓库里**：证书、私钥、Token、索引、hub 状态） |
| GM 工作台 | `https://<SERVER_IP>/console/`（Token 登录，Token 在 `data2/admin-token.txt`） |
| 玩家 API | `https://<SERVER_IP>:8443`（TLS 证书固定，由登录器使用） |

## 开发流程

```bash
# 1. 本机改代码（gm-console 前端 / gateway 网关 / gm 工具）
cd gm-console && npm ci && npm run build      # 前端至少能构建通过

# 2. 提交
git add -A && git commit -m "feat(gm): ..." && git push

# 3. 服务器拉取（在 /opt/purplemagic-src）
git pull

# 4. 重新构建并上线
#    方式一：S5A3 运维工具 → 第④页 → 「部署 GM」
#    方式二：命令行
docker compose --env-file /opt/purplemagic/data2/compose.env \
  -f deploy/selfhost/compose.yaml build
docker compose --env-file /opt/purplemagic/data2/compose.env \
  -f deploy/selfhost/compose.yaml up -d gateway gm edge
```

前端改动必须重新构建镜像（构建产物进镜像的 `/opt/purplemagic/web`），改完浏览器 `Ctrl+F5`。

## 目录速查

| 目录 | 内容 |
|---|---|
| `gm-console/` | GM 工作台前端（React 19 + Vite 6 + TypeScript） |
| `gateway/` | PlayerGateway / GMService / GMIndex / launcherhub（Go 1.26.5） |
| `gm/` | GMTool 离线工具（Go） |
| `deploy/selfhost/` | Dockerfile、compose、`prepare.py`、配置模板 |
| `docs/` | 部署、网关契约、GM 各功能说明 |
| `ops/` | **部署运行手册与脚本**（deploy / status / reindex / 重置 Token / settings 模板） |
| `client/` | 客户端登录器发行文件（`PurpleMagic-<版本>.exe` + 版本哈希表） |
| `tools/s5a3-ops/` | S5A3 服务端管理工具（Python + tkinter 桌面程序，含构建脚本） |

## 回滚

```bash
# 构建前先打标签
docker tag purplemagic:local purplemagic:rollback-$(date +%m%d-%H%M)

# 需要时回滚
docker tag purplemagic:rollback-<时间> purplemagic:local
docker compose ... up -d gateway gm edge
```

## 不要提交

`data2/` 及其内容、任何证书与私钥、`admin-token.txt`、数据库导出、构建产物（`dist/`、`node_modules/`）。

> **注（2026-11-19）**：公开前做过一次历史清洗——移除了服务器 IP 与 `client/*.exe` 二进制，
> 因此**所有提交哈希都已改变**（旧文档里引用的 `b324d00`/`b7c2779`/`2b32651` 等不再存在）。
> 本地旧工作副本请重新 clone，不要直接 `git pull`。
