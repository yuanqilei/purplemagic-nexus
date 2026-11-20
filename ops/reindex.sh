#!/usr/bin/env bash
# 重建四个索引（换 Script.pvf 后才需要，约 15 分钟），然后重启 gm。
set -euo pipefail

SRC="${SRC:-/opt/purplemagic-src}"
DATA="${DATA:-/opt/purplemagic/data2}"
COMPOSE=(docker compose --env-file "$DATA/compose.env" -f deploy/selfhost/compose.yaml)

cd "$SRC"
for step in index-pvf index-items index-quests index-professions; do
  echo ">>> $step"
  "${COMPOSE[@]}" --profile setup run --rm "$step"
done

echo ">>> restart gm"
"${COMPOSE[@]}" restart gm
sleep 3
curl -sk -m 10 -o /dev/null -w 'console %{http_code}\n' https://127.0.0.1/console/
