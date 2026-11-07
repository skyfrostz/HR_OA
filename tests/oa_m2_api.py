"""M2 API acceptance with real MySQL, transaction fault injection and persistence."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager
from dataclasses import dataclass
import json
import os
from pathlib import Path
import secrets
import subprocess
from threading import Barrier
import time
from urllib.error import HTTPError
from urllib.request import Request

from oa_api import OPENER, PREFIX, local_base, mysql


@dataclass
class Response:
    status: int
    headers: object
    body: bytes

    @property
    def data(self):
        envelope = json.loads(self.body)
        assert envelope["code"] == (0 if self.status < 400 else 7)
        return envelope["data"]


def send(base, route, token=None, payload=None, key=None):
    headers = {}
    if token:
        headers["x-token"] = token
    if key is not None:
        headers["Idempotency-Key"] = key
    content = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        content = json.dumps(payload, ensure_ascii=False).encode()
    request = Request(base + PREFIX + route, data=content, headers=headers)
    try:
        response = OPENER.open(request, timeout=30)
    except HTTPError as error:
        response = error
    with response:
        return Response(response.status, response.headers, response.read())


def expect(response, status, error=None):
    assert response.status == status, f"expected HTTP {status}, got {response.status}"
    data = response.data
    if error:
        assert data["errorCode"] == error, f"expected {error}, got {data['errorCode']}"
    return data


def call(base, route, token=None, *, payload=None, key=None, status=200, error=None):
    result = send(base, route, token, payload, key)
    assert result.status == status, f"{route}: expected HTTP {status}, got {result.status}"
    return expect(result, status, error)


def login(base, username, role):
    data = call(base, "/auth/login", payload={
        "username": username, "password": os.environ["OA_SEED_PASSWORD"],
    })
    assert data["user"]["role"] == role
    identity = call(base, "/auth/me", data["token"])
    assert identity["user"] == data["user"]
    assert identity["permissions"] == data["permissions"]
    return data


def intent():
    return secrets.token_hex(16)


def general(label):
    return {"type": "GENERAL", "title": label + " " + intent()[:10],
            "form": {"content": "模拟审批验收正文"}}


def race(*operations):
    barrier = Barrier(len(operations), timeout=5)

    def execute(operation):
        barrier.wait()
        return operation()

    with ThreadPoolExecutor(max_workers=len(operations)) as pool:
        futures = [pool.submit(execute, operation) for operation in operations]
        return [future.result() for future in futures]


def assert_replay(original, replay):
    assert original.status == replay.status
    assert original.body == replay.body, "replayed HTTP result body differs from committed result"
    assert replay.headers.get("Idempotency-Replayed") == "true"


def action_route(task):
    return f"/tasks/{task['id']}/actions"


def action(task, decision="APPROVE", **fields):
    return {"action": decision, "version": task["version"], "comment": "", **fields}


def request_detail(base, identifier, token):
    return call(base, f"/requests/{identifier}", token)


def list_items(base, route, token):
    return call(base, route + "?page=1&pageSize=100", token)["list"]


def accounts(base):
    roles = {
        "employee": "EMPLOYEE", "manager": "APPROVER", "finance": "APPROVER",
        "delegate": "APPROVER", "hr_admin": "HR_ADMIN", "hr_viewer": "HR_VIEWER",
    }
    users = {name: login(base, name, role) for name, role in roles.items()}
    for name, user in users.items():
        permissions = user["permissions"]
        assert permissions["canSubmitRequests"] is (name in ("employee", "manager", "finance", "delegate"))
        assert permissions["canApproveRequests"] is (roles[name] == "APPROVER")
    return users


@contextmanager
def disabled_finance(identifier):
    scope = f"id = {int(identifier)} AND username = 'finance' AND deleted_at IS NULL"
    assert mysql(f"SELECT enable FROM sys_users WHERE {scope}") == "1"
    try:
        assert mysql(f"UPDATE sys_users SET enable = 2 WHERE {scope}; SELECT ROW_COUNT()") == "1"
        yield
    finally:
        mysql(f"UPDATE sys_users SET enable = 1 WHERE {scope}")


def validate_templates(base, users):
    employee = users["employee"]["token"]
    templates = call(base, "/request-templates", employee)
    assert templates["integrationMode"] == "mock"
    assert {row["type"] for row in templates["list"]} == {"LEAVE", "EXPENSE", "GENERAL"}
    expected = [users[name]["user"]["id"] for name in ("manager", "finance")]
    for template in templates["list"]:
        assert template["version"] == 1
        assert [step["assigneeId"] for step in template["route"]] == expected
    for name in ("manager", "finance", "delegate"):
        approver = users[name]
        own = list_items(base, "/employees", approver["token"])
        assert [row["id"] for row in own] == [approver["user"]["employeeId"]]
        assert "salaryCents" in own[0]
        call(base, "/employees", approver["token"], payload={}, status=403)
    for name in ("hr_admin", "hr_viewer"):
        call(base, "/requests", users[name]["token"], payload=general("HR越权"), key=intent(), status=403)
    call(base, "/requests", employee, payload=general("缺少幂等键"), status=400)
    call(base, "/requests", employee, payload=general("无效幂等键"), key="invalid key", status=400)
    for payload in (
        {"type": "LEAVE", "title": "日期颠倒", "form": {"startDate": "2026-12-10", "endDate": "2026-12-01", "reason": "模拟"}},
        {"type": "EXPENSE", "title": "小数金额", "form": {"expenseDate": "2026-11-06", "amountCents": 1.5, "reason": "模拟"}},
        {"type": "EXPENSE", "title": "超限金额", "form": {"expenseDate": "2026-11-06", "amountCents": 2**53, "reason": "模拟"}},
        {"type": "GENERAL", "title": "篡改路由", "form": {"content": "模拟", "assigneeId": 1}},
    ):
        call(base, "/requests", employee, payload=payload, key=intent(), status=400)
    for payload in (
        {"type": "LEAVE", "title": general("合法请假")["title"],
         "form": {"startDate": "2026-12-01", "endDate": "2026-12-02", "reason": "模拟"}},
        {"type": "EXPENSE", "title": general("合法报销")["title"],
         "form": {"expenseDate": "2026-11-06", "amountCents": 2330, "reason": "模拟"}},
    ):
        created = call(base, "/requests", employee, payload=payload, key=intent(), status=201)
        assert created["form"] == payload["form"]
    print("PASS M2 role permissions, template routing and typed form validation", flush=True)


def duplicate(base, route, token, payload, key, status):
    results = race(*[lambda: send(base, route, token, payload, key) for _ in range(2)])
    committed = [row for row in results if row.status == status and row.headers.get("Idempotency-Replayed") != "true"]
    assert len(committed) == 1, "expected exactly one committed original intent"
    original = committed[0]
    for row in results:
        if row is original:
            continue
        if row.status == 409:
            expect(row, 409, "IDEMPOTENCY_IN_PROGRESS")
        else:
            assert_replay(original, row)
    assert_replay(original, send(base, route, token, payload, key))
    return original


def create_flow(base, users):
    employee, manager, finance, delegate, admin, viewer = (
        users[name]["token"] for name in ("employee", "manager", "finance", "delegate", "hr_admin", "hr_viewer")
    )
    applicant_id, manager_id, finance_id, delegate_id = (
        users[name]["user"]["id"] for name in ("employee", "manager", "finance", "delegate")
    )
    payload = general("幂等转交验收")
    key = intent()
    submitted = duplicate(base, "/requests", employee, payload, key, 201)
    record = submitted.data
    identifier = record["id"]
    assert record["status"] == "IN_REVIEW" and record["currentStep"] == 1
    assert record["applicantId"] == applicant_id and record["templateVersion"] == 1
    assert record["form"] == payload["form"]
    assert len(record["tasks"]) == 1 and record["tasks"][0]["version"] == 1
    assert [row["action"] for row in record["history"]] == ["SUBMIT"]
    assert [row["assigneeId"] for row in record["route"]] == [manager_id, finance_id]
    call(base, "/requests", employee, payload=dict(payload, title="不同意图"), key=key,
         status=422, error="IDEMPOTENCY_KEY_REUSED")
    assert sum(row["title"] == payload["title"] for row in list_items(base, "/requests", employee)) == 1
    assert all(row["applicantId"] == applicant_id for row in list_items(base, "/requests", employee))
    for token in (finance, delegate, admin, viewer):
        call(base, f"/requests/{identifier}", token, status=404)
    assert not any(row["requestId"] == identifier for row in list_items(base, "/tasks", finance))

    first = record["tasks"][0]
    route = action_route(first)
    targets_route = f"/tasks/{first['id']}/transfer-targets"
    targets = call(base, targets_route, manager)["list"]
    assert {row["id"] for row in targets} >= {finance_id, delegate_id}
    assert not {manager_id, applicant_id} & {row["id"] for row in targets}
    assert all(set(row) == {"id", "displayName"} for row in targets)
    call(base, route, finance, payload=action(first), key=intent(), status=403)
    call(base, route, manager, payload=action(first, "TRANSFER", targetUserId=delegate_id), key=intent(), status=400)
    moved_payload = action(first, "TRANSFER", targetUserId=delegate_id, comment="模拟转交")
    transfer = duplicate(base, route, manager, moved_payload, intent(), 200).data
    moved = transfer["tasks"][0]
    assert transfer["route"] == record["route"] and len(transfer["tasks"]) == 1
    assert (moved["id"], moved["assigneeId"], moved["status"], moved["version"]) == (
        first["id"], delegate_id, "PENDING", first["version"] + 1,
    )
    assert [row["action"] for row in transfer["history"]] == ["SUBMIT", "TRANSFER"]
    assert request_detail(base, identifier, manager)["id"] == identifier
    assert request_detail(base, identifier, delegate)["id"] == identifier
    assert not any(row["id"] == first["id"] for row in list_items(base, "/tasks", manager))
    call(base, targets_route, manager, status=403)
    call(base, route, manager, payload=action(moved), key=intent(), status=403)
    call(base, route, delegate, payload=action(first), key=intent(), status=409, error="STALE_TASK")

    approved = call(base, route, delegate, payload=action(moved), key=intent())
    assert approved["currentStep"] == 2 and len(approved["tasks"]) == 2
    last = approved["tasks"][1]
    assert last["assigneeId"] == finance_id and last["status"] == "PENDING"
    decision = action(last)
    decision_key = intent()
    competing_key = intent()
    competing = race(
        lambda: send(base, action_route(last), finance, decision, decision_key),
        lambda: send(base, action_route(last), finance, decision, competing_key),
    )
    assert sorted(row.status for row in competing) == [200, 409], "different keys both applied a decision"
    index = next(i for i, row in enumerate(competing) if row.status == 200)
    expect(competing[1 - index], 409, "STALE_TASK")
    result = competing[index]
    replay_key = (decision_key, competing_key)[index]
    assert_replay(result, send(base, action_route(last), finance, decision, replay_key))
    with disabled_finance(finance_id):
        call(base, action_route(last), finance, payload=decision, key=replay_key, status=401)
    finished = result.data
    assert finished["status"] == "APPROVED" and finished["currentStep"] is None
    assert [row["action"] for row in finished["history"]] == ["SUBMIT", "TRANSFER", "APPROVE", "APPROVE"]
    for actor, expected_action in ((manager, "TRANSFER"), (delegate, "APPROVE"), (finance, "APPROVE")):
        history = [row for row in list_items(base, "/tasks/history", actor) if row["requestId"] == identifier]
        assert [row["action"] for row in history] == [expected_action]
    print("PASS concurrent same-key submit/transfer, exact replay, cross-user scope, transfer and decision race", flush=True)
    return {"id": identifier, "final": finished, "decisionRoute": action_route(last),
            "decisionKey": replay_key, "decisionPayload": decision,
            "response": result.body.decode()}


def reject_and_resubmit(base, users):
    employee = users["employee"]["token"]
    manager = users["manager"]["token"]
    delegate = users["delegate"]["token"]
    record = call(base, "/requests", employee, payload=general("驳回验收"), key=intent(), status=201)
    first = record["tasks"][0]
    rejected = call(base, action_route(first), manager,
                    payload=action(first, "REJECT", comment="模拟补充说明"), key=intent())
    assert rejected["status"] == "REJECTED" and len(rejected["tasks"]) == 1
    assert [item["action"] for item in rejected["history"]] == ["SUBMIT", "REJECT"]
    previous = record["id"]
    cross = send(base, "/requests", delegate, dict(general("跨人重提"), previousRequestId=previous), intent())
    assert cross.status in (400, 404), f"cross-user resubmission expected rejection, got {cross.status}"
    assert cross.data["errorCode"]
    resubmitted = call(base, "/requests", employee, payload=dict(general("本人重提"), previousRequestId=previous),
                       key=intent(), status=201)
    assert resubmitted["id"] != previous and resubmitted["previousRequestId"] == previous
    assert resubmitted["status"] == "IN_REVIEW"
    assert request_detail(base, previous, employee) == rejected
    print("PASS rejected request preserved, cross-user resubmission denied, new linked record created", flush=True)
    return {"rejected": previous, "resubmitted": resubmitted["id"]}


def wait_for(check, description, timeout=90):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = check()
        if result is not None:
            return result
        time.sleep(0.5)
    raise AssertionError(f"timed out waiting for {description}")


def route_unavailable(base, users):
    employee = users["employee"]["token"]
    manager = users["manager"]["token"]
    record = call(base, "/requests", employee, payload=general("复核人不可用验收"), key=intent(), status=201)
    first = record["tasks"][0]
    key = intent()
    with disabled_finance(users["finance"]["user"]["id"]):
        call(base, action_route(first), manager, payload=action(first), key=key,
             status=409, error="ROUTE_UNAVAILABLE")
        assert request_detail(base, record["id"], employee) == record
        call(base, "/requests", employee, payload=general("路由不可用验收"), key=intent(),
             status=409, error="ROUTE_UNAVAILABLE")
    resumed = call(base, action_route(first), manager, payload=action(first), key=key)
    assert resumed["currentStep"] == 2
    print("PASS unavailable next approver rolls back transition; same key succeeds after restoration", flush=True)


def verify_notifications(base, users, identifier):
    expected = {
        "manager": ["TASK_ASSIGNED"], "delegate": ["TASK_ASSIGNED"], "finance": ["TASK_ASSIGNED"],
        "employee": ["REQUEST_APPROVED"], "hr_admin": [], "hr_viewer": [],
    }

    def inboxes():
        events = []
        for username, kinds in expected.items():
            rows = [row for row in list_items(base, "/notifications", users[username]["token"])
                    if row["requestId"] == identifier]
            if sorted(row["kind"] for row in rows) != kinds:
                return None
            for row in rows:
                assert row["deliveryMode"] == "mock"
                assert row["title"] and "模拟审批验收正文" not in row["title"]
                assert set(row) == {"id", "eventId", "requestId", "kind", "title", "createdAt", "deliveryMode"}
                events.append(row["eventId"])
        assert len(events) == len(set(events)) == 4
        return events

    events = wait_for(inboxes, "same four outbox events to reach their mock inboxes after restart")
    time.sleep(1)
    assert inboxes() == events
    print("PASS pending mock events delivered once to correct recipients after restart", flush=True)
    return events


def mysql_admin(sql):
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "mysql", "sh", "-ec",
         'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch --skip-column-names -u root "$MYSQL_DATABASE"'],
        input=sql, text=True, capture_output=True, timeout=20,
    )
    if result.returncode:
        raise RuntimeError("failed to apply or remove scoped development MySQL trigger")
    return result.stdout.strip()


@contextmanager
def fail_outbox_insert(identifier):
    name = "qa_outbox_reject_" + intent()[:12]
    # Only an outbox row for this test's new request can hit the trigger.
    mysql_admin(f"""
DELIMITER //
CREATE TRIGGER {name} AFTER INSERT ON oa_notification_outbox FOR EACH ROW
BEGIN
    IF NEW.request_id = {int(identifier)} THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'local M2 QA injected outbox failure';
    END IF;
END//
DELIMITER ;
""")
    try:
        yield
    finally:
        mysql_admin(f"DROP TRIGGER {name}")


def verify_atomic_rollback(base, users):
    employee = users["employee"]["token"]
    manager = users["manager"]["token"]
    record = call(base, "/requests", employee, payload=general("事务回滚验收"), key=intent(), status=201)
    identifier = int(record["id"])
    first = record["tasks"][0]
    route, body, key = action_route(first), action(first), intent()

    def counts():
        return mysql(f"""
SELECT (SELECT COUNT(*) FROM oa_tasks WHERE request_id={identifier}),
       (SELECT COUNT(*) FROM oa_request_actions WHERE request_id={identifier}),
       (SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={identifier}),
       (SELECT COUNT(*) FROM oa_intents WHERE intent_key='{key}');
""")

    assert counts() == "1\t1\t1\t0"
    with fail_outbox_insert(identifier):
        call(base, route, manager, payload=body, key=key, status=500)
        assert request_detail(base, identifier, employee) == record
        assert counts() == "1\t1\t1\t0"
    resumed = call(base, route, manager, payload=body, key=key)
    assert resumed["currentStep"] == 2 and counts() == "2\t2\t2\t1"
    print("PASS injected outbox INSERT failure rolls back task, audit, outbox and intent; retry succeeds", flush=True)


def event_rows(identifier):
    raw = mysql(f"""
SELECT id, attempts, delivered_at IS NULL FROM oa_notification_outbox
WHERE request_id={int(identifier)} ORDER BY id;
""")
    return [tuple(map(int, row.split("\t"))) for row in raw.splitlines()]


def confirm_pending_failures(base, users, identifier):
    def failures():
        rows = event_rows(identifier)
        assert len(rows) == 4 and all(pending == 1 for _, _, pending in rows)
        return rows if all(attempts >= 1 for _, attempts, _ in rows) else None

    events = wait_for(failures, "persistent mock delivery failures for each outbox event")
    assert mysql(f"SELECT COUNT(*) FROM oa_mock_notifications WHERE request_id={int(identifier)}") == "0"
    for user in users.values():
        assert not any(row["requestId"] == identifier for row in list_items(base, "/notifications", user["token"]))
    assert request_detail(base, identifier, users["employee"]["token"])["status"] == "APPROVED"
    print("PASS mock failure increments persistent attempts and leaves approval committed", flush=True)
    return events


def create(base):
    users = accounts(base)
    validate_templates(base, users)
    flow = create_flow(base, users)
    resubmission = reject_and_resubmit(base, users)
    route_unavailable(base, users)
    verify_atomic_rollback(base, users)
    flow["events"] = confirm_pending_failures(base, users, flow["id"])
    return {"flow": flow, "resubmission": resubmission}


def verify(base, state):
    users = accounts(base)
    employee = users["employee"]["token"]
    finance = users["finance"]["token"]
    flow = state["flow"]
    identifier = flow["id"]
    assert request_detail(base, identifier, employee) == flow["final"]
    replay = send(base, flow["decisionRoute"], finance, flow["decisionPayload"], flow["decisionKey"])
    assert replay.status == 200 and replay.headers.get("Idempotency-Replayed") == "true"
    assert replay.body.decode() == flow["response"]
    assert request_detail(base, identifier, employee) == flow["final"]
    rejected = request_detail(base, state["resubmission"]["rejected"], employee)
    resubmitted = request_detail(base, state["resubmission"]["resubmitted"], employee)
    assert rejected["status"] == "REJECTED" and resubmitted["previousRequestId"] == rejected["id"]
    print("PASS restart preserves workflow, audit, linkage, snapshots and exact decision replay", flush=True)
    received = verify_notifications(base, users, identifier)
    before = {event_id: attempts for event_id, attempts, _ in flow["events"]}
    after = event_rows(identifier)
    assert sorted(received) == sorted(before) == sorted(event_id for event_id, _, _ in after)
    assert all(pending == 0 and attempts >= before[event_id] for event_id, attempts, pending in after)
    assert mysql(f"SELECT COUNT(*) FROM oa_mock_notifications WHERE request_id={int(identifier)}") == "4"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("create", "verify"))
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/m2/oa-m2-state.json"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    assert call(args.base, "/health")["database"] == "ok"
    if args.mode == "create":
        result = create(args.base)
        args.state.parent.mkdir(parents=True, exist_ok=True)
        args.state.write_text(json.dumps(result, ensure_ascii=False) + "\n")
    else:
        verify(args.base, json.loads(args.state.read_text()))


if __name__ == "__main__":
    main()
