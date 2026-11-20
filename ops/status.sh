#!/usr/bin/env bash
# 只读状态：容器、端口、探测、Token/索引文件、ufw。
set -uo pipefail

SRC="${SRC:-/opt/purplemagic-src}"
DATA="${DATA:-/opt/purplemagic/data2}"
COMPOSE=(docker compose --env-file "$DATA/compose.env" -f deploy/selfhost/compose.yaml)

echo "--- 容器 ---"
cd "$SRC" && "${COMPOSE[@]}" ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null

echo "--- 端口 ---"
ss -tlnp 2>/dev/null | grep -E ':443 |:8443|:18080|:18081|:18444' || echo "  (无监听)"

echo "--- 探测 ---"
curl -sk -m 8 -o /dev/null -w '  console  %{http_code}\n' https://127.0.0.1/console/ || true
curl -sk -m 8 -o /dev/null -w '  gateway  %{http_code}\n' https://127.0.0.1:8443/api/status || true
curl -sk -m 8 -o /dev/null -w '  bootstrap %{http_code}\n' https://127.0.0.1:8443/launcher/v2/bootstrap.json || true

echo "--- 数据 ---"
ls -l "$DATA/admin-token.txt" "$DATA/indexes/items.json" 2>/dev/null || echo "  缺 Token 或索引"

echo "--- 代码版本 ---"
cd "$SRC" && git log --oneline -1 && git status -sb | head -2

echo "--- ufw ---"
ufw status | head -12
