#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""重置 GM 管理 Token：改 gateway.json 两处 sha256 + admin-token.txt + 重启 gateway。

用法（必须以能读 data2 的用户跑）：
    runuser -u dnf90gm -- env HOME=/tmp python3 ops/reset-admin-token.py
成功后把打印出来的 Token 存好；它同时只用于 GM 工作台登录与 Hub 管理接口。
"""
import hashlib
import json
import os
import secrets
import subprocess
import sys
import time

DATA = os.environ.get("PM_DATA", "/opt/purplemagic/data2")
SRC = os.environ.get("SRC", "/opt/purplemagic-src")


def main():
    gateway_path = os.path.join(DATA, "gateway.json")
    token_path = os.path.join(DATA, "admin-token.txt")

    token = secrets.token_urlsafe(36)
    digest = hashlib.sha256(token.encode("utf-8")).hexdigest()

    with open(gateway_path, encoding="utf-8") as f:
        cfg = json.load(f)
    cfg["management"]["loginKeySha256"] = digest
    cfg["launcher_hub"]["admin_token_sha256"] = digest

    tmp = gateway_path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
    os.chmod(tmp, 0o600)
    os.replace(tmp, gateway_path)

    with open(token_path, "w", encoding="utf-8") as f:
        f.write(token + "\n")
    os.chmod(token_path, 0o600)

    print("token updated, restarting gateway …")
    subprocess.run(
        ["docker", "compose", "--env-file", os.path.join(DATA, "compose.env"),
         "-f", "deploy/selfhost/compose.yaml", "restart", "gateway"],
        cwd=SRC, check=False)

    time.sleep(2)
    code = subprocess.run(
        ["curl", "-sk", "-m", "8", "-o", "/dev/null", "-w", "%{http_code}",
         "-H", "Authorization: Bearer " + token,
         "https://127.0.0.1/admin/api/news"],
        capture_output=True, text=True).stdout.strip()
    print("verify /admin/api/news ->", code, "(200 = ok)")
    print("ADMIN_TOKEN=" + token)
    return 0 if code == "200" else 1


if __name__ == "__main__":
    sys.exit(main())
