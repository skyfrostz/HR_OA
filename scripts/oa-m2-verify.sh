#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."

set -a
source ./.env
set +a
results=tests/results/m2
browser_python="${OA_QA_PYTHON:-.local/qa-venv/bin/python}"

restore_notifications() {
    OA_MOCK_NOTIFY_FAIL=false docker compose up -d --wait --wait-timeout 120
}

docker compose build

# The fault applies to the development adapter only. Restore it even if the
# verification exits during the fault phase, without removing either volume.
trap restore_notifications EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
OA_MOCK_NOTIFY_FAIL=true docker compose up -d --no-build --wait --wait-timeout 120
python3 -B tests/oa_m2_api.py create --state "$results/oa-m2-state.json"
OA_MOCK_NOTIFY_FAIL=false docker compose up -d --force-recreate --wait --wait-timeout 120
trap - EXIT INT TERM
python3 -B tests/oa_m2_api.py verify --state "$results/oa-m2-state.json"
"$browser_python" -B tests/oa_m2_browser.py --artifacts "$results/browser"
./scripts/oa-m2-m1-regression.sh
