#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."

running_api_image() {
    local container
    container="$(docker compose ps -q api)"
    [[ -n "$container" ]] || return 1
    docker inspect --format '{{.Image}}' "$container"
}

backup() {
    local parent="${1:-.local/backups}" destination image
    if ! docker compose ps --status running --services | rg -qx api; then
        printf '%s\n' 'API must be running before a coordinated backup.' >&2
        return 1
    fi
    image="$(running_api_image)"
    mkdir -p "$parent"
    destination="$(mktemp -d "$parent/oa-backup.XXXXXXXX")"
    destination="$(cd "$destination" && pwd -P)"
    trap 'docker compose start --wait --wait-timeout 120 api >&2' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    docker compose stop -t 30 api >&2

    docker compose exec -T mysql sh -ec '
        MYSQL_PWD="$MYSQL_PASSWORD" mysqldump --single-transaction --skip-lock-tables \
          --no-tablespaces --set-gtid-purged=OFF --hex-blob -u "$MYSQL_USER" "$MYSQL_DATABASE"
    ' > "$destination/mysql.sql"
    docker compose run --rm --no-deps -T --entrypoint tar api \
        -C /var/lib/oa -cf - private > "$destination/private.tar"
    test -s "$destination/mysql.sql"
    test -s "$destination/private.tar"
    printf '%s\n' "$image" > "$destination/API_IMAGE_ID"
    (cd "$destination" && shasum -a 256 mysql.sql private.tar API_IMAGE_ID > SHA256SUMS)
    docker compose start --wait --wait-timeout 120 api >&2
    trap - EXIT INT TERM
    printf '%s\n' "$destination"
}

validate_backup() {
    local archive="$1"
    python3 - "$archive" <<'PY'
import hashlib
from pathlib import Path
import re
import stat
import sys
import tarfile

root = Path(sys.argv[1])
if not root.is_dir() or stat.S_IMODE(root.stat().st_mode) & 0o077:
    raise SystemExit("Backup directory must restrict access to its owner")
members = {"mysql.sql", "private.tar", "API_IMAGE_ID", "SHA256SUMS"}
for filename in members:
    entry = root / filename
    if entry.is_symlink() or not entry.is_file() or stat.S_IMODE(entry.stat().st_mode) & 0o077:
        raise SystemExit("Missing, invalid or publicly accessible backup file")
lines = (root / "SHA256SUMS").read_text().splitlines()
expected = {}
for line in lines:
    match = re.fullmatch(r"([a-f0-9]{64})  (mysql\.sql|private\.tar|API_IMAGE_ID)", line)
    if not match or match[2] in expected:
        raise SystemExit("Invalid backup checksum list")
    expected[match[2]] = match[1]
if set(expected) != members - {"SHA256SUMS"}:
    raise SystemExit("Backup checksum list is incomplete")
for filename, digest in expected.items():
    actual = hashlib.sha256()
    with (root / filename).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            actual.update(chunk)
    if actual.hexdigest() != digest:
        raise SystemExit("Backup checksum mismatch")
if not re.fullmatch(r"sha256:[a-f0-9]{64}\n", (root / "API_IMAGE_ID").read_text()):
    raise SystemExit("Invalid backup API image identifier")
with tarfile.open(root / "private.tar") as archive:
    seen = set()
    for member in archive:
        if member.name in seen:
            raise SystemExit("Duplicate private archive member")
        seen.add(member.name)
        if member.name == "private" and member.isdir() and member.mode & 0o777 == 0o700:
            continue
        if member.isfile() and member.mode & 0o777 == 0o600 and re.fullmatch(r"private/[a-f0-9]{32}", member.name):
            continue
        raise SystemExit("Unexpected private archive member")
    if "private" not in seen:
        raise SystemExit("Private archive directory is missing")
PY
}

restore() {
    local archive="${1:?Specify the backup directory}"
    local employee_state="${2:?Specify the synthetic employee fixture state}"
    local approval_state="${3:?Specify the synthetic approval fixture state}"
    local m3_state="${4:-}"
    local m4_state="${5:-}"
    local -a extra_checks=()
    local project address image mysql_id api_id
    validate_backup "$archive"
    test -r "$employee_state"
    test -r "$approval_state"
    if [[ -n "$m3_state" ]]; then
        test -r "$m3_state"
        extra_checks+=(--m3-state "$m3_state")
    fi
    if [[ -n "$m4_state" ]]; then
        test -r "$m4_state"
        extra_checks+=(--m4-state "$m4_state")
    fi
    set -a
    source ./.env
    set +a
    project="hr-oa-restore-$(openssl rand -hex 6)"
    export OA_RESTORE_DB_PASSWORD OA_RESTORE_DB_ROOT_PASSWORD OA_RESTORE_JWT_SECRET OA_RESTORE_API_IMAGE
    OA_RESTORE_DB_PASSWORD="$(openssl rand -hex 32)"
    OA_RESTORE_DB_ROOT_PASSWORD="$(openssl rand -hex 32)"
    OA_RESTORE_JWT_SECRET="$(openssl rand -hex 32)"
    image="$(< "$archive/API_IMAGE_ID")"
    # Use the exact local API image that produced the snapshot, without a registry pull.
    docker image inspect "$image" --format '{{.Id}}' >/dev/null
    OA_RESTORE_API_IMAGE="hr-oa-api-restore:${image#sha256:}"
    docker image tag "$image" "$OA_RESTORE_API_IMAGE"

    isolated() {
        docker compose --project-name "$project" --file deploy/oa/restore.compose.yaml "$@"
    }
    trap 'isolated down >&2' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    isolated up -d --pull never --wait --wait-timeout 120 mysql >&2
    mysql_id="$(isolated ps -q mysql)"
    [[ "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$mysql_id")" == "$project" ]]
    docker inspect --format '{{json .HostConfig.Tmpfs}}' "$mysql_id" | rg -Fq '"/var/lib/mysql"'
    isolated exec -T mysql sh -ec '
        MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch -u "$MYSQL_USER" "$MYSQL_DATABASE"
    ' < "$archive/mysql.sql"
    isolated up -d --pull never --wait --wait-timeout 120 api >&2
    api_id="$(isolated ps -q api)"
    [[ "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$api_id")" == "$project" ]]
    [[ "$(docker inspect --format '{{len .Mounts}}' "$api_id")" == 0 ]]
    isolated exec -T api tar -xmf - -C /var/lib/oa < "$archive/private.tar"
    address="$(isolated port api 8888)"
    [[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]]
    python3 -B tests/oa_restore_verify.py --base "http://$address" \
        --employee-state "$employee_state" --approval-state "$approval_state" ${extra_checks[@]+"${extra_checks[@]}"}
    isolated down >&2
    trap - EXIT INT TERM
}

case "${1:-}" in
    backup) shift; backup "$@" ;;
    restore) shift; restore "$@" ;;
    *)
        printf 'Usage: %s backup [private-parent] | restore <backup-directory> <employee-state.json> <approval-state.json> [m3-state.json] [m4-state.json]\n' "$0" >&2
        exit 2
        ;;
esac
