#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."

case "${1:-up}" in
    init)
        python3 - <<'PYENV'
import os
import secrets

keys = ("OA_DB_PASSWORD", "OA_DB_ROOT_PASSWORD", "OA_JWT_SECRET", "OA_SEED_PASSWORD")
try:
    fd = os.open(".env", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    print("已保留现有 .env。")
else:
    with os.fdopen(fd, "w", encoding="utf-8") as env_file:
        for key in keys:
            env_file.write(f"{key}={secrets.token_hex(32)}\n")
    print("已生成本机开发 .env。")
PYENV
        ;;
    up)
        docker compose up -d --build --wait --wait-timeout 120
        printf '%s\n' 'HR OA 开发入口 http://127.0.0.1:8080/oa.html'
        ;;
    stop)
        docker compose stop
        ;;
    verify)
        # Read only the repository owner's generated local environment file.
        set -a
        source ./.env
        set +a
        python3 -B tests/oa_api.py create
        # Recreate containers without deleting named volumes to verify persistence across replacement.
        docker compose up -d --force-recreate --wait --wait-timeout 120
        python3 -B tests/oa_api.py verify
        ;;
    *)
        printf 'Usage: %s [init|up|stop|verify]\n' "$0" >&2
        exit 2
        ;;
esac
