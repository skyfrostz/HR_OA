"""Capture timed HR OA contract transitions with viewport video, frame samples and Chromium tracing."""
import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_ui_ux_browser import ADMIN, Capture, Probe, Traffic, button, goto, heading, login, monitor_ready, set_theme

CONDITIONS = (
    (1440, 900, "light", "no-preference"),
    (1366, 768, "dark", "no-preference"),
    (390, 844, "light", "no-preference"),
    (360, 800, "dark", "no-preference"),
    (320, 740, "dark", "reduce"),
    (1440, 900, "light", "reduce"),
)

FRAME_SAMPLER = """button => {
  const startedAt = performance.now();
  const epochMs = performance.timeOrigin + startedAt;
  const target = (button.closest('tr')?.cells[1] ||
    button.closest('.oa-reminder-list li')?.querySelector('p'))?.textContent.trim();
  const initialRoute = location.hash.split('?')[0];
  const frames = [], scroll = [], focus = [], animations = [];
  const eventTime = () => +(performance.now() - startedAt).toFixed(2);
  const route = () => location.hash.split('?')[0];
  const describe = e => e?.id || String(e?.className || e?.tagName || '').slice(0, 90);
  function onScroll(e) {
    scroll.push([eventTime(), describe(e.target),
      e.target.scrollTop ?? scrollY, e.target.scrollLeft ?? scrollX]);
  }
  function onFocus(e) { focus.push([eventTime(), describe(e.target)]); }
  function onMotion(e) {
    animations.push([eventTime(), e.type, describe(e.target), e.propertyName || e.animationName]);
  }
  const motionEvents = ['transitionrun', 'transitionend', 'transitioncancel',
    'animationstart', 'animationend', 'animationcancel'];
  addEventListener('scroll', onScroll, true);
  addEventListener('focusin', onFocus, true);
  for (const type of motionEvents) addEventListener(type, onMotion, true);
  function sample() {
    const dialog = document.querySelector('.el-drawer[role="dialog"], [role="dialog"]');
    const rect = dialog?.getBoundingClientRect();
    const main = document.querySelector('.app-main, .app-main-nofixed-header');
    const children = [...(dialog?.querySelectorAll('h1,h2,h3,h4,dd,span') || [])];
    const label = dialog?.querySelector('h1,h2,h3,[role="heading"], .el-drawer__title');
    frames.push({
      ms: eventTime(), route: route(),
      reminders: !!main?.querySelector('#oa-reminder-heading'),
      employeeDirectory: !!main?.querySelector('#employees-heading, [aria-label="员工档案"]'),
      mainChild: describe(main?.firstElementChild), focus: describe(document.activeElement),
      scroll: [scrollY, main?.querySelector('.el-scrollbar__wrap')?.scrollTop ?? main?.scrollTop,
        dialog?.querySelector('.el-drawer__body')?.scrollTop, dialog?.closest('.el-overlay')?.scrollLeft],
      dialog: rect ? [rect.x, rect.y, rect.width, rect.height].map(v => +v.toFixed(1)) : null,
      label: label?.textContent.trim().slice(0, 45),
      loading: !!dialog?.querySelector('.el-skeleton, [aria-busy="true"], [role="status"][aria-label="正在加载"]'),
      drawerInRoute: !!dialog?.closest('.app-main, .app-main-nofixed-header'),
      targetVisible: !!target && children.some(e => e.children.length === 0 && e.textContent.trim() === target),
      viewport: [innerWidth, innerHeight, document.documentElement.scrollWidth]
    });
    if (performance.now() - startedAt < 2700) requestAnimationFrame(sample);
    else {
      removeEventListener('scroll', onScroll, true);
      removeEventListener('focusin', onFocus, true);
      for (const type of motionEvents) removeEventListener(type, onMotion, true);
      window.__oaMotion = {epochMs, initialRoute, frames, scroll, focus, animations, target};
    }
  }
  performance.mark('oa-contract-activate');
  sample();
  button.click();
}"""


def save_json(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def timed_transition(page, view, path, hold_ms):
    cdp = page.context.new_cdp_session(page)
    requests, by_id, finished = [], {}, []
    def started(e):
        url = urlsplit(e["request"]["url"])
        if not url.path.startswith("/api/oa/v1/"):
            return
        item = {
            "method": e["request"]["method"],
            "path": re.sub(r"/\d+(?=/|$)", "/:id", url.path),
            "start": e["timestamp"], "epochMs": 1000 * e["wallTime"],
        }
        by_id[e["requestId"]] = item
        requests.append(item)
    def updated(e, key, value):
        if e["requestId"] in by_id:
            by_id[e["requestId"]][key] = value
    cdp.on("Network.requestWillBeSent", started)
    cdp.on("Network.responseReceived", lambda e: updated(e, "response", [e["timestamp"], e["response"]["status"]]))
    cdp.on("Network.loadingFinished", lambda e: updated(e, "end", e["timestamp"]))
    cdp.on("Network.loadingFailed", lambda e: updated(e, "failed", [e["timestamp"], e.get("canceled", False)]))
    cdp.on("Tracing.tracingComplete", lambda event: finished.append(event))
    cdp.send("Network.enable")
    cdp.send("Tracing.start", {"transferMode": "ReturnAsStream",
                              "categories": "devtools.timeline,blink.user_timing,loading,disabled-by-default-devtools.timeline.frame"})
    pattern = re.compile(r"/api/oa/v1/contracts/\d+$")
    if hold_ms:
        def delay(route):
            # Hold a genuine response; request, status and bytes still come from OA.
            response = route.fetch()
            page.wait_for_timeout(hold_ms)
            try:
                route.fulfill(response=response)
            finally:
                response.dispose()
        page.route(pattern, delay)
    try:
        view.evaluate(FRAME_SAMPLER)
        page.wait_for_function("window.__oaMotion !== undefined", timeout=9000)
        evidence = page.evaluate("window.__oaMotion")
        evidence["network"] = requests
        for item in requests:
            item["startAfterClickMs"] = round(item["epochMs"] - evidence["epochMs"], 2)
            if "response" in item:
                item["responseAfterClickMs"] = round(item["startAfterClickMs"] + 1000 * (item["response"][0] - item["start"]), 2)
    finally:
        cdp.send("Tracing.end")
        if hold_ms:
            page.unroute(pattern, delay)
    for _ in range(30):
        if finished:
            break
        page.wait_for_timeout(100)
    assert finished, "Chromium trace stream did not finish"
    stream = finished[0]["stream"]
    with (path / "chrome-trace.json").open("w") as handle:
        while True:
            chunk = cdp.send("IO.read", {"handle": stream})
            assert not chunk.get("base64Encoded"), "expected UTF-8 Chromium trace stream"
            handle.write(chunk["data"])
            if chunk["eof"]:
                break
    cdp.send("IO.close", {"handle": stream})
    cdp.detach()
    return evidence


def capture(browser, args, condition, hold_ms):
    width, height, theme, motion = condition
    name = f"{width}x{height}-{theme}-{motion}" + (f"-hold{hold_ms}" if hold_ms else "")
    path = args.artifacts / name
    path.mkdir(parents=True, exist_ok=True)
    dimensions = {"width": width, "height": height}
    ctx = browser.new_context(viewport=dimensions, device_scale_factor=1, is_mobile=width < 600,
                              has_touch=width < 600, color_scheme=theme, reduced_motion=motion,
                              locale="zh-CN", timezone_id="Asia/Shanghai", record_video_dir=path,
                              record_video_size=dimensions)
    page = ctx.new_page()
    video = page.video
    try:
        probe = Probe(page)
        page.goto(args.base + "/oa.html")
        Capture.auth_retry(None, lambda: login(page, ADMIN, Traffic(ctx, args.base)), probe)
        expect(heading(page, "员工档案")).to_be_visible()
        if theme == "dark":
            set_theme(page)
        with page.expect_response(lambda r: r.ok and "/departments" in urlsplit(r.url).path):
            goto(page, "/departments")
        page.wait_for_load_state("networkidle")
        departments_view = page.evaluate("""() => {
          const node = document.querySelector('.oa-org-tree-name, .oa-department-list li');
          const root = document.querySelector('.oa-departments');
          const options = root?.querySelector('details');
          return {width: innerWidth, height: innerHeight, documentWidth: document.documentElement.scrollWidth,
            firstDepartmentTop: node?.getBoundingClientRect().top ?? null,
            mobileOptionsOpen: options?.open ?? null, rootKind: root ? 'organization' : 'directory'};
        }""")
        page.screenshot(path=path / "departments.png", animations="disabled")
        goto(page, "/contract-reminders")
        region, state = monitor_ready(page, "/contract-reminders", probe)
        assert state == "rows", "local data must include at least one contract reminder"
        view = button(region, "查看合同").first
        expect(view).to_be_enabled()
        view.scroll_into_view_if_needed()
        page.wait_for_load_state("networkidle")
        page.screenshot(path=path / "contract-before-click.png", animations="disabled")
        evidence = timed_transition(page, view, path, hold_ms)
        page.screenshot(path=path / "contract-after-click.png", animations="disabled")
        evidence.update({"runtime": args.runtime, "role": ADMIN, "browser": browser.version,
                         "condition": condition, "contractResponseHoldMs": hold_ms,
                         "departmentViewport": departments_view,
                         "networkClock": "CDP monotonic timestamp plus epoch mapped to performance.timeOrigin",
                         "frameClock": "requestAnimationFrame relative to activation; geometry reads may add observer overhead"})
        save_json(path / "timeline.json", evidence)
    finally:
        ctx.close()
        video.save_as(path / "sequence.webm")
        video.delete()
    files = ("departments.png", "contract-before-click.png", "contract-after-click.png",
             "timeline.json", "chrome-trace.json", "sequence.webm")
    return name, {name: hashlib.sha256((path / name).read_bytes()).hexdigest() for name in files}


def describe_capture(path, files):
    for name, digest in files.items():
        assert hashlib.sha256((path / name).read_bytes()).hexdigest() == digest, (path, name)
    data = json.loads((path / "timeline.json").read_text())
    trace = json.loads((path / "chrome-trace.json").read_text())
    frames = data["frames"]
    drawers = [frame for frame in frames if frame["dialog"] and frame["dialog"][2] > 0]
    if not drawers:
        raise AssertionError(f"No drawer frames: {path}")
    widths = sorted({frame["dialog"][2] for frame in drawers})
    direct = [request for request in data["network"] if request["path"] == "/api/oa/v1/contracts/:id"]
    assert len(direct) == 1 and direct[0]["response"][1] == 200, path
    first_target = next((frame["ms"] for frame in drawers if frame["targetVisible"]), None)
    overlay_x = max(abs(frame["scroll"][3] or 0) for frame in drawers)
    counters = Counter(event.get("name") for event in trace["traceEvents"])
    return {
        "role": data["role"], "browser": data["browser"], "condition": data["condition"], "target": data["target"],
        "runtime": data["runtime"], "sampledFrames": len(frames),
        "routes": sorted({frame["route"] for frame in frames}),
        "employeeFrames": sum(frame["employeeDirectory"] for frame in frames),
        "drawerInRouteFrames": (sum(bool(frame["drawerInRoute"]) for frame in drawers)
                                if "drawerInRoute" in drawers[0] else None),
        "firstDrawerMs": drawers[0]["ms"], "firstTargetTextMs": first_target,
        "focus": data["focus"], "widths": widths, "overlayScrollXMax": overlay_x,
        "mainScrollRange": sorted({frame["scroll"][1] for frame in frames if frame["scroll"][1] is not None}),
        "loadingFrames": (sum(bool(frame["loading"]) for frame in drawers)
                          if "loading" in drawers[0] else None),
        "detailRequestMs": direct[0]["startAfterClickMs"], "detailResponseMs": direct[0]["responseAfterClickMs"],
        "requests": [[request["method"], request["path"]] for request in data["network"]],
        "transitions": [event for event in data["animations"] if event[1] in ("animationstart", "transitionrun")
                        and event[3] in ("transform", "opacity")],
        "traceEventCounts": {event: counters[event] for event in ("Layout", "Paint", "DrawFrame", "UpdateLayoutTree")},
        "departmentViewport": data.get("departmentViewport"),
        "files": {name: {"path": str(path / name), "sha256": digest} for name, digest in files.items()},
    }


def gallery(root, pairs, ffmpeg, clip_starts):
    department = {}
    for phase in ("before", "after"):
        folder = root / (phase + "-departments")
        index = json.loads((folder / "index.json").read_text())
        assert "FAIL" not in index["latestChecks"].values()
        for key, shot in index["shots"].items():
            source = folder / shot["file"]
            assert hashlib.sha256(source.read_bytes()).hexdigest() == shot["sha256"]
            department.setdefault(key, {})[phase] = {
                "actualRuntime": index["runtime"], "path": str(source.resolve()), "sha256": shot["sha256"],
                "state": shot["state"], "viewport": shot["requestedViewport"], "theme": shot["requestedTheme"],
                "role": shot["role"],
            }
    for key, item in department.items():
        assert "after" in item, key
        if "before" not in item:
            assert key.startswith("department-real-details-"), key
        else:
            assert all(item["before"][field] == item["after"][field]
                       for field in ("viewport", "theme", "role")), key

    clips = {}
    for name in ("1440x900-light-no-preference", "390x844-light-no-preference"):
        clips[name] = {}
        for phase in ("before", "after"):
            evidence = pairs[name][phase]
            source = Path(evidence["files"]["sequence.webm"]["path"])
            record = {"runtime": evidence["runtime"], "condition": evidence["condition"],
                      "internalRawVideo": str(source.resolve())}
            if ffmpeg:
                # The operator aligns the trim to a visible reminder frame.
                # Raw recordings begin with sign-in and cannot be published.
                start = clip_starts[name][phase]
                assert isinstance(start, (int, float)) and start > 0
                directory = root / "gallery-clips"
                directory.mkdir(exist_ok=True)
                clip = directory / f"{phase}-{name}.webm"
                subprocess.run([str(ffmpeg), "-hide_banner", "-loglevel", "error", "-y", "-threads", "1",
                                "-i", str(source), "-ss", str(start), "-t", "3.35", "-c:v", "libvpx",
                                "-deadline", "realtime", "-cpu-used", "4", "-threads", "2",
                                "-crf", "18", "-b:v", "0", str(clip)], check=True)
                record.update({"path": str(clip.resolve()), "sha256": hashlib.sha256(clip.read_bytes()).hexdigest(),
                               "startSeconds": start, "editing": "trimmed to the inspected reminder viewport and encoded"})
            clips[name][phase] = record
    save_json(root / "gallery-materials.json", {
        "department": department, "contractClips": clips,
        "scope": "Synthetic local QA/demo records. Raw videos are internal; visually confirm clips before publication."
    })


def compare(root, ffmpeg, clip_starts):
    before, after = (json.loads((root / phase / "index.json").read_text()) for phase in ("before", "after"))
    assert before["captures"].keys() == after["captures"].keys()
    pairs = {}
    for name, a in before["captures"].items():
        b = after["captures"][name]
        old, new = (describe_capture(root / phase / name, files)
                    for phase, files in (("before", a), ("after", b)))
        assert all(old[key] == new[key] for key in ("role", "browser", "condition", "target")), name
        assert old["target"] and new["firstTargetTextMs"] is not None, name
        failures = []
        if new["employeeFrames"] or new["routes"] != ["#/contract-reminders"]:
            failures.append("intermediate route or employee directory")
        if new["drawerInRouteFrames"]:
            failures.append("drawer is inside the route transform container")
        if len(new["widths"]) != 1 or new["widths"][0] != min(new["condition"][0], 880):
            failures.append("drawer changed width or differs from target width")
        if new["overlayScrollXMax"] > 1 or len(new["mainScrollRange"]) > 1:
            failures.append("opening changed underlying or overlay scroll")
        if any(re.search(r"/(?:employees|departments)(?:/|$)", path) for _, path in new["requests"]):
            failures.append("unexpected employee/department request")
        if "hold700" in name and (new["firstDrawerMs"] >= new["detailResponseMs"] or not new["loadingFrames"]):
            failures.append("missing visible loading before delayed response")
        if new["condition"][3] == "reduce" and any(item[3] == "transform" for item in new["transitions"]):
            failures.append("transform ran under reduced motion")
        if sum(event[3] == "transform" and "el-drawer" in event[2] for event in new["transitions"]) > 1:
            failures.append("drawer transform repeated")
        organization = new["departmentViewport"]
        if organization["documentWidth"] > organization["width"] + 1:
            failures.append("department page overflows the viewport")
        if new["condition"][0] < 600 and (organization["mobileOptionsOpen"] is not False or
                                           organization["firstDepartmentTop"] is None or
                                           organization["firstDepartmentTop"] >= organization["height"]):
            failures.append("department mobile default is missing a visible node or has open options")
        pairs[name] = {"before": old, "after": new, "failures": failures}
        print(name, f"drawer {old['firstDrawerMs']}→{new['firstDrawerMs']}ms, "
                    f"GET {old['detailRequestMs']}→{new['detailRequestMs']}ms",
              "PASS" if not failures else failures)
    before_data, after_data = (root / f"{phase}-fixture.json" for phase in ("baseline", "candidate"))
    old, new = (json.loads(path.read_text()) for path in (before_data, after_data))
    assert old == new and len(old) == 33
    save_json(root / "motion-pairs.json", {
        "pairs": pairs, "fixtureFingerprintsMatched": len(old),
        "fixtureSources": [str(path.resolve()) for path in (before_data, after_data)]
    })
    gallery(root, pairs, ffmpeg, clip_starts)
    return int(any(item["failures"] for item in pairs.values()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--runtime")
    parser.add_argument("--compare", action="store_true")
    parser.add_argument("--ffmpeg", type=Path, help="optional ffmpeg executable for trimmed gallery clips")
    parser.add_argument("--clip-starts", type=Path, help="JSON of inspected trim times by condition and phase")
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()
    if args.compare:
        if bool(args.ffmpeg) != bool(args.clip_starts):
            parser.error("--ffmpeg and --clip-starts must be supplied together")
        clip_starts = json.loads(args.clip_starts.read_text()) if args.clip_starts else None
        return compare(args.artifacts, args.ffmpeg, clip_starts)
    if not args.runtime:
        parser.error("--runtime is required for capture")
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("export OA_SEED_PASSWORD")
    index = {}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            for condition, hold in [(item, 0) for item in CONDITIONS] + [(CONDITIONS[0], 700)]:
                name, files = capture(browser, args, condition, hold)
                index[name] = files
                save_json(args.artifacts / "index.json", {"runtime": args.runtime, "captures": index})
                print(f"Captured {name}", flush=True)
        finally:
            browser.close()


if __name__ == "__main__":
    raise SystemExit(main())
