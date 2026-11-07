"""Build or verify a credential-free inventory of an isolated OA restore."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tarfile
from urllib.parse import urlsplit
from urllib.request import ProxyHandler, build_opener


def file_inventory(archive):
    result = {}
    for member in archive:
        if member.isdir() and member.name == "private":
            continue
        if not member.isfile() or not re.fullmatch(r"private/[0-9a-f]{32}", member.name) or member.name in result:
            raise SystemExit("Invalid or duplicate private file in inventory")
        digest = hashlib.sha256()
        with archive.extractfile(member) as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        result[member.name] = {"bytes": member.size, "sha256": digest.hexdigest()}
    return result


def restored_files(compose):
    with subprocess.Popen([*compose, "exec", "-T", "api", "tar", "-cf", "-", "-C", "/var/lib/oa", "private"],
                          stdout=subprocess.PIPE) as process:
        with tarfile.open(fileobj=process.stdout, mode="r|") as archive:
            found = file_inventory(archive)
        if process.wait() != 0:
            raise SystemExit("Isolated API file inventory failed")
    return found


def table_inventory(compose):
    command = [*compose, "exec", "-T", "mysql", "sh", "-ec",
               'test "$MYSQL_DATABASE" = hr_oa_restore; MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"']

    def query(sql):
        result = subprocess.run(command, input=sql, text=True, capture_output=True, timeout=45)
        if result.returncode:
            raise SystemExit("Isolated MySQL inventory failed")
        return result.stdout.strip().splitlines()

    tables = query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() "
                   "AND table_type='BASE TABLE' AND (LEFT(table_name,3)='oa_' OR table_name='sys_users') ORDER BY table_name;")
    if not {"oa_employees", "oa_contracts", "oa_contract_files", "sys_users"} <= set(tables):
        raise SystemExit("Required OA tables missing from isolated database")
    if any(not re.fullmatch(r"[a-z0-9_]+", table) for table in tables):
        raise SystemExit("Unexpected MySQL table identifier")
    if query("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() "
             "AND table_name='oa_contract_files' AND column_name='storage_backend';") == ["1"]:
        if query("SELECT COUNT(*) FROM oa_contract_files WHERE deleted_at IS NULL AND storage_backend<>'local';") != ["0"]:
            raise SystemExit("Remote objects require a separate coordinated snapshot")
    counts = query(" UNION ALL ".join(f"SELECT '{table}', COUNT(*) FROM `{table}`" for table in tables) + ";")
    keys = query("SELECT storage_key FROM oa_contract_files WHERE deleted_at IS NULL ORDER BY id;")
    if any(not re.fullmatch(r"[0-9a-f]{32}", key) for key in keys):
        raise SystemExit("Invalid storage key in isolated database inventory")
    return {name: int(count) for name, count in (line.split("\t", 1) for line in counts)}, keys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", required=True)
    parser.add_argument("--base", required=True)
    parser.add_argument("mode", choices=("generate", "verify"))
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    origin = urlsplit(args.base)
    if not re.fullmatch(r"hr-oa-restore-[0-9a-f]{12}", args.project) or (
            origin.scheme != "http" or origin.hostname != "127.0.0.1" or not origin.port or origin.path or
            origin.username or origin.password or origin.query or origin.fragment):
        parser.error("Use the isolated local restore script")
    compose = ["docker", "compose", "--env-file", "/dev/null", "--project-name", args.project,
               "--file", "deploy/oa/restore.compose.yaml"]
    with build_opener(ProxyHandler({})).open(args.base + "/api/oa/v1/health", timeout=10) as reply:
        if reply.status != 200 or json.load(reply)["data"]["database"] != "ok":
            raise SystemExit("Isolated API health failed")
    with tarfile.open(args.archive / "private.tar", mode="r:") as archive:
        archived = file_inventory(archive)
    restored = restored_files(compose)
    if archived != restored:
        raise SystemExit("Isolated API file checksums do not match the archive")
    tables, keys = table_inventory(compose)
    if any("private/" + key not in restored for key in keys):
        raise SystemExit("Isolated database has a missing private attachment")
    inventory = {"formatVersion": 1, "tables": tables, "attachments": restored}
    if args.mode == "generate":
        print(json.dumps(inventory, sort_keys=True, indent=2))
        return
    expected = json.loads((args.archive / "RESTORE_INVENTORY.json").read_text())
    if expected != inventory:
        raise SystemExit("Isolated restore inventory differs from the portable backup")
    print(f"PASS portable restore: API health, {len(inventory['tables'])} table counts, "
          f"and {len(restored)} attachment byte hashes match")


if __name__ == "__main__":
    main()
