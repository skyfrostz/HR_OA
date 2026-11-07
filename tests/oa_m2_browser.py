"""Real Chromium H5 acceptance using the local M2 API and development MySQL."""
import argparse
import os
from pathlib import Path
import secrets

from playwright.sync_api import Error, expect, sync_playwright

from oa_api import PREFIX, local_base
from oa_browser import assert_no_persistent_storage, button, login as pc_login


def login(page, base, username):
    page.goto(base + "/h5.html", wait_until="networkidle")
    expect(page.get_by_role("heading", name="登录移动审批")).to_be_visible()
    page.get_by_label("账号", exact=True).fill(username)
    try:
        page.get_by_label("密码", exact=True).fill(os.environ["OA_SEED_PASSWORD"])
    except Error:
        raise AssertionError("the password field could not be filled") from None
    with page.expect_response(lambda reply: reply.url.endswith(PREFIX + "/auth/login")) as response:
        button(page, "登录").click()
    assert response.value.ok, "H5 login failed"
    expect(button(page, "退出登录")).to_be_visible()


def tab(page, name):
    return page.locator(".h5-nav").get_by_text(name, exact=True)


def no_overflow(page):
    assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "H5 horizontal overflow"


def unobstructed(control):
    control.scroll_into_view_if_needed()
    assert control.evaluate("""element => {
        const rect = element.getBoundingClientRect()
        const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
        return hit === element || (hit !== null && element.contains(hit))
    }"""), "control is covered or outside the viewport"


def check_pc(browser, base, artifacts):
    page = browser.new_page(viewport={"width": 1440, "height": 960})
    page.goto(base + "/oa.html", wait_until="networkidle")
    pc_login(page, "hr_admin")
    h5_link = page.get_by_role("link", name="移动审批", exact=True)
    expect(h5_link).to_be_visible()
    assert h5_link.get_attribute("href").endswith("h5.html")
    no_overflow(page)
    page.screenshot(path=artifacts / "pc-employee-list-1440.png", animations="disabled")
    table = page.get_by_role("table", name="员工列表")
    button(table, "查看档案").first.click()
    expect(page.locator("#employee-heading")).to_be_visible()
    expect(page.get_by_text("约定工资", exact=True)).to_be_visible()
    unobstructed(button(page, "返回员工列表"))
    no_overflow(page)
    page.screenshot(path=artifacts / "pc-employee-detail-1440.png", animations="disabled", full_page=True)
    assert_no_persistent_storage(page)
    if h5_link.get_attribute("target") == "_blank":
        with page.expect_popup() as opened:
            h5_link.click()
        page = opened.value
    else:
        h5_link.click()
    expect(page.get_by_role("heading", name="登录移动审批")).to_be_visible()
    expect(page.get_by_role("link", name="PC 管理端")).to_be_visible()
    page.get_by_role("link", name="PC 管理端").click()
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()
    print("PASS PC 1440px list and detail, navigation to H5 and back, no overflow", flush=True)


def open_record(page, title):
    page.wait_for_load_state("networkidle")
    row = page.get_by_role("button", name=title, exact=False)
    while not row.count() and button(page, "下一页").count() and button(page, "下一页").is_enabled():
        with page.expect_response(lambda response: response.ok and "pageSize=" in response.url):
            button(page, "下一页").click()
        page.wait_for_load_state("networkidle")
    expect(row).to_have_count(1)
    row.click()
    expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()


def submit_after_lost_response(page, title, content):
    page.get_by_role("button", name="通用申请", exact=False).click()
    page.get_by_label("标题", exact=True).fill(title)
    page.get_by_label("申请内容", exact=True).fill(content)
    captured = []
    committed = []
    path = "**/api/oa/v1/requests"

    def capture(request):
        if request.method == "POST" and request.url.endswith(PREFIX + "/requests"):
            captured.append((request.header_value("idempotency-key"), request.post_data))

    def lose_response(route):
        if route.request.method != "POST":
            route.continue_()
            return
        response = route.fetch(max_retries=0)
        committed.append(response.status)
        response.dispose()
        route.abort()

    page.on("request", capture)
    page.route(path, lose_response)
    try:
        button(page, "提交申请").locator("xpath=ancestor::form").evaluate(
            "(form) => { form.requestSubmit(); form.requestSubmit() }"
        )
        expect(button(page, "重试原提交")).to_be_visible()
        expect(page.get_by_label("标题", exact=True)).to_be_disabled()
        assert committed == [201] and len(captured) == 1
    finally:
        page.unroute(path, lose_response)
    try:
        with page.expect_response(lambda response: response.url.endswith(PREFIX + "/requests")
                                  and response.request.method == "POST") as retried:
            button(page, "重试原提交").click()
        response = retried.value
        assert response.status == 201 and response.header_value("idempotency-replayed") == "true"
        assert len(captured) == 2 and captured[0] == captured[1] and captured[0][0]
        request = response.json()["data"]
        expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()
        return request["id"]
    finally:
        page.remove_listener("request", capture)


def approve(page):
    button(page, "通过").click()
    with page.expect_response(lambda response: response.url.endswith("/actions")) as info:
        button(page, "确认通过").click()
    assert info.value.ok, f"H5 approval failed with {info.value.status}"
    expect(page.get_by_role("heading", name="处理待办")).to_have_count(0)


def check(browser, base, artifacts):
    title = "H5 模拟通用申请 " + secrets.token_hex(5)
    content = "测试专用内容 " + secrets.token_hex(4)

    def phone():
        return browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)

    employee, manager, delegate, finance = (phone() for _ in range(4))
    login(employee, base, "employee")
    tab(employee, "发起申请").click()
    expect(employee.get_by_role("button", name="通用申请", exact=False)).to_be_visible()
    no_overflow(employee)
    employee.screenshot(path=artifacts / "h5-workbench-390.png", animations="disabled")
    identifier = submit_after_lost_response(employee, title, content)
    assert content not in employee.url and title not in employee.url
    no_overflow(employee)
    assert_no_persistent_storage(employee)
    print("PASS H5 employee submission, synchronous double-click guard, same key/body after lost response", flush=True)

    login(manager, base, "manager")
    tab(manager, "待办").click()
    open_record(manager, title)
    no_overflow(manager)
    unobstructed(button(manager, "通过"))
    unobstructed(button(manager, "转交"))
    dock = manager.get_by_role("group", name="当前任务操作")
    navigation = manager.locator(".h5-nav-shell")
    assert dock.evaluate("element => getComputedStyle(element).position") == "fixed"
    assert navigation.evaluate("element => getComputedStyle(element).position") == "fixed"
    dock_bounds, nav_bounds = dock.bounding_box(), navigation.bounding_box()
    assert dock_bounds["y"] + dock_bounds["height"] <= nav_bounds["y"] + 1, "H5 dock overlaps navigation"
    manager.evaluate("window.scrollTo(0, document.documentElement.scrollHeight)")
    last_entry = manager.get_by_role("region", name="操作记录").locator("li").last
    bounds = last_entry.bounding_box()
    assert bounds["y"] + bounds["height"] <= dock_bounds["y"] + 1, "last history entry obscured by H5 dock"
    manager.evaluate("window.scrollTo(0, 0)")
    manager.screenshot(path=artifacts / "h5-task-detail-390.png", animations="disabled")
    button(manager, "转交").click()
    target = manager.locator("#h5-transfer-target")
    expect(target).to_be_visible()
    options = [item for item in target.locator("option").all_text_contents() if "转交" in item]
    assert len(options) == 1, "expected one synthetic delegate in transfer candidates"
    target.select_option(label=options[0].strip())
    manager.locator("#h5-action-comment").fill("浏览器模拟转交")
    unobstructed(button(manager, "确认转交"))
    with manager.expect_response(lambda response: response.url.endswith("/actions")) as info:
        button(manager, "确认转交").click()
    assert info.value.ok, f"H5 transfer failed with {info.value.status}"
    expect(manager.get_by_role("heading", name="处理待办")).to_have_count(0)
    tab(manager, "已办").click()
    open_record(manager, title)
    expect(manager.get_by_text("浏览器模拟转交", exact=True)).to_be_visible()
    assert_no_persistent_storage(manager)

    login(delegate, base, "delegate")
    tab(delegate, "待办").click()
    open_record(delegate, title)
    approve(delegate)
    assert_no_persistent_storage(delegate)

    login(finance, base, "finance")
    tab(finance, "待办").click()
    open_record(finance, title)
    approve(finance)
    no_overflow(finance)
    assert_no_persistent_storage(finance)

    button(employee, "返回列表").click()
    expect(employee.get_by_role("heading", name="我的申请")).to_be_visible()
    open_record(employee, title)
    tab(employee, "我的申请").click()
    expect(employee.get_by_role("heading", name="我的申请")).to_be_visible()
    open_record(employee, title)
    expect(employee.get_by_role("region", name=title).get_by_text("已通过", exact=True)).to_be_visible()
    print("PASS H5 current tab and explicit back both return to the request list", flush=True)
    expect(employee.get_by_text(content, exact=True)).to_be_visible()
    employee.screenshot(path=artifacts / "employee-approved-390.png", full_page=True, animations="disabled")
    print("PASS H5 manager transfer, old assignee history, delegate approval, finance approval and applicant readback",
          flush=True)

    with employee.expect_response(lambda reply: "/notifications?" in reply.url and reply.ok) as notifications:
        tab(employee, "站内通知").click()
    data = notifications.value.json()["data"]
    for _ in range(15):
        matches = [(index, row) for index, row in enumerate(data["list"]) if row["requestId"] == identifier]
        if matches:
            break
        with employee.expect_response(lambda reply: "/notifications?" in reply.url and reply.ok) as refreshed:
            tab(employee, "我的申请").click()
            tab(employee, "站内通知").click()
        data = refreshed.value.json()["data"]
        employee.wait_for_load_state("networkidle")
    assert len(matches) == 1
    index, row = matches[0]
    assert row["kind"] == "REQUEST_APPROVED" and row["deliveryMode"] == "mock"
    assert title not in row["title"] and content not in row["title"]
    employee.locator(".h5-record-button").nth(index).click()
    expect(employee.get_by_role("heading", name=title, exact=True)).to_be_visible()
    assert_no_persistent_storage(employee)
    employee.reload(wait_until="networkidle")
    expect(employee.get_by_role("heading", name="登录移动审批")).to_be_visible()
    print("PASS H5 mock notification opens authorized detail; browser storage empty and refresh reauthenticates",
          flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/m2/browser"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            check_pc(browser, args.base, args.artifacts)
            check(browser, args.base, args.artifacts)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
