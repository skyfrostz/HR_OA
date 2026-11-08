"""Real browser task checks on a disposable OA CI stack."""
import argparse
from datetime import datetime
import os
from pathlib import Path
import re
import traceback
from zoneinfo import ZoneInfo

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base
from oa_browser import button
from oa_m2_api import intent, login, request_detail
from oa_m3_api import binary_workbook, call, fill_workbook
from oa_m2_browser import login as h5_login, tab, unobstructed
from oa_m4_browser import cross_end, directory, employee_landing, menu, open_employee, pc_page
from oa_m5_browser import wait_write


def employee_edit(browser, base, artifacts):
    page, _, auth = pc_page(browser, base, "hr_admin")
    token = auth["token"]
    department = call(base, "/departments", token)["list"][0]
    before = call(base, "/employees", token, "POST", {
        "employeeNo": "UI-BROWSER-" + intent()[:10], "name": "编辑验收用合成员工",
        "departmentId": department["id"], "birthDate": "1990-01-01", "hireDate": "2020-01-01",
        "salaryCents": 160000,
    }, 201)
    employee_landing(page)
    panel = directory(page)
    filter_field = panel.get_by_label("搜索员工", exact=True)
    filter_field.fill(before["employeeNo"])
    with page.expect_response(lambda r: r.url.endswith("/employees/search") and r.ok):
        button(panel, "搜索").click()
    open_employee(page, before["employeeNo"])
    dialog = page.get_by_role("dialog", name="员工档案详情")
    editor = button(dialog, "编辑员工")
    editor.click()
    name = page.locator("#employee-name")
    expect(name).to_be_editable()
    name.fill("这次未保存")
    button(dialog, "取消").click()
    expect(dialog.locator("#employee-heading")).to_have_text(before["name"])
    assert call(base, f"/employees/{before['id']}", token)["version"] == before["version"]
    editor.click()
    name.fill("已保存的界面验收员工")
    saved = wait_write(page, f"/employees/{before['id']}", button(dialog, "保存员工"), method="PUT")
    assert saved["version"] == before["version"] + 1 and saved["name"] == "已保存的界面验收员工"
    expect(dialog.locator("#employee-heading")).to_have_text(saved["name"])
    page.screenshot(path=artifacts / "saved-employee.png", full_page=False, animations="disabled")
    page.keyboard.press("Escape")
    expect(dialog).to_have_count(0)
    expect(filter_field).to_have_value(before["employeeNo"])
    target = button(panel, "查看档案").first
    try:
        expect(target).to_be_focused()
    except AssertionError:
        print("Focus after save and close:", page.evaluate("() => ({tag:document.activeElement.tagName,id:document.activeElement.id})"), flush=True)
        raise
    print("PASS UI/UX browser edit cancel/save, live persistence and restored roster filter/focus", flush=True)


def replace_workbook(browser, base, artifacts):
    page, _, auth = pc_page(browser, base, "hr_admin")
    token = auth["token"]
    person = login(base, "employee", "EMPLOYEE")["user"]["employeeId"]
    department = call(base, f"/employees/{person}", token)["department"]
    template = binary_workbook(base, "/employees/import-template.xlsx", token)
    menu(page, "Excel 导入")
    field = page.locator("#oa-import-file")
    prefix = "UI-R-" + intent()[:10]

    def workbook(index):
        return {"name": f"界面验收预览替换-{index}.xlsx",
                "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                "buffer": fill_workbook(template, [(f"{prefix}-{index}", "合成员工", department,
                                                   "1990-01-01", "2020-01-01", "1000.00")])}
    field.set_input_files(workbook(1))
    old = wait_write(page, "/employee-imports/preview", button(page, "预览导入"))
    expect(button(page, "确认导入")).to_be_enabled()
    with page.expect_response(lambda r: r.url.endswith(f"/employee-imports/{old['id']}/discard")) as response:
        field.set_input_files(workbook(2))
    assert response.value.ok
    expect(button(page, "确认导入")).to_have_count(0)
    expired = call(base, f"/employee-imports/{old['id']}", token)
    assert expired["status"] == "EXPIRED" and expired["rows"] == []
    call(base, f"/employee-imports/{old['id']}/confirm", token, "POST", {}, 410, "PREVIEW_EXPIRED")
    current = wait_write(page, "/employee-imports/preview", button(page, "预览导入"))
    assert old["id"] != current["id"] and current["canConfirm"]
    page.screenshot(path=artifacts / "replaced-excel-new-preview.png", full_page=False, animations="disabled")
    committed = wait_write(page, f"/employee-imports/{current['id']}/confirm", button(page, "确认导入"))
    assert committed["importedCount"] == 1
    assert call(base, "/employees/search", token, "POST", {"query": f"{prefix}-1"})["total"] == 0
    assert call(base, "/employees/search", token, "POST", {"query": f"{prefix}-2"})["total"] == 1
    print("PASS UI/UX browser workbook replacement expires old server preview and imports only the replacement", flush=True)


def h5_long_submit(browser, base, artifacts):
    page = browser.new_page(viewport={"width": 320, "height": 740}, is_mobile=True, has_touch=True)
    h5_login(page, base, "employee")
    tab(page, "发起申请").click()
    page.get_by_role("list", name="申请类型").get_by_role("button", name=re.compile("报销申请")).click()
    title = "手机窄屏模拟报销 " + intent()[:8]
    reason = "用于验证长原因、窄屏固定栏、字段输入和实际提交的合成内容。" * 14
    page.get_by_label("标题", exact=True).fill(title)
    page.get_by_label("费用日期", exact=True).fill(datetime.now(ZoneInfo("Asia/Shanghai")).date().isoformat())
    page.get_by_label("报销金额（元）", exact=True).fill("12.34")
    last = page.get_by_label("报销事由", exact=True)
    last.fill(reason)
    last.focus()
    last.evaluate("e => e.scrollIntoView({block:'center'})")
    unobstructed(last)
    expect(last).to_be_focused()
    submit = button(page, "提交申请")
    expect(submit).to_be_enabled()
    unobstructed(submit)
    page.screenshot(path=artifacts / "h5-long-enabled-submit-320.png", full_page=False, animations="disabled")
    created = wait_write(page, "/requests", submit)
    token = login(base, "employee", "EMPLOYEE")["token"]
    saved = request_detail(base, created["id"], token)
    assert saved["title"] == title and saved["type"] == "EXPENSE" and saved["form"]["reason"] == reason
    expect(page.get_by_role("heading", name=title, exact=True)).to_be_visible()
    print("PASS UI/UX H5 320px last field and enabled submit reachable; long reason persisted", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"hr-oa-ci-[a-f0-9]{12}", os.environ.get("OA_CI_PROJECT", "")):
        parser.error("requires the disposable CI environment")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        failed = []
        try:
            for check in (employee_edit, replace_workbook, h5_long_submit, cross_end):
                try:
                    check(browser, args.base, args.artifacts)
                except Exception:
                    failed.append(check.__name__)
                    traceback.print_exc()
        finally:
            browser.close()
    if failed:
        raise SystemExit("Browser regression failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
