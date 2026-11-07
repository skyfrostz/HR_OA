"""Real Pure Admin M3 browser flows against the development MySQL stack."""
import argparse
import json
import os
from pathlib import Path
import re

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_browser import button, open_employee
from oa_m3_api import EXPORT, HEADERS, XLSX, fill_workbook, workbook_rows
from oa_pure_browser import Traffic, check_storage, login, no_overflow


def go_menu(page, label):
    page.locator(".sidebar-container").get_by_text(label, exact=True).click()
    expect(page.get_by_role("heading", name=label, exact=True)).to_be_visible()


def download(page, control):
    with page.expect_download() as download:
        control.click()
    file = download.value
    assert file.suggested_filename.lower().endswith(".xlsx")
    return file.path().read_bytes()


def upload_preview(page, data):
    page.locator("#oa-import-file").set_input_files({"name": "模拟导入.xlsx", "mimeType": XLSX, "buffer": data})
    with page.expect_response(lambda result: result.url.endswith("/employee-imports/preview")) as preview:
        button(page, "预览导入").click()
    assert preview.value.status == 201
    expect(page.get_by_role("heading", name="预览结果")).to_be_visible()
    return preview.value.json()["data"]


def create_department(page, department):
    go_menu(page, "部门管理")
    button(page, "新增部门").click()
    page.get_by_label("部门名称").fill(department)
    with page.expect_response(lambda result: result.url.endswith("/departments")
                              and result.request.method == "POST") as response:
        button(page, "保存部门").click()
    assert response.value.status == 201
    assert response.value.json()["data"]["name"] == department
    expect(page.get_by_role("heading", name="部门名录")).to_be_visible()


def confirm_uncertain_import(page, preview):
    confirmation_url = f"**/api/oa/v1/employee-imports/{preview['id']}/confirm"
    results = []

    def lose_response(route):
        response = route.fetch(max_retries=0)
        results.append(response.status)
        response.dispose()
        route.abort()

    page.route(confirmation_url, lose_response)
    try:
        button(page, "确认导入").click()
        expect(button(page, "重试同一预览")).to_be_visible()
        assert results == [200]
        current = page.url
        page.locator(".sidebar-container").get_by_text("员工档案", exact=True).click()
        expect(button(page, "重试同一预览")).to_be_visible()
        assert page.url == current, "Pure menu discarded an unconfirmed import result"
        expect(page.locator(".sidebar-container .el-menu-item.is-active")).to_have_text("Excel 导入")
        page.locator(".tags-view").get_by_text("员工档案", exact=True).click()
        expect(button(page, "重试同一预览")).to_be_visible()
        assert page.url == current, "Pure tab discarded an unconfirmed import result"
    finally:
        page.unroute(confirmation_url, lose_response)
    with page.expect_response(lambda response: response.url.endswith(f"/{preview['id']}/confirm")) as retried:
        button(page, "重试同一预览").click()
    assert retried.value.status == 200
    assert retried.value.json()["data"]["importedCount"] == 1
    expect(page.get_by_text("已导入 1 名员工。", exact=True)).to_be_visible()
    return retried.value.json()["data"]["employeeIds"][0]


def main_check(page, traffic, base, state, artifacts):
    page.goto(base + "/oa.html", wait_until="networkidle")
    login(page, "hr_admin", traffic)
    suffix = os.urandom(4).hex().upper()
    number = "MB" + suffix
    name = "模拟网页导入员工 " + suffix
    changed = "模拟网页编辑员工 " + suffix
    department = "模拟网页部门" + suffix
    traffic.sensitive.update({number, name, changed, department, "¥654,321.08", "65432108"})
    create_department(page, department)
    page.screenshot(path=artifacts / "m3-departments-1440.png", animations="disabled")

    go_menu(page, "Excel 导入")
    template = download(page, button(page, "下载导入模板"))
    assert workbook_rows(template) == [HEADERS]
    row = (number, name, department, "1991-01-01", "2022-01-01", "654321.08")
    wrong_department = row[:2] + ("不存在的模拟部门" + suffix,) + row[3:]
    invalid = upload_preview(page, fill_workbook(template, [wrong_department]))
    assert invalid["errorRows"] == 1 and not invalid["canConfirm"]
    expect(button(page, "确认导入")).to_be_disabled()
    page.screenshot(path=artifacts / "m3-import-errors-1440.png", animations="disabled")
    valid = upload_preview(page, fill_workbook(template, [row]))
    assert valid["canConfirm"] and valid["validRows"] == 1
    employee_id = confirm_uncertain_import(page, valid)

    go_menu(page, "员工档案")
    open_employee(page, number)
    expect(page.get_by_role("heading", name=name)).to_be_visible()
    button(page, "编辑员工").click()
    page.locator("#employee-name").fill(changed)
    with page.expect_response(lambda response: response.url.endswith(f"/employees/{employee_id}")
                              and response.request.method == "PUT") as edited:
        button(page, "保存员工").click()
    assert edited.value.status == 200 and edited.value.json()["data"]["name"] == changed
    expect(page.get_by_role("heading", name=changed)).to_be_visible()

    annual = page.get_by_role("region", name="年假记录")
    button(annual, "新增年假记录").click()
    annual.get_by_label("年份").fill("2026")
    annual.get_by_label("应休（天）").fill("7.5")
    annual.get_by_label("已休（天）").fill("2")
    with page.expect_response(lambda response: response.url.endswith(f"/employees/{employee_id}/annual-leave/2026")
                              and response.request.method == "PUT") as saved:
        button(annual, "保存年假记录").click()
    assert saved.value.status == 200 and saved.value.json()["data"]["remainingHalfDays"] == 11
    expect(annual.get_by_text("5.5 天", exact=True)).to_be_visible()
    page.screenshot(path=artifacts / "m3-annual-leave-1440.png", full_page=True, animations="disabled")

    go_menu(page, "合同到期提醒")
    due = page.get_by_role("row").filter(has_text=state["due"]["contractNo"])
    expect(due).to_have_count(1)
    expect(due.get_by_text(re.compile("今日|今天|当日"))).to_be_visible()
    page.screenshot(path=artifacts / "m3-reminders-1440.png", animations="disabled")
    button(due, "查看合同").click()
    expect(page.get_by_role("heading", name=state["due"]["employeeName"])).to_be_visible()
    expect(page.get_by_role("article").filter(has_text=state["due"]["contractNo"])).to_be_visible()
    no_overflow(page)
    check_storage(page, traffic.sensitive)
    button(page, "退出登录").click()

    login(page, "hr_viewer", traffic)
    directory = page.locator(".sidebar-container")
    expect(directory.get_by_text("Excel 导入", exact=True)).to_have_count(0)
    go_menu(page, "部门管理")
    expect(button(page, "新增部门")).to_have_count(0)
    go_menu(page, "员工档案")
    exported = download(page, page.get_by_role("button", name=re.compile("导出")))
    records = workbook_rows(exported)
    assert records[0] == EXPORT
    assert any(item[0] == number and item[1] == changed for item in records[1:])
    open_employee(page, number)
    expect(annual.get_by_text("5.5 天", exact=True)).to_be_visible()
    expect(button(annual, "新增年假记录")).to_have_count(0)
    expect(button(page, "编辑员工")).to_have_count(0)
    expect(page.get_by_text("约定工资", exact=True)).to_have_count(0)
    check_storage(page, traffic.sensitive)
    traffic.assert_local()
    print("PASS M3 browser department, XLSX error/retry/export, employee edit, annual leave, reminders and viewer scope")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/m3/state.json"))
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/m3/browser"))
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            context = browser.new_context(viewport={"width": 1440, "height": 960})
            traffic = Traffic(context, args.base)
            main_check(context.new_page(), traffic, args.base, json.loads(args.state.read_text()), args.artifacts)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
