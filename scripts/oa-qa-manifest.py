"""Print a SHA-256 manifest for deployed OA sources and the full pinned Pure tree."""
import hashlib
from pathlib import Path, PurePosixPath
import subprocess

ROOT = Path(__file__).resolve().parents[1]
SCOPE = (
    "AGENTS.md", ".gitignore", ".dockerignore", "Dockerfile", "compose.yaml",
    "deploy/oa", "scripts", "tests", "docs/OA_API_CONTRACT.md", "docs/M2_API_CONTRACT.md",
    "docs/M3_API_CONTRACT.md", "docs/M3_USER_GUIDE.md", "docs/PURE_ADMIN_INTEGRATION.md",
    "docs/PURE_ADMIN_CHANGES.md", "docs/INTEGRATION_STATUS.md", "docs/LOCAL_DEVELOPMENT.md", "docs/PURE_ADMIN_UPSTREAM.json", "docs/PURE_ADMIN_UPSTREAM_SHA256SUMS",
    "server/go.mod", "server/go.sum", "server/cmd/oa", "server/oa",
    "web/.gitignore", "web/h5.html", "web/oa.html", "web/vite.oa.config.js",
    "web/package.json", "web/package-lock.json", "web/src/oa", "web/src/h5", "web-pure-admin",
)


def digest(path):
    return hashlib.sha256((ROOT / path).read_bytes()).hexdigest()


tracked = subprocess.check_output(
    ["git", "ls-files", "-z", "-c", "-o", "--exclude-standard", "--", *SCOPE], cwd=ROOT,
).decode().split("\0")
original = {}
for line in (ROOT / "docs/PURE_ADMIN_UPSTREAM_SHA256SUMS").read_text().splitlines():
    expected, path = line.split("  ", 1)
    assert path.startswith("web-pure-admin/") and ".." not in PurePosixPath(path).parts
    original[path] = expected
assert len(original) == 636

for path in sorted(set(tracked) | set(original)):
    parts = PurePosixPath(path).parts
    if not path or any(part in ("node_modules", ".local", ".pnpm-store", "dist", "dist-oa") for part in parts):
        continue
    if PurePosixPath(path).name.startswith(".env"):
        # The only environment files eligible are verified upstream source files.
        if path not in original:
            continue
        if digest(path) != original[path]:
            raise SystemExit(f"Upstream environment template differs from pin: {path}")
    print(f"{digest(path)}  {path}")
