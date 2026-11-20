# PurpleMagic Nexus

登录器 + 网关 + GM 服务 + 管理后台，四者同属一个窗口、同一源码树，统一构建与部署。

## 归属

- 上游：`Cec1c/PurpleMagic`（登录器 / 网关 / GM 来源）
- 原始资料：`02_登录器-GM网关-后台-源码-1648f2f\xianren-src`，历史构建参考 `03_登录器exe-成品与构建-1.0.15`

## 迁入范围

- `launcher` / `gateway` / `gm` / `gm-console`
- `client/versions.json`
- 相关部署与文档
- 03 中仍需要的构建脚本与美术原件

原 `dnf90-gm` 仓的内容已并入本仓，不再拆分。

## 版本与发布

- 登录器版本常量：`launcher/Engine/LauncherEngine.cs` 的 `VersionText`
- 当前交付以 06 中的登录器 1.0.16 为准；03 中 1.0.15 仅作构建方法参考
- 构建源码路径须为纯 ASCII
- 发布顺序：客户端构建校验 → 玩家包 URL 可下载 → 更新本仓 `client/versions.json` → 重编 / 发布登录器并更新网关分发
- 保持 `versions.json` 原有字段契约

## 发布责任

登录器 EXE、玩家 API、客户端版本分发、GM 服务和后台。已冻结的 gm-web 仅在有兼容需求时保留。

## 禁止提交

口令、Token、私钥、证书私钥、真实 `.env`、数据库连接串、玩家数据、线上配置、本机 DPAPI 凭据。配置示例一律用占位值。大文件（登录器 EXE、美术原件二进制）放 Release 附件或对象存储。

## 许可证

保留上游 `Cec1c/PurpleMagic` 的许可证、版权和来源记录。
