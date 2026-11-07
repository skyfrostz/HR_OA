#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."
set -a
source ./.env
set +a

state=tests/results/m3/state.json
python3 -B tests/oa_m3_api.py verify --results "$state"
"${OA_QA_PYTHON:-.local/qa-venv/bin/python}" -B tests/oa_m3_browser.py --state "$state"
archive="$(./scripts/oa-backup.sh backup)"
./scripts/oa-backup.sh restore "$archive" tests/results/oa-state.json tests/results/m2/oa-m2-state.json "$state"
