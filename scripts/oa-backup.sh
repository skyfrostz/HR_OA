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
    local parent="${1:-.local/backups}" destination image storage_column remote_files
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

    storage_column="$(docker compose exec -T mysql sh -ec '
        MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"
    ' <<'SQL'
SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name='oa_contract_files' AND column_name='storage_backend';
SQL
    )"
    if [[ "$storage_column" == 1 ]]; then
        remote_files="$(docker compose exec -T mysql sh -ec '
            MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"
        ' <<'SQL'
SELECT COUNT(*) FROM oa_contract_files WHERE deleted_at IS NULL AND storage_backend <> 'local';
SQL
        )"
        if [[ "$remote_files" != 0 ]]; then
            printf '%s\n' 'Local backup requires a separate coordinated OSS snapshot for remote attachments.' >&2
            return 1
        fi
    fi

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

pack_backup() {
    local source="${1:?Specify the backup directory}"
    local parent="${2:-.local/backups}" destination api_image mysql_image file
    validate_backup "$source"
    api_image="$(< "$source/API_IMAGE_ID")"
    docker image inspect "$api_image" --format '{{.Id}}' >/dev/null
    mysql_image="$(docker image inspect mysql:8.4.8 --format '{{.Id}}')"
    mkdir -p "$parent"
    destination="$(mktemp -d "$parent/oa-portable.XXXXXXXX")"
    for file in mysql.sql private.tar API_IMAGE_ID SHA256SUMS; do
        cp "$source/$file" "$destination/$file"
    done
    printf '%s\n' "$mysql_image" > "$destination/MYSQL_IMAGE_ID"
    docker image save "$api_image" mysql:8.4.8 | gzip -1 > "$destination/images.tar.gz"
    restore_runtime "$destination" inventory generate "$destination" > "$destination/RESTORE_INVENTORY.json"
    (cd "$destination" && shasum -a 256 MYSQL_IMAGE_ID images.tar.gz RESTORE_INVENTORY.json > IMAGE_SHA256SUMS)
    validate_portable "$destination"
    (cd "$destination" && pwd -P)
}

validate_portable() {
    local archive="$1"
    validate_backup "$archive"
    python3 - "$archive" <<'PY'
import hashlib
from pathlib import Path
import re
import stat
import sys

root = Path(sys.argv[1])
names = {"MYSQL_IMAGE_ID", "images.tar.gz", "RESTORE_INVENTORY.json"}
for name in names | {"IMAGE_SHA256SUMS"}:
    path = root / name
    if path.is_symlink() or not path.is_file() or stat.S_IMODE(path.stat().st_mode) & 0o077:
        raise SystemExit("Invalid or publicly accessible portable image file")
expected = {}
for line in (root / "IMAGE_SHA256SUMS").read_text().splitlines():
    match = re.fullmatch(r"([a-f0-9]{64})  (MYSQL_IMAGE_ID|images\.tar\.gz|RESTORE_INVENTORY\.json)", line)
    if not match or match[2] in expected:
        raise SystemExit("Invalid portable checksum list")
    expected[match[2]] = match[1]
if set(expected) != names:
    raise SystemExit("Incomplete portable checksum list")
for name, digest in expected.items():
    actual = hashlib.sha256()
    with (root / name).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            actual.update(chunk)
    if actual.hexdigest() != digest:
        raise SystemExit("Portable checksum mismatch")
if not re.fullmatch(r"sha256:[a-f0-9]{64}\n", (root / "MYSQL_IMAGE_ID").read_text()):
    raise SystemExit("Invalid portable MySQL image ID")
PY
}

load_images() {
    local archive="${1:?Specify the portable backup directory}"
    validate_portable "$archive"
    docker image load --input "$archive/images.tar.gz"
    docker image inspect "$(< "$archive/API_IMAGE_ID")" --format '{{.Id}}' >/dev/null
    if [[ "$(docker image inspect mysql:8.4.8 --format '{{.Id}}')" != "$(< "$archive/MYSQL_IMAGE_ID")" ]]; then
        printf '%s\n' 'Loaded MySQL image does not match the portable backup.' >&2
        return 1
    fi
}

restore() {
    local archive="${1:?Specify the backup directory}"
    local employee_state="${2:?Specify the synthetic employee fixture state}"
    local approval_state="${3:?Specify the synthetic approval fixture state}"
    local m3_state="${4:-}"
    local m4_state="${5:-}"
    local m5_state="${6:-}"
    local m6_state="${7:-}"
    local -a extra_checks=()
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
    if [[ -n "$m5_state" ]]; then
        test -r "$m5_state"
        extra_checks+=(--m5-state "$m5_state")
    fi
    if [[ -n "$m6_state" ]]; then
        test -r "$m6_state"
        extra_checks+=(--m6-state "$m6_state")
    fi
    if [[ -z "${OA_SEED_PASSWORD:-}" && -r ./.env ]]; then
        set -a
        source ./.env
        set +a
    fi
    : "${OA_SEED_PASSWORD:?Provide the original synthetic fixture login password separately}"
    export OA_SEED_PASSWORD
    restore_runtime "$archive" fixtures --employee-state "$employee_state" --approval-state "$approval_state" \
        ${extra_checks[@]+"${extra_checks[@]}"}
}

restore_runtime() (
    local archive="$1" mode="$2" project address image mysql_id api_id
    shift 2
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
        docker compose --env-file /dev/null --project-name "$project" --file deploy/oa/restore.compose.yaml "$@"
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
    if [[ "$mode" == inventory ]]; then
        python3 -B scripts/oa-restore-inventory.py --project "$project" --base "http://$address" "$@"
    else
        python3 -B tests/oa_restore_verify.py --base "http://$address" "$@"
    fi
    isolated down >&2
    trap - EXIT INT TERM
)

restore_check() {
    local archive="${1:?Specify the portable backup directory}"
    load_images "$archive" >&2
    restore_runtime "$archive" inventory verify "$archive"
}

case "${1:-}" in
    backup) shift; backup "$@" ;;
    pack) shift; pack_backup "$@" ;;
    verify) shift; validate_portable "${1:?Specify the portable backup directory}" ;;
    load-images) shift; load_images "$@" ;;
    restore-check) shift; restore_check "$@" ;;
    restore) shift; restore "$@" ;;
    *)
        printf 'Usage: %s backup [private-parent] | pack <backup-directory> [private-parent] | verify <portable-directory> | load-images <portable-directory> | restore-check <portable-directory> | restore <backup-directory> <employee-state.json> <approval-state.json> [m3-state.json] [m4-state.json] [m5-state.json] [m6-state.json]\n' "$0" >&2
        exit 2
        ;;
esac
