"""M4 business, authorization and upgrade checks using synthetic MySQL records."""
import argparse
import hashlib
from datetime import datetime
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
from zoneinfo import ZoneInfo

from oa_api import fixture_pdf, local_base, upload, download
from oa_m2_api import action, action_route, call as workflow, general, intent, login, race, request_detail, send as workflow_send
from oa_m3_api import call, employee_input, page_all, send

REASON = "M4 模拟验收"
TODAY = datetime.now(ZoneInfo("Asia/Shanghai")).date().isoformat()


def employee(base, admin, name, **fields):
    return call(base, "/employees", admin, "POST",
                employee_input("M4" + intent()[:12], name, **(fields or {"department": "模拟 M4"})), 201)


def audits(base, token, query=""):
    return page_all(base, "/audit-events" + query, token)


def account(base, admin, employee_id):
    return call(base, f"/employees/{employee_id}/account", admin)


def account_action(base, admin, employee_id, kind, *, status=200, error=None, version=None, **fields):
    route = f"/employees/{employee_id}/account/actions"
    if version is None:
        version = account(base, admin, employee_id)["version"]
    return call(base, route, admin, "POST", {"action": kind, "reason": REASON, "version": version, **fields},
                status, error)


def login_password(base, username, password, status=200):
    return workflow(base, "/auth/login", payload={"username": username, "password": password}, status=status)


def test_accounts(base, admin, viewer):
    record = employee(base, admin, "模拟账号生命周期")
    identifier = record["id"]
    call(base, f"/employees/{identifier}/account", viewer, status=403)
    initial = account(base, admin, identifier)
    assert initial["account"] is None and initial["employmentStatus"] == "ACTIVE"
    username = "m4-" + intent()[:16]
    password, new_password = secrets.token_hex(20), secrets.token_hex(20)
    account_action(base, admin, identifier, "CREATE", username=username, password=password,
                   role="HR_ADMIN", status=400, error="INVALID_INPUT")
    created = account_action(base, admin, identifier, "CREATE", username=username, password=password,
                             role="EMPLOYEE")
    assert created["version"] == initial["version"] + 1 and created["account"]["enabled"]
    assert password not in json.dumps(created)
    token_initial = login_password(base, username, password)["token"]
    assert [row["id"] for row in page_all(base, "/employees", token_initial)] == [identifier]
    account_action(base, admin, identifier, "SET_ROLE", role="APPROVER")
    call(base, "/auth/me", token_initial, status=401, error="UNAUTHENTICATED")
    token_approver = login_password(base, username, password)["token"]
    assert call(base, "/auth/me", token_approver)["user"]["role"] == "APPROVER"
    version_before_reset = account(base, admin, identifier)["version"]
    raced_login, reset = race(
        lambda: workflow_send(base, "/auth/login", payload={"username": username, "password": password}),
        lambda: account_action(base, admin, identifier, "RESET_PASSWORD", version=version_before_reset, password=new_password),
    )
    assert reset["version"] == version_before_reset + 1
    assert raced_login.status in (200, 401)
    raced_tokens = (raced_login.data["token"],) if raced_login.status == 200 else ()
    for obsolete in (token_approver, *raced_tokens):
        call(base, "/auth/me", obsolete, status=401, error="UNAUTHENTICATED")
    login_password(base, username, password, 401)
    token_reset = login_password(base, username, new_password)["token"]
    disabled = account_action(base, admin, identifier, "DISABLE")
    assert disabled["account"]["enabled"] is False and disabled["employmentStatus"] == "ACTIVE"
    call(base, "/auth/me", token_reset, status=401, error="UNAUTHENTICATED")
    login_password(base, username, new_password, 401)
    detail = call(base, f"/employees/{identifier}", admin)
    payload = {key: detail[key] for key in ("employeeNo", "birthDate", "hireDate", "departmentId", "salaryCents", "version")}
    call(base, f"/employees/{identifier}", admin, "PUT", dict(payload, name="模拟停用后编辑"))
    assert account(base, admin, identifier)["account"]["enabled"] is False
    account_action(base, admin, identifier, "ENABLE")
    token_enabled = login_password(base, username, new_password)["token"]
    for obsolete in (token_initial, token_approver, token_reset, *raced_tokens):
        call(base, "/auth/me", obsolete, status=401, error="UNAUTHENTICATED")
    assert call(base, "/auth/me", token_enabled)["user"]["employeeId"] == identifier
    events = audits(base, admin, f"?employeeId={identifier}&type=ACCOUNT")
    actions = [event["action"] for event in events]
    assert actions == ["ENABLE", "DISABLE", "RESET_PASSWORD", "SET_ROLE", "CREATE"], actions
    serialized = json.dumps(events)
    assert all(value not in serialized for value in (password, new_password, token_initial, token_reset))
    assert all(set(event[field] or {}) <= {"userId", "role", "enabled"}
               for event in events for field in ("before", "after"))
    print("PASS M4 account creation, scoped login, controlled roles, reset, disable/enable and secret-free audit", flush=True)
    return identifier, token_enabled, (token_initial, token_approver, token_reset, *raced_tokens)


def test_contract_and_pay(base, admin, viewer, other, own, identifier):
    detail = call(base, f"/employees/{identifier}", admin)
    update = {key: detail[key] for key in ("employeeNo", "name", "birthDate", "hireDate", "departmentId", "version")}
    raised = call(base, f"/employees/{identifier}", admin, "PUT",
                  dict(update, salaryCents=detail["salaryCents"] + 500, reason=REASON))
    events = audits(base, admin, f"?employeeId={identifier}&type=SALARY")
    assert events[0]["before"]["salaryCents"] == detail["salaryCents"]
    assert events[0]["after"]["salaryCents"] == raised["salaryCents"]
    assert audits(base, viewer, f"?employeeId={identifier}&type=SALARY") == []
    assert all(event["type"] != "SALARY" for event in audits(base, viewer, f"?employeeId={identifier}"))
    assert audits(base, other, f"?employeeId={identifier}") == []
    assert audits(base, own, f"?employeeId={identifier}&type=SALARY")[0]["after"] == events[0]["after"]
    note = "年假备注不应进入审计 " + intent()[:12]
    leave_route = f"/employees/{identifier}/annual-leave/2026"
    leave = {"version": 0, "entitledHalfDays": 18, "usedHalfDays": 5, "note": note, "reason": REASON}
    call(base, leave_route, admin, "PUT", leave)
    call(base, leave_route, admin, "PUT", leave, 409, "STALE_RECORD")
    leave_events = audits(base, viewer, f"?employeeId={identifier}&type=ANNUAL_LEAVE")
    assert len(leave_events) == 1 and leave_events[0]["reason"] == ""
    assert audits(base, admin, f"?employeeId={identifier}&type=ANNUAL_LEAVE")[0]["reason"] == REASON
    assert leave_events[0]["after"] == {"year": 2026, "entitledHalfDays": 18, "usedHalfDays": 5}

    suffix = intent()[:12]
    payload = {"contractNo": "M4-" + suffix, "status": "DRAFT", "startDate": "2025-01-01",
               "endDate": "2027-12-31", "reason": REASON}
    contract = call(base, f"/employees/{identifier}/contracts", admin, "POST", payload, 201)
    route = f"/contracts/{contract['id']}"
    call(base, route, other, status=404)
    pdf = fixture_pdf()
    attachment = upload(base, contract["id"], admin, "m4-fixture.pdf", pdf)
    edit = {key: payload[key] for key in ("contractNo", "startDate", "endDate", "reason")}
    call(base, route, admin, "PUT", dict(edit, version=contract["version"], endDate="2028-12-31"))
    current = call(base, route, admin)
    assert current["version"] == contract["version"] + 1
    sign = {"action": "SIGN", "version": current["version"], "reason": REASON, "effectiveDate": TODAY}
    replies = race(
        lambda: send(base, route + "/actions", admin, "POST", sign),
        lambda: send(base, route + "/actions", admin, "POST", sign),
    )
    assert sorted(reply.status for reply in replies) == [200, 409]
    signed = next(reply.data for reply in replies if reply.status == 200)
    assert signed["status"] == "SIGNED" and signed["signedDate"] == TODAY
    assert signed["version"] == current["version"] + 1
    denied = next(reply.data for reply in replies if reply.status == 409)
    assert denied["errorCode"] == "STALE_RECORD"
    call(base, route, admin, "PUT", dict(edit, version=signed["version"]), 409, "INVALID_STATE")
    actions = audits(base, viewer, f"?contractId={contract['id']}&type=CONTRACT")
    assert [item["action"] for item in actions] == ["SIGN", "UPDATE", "CREATE"]
    assert actions[0]["before"]["status"] == "DRAFT" and actions[0]["after"]["status"] == "SIGNED"
    terminated = call(base, route + "/actions", admin, "POST",
                      dict(sign, action="TERMINATE", version=signed["version"]))
    assert terminated["status"] == "TERMINATED" and terminated["terminatedDate"] == TODAY
    renewed = call(base, f"/employees/{identifier}/contracts", admin, "POST",
                   dict(payload, contractNo="M4-R-" + suffix, previousContractId=contract["id"]), 201)
    assert renewed["previousContractId"] == contract["id"]
    assert call(base, route, viewer)["files"][0]["id"] == attachment["id"]
    assert download(base, attachment["id"], viewer) == hashlib.sha256(pdf).hexdigest()
    due = call(base, f"/employees/{identifier}/contracts", admin, "POST",
               dict(payload, contractNo="M4-DUE-" + suffix, endDate=TODAY, status="SIGNED"), 201)
    for index in range(20):
        call(base, f"/employees/{identifier}/contracts", admin, "POST",
             dict(payload, contractNo=f"M4-PG-{suffix}-{index}"), 201)
    page1 = call(base, f"/employees/{identifier}/contracts?page=1&pageSize=20", admin)
    assert all(row["id"] != due["id"] for row in page1["list"])
    assert call(base, f"/contracts/{due['id']}", viewer)["id"] == due["id"]
    print("PASS M4 salary audit projection, contract race/state/history, attachment, direct detail beyond page one", flush=True)
    return {"id": contract["id"], "terminated": terminated["terminatedDate"], "employeeId": identifier,
            "due": {"id": due["id"], "contractNo": due["contractNo"]}}


def isolated_compose():
    project = os.environ["OA_M4_QA_PROJECT"]
    assert project.startswith("hr-oa-m4-qa-")
    return ["docker", "compose", "--project-name", project, "-f", "deploy/oa/restore.compose.yaml"]


def isolated_mysql(sql):
    output = subprocess.run(
        [*isolated_compose(), "exec", "-T", "mysql", "sh", "-ec",
         'test "$MYSQL_DATABASE" = hr_oa_restore; MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"'],
        input=sql, text=True, capture_output=True, timeout=20,
    )
    if output.returncode:
        raise RuntimeError("isolated M4 fixture SQL failed")
    return output.stdout.strip()


def test_audit_rollback(base, admin, identifier):
    payload = {"contractNo": "M4-RB-" + intent()[:12], "status": "DRAFT", "startDate": "2025-01-01",
               "endDate": "2027-12-31", "reason": REASON}
    contract = call(base, f"/employees/{identifier}/contracts", admin, "POST", payload, 201)
    contract_id = int(contract["id"])
    name = "m4_qa_audit_" + intent()[:10]
    route = f"/contracts/{contract_id}"
    before = audits(base, admin, f"?contractId={contract_id}")

    def root_sql(sql):
        result = subprocess.run(
            [*isolated_compose(), "exec", "-T", "mysql", "sh", "-ec",
             'test "$MYSQL_DATABASE" = hr_oa_restore; MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch -u root "$MYSQL_DATABASE"'],
            input=sql, text=True, capture_output=True, timeout=20,
        )
        if result.returncode:
            raise RuntimeError("scoped M4 audit-trigger operation failed")

    root_sql(f"""
DELIMITER //
CREATE TRIGGER {name} AFTER INSERT ON oa_audit_events FOR EACH ROW
BEGIN
    IF NEW.contract_id = {contract_id} THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'isolated QA audit failure';
    END IF;
END//
DELIMITER ;
""")
    mutation = {"action": "SIGN", "version": contract["version"], "reason": REASON, "effectiveDate": TODAY}
    try:
        call(base, route + "/actions", admin, "POST", mutation, 500)
        assert call(base, route, admin) == contract
        assert audits(base, admin, f"?contractId={contract_id}") == before
    finally:
        root_sql(f"DROP TRIGGER {name}")
    signed = call(base, route + "/actions", admin, "POST", mutation)
    assert signed["status"] == "SIGNED"
    assert len(audits(base, admin, f"?contractId={contract_id}")) == len(before) + 1
    print("PASS isolated M4 failed audit INSERT rolls back contract and version; same version then succeeds", flush=True)


def test_binding(base, admin):
    previous = employee(base, admin, "模拟解绑来源")
    selected = employee(base, admin, "模拟重新绑定")
    username, password = "m4-bind-" + intent()[:12], secrets.token_hex(20)
    created = account_action(base, admin, previous["id"], "CREATE", username=username,
                             password=password, role="EMPLOYEE")
    user_id = created["account"]["id"]
    old_token = login_password(base, username, password)["token"]
    isolated_mysql(f"UPDATE oa_employees SET user_id=NULL WHERE id={int(previous['id'])} AND user_id={int(user_id)}")
    candidates = page_all(base, f"/employees/{selected['id']}/account-candidates", admin)
    assert any(row["id"] == user_id and row["role"] == "EMPLOYEE" for row in candidates)
    bound = account_action(base, admin, selected["id"], "BIND", userId=user_id)
    assert bound["account"]["id"] == user_id and bound["account"]["role"] == "EMPLOYEE"
    call(base, "/auth/me", old_token, status=401)
    assert login_password(base, username, password)["user"]["employeeId"] == selected["id"]
    account_action(base, admin, previous["id"], "BIND", userId=user_id, status=409, error="ACCOUNT_CONFLICT")
    print("PASS isolated M4 unbound candidate, unique binding, previous token revoked", flush=True)


def wait_health(base):
    from oa_api import request
    from urllib.error import URLError
    for _ in range(60):
        try:
            status, _, _ = request(base, "/api/oa/v1/health")
            if status == 200:
                return
        except (URLError, OSError):
            pass
        time.sleep(1)
    raise AssertionError("isolated API failed to recover")


def test_handoff(base, admin, employee_token, manager, finance, successor_id, successor_token):
    finance_employee = finance["user"]["employeeId"]
    finance_user = finance["user"]["id"]
    current = workflow(base, "/requests", employee_token, payload=general("M4 当前待办交接"), key=intent(), status=201)
    first = current["tasks"][0]
    current = workflow(base, action_route(first), manager["token"], payload=action(first), key=intent())
    assert current["tasks"][1]["assigneeId"] == finance_user
    future = workflow(base, "/requests", employee_token, payload=general("M4 未来节点交接"), key=intent(), status=201)
    conflict = workflow(base, "/requests", successor_token, payload=general("M4 交接自审冲突"), key=intent(), status=201)
    target = call(base, "/auth/me", successor_token)["user"]["id"]
    assert target in {item["id"] for item in page_all(base, f"/employees/{finance_employee}/handoff-targets", admin)}
    before = account(base, admin, finance_employee)
    old_audit = audits(base, admin, f"?employeeId={finance_employee}&type=EMPLOYMENT")
    account_action(base, admin, finance_employee, "OFFBOARD", status=409, error="HANDOFF_REQUIRED")
    account_action(base, admin, finance_employee, "OFFBOARD", handoffUserId=target,
                   status=409, error="HANDOFF_CONFLICT")
    assert account(base, admin, finance_employee) == before
    assert audits(base, admin, f"?employeeId={finance_employee}&type=EMPLOYMENT") == old_audit
    task = conflict["tasks"][0]
    workflow(base, action_route(task), manager["token"],
             payload=action(task, "REJECT", comment="模拟交接冲突处理"), key=intent())
    offboarded = account_action(base, admin, finance_employee, "OFFBOARD", handoffUserId=target)
    assert offboarded["employmentStatus"] == "OFFBOARDED" and not offboarded["account"]["enabled"]
    call(base, "/auth/me", finance["token"], status=401)
    account_action(base, admin, finance_employee, "ENABLE", status=409, error="INVALID_STATE")
    moved = request_detail(base, current["id"], employee_token)
    assert moved["route"] == current["route"]
    assert moved["tasks"][1]["assigneeId"] == target
    assert moved["tasks"][1]["version"] == current["tasks"][1]["version"] + 1
    assert any(event["action"] == "HANDOFF" for event in moved["history"])
    future_before = request_detail(base, future["id"], employee_token)
    assert future_before["route"] == future["route"]
    pending_handoff = [item for item in future_before["history"] if item["action"] == "HANDOFF"]
    assert any(item["taskId"] is None and item["targetUserId"] == target for item in pending_handoff)
    call(base, f"/requests/{future['id']}", successor_token, status=404)
    target_events = f"SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={int(future['id'])} AND recipient_id={int(target)}"
    assert isolated_mysql(target_events) == "0"
    first = future_before["tasks"][0]
    progressed = workflow(base, action_route(first), manager["token"], payload=action(first), key=intent())
    assert progressed["route"] == future["route"] and progressed["tasks"][1]["assigneeId"] == target
    assert isolated_mysql(target_events) == "1"
    assert request_detail(base, future["id"], successor_token)["id"] == future["id"]
    new = workflow(base, "/requests", employee_token, payload=general("M4 新申请交接路由"), key=intent(), status=201)
    assert new["route"][1]["assigneeId"] == target
    task = moved["tasks"][1]
    final = workflow(base, action_route(task), successor_token, payload=action(task), key=intent())
    assert final["status"] == "APPROVED"
    assert audits(base, admin, f"?employeeId={finance_employee}&type=EMPLOYMENT")[0]["action"] == "OFFBOARD"
    account_events = audits(base, admin, f"?employeeId={finance_employee}&type=ACCOUNT")
    assert account_events[0]["after"]["handoffUserId"] == target
    assert call(base, "/auth/me", successor_token)["user"]["employeeId"] == successor_id
    print("PASS isolated M4 offboarding rolls back conflicts; current/future/new routing transferred without snapshot rewrite", flush=True)
    return finance["token"]


def test_tree_and_filters(base, admin, viewer, employee_token, manager):
    parent = call(base, "/departments", admin, "POST", {"name": "模拟 M4 父 " + intent()[:8]}, 201)
    child = call(base, "/departments", admin, "POST", {"name": "模拟 M4 子 " + intent()[:8], "parentId": parent["id"]}, 201)
    employee(base, admin, "模拟组织人数", departmentId=child["id"])
    renamed = call(base, f"/departments/{child['id']}", admin, "PUT",
                   {"name": child["name"] + " 更名", "parentId": parent["id"], "version": child["version"], "reason": REASON})
    dept_events = [event for event in audits(base, viewer, f"?departmentId={child['id']}&type=DEPARTMENT")
                   if event["employeeId"] is None]
    assert dept_events[0]["before"]["name"] == child["name"] and dept_events[0]["after"]["name"] == renamed["name"]
    assert audits(base, employee_token, f"?departmentId={child['id']}&type=DEPARTMENT") == []
    child = renamed
    tree = call(base, "/departments/tree", viewer)
    assert call(base, "/departments/tree", admin) == tree
    nodes = {row["id"]: row for row in tree["list"]}
    assert nodes[parent["id"]]["employeeCount"] == 0
    assert nodes[child["id"]]["employeeCount"] == 1 and nodes[child["id"]]["parentId"] == parent["id"]
    call(base, "/departments/tree", employee_token, status=403)
    new = workflow(base, "/requests", employee_token, payload=general("M4 审批筛选"), key=intent(), status=201)
    assert new["id"] in {row["id"] for row in page_all(base, f"/requests?type=GENERAL&status=IN_REVIEW&fromDate={TODAY}&toDate={TODAY}", employee_token)}
    assert new["id"] not in {row["id"] for row in page_all(base, "/requests?type=LEAVE", employee_token)}
    assert new["id"] in {row["requestId"] for row in page_all(base, "/tasks?status=IN_REVIEW&type=GENERAL", manager)}
    assert page_all(base, "/tasks?status=APPROVED", manager) == []
    call(base, f"/tasks?fromDate={TODAY}&toDate=2000-01-01", manager, status=400)
    task = new["tasks"][0]
    workflow(base, action_route(task), manager, payload=action(task, "REJECT", comment="模拟筛选结束"), key=intent())
    assert new["id"] in {row["requestId"] for row in page_all(base, f"/tasks/history?status=REJECTED&fromDate={TODAY}", manager)}
    print("PASS M4 authorized direct organization counts and request/task/history filters", flush=True)
    return child, parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("live", "isolated"))
    parser.add_argument("--base", type=local_base, default=os.environ.get("OA_M4_QA_BASE", "http://127.0.0.1:8080"))
    parser.add_argument("--state", type=Path, default=Path("tests/results/m4/api-state.json"))
    args = parser.parse_args()
    if args.mode == "isolated":
        assert args.base == os.environ["OA_M4_QA_BASE"]
        isolated_compose()
    admin = login(args.base, "hr_admin", "HR_ADMIN")["token"]
    viewer = login(args.base, "hr_viewer", "HR_VIEWER")["token"]
    worker = login(args.base, "employee", "EMPLOYEE")["token"]
    manager = login(args.base, "manager", "APPROVER")
    identifier, token, expired = test_accounts(args.base, admin, viewer)
    if args.mode == "isolated":
        test_binding(args.base, admin)
        test_audit_rollback(args.base, admin, identifier)
        finance = login(args.base, "finance", "APPROVER")
        retired = test_handoff(args.base, admin, worker, manager, finance, identifier, token)
        subprocess.run([*isolated_compose(), "restart", "api"], check=True, timeout=45)
        address = subprocess.check_output([*isolated_compose(), "port", "api", "8888"], text=True, timeout=10).strip()
        restarted_base = local_base("http://" + address)
        print(f"Isolated API loopback before restart {args.base}; after restart {restarted_base}", flush=True)
        wait_health(restarted_base)
        for obsolete in (*expired, retired):
            call(restarted_base, "/auth/me", obsolete, status=401, error="UNAUTHENTICATED")
        assert call(restarted_base, "/auth/me", token)["user"]["employeeId"] == identifier
        print("PASS isolated M4 account revocation remains enforced after restart", flush=True)
    else:
        contract = test_contract_and_pay(args.base, admin, viewer, worker, token, identifier)
        department, parent = test_tree_and_filters(args.base, admin, viewer, worker, manager["token"])
        args.state.parent.mkdir(parents=True, exist_ok=True)
        args.state.write_text(json.dumps({"employeeId": identifier, "contract": contract, "department": department, "parent": parent},
                                         ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
