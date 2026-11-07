#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Reuse the M1 behavior checks with separate artifacts for the M2 run.
set -a
source ./.env
set +a
results=tests/results/m2/m1-regression
baseline=tests/results/oa-state.json
if [[ -f "$baseline" ]]; then
    python3 -B tests/oa_api.py verify --state "$baseline"
    printf '%s\n' 'PASS pre-M2 employee, contract and attachment survived migration'
else
    printf '%s\n' 'NOT RUN pre-M2 fixture readback (M1 state artifact unavailable)'
fi
python3 -B tests/oa_api.py create --state "$results/oa-state.json"
docker compose up -d --force-recreate --wait --wait-timeout 120
python3 -B tests/oa_api.py verify --state "$results/oa-state.json"
"${OA_QA_PYTHON:-.local/qa-venv/bin/python}" -B tests/oa_browser.py \
    --state "$results/oa-state.json" --artifacts "$results/browser"
