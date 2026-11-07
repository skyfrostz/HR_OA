"""Chromium acceptance against the real local OA page and development MySQL."""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
from urllib.parse import urlsplit

from playwright.sync_api import Error, expect, sync_playwright

from oa_api import PREFIX, fixture_pdf, local_base


def button(page, name):
    return page.get_by_role("button", name=name, exact=True)


def login(page, user):
    page.get_by_label("账号", exact=True).fill(user)
    try:
        page.get_by_label("密码", exact=True).fill(os.environ["OA_SEED_PASSWORD"])
    except Error:
        raise AssertionError("the password field could not be filled") from None
    button(page, "登录").click()
    expect(page.get_by_role("heading", name="员工档案")).to_be_visible()
    expect(page.get_by_role("table", name="员工列表")).to_be_visible()


def logout(page):
    button(page, "退出登录").click()
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()


def assert_no_persistent_storage(page):
    assert page.evaluate("""async () => localStorage.length === 0 && sessionStorage.length === 0
        && (await indexedDB.databases()).length === 0 && (await caches.keys()).length === 0""")
    assert page.context.cookies() == []


def open_employee(page, number):
    table = page.get_by_role("table", name="员工列表")
    row = table.get_by_role("row").filter(has_text=number)
    next_page = button(page, "下一页")
    while not row.count() and next_page.count() and next_page.is_enabled():
        with page.expect_response(lambda response: "/employees?" in response.url and response.ok):
            next_page.click()
        page.wait_for_load_state("networkidle")
        expect(table).to_be_visible()
    expect(row).to_have_count(1)
    button(row, "查看档案").click()
    expect(page.locator("#employee-heading")).to_be_visible()


def download_pdf(page, scope, filename):
    with page.expect_download() as info:
        button(scope, "下载 " + filename).click()
    download = info.value
    assert download.suggested_filename == filename
    assert download.path().read_bytes() == fixture_pdf()


def check_uncertain_upload(page, contract, pdf, writes):
    uncertain = dict(pdf, name="uncertain-" + pdf["name"])
    button(contract, "添加附件").click()
    upload_form = contract.get_by_role("region", name="添加合同附件")
    upload_form.get_by_label("选择文件").set_input_files(uncertain)
    committed = []
    pattern = "**/api/oa/v1/contracts/*/files"

    def lose_response(route):
        response = route.fetch(max_retries=0)
        committed.append(response.status)
        response.dispose()
        route.abort()

    page.route(pattern, lose_response)
    try:
        # Deliver two submit events during the same pending write.
        upload_form.locator("form").evaluate("(form) => { form.requestSubmit(); form.requestSubmit() }")
        expect(upload_form.get_by_text("上传结果未知。", exact=False)).to_be_visible()
        expect(button(upload_form, "上传附件")).to_have_count(0)
        assert committed == [201]
        assert sum(path.endswith("/files") for path in writes) == 2
    finally:
        page.unroute(pattern, lose_response)
    button(upload_form, "刷新并核对附件").click()
    expect(contract.get_by_text(uncertain["name"], exact=True)).to_be_visible()
    expect(button(upload_form, "上传附件")).to_be_disabled()
    button(upload_form, "取消").click()
    print("PASS Chromium double submit guard, lost response requires readback before retry", flush=True)


def check(page, base, own_contract_no, artifacts):
    suffix = secrets.token_hex(5).upper()
    employee_no, contract_no, filename = f"BE{suffix}", f"BC{suffix}", f"fixture-{suffix}.pdf"
    writes = []

    def record_request(request):
        url = urlsplit(request.url)
        if request.method == "POST" and url.path.startswith(PREFIX):
            writes.append(url.path)
        assert all(value not in request.url for value in (employee_no, contract_no, filename))

    page.on("request", record_request)
    page.goto(base + "/oa.html", wait_until="networkidle")
    login(page, "hr_admin")
    button(page, "新增员工").click()
    for field, value in {
        "employee-no": employee_no, "employee-name": "模拟浏览器员工", "employee-department": "模拟人事部",
        "employee-birth": "1991-01-15", "employee-hire": "2020-07-01", "employee-salary": "12345.67",
    }.items():
        page.locator(f"#{field}").fill(value)
    button(page, "保存员工").click()
    expect(page.get_by_text(re.compile(rf"^(?:工号\s+)?{re.escape(employee_no)}$"))).to_be_visible()
    expect(page.get_by_text("¥12,345.67", exact=True)).to_be_visible()
    assert writes.count(PREFIX + "/employees") == 1

    button(page, "新建合同").click()
    page.get_by_label("合同编号").fill(contract_no)
    page.get_by_label("开始日期").fill("2024-01-01")
    page.get_by_label("结束日期").fill("2027-12-31")
    button(page, "保存合同").click()
    contract = page.get_by_role("article").filter(has_text=contract_no)
    expect(contract).to_have_count(1)
    pdf = {"name": filename, "mimeType": "application/pdf", "buffer": fixture_pdf()}
    button(contract, "添加附件").click()
    contract.locator('input[type="file"]').set_input_files(pdf)
    button(contract, "上传附件").click()
    expect(contract.get_by_text(filename, exact=True)).to_be_visible()
    download_pdf(page, contract, filename)
    assert sum(path.endswith("/contracts") for path in writes) == 1
    assert sum(path.endswith("/files") for path in writes) == 1
    check_uncertain_upload(page, contract, pdf, writes)
    page.screenshot(path=artifacts / "admin-contract.png", full_page=True, animations="disabled")
    assert_no_persistent_storage(page)
    print("PASS Chromium admin real login, create employee and contract, upload and download", flush=True)

    logout(page)
    login(page, "hr_viewer")
    expect(button(page, "新增员工")).to_have_count(0)
    expect(page.get_by_role("columnheader", name="约定工资")).to_have_count(0)
    open_employee(page, employee_no)
    expect(page.get_by_text(re.compile(rf"^(?:工号\s+)?{re.escape(employee_no)}$"))).to_be_visible()
    expect(page.get_by_text("约定工资", exact=True)).to_have_count(0)
    expect(button(page, "新建合同")).to_have_count(0)
    expect(contract.locator('input[type="file"]')).to_have_count(0)
    expect(contract.get_by_text(filename, exact=True)).to_be_visible()
    download_pdf(page, contract, filename)
    assert_no_persistent_storage(page)
    print("PASS Chromium viewer can read and download while salary and write controls stay hidden", flush=True)

    logout(page)
    login(page, "employee")
    table = page.get_by_role("table", name="员工列表")
    expect(table.locator("tbody tr")).to_have_count(1)
    expect(table.get_by_text(employee_no, exact=True)).to_have_count(0)
    expect(table.get_by_role("columnheader", name="约定工资")).to_be_visible()
    button(table, "查看档案").click()
    expect(page.get_by_text("约定工资", exact=True)).to_be_visible()
    own_contract = page.get_by_role("article").filter(has_text=own_contract_no)
    expect(own_contract).to_be_visible()
    download_pdf(page, own_contract, "fixture.pdf")
    expect(contract).to_have_count(0)
    assert_no_persistent_storage(page)
    page.reload(wait_until="networkidle")
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()
    print("PASS Chromium employee sees own row, salary and attachment only; reload requires login", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/oa-state.json"))
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/browser"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    state = json.loads(args.state.read_text())
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            page = browser.new_page()
            check(page, args.base, state["own_contract_no"], args.artifacts)
            narrow = browser.new_page(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True)
            narrow.goto(args.base + "/oa.html", wait_until="networkidle")
            login(narrow, "employee")
            table = narrow.get_by_role("table", name="员工列表")
            expect(table.locator("tbody tr")).to_have_count(1)
            button(table, "查看档案").click()
            own = narrow.get_by_role("article").filter(has_text=state["own_contract_no"])
            expect(own).to_be_visible()
            assert narrow.evaluate("document.documentElement.scrollWidth <= window.innerWidth")
            narrow.screenshot(path=args.artifacts / "employee-narrow.png", full_page=True, animations="disabled")
            print("PASS Chromium 390px viewport employee detail and no page-level horizontal overflow")
        finally:
            browser.close()


if __name__ == "__main__":
    main()
