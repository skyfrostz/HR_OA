#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."
set -a
source ./.env
set +a

results=tests/results/pure
baseline=tests/results/oa-state.json
python3 -B tests/oa_api.py verify --state "$baseline"
python3 -B tests/oa_api.py create --state "$results/oa-state.json"
docker compose up -d --force-recreate --wait --wait-timeout 120
python3 -B tests/oa_api.py verify --state "$results/oa-state.json"
"${OA_QA_PYTHON:-.local/qa-venv/bin/python}" -B tests/oa_pure_browser.py \
    --state "$results/oa-state.json" --artifacts "$results/browser"
