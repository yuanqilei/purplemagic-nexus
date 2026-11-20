#!/usr/bin/env bash
# 本服一键部署：拉代码 → 构建镜像 → 重启三个服务 → 探测。
# 用法：bash ops/deploy.sh [--no-build]
set -euo pipefail

SRC="${SRC:-/opt/purplemagic-src}"
DATA="${DATA:-/opt/purplemagic/data2}"
COMPOSE=(docker compose --env-file "$DATA/compose.env" -f deploy/selfhost/compose.yaml)

cd "$SRC"

echo "[1/4] git pull"
git pull --ff-only

if [ "${1:-}" != "--no-build" ]; then
  echo "[2/4] build（改过前端或 Go 才需要，未改可加 --no-build 跳过）"
  "${COMPOSE[@]}" build
else
  echo "[2/4] 跳过构建"
fi

echo "[3/4] up -d gateway gm edge"
"${COMPOSE[@]}" up -d gateway gm edge

echo "[4/4] 探测"
sleep 4
curl -sk -m 10 -o /dev/null -w '  console  %{http_code}\n' https://127.0.0.1/console/
curl -sk -m 10 -o /dev/null -w '  gateway  %{http_code}\n' https://127.0.0.1:8443/api/status
curl -sk -m 10 -o /dev/null -w '  bootstrap %{http_code}\n' https://127.0.0.1:8443/launcher/v2/bootstrap.json
echo "完成。后台 https://<服务器IP>/console/"
