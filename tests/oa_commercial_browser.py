"""Commercial OA browser checks on the real combined Pure and H5 deployment."""
import argparse
import json
import os
from pathlib import Path
import re
from urllib.parse import unquote

from playwright.sync_api import Error, expect, sync_playwright

from oa_api import local_base
from oa_browser import button
from oa_commercial_api import EXPORT, workbook_rows
from oa_m2_api import call, general, intent, login as api_login
from oa_m2_browser import login as h5_login, tab
from oa_pure_browser import Traffic, check_storage, login, no_overflow

SEARCH = "**/api/oa/v1/employees/search"
EXPORT_PATH = "/api/oa/v1/employees/export.xlsx"


def search_request(response):
    return response.request.method == "POST" and response.url.endswith("/employees/search")


def employee_table(page):
    return page.get_by_role("table", name="员工列表")


def check_search(page, state, traffic):
    tag = state["query"]
    traffic.sensitive.update({tag, state["name"], *state["rows"]})
    query = page.get_by_role("searchbox")
    search = button(page, "搜索")
    table = employee_table(page)
    expect(table).to_be_visible()
    initial = table.locator("tbody tr").count()
    if initial == 20:
        with page.expect_response(search_request) as paged:
            button(page, "下一页").click()
        assert paged.value.request.post_data_json["page"] == 2

    query.fill(tag)
    with page.expect_response(search_request) as applied:
        search.click()
    assert applied.value.request.post_data_json["page"] == 1
    expect(table.locator("tbody tr")).to_have_count(4)
    page.get_by_label("部门筛选").select_option(str(state["department"]["id"]))
    with page.expect_response(search_request) as department:
        search.click()
    assert department.value.json()["data"]["total"] == 3
    expect(table.locator("tbody tr")).to_have_count(3)
    expect(table.get_by_role("columnheader", name="约定工资")).to_have_count(0)

    for size in (50, 100, 20):
        with page.expect_response(search_request) as resized:
            page.get_by_label("每页条数").select_option(str(size))
        payload = resized.value.request.post_data_json
        assert (payload["query"], payload["departmentId"], payload["page"], payload["pageSize"]) == (
            tag, state["department"]["id"], 1, size,
        )
        expect(table.locator("tbody tr")).to_have_count(3)
    query.fill("未应用 " + tag)
    with page.expect_download() as file, page.expect_response(
        lambda response: response.url.endswith(EXPORT_PATH)
    ) as downloaded:
        page.get_by_role("button", name="按当前筛选导出").click()
    request = downloaded.value.request
    assert request.method == "POST"
    assert request.post_data_json == {"query": tag, "departmentId": state["department"]["id"]}
    rows = workbook_rows(file.value.path().read_bytes())
    assert rows[0] == EXPORT and [row[0] for row in rows[1:]] == state["rows"]

    # Delay a real response while submitting a different intent. The earlier
    # response must be canceled or ignored even if it is delivered last.
    held = []

    def delay(route):
        if route.request.post_data_json.get("query") == state["rows"][0]:
            held.append(route)
        else:
            route.continue_()

    page.route(SEARCH, delay)
    try:
        query.fill(state["rows"][0])
        search.click()
        query.fill(state["rows"][1])
        with page.expect_response(lambda response: search_request(response)
                                  and response.request.post_data_json["query"] == state["rows"][1]):
            search.click()
        assert len(held) == 1
        try:
            held[0].continue_()
        except Error:
            pass  # A canceled request may be invalidated by Chromium.
        expect(table.locator("tbody tr")).to_have_count(1)
        expect(table.get_by_text(state["rows"][1], exact=True)).to_be_visible()
        page.wait_for_timeout(200)
        expect(table.get_by_text(state["rows"][0], exact=True)).to_have_count(0)
    finally:
        page.unroute(SEARCH, delay)

    query.fill("没有匹配的测试员工 " + tag)
    with page.expect_response(search_request) as no_matches:
        search.click()
    assert no_matches.value.json()["data"]["total"] == 0
    with page.expect_response(search_request) as cleared:
        button(page, "清空").click()
    assert cleared.value.request.post_data_json["query"] == ""
    assert cleared.value.request.post_data_json.get("departmentId") is None
    expect(table).to_be_visible()
    assert tag not in unquote(page.url)
    check_storage(page, traffic.sensitive)
    print("PASS Pure filters, page size, export uses applied filters, slow prior response and private search state")


def check_department(page, state):
    sidebar = page.locator(".sidebar-container")
    sidebar.get_by_text("部门管理", exact=True).click()
    expect(page.get_by_role("heading", name="部门管理")).to_be_visible()
    button(page, "新增部门").click()
    dirty = state["query"] + " 未保存草稿"
    editor = page.get_by_label("部门名称")
    editor.fill(dirty)
    start = page.url
    for control in (sidebar, page.locator(".tags-view")):
        target = control.get_by_text("员工档案", exact=True)
        if target.is_enabled():
            target.click()
        expect(editor).to_have_value(dirty)
        assert page.url == start
    refresh = button(page, "刷新")
    if refresh.is_enabled():
        refresh.click()
    expect(editor).to_have_value(dirty)
    expect(sidebar.locator(".el-menu-item.is-active")).to_have_text("部门管理")
    button(page, "取消").click()
    sidebar.get_by_text("员工档案", exact=True).click()
    expect(employee_table(page)).to_be_visible()
    print("PASS department unsaved draft survives sidebar, tabs and list refresh; cancel unlocks navigation")


def contrast(scope, selectors):
    return scope.evaluate(r"""(scope, selectors) => {
        const canvas = document.createElement('canvas');
        canvas.width = canvas.height = 1;
        const context = canvas.getContext('2d', {willReadFrequently: true});
        function lightness(color) {
            context.clearRect(0, 0, 1, 1);
            context.fillStyle = color;
            context.fillRect(0, 0, 1, 1);
            const channels = Array.from(context.getImageData(0, 0, 1, 1).data).slice(0, 3);
            const rgb = channels.map(channel => {
                const value = channel / 255;
                return value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4;
            });
            return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722;
        }
        return Object.fromEntries(Object.entries(selectors).map(([label, selector]) => {
            const text = scope.querySelector(selector);
            let surface = text;
            while (getComputedStyle(surface).backgroundColor === 'rgba(0, 0, 0, 0)') surface = surface.parentElement;
            const foreground = lightness(getComputedStyle(text).color);
            const background = lightness(getComputedStyle(surface).backgroundColor);
            return [label, (Math.max(foreground, background) + .05) / (Math.min(foreground, background) + .05)];
        }));
    }""", selectors)


def theme_shots(page, artifacts, width):
    no_overflow(page)
    panel = page.locator(".oa-table-panel")
    light = panel.evaluate("node => getComputedStyle(node).backgroundColor")
    table = employee_table(page)
    selectors = {"number": "tbody .oa-employee-number", "action": "tbody button"}
    if width == 1440:
        selectors["heading"] = "thead th"
    light_ratios = contrast(table, selectors)
    assert min(light_ratios.values()) >= 4.5, f"light employee-table contrast below 4.5:1: {light_ratios}"
    separator = table.locator("tbody td" if width == 1440 else "tbody tr").first
    light_border = separator.evaluate("node => getComputedStyle(node).borderBottomColor")
    page.screenshot(path=artifacts / f"pc-light-{width}.png", animations="disabled")
    button(page, "界面设置").click()
    page.locator(".right-panel").get_by_text("深色", exact=True).click()
    page.locator(".right-panel-background").click(position={"x": 5, "y": 5})
    expect(page.locator("html")).to_have_class(re.compile(r"\bdark\b"))
    no_overflow(page)
    assert panel.evaluate("node => getComputedStyle(node).backgroundColor") != light
    page.screenshot(path=artifacts / f"pc-dark-{width}.png", animations="disabled")
    table_ratios = contrast(table, selectors)
    nav_ratios = contrast(page.locator(".navbar"), {"mobile_link": ".oa-navbar-link"})
    nav_ratios.update(contrast(page.locator(".tags-view"), {"active_tab": ".scroll-item.is-active .tag-title"}))
    assert min(nav_ratios.values()) >= 4.5, f"dark navigation contrast below 4.5:1: {nav_ratios}"
    dark_border = separator.evaluate("node => getComputedStyle(node).borderBottomColor")

    page.evaluate("window.location.hash = '/mobile-approvals'")
    card = page.get_by_role("region", name="移动审批入口")
    expect(card).to_be_visible()
    expect(card.get_by_role("link", name="打开移动审批")).to_be_visible()
    entry_ratios = contrast(card, {"heading": "h1", "body": "p"})
    assert min(entry_ratios.values()) >= 4.5, f"dark mobile-entry contrast below 4.5:1: {entry_ratios}"
    no_overflow(page)
    page.screenshot(path=artifacts / f"pc-dark-mobile-entry-{width}.png", animations="disabled")
    page.evaluate("window.location.hash = '/employees'")
    expect(employee_table(page)).to_be_visible()
    assert min(table_ratios.values()) >= 4.5, f"dark employee-table contrast below 4.5:1: {table_ratios}"
    assert dark_border != light_border, "list row separator did not follow theme"
    print(f"PASS Pure {width}px; minimum list light {min(light_ratios.values()):.2f}:1, "
          f"dark {min(table_ratios.values()):.2f}:1, dark entry {min(entry_ratios.values()):.2f}:1, "
          f"dark navigation {min(nav_ratios.values()):.2f}:1")


def check_h5_refresh(browser, base, artifacts):
    page = browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
    h5_login(page, base, "employee")
    tab(page, "我的申请").click()
    expect(page.get_by_role("heading", name="我的申请")).to_be_visible()
    page.wait_for_load_state("networkidle")
    fresh = general("商业 QA 列表刷新")
    token = api_login(base, "employee", "EMPLOYEE")["token"]
    call(base, "/requests", token, payload=fresh, key=intent(), status=201)
    new_record = page.get_by_role("button", name=re.compile(re.escape(fresh["title"])))
    expect(new_record).to_have_count(0)
    with page.expect_response(lambda response: "/api/oa/v1/requests?" in response.url and response.ok):
        button(page, "刷新").click()
    expect(page.get_by_role("heading", name="我的申请")).to_be_visible()
    expect(new_record).to_be_visible()
    no_overflow(page)
    page.screenshot(path=artifacts / "h5-list-refresh-390.png", animations="disabled")
    print("PASS H5 refresh rereads the current list without reloading the session")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/commercial/state.json"))
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/commercial/browser"))
    args = parser.parse_args()
    assert os.environ.get("OA_SEED_PASSWORD")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    state = json.loads(args.state.read_text())
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            context = browser.new_context(viewport={"width": 1440, "height": 960})
            traffic = Traffic(context, args.base)
            page = context.new_page()
            page.goto(args.base + "/oa.html", wait_until="networkidle")
            login(page, "hr_viewer", traffic)
            check_search(page, state, traffic)
            theme_shots(page, args.artifacts, 1440)
            page.get_by_role("searchbox").fill(state["query"])
            with page.expect_response(search_request):
                button(page, "搜索").click()
            button(page, "退出登录").click()
            login(page, "hr_admin", traffic)
            expect(page.get_by_role("searchbox")).to_have_value("")
            check_department(page, state)
            traffic.assert_local()

            mobile = browser.new_context(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
            mobile_traffic = Traffic(mobile, args.base)
            phone = mobile.new_page()
            phone.goto(args.base + "/oa.html", wait_until="networkidle")
            login(phone, "employee", mobile_traffic)
            expect(employee_table(phone)).to_be_visible()
            theme_shots(phone, args.artifacts, 390)
            mobile_traffic.assert_local()
            check_h5_refresh(browser, args.base, args.artifacts)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
