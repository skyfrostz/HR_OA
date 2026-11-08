"""Verify paired UI capture manifests and publish same-condition deltas."""
import argparse
import hashlib
import json
from pathlib import Path
from urllib.parse import quote


def read(root, phase, capture=None):
    folder = root / (capture or phase)
    index = json.loads((folder / "index.json").read_text())
    assert index["phase"] == phase
    for entry in index["shots"].values():
        name = Path(entry["file"])
        assert name == Path(name.name) and name.suffix == ".png"
        assert hashlib.sha256((folder / name).read_bytes()).hexdigest() == entry["sha256"], name
    return index


def final_manifest(root, release, delta, mock):
    before, c6 = (read(root, phase) for phase in ("before", "after"))
    folder = "after-pc-header-delta"
    c7 = read(root, "after", folder)
    assert before["browser"] == c6["browser"] == c7["browser"]
    assert c7["runtime"] == (delta / "SOURCE_VERSION").read_text().strip()
    assert c6["runtime"] == (release / "API_VERSION").read_text().strip()
    assert "FAIL" not in c7["latestChecks"].values(), c7["failedAttempts"]

    def hashes(path):
        return dict(reversed(line.split(maxsplit=1)) for line in path.read_text().splitlines())

    source6, source7 = (hashes(item / "SOURCE_SHA256SUMS") for item in (release, delta))
    changed = [p for p in sorted(source6.keys() | source7.keys()) if source6.get(p) != source7.get(p)]
    assert changed == ["web-pure-admin/src/layout/components/lay-navbar/index.vue"], changed
    h5_manifests = [item / "H5_ASSETS_SHA256" for item in (release, delta, mock)]
    h5 = hashes(h5_manifests[0])
    assert len(h5) == 7 and all(hashes(p) == h5 for p in h5_manifests)
    fixtures = [root / name for name in ("baseline-fixture.json", "candidate-fixture-header-delta.json")]
    first, last = (json.loads(p.read_text()) for p in fixtures)
    assert first == last
    fields = ("role", "requestedViewport", "requestedTheme", "state", "route")
    samples = {name: shot["contrast"]["navbarRoleText"] for name, shot in c7["shots"].items()
               if "navbarRoleText" in shot.get("contrast", {})}
    assert len(samples) >= 6 and min(samples.values()) >= 4.5, samples
    for name, shot in c7["shots"].items():
        assert all(shot["checks"].values()), name
        assert all(value >= 4.5 for value in shot.get("contrast", {}).values()), (name, shot["contrast"])
    data = {
        "beforeRuntime": before["runtime"], "releaseRuntime": c7["runtime"],
        "fullComparisonRuntime": c6["runtime"], "browser": c7["browser"],
        "root": str(root.resolve()), "changedSourceSinceFullComparison": changed,
        "images": (delta / "IMAGES").read_text().splitlines(),
        "fixtureFingerprints": {"count": len(first), "files": [p.name for p in fixtures],
                                "exceptions": "Temporary import preview IDs, expiry times and history rows."},
        "assetEquivalence": {"h5": {"statement": "H5 HTML and every packaged H5 asset are byte-identical across the full comparison, release and mock regression. Only PC navbar source changed.",
                                    "manifests": [str(p.resolve()) for p in h5_manifests], "sha256": h5}},
        "captureFailureHistory": [
            {"actualRuntime": c6["runtime"], "index": "after-release-race-first/first-capture-index.json",
             "attempts": c6["failedAttempts"]},
            {"actualRuntime": c7["runtime"], "index": folder + "/index.json", "attempts": c7["failedAttempts"]}],
        "navbarRoleTextContrast": samples, "pairs": {}
    }
    lines = ["# Final HR OA screenshot pairs", "",
             f"PC: fresh captures on `{c7['runtime']}`. H5: original captures on `{c6['runtime']}` with seven matching file hashes.",
             "", "| Scenario | Before | After | Actual after runtime |", "| --- | --- | --- | --- |"]

    def source(shot, directory, runtime):
        return {"path": f"{directory}/{shot['file']}", "actualRuntime": runtime, "sha256": shot["sha256"],
                "role": shot["role"], "state": shot["state"], "viewport": shot["requestedViewport"],
                "theme": shot["requestedTheme"], "route": shot["route"]}

    pc_count = 0
    for key, a in sorted(before["shots"].items()):
        is_h5 = a["route"] == "/h5.html"
        runtime, directory = (c6, "after") if is_h5 else (c7, folder)
        assert key in runtime["shots"], ("missing", key, directory)
        b = runtime["shots"][key]
        differences = {field: [a[field], b[field]] for field in fields if a[field] != b[field]}
        for axis in ("width", "height", "darkClass"):
            assert a["viewport"][axis] == b["viewport"][axis], (key, axis)
        if key.startswith("contract-reminder-double-activation-"):
            assert differences.pop("state") == ["target-lost", "target-open"]
        assert not differences, (key, differences)
        lhs, rhs = source(a, "before", before["runtime"]), source(b, directory, runtime["runtime"])
        if is_h5:
            rhs["assetEquivalence"] = "h5"
        else:
            pc_count += 1
        data["pairs"][key] = {"before": lhs, "after": rhs, "conditionDifferences": differences}
        lines.append(f"| {key} | [PNG]({quote(lhs['path'])}) | [PNG]({quote(rhs['path'])}) | `{rhs['actualRuntime']}` |")
    data["counts"] = {"paired": len(data["pairs"]), "pcRecaptured": pc_count, "h5Reused": len(data["pairs"]) - pc_count}
    (root / "pairs-final.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
    (root / "PAIRS_FINAL.md").write_text("\n".join(lines) + "\n")
    print("Final exact-runtime pairs:", data["counts"], "fixtures:", len(first),
          "navbar contrast samples:", len(samples), "minimum:", min(samples.values()))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("--final-runtime-manifests", nargs=3, type=Path, metavar=("FULL", "FINAL", "MOCK"))
    args = parser.parse_args()
    root = args.root
    if args.final_runtime_manifests:
        return final_manifest(root, *args.final_runtime_manifests)
    before, after = (read(root, phase) for phase in ("before", "after"))
    old, new = before["shots"], after["shots"]
    fields = ("role", "requestedViewport", "requestedTheme", "state")
    data = {"beforeRuntime": before["runtime"], "afterRuntime": after["runtime"],
            "browsers": [before["browser"], after["browser"]], "pairs": {},
            "missing": sorted(old.keys() - new.keys()), "afterOnly": sorted(new.keys() - old.keys()),
            "visualFailures": {}, "captureFailureHistory": after["failedAttempts"],
            "captureFailures": [item for item in after["failedAttempts"]
                                if after.get("latestChecks", {}).get(item["scenario"]) != "PASS"]}
    baseline_fixture = root / "baseline-fixture.json"
    candidate_fixture = root / "candidate-fixture.json"
    if baseline_fixture.is_file() and candidate_fixture.is_file():
        a, b = (json.loads(p.read_text()) for p in (baseline_fixture, candidate_fixture))
        data["fixtureFingerprints"] = {
            "matched": sorted(k for k in a if a[k] == b.get(k)),
            "differences": sorted(k for k in a.keys() | b.keys() if a.get(k) != b.get(k)),
            "notes": "Import preview IDs, expiry times and temporary history rows are generated for each capture."
        }
    lines = ["# HR OA browser screenshot pairs", "",
             f"Before: `{before['runtime']}`; after: `{after['runtime']}`.",
             "", "| Scenario | Before | After | Vertical position change (CSS px) |",
             "| --- | --- | --- | --- |"]
    for key, shot in new.items():
        failures = [label for label, passed in shot.get("checks", {}).items() if not passed]
        failures.extend(f"contrast {label} {ratio:.2f}:1" for label, ratio in shot.get("contrast", {}).items()
                        if ratio < 4.5)
        if failures:
            data["visualFailures"][key] = failures
    for key in sorted(old.keys() & new.keys()):
        a, b = old[key], new[key]
        differences = {field: [a[field], b[field]] for field in fields if a[field] != b[field]}
        actual_a = [a["viewport"]["width"], a["viewport"]["height"]]
        actual_b = [b["viewport"]["width"], b["viewport"]["height"]]
        if actual_a != actual_b:
            differences["actualViewport"] = [actual_a, actual_b]
        if key.startswith("contract-reminder-double-activation-"):
            # Scenario stimulus matches; this status is the behavior under test.
            differences.pop("state", None)
        positions = {}
        for name in sorted(a["anchors"].keys() & b["anchors"].keys()):
            left, right = a["anchors"][name], b["anchors"][name]
            if left and right:
                positions[name] = [left["y"], right["y"], right["y"] - left["y"]]
        data["pairs"][key] = {"before": a["file"], "after": b["file"], "role": b["role"], "state": b["state"],
                              "viewport": actual_b, "theme": b["requestedTheme"],
                              "sha256": {"before": a["sha256"], "after": b["sha256"]},
                              "conditionDifferences": differences, "y": positions}
        delta = " · ".join(f"{name}: {v[0]} → {v[1]} ({v[2]:+d})" for name, v in positions.items())
        if differences:
            delta = ("Condition mismatch: " + ", ".join(differences) + " · " + delta).rstrip(" ·")
        lines.append(f"| {key} | [PNG](before/{quote(a['file'])}) | [PNG](after/{quote(b['file'])}) | {delta} |")
    lines += ["", f"Missing after capture: {', '.join(data['missing']) or 'none'}.",
              f"After-only capture: {', '.join(data['afterOnly']) or 'none'}."]
    (root / "pairs.json").write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
    (root / "PAIRS.md").write_text("\n".join(lines) + "\n")
    mismatches = sum(bool(entry["conditionDifferences"]) for entry in data["pairs"].values())
    print(f"Matched {len(data['pairs'])}; missing {len(data['missing'])}; condition mismatches {mismatches}")
    if data["visualFailures"]:
        print(f"Measured visual failures: {len(data['visualFailures'])}")
    if data["captureFailures"]:
        print(f"Capture failures: {len(data['captureFailures'])}")
    return int(bool(data["missing"] or mismatches or data["visualFailures"] or data["captureFailures"]
                    or data.get("fixtureFingerprints", {}).get("differences")
                    or before["browser"] != after["browser"]))


if __name__ == "__main__":
    raise SystemExit(main())
