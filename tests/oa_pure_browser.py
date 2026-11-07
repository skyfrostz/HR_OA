"""Pure Admin browser acceptance against the real loopback OA deployment."""
import argparse
import json
import os
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

from playwright.sync_api import Error, expect, sync_playwright

from oa_api import PREFIX, local_base


class Traffic:
    def __init__(self, context, base):
        self.origin = urlsplit(base).netloc
        self.urls = []
        self.sensitive = set()
        context.on("request", lambda request: self.urls.append(request.url))

    def assert_local(self):
        for url in self.urls:
            parsed = urlsplit(url)
            if parsed.scheme not in ("http", "https"):
                continue
            assert parsed.scheme == "http" and parsed.netloc == self.origin, (
                "the browser requested a resource outside the local OA origin"
            )
            assert not re.search(r"/(?:mock|mocks|demo|demos)(?:/|$)", parsed.path, re.I)
            assert not parsed.path.startswith("/api/") or parsed.path.startswith(PREFIX + "/")
            assert all(value not in unquote(url) for value in self.sensitive), "sensitive value in request URL"


def check_storage(page, sensitive):
    # Pure preferences may be persistent. Inspect each origin store without logging values.
    data = page.evaluate("""async () => {
        const databases = await indexedDB.databases();
        const read = dbInfo => new Promise((resolve, reject) => {
            const open = indexedDB.open(dbInfo.name, dbInfo.version);
            open.onerror = () => reject(open.error);
            open.onsuccess = async () => {
                const db = open.result;
                const stores = Array.from(db.objectStoreNames);
                try {
                    const entries = await Promise.all(stores.map(name => new Promise((success, failure) => {
                        const tx = db.transaction(name, 'readonly');
                        const request = tx.objectStore(name).getAll();
                        request.onerror = () => failure(request.error);
                        request.onsuccess = () => success([name, request.result]);
                    })));
                    resolve([db.name, entries]);
                } catch (error) {
                    reject(error);
                } finally {
                    db.close();
                }
            };
        });
        return {
            local: Object.entries(localStorage),
            session: Object.entries(sessionStorage),
            databases: await Promise.all(databases.map(read)),
            caches: await caches.keys()
        };
    }""")
    assert not data["caches"], "unexpected persistent browser response cache"
    data["cookies"] = page.context.cookies()
    serialized = json.dumps(data, ensure_ascii=False)
    assert all(value not in serialized for value in sensitive), "sensitive value persisted in browser storage"
    identity_key = re.compile(r"^(?:x-token|token|access[-_]?token|authorization|user[-_]?info|identity|session)$", re.I)
    assert all(not identity_key.fullmatch(key) for key, _ in data["local"] + data["session"])
    assert all(not identity_key.fullmatch(cookie["name"]) for cookie in data["cookies"])


def login(page, username, traffic):
    expect(page.get_by_role("heading", name=re.compile(r"登录"))).to_be_visible()
    page.get_by_label("账号", exact=True).fill(username)
    try:
        page.get_by_label("密码", exact=True).fill(os.environ["OA_SEED_PASSWORD"])
    except Error:
        raise AssertionError("the password field could not be filled") from None
    with page.expect_response(lambda reply: reply.url.endswith(PREFIX + "/auth/login")) as response:
        page.get_by_role("button", name="登录", exact=True).click()
    assert response.value.ok, "the local OA login failed"
    session = response.value.json()["data"]
    traffic.sensitive.add(session["token"])
    traffic.sensitive.add(os.environ["OA_SEED_PASSWORD"])
    return session


def no_overflow(page):
    assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "page overflows viewport"


def check_pure_pending_upload(page, contract, pdf, writes, sidebar, tags):
    from oa_browser import button

    uncertain = dict(pdf, name="uncertain-" + pdf["name"])
    button(contract, "添加附件").click()
    upload = contract.get_by_role("region", name="添加合同附件")
    upload.get_by_label("选择文件").set_input_files(uncertain)
    held = []
    pattern = "**/api/oa/v1/contracts/*/files"
    def route_handler(route):
        held.append(route)

    def try_navigation():
        current_url = page.url
        for controls in (sidebar, tags):
            target = controls.get_by_text("移动审批入口", exact=True)
            expect(target).to_be_visible()
            if target.is_enabled():
                target.click()
            # Let Vue Router settle, without waiting for the deliberately held request.
            page.evaluate("() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))")
            expect(upload).to_be_visible()
            assert page.url == current_url, "Pure navigation discarded a pending or uncertain write"
            expect(sidebar.locator(".el-menu-item.is-active")).to_have_text("员工档案")

    page.route(pattern, route_handler)
    try:
        upload.locator("form").evaluate("(form) => { form.requestSubmit(); form.requestSubmit() }")
        expect(button(upload, "上传附件")).to_be_disabled()
        try_navigation()
        assert len(held) == 1, "concurrent form submissions issued multiple uploads"
        response = held[0].fetch(max_retries=0)
        try:
            assert response.status == 201
        finally:
            response.dispose()
            held[0].abort()
        expect(upload.get_by_text("上传结果未知。", exact=False)).to_be_visible()
        assert sum(path.endswith("/files") for path in writes) == 2
        try_navigation()
    finally:
        page.unroute(pattern, route_handler)

    button(upload, "刷新并核对附件").click()
    expect(contract.get_by_text(uncertain["name"], exact=True)).to_be_visible()
    expect(button(upload, "上传附件")).to_be_disabled()
    button(upload, "取消").click()
    print("PASS Pure sidebar and tabs preserve pending and uncertain attachment write", flush=True)


def run(browser, base, state, artifacts):
    from oa_api import fixture_pdf
    from oa_browser import button, download_pdf, open_employee

    context = browser.new_context(viewport={"width": 1440, "height": 960})
    traffic = Traffic(context, base)
    page = context.new_page()
    page.goto(base + "/oa.html", wait_until="networkidle")
    admin = login(page, "hr_admin", traffic)
    assert admin["user"]["role"] == "HR_ADMIN"
    table = page.get_by_role("table", name="员工列表")
    expect(table).to_be_visible()
    expect(page.locator(".app-wrapper")).to_be_visible()
    sidebar = page.locator(".sidebar-container")
    tags = page.locator(".tags-view")
    expect(sidebar).to_be_visible()
    expect(tags).to_be_visible()
    expect(tags.get_by_text("员工档案", exact=True)).to_be_visible()
    assert any("/pure-assets/" in url for url in traffic.urls), "PC entry did not load a Pure build asset"
    no_overflow(page)
    page.screenshot(path=artifacts / "pure-employees-1440.png", animations="disabled")

    sidebar.get_by_text("移动审批入口", exact=True).click()
    expect(page.get_by_role("heading", name="移动审批入口")).to_be_visible()
    expect(tags.get_by_text("移动审批入口", exact=True)).to_be_visible()
    tags.get_by_text("员工档案", exact=True).click()
    expect(table).to_be_visible()
    tags.get_by_text("移动审批入口", exact=True).click()
    expect(page.get_by_role("heading", name="移动审批入口")).to_be_visible()
    tags.get_by_text("员工档案", exact=True).click()
    expect(table).to_be_visible()

    number = "PB" + os.urandom(5).hex().upper()
    name, contract_no, filename = "模拟Pure验收员工 " + number, "CT" + number, "fixture-" + number + ".pdf"
    traffic.sensitive.update({number, name, contract_no, filename, "97531842", "¥975,318.42"})
    writes = []

    def record_write(request):
        if request.method == "POST":
            writes.append(urlsplit(request.url).path)

    page.on("request", record_write)
    button(page, "新增员工").click()
    for field, value in {
        "employee-no": number, "employee-name": name, "employee-birth": "1991-01-15",
        "employee-hire": "2020-07-01", "employee-salary": "975318.42",
    }.items():
        page.locator("#" + field).fill(value)
    department = page.locator("#employee-department")
    if department.evaluate("element => element.tagName") == "SELECT":
        department.select_option(index=1)
    else:
        department.fill("模拟Pure测试部门")
    with page.expect_response(lambda reply: reply.url.endswith(PREFIX + "/employees")
                              and reply.request.method == "POST") as created:
        button(page, "保存员工").click()
    assert created.value.status == 201
    identifier = created.value.json()["data"]["id"]
    expect(page.get_by_role("heading", name=name)).to_be_visible()
    expect(page.get_by_text("¥975,318.42", exact=True)).to_be_visible()
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
    check_pure_pending_upload(page, contract, pdf, writes, sidebar, tags)
    no_overflow(page)
    page.screenshot(path=artifacts / "pure-employee-detail-1440.png", full_page=True, animations="disabled")
    check_storage(page, traffic.sensitive)
    print("PASS Pure 1440px layout, sidebar and real tabs; admin employee/contract/file flow", flush=True)

    button(page, "退出登录").click()
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()
    expect(page.locator(".tags-view")).to_have_count(0)
    expect(page.get_by_text(number, exact=False)).to_have_count(0)
    check_storage(page, traffic.sensitive)
    viewer = login(page, "hr_viewer", traffic)
    assert viewer["user"]["role"] == "HR_VIEWER"
    expect(table).to_be_visible()
    expect(tags.get_by_text("移动审批入口", exact=True)).to_have_count(0)
    expect(button(page, "新增员工")).to_have_count(0)
    expect(table.get_by_role("columnheader", name="约定工资")).to_have_count(0)
    open_employee(page, number)
    expect(page.get_by_role("heading", name=name)).to_be_visible()
    expect(page.get_by_text("约定工资", exact=True)).to_have_count(0)
    expect(button(page, "新建合同")).to_have_count(0)
    download_pdf(page, contract, filename)
    check_storage(page, traffic.sensitive)

    page.reload(wait_until="networkidle")
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()
    expect(page.locator(".tags-view")).to_have_count(0)
    expect(page.get_by_text(number, exact=False)).to_have_count(0)
    for username, expected_role in (("employee", "EMPLOYEE"), ("manager", "APPROVER")):
        session = login(page, username, traffic)
        assert session["user"]["role"] == expected_role
        expect(table.locator("tbody tr")).to_have_count(1)
        expect(table.get_by_text(number, exact=True)).to_have_count(0)
        expect(table.get_by_role("columnheader", name="约定工资")).to_be_visible()
        assert session["user"]["employeeId"] != identifier
        button(table, "查看档案").click()
        expect(page.get_by_text("约定工资", exact=True)).to_be_visible()
        expect(page.get_by_text(number, exact=False)).to_have_count(0)
        if username == "employee":
            own_contract = page.get_by_role("article").filter(has_text=state["own_contract_no"])
            expect(own_contract).to_be_visible()
            download_pdf(page, own_contract, "fixture.pdf")
        expect(button(page, "新建合同")).to_have_count(0)
        check_storage(page, traffic.sensitive)
        button(page, "退出登录").click()
        expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()

    # Reuse the same origin and inspect the mobile Pure layout after role switches.
    page.set_viewport_size({"width": 390, "height": 844})
    login(page, "employee", traffic)
    expect(table).to_be_visible()
    no_overflow(page)
    page.screenshot(path=artifacts / "pure-employees-390.png", animations="disabled")
    button(table, "查看档案").click()
    expect(page.get_by_text("约定工资", exact=True)).to_be_visible()
    no_overflow(page)
    page.screenshot(path=artifacts / "pure-employee-detail-390.png", full_page=True, animations="disabled")
    check_storage(page, traffic.sensitive)

    # A previously available upstream demo URL must not mount demo content.
    page.goto(base + "/oa.html#/system/user", wait_until="networkidle")
    expect(page.get_by_text("抱歉，你访问的页面不存在")).to_be_visible()
    page.reload(wait_until="networkidle")
    expect(page.get_by_role("heading", name="登录人事管理系统")).to_be_visible()
    expect(table).to_have_count(0)
    assert all(value not in unquote(page.url) for value in traffic.sensitive)
    traffic.assert_local()
    context.close()
    print("PASS Pure viewer/employee/approver scope, logout/refresh cache cleanup, 390px and local-only traffic", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/oa-state.json"))
    parser.add_argument("--artifacts", type=Path, default=Path("tests/results/pure/browser"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            run(browser, args.base, json.loads(args.state.read_text()), args.artifacts)
            from oa_m2_browser import check as check_h5
            check_h5(browser, args.base, args.artifacts)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
