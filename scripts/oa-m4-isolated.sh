#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."

milestone="${OA_QA_MILESTONE:-4}"
case "$milestone" in
    4) prior_state=() ;;
    5) prior_state=(--m4-state tests/results/m4/api-state.json) ;;
    6) prior_state=(--m4-state tests/results/m4/api-state.json --m5-state tests/results/m5/browser-state.json) ;;
    *) printf '%s\n' 'OA_QA_MILESTONE must be 4, 5, or 6' >&2; exit 2 ;;
esac
baseline="$PWD/.local/m${milestone}-baseline"
archive="$(< "$baseline/BACKUP_PATH")"
case "$archive" in
    "$baseline"/oa-backup.*) ;;
    *) printf '%s\n' 'Unexpected QA backup path' >&2; exit 2 ;;
esac
(cd "$baseline" && shasum -a 256 -c SHA256SUMS)
(cd "$archive" && shasum -a 256 -c SHA256SUMS)
set -a
source ./.env
set +a
project="hr-oa-m${milestone}-qa-$(openssl rand -hex 6)"
export OA_RESTORE_DB_PASSWORD OA_RESTORE_DB_ROOT_PASSWORD OA_RESTORE_JWT_SECRET OA_RESTORE_API_IMAGE
OA_RESTORE_DB_PASSWORD="$(openssl rand -hex 32)"
OA_RESTORE_DB_ROOT_PASSWORD="$(openssl rand -hex 32)"
OA_RESTORE_JWT_SECRET="$(openssl rand -hex 32)"
OA_RESTORE_API_IMAGE=hr-oa-api:latest
api_image="$(docker image inspect --format '{{.Id}}' "$OA_RESTORE_API_IMAGE")"
if [[ "$api_image" == "$(< "$archive/API_IMAGE_ID")" ]]; then
    printf '%s\n' 'Build the candidate API image before running isolated upgrade checks.' >&2
    exit 2
fi

isolated() {
    docker compose --project-name "$project" -f deploy/oa/restore.compose.yaml "$@"
}
trap 'isolated down >&2' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
isolated up -d --pull never --wait --wait-timeout 120 mysql >&2
db="$(isolated ps -q mysql)"
[[ "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$db")" == "$project" ]]
docker inspect --format '{{json .HostConfig.Tmpfs}}' "$db" | rg -Fq '"/var/lib/mysql"'
isolated exec -T mysql sh -ec '
    test "$MYSQL_DATABASE" = hr_oa_restore
    MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch -u "$MYSQL_USER" "$MYSQL_DATABASE"
' < "$archive/mysql.sql"
isolated up -d --pull never --wait --wait-timeout 120 api >&2
api="$(isolated ps -q api)"
[[ "$(docker inspect --format '{{.Image}}' "$api")" == "$api_image" ]]
[[ "$(docker inspect --format '{{len .Mounts}}' "$api")" == 0 ]]
isolated exec -T api tar -xmf - -C /var/lib/oa < "$archive/private.tar"
address="$(isolated port api 8888)"
[[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]]
python3 -B tests/oa_restore_verify.py --base "http://$address" \
    --employee-state tests/results/oa-state.json --approval-state tests/results/m2/oa-m2-state.json \
    --m3-state tests/results/m3/state.json ${prior_state[@]+"${prior_state[@]}"}

# If supplied, the isolated suite receives its own origin and project ID.
if [[ $# -gt 0 ]]; then
    export OA_QA_BASE="http://$address" OA_QA_PROJECT="$project"
    if [[ "$milestone" == 4 ]]; then
        export OA_M4_QA_BASE="$OA_QA_BASE" OA_M4_QA_PROJECT="$OA_QA_PROJECT"
    fi
    if "$@"; then
        :
    else
        status=$?
        docker inspect --format 'Isolated API state={{.State.Status}} health={{.State.Health.Status}} exit={{.State.ExitCode}}' "$api" >&2
        isolated logs --tail 30 --no-color api | rg -oi 'migrat[a-z]*|bootstrap|initializ[a-z]*|failed|fatal|panic|shutdown|listening' >&2 || true
        exit "$status"
    fi
fi
isolated down >&2
trap - EXIT INT TERM
