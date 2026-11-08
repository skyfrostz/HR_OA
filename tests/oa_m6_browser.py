"""M6 Chromium acceptance with real API responses and synthetic documents."""
import argparse
from datetime import datetime, timedelta
import hashlib
import json
import os
from pathlib import Path
import re
from urllib.parse import urlsplit
from zoneinfo import ZoneInfo

from playwright.sync_api import expect, sync_playwright

from oa_api import local_base, upload
from oa_browser import button
from oa_commercial_browser import contrast
from oa_m2_api import intent, login
from oa_m2_browser import tab
from oa_m3_api import binary_workbook, call, fill_workbook, page_all, workbook_rows
from oa_m4_browser import directory, employee_landing, menu, open_employee, pc_page, screenshot, set_theme
from oa_m5_browser import heading, show, wait_write
from oa_m6_providers import binding, restart_api, signed_contract, wait_for
from oa_pure_browser import check_storage

FIXTURES = Path(__file__).parent / "fixtures"
# The four PDF documents are generated synthetic browser fixtures from the M6
# frontend preview. The normal fixture has Chinese CID text plus inert URI/JS probes.
PDF = "模拟劳动合同.pdf"


def find_employee(page, employee_no):
    employee_landing(page)
    panel = directory(page)
    panel.get_by_label("搜索员工").fill(employee_no)
    with page.expect_response(lambda reply: reply.url.endswith("/employees/search") and reply.ok):
        button(panel, "搜索").click()
    open_employee(page, employee_no)


def attach(page, contract, name, fixture):
    button(contract, "添加附件").click()
    expect(page.locator("#oa-upload-classification")).to_have_value("RESTRICTED")
    page.locator("#oa-attachment-input").set_input_files({
        "name": name, "mimeType": "application/pdf", "buffer": fixture.read_bytes(),
    })
    return wait_write(page, "/files", button(page, "上传附件"))


def attachment_flow(browser, base, page, admin, employee_id, artifacts):
    profile = call(base, f"/employees/{employee_id}", admin)
    today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
    record = call(base, f"/employees/{employee_id}/contracts", admin, "POST", {
        "contractNo": "M6-QA-" + intent()[:10], "startDate": str(today - timedelta(days=365)),
        "endDate": str(today + timedelta(days=365)), "status": "DRAFT", "reason": "M6 模拟合同验收",
    }, 201)
    find_employee(page, profile["employeeNo"])
    contract = page.get_by_role("article").filter(has=page.get_by_text(record["contractNo"], exact=True))
    expect(contract).to_be_visible()
    external = []
    page.route(re.compile(r"^https?://example\.invalid/"), lambda route: (external.append(route.request.url), route.abort()))
    saved = attach(page, contract, PDF, FIXTURES / "m6-normal.pdf")
    assert saved["classification"] == "RESTRICTED"
    selected = page.get_by_role("region", name="合同详情", exact=True).filter(
        has=page.get_by_role("heading", name=record["contractNo"], exact=True))
    expect(selected.get_by_text("敏感", exact=True)).to_be_visible()
    screenshot(page, artifacts, "restricted-file-1440.png")
    preview = button(selected, "预览 " + PDF)
    with page.expect_response(lambda reply: reply.url.endswith(f"/files/{saved['id']}/preview")) as requested:
        preview.click()
    assert requested.value.ok
    dialog = page.get_by_role("dialog", name="附件预览 · " + PDF, exact=True)
    canvas = dialog.locator(".oa-attachment-preview canvas")
    expect(canvas).to_have_count(1)
    expect(canvas).to_be_visible()
    expect(dialog.get_by_text("正在读取附件…", exact=True)).to_be_hidden()
    assert canvas.evaluate("""canvas => {
        const context=canvas.getContext('2d'); const p=context.getImageData(0,0,canvas.width,canvas.height).data;
        let ink=0; for(let i=0;i<p.length;i+=4) if(p[i+3] && p[i]+p[i+1]+p[i+2]<620) ink++;
        return ink>1000;
    }""")
    assert not dialog.locator(".oa-attachment-preview :is(a,iframe,object,embed,.annotationLayer)").count()
    assert external == []
    screenshot(page, artifacts, "pdf-preview-1440.png")
    button(dialog, "关闭预览").click()
    expect(dialog).to_have_count(0)

    for kind in ("huge", "encrypted", "malformed"):
        name = f"M6-{kind}.pdf"
        rejected = upload(base, record["id"], admin, name, (FIXTURES / f"m6-{kind}.pdf").read_bytes())
        assert rejected["originalName"] == name
    # Reopen the employee to obtain a fresh, authorized contract read.
    button(page, "返回员工列表").click()
    open_employee(page, profile["employeeNo"])
    for kind in ("huge", "encrypted", "malformed"):
        name = f"M6-{kind}.pdf"
        button(contract, "预览 " + name).click()
        problem = page.get_by_role("dialog", name="附件预览 · " + name, exact=True)
        expect(problem.get_by_text("无法显示此附件，可关闭预览后下载原件核对。", exact=True)).to_be_visible()
        expect(problem.locator("canvas")).to_have_count(0)
        button(problem, "关闭预览").click()
        expect(problem).to_have_count(0)

    image_file = upload(base, record["id"], admin, "M6-样例.png", (FIXTURES / "pixel.png").read_bytes())
    button(page, "返回员工列表").click()
    open_employee(page, profile["employeeNo"])
    page.evaluate("""() => {
      window.qaRevokedBlobURLs = [];
      const release = URL.revokeObjectURL.bind(URL);
      URL.revokeObjectURL = url => { window.qaRevokedBlobURLs.push(url); return release(url); };
    }""")
    button(contract, "预览 M6-样例.png").click()
    image = page.get_by_role("dialog", name="附件预览 · M6-样例.png", exact=True)
    picture = image.get_by_role("img", name="M6-样例.png", exact=True)
    expect(picture).to_be_visible()
    url = picture.get_attribute("src")
    assert url.startswith("blob:") and image_file["id"] != saved["id"]
    button(image, "关闭预览").click()
    expect(image).to_have_count(0)
    assert page.evaluate("url => window.qaRevokedBlobURLs.includes(url)", url)

    viewer, viewer_traffic, _ = pc_page(browser, base, "hr_viewer")
    find_employee(viewer, profile["employeeNo"])
    visible_contract = viewer.get_by_role("article").filter(has=viewer.get_by_text(record["contractNo"], exact=True))
    expect(visible_contract).to_be_visible()
    expect(visible_contract.get_by_text(PDF, exact=False)).to_have_count(0)
    expect(button(visible_contract, "预览 M6-样例.png")).to_be_visible()
    screenshot(viewer, artifacts, "viewer-file-scope-1440.png")
    viewer_traffic.assert_local()
    button(page, "返回员工列表").click()
    print("PASS M6 real attachment upload defaults sensitive; scope, Chinese PDF canvas, inert URI/JS, failure recovery and image release", flush=True)
    return {"contractId": record["id"], "fileId": saved["id"], "employeeId": employee_id,
            "sha256": hashlib.sha256((FIXTURES / "m6-normal.pdf").read_bytes()).hexdigest()}


def excel_flow(page, admin, base, department, artifacts):
    menu(page, "Excel 导入")
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    suffix = intent()[:8]
    valid = ("M6-" + suffix, "模拟验收有效员工", department, "1990-01-01", "2022-01-01", "1500.50")
    invalid = ("ME-" + suffix, "模拟日期错误员工", department, "1990-02-30", "2022-01-01", "1500.50")

    def preview(rows):
        page.locator("#oa-import-file").set_input_files({
            "name": "M6-模拟导入.xlsx", "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            "buffer": fill_workbook(template, rows),
        })
        return wait_write(page, "/employee-imports/preview", button(page, "预览导入"))

    errors = preview([valid, invalid])
    expect(heading(page, "预览结果")).to_be_visible()
    # Error-priority redesign may default to errors; first show all rows so the
    # keyboard test still proves the error-only control changes the view.
    all_rows = button(page, "全部行")
    if all_rows.get_attribute("aria-pressed") == "false":
        all_rows.click()
    expect(page.get_by_text(valid[0], exact=True)).to_be_visible()
    only = page.get_by_role("button", name=re.compile("^仅错误行"))
    only.focus()
    page.keyboard.press("Space")
    expect(only).to_have_attribute("aria-pressed", "true")
    expect(page.get_by_text(valid[0], exact=True)).to_have_count(0)
    expect(page.get_by_text(invalid[0], exact=True)).to_be_visible()
    screenshot(page, artifacts, "excel-errors-1440.png")
    with page.expect_download() as downloaded:
        button(page, "下载错误行").click()
    rows = workbook_rows(Path(downloaded.value.path()).read_bytes())
    assert any(invalid[0] in row for row in rows[1:]) and not any(valid[0] in row for row in rows[1:])
    selected = preview([valid])
    committed = wait_write(page, f"/employee-imports/{selected['id']}/confirm", button(page, "确认导入"))
    assert committed["importedCount"] == 1
    expect(page.get_by_text(re.compile(r"已导入 1 名员工|1\s*名员工已导入"))).to_be_visible()
    result_region = page.get_by_role("region", name="Excel 导入", exact=True)
    ratios = contrast(result_region, {"success_text": ".oa-import-count, .oa-inline-alert.el-alert--success .el-alert__title"})
    print(f"M6 import success notice contrast {ratios}", flush=True)
    assert min(ratios.values()) >= 4.5
    expect(page.get_by_text(valid[0], exact=True)).to_have_count(0)
    disclosure = page.locator("summary").filter(has_text=re.compile("^历史预览与导入结果$"))
    if disclosure.count():
        disclosure.click()
    history = page.get_by_role("region", name="我的导入记录", exact=True)
    with page.expect_response(lambda reply: reply.url.endswith(f"/employee-imports/{selected['id']}/result") and reply.ok):
        button(history, "查看结果").first.click()
    screenshot(page, artifacts, "excel-result-history-1440.png")
    print("PASS M6 Excel keyboard error filter, real XLSX download, confirmation and history result without source rows", flush=True)
    return errors["id"], selected["id"]


def integration_shots(browser, page, base, artifacts):
    menu(page, "集成状态")
    region = page.get_by_role("region", name="集成状态", exact=True)
    expect(region.locator(".oa-integration-status")).to_have_count(3)
    for label, mode in (("light", False), ("dark", True)):
        if mode:
            set_theme(page)
        states = {f"state-{k}": f".oa-integration-grid > section:nth-child({k}) .oa-integration-status" for k in (1, 2, 3)}
        states["detail"] = ".oa-integration-grid .oa-muted"
        ratios = contrast(region, states)
        assert min(ratios.values()) >= 4.5, f"integration {label} contrast: {ratios}"
        print(f"M6 integration {label} contrast {ratios}", flush=True)
        screenshot(page, artifacts, f"integration-{label}-1440.png")
    phone, traffic, _ = pc_page(browser, base, "hr_admin", 390)
    set_theme(phone)
    show(phone, "/integrations", "集成状态")
    expect(phone.locator(".oa-integration-status")).to_have_count(3)
    screenshot(phone, artifacts, "integration-dark-390.png")
    traffic.assert_local()


def mock_logins(browser, base, artifacts, *, notice_only=False):
    for width, entry in ((1440, "oa.html"), (390, "h5.html")):
        page = browser.new_page(viewport={"width": width, "height": 960 if width == 1440 else 844},
                                is_mobile=width == 390, has_touch=width == 390,
                                color_scheme="dark" if width == 390 else "light")
        page.goto(base + "/" + entry)
        link = page.get_by_role("link", name="企业微信模拟登录", exact=True)
        expect(link).to_be_visible()
        if not notice_only:
            screenshot(page, artifacts, f"wecom-login-{width}.png")
        with page.expect_response(lambda reply: reply.url.endswith("/auth/wecom/exchange") and reply.ok) as exchange:
            link.click()
        token = exchange.value.json()["data"]["token"]
        expect(button(page, "退出登录")).to_be_visible()
        assert not urlsplit(page.url).query
        assert not page.context.cookies(), "authorization cookies persisted after successful exchange"
        check_storage(page, {token})
        if not notice_only:
            screenshot(page, artifacts, f"wecom-signed-in-{width}.png")
        if width == 1440:
            menu(page, "站内通知")
            open_contract = button(page, "查看合同详情").first
            expect(open_contract).to_be_visible()
            card = open_contract.locator("xpath=ancestor::li")
            ratios = contrast(card, {"delivery_status": ".oa-delivery-status"})
            print(f"M6 PC 1440px light contract notice contrast {ratios}", flush=True)
            assert min(ratios.values()) >= 4.5
            screenshot(page, artifacts, "contract-notice-pc-1440.png")
            with page.expect_response(lambda reply: re.search(r"/contracts/[0-9]+$", reply.url) and reply.ok):
                open_contract.click()
            expect(page.get_by_role("dialog", name="员工档案详情")).to_be_visible()
        else:
            tab(page, "站内通知").click()
            notice = page.get_by_role("link", name="查看合同到期提醒", exact=True).first
            expect(notice).to_have_attribute("href", "./oa.html#/contract-reminders")
            card = notice.locator("xpath=ancestor::article")
            expect(card.get_by_text(re.compile("^模拟(?: API 已接受)?$"))).to_be_visible()
            ratios = contrast(card, {"delivery_status": ".van-tag", "description": "time, .h5-muted"})
            print(f"M6 H5 390px dark contract notice contrast {ratios}", flush=True)
            assert min(ratios.values()) >= 4.5
            screenshot(page, artifacts, "contract-notice-h5-390.png")
    print("PASS M6 real PC/H5 browser mock OAuth and in-memory OA session", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--mock", action="store_true")
    parser.add_argument("--prepare-mock", action="store_true", help="seed mock-only fixtures in the disposable CI project")
    parser.add_argument("--artifacts", type=Path)
    parser.add_argument("--section", choices=("all", "contract-notices"), default="all")
    args = parser.parse_args()
    if args.section == "contract-notices" and not args.prepare_mock:
        parser.error("contract-notices requires the isolated mock fixtures")
    mock_artifacts = "tests/results/m6/mock-browser-final" if args.prepare_mock else "tests/results/m6/mock-browser"
    output = args.artifacts or Path(mock_artifacts if args.mock else "tests/results/m6/browser")
    output.mkdir(parents=True, exist_ok=True)
    employee = login(args.base, "employee", "EMPLOYEE")
    employee_id = employee["user"]["employeeId"]
    if args.prepare_mock:
        if not args.mock:
            parser.error("--prepare-mock requires --mock and a disposable CI project")
        # compose_exec/restart_api also require the script's random CI project.
        if not re.fullmatch(r"hr-oa-ci-[0-9a-f]{12}", os.environ.get("OA_CI_PROJECT", "")):
            parser.error("--prepare-mock requires the disposable CI script")
        admin = login(args.base, "hr_admin", "HR_ADMIN")["token"]
        binding(args.base, admin, employee_id, "Synthetic-A")
        due = signed_contract(args.base, admin, employee_id, 10)
        restart_api(args.base)
        current = login(args.base, "employee", "EMPLOYEE")["token"]
        wait_for(lambda: any(row.get("contractId") == due["id"] for row in page_all(args.base, "/notifications", current)),
                 "synthetic scheduled contract notice did not arrive")
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            if args.section == "contract-notices":
                mock_logins(browser, args.base, output, notice_only=True)
                return
            if args.mock:
                mock_logins(browser, args.base, output)
            page, traffic, administrator = pc_page(browser, args.base, "hr_admin")
            admin = administrator["token"]
            if args.mock:
                profile = call(args.base, f"/employees/{employee_id}", admin)
                find_employee(page, profile["employeeNo"])
                button(page.get_by_role("navigation", name="员工详情分组"), "企业微信").click()
                expect(page.get_by_role("region", name="企业微信身份绑定").get_by_text("synthetic-a", exact=True)).to_be_visible()
                screenshot(page, output, "wecom-binding-1440.png")
                button(page, "返回员工列表").click()
            state = attachment_flow(browser, args.base, page, admin, employee_id, output)
            profile = call(args.base, f"/employees/{employee_id}", admin)
            state["expiredCandidateId"], state["importId"] = excel_flow(page, admin, args.base, profile["department"], output)
            integration_shots(browser, page, args.base, output)
            check_storage(page, traffic.sensitive | {PDF})
            traffic.assert_local()
            (output.parent / ("mock-browser-state.json" if args.mock else "browser-state.json")).write_text(
                json.dumps(state, indent=2) + "\n")
        finally:
            browser.close()


if __name__ == "__main__":
    main()
