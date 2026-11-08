"""Real M4 browser checks for PC/H5 approval and Pure personnel modules."""
import argparse
import json
import os
from pathlib import Path
import re
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_browser import button
from oa_m2_browser import open_record
from oa_commercial_browser import contrast
from oa_m2_api import general, request_detail
from oa_m2_browser import login as h5_login, tab
from oa_m3_api import call
from oa_pure_browser import Traffic, check_storage, login, no_overflow


def pc_page(browser, base, username, width=1440):
    context = browser.new_context(viewport={"width": width, "height": 960 if width == 1440 else 844},
                                  is_mobile=width == 390, has_touch=width == 390)
    traffic = Traffic(context, base)
    page = context.new_page()
    page.goto(base + "/oa.html", wait_until="networkidle")
    session = login(page, username, traffic)
    return page, traffic, session


def menu(page, title):
    page.locator(".sidebar-container").get_by_text(title, exact=True).click()
    expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()


def screenshot(page, artifacts, name):
    no_overflow(page)
    page.screenshot(path=artifacts / name, animations="disabled")


def directory(page):
    return page.get_by_role("region", name="员工名册")


def employee_landing(page):
    page.evaluate("window.location.hash = '/employees'")
    expect(directory(page)).to_be_visible()


def set_theme(page, dark=True):
    button(page, "界面设置").click()
    page.locator(".right-panel").get_by_text("深色" if dark else "浅色", exact=True).click()
    page.locator(".right-panel-background").click(position={"x": 5, "y": 5})
    if dark:
        expect(page.locator("html")).to_have_class(re.compile(r"\bdark\b"))
    else:
        expect(page.locator("html")).not_to_have_class(re.compile(r"\bdark\b"))


def open_employee(page, number):
    panel = directory(page)
    row = panel.get_by_role("row").filter(has_text=number)
    next_page = button(panel, "下一页")
    while not row.count() and next_page.count() and next_page.is_enabled():
        with page.expect_response(lambda response: response.url.endswith("/employees/search") and response.ok):
            next_page.click()
        page.wait_for_load_state("networkidle")
    expect(row).to_have_count(1)
    button(row, "查看档案").click()
    expect(page.locator("#employee-heading")).to_be_visible()


def theme_shots(page, artifacts, width):
    panel = directory(page)
    expect(panel).to_be_visible()
    selectors = {"number": ".oa-employee-number", "action": ".oa-table-action button"}
    if width == 1440:
        selectors["heading"] = ".oa-directory-pure-table thead th .cell"
        selectors["action"] = ".oa-directory-pure-table tbody button"
    light = contrast(panel, selectors)
    screenshot(page, artifacts, f"m4-light-{width}.png")
    set_theme(page)
    dark = contrast(panel, selectors)
    screenshot(page, artifacts, f"m4-dark-{width}.png")
    print(f"M4 {width}px employee list contrast light {light}, dark {dark}", flush=True)
    return min(light.values(), default=0), min(dark.values(), default=0)

def cross_end(browser, base, artifacts):
    applicant, traffic, session = pc_page(browser, base, "employee")
    manager, manager_traffic, _ = pc_page(browser, base, "manager")
    mobile = browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
    h5_login(mobile, base, "manager")
    draft = general("M4 跨端审批")
    title = draft["title"]
    traffic.sensitive.update({title, draft["form"]["content"]})
    manager_traffic.sensitive.update({title, draft["form"]["content"]})
    menu(applicant, "审批中心")
    button(applicant, "发起申请").click()
    applicant.get_by_role("list", name="申请类型").get_by_role("button", name=re.compile("通用申请")).click()
    applicant.get_by_label("标题", exact=True).fill(title)
    applicant.get_by_label("申请内容", exact=True).fill(draft["form"]["content"])
    with applicant.expect_response(lambda response: response.url.endswith("/requests") and response.request.method == "POST") as sent:
        button(applicant, "提交申请").click()
    assert sent.value.status == 201
    created = sent.value.json()["data"]
    expect(applicant.get_by_role("heading", name=title, exact=True)).to_be_visible()
    menu(manager, "审批中心")
    views = manager.get_by_role("navigation", name="审批视图")
    button(views, "已办").focus()
    manager.keyboard.press("Space")
    expect(manager.get_by_role("heading", name="已办", exact=True)).to_be_visible()
    button(views, "待办").focus()
    manager.keyboard.press("Enter")
    expect(manager.get_by_role("heading", name="待办", exact=True)).to_be_visible()
    disclosure = manager.locator("details.pc-filter-disclosure > summary")
    if disclosure.count():
        disclosure.click()
    manager.get_by_label("申请类型").select_option("GENERAL")
    manager.get_by_label("申请状态").select_option("IN_REVIEW")
    with manager.expect_response(lambda response: "/tasks?" in response.url and "type=GENERAL" in response.url) as filtered:
        button(manager, "筛选").click()
    assert filtered.value.ok
    row = manager.get_by_role("button", name=re.compile(re.escape(title)))
    expect(row).to_be_visible()
    row.click()
    expect(manager.get_by_role("heading", name=title, exact=True)).to_be_visible()
    screenshot(manager, artifacts, "pc-approvals-1440.png")
    tab(mobile, "待办").click()
    open_record(mobile, title)
    screenshot(mobile, artifacts, "h5-shared-task-390.png")

    task = created["tasks"][0]
    pattern = f"**/tasks/{task['id']}/actions"
    held = []
    results = []

    def queue(route):
        held.append(route)

    def record(response):
        if response.url.endswith(f"/tasks/{task['id']}/actions"):
            results.append(response.status)

    for page in (manager, mobile):
        page.on("response", record)
        page.route(pattern, queue)
        button(page, "通过").click()
        button(page, "确认通过").click()
    for page in (manager, mobile):
        expect(page.get_by_role("region", name="处理待办").locator('button[type="submit"]')).to_be_disabled()
    assert len(held) == 2, "PC and H5 did not both submit the same task"
    assert len({route.request.header_value("idempotency-key") for route in held}) == 2
    assert all(route.request.post_data_json["version"] == task["version"] for route in held)
    for route in held:
        route.continue_()
    for _ in range(60):
        if len(results) == 2:
            break
        manager.wait_for_timeout(100)
    assert sorted(results) == [200, 409], results
    saved = request_detail(base, created["id"], session["token"])
    assert [item["action"] for item in saved["history"]] == ["SUBMIT", "APPROVE"]
    assert len(saved["tasks"]) == 2 and saved["currentStep"] == 2
    expect(row).to_have_count(0)
    for page in (manager, mobile):
        page.unroute(pattern, queue)
        expect(page.get_by_role("heading", name="处理待办")).to_have_count(0)
    check_storage(applicant, traffic.sensitive)
    check_storage(manager, manager_traffic.sensitive)
    traffic.assert_local()
    manager_traffic.assert_local()
    phone, phone_traffic, _ = pc_page(browser, base, "employee", 390)
    set_theme(phone)
    phone.evaluate("window.location.hash = '/approvals'")
    expect(phone.get_by_role("heading", name="审批中心", exact=True)).to_be_visible()
    phone.get_by_role("button", name=re.compile(re.escape(title))).click()
    expect(phone.get_by_role("heading", name=title, exact=True)).to_be_visible()
    expect(button(phone, "返回列表")).to_be_visible()
    screenshot(phone, artifacts, "pc-approval-dark-390.png")
    set_theme(phone, dark=False)
    screenshot(phone, artifacts, "pc-approval-light-390.png")
    button(phone, "返回列表").click()
    expect(phone.get_by_role("button", name=re.compile(re.escape(title)))).to_be_visible()
    phone_traffic.assert_local()
    print("PASS M4 PC creates and filters; PC/H5 two simultaneous task actions yield one decision and one stale response", flush=True)


def organization(browser, base, state, artifacts):
    page, traffic, _ = pc_page(browser, base, "hr_admin")
    employee_landing(page)
    table = directory(page)
    expect(table).to_be_visible()
    expect(table.get_by_role("columnheader", name="约定工资")).to_be_visible()
    column_control = page.get_by_role("button", name=re.compile("列设置|列显示"))
    column_control.click()
    salary_checkbox = page.get_by_role("checkbox", name="约定工资")
    expect(salary_checkbox).to_have_count(1)
    salary_checkbox.locator("xpath=ancestor::label").click()
    expect(table.get_by_role("columnheader", name="约定工资")).to_have_count(0)
    salary_checkbox.locator("xpath=ancestor::label").click()
    expect(table.get_by_role("columnheader", name="约定工资")).to_be_visible()
    page.get_by_role("heading", name="员工档案", exact=True).click()
    button(page, "退出登录").click()
    login(page, "hr_viewer", traffic)
    employee_landing(page)
    expect(table).to_be_visible()
    column_control.click()
    expect(page.get_by_role("checkbox", name="列展示")).to_have_count(1)
    expect(page.get_by_role("checkbox", name="约定工资")).to_have_count(0)
    expect(table.get_by_role("columnheader", name="约定工资")).to_have_count(0)
    page.get_by_role("heading", name="员工档案", exact=True).click()
    page.get_by_role("button", name=re.compile("表格密度|密度")).click()
    page.get_by_text("紧凑", exact=True).click()
    expect(table.locator("[data-density]")).to_have_attribute("data-density", "small")
    page.get_by_role("button", name="全屏", exact=True).click()
    expect(page.locator(".oa-directory-fullscreen")).to_be_visible()
    page.get_by_role("button", name="退出全屏").click()
    expect(page.locator(".oa-directory-fullscreen")).to_have_count(0)

    jump = page.get_by_label("页码", exact=True)
    expect(jump).to_be_visible()
    jump.fill("2")
    with page.expect_response(lambda response: response.url.endswith("/employees/search") and
                              response.request.post_data_json.get("page") == 2):
        button(page, "跳转").click()
    parent = page.locator(".el-tree-node").filter(has=page.get_by_text(state["parent"]["name"], exact=True)).last
    if parent.get_attribute("aria-expanded") != "true":
        parent.locator(".el-tree-node__expand-icon").first.click()
    node = page.get_by_role("treeitem").filter(has_text=state["department"]["name"]).last
    expect(node).to_be_visible()
    with page.expect_response(lambda response: response.url.endswith("/employees/search") and
                              response.request.post_data_json.get("departmentId") == state["department"]["id"]):
        node.get_by_text(state["department"]["name"], exact=True).click()
    expect(button(table, "查看档案")).to_have_count(1)
    expect(node).to_contain_text("1 人")
    screenshot(page, artifacts, "organization-tree-table-1440.png")
    trigger = button(table, "查看档案")
    page.emulate_media(reduced_motion="reduce")
    trigger.click()
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible()
    expect(page.locator("#employee-heading")).to_be_visible()
    assert dialog.evaluate("""node => matchMedia('(prefers-reduced-motion: reduce)').matches &&
        node.getAnimations({subtree: true}).every(animation => {
            const timing = animation.effect.getComputedTiming()
            const transforms = animation.effect.getKeyframes().map(frame => frame.transform).filter(Boolean)
            return timing.iterations !== Infinity && new Set(transforms).size <= 1
        })""")
    page.keyboard.press("Escape")
    expect(dialog).to_be_hidden()
    expect(trigger).to_be_focused()
    expect(button(table, "查看档案")).to_have_count(1)
    expect(page.get_by_label("部门筛选")).to_have_value(str(state["department"]["id"]))
    button(page, "清空").click()
    expect(button(table, "查看档案").first).to_be_visible()
    desktop = theme_shots(page, artifacts, 1440)
    check_storage(page, traffic.sensitive)
    traffic.assert_local()

    phone, phone_traffic, _ = pc_page(browser, base, "employee", 390)
    employee_landing(phone)
    expect(button(directory(phone), "查看档案")).to_have_count(1)
    expect(phone.get_by_role("heading", name="组织架构")).to_have_count(0)
    mobile = theme_shots(phone, artifacts, 390)
    phone_traffic.assert_local()
    print("PASS M4 organization direct counts, role-safe columns, table density/fullscreen/jump and filter retention", flush=True)
    assert min(*desktop, *mobile) >= 4.5, f"M4 contrast below 4.5:1: desktop {desktop}, mobile {mobile}"


def lifecycle(browser, base, state, artifacts):
    page, traffic, admin = pc_page(browser, base, "hr_admin")
    employee_landing(page)
    expect(button(directory(page), "查看档案").first).to_be_visible()
    record = call(base, f"/employees/{state['employeeId']}", admin["token"])
    open_employee(page, record["employeeNo"])
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible()
    button(dialog, "编辑员工").click()
    draft = dialog.locator("#employee-name")
    draft.fill("M4 模拟未保存档案")
    expect(button(dialog.get_by_role("navigation", name="员工详情分组"), "员工账号")).to_be_disabled()
    page.keyboard.press("Escape")
    expect(dialog).to_be_visible()
    expect(draft).to_have_value("M4 模拟未保存档案")
    button(dialog, "取消").click()
    contract = call(base, f"/contracts/{state['contract']['id']}", admin["token"])
    article = dialog.get_by_role("article").filter(has=page.get_by_text(contract["contractNo"], exact=True))
    contract_pages = page.get_by_role("navigation", name=re.compile("合同.*分页"))
    while not article.count() and contract_pages.count() and button(contract_pages, "下一页").is_enabled():
        with page.expect_response(lambda response: "/contracts?" in response.url and response.ok):
            button(contract_pages, "下一页").click()
        page.wait_for_load_state("networkidle")
    expect(article).to_be_visible()
    expect(article.get_by_text("已终止", exact=True)).to_be_visible()
    expect(button(article, "预览 m4-fixture.pdf")).to_be_visible()
    button(article, "查看合同详情").click()
    contract_history = dialog.get_by_role("region", name="合同状态与版本历史")
    expect(contract_history.get_by_text("终止 · 合同", exact=True)).to_be_visible()
    dialog.get_by_role("heading", name=contract["contractNo"], exact=True).evaluate(
        "node => { node.style.scrollMarginTop = '6rem'; node.scrollIntoView({block:'start'}); }")
    screenshot(page, artifacts, "contract-lifecycle-1440.png")

    groups = dialog.get_by_role("navigation", name="员工详情分组")
    button(groups, "员工账号").focus()
    page.keyboard.press("Enter")
    account = page.get_by_role("region", name="员工账号")
    expect(account).to_be_visible()
    button(account, "停用登录").click()
    account.get_by_label("操作原因").fill("M4 模拟浏览器停用")
    with page.expect_response(lambda response: response.url.endswith("/account/actions")) as disabled:
        button(account, "确认停用登录").click()
    assert disabled.value.ok and not disabled.value.json()["data"]["account"]["enabled"]
    expect(account.get_by_text("已停用", exact=True)).to_be_visible()
    account.scroll_into_view_if_needed()
    screenshot(page, artifacts, "employee-account-1440.png")
    button(account, "启用登录").click()
    account.get_by_label("操作原因").fill("M4 模拟浏览器启用")
    with page.expect_response(lambda response: response.url.endswith("/account/actions")) as enabled:
        button(account, "确认启用登录").click()
    assert enabled.value.ok and enabled.value.json()["data"]["account"]["enabled"]
    button(dialog, "编辑员工").click()
    updated_name = record["name"].removesuffix("· 浏览器复核")
    if updated_name == record["name"]:
        updated_name += "· 浏览器复核"
    draft.fill(updated_name)
    with page.expect_response(lambda response: response.url.endswith(f"/employees/{record['id']}") and
                              response.request.method == "PUT") as saved:
        button(dialog, "保存员工").click()
    assert saved.value.ok, f"edit after account action failed: {saved.value.status}"
    expect(dialog.locator("#employee-heading")).to_have_text(updated_name)

    history = page.get_by_role("region", name="变更历史", exact=True)
    button(groups, "变更历史").focus()
    page.keyboard.press("Space")
    expect(history).to_be_visible()
    history.get_by_label("变更类型").select_option("SALARY")
    expect(history.locator(".oa-audit-timeline").get_by_text(re.compile("约定工资")).first).to_be_visible()
    history.scroll_into_view_if_needed()
    screenshot(page, artifacts, "audit-history-1440.png")
    page.keyboard.press("Escape")
    expect(dialog).to_be_hidden()
    button(page, "退出登录").click()
    login(page, "hr_viewer", traffic)
    employee_landing(page)
    expect(button(directory(page), "查看档案").first).to_be_visible()
    open_employee(page, record["employeeNo"])
    expect(account).to_have_count(0)
    expect(button(groups, "员工账号")).to_have_count(0)
    button(groups, "变更历史").click()
    expect(history).to_be_visible()
    assert "SALARY" not in history.get_by_label("变更类型").locator("option").evaluate_all("(options) => options.map(option => option.value)")
    expect(history.get_by_text(re.compile("约定工资"))).to_have_count(0)
    page.keyboard.press("Escape")
    expect(dialog).to_be_hidden()
    menu(page, "合同到期提醒")
    due = state["contract"]["due"]
    reminder = page.get_by_role("row").filter(has_text=due["contractNo"])
    expect(reminder).to_have_count(1)
    with page.expect_response(lambda response: response.url.endswith(f"/contracts/{due['id']}") and response.ok) as reply:
        button(reminder, "查看合同").click()
    contract = reply.value.json()["data"]
    assert (contract["id"], contract["employeeId"], contract["contractNo"]) == (
        due["id"], state["employeeId"], due["contractNo"])
    assert urlsplit(page.url).fragment == "/contract-reminders"
    expect(dialog.get_by_text(due["contractNo"], exact=True).first).to_be_visible()
    expect(button(dialog, "终止合同")).to_have_count(0)
    screenshot(page, artifacts, "contract-reminder-direct-1440.png")
    check_storage(page, traffic.sensitive)
    traffic.assert_local()
    phone, phone_traffic, _ = pc_page(browser, base, "hr_admin", 390)
    set_theme(phone)
    employee_landing(phone)
    open_employee(phone, record["employeeNo"])
    phone_dialog = phone.get_by_role("dialog")
    drawer_ratios = contrast(phone_dialog, {"field_label": ".oa-description dt", "muted": ".oa-profile-head .oa-muted",
                                           "back_caption": ".oa-detail-back > span"})
    print(f"M4 390px dark drawer contrast {drawer_ratios}", flush=True)
    assert min(drawer_ratios.values()) >= 4.5, f"M4 dark drawer contrast {drawer_ratios}"
    phone_groups = phone_dialog.get_by_role("navigation", name="员工详情分组")
    for title, shot in (("员工账号", "employee-drawer"), ("劳动合同", "contract-drawer"), ("变更历史", "audit-drawer")):
        button(phone_groups, title).click()
        panel = phone_dialog.get_by_role("region", name=title, exact=True)
        expect(panel).to_be_visible()
        assert phone_dialog.evaluate("node => node.scrollWidth <= node.clientWidth + 1"), title
        panel.get_by_role("heading", name=title, exact=True).scroll_into_view_if_needed()
        if title == "员工账号":
            expect(button(panel, "停用登录")).to_be_enabled()
        screenshot(phone, artifacts, f"{shot}-dark-390.png")
    phone_traffic.assert_local()
    print("PASS M4 browser contract state and attachment, account disable/enable, authorized audit and salary hiding", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/m4/api-state.json"))
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/m4/browser"))
    parser.add_argument("--section", choices=("all", "approvals", "organization", "lifecycle"), default="all")
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    state = json.loads(args.state.read_text())
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            if args.section in ("all", "approvals"):
                cross_end(browser, args.base, args.artifacts)
            if args.section in ("all", "organization"):
                organization(browser, args.base, state, args.artifacts)
            if args.section in ("all", "lifecycle"):
                lifecycle(browser, args.base, state, args.artifacts)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
