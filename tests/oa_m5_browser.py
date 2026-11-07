"""M5 browser acceptance against the frozen local stack and synthetic records."""
import argparse
from datetime import date, timedelta
import json
from pathlib import Path
import re

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_browser import button
from oa_commercial_browser import contrast
from oa_m2_api import call as post, intent, request_detail
from oa_m2_browser import login as h5_login, open_record, tab
from oa_m3_api import call
from oa_m4_browser import menu, pc_page, screenshot, set_theme
from oa_m5_api import annual
from oa_pure_browser import check_storage


def heading(page, title):
    return page.get_by_role("heading", name=title, exact=True)


def show(page, path, title):
    page.evaluate("(path) => { window.location.hash = path }", path)
    expect(heading(page, title)).to_be_visible()


def wait_write(page, route, trigger, method="POST"):
    with page.expect_response(lambda reply: reply.url.endswith(route) and reply.request.method == method) as changed:
        trigger.click()
    assert changed.value.ok, f"{method} {route} returned {changed.value.status}"
    return changed.value.json()["data"]


def dashboard_contrast(page, label):
    region = page.get_by_role("region", name="工作台", exact=True)
    ratios = contrast(region, {"count": ".el-statistic__number", "caption": ".oa-dashboard-section .oa-muted"})
    assert min(ratios.values()) >= 4.5, f"{label} dashboard contrast: {ratios}"
    print(f"M5 {label} dashboard contrast {ratios}", flush=True)


def balance_card_contrast(region):
    return region.evaluate("""root => {
        const node = root.querySelector('td[data-label]');
        const context = document.createElement('canvas').getContext('2d', {willReadFrequently:true});
        function luminance(color) {
            context.clearRect(0,0,1,1); context.fillStyle=color; context.fillRect(0,0,1,1);
            const [r,g,b] = Array.from(context.getImageData(0,0,1,1).data).slice(0,3).map(value => {
                const channel=value/255; return channel<=.04045 ? channel/12.92 : ((channel+.055)/1.055)**2.4;
            });
            return r*.2126 + g*.7152 + b*.0722;
        }
        let surface=node;
        while(getComputedStyle(surface).backgroundColor==='rgba(0, 0, 0, 0)') surface=surface.parentElement;
        const bg=luminance(getComputedStyle(surface).backgroundColor);
        const fg=luminance(getComputedStyle(node,'::before').color);
        return (Math.max(fg,bg)+.05)/(Math.min(fg,bg)+.05);
    }""")


def pc_preview(page, title, day, leave_type, artifacts):
    menu(page, "审批中心")
    button(page, "发起申请").click()
    editor = page.get_by_role("region", name="发起申请", exact=True)
    editor.get_by_role("list", name="申请类型").get_by_role("button", name=re.compile("请假申请")).click()
    editor.get_by_label("标题", exact=True).fill(title)
    editor.locator("#pc-leave-type").select_option(str(leave_type))
    for field in ("开始日期", "结束日期"):
        editor.get_by_label(field, exact=True).fill(day.isoformat())
    editor.get_by_label("开始时段").select_option("PM")
    editor.get_by_label("结束时段").select_option("PM")
    editor.get_by_label("请假事由").fill("M5 模拟浏览器验收")
    calculate = button(editor, "预览请假天数")
    preview = wait_write(page, "/leave-preview", calculate)
    assert preview["canSubmit"] and preview["calculation"]["totalHalfDays"] == 1
    region = page.get_by_role("region", name="请假计算预览")
    expect(region.get_by_role("heading", name=re.compile("0.5 天"))).to_be_visible()
    editor.get_by_label("开始时段").select_option("AM")
    expect(region).to_have_count(0)
    editor.get_by_label("开始时段").select_option("PM")
    wait_write(page, "/leave-preview", calculate)
    page.set_viewport_size({"width": 1440, "height": 1600})
    editor.get_by_role("heading", name="发起申请", exact=True).evaluate("node => node.scrollIntoView({block:'start'})")
    screenshot(page, artifacts, "pc-leave-preview-1440.png")
    page.set_viewport_size({"width": 1440, "height": 960})
    accepted = []

    def drop_committed_reply(route):
        reply = route.fetch()
        assert reply.status == 201
        accepted.append((route.request.header_value("idempotency-key"), route.request.post_data, reply.json()["data"]["id"]))
        route.abort()

    pattern = "**/api/oa/v1/requests"
    page.route(pattern, drop_committed_reply, times=1)
    button(page, "提交申请").click()
    retry = button(page, "重试原提交")
    expect(retry).to_be_visible()
    with page.expect_response(lambda reply: reply.url.endswith("/requests") and reply.request.method == "POST") as retried:
        retry.click()
    assert retried.value.status == 201 and retried.value.header_value("Idempotency-Replayed") == "true"
    result = retried.value.json()["data"]
    assert accepted == [(retried.value.request.header_value("idempotency-key"), retried.value.request.post_data, result["id"])]
    expect(heading(page, title)).to_be_visible()
    screenshot(page, artifacts, "pc-leave-snapshot-1440.png")
    return result


def live(browser, base, artifacts):
    employee, employee_traffic, applicant = pc_page(browser, base, "employee")
    admin_page, admin_traffic, administrator = pc_page(browser, base, "hr_admin")
    admin, token = administrator["token"], applicant["token"]
    employee_id = applicant["user"]["employeeId"]
    calendar = call(base, "/leave-calendar", admin)
    exceptions = {row["date"]: row["working"] for row in calendar["exceptions"]}
    leave_type, = [row for row in call(base, "/leave-types", token)["list"] if row["usesBalance"]]
    for offset in range(14, 104):
        day = date.today() + timedelta(days=offset)
        if exceptions.get(day.isoformat(), day.isoweekday() in calendar["workingWeekdays"]):
            check = post(base, "/leave-preview", token, payload={"leaveTypeId": leave_type["id"],
                         "startDate": str(day), "endDate": str(day), "startHalf": "PM", "endHalf": "PM"})
            if "LEAVE_OVERLAP" not in check["issues"]:
                break
    else:
        raise AssertionError("no unoccupied working half day available for the synthetic browser fixture")
    row = annual(base, admin, employee_id, day.year)
    if not row or row["remainingHalfDays"] < 2:
        used = row["usedHalfDays"] if row else 0
        consumed = row["approvalUsedHalfDays"] + row["reservedHalfDays"] if row else 0
        call(base, f"/employees/{employee_id}/annual-leave/{day.year}", admin, "PUT", {
            "version": row["version"] if row else 0, "usedHalfDays": used,
            "entitledHalfDays": used + consumed + 2, "note": row["note"] if row else "模拟 M5 验收额度",
            "reason": "M5 模拟浏览器验收",
        })
    annual_before = annual(base, admin, employee_id, day.year)
    menu(employee, "工作台")
    expect(button(employee, "查看我的申请")).to_be_visible()
    expect(button(employee, "查看审批异常")).to_have_count(0)
    dashboard_contrast(employee, "1440px light")
    screenshot(employee, artifacts, "employee-dashboard-1440.png")
    title = "M5 模拟跨端请假 " + intent()[:8]
    employee_traffic.sensitive.add(title)
    created = pc_preview(employee, title, day, leave_type["id"], artifacts)
    request_id = created["id"]
    snapshot = request_detail(base, request_id, token)["leaveCalculation"]
    assert snapshot["totalHalfDays"] == 1
    assert annual(base, admin, employee_id, day.year)["reservedHalfDays"] == annual_before["reservedHalfDays"] + 1

    mobile_employee = browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
    h5_login(mobile_employee, base, "employee")
    tab(mobile_employee, "我的申请").click()
    open_record(mobile_employee, title)
    button(mobile_employee, "催办").click()
    wait_write(mobile_employee, f"/requests/{request_id}/actions", button(mobile_employee, "确认催办"))
    screenshot(mobile_employee, artifacts, "h5-leave-reminder-390.png")

    mobile_approver = browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
    h5_login(mobile_approver, base, "manager")
    tab(mobile_approver, "待办").click()
    open_record(mobile_approver, title)
    screenshot(mobile_approver, artifacts, "h5-leave-task-390.png")
    first = request_detail(base, request_id, token)["tasks"][0]
    button(mobile_approver, "通过").click()
    wait_write(mobile_approver, f"/tasks/{first['id']}/actions", button(mobile_approver, "确认通过"))

    finance, finance_traffic, _ = pc_page(browser, base, "finance")
    menu(finance, "工作台")
    expect(button(finance, "查看待我审批")).to_be_visible()
    button(finance, "查看待我审批").click()
    expect(heading(finance, "待办")).to_be_visible()
    open_record(finance, title)
    second = request_detail(base, request_id, token)["tasks"][-1]
    button(finance, "通过").click()
    wait_write(finance, f"/tasks/{second['id']}/actions", button(finance, "确认通过"))
    assert request_detail(base, request_id, token)["status"] == "APPROVED"
    used = annual(base, admin, employee_id, day.year)
    assert used["approvalUsedHalfDays"] == annual_before["approvalUsedHalfDays"] + 1

    show(employee, "/my-leave", "我的假期")
    ledger = employee.get_by_role("region", name="余额流水")
    expect(button(ledger, f"查看关联申请 #{request_id}").first).to_be_visible()
    ratios = contrast(employee.get_by_role("region", name="年假记录"), {"header": "thead th .cell", "value": "tbody td .cell"})
    print(f"M5 1440px light annual balance contrast {ratios}", flush=True)
    assert min(ratios.values()) >= 4.5
    screenshot(employee, artifacts, "pc-leave-ledger-1440.png")
    settings_screens(browser, base, admin_page, artifacts, request_id)
    final = request_detail(base, request_id, token)
    assert final["status"] == "CANCELLED" and final["leaveCalculation"] == snapshot
    after = annual(base, admin, employee_id, day.year)
    for key in ("reservedHalfDays", "approvalUsedHalfDays", "remainingHalfDays"):
        assert after[key] == annual_before[key]
    button(mobile_employee, "我的假期").click()
    expect(heading(mobile_employee, "我的假期")).to_be_visible()
    expect(mobile_employee.get_by_text("撤销冲回", exact=False).first).to_be_visible()
    mobile_employee.emulate_media(color_scheme="dark", reduced_motion="reduce")
    ratios = contrast(mobile_employee.get_by_role("region", name="我的假期", exact=True),
                      {"label": ".h5-details dt", "value": ".h5-details dd"})
    print(f"M5 H5 390px dark annual balance contrast {ratios}", flush=True)
    assert min(ratios.values()) >= 4.5
    screenshot(mobile_employee, artifacts, "h5-leave-ledger-390.png")
    for page, traffic in ((employee, employee_traffic), (finance, finance_traffic), (admin_page, admin_traffic)):
        check_storage(page, traffic.sensitive)
        traffic.assert_local()
    print("PASS M5 PC preview, H5 reminder and approval, PC final approval, HR cancellation and shared ledger", flush=True)
    balance = {key: after[key] for key in ("entitledHalfDays", "usedHalfDays", "approvalUsedHalfDays",
                                          "reservedHalfDays", "remainingHalfDays")}
    return {"requestId": request_id, "employeeId": employee_id, "year": day.year,
            "calculation": snapshot, "annual": balance}


def settings_screens(browser, base, page, artifacts, request_id):
    menu(page, "工作台")
    expect(button(page, "查看已批请假")).to_be_visible()
    screenshot(page, artifacts, "admin-dashboard-1440.png")
    button(page, "查看已批请假").click()
    expect(heading(page, "已批请假")).to_be_visible()
    record = page.get_by_role("listitem").filter(has_text=re.compile(rf"申请 #{request_id}\b"))
    expect(record).to_be_visible()
    screenshot(page, artifacts, "approved-leave-1440.png")
    button(record, "撤销此请假").click()
    button(page, "撤销请假").click()
    page.get_by_label("操作原因", exact=True).fill("M5 模拟撤销并核对余额")
    wait_write(page, f"/requests/{request_id}/actions", button(page, "确认撤销请假"))
    show(page, "/workflow-settings", "请假与审批配置")
    for label, title, name in (("请假类型", "请假类型", "leave-types"), ("工作日历", "工作日历", "work-calendar"),
                               ("审批规则", "顺序审批规则", "approval-rules")):
        segment = button(page.get_by_role("navigation", name="配置分组"), label)
        segment.focus()
        page.keyboard.press("Space")
        expect(segment).to_have_attribute("aria-pressed", "true")
        expect(heading(page, title)).to_be_visible()
        if name == "approval-rules":
            button(page, "新增审批规则").click()
            expect(heading(page, "新增审批规则")).to_be_focused()
            rule_name = "M5 模拟停用规则 " + intent()[:8]
            page.get_by_label("规则名称").fill(rule_name)
            page.get_by_label("申请类型").select_option("EXPENSE")
            page.get_by_label("使用状态").select_option(label="停用")
            page.get_by_label("步骤名称").fill("模拟主管初审")
            page.locator(".oa-rule-steps").get_by_label("审批人").select_option(label="演示部门负责人")
            rule = wait_write(page, "/approval-rules", button(page, "保存审批规则"))
            assert rule["enabled"] is False and rule["name"] == rule_name
            expect(page.get_by_text(rule_name, exact=True)).to_be_visible()
        heading(page, title).scroll_into_view_if_needed()
        screenshot(page, artifacts, f"{name}-1440.png")
    show(page, "/workflow-exceptions", "审批异常")
    screenshot(page, artifacts, "workflow-exceptions-1440.png")
    phone, traffic, _ = pc_page(browser, base, "hr_admin", 390)
    set_theme(phone)
    show(phone, "/dashboard", "工作台")
    expect(button(phone, "查看审批异常")).to_be_visible()
    dashboard_contrast(phone, "390px dark")
    screenshot(phone, artifacts, "admin-dashboard-dark-390.png")
    show(phone, "/workflow-settings", "请假与审批配置")
    if not heading(phone, "工作日历").is_visible():
        button(phone, "工作日历").click()
    expect(heading(phone, "工作日历")).to_be_visible()
    button(phone, "编辑工作日历").click()
    expect(phone.get_by_role("heading", name=re.compile("编辑工作日历"))).to_be_focused()
    screenshot(phone, artifacts, "work-calendar-dark-390.png")
    button(phone, "取消").click()
    show(phone, "/workflow-exceptions", "审批异常")
    traffic.assert_local()
    balance_phone, balance_traffic, _ = pc_page(browser, base, "employee", 390)
    set_theme(balance_phone)
    show(balance_phone, "/my-leave", "我的假期")
    annual_region = balance_phone.get_by_role("region", name="年假记录", exact=True)
    expect(annual_region.locator("td[data-label]").first).to_be_visible()
    ratio = balance_card_contrast(annual_region)
    print(f"M5 PC 390px dark annual card label contrast {ratio:.2f}:1", flush=True)
    assert ratio >= 4.5
    screenshot(balance_phone, artifacts, "pc-balance-dark-390.png")
    balance_traffic.assert_local()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/m5/browser-final"))
    parser.add_argument("--state", type=Path, default=Path("tests/results/m5/browser-state.json"))
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            state = live(browser, args.base, args.artifacts)
        finally:
            browser.close()
    args.state.write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
