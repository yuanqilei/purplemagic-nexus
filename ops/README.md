# ops · 本服部署运行手册

这里放**部署这件事本身**：脚本、配置模板、运行手册。代码本体在仓库其他目录。

> 敏感信息一律不进仓库：`data2/`（证书、私钥、Token、索引、hub 状态）、真实 `settings.json`（含数据库口令与游戏 Token）。

## 一、这套部署长什么样

| 项 | 值 |
|---|---|
| 代码目录 | `/opt/purplemagic-src`（本仓库的副本，`origin` 指向本仓库） |
| 数据/配置目录 | `/opt/purplemagic/data2`（**不在仓库**） |
| 编排 | `deploy/selfhost/compose.yaml`，host 网络，三个容器 |
| 容器 | `purplemagic-gateway-1`（8443/18444）、`purplemagic-gm-1`（18080/18081）、`purplemagic-edge-1`（nginx，443） |
| 对外端口 | 443（GM 工作台）、8443（玩家 API） |
| 只监听本机 | 18080、18081、18444 |
| 防火墙 | ufw 只放行 22 / 80 / 443 / 8443 |

## 二、日常部署

```bash
bash ops/deploy.sh              # git pull + build + up -d + 探测
bash ops/deploy.sh --no-build   # 只拉代码重启（没改前端/Go 时用）
bash ops/status.sh              # 只读状态
```

对应的图形入口是 S5A3 运维工具第④页「部署 GM」（等价于上面第一条）。

## 三、改完前端/后端要注意

- 前端（`gm-console/`）改动必须重新 `build`，产物进镜像的 `/opt/purplemagic/web`；
- 浏览器打开 `https://<IP>/console/` 后按 `Ctrl+F5`，否则是旧缓存；
- 改 gateway 会短暂断开玩家 API（8443），选没人时做。

## 四、Token

GM 工作台和 Hub 管理接口共用一个 Token：

```bash
runuser -u dnf90gm -- env HOME=/tmp python3 ops/reset-admin-token.py
```

它会改 `data2/gateway.json` 的两处 sha256、重写 `data2/admin-token.txt`、重启 gateway，并打印新 Token（只显示这一次）。

## 五、索引

只有换 `Script.pvf` 才需要：

```bash
bash ops/reindex.sh     # 四个 indexer + 重启 gm，约 15 分钟
```

## 六、从零部署（新机器）

1. 准备 `data2`（必须用非 root 用户跑）：

```bash
git clone <本仓库> /opt/purplemagic-src
cd /opt/purplemagic-src
cp deploy/selfhost/settings.example.json /opt/purplemagic/settings.json
# 按 ops/settings.s5a3.example.json 的字段填：数据库 DSN、PVF、证书、游戏 Token、GM 域名
runuser -u dnf90gm -- env HOME=/tmp python3 deploy/selfhost/prepare.py \
  --settings /opt/purplemagic/settings.json --output /opt/purplemagic/data2
```

2. 建索引 + 建表：

```bash
DATA=/opt/purplemagic/data2 bash ops/reindex.sh
docker compose --env-file /opt/purplemagic/data2/compose.env \
  -f deploy/selfhost/compose.yaml --profile setup run --rm schema
```

3. 起服务：

```bash
bash ops/deploy.sh --no-build
```

## 七、回滚

```bash
# 构建前打标签
docker tag purplemagic:local purplemagic:rollback-$(date +%m%d-%H%M)

# 回滚
docker tag purplemagic:rollback-<时间> purplemagic:local
cd /opt/purplemagic-src && bash ops/deploy.sh --no-build
```

代码回滚：`git revert` / `git reset --hard <commit>` 后重新构建。

## 八、构建环境注意事项（本服特殊）

- `deploy/selfhost/Dockerfile` 有两处本地补丁：`GOPROXY=https://goproxy.cn,direct`、删掉 runtime 的 `apt-get`（服务器时钟 2026 年，Debian 源签名校验失败）。**从上游合并覆盖后必须重新打补丁**；
- `prepare.py` 拒绝 root，必须 `runuser -u dnf90gm`；
- 服务器访问 `proxy.golang.org` 不通，只能走 goproxy.cn。

## 九、不做什么

- 不在仓库里存 `data2`、证书、私钥、Token、数据库导出；
- 不对外暴露 18080/18081/18444；
- 不用真实玩家资产做测试。
