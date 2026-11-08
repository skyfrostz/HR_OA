"""HR OA department chart and in-place contract reminder browser acceptance."""
import argparse
from datetime import datetime, timedelta
import hashlib
import os
from pathlib import Path
import re
from urllib.parse import parse_qs, urlsplit
from zoneinfo import ZoneInfo

from playwright.sync_api import Error, expect, sync_playwright

from oa_api import fixture_pdf, local_base, upload
from oa_commercial_browser import contrast
from oa_m2_api import intent, login
from oa_m3_api import call, page_all, send
from oa_ui_ux_browser import ADMIN, VIEWER, Capture, DESKTOP, PHONE, button, goto, heading, monitor_ready

REMINDER_PATH = "/contract-reminders"
DETAIL_PATH = re.compile(r"/api/oa/v1/contracts/\d+$")
TREE_PATH = "**/api/oa/v1/departments/tree"
DEPARTMENT_REGION = re.compile(r"^部门管理(?:\s*\d+\s*个部门)?$")


def check_response(response, expected):
    assert response.ok
    actual = response.json()["data"]
    for key, value in (("id", expected["contractId"]), ("employeeId", expected["employeeId"]),
                       ("contractNo", expected["contractNo"])):
        assert actual[key] == value, key


def reminders(cap, page, role=ADMIN):
    goto(page, REMINDER_PATH)
    region, state = monitor_ready(page, REMINDER_PATH, None)
    assert state == "rows"
    rows = call(cap.base, REMINDER_PATH + "?days=30&page=1&pageSize=20", cap.token(role))["list"]
    assert len(rows) >= 2, "requires two real seeded due contracts"

    def target(row):
        row_container = region.locator("tbody tr, .oa-reminder-list li").filter(
            has=page.get_by_text(row["contractNo"], exact=True))
        return button(row_container, "查看合同")

    return rows, target


def assert_in_place(page, probe, start):
    assert urlsplit(page.url).fragment == REMINDER_PATH
    expect(page.locator("#oa-reminder-heading")).to_be_attached()
    assert all(not re.search(r"/(?:employees|departments)(?:/|$)", path)
               for _, path, _ in probe.events[start:])


def detail_heading(dialog, contract_no):
    return dialog.get_by_role("heading", name=contract_no, exact=True)


def queue_state(page):
    return page.evaluate("""() => ({
      route: location.hash,
      range: document.querySelector('.oa-filter select')?.value,
      active: document.querySelector('nav[aria-label="到期提醒分页"] [aria-current]')?.textContent,
      scroll: [window.scrollY, ...[...document.querySelectorAll('.app-main .el-scrollbar__wrap')]
        .map(node => node.scrollTop)]
    })""")


def open_close(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        rows, target = reminders(cap, page)
        a, b = rows[:2]
        first = target(a)
        expect(first).to_be_enabled()
        first.scroll_into_view_if_needed()
        before = queue_state(page)
        start = len(probe.events)
        dialog = page.get_by_role("dialog")
        with page.expect_response(lambda response: urlsplit(response.url).path ==
                                  f"/api/oa/v1/contracts/{a['contractId']}") as reply:
            first.evaluate("e => { e.click(); e.click(); }")
        check_response(reply.value, a)
        expect(dialog).to_have_count(1)
        expect(detail_heading(dialog, a["contractNo"])).to_be_visible()
        assert_in_place(page, probe, start)
        assert [item for item in probe.events[start:] if item[:2] == ["GET", "/api/oa/v1/contracts/:id"]] == [
            ["GET", "/api/oa/v1/contracts/:id", 200]]
        box = dialog.bounding_box()
        assert box and (abs(box["width"] - size[0]) <= 1 if size[0] < 600 else 870 <= box["width"] <= 890)
        close = button(dialog, "关闭")
        header = dialog.locator(".el-drawer__header")
        body = dialog.locator(".el-drawer__body")
        geometry = body.evaluate("""body => ({
          overflow: getComputedStyle(body).overflowY, scrollHeight: body.scrollHeight, height: body.clientHeight
        })""")
        assert geometry["overflow"] == "auto", geometry
        if geometry["scrollHeight"] > geometry["height"]:
            header_y = header.bounding_box()["y"]
            body.evaluate("e => { e.scrollTop = e.scrollHeight; }")
            assert abs(header.bounding_box()["y"] - header_y) <= 1
            body.evaluate("e => { e.scrollTop = 0; }")
        if size[0] < 600:
            assert close.bounding_box()["height"] >= 44
        tones = contrast(dialog, {"title": ".oa-contract-drawer-title", "employee": ".oa-contract-drawer-employee"})
        assert min(tones.values()) >= 4.5, tones
        cap.shot(page, probe, "reminder-open", ADMIN, size, theme, anchors={"drawer": dialog, "close": close}, sample=tones)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        expect(first).to_be_focused()
        assert queue_state(page) == before
        with page.expect_response(lambda response: urlsplit(response.url).path ==
                                  f"/api/oa/v1/contracts/{b['contractId']}") as reply:
            target(b).click()
        check_response(reply.value, b)
        expect(detail_heading(dialog, b["contractNo"])).to_be_visible()
        expect(detail_heading(dialog, a["contractNo"])).to_have_count(0)
        assert_in_place(page, probe, start)
    finally:
        page.context.close()


def stale_response(cap):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        rows, target = reminders(cap, page)
        a, b = rows[:2]
        dialog = page.get_by_role("dialog")
        held = []
        start = len(probe.events)

        def hold(route):
            response = route.fetch()
            check_response(response, a)
            held.append((route, response))

        endpoint = f"**/api/oa/v1/contracts/{a['contractId']}"
        page.route(endpoint, hold)
        target(a).click()
        expect(dialog).to_be_visible()
        expect(detail_heading(dialog, a["contractNo"])).to_have_count(0)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        expect(target(a)).to_be_focused()
        with page.expect_response(lambda response: urlsplit(response.url).path ==
                                  f"/api/oa/v1/contracts/{b['contractId']}") as reply:
            target(b).click()
        check_response(reply.value, b)
        expect(detail_heading(dialog, b["contractNo"])).to_be_visible()
        assert len(held) == 1
        route, response = held.pop()
        try:
            route.fulfill(response=response)
        except Error as error:
            if not re.search(r"closed|cancel|abort|invalid interception", str(error), re.I):
                raise
        finally:
            response.dispose()
        page.wait_for_load_state("networkidle")
        expect(detail_heading(dialog, b["contractNo"])).to_be_visible()
        expect(detail_heading(dialog, a["contractNo"])).to_have_count(0)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        expect(target(b)).to_be_focused()
        assert_in_place(page, probe, start)
        cap.index["probes"]["out-of-order"] = {"network": probe.events[start:], "lateAReplacedB": False}
        cap.save()
    finally:
        page.context.close()


def rapid_reopen_lock(cap):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        page.emulate_media(reduced_motion="reduce")
        rows, target = reminders(cap, page)
        start = len(probe.events)
        first, second = rows[:2]
        dialog = page.get_by_role("dialog")

        def lock_state():
            return page.evaluate("""() => ({
              locked: document.body.classList.contains('el-popup-parent--hidden'),
              ownerLock: document.body.classList.contains('oa-contract-scroll-locked'),
              width: document.body.style.width,
              overflow: getComputedStyle(document.body).overflow
            })""")

        initial = lock_state()
        target(first).click()
        expect(detail_heading(dialog, first["contractNo"])).to_be_visible()
        opened = lock_state()
        assert opened["locked"] or opened["overflow"] == "hidden", opened
        close = button(dialog, "关闭")
        expect(close).to_be_visible()
        # Capture the actual interval in the page clock and drive the next
        # visible trigger in the first frame after the prior dialog closes.
        interval = target(second).evaluate("""next => new Promise((resolve, reject) => {
          const close = document.querySelector('.oa-contract-drawer .el-drawer__header button');
          if (!close) return reject(new Error('drawer close control missing'));
          const started = performance.now();
          close.click();
          const reopen = () => {
            const drawer = document.querySelector('.oa-contract-drawer');
            if (!drawer || !drawer.getClientRects().length) {
              next.click();
              resolve(performance.now() - started);
            } else if (performance.now() - started < 300) requestAnimationFrame(reopen);
            else reject(new Error('drawer did not close before the requested reopen'));
          };
          requestAnimationFrame(reopen);
        })""")
        assert interval < 200, interval
        expect(detail_heading(dialog, second["contractNo"])).to_be_visible()
        state = queue_state(page)
        page.mouse.move(20, 450)
        page.mouse.wheel(0, 640)
        page.wait_for_timeout(240)  # Cross the specific 200ms unlock boundary.
        reopened = lock_state()
        cap.index["probes"]["rapid-reopen-lock"] = {
            "elapsedMs": interval, "initial": initial, "opened": opened,
            "afterFirstUnlockDeadline": reopened, "queueUnchanged": queue_state(page) == state,
        }
        cap.save()
        assert reopened["locked"] or reopened["overflow"] == "hidden", reopened
        assert queue_state(page) == state
        for key in ("Tab", "Shift+Tab"):
            page.keyboard.press(key)
            assert dialog.evaluate("node => node.contains(document.activeElement)")
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        expect(target(second)).to_be_focused()
        page.wait_for_timeout(240)
        final_lock = lock_state()
        cap.index["probes"]["rapid-reopen-lock"]["afterFinalClose240Ms"] = final_lock
        if final_lock != initial:
            page.wait_for_timeout(760)
        cap.index["probes"]["rapid-reopen-lock"]["afterFinalCloseSettled"] = lock_state()
        cap.save()
        assert final_lock == initial, final_lock
        assert_in_place(page, probe, start)
    finally:
        page.context.close()


def error_states(cap):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        rows, target = reminders(cap, page)
        a, b = rows[:2]
        dialog = page.get_by_role("dialog")
        target(a).click()
        expect(detail_heading(dialog, a["contractNo"])).to_be_visible()
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        start = len(probe.events)
        endpoint = f"**/api/oa/v1/contracts/{b['contractId']}"
        cases = [(403, "FORBIDDEN", "权限|禁止|无权"), (404, "NOT_FOUND", "不存在|找不到|无法找到"),
                 (500, "INTERNAL_ERROR", "失败|错误|重试"), (None, "", "失败|网络|重试")]
        messages = []
        for status, code, message in cases:
            def fail(route):
                if status is None:
                    route.abort("failed")
                else:
                    route.fulfill(status=status, json={"code": status, "data": {"errorCode": code}, "msg": code})
            page.route(endpoint, fail)
            target(b).click()
            expect(dialog).to_be_visible()
            alert = dialog.get_by_role("alert").filter(has_text=re.compile(message)).first
            expect(alert).to_be_visible()
            messages.append(alert.inner_text().strip())
            expect(detail_heading(dialog, a["contractNo"])).to_have_count(0)
            expect(detail_heading(dialog, b["contractNo"])).to_have_count(0)
            cap.shot(page, probe, f"reminder-error-{status or 'network'}", ADMIN, DESKTOP, "light",
                     state="controlled failure of the existing contract detail endpoint", anchors={"alert": alert})
            page.unroute(endpoint, fail)
            page.keyboard.press("Escape")
            expect(dialog).to_have_count(0)
        assert messages[0] != messages[1] and messages[0] != messages[3]
        def wrong_employee(route):
            response = route.fetch()
            try:
                payload = response.json()
                payload["data"]["employeeId"] += 100_000
                route.fulfill(response=response, json=payload)
            finally:
                response.dispose()

        page.route(endpoint, wrong_employee)
        target(b).click()
        expect(dialog.get_by_role("alert").filter(has_text=re.compile("员工|对象"))).to_be_visible()
        expect(detail_heading(dialog, b["contractNo"])).to_have_count(0)
        page.unroute(endpoint, wrong_employee)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        with page.expect_response(lambda r: r.ok and DETAIL_PATH.search(r.url)) as reply:
            target(b).click()
        check_response(reply.value, b)
        expect(detail_heading(dialog, b["contractNo"])).to_be_visible()
        assert_in_place(page, probe, start)
    finally:
        page.context.close()


def enter_date(page, label, value):
    field = page.get_by_label(label, exact=True)
    field.fill(value.isoformat())
    field.press("Tab")
    expect(field).to_have_value(value.isoformat())


def viewer_contract(cap):
    page, probe = cap.open(VIEWER, DESKTOP, "dark")
    try:
        rows, target = reminders(cap, page, VIEWER)
        first = rows[0]
        start = len(probe.events)
        with page.expect_response(lambda r: urlsplit(r.url).path ==
                                  f"/api/oa/v1/contracts/{first['contractId']}") as reply:
            target(first).click()
        check_response(reply.value, first)
        dialog = page.get_by_role("dialog")
        expect(detail_heading(dialog, first["contractNo"])).to_be_visible()
        for name in ("编辑草稿", "登记签署", "续约", "终止合同", "添加附件"):
            expect(button(dialog, name)).to_have_count(0)
        expect(dialog.get_by_role("button", name=re.compile("调整访问范围"))).to_have_count(0)
        assert_in_place(page, probe, start)
    finally:
        page.context.close()


def contract_writes(cap, reminder):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        token = cap.token(ADMIN)
        _, target = reminders(cap, page)
        path = f"/contracts/{reminder['id']}"
        ledger = f"/employees/{reminder['employeeId']}/contracts"
        before = call(cap.base, path, token)
        dialog = page.get_by_role("dialog")
        start = len(probe.events)
        target({"contractNo": reminder["contractNo"]}).click()
        expect(detail_heading(dialog, reminder["contractNo"])).to_be_visible()
        attachment = next(file for file in before["files"] if file["originalName"] == "org-qa-contract.pdf")
        preview_path = f"/files/{attachment['id']}/preview"
        raw = send(cap.base, preview_path, token)
        page.evaluate("""path => {
          const original = fetch;
          window.qaPreviewDigest = null;
          window.fetch = async (...args) => {
            const reply = await original(...args);
            if (new URL(reply.url).pathname.endsWith(path)) {
              void reply.clone().arrayBuffer().then(async bytes => {
                const digest = await crypto.subtle.digest('SHA-256', bytes);
                window.qaPreviewDigest = {bytes: bytes.byteLength, sha256: Array.from(new Uint8Array(digest),
                  part => part.toString(16).padStart(2, '0')).join('')};
              });
            }
            return reply;
          };
        }""", preview_path)
        with page.expect_response(lambda r: r.url.endswith(preview_path) and r.request.method == "GET") as response:
            button(dialog, "预览 org-qa-contract.pdf").click()
        received = response.value.body()
        cap.index["probes"]["reminder-attachment-preview"] = {
            "status": response.value.status, "type": response.value.headers.get("content-type"),
            "contentLength": response.value.headers.get("content-length"),
            "cdpBytes": len(received), "cdpSha256": hashlib.sha256(received).hexdigest(),
            "apiStatus": raw.status, "apiBytes": len(raw.body), "apiSha256": hashlib.sha256(raw.body).hexdigest(),
            "expectedSha256": hashlib.sha256(fixture_pdf()).hexdigest()
        }
        cap.save()
        page.wait_for_function("window.qaPreviewDigest !== null")
        digest = page.evaluate("window.qaPreviewDigest")
        cap.index["probes"]["reminder-attachment-preview"]["browserFetch"] = digest
        cap.save()
        assert response.value.ok and raw.status == 200 and raw.body == fixture_pdf()
        assert digest == {"bytes": len(fixture_pdf()), "sha256": hashlib.sha256(fixture_pdf()).hexdigest()}
        preview = page.get_by_role("button", name=re.compile("关闭预览"))
        expect(preview).to_be_visible()
        preview.click()
        expect(preview).to_have_count(0)
        page.wait_for_timeout(240)
        assert page.evaluate("getComputedStyle(document.body).overflow") == "hidden"
        with page.expect_download() as downloaded:
            button(dialog, "下载 org-qa-contract.pdf").click()
        assert downloaded.value.path().read_bytes() == fixture_pdf()
        button(dialog, "续约").click()
        form = dialog.get_by_role("region", name="续约合同")
        field = form.get_by_label("合同编号", exact=True)
        field.fill("QA-未提交")
        button(form, "取消").click()
        expect(detail_heading(dialog, reminder["contractNo"])).to_be_visible()
        assert call(cap.base, path, token) == before
        button(dialog, "续约").click()
        field.fill("QA-明确放弃")
        button(dialog, "关闭").click()
        button(page, "继续编辑").click()
        expect(field).to_have_value("QA-明确放弃")
        button(dialog, "关闭").click()
        button(page, "放弃修改").click()
        expect(dialog).to_have_count(0)
        assert call(cap.base, path, token) == before
        target({"contractNo": reminder["contractNo"]}).click()
        expect(detail_heading(dialog, reminder["contractNo"])).to_be_visible()

        today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
        for uncertain in (False, True):
            button(dialog, "续约").click()
            contract_no = "ORG-QA-RENEW-" + intent()[:9]
            field.fill(contract_no)
            enter_date(form, "开始日期", today + timedelta(days=1))
            enter_date(form, "结束日期", today + timedelta(days=366))
            form.get_by_label("操作原因").fill("隔离项目验证关联续约")
            submit = button(form, "保存合同")
            writes = []
            endpoint = "**/api/oa/v1" + ledger

            if uncertain:
                def lose_response(route):
                    response = route.fetch(max_retries=0)
                    try:
                        assert response.status == 201
                        writes.append(response.json()["data"])
                    finally:
                        response.dispose()
                    route.abort("failed")

                page.route(endpoint, lose_response)
                try:
                    submit.evaluate("e => { e.click(); e.click(); }")
                    retry = form.get_by_role("button", name=re.compile("重新读取合同|核对"))
                    expect(retry).to_be_visible()
                    assert len(writes) == 1
                    expect(field).to_have_value(contract_no)
                finally:
                    page.unroute(endpoint, lose_response)
                assert not any(method == "GET" and path == "/api/oa/v1/employees/:id/contracts"
                               for method, path, _ in probe.events[start:])
                saved = writes[0]
                list_request = re.compile(re.escape("/api/oa/v1" + ledger) + r"\?.*$")
                pages = []

                def reconcile_pages(route):
                    number = int(re.search(r"(?:\?|&)page=(\d+)", route.request.url)[1])
                    pages.append(number)
                    candidates = [dict(before, contractNo=contract_no + "-DECOY")] if number == 1 else [saved]
                    route.fulfill(json={"code": 0, "data": {
                        "page": number, "pageSize": 100, "total": 101, "list": candidates
                    }, "msg": "操作成功"})

                new_path = f"**/api/oa/v1/contracts/{saved['id']}"

                def wrong_origin(route):
                    response = route.fetch()
                    try:
                        payload = response.json()
                        payload["data"]["previousContractId"] = None
                        route.fulfill(response=response, json=payload)
                    finally:
                        response.dispose()

                page.route(list_request, reconcile_pages)
                page.route(new_path, wrong_origin)
                retry.click()
                expect(form.get_by_text(re.compile("来源不一致|尚未确认"))).to_be_visible()
                expect(field).to_have_value(contract_no)
                assert pages == [1, 2], pages
                page.unroute(new_path, wrong_origin)
                retry.click()
                expect(dialog.get_by_text(contract_no, exact=True).first).to_be_visible()
                assert pages == [1, 2, 1, 2], pages
                page.unroute(list_request, reconcile_pages)
                cap.index["probes"]["renew-reconcile"] = {
                    "submission": "one real 201, response dropped", "listPages": pages,
                    "paging": "controlled list pagination with a prefix decoy on page 1",
                    "wrongPreviousContractIdRejected": True,
                    "reconciledByRealDetail": True,
                }
                cap.save()
            else:
                with page.expect_response(lambda response: urlsplit(response.url).path ==
                                          "/api/oa/v1" + ledger and response.request.method == "POST") as posted:
                    submit.click()
                assert posted.value.status == 201
                saved = posted.value.json()["data"]
                expect(form).to_be_hidden()
                assert not any(method == "GET" and path == "/api/oa/v1/employees/:id/contracts"
                               for method, path, _ in probe.events[start:])
            matches = [item for item in page_all(cap.base, ledger, token) if item["contractNo"] == contract_no]
            assert len(matches) == 1
            assert (saved["id"], saved["employeeId"], saved["previousContractId"], saved["status"]) == (
                matches[0]["id"], before["employeeId"], before["id"], "DRAFT")
            after = call(cap.base, path, token)
            for key in ("id", "employeeId", "contractNo", "startDate", "endDate", "status", "files"):
                assert after[key] == before[key], key
            if not uncertain:
                page.keyboard.press("Escape")
                expect(dialog).to_have_count(0)
                target({"contractNo": reminder["contractNo"]}).click()
                expect(detail_heading(dialog, reminder["contractNo"])).to_be_visible()
        assert urlsplit(page.url).fragment == REMINDER_PATH
    finally:
        page.context.close()


def terminate_contract(cap, reminder, conflicting):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        token = cap.token(ADMIN)
        _, target = reminders(cap, page)
        path = f"/contracts/{reminder['id']}"
        before = call(cap.base, path, token)
        dialog = page.get_by_role("dialog")
        target({"contractNo": reminder["contractNo"]}).click()
        expect(detail_heading(dialog, reminder["contractNo"])).to_be_visible()
        button(dialog, "终止合同").click()
        form = dialog.get_by_role("region", name="终止合同")
        day = datetime.now(ZoneInfo("Asia/Shanghai")).date()
        enter_date(form, "终止日期", day)
        reason = form.get_by_label("操作原因")
        reason.fill("隔离项目验证终止合同")
        if conflicting:
            call(cap.base, path + "/actions", token, "POST", {
                "action": "TERMINATE", "version": before["version"],
                "reason": "隔离并发写入", "effectiveDate": day.isoformat(),
            })
        with page.expect_response(lambda response: urlsplit(response.url).path ==
                                  f"/api/oa/v1{path}/actions") as reply:
            button(form, "确认终止合同").click()
        if conflicting:
            assert reply.value.status == 409 and reply.value.json()["data"]["errorCode"] == "STALE_RECORD"
            expect(reason).to_have_value("隔离项目验证终止合同")
            expect(form.get_by_role("alert")).to_be_visible()
        else:
            assert reply.value.ok and reply.value.json()["data"]["status"] == "TERMINATED"
            expect(form).to_be_hidden()
            expect(page.get_by_text(re.compile("合同已终止.*不再符合到期提醒"))).to_be_attached()
            if dialog.is_visible():
                page.keyboard.press("Escape")
                expect(dialog).to_have_count(0)
            expect(target({"contractNo": reminder["contractNo"]})).to_have_count(0)
            assert queue_state(page)["range"] == "30"
        assert call(cap.base, path, token)["status"] == "TERMINATED"
        assert urlsplit(page.url).fragment == REMINDER_PATH
    finally:
        page.context.close()


def reminder_last_page(cap, employee_id):
    page, probe = cap.open(ADMIN, DESKTOP, "light")
    try:
        token = cap.token(ADMIN)
        goto(page, REMINDER_PATH)
        region = page.get_by_role("region", name="合同到期提醒", exact=True)
        range_control = region.locator(".oa-filter select")
        expect(range_control).to_have_count(1)
        range_control.select_option("7")
        base_path = REMINDER_PATH + "?days=7&pageSize=20"
        original = call(cap.base, base_path + "&page=1", token)
        page_size = original["pageSize"]
        extra = (1 - original["total"]) % page_size
        if original["total"] + extra <= page_size or not extra:
            extra += page_size
        today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
        prefix = "QA-LAST-" + intent()[:8]
        for index in range(extra):
            target = call(cap.base, f"/employees/{employee_id}/contracts", token, "POST", {
                "contractNo": f"{prefix}-{index}", "status": "SIGNED",
                "startDate": str(today - timedelta(days=30)), "endDate": str(today + timedelta(days=7)),
                "reason": "隔离项目末页回退验收",
            }, 201)
        last_page = (original["total"] + extra - 1) // page_size + 1
        current = call(cap.base, base_path + f"&page={last_page}", token)
        assert len(current["list"]) == 1 and current["list"][0]["contractId"] == target["id"]
        first = call(cap.base, base_path + "&page=1", token)["list"][0]["contractNo"]
        button(region, "刷新").click()
        for number in range(2, last_page + 1):
            expect(button(region, "下一页")).to_be_enabled()
            with page.expect_response(lambda response: "/contract-reminders?" in response.url and
                                      parse_qs(urlsplit(response.url).query).get("page") == [str(number)]):
                button(region, "下一页").click()
        last_row = region.get_by_role("row").filter(has_text=target["contractNo"])
        expect(last_row).to_have_count(1)
        button(last_row, "查看合同").click()
        dialog = page.get_by_role("dialog", name="合同详情")
        expect(detail_heading(dialog, target["contractNo"])).to_be_visible()
        button(dialog, "终止合同").click()
        form = dialog.get_by_role("region", name="终止合同")
        enter_date(form, "终止日期", today)
        form.get_by_label("操作原因").fill("隔离项目验证末页合同移除")
        with page.expect_response(lambda response: "/contract-reminders?" in response.url and
                                  parse_qs(urlsplit(response.url).query).get("page") == [str(last_page - 1)]) as reply:
            button(form, "确认终止合同").click()
        assert reply.value.ok
        assert reply.value.json()["data"]["page"] == last_page - 1
        assert reply.value.json()["data"]["total"] == original["total"] + extra - 1
        expect(range_control).to_have_value("7")
        expect(last_row).to_have_count(0)
        if last_page == 2:
            expect(region.get_by_text(first, exact=True)).to_be_visible()
        cap.index["probes"]["reminder-last-page"] = {
            "filteredDays": 7, "from": last_page, "to": last_page - 1, "status": "TERMINATED",
            "request": "real MySQL queue refresh, last valid page 200"
        }
        cap.save()
    finally:
        page.context.close()


def department_fixture(count):
    """Flat GET shape. Two roots and one deep chain plus bounded branches."""
    records = []
    long = "华东区域跨部门业务支持与数据治理服务组织验收长名称"
    for index in range(count):
        identifier = 10_000 + index
        parent = None if index in (0, 1) else (
            10_000 if index == 2 else 10_000 + index - 1 if index <= 20 else 10_000 + index % 8)
        name = f"模拟组织节点 {index:03d}" + (f" {long}" if index == 20 else "")
        records.append({"id": identifier, "name": name, "parentId": parent,
                        "employeeCount": index % 7, "version": 1})
    return records


def locate_department(scope, name):
    search = scope.get_by_role("searchbox").or_(scope.get_by_label(re.compile("搜索部门"))).first
    search.fill(name)
    search.press("Enter")
    node = scope.get_by_role("button", name="查看部门 " + name, exact=True)
    expect(node).to_be_in_viewport(ratio=0.55)
    return node


def chart_keyboard(cap, page, scope, fallback_name):
    reset = button(scope, "恢复视图")
    reset.click()
    reset.focus()
    samples = []
    for _ in range(46):
        page.keyboard.press("Tab")
        page.evaluate("() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))")
        sample = page.evaluate("""() => {
          const active = document.activeElement;
          if (!active?.closest('.vue-flow__node')) return null;
          const node = active.getBoundingClientRect();
          const clip = active.closest('.vue-flow').getBoundingClientRect();
          const x = node.x + node.width / 2, y = node.y + node.height / 2;
          const hit = document.elementFromPoint(x, y);
          const painted = x >= clip.left && x <= clip.right && y >= clip.top && y <= clip.bottom &&
                          x >= 0 && x <= innerWidth && y >= 0 && y <= innerHeight;
          return {
            label: active.getAttribute('aria-label'),
            painted, hit: hit === active || active.contains(hit),
            rect: [Math.round(x), Math.round(y)], clip: [Math.round(clip.top), Math.round(clip.bottom)],
            pageX: scrollX, flowScrollX: active.closest('.vue-flow').scrollLeft,
            documentFits: document.documentElement.scrollWidth <= innerWidth + 1,
          };
        }""")
        if sample is not None:
            samples.append(sample)
        if len(samples) >= 30:
            break
    button(scope, "层级树").click()
    fallback = locate_department(scope, fallback_name)
    fallback.press("Enter")
    dialog = page.get_by_role("dialog", name="部门详情")
    expect(dialog.get_by_role("heading", name=fallback_name, exact=True)).to_be_visible()
    page.keyboard.press("Escape")
    expect(dialog).to_have_count(0)
    expect(fallback).to_be_focused()
    button(scope, "结构图").click()
    cap.index["probes"]["chart-keyboard"] = {"tabSamples": samples, "nativeTreeFallback": "PASS"}
    cap.save()
    assert len(samples) >= 10, "Tab did not traverse chart controls"
    assert all(s["painted"] and s["hit"] and not s["pageX"] and s["documentFits"] for s in samples), samples


def departments_fixture(cap, size, theme, count):
    page, probe = cap.open(ADMIN, size, theme)
    records = department_fixture(count)
    try:
        tree_reads = []

        def tree(route):
            tree_reads.append(route.request.method)
            route.fulfill(json={"code": 0, "data": {"list": records}, "msg": "操作成功"})

        page.route(TREE_PATH, tree)
        goto(page, "/departments")
        expect(heading(page, "部门管理")).to_be_visible()
        scope = page.get_by_role("region", name=DEPARTMENT_REGION)
        if not records:
            expect(scope.get_by_text(re.compile("暂无部门|暂无组织"))).to_be_visible()
            cap.shot(page, probe, "departments-synthetic-empty", ADMIN, size, theme, state="controlled GET fixture, zero records")
        else:
            node = lambda item: scope.get_by_role("button", name="查看部门 " + item["name"], exact=True)
            for item in records[:3]:
                expect(node(item)).to_be_visible()
            expect(node(records[3])).to_have_count(0)
            canvas = scope.locator(".vue-flow")
            if size[0] < 600:
                expect(canvas).to_be_hidden()
                tree = scope.get_by_role("list", name="部门层级列表")
                expect(tree).to_be_visible()
                parent_branch = tree.get_by_role("button", name=f"收起{records[0]['name']}的下级部门")
                expect(parent_branch).to_have_attribute("aria-expanded", "true")
                assert parent_branch.bounding_box()["height"] >= 44
                child = node(records[2])
                assert child.evaluate("""e => parseFloat(getComputedStyle(e.closest('li'), '::before').borderLeftWidth) > 0""")
                parent_branch.press("Enter")
                expect(child).to_have_count(0)
                tree.get_by_role("button", name=f"展开{records[0]['name']}的下级部门").press("Enter")
                expect(child).to_be_visible()
            else:
                expect(canvas).to_be_visible()
                expect(canvas.locator(".vue-flow__node.draggable")).to_have_count(0)
            selected = locate_department(scope, records[20]["name"])
            expect(selected).to_contain_text(records[20]["name"])
            assert len(tree_reads) == 1
            cap.shot(page, probe, f"departments-synthetic-{count}", ADMIN, size, theme,
                     state=f"controlled GET fixture, {count} records and a 20-level branch")
            if size[0] >= 600:
                expand = scope.get_by_role("button", name=re.compile("展开全部"))
                collapse = button(scope, "收起至两层")
                expand.click()
                expect(canvas.locator(".vue-flow__node")).to_have_count(count)
                if count == 100:
                    cap.run("100-node-chart-keyboard-focus", chart_keyboard, cap, page, scope, records[20]["name"])
                collapse.click()
                roots = {item["id"] for item in records if item["parentId"] is None}
                two_levels = sum(item["parentId"] is None or item["parentId"] in roots for item in records)
                expect(canvas.locator(".vue-flow__node")).to_have_count(two_levels)
                zoom = scope.locator("output[aria-label='当前缩放']")
                previous_zoom = zoom.inner_text()
                button(scope, "放大组织图").click()
                expect(zoom).not_to_have_text(previous_zoom)
                button(scope, "恢复视图").click()
                expect(zoom).to_have_text("100%")
            if size[0] < 600:
                options = scope.locator("summary").filter(has_text="浏览选项")
                expect(options.locator("xpath=..")).not_to_have_attribute("open", "")
                options.click()
                expect(options.locator("xpath=..")).to_have_attribute("open", "")
            button(scope, "列表").click()
            expect(canvas).to_be_hidden()
            button(scope, "结构图").click()
            expect(canvas).to_be_visible()
        assert tree_reads and set(tree_reads) == {"GET"}
    finally:
        page.context.close()


def department_live(cap, role, parent, child):
    page, probe = cap.open(role, DESKTOP, "light")
    try:
        goto(page, "/departments")
        scope = page.get_by_role("region", name=DEPARTMENT_REGION)
        node = locate_department(scope, parent["name"])
        view = scope.locator(".vue-flow__transformationpane")
        expect(view).to_be_visible()
        expect(view).to_have_attribute("style", re.compile("transform:"))
        transform = view.evaluate("node => node.style.transform")
        expect(node).to_have_attribute("aria-pressed", "true")
        with page.expect_response(lambda r: r.url.endswith("/departments/tree") and r.ok):
            button(scope, "刷新").click()
        expect(node).to_have_attribute("aria-pressed", "true")
        expect(view).to_have_attribute("style", re.compile(re.escape(transform)))
        node.click()
        dialog = page.get_by_role("dialog")
        expect(dialog.get_by_text(parent["name"], exact=True).first).to_be_visible()
        expect(view).to_have_attribute("style", re.compile(re.escape(transform)))
        cap.shot(page, probe, "department-live-details", role, DESKTOP, "light",
                 state="isolated MySQL, real organization node selected")
        if role == VIEWER:
            for label in ("编辑部门", "新增下级", "保存部门"):
                expect(button(dialog, label)).to_have_count(0)
            button(dialog, "变更历史").click()
            expect(dialog.get_by_role("region", name="部门变更历史")).to_be_visible()
            return
        button(dialog, "编辑部门").click()
        token = cap.token(role)
        field = dialog.get_by_label("部门名称", exact=True)
        expect(field).to_have_value(parent["name"])
        upper = dialog.locator("#department-parent")
        if upper.evaluate("node => node.tagName") == "SELECT":
            blocked = upper.locator("option").evaluate_all("(options) => options.map(o => o.value)")
            assert not {str(parent["id"]), str(child["id"])}.intersection(blocked)
        else:
            upper.click()
            for item in (parent, child):
                expect(page.get_by_role("option", name=item["name"], exact=True)).to_have_count(0)
            page.keyboard.press("Escape")
        endpoint = f"/departments/{parent['id']}"
        changed = parent["name"] + " 已保存"
        field.fill(changed)
        with page.expect_response(lambda r: r.url.endswith(endpoint) and r.request.method == "PUT") as reply:
            button(dialog, "保存部门").click()
        saved = reply.value.json()["data"]
        assert reply.value.ok and saved["name"] == changed and saved["version"] == parent["version"] + 1
        expect(dialog.get_by_text(changed, exact=True).first).to_be_visible()
        expect(view).to_have_attribute("style", re.compile(re.escape(transform)))
        button(dialog, "新增下级").click()
        added_name = "隔离新增下级 " + intent()[:8]
        expect(upper).to_have_value(str(parent["id"]))
        field.fill(added_name)
        with page.expect_response(lambda r: r.url.endswith("/departments") and r.request.method == "POST") as reply:
            button(dialog, "保存部门").click()
        added = reply.value.json()["data"]
        assert reply.value.status == 201 and added["parentId"] == parent["id"] and added["name"] == added_name
        expect(dialog.get_by_text(added_name, exact=True).first).to_be_visible()
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        locate_department(scope, changed).click()
        button(dialog, "编辑部门").click()
        call(cap.base, endpoint, token, "PUT", {
            "name": changed + " 并发", "parentId": None, "version": saved["version"]
        })
        field.fill(changed + " 未保存")
        with page.expect_response(lambda r: r.url.endswith(endpoint) and r.request.method == "PUT") as reply:
            button(dialog, "保存部门").click()
        assert reply.value.status == 409
        assert reply.value.json()["data"]["errorCode"] == "STALE_RECORD"
        expect(field).to_have_value(changed + " 未保存")
        expect(dialog.get_by_role("alert")).to_be_visible()
    finally:
        page.context.close()


def department_gallery(cap, size, theme, *, before):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        target = "演示部门"
        with page.expect_response(lambda r: r.ok and urlsplit(r.url).path.startswith("/api/oa/v1/departments")):
            goto(page, "/departments")
        page.wait_for_load_state("networkidle")
        first = None
        tones = None
        if not before:
            content = (page.get_by_role("list", name="部门层级列表") if size[0] < 600 else
                       page.get_by_role("region", name="部门组织结构图"))
            first = content.locator(".oa-org-tree-name" if size[0] < 600 else ".oa-org-node-title").first
            selector = ".oa-org-tree-name strong" if size[0] < 600 else ".oa-org-node-title"
            tones = contrast(content, {"departmentName": selector})
            assert min(tones.values()) >= 4.5, tones
        cap.shot(page, probe, "department-real-default", ADMIN, size, theme, state="same real demo organization",
                 anchors={"firstDepartment": first} if first is not None else None, sample=tones)
        if before:
            entry = page.get_by_text(target, exact=True).first
            expect(entry).to_be_visible()
            cap.shot(page, probe, "department-real-locate", ADMIN, size, theme,
                     state="old paginated list, same demo department visible", anchors={"department": entry})
            entry.locator("xpath=ancestor::*[.//button[normalize-space()='变更历史']][1]").get_by_role(
                "button", name="变更历史").click()
            history = page.get_by_role("region", name="部门变更历史", exact=True)
            expect(history).to_be_visible()
            history.scroll_into_view_if_needed()
        else:
            entry = locate_department(page, target)
            cap.shot(page, probe, "department-real-locate", ADMIN, size, theme,
                     state="new search located the same demo department", anchors={"department": entry})
            entry.click()
            dialog = page.get_by_role("dialog")
            expect(dialog.get_by_text(target, exact=True).first).to_be_visible()
            cap.shot(page, probe, "department-real-details", ADMIN, size, theme,
                     state="new details drawer for the same demo department", anchors={"drawer": dialog})
            dialog.get_by_role("button", name=re.compile("变更历史|查看历史")).click()
            history = dialog.get_by_role("region", name=re.compile("历史")).first
        expect(history).to_be_visible()
        page.wait_for_load_state("networkidle")
        cap.shot(page, probe, "department-real-history", ADMIN, size, theme,
                 state="same demo department audit history", anchors={"history": history})
    finally:
        page.context.close()


def department_edge_contrast(cap, size, theme):
    page, probe = cap.open(ADMIN, size, theme)
    try:
        goto(page, "/departments")
        narrow = size[0] < 600
        selector = ".oa-org-tree-list .has-parent" if narrow else ".vue-flow__edge-path"
        edge = page.locator(selector).first
        edge.wait_for(state="attached")
        result = edge.evaluate(r"""(e, narrow) => {
          const style = getComputedStyle(e, narrow ? '::before' : null);
          const rect = e.getBoundingClientRect();
          let background;
          for (let ancestor = e.parentElement; ancestor && !background; ancestor = ancestor.parentElement) {
            const current = getComputedStyle(ancestor).backgroundColor;
            if (current !== 'transparent' && current !== 'rgba(0, 0, 0, 0)') background = current;
          }
          const stroke = narrow ? style.borderLeftColor : style.stroke;
          const width = narrow ? style.borderLeftWidth : style.strokeWidth;
          const rgba = value => (value.match(/[\d.]+/g) || []).map(Number);
          const foreground = rgba(stroke), base = rgba(background);
          const opacity = Number(style.opacity) * (narrow ? 1 : Number(style.strokeOpacity)) * (foreground[3] ?? 1);
          const blended = foreground.slice(0, 3).map((channel, index) => channel * opacity + base[index] * (1 - opacity));
          const lum = color => color.slice(0, 3).map(x => x / 255)
            .map(x => x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4)
            .reduce((sum, value, i) => sum + [.2126, .7152, .0722][i] * value, 0);
          const values = [lum(blended), lum(base)].sort((a, b) => a - b);
          return {stroke, background, width, opacity, contrast: (values[1] + .05) / (values[0] + .05),
            inViewport: rect.x >= 0 && rect.x < innerWidth && rect.y < innerHeight && rect.bottom > 0};
        }""", narrow)
        label = f"{size[0]}-{theme}"
        cap.index["probes"][label] = result
        cap.save()
        cap.shot(page, probe, "department-edge", ADMIN, size, theme)
        print(label, result, flush=True)
        assert result["inViewport"] and float(result["width"].removesuffix("px")) > 0
        assert result["contrast"] >= 3, result
    finally:
        page.context.close()


def seed_isolated(base):
    assert re.fullmatch(r"hr-oa-ci-[0-9a-f]{12}", os.environ.get("OA_CI_PROJECT", ""))
    assert urlsplit(base).port == int(os.environ["OA_CI_WEB_PORT"])
    admin = login(base, ADMIN, "HR_ADMIN")["token"]
    viewer = login(base, VIEWER, "HR_VIEWER")["token"]
    employee_id = login(base, "employee", "EMPLOYEE")["user"]["employeeId"]
    suffix = intent()[:8]
    parent = call(base, "/departments", admin, "POST", {"name": f"组织验收父部门 {suffix}"}, 201)
    child = call(base, "/departments", admin, "POST",
                 {"name": f"组织验收子部门 {suffix}", "parentId": parent["id"]}, 201)
    path = f"/departments/{parent['id']}"
    edit = {"name": parent["name"], "parentId": child["id"], "version": parent["version"]}
    call(base, path, admin, "PUT", edit, 400)
    call(base, path, viewer, "PUT", edit, 403)
    tree = {item["id"]: item for item in call(base, "/departments/tree", admin)["list"]}
    assert tree[parent["id"]]["parentId"] is None
    assert tree[child["id"]]["parentId"] == parent["id"]
    assert tree[parent["id"]]["version"] == parent["version"]
    today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
    due = []
    for index in range(3):
        due.append(call(base, f"/employees/{employee_id}/contracts", admin, "POST", {
            "contractNo": f"ORG-QA-{suffix}-{index}", "status": "SIGNED",
            "startDate": str(today - timedelta(days=250)), "endDate": str(today),
            "reason": "隔离项目合同提醒界面验收"
        }, 201))
    upload(base, due[0]["id"], admin, "org-qa-contract.pdf", fixture_pdf())
    return parent, child, due


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--runtime", required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--prepare-isolated", action="store_true")
    parser.add_argument("--section", choices=("acceptance", "gallery", "gallery-before", "edge-contrast"), default="acceptance")
    parser.add_argument("--case", help="run matching scenario labels only")
    args = parser.parse_args()
    args.phase = "before" if args.section == "gallery-before" else "after"
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("export OA_SEED_PASSWORD")
    isolated = seed_isolated(args.base) if args.prepare_isolated else None
    if isolated:
        print("PASS real MySQL department cycle rejection and read-only write guard", flush=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            cap = Capture(browser, args.base, args)
            def run(label, callback, *inputs):
                if not args.case or re.search(args.case, label):
                    cap.run(label, callback, *inputs)
            if args.section == "edge-contrast":
                for size in (DESKTOP, PHONE):
                    for theme in ("light", "dark"):
                        run(f"edge-contrast-{size}-{theme}", department_edge_contrast, cap, size, theme)
                return int("FAIL" in cap.index["latestChecks"].values())
            if args.section.startswith("gallery"):
                for size, theme in ((DESKTOP, "light"), ((1366, 768), "dark"), (PHONE, "dark")):
                    run(f"department-gallery-{size}", lambda: department_gallery(
                        cap, size, theme, before=args.phase == "before"))
                return int("FAIL" in cap.index["latestChecks"].values())
            run("open-close-desktop", open_close, cap, DESKTOP, "light")
            run("open-close-phone", open_close, cap, PHONE, "dark")
            run("stale-response", stale_response, cap)
            run("rapid-reopen-lock-reduced-motion", rapid_reopen_lock, cap)
            run("error-states", error_states, cap)
            run("viewer-contract-read-only", viewer_contract, cap)
            if isolated:
                run("isolated-contract-cancel-renew-and-reconcile", contract_writes, cap, isolated[2][0])
                run("isolated-contract-terminate-and-refresh", terminate_contract, cap, isolated[2][1], False)
                run("isolated-contract-version-conflict", terminate_contract, cap, isolated[2][2], True)
                run("isolated-contract-last-page-fallback", reminder_last_page, cap, isolated[2][0]["employeeId"])
                run("department-viewer-real", department_live, cap, VIEWER, *isolated[:2])
                run("department-save-and-conflict-real", department_live, cap, ADMIN, *isolated[:2])
            for size, theme, count in ((DESKTOP, "light", 0), (DESKTOP, "light", 100),
                                       (DESKTOP, "dark", 500), (PHONE, "dark", 100)):
                run(f"department-fixture-{count}-{size}", departments_fixture, cap, size, theme, count)
            return int("FAIL" in cap.index["latestChecks"].values())
        finally:
            browser.close()


if __name__ == "__main__":
    raise SystemExit(main())
