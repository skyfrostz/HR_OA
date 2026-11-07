#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."
set -a
source ./.env
set +a

project="hr-oa-export-qa-$(openssl rand -hex 6)"
export OA_RESTORE_DB_PASSWORD OA_RESTORE_DB_ROOT_PASSWORD OA_RESTORE_JWT_SECRET OA_RESTORE_API_IMAGE
OA_RESTORE_DB_PASSWORD="$(openssl rand -hex 32)"
OA_RESTORE_DB_ROOT_PASSWORD="$(openssl rand -hex 32)"
OA_RESTORE_JWT_SECRET="$(openssl rand -hex 32)"
OA_RESTORE_API_IMAGE=hr-oa-api:latest
live_api="$(docker compose ps -q api)"
[[ "$(docker image inspect --format '{{.Id}}' "$OA_RESTORE_API_IMAGE")" == "$(docker inspect --format '{{.Image}}' "$live_api")" ]]

isolated() {
    docker compose --project-name "$project" -f deploy/oa/restore.compose.yaml \
        -f deploy/oa/commercial-limit.compose.yaml "$@"
}
trap 'isolated down >&2' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
isolated up -d --pull never --wait --wait-timeout 120 >&2
db="$(isolated ps -q mysql)"
[[ "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$db")" == "$project" ]]
docker inspect --format '{{json .HostConfig.Tmpfs}}' "$db" | rg -Fq '"/var/lib/mysql"'
address="$(isolated port api 8888)"
[[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]]

isolated exec -T mysql sh -ec '
    test "$MYSQL_DATABASE" = hr_oa_restore
    MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch -u "$MYSQL_USER" "$MYSQL_DATABASE"
' <<'SQL'
SET SESSION cte_max_recursion_depth=5001;
INSERT INTO oa_employees (employee_no,name,birth_date,hire_date,department,version,salary_cents)
WITH RECURSIVE samples(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM samples WHERE n < 5001)
SELECT CONCAT('QA-CAP-', LPAD(n, 4, '0')), IF(n=5001,'QA-CAP-overflow','QA-CAP-exact'),
    '1990-01-01', '2022-01-01', 'QA synthetic export limit', 1, 1234 FROM samples;
SQL
PYTHONPATH=tests python3 -B - "http://$address" <<'PY'
import sys
from oa_commercial_api import export
from oa_m2_api import login
from oa_m3_api import call

base = sys.argv[1]
for name, role in (("hr_admin", "HR_ADMIN"), ("hr_viewer", "HR_VIEWER")):
    token = login(base, name, role)["token"]
    rows = export(base, token, {"query": "QA-CAP-exact"})
    assert len(rows) == 5001 and rows[1][0] == "QA-CAP-0001" and rows[-1][0] == "QA-CAP-5000"
    call(base, "/employees/export.xlsx", token, "POST", {"query": "QA-CAP-"},
         status=409, error="EXPORT_LIMIT")
print("PASS MySQL isolated XLSX export returns all 5000 matches and rejects 5001 for admin and viewer")
PY
isolated down >&2
trap - EXIT INT TERM
