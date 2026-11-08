"""Paired HR OA screenshots and browser probes against the local Compose stack."""
import argparse
from collections import Counter
from datetime import datetime, timedelta
import hashlib
import json
import os
from pathlib import Path
import re
import time
from urllib.parse import urlsplit
from zoneinfo import ZoneInfo

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_m2_api import request_detail
from oa_m3_api import binary_workbook, call, fill_workbook
from oa_m5_api import annual as annual_balance
from oa_m5_browser import wait_write
from oa_browser import button
from oa_commercial_browser import contrast
from oa_m2_browser import login as h5_login, tab
from oa_m4_browser import directory, set_theme
from oa_pure_browser import Traffic, login

ADMIN, STAFF, MANAGER, VIEWER = "hr_admin", "employee", "manager", "hr_viewer"
ROUTES = {
    "/dashboard": ("工作台", ADMIN), "/employees": ("员工档案", ADMIN),
    "/departments": ("部门管理", ADMIN), "/employee-import": ("Excel 导入", ADMIN),
    "/contract-reminders": ("合同到期提醒", ADMIN), "/approvals": ("审批中心", MANAGER),
    "/mobile-approvals": ("移动审批入口", STAFF), "/my-leave": ("我的假期", STAFF),
    "/workflow-settings": ("请假与审批配置", ADMIN), "/approved-leaves": ("已批请假", ADMIN),
    "/workflow-exceptions": ("审批异常", ADMIN), "/integrations": ("集成状态", ADMIN),
    "/notifications": ("站内通知", STAFF),
}
DESKTOP = (1440, 900)
LAPTOP = (1366, 768)
PHONE = (390, 844)
LONG = "这是用于界面验收的合成说明，检查长中文内容换行、最后字段和提交操作的可达性。" * 6


def heading(page, name):
    if name == "员工档案":
        return page.get_by_role("heading", name=re.compile(r"^员工档案(?:\s*\d+\s*人)?$"), level=1)
    if name == "部门管理":
        return page.get_by_role("heading", name=re.compile(r"^部门管理(?:\s*\d+\s*个部门)?$"), level=1)
    return page.get_by_role("heading", name=name, exact=True)


def position(locator):
    if not locator.count() or not locator.first.is_visible():
        return None
    return locator.first.evaluate("""e => {
      const r=e.getBoundingClientRect(), s=getComputedStyle(e);
      const x=r.x+r.width/2, y=r.y+r.height/2, h=document.elementFromPoint(x,y);
      return {x:Math.round(r.x),y:Math.round(r.y),w:Math.round(r.width),h:Math.round(r.height),
        centerHit:h===e||e.contains(h), fontSize:s.fontSize, hasLabel:!!e.labels?.length,
        invalid:e.getAttribute('aria-invalid'), described:!!e.getAttribute('aria-describedby')};
    }""")


def placeholder_ratio(field):
    if not field.count() or not field.first.is_enabled():
        return None
    sample = field.first.evaluate("""e => {
 const ctx=document.createElement('canvas').getContext('2d',{willReadFrequently:true});
 function rgba(css) {
   ctx.clearRect(0,0,1,1);ctx.fillStyle=css;ctx.fillRect(0,0,1,1);
   const [r,g,b,a]=ctx.getImageData(0,0,1,1).data;return [r,g,b,a/255];
 }
 function layer(top,under) {
   return top.slice(0,3).map((v,i)=>top[3]*v+(1-top[3])*under[i]);
 }
 function lum(rgb) {
   const c=rgb.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4);
   return c[0]*.2126+c[1]*.7152+c[2]*.0722;
 }
 const chain=[];for(let n=e;n;n=n.parentElement)chain.push(rgba(getComputedStyle(n).backgroundColor));
 const bg=chain.reverse().reduce((under,top)=>layer(top,under),[255,255,255]);
 const p=getComputedStyle(e,'::placeholder'), color=rgba(p.color);
 color[3]*=Number(p.opacity);
 const text=layer(color,bg), a=lum(text), b=lum(bg);
 return {present:!!e.getAttribute('placeholder'),disabled:e.disabled,color:p.color,opacity:p.opacity,
         background:bg,ratio:(Math.max(a,b)+.05)/(Math.min(a,b)+.05)};
}""")
    return sample["ratio"] if sample["present"] and not sample["disabled"] else None


def api_path(url):
    return re.sub(r"/\d+(?=/|$)", "/:id", urlsplit(url).path)


class Probe:
    def __init__(self, page):
        self.events = []
        self.errors = []
        self.document = []
        self.retry_after = None
        page.on("response", self.response)
        page.on("requestfailed", self.failed)
        page.on("pageerror", lambda _: self.errors.append("uncaught-js"))
        page.on("console", lambda message: self.errors.append(
            "console-error:" + api_path(message.location.get("url", "")))
            if message.type == "error" else None)

    def response(self, response):
        request = response.request
        if request.is_navigation_request():
            self.document.append([api_path(response.url), response.status])
        if response.url.endswith("/api/oa/v1/auth/login"):
            self.retry_after = int(response.header_value("retry-after") or "0") if response.status == 429 else None
        if "/api/oa/v1/" in response.url:
            self.events.append([request.method, api_path(response.url), response.status])

    def failed(self, request):
        if "/api/oa/v1/" in request.url:
            self.events.append([request.method, api_path(request.url),
                                "aborted" if "ABORT" in (request.failure or "") else "network-failure"])


class Capture:
    def __init__(self, browser, base, args):
        self.browser, self.base, self.args = browser, base, args
        self.output = args.artifacts
        self.sessions = {}
        self.output.mkdir(parents=True, exist_ok=True)
        self.index_file = self.output / "index.json"
        self.index = json.loads(self.index_file.read_text()) if self.index_file.exists() else {
            "phase": args.phase, "runtime": args.runtime, "browser": browser.version,
            "shots": {}, "probes": {}, "failedAttempts": []}
        if self.index["runtime"] != args.runtime:
            raise ValueError("use another artifact directory for another runtime")
        self.index.setdefault("latestChecks", {})

    def save(self):
        tmp = self.index_file.with_suffix(".tmp")
        tmp.write_text(json.dumps(self.index, ensure_ascii=False, indent=2) + "\n")
        tmp.replace(self.index_file)

    def run(self, label, function, *args):
        try:
            function(*args)
            self.index["latestChecks"][label] = "PASS"
            self.save()
            print(f"PASS captured {label}", flush=True)
        except Exception as error:
            # Raw Playwright diagnostics may embed input values; retain error type and failing line.
            import traceback
            frames = traceback.extract_tb(error.__traceback__)
            tests_dir = Path(__file__).resolve().parent
            local = [frame for frame in frames if Path(frame.filename).resolve().parent == tests_dir]
            first = local[-1] if local else frames[-1]
            item = {"scenario": label, "type": type(error).__name__, "file": Path(first.filename).name,
                    "line": first.lineno,
                    "check": first.line}
            self.index["failedAttempts"].append(item)
            self.index["latestChecks"][label] = "FAIL"
            self.save()
            print(f"FAIL {json.dumps(item, ensure_ascii=False)}", flush=True)

    def open(self, role, size, theme, *, h5=False, signed=True, initial=""):
        ctx = self.browser.new_context(viewport=dict(zip(("width","height"), size)), device_scale_factor=1,
                                       locale="zh-CN", timezone_id="Asia/Shanghai", color_scheme=theme,
                                       is_mobile=size[0] < 600, has_touch=size[0] < 600)
        page = ctx.new_page()
        page.set_default_timeout(6000)
        probe = Probe(page)
        try:
            if h5 and signed:
                self.auth_retry(lambda: h5_login(page, self.base, role), probe)
            else:
                page.goto(self.base + ("/h5.html" if h5 else "/oa.html") + initial)
                if signed:
                    traffic = Traffic(ctx, self.base)
                    self.sessions[role] = self.auth_retry(lambda: login(page, role, traffic), probe)
                    expect(heading(page, ROUTES["/employees"][0]).or_(
                        page.locator(".sidebar-container")).first).to_be_attached()
                    if theme == "dark":
                        set_theme(page)
        except Exception:
            ctx.close()
            raise
        return page, probe

    def auth_retry(self, action, probe):
        for attempt in range(5):
            try:
                return action()
            except AssertionError:
                retry_after = probe.retry_after
                if retry_after is None or not 0 < retry_after <= 30 or attempt == 4:
                    raise
                print(f"QA capture received real login HTTP 429; waiting Retry-After={retry_after}s", flush=True)
                time.sleep(retry_after + .1)

    def token(self, role):
        return self.sessions[role]["token"]

    def shot(self, page, probe, label, role, size, theme, *, state="live synthetic data",
             anchors=None, start=0, sample=None):
        page.evaluate("""async () => {
          await document.fonts.ready;
          const frame = () => new Promise(requestAnimationFrame);
          await frame(); await frame();
          const active = document.getAnimations().filter(a =>
            a.playState === 'running' && Number.isFinite(a.effect?.getComputedTiming().endTime));
          if (active.length) await Promise.race([
            Promise.allSettled(active.map(a => a.finished)), new Promise(done => setTimeout(done, 500))
          ]);
          await frame(); await frame();
        }""")
        measurements = {name: position(locator) for name, locator in (anchors or {}).items()}
        viewport = page.evaluate("""() => ({
          width:innerWidth,height:innerHeight,docWidth:document.documentElement.scrollWidth,
          bodyWidth:document.body.scrollWidth,scrollY,visualHeight:visualViewport.height,
          colorScheme:getComputedStyle(document.documentElement).colorScheme,
          darkClass:document.documentElement.classList.contains('dark'),
          metaViewport:document.querySelector('meta[name=viewport]')?.content,
          scrollContainers:[...document.querySelectorAll('main,.app-main,.el-drawer__body')]
            .filter(e=>e.scrollHeight>e.clientHeight).map(e=>({top:e.scrollTop,height:e.clientHeight,content:e.scrollHeight}))
        })""")
        key = f"{label}-{size[0]}x{size[1]}-{theme}"
        file = self.output / (key + ".png")
        page.screenshot(path=file, animations="disabled", full_page=False)
        url = urlsplit(page.url)
        entry = {"file": file.name, "sha256": hashlib.sha256(file.read_bytes()).hexdigest(),
                 "role": role if role != "anonymous" else "anonymous", "state": state,
                 "requestedViewport": list(size), "requestedTheme": theme,
                 "route": url.path + (("#" + url.fragment.split("?")[0]) if url.fragment else ""),
                 "viewport": viewport, "anchors": measurements, "traffic": probe.events[start:],
                 "console": dict(Counter(probe.errors))}
        directives = dict(re.findall(r"([a-z-]+)\s*=\s*([a-z0-9.]+)",
                                     (viewport["metaViewport"] or "").lower()))
        maximum = directives.get("maximum-scale", "")
        entry["checks"] = {
            "documentFits": viewport["docWidth"] <= viewport["width"] + 1,
            "bodyFits": viewport["bodyWidth"] <= viewport["width"] + 1,
            "zoomEnabled": directives.get("user-scalable") not in {"no", "0"} and
                           (not re.fullmatch(r"[0-9]+(?:\.[0-9]+)?", maximum) or float(maximum) > 1),
        }
        if sample:
            entry["contrast"] = sample
        self.index["shots"][key] = entry
        self.save()
        print(key, {n: val and val["y"] for n,val in measurements.items()}, flush=True)
        return entry


def goto(page, route):
    page.evaluate("(path) => {location.hash = path}", route)
    expect(heading(page, ROUTES[route][0])).to_be_visible()


def selected_type(page, name):
    page.get_by_role("list", name="申请类型").get_by_role("button", name=re.compile(name)).click()


def role_label_contrast(page):
    header = page.locator("header.navbar")
    selector = ".oa-navbar-role .el-tag__content"
    if header.locator(selector).count() and header.locator(selector).first.is_visible():
        return contrast(header, {"navbarRoleText": selector})
    return {}


def roster(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/employees")
        region = directory(page)
        view = button(region, "查看档案").first
        expect(view).to_be_visible()
        first = region.get_by_role("row").filter(has=view).first
        if not first.count():
            first = region.locator(".oa-person-name").first
        anchors = {"firstEmployee": first, "tableHeader": region.get_by_role("columnheader").first, "rowAction": view}
        sample = {}
        for key, selector in (("employeeId", ".oa-employee-number"), ("rowAction", ".oa-employee-open, .oa-table-action button, .oa-directory-pure-table tbody button")):
            if region.locator(selector).count():
                sample.update(contrast(region, {key: selector}))
        if getattr(cap.args, "section", None) == "header-delta":
            sample.update(role_label_contrast(page))
        hint = placeholder_ratio(region.get_by_label("搜索员工", exact=True))
        if hint is not None:
            sample["searchPlaceholder"] = hint
        if size[0] < 600:
            anchors["settings"] = button(page, "界面设置")
            anchors["logout"] = button(page, "退出登录")
        capture = cap.shot(page, probe, "employees-list", ADMIN, size, theme, anchors=anchors, sample=sample)
        if size[0] < 600 and region.locator(".oa-mobile-employee").count():
            rows = region.locator(".oa-mobile-employee").evaluate_all("""rows => rows.map(e => {
              const r=e.getBoundingClientRect(); return {top:r.top,bottom:r.bottom,height:r.height}
            })""")
            cap.index["probes"][f"compact-roster-{size[0]}-{theme}"] = {
                "fullRows": sum(0 <= row["top"] and row["bottom"] <= size[1] for row in rows),
                "firstRowHeight": round(rows[0]["height"]), "firstActionHeight": position(view)["h"]}
            cap.save()
        if cap.args.phase == "after" and size[0] < 600:
            for name in ("settings", "logout"):
                rect = capture["anchors"][name]
                assert rect and rect["centerHit"] and rect["x"] >= 0 and rect["x"] + rect["w"] <= size[0] + 1, name
        view.click()
        dialog = page.get_by_role("dialog")
        expect(dialog).to_be_visible()
        nav = dialog.get_by_role("navigation", name="员工详情分组")
        cap.shot(page, probe, "employees-detail", ADMIN, size, theme,
                 anchors={"businessTabs": nav, "employeeHeading": dialog.locator("#employee-heading")})
        button(dialog, "编辑员工").click()
        editor = page.get_by_label("姓名", exact=True).or_(page.locator("#employee-name")).first
        expect(editor).to_be_visible()
        cap.shot(page, probe, "employees-edit", ADMIN, size, theme,
                 anchors={"businessTabs": nav, "firstInput": editor})
    finally:
        page.context.close()


def pc_create(cap, size, theme):
    page, probe = cap.open(STAFF, size, theme)
    try:
        goto(page, "/approvals")
        if cap.args.phase == "after":
            page.wait_for_load_state("networkidle")
            tags = {tone: f".pc-list .el-tag--{tone}" for tone in ("success", "warning", "info", "danger")
                    if page.locator(f".pc-list .el-tag--{tone}").count()}
            sample = contrast(page.locator("body"), tags) if tags else {}
            if getattr(cap.args, "section", None) == "header-delta":
                sample.update(role_label_contrast(page))
            cap.shot(page, probe, "pc-application-status", STAFF, size, theme, sample=sample)
        button(page, "发起申请").click()
        selected_type(page, "通用申请")
        field = page.get_by_label("标题", exact=True)
        expect(field).to_be_editable()
        cap.shot(page, probe, "pc-approval-create", STAFF, size, theme,
                 anchors={"firstInput": field, "submit": button(page, "提交申请")},
                 sample=role_label_contrast(page) if getattr(cap.args, "section", None) == "header-delta" else None)
    finally:
        page.context.close()


def h5_long(cap, size, theme):
    page, probe = cap.open(STAFF, size, theme, h5=True)
    try:
        tab(page, "发起申请").click()
        selected_type(page, "请假申请")
        title = page.get_by_label("标题", exact=True)
        end = page.get_by_label("请假事由", exact=True)
        submit = button(page, "提交申请")
        anchors = {"firstInput": title, "lastInput": end, "submit": submit,
                   "globalNav": page.locator(".h5-nav"), "taskBar": page.locator(".h5-editor-actions, .h5-task-dock, .h5-task-actions").first}
        expect(title).to_be_editable()
        expect(end).to_be_editable()
        cap.shot(page, probe, "h5-long-top", STAFF, size, theme, anchors=anchors)
        title.fill("UI 验收合成请假申请")
        end.fill(LONG)
        end.evaluate("e => e.scrollIntoView({block:'center'})")
        end.focus()
        cap.shot(page, probe, "h5-long-last-field", STAFF, size, theme, anchors=anchors)
        submit.evaluate("e => e.scrollIntoView({block:'center'})")
        cap.shot(page, probe, "h5-long-submit", STAFF, size, theme,
                 state="incomplete synthetic draft; leave preview not submitted", anchors=anchors)
    finally:
        page.context.close()


def representative(cap):
    for size, theme in ((DESKTOP, "light"), (PHONE, "light"), (LAPTOP, "dark"), (PHONE, "dark")):
        cap.run(f"roster-{size}-{theme}", roster, cap, size, theme)
    for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark"), (PHONE, "light")):
        cap.run(f"pc-create-{size}-{theme}", pc_create, cap, size, theme)
    for size, theme in ((PHONE, "light"), (PHONE, "dark"), ((360, 800), "light"), ((320, 740), "dark")):
        cap.run(f"h5-long-{size}-{theme}", h5_long, cap, size, theme)


def alert_contrast(scope, tone):
    prefix = f".el-alert--{tone} "
    selectors = {f"{tone}-{part}": f"{prefix}.el-alert__{part}" for part in ("title", "description")
                 if scope.locator(f"{prefix}.el-alert__{part}").count()}
    return contrast(scope, selectors) if selectors else None


def monitor_ready(page, path, probe):
    region = page.get_by_role("region", name=ROUTES[path][0], exact=True)
    expect(region).to_be_visible()
    expect(region.get_by_role("status", name="正在加载")).to_have_count(0)
    live = region.locator("tbody tr") if path == "/contract-reminders" else region.locator(".oa-notification-list li")
    if path == "/contract-reminders":
        live = live.or_(button(region, "查看合同"))
    else:
        live = live.or_(button(region, "查看合同详情")).or_(button(region, "查看申请详情"))
    empty = region.get_by_text(re.compile("所选范围内暂无到期合同|暂无站内通知"))
    error = button(region, "重新加载")
    expect(live.or_(empty).or_(error).first).to_be_visible()
    return region, "rows" if live.count() else "empty" if empty.count() else "error"


def probe_routes(cap, role, theme):
    page, probe = cap.open(role, DESKTOP, theme)
    try:
        for cycle in range(3):
            for path in ("/contract-reminders", "/notifications"):
                start, errors = len(probe.events), len(probe.errors)
                page.locator(".sidebar-container").get_by_text(ROUTES[path][0], exact=True).click()
                expect(heading(page, ROUTES[path][0])).to_be_visible()
                region, state = monitor_ready(page, path, probe)
                key = f"{role}-{theme}-sidebar-{cycle}-{path[1:]}"
                cap.index["probes"][key] = {"state": state, "hashMatches": urlsplit(page.url).fragment == path,
                                            "api": probe.events[start:], "console": probe.errors[errors:]}
                if cycle == 0:
                    cap.shot(page, probe, f"{path[1:]}-{role}", role, DESKTOP, theme, state="live: " + state,
                             anchors={"heading": region.get_by_role("heading").first}, start=start)
                assert state != "error"
        # A click on a real shell tag closes it and updates the route.
        current = page.locator(".tags-view").get_by_text("站内通知", exact=True)
        close = current.locator("xpath=ancestor::*[.//*[contains(@class,'close')]][1]").locator(
            ".el-icon-close, .chrome-close-btn").last
        if close.count() and close.is_visible():
            start = len(probe.events)
            close.click()
            expect(heading(page, "站内通知")).to_have_count(0)
            goto(page, "/notifications")
            _, state = monitor_ready(page, "/notifications", probe)
            cap.index["probes"][f"{role}-{theme}-closed-reopened"] = {"state": state, "api": probe.events[start:]}
        else:
            cap.index["probes"][f"{role}-{theme}-closed-reopened"] = {"state": "NOT RUN: shell close control absent"}
        cap.save()
    finally:
        page.context.close()


def network_route(cap, path, state, theme="light"):
    page, probe = cap.open(ADMIN, DESKTOP, theme)
    match = re.compile(r"/api/oa/v1" + path + r"\?")
    try:
        held = []
        start = len(probe.events)
        def intercept(route):
            if state == "loading":
                held.append(route)
            elif state == "500":
                route.fulfill(status=500, json={"code": "QA_INJECTED", "msg": "验收注入：服务暂不可用"})
            else:
                route.fulfill(status=403, json={"code": "FORBIDDEN", "msg": "验收注入：没有访问权限"})
        page.route(match, intercept, times=1)
        goto(page, path)
        region = page.get_by_role("region", name=ROUTES[path][0], exact=True)
        if state == "loading":
            expect(region.get_by_role("status", name="正在加载")).to_be_visible()
        else:
            expect(button(region, "重新加载")).to_be_visible()
        cap.shot(page, probe, f"{path[1:]}-injected-{state}", ADMIN, DESKTOP, theme,
                 state="controlled network interception: " + state, start=start,
                 sample=alert_contrast(region, "error") if state != "loading" else None)
        if state == "loading":
            assert len(held) == 1
            held[0].continue_()
        else:
            button(region, "重新加载").click()
        _, final = monitor_ready(page, path, probe)
        assert final in ("rows", "empty")
        mode = "" if theme == "light" else "-" + theme
        cap.index["probes"][f"{path[1:]}-{state}{mode}-recovered"] = {"state": final, "api": probe.events[start:]}
        cap.save()
    finally:
        page.context.close()


def direct_route(cap, path):
    # Start at the deep URL before authentication; also explicitly assign the
    # target hash after auth to cover signed-in router entry if the login guard
    # canonicalizes the first navigation.
    page, probe = cap.open(STAFF, DESKTOP, "light", initial="#" + path)
    try:
        first = urlsplit(page.url).fragment
        goto(page, path)
        _, state = monitor_ready(page, path, probe)
        cap.index["probes"][f"direct-{path[1:]}"] = {
            "state": state, "firstHashAfterLogin": first.split("?")[0],
            "finalHash": urlsplit(page.url).fragment, "document": probe.document,
            "api": probe.events, "console": probe.errors}
        cap.save()
    finally:
        page.context.close()


def audit(cap):
    for role, theme in ((ADMIN, "light"), (ADMIN, "dark"), (STAFF, "light")):
        cap.run(f"switch-{role}-{theme}", probe_routes, cap, role, theme)
    for path in ("/contract-reminders", "/notifications"):
        cap.run(f"direct-{path}", direct_route, cap, path)
        for state in ("loading", "500", "403"):
            cap.run(f"{path}-{state}", network_route, cap, path, state)
    if cap.args.phase == "after":
        cap.run("contract-reminders-500-dark", network_route, cap, "/contract-reminders", "500", "dark")


def list_and_login(cap, *, pc_only=False):
    for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark"), (PHONE, "light")):
        for h5 in ([False, True] if size == PHONE and not pc_only else [False]):
            page, probe = cap.open("anonymous", size, theme, h5=h5, signed=False)
            try:
                field = page.get_by_label("账号", exact=True)
                expect(field).to_be_visible()
                cap.shot(page, probe, "h5-login" if h5 else "pc-login", "anonymous", size, theme,
                         state="empty login form; no credentials", anchors={"firstInput": field})
            finally:
                page.context.close()
    for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark"), (PHONE, "light")):
        for role in (ADMIN, STAFF, MANAGER):
            page, probe = cap.open(role, size, theme)
            try:
                for path, (title, owner) in ROUTES.items():
                    if owner == role:
                        cap.run(f"{path}-{size}-{theme}", route_shot, cap, page, probe, path, role, size, theme)
                if role == STAFF:
                    cap.run(f"dashboard-staff-{size}", route_shot, cap, page, probe, "/dashboard", role, size, theme)
            finally:
                page.context.close()


def route_shot(cap, page, probe, path, role, size, theme):
    start = len(probe.events)
    goto(page, path)
    page.wait_for_load_state("networkidle")
    region_name = re.compile(r"^部门管理(?:\s*\d+\s*个部门)?$") if path == "/departments" else ROUTES[path][0]
    region = page.get_by_role("region", name=region_name, exact=True).first
    anchor = region.get_by_role("heading").nth(1)
    sample = None
    if path == "/integrations":
        expect(region.locator(".oa-integration-status")).to_have_count(3)
        selectors = {f"state-{k}": f".oa-integration-grid > section:nth-child({k}) .oa-integration-status" for k in (1, 2, 3)}
        selectors["detail"] = ".oa-integration-grid .oa-muted"
        sample = contrast(region, selectors)
    elif path in ("/contract-reminders", "/notifications"):
        selectors = {f"tag-{tone}": f".el-tag--{tone}" for tone in ("warning", "danger", "info")
                     if region.locator(f".el-tag--{tone}").count()}
        if selectors:
            sample = contrast(region, selectors)
    if cap.args.section == "header-delta":
        sample = (sample or {}) | role_label_contrast(page)
    cap.shot(page, probe, path[1:] + "-route-" + role, role, size, theme,
             anchors={"heading": heading(page, ROUTES[path][0]), "businessContent": anchor}, start=start, sample=sample)


def employee_states(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/employees")
        panel = directory(page)
        view = button(panel, "查看档案").first
        expect(view).to_be_visible()
        if size == DESKTOP:
            nav = panel.get_by_role("navigation", name=re.compile("分页")).last
            next_page = button(nav, "下一页")
            if next_page.count() and next_page.is_enabled():
                with page.expect_response(lambda r: r.url.endswith("/employees/search") and r.ok):
                    next_page.click()
                expect(view).to_be_visible()
                cap.shot(page, probe, "employees-page-2", ADMIN, size, theme)
        field = panel.get_by_label("搜索员工", exact=True)
        field.fill("UIUX-不可能匹配-20261108")
        button(panel, "搜索").click()
        expect(panel.get_by_text(re.compile("没有符合当前筛选条件的员工"))).to_be_visible()
        cap.shot(page, probe, "employees-search-empty", ADMIN, size, theme, anchors={"search": field})
        staff = cap.token(ADMIN)
        fixture = call(cap.base, "/employees/1", staff)
        field.fill(fixture["employeeNo"])
        button(panel, "搜索").click()
        expect(view).to_be_visible()
        expect(button(panel, "查看档案")).to_have_count(1)
        view.focus()
        page.keyboard.press("Enter")
        dialog = page.get_by_role("dialog")
        expect(dialog).to_be_visible()
        for title, key in (("劳动合同", "contracts"), ("员工账号", "account"), ("变更历史", "audit")):
            nav = dialog.get_by_role("navigation", name="员工详情分组")
            button(nav, title).click()
            page.wait_for_load_state("networkidle")
            sample = None
            anchors = {"firstContract": dialog.get_by_role("article").first, "businessTabs": nav}
            if key == "contracts" and cap.args.phase == "after":
                selectors = {"attachmentPreview": '.oa-file-list button[aria-label^="预览 "]:not(:disabled)',
                             "attachmentDownload": '.oa-file-list button[aria-label^="下载 "]:not(:disabled)'}
                for action, selector in selectors.items():
                    anchors[action] = dialog.locator(selector).first
                    expect(anchors[action]).to_be_visible()
                sample = contrast(dialog, selectors)
            cap.shot(page, probe, f"employees-{key}", ADMIN, size, theme,
                     anchors=anchors, sample=sample)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        cap.index["probes"][f"employee-focus-return-{size[0]}-{theme}"] = view.evaluate("e => document.activeElement === e")
        cap.save()
    finally:
        page.context.close()


def approval_detail(cap, size, theme):
    page, probe = cap.open(MANAGER, size, theme)
    try:
        goto(page, "/approvals")
        history = button(page.get_by_role("navigation", name="审批视图"), "已办")
        history.click()
        record = page.locator(".pc-record-button").first
        expect(record).to_be_visible()
        variants = {f"tag-{tone}": f".pc-list .el-tag--{tone}" for tone in ("success", "warning", "info", "danger")
                    if page.locator(f".pc-list .el-tag--{tone}").count()}
        cap.shot(page, probe, "approval-list-many", MANAGER, size, theme, anchors={"firstRecord": record},
                 sample=contrast(page.locator("body"), variants) if variants else None)
        title = record.locator(".pc-record-title").inner_text()
        record.click()
        expect(heading(page, title)).to_be_visible()
        cap.shot(page, probe, "approval-detail", MANAGER, size, theme,
                 anchors={"heading": heading(page, title), "back": button(page, "返回列表")})
    finally:
        page.context.close()


def workflow(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/workflow-settings")
        button(page, "工作日历").click()
        expect(heading(page, "工作日历")).to_be_visible()
        page.wait_for_load_state("networkidle")
        cap.shot(page, probe, "work-calendar", ADMIN, size, theme)
        button(page, "审批规则").click()
        button(page, "新增审批规则").click()
        expect(heading(page, "第 1 步")).to_be_visible()
        page.wait_for_load_state("networkidle")
        rules = {"stepsHeading": heading(page, "审批顺序"), "firstStep": heading(page, "第 1 步"),
                 "candidatePages": page.get_by_role("navigation", name="审批人候选分页"),
                 "rulePages": page.get_by_role("navigation", name="审批规则分页")}
        heading(page, "审批顺序").evaluate("e => e.scrollIntoView({block:'center'})")
        cap.shot(page, probe, "workflow-rule-one-step", ADMIN, size, theme, anchors=rules)
        for _ in range(9):
            button(page, "添加审批步骤").click()
        expect(button(page, "添加审批步骤")).to_be_disabled()
        last = heading(page, "第 10 步")
        last.evaluate("e => e.scrollIntoView({block:'center'})")
        cap.shot(page, probe, "workflow-rule-ten-steps", ADMIN, size, theme,
                 anchors=rules | {"lastStep": last}, state="unsaved rule with ten draft steps")
    finally:
        page.context.close()


def excel(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/employee-import")
        uploader = page.locator("#oa-import-file")
        expect(button(page, "预览导入")).to_be_visible()
        cap.shot(page, probe, "excel-select", ADMIN, size, theme, anchors={"fileInput": uploader})
        admin = cap.token(ADMIN)
        template = binary_workbook(cap.base, "/employees/import-template.xlsx", admin)
        department = call(cap.base, "/employees/1", admin)["department"]
        data = [
            ("UIUX-V", "界面验收专用合成员工", department, "1990-01-01", "2022-01-01", "1500.50"),
            ("UIUX-D", "界面验收专用错误日期", department, "1990-02-30", "2022-01-01", "1500.50"),
        ]
        uploader.set_input_files({
            "name": "用于验收长中文文件名和窄屏上传按钮换行效果的模拟劳动人事档案工作簿.xlsx",
            "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            "buffer": fill_workbook(template, data),
        })
        result = wait_write(page, "/employee-imports/preview", button(page, "预览导入"))
        assert result["validRows"] == 1 and result["errorRows"] == 1
        expect(heading(page, "预览结果")).to_be_visible()
        all_rows = button(page, "全部行")
        if all_rows.get_attribute("aria-pressed") == "false":
            all_rows.click()
        cap.shot(page, probe, "excel-preview-mixed", ADMIN, size, theme,
                 state="real mixed preview; same workbook bytes and filename; never confirmed")
        errors = page.get_by_role("button", name=re.compile("^仅错误行"))
        errors.focus()
        page.keyboard.press("Space")
        expect(errors).to_have_attribute("aria-pressed", "true")
        preview_region = page.get_by_role("region", name="预览结果", exact=True)
        alert_sample = alert_contrast(preview_region, "warning")
        if cap.args.phase == "after":
            assert alert_sample and "warning-title" in alert_sample
        cap.shot(page, probe, "excel-preview-error-only", ADMIN, size, theme, sample=alert_sample)
        disclosure = page.locator("summary").filter(has_text=re.compile("^历史预览与导入结果$"))
        if disclosure.count():
            disclosure.click()
        history = page.get_by_role("region", name="我的导入记录", exact=True)
        record = button(history, "查看结果").first
        expect(record).to_be_visible()
        record.click()
        expect(page.get_by_text(re.compile(r"已导入 [0-9]+ 名员工|[0-9]+\s*名员工已导入")).first.or_(heading(page, "导入完成")).first).to_be_visible()
        success = page.locator(".oa-import-count")
        cap.shot(page, probe, "excel-history-result", ADMIN, size, theme,
                 state="existing committed synthetic import result; no new employee written",
                 sample=contrast(page.locator("body"), {"success-result": ".oa-import-count"}) if success.count() else None)
    finally:
        page.context.close()


def extended(cap):
    cap.run("route-matrix", list_and_login, cap)
    for size, theme in ((DESKTOP, "light"), (PHONE, "dark")):
        for fn in (employee_states, approval_detail, workflow, excel):
            cap.run(f"{fn.__name__}-{size}-{theme}", fn, cap, size, theme)


def reminder_double_activation(cap):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        page.wait_for_load_state("networkidle")
        attempts = []
        count = 3 if cap.args.phase == "after" else 1
        in_place = cap.args.phase == "after"
        rows = call(cap.base, "/contract-reminders?days=30&page=1&pageSize=20", cap.token(ADMIN))["list"]
        for cycle in range(count):
            goto(page, "/contract-reminders")
            region, state = monitor_ready(page, "/contract-reminders", probe)
            assert state == "rows", "requires the seeded due contract"
            view = button(region, "查看合同").first
            expect(view).to_be_enabled()
            row = view.locator("xpath=ancestor::tr[1]")
            contract_no = row.get_by_role("cell").nth(1).inner_text().strip()
            target = next(item for item in rows if item["contractNo"] == contract_no)
            start = len(probe.events)
            if in_place:
                path = f"/api/oa/v1/contracts/{target['contractId']}"
                with page.expect_response(lambda r: urlsplit(r.url).path == path and r.request.method == "GET") as response:
                    view.evaluate("e => { e.click(); e.click(); }")
                detail = response.value.json()["data"]
                assert response.value.ok and detail["id"] == target["contractId"]
                assert detail["employeeId"] == target["employeeId"] and detail["contractNo"] == contract_no
                expect(heading(page, "合同到期提醒")).to_be_visible()
                assert urlsplit(page.url).fragment == "/contract-reminders"
            else:
                view.evaluate("e => { e.click(); e.click(); }")
                expect(heading(page, "员工档案")).to_be_visible()
            page.wait_for_load_state("networkidle")
            dialog = page.get_by_role("dialog")
            matched = dialog.get_by_text(contract_no, exact=True)
            if cap.args.phase == "after":
                # A quiet network window can occur before the drawer mounts
                # its ledger. Observe the authorized target, not that window.
                try:
                    expect(matched.first).to_be_visible(timeout=8000)
                except AssertionError:
                    pass
            record = {"state": "target-open" if matched.count() and matched.first.is_visible() else "target-lost",
                      "route": urlsplit(page.url).fragment.split("?")[0],
                      "dialog": dialog.count() > 0, "api": probe.events[start:],
                      "stimulus": "two DOM click activations in one browser task; real backend"}
            attempts.append(record)
            key = "contract-reminder-double-activation"
            cap.index["probes"][key] = attempts[0] | {"repeats": attempts.copy()}
            cap.save()
            if cycle == 0:
                cap.shot(page, probe, key, ADMIN, DESKTOP, "light", state=record["state"])
            if cap.args.phase == "after":
                assert record["state"] == "target-open"
                assert [item for item in record["api"] if item[:2] == ["GET", "/api/oa/v1/contracts/:id"]] == [
                    ["GET", "/api/oa/v1/contracts/:id", 200]]
                assert all(not re.search(r"/(?:employees|departments)(?:/|$)", item[1]) for item in record["api"])
            if cycle + 1 < count:
                page.keyboard.press("Escape")
                expect(dialog).to_have_count(0)
                expect(view).to_be_focused()
    finally:
        page.context.close()


def accessibility_edges(cap, *, pc_only=False):
    for size, theme in (() if pc_only else ((PHONE, "light"), ((320, 740), "dark"))):
        page, probe = cap.open(STAFF, size, theme, h5=True)
        try:
            tab(page, "发起申请").click()
            selected_type(page, "请假申请")
            expect(page.locator("#h5-leave-type")).to_be_enabled()
            button(page, "预览请假天数").click()
            alert = page.get_by_role("alert").filter(has_text=re.compile("有效|请假类型")).first
            expect(alert).to_be_visible()
            active = page.evaluate("""() => {
              const e=document.activeElement;
              return {tag:e.tagName,id:e.id || null,errorIds:e.getAttribute('aria-describedby'),
                alertFocused:e.getAttribute('role')==='alert'};
            }""")
            cap.index["probes"][f"h5-leave-invalid-focus-{size[0]}"] = active
            cap.shot(page, probe, "h5-leave-invalid", STAFF, size, theme,
                     state="no leave type or dates; client-side validation",
                     anchors={"error": alert, "leaveType": page.locator("#h5-leave-type"),
                              "focused": page.locator(":focus").last})
            if cap.args.phase == "after":
                field = page.locator("#h5-leave-type")
                expect(field).to_be_focused()
                expect(field).to_have_attribute("aria-invalid", "true")
                assert field.get_attribute("aria-describedby") == alert.get_attribute("id")
                assert position(alert)["centerHit"], "inline error must remain in view when the invalid field receives focus"
        finally:
            page.context.close()
    page, probe = cap.open(STAFF, DESKTOP, "light")
    try:
        token = cap.token(STAFF)
        call(cap.base, "/departments", token, status=403)
        page.evaluate("location.hash = '/employee-import'")
        expect(page).to_have_url(re.compile("#/error/403"))
        expect(heading(page, "Excel 导入")).to_have_count(0)
        cap.shot(page, probe, "staff-import-forbidden", STAFF, DESKTOP, "light",
                 state="real employee role guard; departments API separately verified 403")
    finally:
        page.context.close()


def attachment_selection(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/employees")
        panel = directory(page)
        admin = cap.token(ADMIN)
        person = call(cap.base, "/employees/1", admin)
        field = panel.get_by_label("搜索员工", exact=True)
        field.fill(person["employeeNo"])
        with page.expect_response(lambda r: r.url.endswith("/employees/search") and r.ok):
            button(panel, "搜索").click()
        button(panel, "查看档案").first.click()
        dialog = page.get_by_role("dialog")
        expect(dialog).to_be_visible()
        button(dialog.get_by_role("navigation", name="员工详情分组"), "劳动合同").click()
        attach = button(dialog, "添加附件").first
        expect(attach).to_be_visible()
        attach.click()
        region = dialog.get_by_role("region", name="添加合同附件")
        picker = region.locator("#oa-attachment-input")
        path = Path(__file__).parent / "fixtures" / "m6-normal.pdf"
        filename = "模拟劳动合同附件超长中文名称用于验证文件选择后的折行和范围标签.pdf"
        picker.set_input_files({"name": filename,
                                "mimeType": "application/pdf", "buffer": path.read_bytes()})
        submit = button(region, "上传附件")
        expect(submit).to_be_enabled()
        if cap.args.phase == "after":
            expect(region.get_by_text(filename, exact=True)).to_be_visible()
        submit.evaluate("e => e.scrollIntoView({block:'center'})")
        cap.shot(page, probe, "attachment-select-long-filename", ADMIN, size, theme,
                 state="same synthetic PDF selected; never uploaded",
                 anchors={"fileInput": picker, "submit": submit, "fileName": region.get_by_role("status").first})
    finally:
        page.context.close()


def dashboard_contract_link(cap):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        goto(page, "/dashboard")
        queue = page.get_by_role("region", name="即将到期与逾期合同")
        row = queue.get_by_role("listitem").first
        view = button(row, "查看")
        expect(view).to_be_enabled()
        contract_no = row.locator("strong").inner_text().rsplit(" · ", 1)[-1]
        start = len(probe.events)
        view.click()
        expect(heading(page, "员工档案")).to_be_visible()
        dialog = page.get_by_role("dialog", name="员工档案详情")
        expect(dialog.get_by_text(contract_no, exact=True).first).to_be_visible()
        cap.shot(page, probe, "dashboard-contract-target", ADMIN, DESKTOP, "light",
                 state="new dashboard live contract row opens exact authorized record", start=start)
        button(dialog, "返回员工列表").click()
        expect(dialog).to_have_count(0)
        goto(page, "/dashboard")
        goto(page, "/employees")
        expect(button(directory(page), "查看档案").first).to_be_visible()
        expect(dialog).to_have_count(0)
        cap.index["probes"]["dashboard-contract-no-residue"] = {"state": "target-open; no stale target on roster reentry"}
        cap.save()
    finally:
        page.context.close()


def dashboard_mine_and_history(cap):
    page, probe = cap.open(MANAGER, DESKTOP, "light")
    try:
        goto(page, "/dashboard")
        shortcut = button(page, "查看我的申请")
        expect(shortcut).to_be_enabled()
        shortcut.evaluate("e => { e.click(); e.click(); }")
        goto_heading = heading(page, "审批中心")
        expect(goto_heading).to_be_visible()
        mine = button(page.get_by_role("navigation", name="审批视图"), "我的申请")
        expect(mine).to_have_attribute("aria-pressed", "true")
        cap.shot(page, probe, "dashboard-mine-double-activation", MANAGER, DESKTOP, "light",
                 state="same-task double activation with submit and approve permissions")
    finally:
        page.context.close()
    state = json.loads((Path(__file__).parent / "results/m5/browser-state.json").read_text())
    page, probe = cap.open(STAFF, DESKTOP, "light")
    try:
        token = cap.token(STAFF)
        request = request_detail(cap.base, state["requestId"], token)
        assert request["leaveCalculation"] == state["calculation"] and request["status"] == "CANCELLED"
        annual = annual_balance(cap.base, token, state["employeeId"], state["year"])
        assert all(annual[key] == val for key, val in state["annual"].items())
        goto(page, "/approvals")
        row = page.get_by_role("button", name=re.compile(re.escape(request["title"])))
        more = button(page, "下一页")
        for _ in range(10):
            page.wait_for_load_state("networkidle")
            if row.count():
                break
            if not more.count() or more.is_disabled():
                break
            more.click()
        expect(row).to_be_visible()
        row.click()
        expect(heading(page, request["title"])).to_be_visible()
        calculation = page.get_by_role("region", name="请假计算记录")
        expect(calculation).to_be_visible()
        cap.shot(page, probe, "historic-leave-calculation", STAFF, DESKTOP, "light",
                 state="original cancelled synthetic leave snapshot unchanged after UI upgrade")
        cap.index["probes"]["historic-leave-calculation"] = {"apiSnapshot": "unchanged", "annualCounters": "unchanged"}
        cap.save()
    finally:
        page.context.close()


def h5_navigation_fit(cap, size, theme):
    page, probe = cap.open(STAFF, size, theme, h5=True)
    try:
        for title in ("工作台", "站内通知"):
            if title == "工作台":
                button(page, title).click()
            else:
                tab(page, title).click()
            expect(heading(page, title)).to_be_visible()
            nav = page.locator(".h5-nav")
            expect(nav).to_be_visible()
            measurements = nav.evaluate('''e => {
              const r=e.getBoundingClientRect();
              return {left:r.left,right:r.right, viewport:innerWidth,
                controls:[...e.querySelectorAll('.van-tabbar-item')].map(item => {
                  const b=item.getBoundingClientRect(), target=document.elementFromPoint(b.x+b.width/2,b.y+b.height/2);
                  return {left:b.left,right:b.right,width:b.width,height:b.height,
                    centerHit:target===item||item.contains(target)};
                })};
            }''')
            cap.index["probes"][f"h5-navigation-{title}-{size[0]}"] = measurements
            cap.shot(page, probe, "h5-nav-" + ("dashboard" if title == "工作台" else "notifications"),
                     STAFF, size, theme, anchors={"globalNav": nav})
            assert measurements["left"] >= -1 and measurements["right"] <= measurements["viewport"] + 1
            assert measurements["controls"] and all(row["centerHit"] and
                row["left"] >= -1 and row["right"] <= measurements["viewport"] + 1
                for row in measurements["controls"])
        tab(page, "我的申请").click()
        expect(heading(page, "我的申请")).to_be_visible()
        page.wait_for_load_state("networkidle")
        variants = {tone: f".h5-list .van-tag--{tone}" for tone in ("success", "warning", "primary", "danger")
                    if page.locator(f".h5-list .van-tag--{tone}").count()}
        cap.shot(page, probe, "h5-application-status", STAFF, size, theme,
                 sample=contrast(page.locator("body"), variants) if variants else None)
        if size == PHONE:
            button(page, "我的假期").click()
            expect(heading(page, "我的假期")).to_be_visible()
            page.wait_for_load_state("networkidle")
            cap.shot(page, probe, "h5-personal-leave", STAFF, size, theme)
    finally:
        page.context.close()


def notification_identity_and_link(cap):
    expected = None
    for h5, size, theme in ((False, DESKTOP, "light"), (True, PHONE, "dark")):
        page, probe = cap.open(STAFF, size, theme, h5=h5)
        try:
            if h5:
                tab(page, "站内通知").click()
            else:
                token = cap.token(STAFF)
                result = call(cap.base, "/notifications?page=1&pageSize=20", token)
                expected = next(row for row in result["list"] if row.get("requestId", 0) > 0)
                detail = request_detail(cap.base, expected["requestId"], token)
                goto(page, "/notifications")
            item = page.locator(".h5-list article" if h5 else ".oa-notification-list > li").filter(
                has=page.get_by_text(f"申请 #{expected['requestId']}", exact=True)
            ).first
            expect(item).to_be_visible()
            target = item.get_by_role("button").first
            expect(target).to_be_enabled()
            cap.shot(page, probe, "notification-business-object-" + ("h5" if h5 else "pc"),
                     STAFF, size, theme, state="real applicant notification with source object identity",
                     anchors={"notice": item, "open": target})
            with page.expect_response(lambda r: r.url.endswith(f"/requests/{expected['requestId']}") and r.ok):
                target.click()
            expect(heading(page, detail["title"])).to_be_visible()
            cap.index["probes"]["notification-object-" + ("h5" if h5 else "pc")] = {
                "state": "visible object identity; opened matching authorized request"}
            cap.save()
        finally:
            page.context.close()


def pc_date_popup(cap, size, theme):
    page, probe = cap.open(STAFF, size, theme)
    try:
        goto(page, "/approvals")
        button(page, "发起申请").click()
        selected_type(page, "请假申请")
        calculate = button(page, "预览请假天数")
        kind = page.locator("#pc-leave-type")
        expect(kind).to_be_enabled()
        calculate.click()
        expect(kind).to_be_focused()
        expect(kind).to_have_attribute("aria-invalid", "true")
        expect(kind).to_have_attribute("aria-describedby", "pc-leave-error")
        first = kind.locator("option:not([disabled])").first.get_attribute("value")
        kind.select_option(first)
        calculate.click()
        start = page.locator("#pc-field-startDate")
        expect(start).to_be_focused()
        expect(start).to_have_attribute("aria-invalid", "true")
        expect(start).to_have_attribute("aria-describedby", "pc-leave-error")
        calendar = page.locator(".el-date-picker:visible").first
        if not calendar.count():
            start.click()
        expect(calendar).to_be_visible()
        labels = calendar.locator(".el-date-picker__header-label").all_text_contents()
        sample = contrast(calendar, {"dateHeader": ".el-date-picker__header-label"})
        hint = placeholder_ratio(start)
        if hint is not None:
            sample["datePlaceholder"] = hint
        cap.shot(page, probe, "pc-date-popup", STAFF, size, theme,
                 state="leave date popup with field error; no write",
                 anchors={"field": start, "datePopup": calendar}, sample=sample)
        assert any(re.search(r"\d+\s*年", label) for label in labels), labels
        start.press("Escape")
        expect(calendar).to_have_count(0)
        expect(start).to_be_focused()
        cap.index["probes"][f"pc-date-popup-{theme}"] = {"ChineseHeader": True, "escapeClosed": True,
                                                         "focusReturned": True, "errorLinked": True}
        cap.save()
    finally:
        page.context.close()


def integration_enabled(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        path = "**/api/oa/v1/integration/status"
        fixture = {"wecomLogin": "configured", "notifications": "wecom", "attachmentStorage": "oss"}
        page.route(path, lambda r: r.fulfill(status=200, json={"code": 0, "data": fixture, "msg": ""}), times=1)
        goto(page, "/integrations")
        statuses = page.locator(".oa-integration-status")
        expect(statuses).to_have_text(["已配置 · 待企业验收", "企业微信 API · 待企业验收", "OSS · 待企业验收"])
        region = page.get_by_role("region", name="集成状态", exact=True)
        selectors = {f"configured-{k}": f".oa-integration-grid > section:nth-child({k}) .oa-integration-status"
                     for k in (1, 2, 3)}
        cap.shot(page, probe, "integration-configured-fixture", ADMIN, size, theme,
                 state="injected configured-state UI only; no real WeCom/OSS traffic",
                 sample=contrast(region, selectors))
    finally:
        page.context.close()


def h5_leave_network_error(cap):
    size = (320, 740)
    page, probe = cap.open(STAFF, size, "dark", h5=True)
    try:
        tab(page, "发起申请").click()
        selected_type(page, "请假申请")
        page.get_by_label("标题", exact=True).fill("合成请假试算网络故障验收")
        kind = page.locator("#h5-leave-type")
        expect(kind).to_be_enabled()
        option = kind.locator("option:not([disabled])").filter(has_text="不扣余额").first
        if not option.count():
            option = kind.locator("option:not([disabled])").first
        leave_type = option.get_attribute("value")
        kind.select_option(leave_type)
        day = datetime.now(ZoneInfo("Asia/Shanghai")).date() + timedelta(days=16)
        while day.isoweekday() > 5:
            day += timedelta(days=1)
        start = page.locator("#h5-field-startDate")
        end = page.locator("#h5-field-endDate")
        for field in (start, end):
            field.fill(day.isoformat())
        reason = page.get_by_label("请假事由", exact=True)
        reason.fill(LONG)
        path = "**/api/oa/v1/leave-preview"
        page.route(path, lambda r: r.fulfill(status=500, json={
            "code": 7, "data": {"errorCode": "QA_INJECTED"}, "msg": "验收注入：服务暂不可用"
        }), times=1)
        calculate = button(page, "预览请假天数")
        with page.expect_response(lambda r: r.url.endswith("/leave-preview") and r.status == 500):
            calculate.click()
        alert = page.locator(".h5-leave-fields").get_by_role("alert")
        expect(alert).to_be_visible()
        expect(calculate).to_be_enabled()
        capture = cap.shot(page, probe, "h5-leave-api-error", STAFF, size, "dark",
                           state="controlled preview API HTTP 500; inputs retained",
                           anchors={"error": alert, "retry": calculate, "taskBar": page.locator(".h5-editor-actions")})
        expect(kind).to_have_value(leave_type)
        expect(start).to_have_value(day.isoformat())
        expect(end).to_have_value(day.isoformat())
        expect(reason).to_have_value(LONG)
        with page.expect_response(lambda r: r.url.endswith("/leave-preview") and r.status == 200) as response:
            calculate.click()
        assert response.value.json()["data"]["calculation"]["totalHalfDays"] >= 0
        expect(page.get_by_role("region", name="请假计算预览")).to_be_visible()
        cap.index["probes"]["h5-preview-http-500-recovered"] = {
            "inputRetained": True, "liveRecovery": 200, "errorCenterHit": capture["anchors"]["error"]["centerHit"],
            "retryCenterHit": capture["anchors"]["retry"]["centerHit"]}
        cap.save()
        assert capture["anchors"]["error"]["centerHit"] and capture["anchors"]["retry"]["centerHit"], \
            "preview API error and its retry should be visible above the sticky actions"
    finally:
        page.context.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--phase", choices=("before","after"), required=True)
    parser.add_argument("--runtime", required=True, help="immutable source/runtime identifier; no secrets")
    parser.add_argument("--section", choices=("representative","audit","extended","race","edges","navigation","release","header-delta","all"), default="all")
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--artifacts", type=Path)
    args = parser.parse_args()
    args.artifacts = args.artifacts or Path("tests/results/ui-ux-20261108") / args.phase
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("export local OA_SEED_PASSWORD; never put it on the command line")
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            cap = Capture(browser, args.base, args)
            if args.section in ("all", "release", "representative"):
                representative(cap)
                if args.phase == "after":
                    for size, theme in (((320, 740), "light"), ((360, 800), "dark")):
                        cap.run(f"narrow-roster-{size}-{theme}", roster, cap, size, theme)
            if args.section in ("all", "release", "audit"):
                audit(cap)
            if args.section in ("all", "release", "extended"):
                extended(cap)
            if args.section in ("all", "release", "race"):
                cap.run("contract-reminder-double-activation", reminder_double_activation, cap)
                if args.phase == "after":
                    cap.run("dashboard-contract-link", dashboard_contract_link, cap)
                    cap.run("dashboard-mine-and-history", dashboard_mine_and_history, cap)
                    cap.run("notification-object-link", notification_identity_and_link, cap)
            if args.section in ("all", "release", "edges"):
                cap.run("accessibility-edges", accessibility_edges, cap)
                for size, theme in ((DESKTOP, "light"), (PHONE, "dark")):
                    cap.run(f"attachment-selection-{size}-{theme}", attachment_selection, cap, size, theme)
            if args.phase == "after" and args.section in ("all", "release", "edges"):
                for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark")):
                    cap.run(f"pc-date-popup-{theme}", pc_date_popup, cap, size, theme)
                    if args.section != "release":
                        cap.run(f"integration-configured-{theme}", integration_enabled, cap, size, theme)
                if args.section != "release":
                    cap.run("h5-leave-network-error", h5_leave_network_error, cap)
            if args.phase == "after" and args.section in ("all", "navigation"):
                for size, theme in ((PHONE, "light"), ((360, 800), "light"), ((320, 740), "dark")):
                    cap.run(f"h5-navigation-{size}-{theme}", h5_navigation_fit, cap, size, theme)
            if args.phase == "after" and args.section == "header-delta":
                cap.run("pc-route-matrix", lambda: list_and_login(cap, pc_only=True))
                for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark"), (PHONE, "light"), (PHONE, "dark"),
                                    ((320, 740), "light"), ((360, 800), "dark")):
                    cap.run(f"header-{size}-{theme}", roster, cap, size, theme)
                for size, theme in ((DESKTOP, "light"), (LAPTOP, "dark"), (PHONE, "light"), (PHONE, "dark")):
                    cap.run(f"delta-create-{size}-{theme}", pc_create, cap, size, theme)
                for size, theme in ((DESKTOP, "light"), (PHONE, "dark")):
                    for scenario in (employee_states, approval_detail, workflow, excel, attachment_selection):
                        cap.run(f"delta-{scenario.__name__}-{size}-{theme}", scenario, cap, size, theme)
                audit(cap)
                cap.run("contract-reminder-double-activation", reminder_double_activation, cap)
                cap.run("pc-authorization-edge", lambda: accessibility_edges(cap, pc_only=True))
            return int("FAIL" in cap.index["latestChecks"].values())
        finally:
            browser.close()


if __name__ == "__main__":
    raise SystemExit(main())
