"""Real OA/MySQL exercising synthetic provider HTTP contracts in an isolated project."""
import argparse
from datetime import datetime, timedelta
import hashlib
from http.cookiejar import CookieJar
import json
import os
import re
import secrets
import subprocess
import time
from urllib.error import HTTPError, URLError
from urllib.parse import parse_qs, urljoin, urlsplit
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener
from zoneinfo import ZoneInfo

from oa_api import NoRedirects, OPENER, PREFIX, api, download, fixture_pdf, local_base, upload
from oa_ci_integration import ci_sql
from oa_m2_api import call as post, general, intent, login
from oa_m3_api import call, page_all, send
from oa_m4_api import account_action, employee as create_employee


def http(opener, url, *, body=None, headers=None):
    request = Request(url, data=None if body is None else json.dumps(body).encode(), headers=headers or {})
    if body is not None:
        request.add_header("Content-Type", "application/json")
    try:
        response = opener.open(request, timeout=15)
    except HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


def control(mock, updates=None):
    status, _, body = http(OPENER, mock + ("/__mock/mode" if updates is not None else "/__mock/state"),
                           body=updates, headers={"X-QA-Control": os.environ["OA_MOCK_CONTROL"]})
    assert status == 200
    return json.loads(body)


def oauth(base, mock, entry, *, should_bind):
    browser = build_opener(ProxyHandler({}), NoRedirects(), HTTPCookieProcessor(CookieJar()))
    status, headers, _ = http(browser, base + PREFIX + f"/auth/wecom/start?entry={entry}")
    assert status in (302, 303), f"{entry} WeCom start HTTP {status}"
    assert "HttpOnly" in headers["Set-Cookie"] and "SameSite=Lax" in headers["Set-Cookie"]
    location = urlsplit(headers["Location"])
    assert f"{location.scheme}://{location.netloc}" == mock and location.path == "/authorize"
    query = parse_qs(location.query)
    assert query.get("login_type") == ["CorpApp"] if entry == "pc" else query.get("scope") == ["snsapi_base"]
    status, headers, _ = http(browser, location.geturl())
    assert status in (302, 303), f"{entry} mock authorization HTTP {status}"
    callback = headers["Location"]
    assert callback.startswith(base + PREFIX + "/auth/wecom/callback?")
    state_before = control(mock)["counts"].get("getuserinfo", 0)
    status, headers, _ = http(browser, callback)
    assert status in (302, 303), f"{entry} WeCom callback HTTP {status}"
    page = "/oa.html" if entry == "pc" else "/h5.html"
    assert urljoin(base, headers["Location"]) == base + page + ("?wecom=ready" if should_bind else "?wecom=failed")
    exchange = base + PREFIX + "/auth/wecom/exchange"
    status, headers, body = http(browser, exchange, body={}, headers={"Origin": base})
    if should_bind:
        assert status == 200 and "no-store" in headers["Cache-Control"]
        token = json.loads(body)["data"]["token"]
        assert call(base, "/auth/me", token)["user"]["role"] == "EMPLOYEE"
    else:
        assert status == 401
        token = None
    assert http(browser, exchange, body={}, headers={"Origin": base})[0] == 401
    after_callback = control(mock)["counts"].get("getuserinfo", 0)
    assert after_callback > state_before
    assert http(browser, callback)[0] in (302, 303)
    assert control(mock)["counts"].get("getuserinfo", 0) == after_callback
    return token


def binding(base, admin, employee_id, member, *, version=None):
    route = f"/employees/{employee_id}/wecom-binding"
    if version is None:
        version = call(base, route, admin)["version"]
    return call(base, route, admin, "PUT", {"version": version, "memberId": member, "reason": "M6 模拟显式绑定"})


def expired_oauth(base, mock):
    browser = build_opener(ProxyHandler({}), NoRedirects(), HTTPCookieProcessor(CookieJar()))
    status, headers, _ = http(browser, base + PREFIX + "/auth/wecom/start?entry=pc")
    assert status == 302
    authorization = headers["Location"]
    state = parse_qs(urlsplit(authorization).query)["state"][0]
    assert re.fullmatch(r"[0-9a-f]{64}", state)
    digest = hashlib.sha256(state.encode()).hexdigest()
    assert ci_sql(f"UPDATE oa_wecom_challenges SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 MINUTE "
                  f"WHERE kind='STATE' AND hash='{digest}'; SELECT ROW_COUNT();") == "1"
    status, headers, _ = http(browser, authorization)
    assert status == 302
    before = control(mock)["counts"].get("getuserinfo", 0)
    status, headers, _ = http(browser, headers["Location"])
    assert status == 303 and urljoin(base, headers["Location"]).endswith("/oa.html?wecom=failed")
    assert control(mock)["counts"].get("getuserinfo", 0) == before


def wait_for(check, explanation, seconds=25):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if result := check():
            return result
        time.sleep(.25)
    raise AssertionError(explanation)


def notification_delivery(base, admin, request_id, wanted):
    event_id = ci_sql(f"SELECT id FROM oa_notification_outbox WHERE request_id={int(request_id)} AND kind='TASK_ASSIGNED' ORDER BY id LIMIT 1;")
    if not event_id:
        return None
    return next((row for row in page_all(base, "/notification-deliveries", admin)
                 if row["id"] == int(event_id) and row["status"] == wanted), None)


def wecom(base, mock, admin, viewer, employee, manager):
    employee_id = employee["user"]["employeeId"]
    manager_id = manager["user"]["employeeId"]
    assert api(base, "/integration/status") == {
        "wecomLogin": "mock", "notifications": "wecom-mock", "attachmentStorage": "oss-mock",
    }
    path = f"/employees/{employee_id}/wecom-binding"
    call(base, path, viewer, status=403)
    assert call(base, path, admin)["memberId"] is None
    oauth(base, mock, "pc", should_bind=False)
    bound = binding(base, admin, employee_id, "Synthetic-A")
    assert bound["memberId"] == "synthetic-a"
    call(base, "/auth/me", employee["token"], status=401)
    stale = {"version": bound["version"] - 1, "memberId": None, "reason": "模拟旧版本"}
    call(base, path, admin, "PUT", stale, 409, "STALE_RECORD")
    before = control(mock)["counts"].get("gettoken", 0)
    control(mock, {"expire_next_token": True})
    employee_token = oauth(base, mock, "h5", should_bind=True)
    assert control(mock)["counts"]["gettoken"] == before + 1
    pc_token = oauth(base, mock, "pc", should_bind=True)
    assert call(base, "/auth/me", pc_token)["user"]["employeeId"] == employee_id
    requests_before = control(mock)["counts"]
    for reserved in ("@all", "@ALL", " @all "):
        call(base, path, admin, "PUT", {"version": bound["version"], "memberId": reserved,
              "reason": "模拟拦截广播目标"}, 400, "INVALID_INPUT")
    assert call(base, path, admin) == bound
    assert call(base, "/auth/me", pc_token)["user"]["employeeId"] == employee_id
    assert control(mock)["counts"] == requests_before
    control(mock, {"next_code": "external"})
    oauth(base, mock, "h5", should_bind=False)
    control(mock, {"next_code": "member-a"})

    binding(base, admin, manager_id, "Synthetic-B")
    call(base, "/notification-deliveries", viewer, status=403)
    for mode, wanted in (("ok", "ACCEPTED"), ("invalid", "FAILED"), ("unknown", "UNCERTAIN")):
        control(mock, {"send_mode": mode})
        body = general("M6 模拟通知隐私")
        created = post(base, "/requests", employee_token, payload=body, key=intent(), status=201)
        delivery = wait_for(lambda: notification_delivery(base, admin, created["id"], wanted),
                            f"outbox did not reach {wanted}")
        assert delivery["mode"] == "wecom-mock" and delivery["attempts"] == 1
        message = control(mock)["messages"][-1]
        assert message["recipient"] == "synthetic-b" and str(delivery["id"]) in message["text"]
        assert body["title"] not in message["text"] and body["form"]["content"] not in message["text"]
    control(mock, {"send_mode": "rate_limit"})
    retried = post(base, "/requests", employee_token, payload=general("M6 模拟限流"), key=intent(), status=201)

    def after_rate_limit():
        event = notification_delivery(base, admin, retried["id"], "PENDING")
        return event if event and event["attempts"] == 1 else None

    wait_for(after_rate_limit, "rate limit did not leave the event pending for retry")
    control(mock, {"send_mode": "ok"})
    delivered = wait_for(lambda: notification_delivery(base, admin, retried["id"], "ACCEPTED"),
                         "rate-limited delivery was not retried")
    assert delivered["attempts"] == 2
    before_legacy = control(mock)["counts"].get("send", 0)
    ci_sql(f"UPDATE oa_wecom_bindings SET member_id='@all' WHERE employee_id={int(manager_id)};")
    try:
        unsafe = post(base, "/requests", employee_token, payload=general("M6 模拟旧非法绑定"), key=intent(), status=201)
        wait_for(lambda: notification_delivery(base, admin, unsafe["id"], "FAILED") or
                 notification_delivery(base, admin, unsafe["id"], "SKIPPED"), "legacy broadcast ID was not blocked")
        assert control(mock)["counts"].get("send", 0) == before_legacy
    finally:
        ci_sql(f"UPDATE oa_wecom_bindings SET member_id='synthetic-b' WHERE employee_id={int(manager_id)};")
    binding(base, admin, employee_id, None)
    call(base, "/auth/me", employee_token, status=401)
    call(base, "/auth/me", pc_token, status=401)
    oauth(base, mock, "pc", should_bind=False)
    binding(base, admin, employee_id, "Synthetic-A")
    expired_oauth(base, mock)
    account_action(base, admin, employee_id, "DISABLE")
    try:
        oauth(base, mock, "h5", should_bind=False)
    finally:
        account_action(base, admin, employee_id, "ENABLE")
    print("PASS M6 WeCom mock PC/H5 one-time auth, explicit binding/epoch, token refresh and three outbox outcomes", flush=True)


def compose_exec(args, *, payload=None, expected_status=0):
    project = os.environ["OA_CI_PROJECT"]
    assert re.fullmatch(r"hr-oa-ci-[0-9a-f]{12}", project)
    result = subprocess.run(["docker", "compose", "--env-file", "/dev/null", "--project-name", project,
                             "--file", "deploy/oa/integration.compose.yaml", *args],
                            input=payload, capture_output=True, timeout=45)
    if result.returncode != expected_status:
        raise RuntimeError(f"Isolated provider fixture command exited {result.returncode}, expected {expected_status}")
    return result.stdout


def restart_api(base):
    compose_exec(["restart", "api"])

    def health():
        try:
            return api(base, "/health")["database"] == "ok"
        except (AssertionError, OSError, URLError):
            return False

    wait_for(health, "isolated API did not become healthy after restart")


def signed_contract(base, admin, employee_id, offset, *, previous=None):
    today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
    payload = {"contractNo": "M6-" + intent()[:12], "startDate": str(today - timedelta(days=90)),
               "endDate": str(today + timedelta(days=offset)), "status": "SIGNED", "reason": "M6 模拟提醒"}
    if previous:
        payload["previousContractId"] = previous
    return call(base, f"/employees/{employee_id}/contracts", admin, "POST", payload, 201)


def scheduled(base, mock, admin, employee, viewer):
    employee_id = employee["user"]["employeeId"]
    bound_user = int(employee["user"]["id"])
    due = signed_contract(base, admin, employee_id, 0)
    boundary = signed_contract(base, admin, employee_id, 30)
    late = signed_contract(base, admin, employee_id, 31)
    past = signed_contract(base, admin, employee_id, -1)
    replaced = signed_contract(base, admin, employee_id, 5)
    successor = signed_contract(base, admin, employee_id, 120, previous=replaced["id"])
    assert successor["previousContractId"] == replaced["id"]
    terminated = signed_contract(base, admin, employee_id, 4)
    today = datetime.now(ZoneInfo("Asia/Shanghai")).date()
    call(base, f"/contracts/{terminated['id']}/actions", admin, "POST", {
        "action": "TERMINATE", "version": terminated["version"], "effectiveDate": str(today), "reason": "M6 模拟终止",
    })
    department_id = call(base, f"/employees/{employee_id}", admin)["departmentId"]
    leaving = create_employee(base, admin, "M6 模拟离职提醒员工", departmentId=department_id)
    account_action(base, admin, leaving["id"], "CREATE", username="m6-" + intent()[:16],
                   password=secrets.token_hex(20), role="EMPLOYEE")
    queued = signed_contract(base, admin, leaving["id"], 12)
    future = signed_contract(base, admin, leaving["id"], 13)
    viewer_id = int(call(base, "/auth/me", viewer)["user"]["id"])
    queued_id = int(queued["id"])
    queued_version = int(queued["version"])
    pending_id = int(ci_sql(f"""
        INSERT INTO oa_notification_outbox (request_id, contract_id, contract_version, reminder_days, schedule_key,
            recipient_id, kind, attempts, delivery_mode, delivery_status, available_at, created_at)
        VALUES (0,{queued_id},{queued_version},30,'contract:{queued_id}:{queued_version}:30:{viewer_id}',
            {viewer_id},'CONTRACT_EXPIRING',0,'mock','PENDING',UTC_TIMESTAMP()+INTERVAL 2 MINUTE,UTC_TIMESTAMP());
        SELECT LAST_INSERT_ID();
    """))
    offboarded = account_action(base, admin, leaving["id"], "OFFBOARD")
    assert offboarded["employmentStatus"] == "OFFBOARDED"
    ci_sql(f"UPDATE oa_notification_outbox SET available_at=UTC_TIMESTAMP()-INTERVAL 1 MINUTE WHERE id={pending_id};")

    def count(contract, recipient=None):
        filter_clause = f" AND recipient_id={int(recipient)}" if recipient else ""
        return int(ci_sql(f"SELECT COUNT(*) FROM oa_notification_outbox WHERE contract_id={int(contract['id'])} AND kind='CONTRACT_EXPIRING'{filter_clause};"))

    restart_api(base)
    wait_for(lambda: count(due, bound_user) == count(boundary, bound_user) == 1,
             "scheduler did not enqueue the 0 and 30 day boundaries")
    assert all(count(item) == 0 for item in (late, past, replaced, successor, terminated))
    assert count(due) == count(boundary) == 3
    later_scan = signed_contract(base, admin, employee_id, 12)
    restart_api(base)
    # This new later-ID row proves the second scan passed the old IDs.
    wait_for(lambda: count(later_scan, bound_user) == 1, "second scheduler scan did not finish")
    wait_for(lambda: any(event["kind"] == "CONTRACT_EXPIRING" and event.get("contractId") == due["id"]
                         for event in page_all(base, "/notifications", viewer)), "HR viewer did not receive a local reminder")
    assert count(due) == count(boundary) == 3
    assert count(due, bound_user) == count(boundary, bound_user) == 1
    assert all(count(item) == 0 for item in (late, past, replaced, successor, terminated))
    wait_for(lambda: ci_sql(f"SELECT delivery_status FROM oa_notification_outbox WHERE id={pending_id};") == "SKIPPED",
             "previously queued HR reminder did not skip an offboarded employee")
    assert ci_sql(f"SELECT COUNT(*) FROM oa_mock_notifications WHERE event_id={pending_id};") == "0"
    assert count(queued) == 1 and count(future) == 0
    assert any("合同" in entry["text"] and entry["recipient"] == "synthetic-a" for entry in control(mock)["messages"])
    print("PASS M6 scheduled 0/30-day reminders, 31/past exclusions, renewed/terminated suppression and restart deduplication", flush=True)


def oss(base, mock, admin, employee_id):
    state = control(mock)
    assert all(state["counts"].get("oss_bucket_" + key, 0) for key in ("acl", "publicAccessBlock", "versioning"))
    contract = signed_contract(base, admin, employee_id, 180)
    body = fixture_pdf()
    file = upload(base, contract["id"], admin, "m6-oss-fixture.pdf", body)
    assert download(base, file["id"], admin) == hashlib.sha256(body).hexdigest()
    after = control(mock)
    assert after["objectCount"] == state["objectCount"] + 1 and after["counts"]["oss_put"] >= 1
    control(mock, {"oss_mode": "corrupt"})
    failure = send(base, f"/files/{file['id']}/preview", admin)
    assert failure.status >= 500 and body not in failure.body
    control(mock, {"oss_mode": "unavailable"})
    before = call(base, f"/contracts/{contract['id']}", admin)["files"]
    upload(base, contract["id"], admin, "failed.pdf", body, status=500)
    assert call(base, f"/contracts/{contract['id']}", admin)["files"] == before
    control(mock, {"oss_mode": "ok"})
    assert send(base, f"/files/{file['id']}/preview", admin).body == body
    print("PASS M6 signed OSS private bucket checks, private write/read, corruption and atomic metadata failure", flush=True)

    def local_source(key):
        identifier = int(ci_sql(f"""
        INSERT INTO oa_contract_files (created_at, updated_at, classification, version, storage_backend, sha256,
            contract_id, original_name, size, content_type, storage_key)
        SELECT UTC_TIMESTAMP(), UTC_TIMESTAMP(), 'STANDARD', 1, 'local', '',
            contract_id, 'm6-migration-fixture.pdf', size, content_type, '{key}'
        FROM oa_contract_files WHERE id={int(file['id'])};
        SELECT LAST_INSERT_ID();
        """))
        compose_exec(["exec", "-T", "api", "sh", "-ec", 'umask 077; cat > "/var/lib/oa/private/$1"', "sh", key], payload=body)
        return identifier, ["exec", "-T", "api", "/usr/local/bin/hr-oa", "--migrate-attachments", f"--after-id={identifier-1}", "--limit=1"]

    key = intent()
    legacy_id, command = local_source(key)
    before = control(mock)["objectCount"]
    dry = json.loads(compose_exec(command))
    digest = hashlib.sha256(body).hexdigest()
    assert dry["list"] == [{"id": legacy_id, "sha256": digest, "status": "LOCAL_VERIFIED"}]
    assert dry["migrated"] == 0 and control(mock)["objectCount"] == before
    control(mock, {"oss_mode": "corrupt"})
    failed = json.loads(compose_exec([*command, "--apply"], expected_status=1))
    assert failed["list"][0]["status"] == "VERIFY_FAILED" and failed["migrated"] == 0
    assert ci_sql(f"SELECT storage_backend FROM oa_contract_files WHERE id={legacy_id};") == "local"
    assert download(base, legacy_id, admin) == digest
    control(mock, {"oss_mode": "ok"})
    applied = json.loads(compose_exec([*command, "--apply"]))
    assert applied["list"] == [{"id": legacy_id, "sha256": digest, "status": "MIGRATED"}]
    assert applied["migrated"] == 1 and control(mock)["objectCount"] == before + 1
    assert ci_sql(f"SELECT storage_backend FROM oa_contract_files WHERE id={legacy_id};") == "oss"
    assert download(base, legacy_id, admin) == digest
    assert compose_exec(["exec", "-T", "api", "sha256sum", f"/var/lib/oa/private/{key}"]).split()[0].decode() == digest
    assert json.loads(compose_exec([*command, "--apply"]))["examined"] == 0
    conflict_key = intent()
    conflict_id, conflict_cmd = local_source(conflict_key)
    remote_seed = (b"x" * len(body)).hex()
    assert http(OPENER, mock + "/__mock/seed-object", body={"key": "qa/" + conflict_key, "contentHex": remote_seed},
                headers={"X-QA-Control": os.environ["OA_MOCK_CONTROL"]})[0] == 200
    conflict = json.loads(compose_exec([*conflict_cmd, "--apply"], expected_status=1))
    assert conflict["list"][0]["status"] == "VERIFY_FAILED" and conflict["migrated"] == 0
    assert ci_sql(f"SELECT storage_backend FROM oa_contract_files WHERE id={conflict_id};") == "local"
    assert download(base, conflict_id, admin) == digest
    print("PASS M6 migration CLI dry run, checksum failure/exit 1, identical retry, conflicting object and retained local source", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    parser.add_argument("--mock", type=local_base, required=True)
    args = parser.parse_args()
    admin = login(args.base, "hr_admin", "HR_ADMIN")["token"]
    viewer = login(args.base, "hr_viewer", "HR_VIEWER")["token"]
    employee = login(args.base, "employee", "EMPLOYEE")
    manager = login(args.base, "manager", "APPROVER")
    wecom(args.base, args.mock, admin, viewer, employee, manager)
    scheduled(args.base, args.mock, admin, employee, viewer)
    oss(args.base, args.mock, admin, employee["user"]["employeeId"])


if __name__ == "__main__":
    main()
