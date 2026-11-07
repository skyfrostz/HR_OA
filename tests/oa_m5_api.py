"""M5 isolated MySQL acceptance. Never run fixture writes against the main stack."""
from datetime import date, timedelta
from calendar import isleap
import json
import os
import re
import secrets
import subprocess

from oa_api import local_base
from oa_m2_api import action, action_route, assert_replay, call as post, general, intent, login, race, request_detail, send as send_post
from oa_m3_api import call, page_all, send
from oa_m4_api import account_action, employee as create_employee, login_password


def isolated_sql(sql, *, root=False):
    project = os.environ.get("OA_QA_PROJECT", "")
    assert re.fullmatch(r"hr-oa-m5-qa-[0-9a-f]{12}", project)
    mysql = ('MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch --skip-column-names -u root "$MYSQL_DATABASE"'
             if root else 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"')
    result = subprocess.run(
        ["docker", "compose", "--project-name", project, "-f", "deploy/oa/restore.compose.yaml",
         "exec", "-T", "mysql", "sh", "-ec",
         'test "$MYSQL_DATABASE" = hr_oa_restore; ' + mysql],
        input=sql, text=True, capture_output=True, timeout=20,
    )
    if result.returncode:
        diagnostic = re.search(r"ERROR [0-9]+ \([A-Z0-9]+\)", result.stderr)
        raise RuntimeError("isolated M5 fixture SQL failed: " + (diagnostic[0] if diagnostic else str(result.returncode)))
    return result.stdout.strip()


def approve_through_fault(base, requester, approver, request_id, annual_read, ledger_read):
    request_id = int(request_id)
    before = request_detail(base, request_id, requester)
    task = before["tasks"][-1]
    annual_before, ledger_before = annual_read(), ledger_read()
    notifications = f"SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={request_id};"
    outbox_before = isolated_sql(notifications)
    key = intent()
    trigger = "m5_qa_ledger_" + intent()[:8]
    isolated_sql(f"""
DELIMITER //
CREATE TRIGGER {trigger} AFTER INSERT ON oa_leave_ledger FOR EACH ROW
BEGIN
    IF NEW.request_id = {request_id} AND NEW.kind = 'CONSUME' THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'isolated QA ledger failure';
    END IF;
END//
DELIMITER ;
""", root=True)
    try:
        post(base, action_route(task), approver, payload=action(task), key=key, status=500)
        assert request_detail(base, request_id, requester) == before
        assert annual_read() == annual_before and ledger_read() == ledger_before
        assert isolated_sql(notifications) == outbox_before
    finally:
        isolated_sql(f"DROP TRIGGER {trigger};", root=True)
    post(base, action_route(task), approver, payload=action(task), key=key)
    print("PASS M5 ledger insert failure atomically rolls back request, task, balance and outbox, then retry succeeds")


def accept_legacy(base, applicant, approvers):
    token, user_id = applicant["token"], int(applicant["user"]["id"])
    employee_id = applicant["user"]["employeeId"]
    old_requests = page_all(base, "/requests?type=LEAVE", token)
    row = next(item for item in old_requests if item["title"].startswith("合法请假 "))
    request_id = int(row["id"])
    old = request_detail(base, request_id, token)
    assert old["leaveCalculation"] is None
    old_payload = {"type": "LEAVE", "title": old["title"], "form": old["form"]}
    post(base, "/requests", token, payload=old_payload, key=intent(), status=400, error="INVALID_INPUT")
    saved_key, saved_hex = isolated_sql(f"""
        SELECT intent_key, HEX(result) FROM oa_intents WHERE actor_id = {user_id} AND result_status = 201
        AND JSON_EXTRACT(CONVERT(result USING utf8mb4), '$.id') = {request_id} LIMIT 1;
    """).split("\t")
    assert re.fullmatch(r"[0-9a-f]{32}", saved_key)
    replay = send_post(base, "/requests", token, old_payload, saved_key)
    assert replay.status == 201 and replay.headers.get("Idempotency-Replayed") == "true"
    assert replay.data == json.loads(bytes.fromhex(saved_hex))
    before = page_all(base, f"/employees/{employee_id}/annual-leave", token)
    while old["status"] == "IN_REVIEW":
        task, = [task for task in old["tasks"] if task["status"] == "PENDING"]
        assignee = approvers[task["assigneeId"]]
        assert send_post(base, action_route(task), assignee, action(task), intent()).status == 200
        old = request_detail(base, request_id, token)
    assert old["leaveCalculation"] is None
    assert page_all(base, f"/employees/{employee_id}/annual-leave", token) == before
    assert page_all(base, f"/employees/{employee_id}/leave-ledger", token) == []
    print("PASS M5 old LEAVE intent replays, fresh legacy payload is rejected, old approval leaves ledger untouched")


def date_range():
    """Pick a year boundary with a working day on each side and a later weekend."""
    year = date.today().year
    while date(year, 12, 31).isoweekday() not in (1, 2, 3, 4):
        year += 1
    return date(year, 12, 31), date(year + 1, 1, 1)


def annual(base, admin, employee_id, year):
    rows = page_all(base, f"/employees/{employee_id}/annual-leave", admin)
    return next((row for row in rows if row["year"] == year), None)


def counters(row):
    return tuple(row[key] for key in (
        "usedHalfDays", "reservedHalfDays", "approvalUsedHalfDays", "remainingHalfDays"
    ))


def workflow_action(base, token, request_id, verb, reason="M5 模拟验收"):
    return send_post(base, f"/requests/{request_id}/actions", token,
                     {"action": verb, "reason": reason}, intent())


def accept_leave(base, admin, applicant, manager, finance):
    token, employee_id = applicant["token"], applicant["user"]["employeeId"]
    last, first = date_range()
    leave_type, = [item for item in call(base, "/leave-types", token)["list"] if item["usesBalance"]]
    calendar = call(base, "/leave-calendar", admin)
    assert calendar["workingWeekdays"] == [1, 2, 3, 4, 5]
    saturday = first + timedelta(days=6 - first.isoweekday())
    sunday = saturday + timedelta(days=1)
    single = {"leaveTypeId": leave_type["id"], "startDate": str(saturday), "endDate": str(saturday),
              "startHalf": "AM", "endHalf": "AM"}
    post(base, "/leave-preview", token, payload=single, status=400, error="INVALID_INPUT")
    updated_calendar = call(base, "/leave-calendar", admin, "PUT", dict(calendar, exceptions=[
        {"date": str(saturday), "working": True},
        {"date": str(sunday + timedelta(days=1)), "working": False},
    ]))
    assert updated_calendar["version"] > calendar["version"]
    half = post(base, "/leave-preview", token, payload=single)["calculation"]
    assert half["totalHalfDays"] == 1 and half["days"] == [{"date": str(saturday), "halves": ["AM"]}]
    post(base, "/leave-preview", token, payload=dict(single, startHalf="PM"),
         status=400, error="INVALID_INPUT")
    rest = str(sunday + timedelta(days=1))
    post(base, "/leave-preview", token, payload=dict(single, startDate=rest, endDate=rest),
         status=400, error="INVALID_INPUT")

    for year, entitlement in ((last.year, 3), (first.year, 1)):
        previous = annual(base, admin, employee_id, year)
        call(base, f"/employees/{employee_id}/annual-leave/{year}", admin, "PUT", {
            "version": previous["version"] if previous else 0,
            "entitledHalfDays": entitlement, "usedHalfDays": 1, "note": "M5 模拟额度", "reason": "M5 模拟",
        })
    cross = dict(single, startDate=str(last), endDate=str(first), startHalf="PM", endHalf="AM")
    preview = post(base, "/leave-preview", token, payload=cross)
    calculation = preview["calculation"]
    assert calculation["totalHalfDays"] == 2
    assert calculation["years"] == [{"year": last.year, "halfDays": 1}, {"year": first.year, "halfDays": 1}]
    assert calculation["days"] == [{"date": str(last), "halves": ["PM"]}, {"date": str(first), "halves": ["AM"]}]
    assert not preview["canSubmit"] and "INSUFFICIENT_LEAVE" in preview["issues"]
    body = {"type": "LEAVE", "title": "M5 跨年 " + intent()[:8], "form": dict(cross, reason="模拟跨年",
            leaveTypeVersion=calculation["leaveTypeVersion"], calendarVersion=calculation["calendarVersion"])}
    before = counters(annual(base, admin, employee_id, last.year))
    post(base, "/requests", token, payload=body, key=intent(), status=409, error="INSUFFICIENT_LEAVE")
    assert counters(annual(base, admin, employee_id, last.year)) == before
    assert page_all(base, f"/employees/{employee_id}/leave-ledger", token) == []
    current = annual(base, admin, employee_id, first.year)
    call(base, f"/employees/{employee_id}/annual-leave/{first.year}", admin, "PUT", {
        "version": current["version"], "entitledHalfDays": 3, "usedHalfDays": 1,
        "note": "M5 模拟额度", "reason": "M5 模拟",
    })

    key = intent()
    created = send_post(base, "/requests", token, body, key)
    assert created.status == 201
    assert_replay(created, send_post(base, "/requests", token, body, key))
    request_id = created.data["id"]
    snapshot = request_detail(base, request_id, token)["leaveCalculation"]
    assert snapshot == calculation
    post(base, "/requests", token, payload=body, key=intent(), status=409, error="LEAVE_OVERLAP")
    for year in (last.year, first.year):
        assert counters(annual(base, admin, employee_id, year)) == (1, 1, 0, 1)
    current = annual(base, admin, employee_id, first.year)
    call(base, f"/employees/{employee_id}/annual-leave/{first.year}", admin, "PUT", {
        "version": current["version"], "entitledHalfDays": 1, "usedHalfDays": 1,
        "note": "不能覆盖预占", "reason": "M5 模拟",
    }, status=409, error="INSUFFICIENT_LEAVE")

    call(base, "/leave-calendar", admin, "PUT", dict(updated_calendar, exceptions=[
        {"date": str(last), "working": False},
    ]))
    post(base, "/requests", token, payload=body, key=intent(), status=409, error="STALE_RECORD")
    assert request_detail(base, request_id, token)["leaveCalculation"] == snapshot
    remind_key = intent()
    remind = {"action": "REMIND"}
    route = f"/requests/{request_id}/actions"
    reminder = send_post(base, route, token, remind, remind_key)
    assert reminder.status == 200
    assert_replay(reminder, send_post(base, route, token, remind, remind_key))
    post(base, route, token, payload=remind, key=intent(), status=429, error="REMINDER_RATE_LIMIT")
    post(base, route, finance, payload=remind, key=intent(), status=404)

    task = request_detail(base, request_id, token)["tasks"][0]
    recipients = isolated_sql(f"SELECT recipient_id FROM oa_notification_outbox WHERE request_id={int(request_id)} AND kind='TASK_REMINDER';")
    assert recipients == str(task["assigneeId"])
    assert send_post(base, action_route(task), manager, action(task), intent()).status == 200
    task = request_detail(base, request_id, token)["tasks"][-1]
    outcomes = race(lambda: send_post(base, action_route(task), finance, action(task), intent()),
                    lambda: workflow_action(base, token, request_id, "WITHDRAW"))
    assert sorted(item.status for item in outcomes) == [200, 409]
    result = request_detail(base, request_id, token)
    if result["status"] == "APPROVED":
        assert request_id in {row["id"] for row in page_all(base, "/approved-leaves", admin)}
        cancel = {"action": "CANCEL", "reason": "M5 模拟撤销"}
        cancel_key = intent()
        cancelled = send_post(base, route, admin, cancel, cancel_key)
        assert cancelled.status == 200
        assert_replay(cancelled, send_post(base, route, admin, cancel, cancel_key))
        post(base, route, admin, payload=cancel, key=intent(), status=409)
        expected_kinds = ["REVERSE", "CONSUME", "RESERVE"]
    else:
        assert result["status"] == "WITHDRAWN"
        expected_kinds = ["RELEASE", "RESERVE"]
    for year in (last.year, first.year):
        assert counters(annual(base, admin, employee_id, year)) == (1, 0, 0, 2)
        entries = page_all(base, f"/employees/{employee_id}/leave-ledger?year={year}", token)
        assert [(row["kind"], row["halfDays"], row["requestId"]) for row in entries] == [
            (kind, 1, request_id) for kind in expected_kinds
        ]
    print("PASS M5 calendar, cross-year rollback and snapshot, scoped reminders, concurrent approval/withdrawal, balanced ledger")


def accept_half_day_contention(base, admin, applicant, manager, finance):
    token, employee_id = applicant["token"], applicant["user"]["employeeId"]
    _, day = date_range()
    leave_type, = [item for item in call(base, "/leave-types", token)["list"] if item["usesBalance"]]
    annual_route = f"/employees/{employee_id}/annual-leave/{day.year}"

    def set_quota(remaining):
        row = annual(base, admin, employee_id, day.year)
        return call(base, annual_route, admin, "PUT", {
            "version": row["version"], "entitledHalfDays": 1 + remaining, "usedHalfDays": 1,
            "note": "M5 模拟半天额度", "reason": "M5 并发验收",
        })

    def application(half):
        form = {"leaveTypeId": leave_type["id"], "startDate": str(day), "endDate": str(day),
                "startHalf": half, "endHalf": half}
        preview = post(base, "/leave-preview", token, payload=form)["calculation"]
        form.update(leaveTypeVersion=preview["leaveTypeVersion"], calendarVersion=preview["calendarVersion"], reason="模拟半天")
        return {"type": "LEAVE", "title": "M5 半天 " + intent()[:8], "form": form}

    set_quota(1)
    first, second = application("AM"), application("PM")
    results = race(lambda: send_post(base, "/requests", token, first, intent()),
                   lambda: send_post(base, "/requests", token, second, intent()))
    assert sorted(result.status for result in results) == [201, 409]
    winner, loser = sorted(results, key=lambda result: result.status)
    assert loser.data["errorCode"] == "INSUFFICIENT_LEAVE"
    assert counters(annual(base, admin, employee_id, day.year)) == (1, 1, 0, 0)
    set_quota(2)
    other = second if results[0] is winner else first
    disjoint = post(base, "/requests", token, payload=other, key=intent(), status=201)
    assert counters(annual(base, admin, employee_id, day.year)) == (1, 2, 0, 0)

    rejected = request_detail(base, winner.data["id"], token)
    task = rejected["tasks"][0]
    post(base, action_route(task), manager, payload=action(task, "REJECT", comment="M5 模拟驳回"), key=intent())
    assert counters(annual(base, admin, employee_id, day.year)) == (1, 1, 0, 1)
    disjoint_id = disjoint["id"]
    task = request_detail(base, disjoint_id, token)["tasks"][-1]
    post(base, action_route(task), manager, payload=action(task), key=intent())
    approve_through_fault(base, token, finance, disjoint_id,
                          lambda: annual(base, admin, employee_id, day.year),
                          lambda: page_all(base, f"/employees/{employee_id}/leave-ledger?year={day.year}", token))
    assert counters(annual(base, admin, employee_id, day.year)) == (1, 0, 1, 1)
    route = f"/requests/{disjoint_id}/actions"
    payload = {"action": "CANCEL", "reason": "M5 模拟并发冲回"}
    first_key, second_key = intent(), intent()
    outcomes = race(lambda: send_post(base, route, admin, payload, first_key),
                    lambda: send_post(base, route, admin, payload, second_key))
    assert sorted(reply.status for reply in outcomes) == [200, 409]
    for reply, key in zip(outcomes, (first_key, second_key)):
        if reply.status == 200:
            assert_replay(reply, send_post(base, route, admin, payload, key))
    assert counters(annual(base, admin, employee_id, day.year)) == (1, 0, 0, 2)
    rows = page_all(base, f"/employees/{employee_id}/leave-ledger?year={day.year}", token)
    assert [row["kind"] for row in rows if row["requestId"] == disjoint_id] == ["REVERSE", "CONSUME", "RESERVE"]
    assert [row["kind"] for row in rows if row["requestId"] == rejected["id"]] == ["RELEASE", "RESERVE"]

    same_slot = application("AM")
    outcomes = race(*[lambda: send_post(base, "/requests", token, same_slot, intent()) for _ in range(2)])
    assert sorted(reply.status for reply in outcomes) == [201, 409]
    winner, loser = sorted(outcomes, key=lambda reply: reply.status)
    assert loser.data["errorCode"] == "LEAVE_OVERLAP"
    assert workflow_action(base, token, winner.data["id"], "WITHDRAW").status == 200

    row = set_quota(1)
    last_slot = application("PM")
    reserve, reduce = race(
        lambda: send_post(base, "/requests", token, last_slot, intent()),
        lambda: send(base, annual_route, admin, "PUT", {
            "version": row["version"], "entitledHalfDays": 1, "usedHalfDays": 1,
            "note": "M5 模拟减少最后额度", "reason": "M5 并发验收",
        }),
    )
    assert (reserve.status, reduce.status) in ((201, 409), (409, 200))
    assert annual(base, admin, employee_id, day.year)["remainingHalfDays"] == 0
    if reserve.status == 201:
        assert workflow_action(base, token, reserve.data["id"], "WITHDRAW").status == 200
    else:
        assert reserve.data["errorCode"] == "INSUFFICIENT_LEAVE"
    print("PASS M5 disjoint halves, same-slot contention, manual quota race, rejection release and single reversal")


def accept_nonbalance_leap_day(base, admin, applicant):
    token, employee_id = applicant["token"], applicant["user"]["employeeId"]
    year = next(year for year in range(date.today().year, date.today().year + 30)
                if isleap(year) and date(year, 2, 29).isoweekday() <= 5)
    day = date(year, 2, 29).isoformat()
    name = "M5 模拟非额度类型 " + intent()[:8]
    kind = post(base, "/leave-types", admin, payload={"name": name, "enabled": True, "usesBalance": False}, status=201)
    form = {"leaveTypeId": kind["id"], "startDate": day, "endDate": day, "startHalf": "AM", "endHalf": "PM"}
    preview = post(base, "/leave-preview", token, payload=form)
    assert preview["balances"] == [] and preview["canSubmit"]
    result = preview["calculation"]
    assert result["years"] == [{"year": year, "halfDays": 2}]
    assert result["days"] == [{"date": day, "halves": ["AM", "PM"]}]
    call(base, f"/leave-types/{kind['id']}", admin, "PUT",
         {"version": kind["version"], "name": name + " 更新", "enabled": True, "usesBalance": False})
    body = {"type": "LEAVE", "title": "M5 模拟闰日 " + intent()[:8], "form": dict(form, reason="模拟闰日",
            leaveTypeVersion=result["leaveTypeVersion"], calendarVersion=result["calendarVersion"])}
    post(base, "/requests", token, payload=body, key=intent(), status=409, error="STALE_RECORD")
    result = post(base, "/leave-preview", token, payload=form)["calculation"]
    body["form"]["leaveTypeVersion"] = result["leaveTypeVersion"]
    prior_ledger = page_all(base, f"/employees/{employee_id}/leave-ledger", token)
    created = post(base, "/requests", token, payload=body, key=intent(), status=201)
    post(base, "/requests", token, payload=body, key=intent(), status=409, error="LEAVE_OVERLAP")
    assert workflow_action(base, token, created["id"], "WITHDRAW").status == 200
    assert page_all(base, f"/employees/{employee_id}/leave-ledger", token) == prior_ledger
    print("PASS M5 leap-day boundaries, stale leave-type version and non-balance leave with overlap but no ledger")



def accept_rules_and_dashboard(base, admin, applicant, manager, finance, viewer):
    token, employee_id = applicant["token"], applicant["user"]["employeeId"]
    department = call(base, f"/employees/{employee_id}", admin)["departmentId"]
    suffix = intent()[:8]
    rule = {"name": "M5 模拟规则 " + suffix, "type": "EXPENSE", "departmentId": None,
            "minAmountCents": None, "maxAmountCents": None, "priority": 1, "enabled": True,
            "steps": [{"name": "常规报销", "assigneeId": manager["user"]["id"]}]}
    post(base, "/approval-rules", viewer, payload=rule, status=403)
    low = post(base, "/approval-rules", admin, payload=rule, status=201)
    scoped = dict(rule, name="M5 模拟部门规则 " + suffix, priority=2, departmentId=department,
                  minAmountCents=1000, maxAmountCents=2000,
                  steps=[{"name": "部门专项", "assigneeId": finance["user"]["id"]}])
    high = post(base, "/approval-rules", admin, payload=scoped, status=201)
    post(base, "/approval-rules", admin,
         payload=dict(scoped, name="M5 模拟冲突 " + suffix, minAmountCents=2000, maxAmountCents=3000),
         status=409, error="RULE_CONFLICT")
    post(base, "/approval-rules", admin,
         payload=dict(scoped, name="M5 模拟错误账号 " + suffix, priority=5,
                      steps=[{"name": "错误账号", "assigneeId": applicant["user"]["id"]}]),
         status=409, error="ROUTE_UNAVAILABLE")
    post(base, "/requests", manager["token"], key=intent(), status=409, error="ROUTE_UNAVAILABLE",
         payload={"type": "EXPENSE", "title": "M5 模拟自审批 " + suffix,
                  "form": {"expenseDate": date.today().isoformat(), "amountCents": 1, "reason": "模拟自审批"}})

    def expense(amount):
        payload = {"type": "EXPENSE", "title": "M5 模拟费用 " + intent()[:8],
                   "form": {"expenseDate": date.today().isoformat(), "amountCents": amount, "reason": "模拟私人费用说明"}}
        saved = post(base, "/requests", token, payload=payload, key=intent(), status=201)
        return request_detail(base, saved["id"], token)

    boundary = expense(2000)
    assert boundary["rule"]["id"] == high["id"] and boundary["rule"]["version"] == high["version"]
    assert boundary["route"][0]["assigneeId"] == finance["user"]["id"]
    assert expense(2001)["rule"]["id"] == low["id"]
    call(base, f"/approval-rules/{high['id']}", admin, "PUT", dict(scoped, enabled=False, version=high["version"]))
    assert expense(2000)["rule"]["id"] == low["id"]
    original = request_detail(base, boundary["id"], token)
    assert original["rule"] == boundary["rule"] and original["route"] == boundary["route"]

    for actor, is_admin in ((token, False), (manager["token"], False), (viewer, False), (admin, True)):
        counts = call(base, "/dashboard?days=30", actor)
        for key, path in (("pendingTasks", "/tasks"), ("myRequests", "/requests"),
                          ("contractReminders", "/contract-reminders?days=30")):
            assert counts[key] == len(page_all(base, path, actor)), key
        for key, path in (("workflowExceptions", "/workflow-exceptions"), ("approvedLeaves", "/approved-leaves")):
            if is_admin:
                assert counts[key] == len(page_all(base, path, actor)), key
            else:
                assert counts[key] is None
                call(base, path, actor, status=403)
    print("PASS M5 inclusive rule priority, conflict and snapshot, dashboard count/list scoping")


def accept_workflow_exceptions(base, admin, applicant, manager, finance, delegate, viewer):
    token = applicant["token"]
    target = int(delegate["user"]["id"])
    finance_id = int(finance["user"]["id"])
    future, current = (post(base, "/requests", token, payload=general(title), key=intent(), status=201)
                       for title in ("M5 模拟未来异常", "M5 模拟当前异常"))
    current_task = current["tasks"][0]
    post(base, action_route(current_task), manager["token"], payload=action(current_task), key=intent())
    isolated_sql(f"UPDATE sys_users SET enable=2 WHERE id={finance_id} AND enable=1;")
    try:
        call(base, "/workflow-exceptions", viewer, status=403)
        details = {row["requestId"]: row for row in page_all(base, "/workflow-exceptions", admin)}
        for created, code in ((future, "FUTURE_UNAVAILABLE"), (current, "CURRENT_UNAVAILABLE")):
            row = details[created["id"]]
            assert row["reason"] == code and row["step"] == 2 and row["assigneeId"] == finance_id
            assert not ({"title", "form", "applicantName"} & row.keys())
            call(base, f"/requests/{created['id']}", admin, status=404)
            call(base, f"/requests/{created['id']}", delegate["token"], status=404)
            path = f"/workflow-exceptions/{row['taskId']}/handoff"
            body = {"step": row["step"], "version": row["version"], "targetUserId": target, "reason": "M5 模拟修复"}
            post(base, path, viewer, payload=body, status=403)
            post(base, path, admin, payload={key: value for key, value in body.items() if key != "step"},
                 status=400, error="INVALID_INPUT")
            post(base, path, admin, payload=dict(body, version=row["version"] + 1), status=409)
            assert post(base, path, admin, payload=body)["assigneeId"] == target
            post(base, path, admin, payload=body, status=409)
            saved = request_detail(base, created["id"], token)
            assert saved["route"] == created["route"]
            handoff = saved["history"][-1]
            assert handoff["action"] == "HANDOFF" and handoff["targetUserId"] == target
            assert handoff["routeStep"] == 2 and handoff["previousAssigneeId"] == finance_id
            assert handoff["taskId"] == (None if code == "FUTURE_UNAVAILABLE" else row["taskId"])
        future_id = int(future["id"])
        call(base, f"/requests/{future_id}", delegate["token"], status=404)
        assert isolated_sql(f"SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={future_id} AND recipient_id={target};") == "0"
        first = request_detail(base, future_id, token)["tasks"][0]
        post(base, action_route(first), manager["token"], payload=action(first), key=intent())
        new_task = request_detail(base, future_id, delegate["token"])["tasks"][-1]
        assert new_task["assigneeId"] == target and new_task["status"] == "PENDING"
        assert isolated_sql(f"SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={future_id} AND recipient_id={target};") == "1"
        assert request_detail(base, current["id"], delegate["token"])["tasks"][-1]["assigneeId"] == target
    finally:
        isolated_sql(f"UPDATE sys_users SET enable=1 WHERE id={finance_id};")
    call(base, f"/requests/{future['id']}", finance["token"], status=404)

    manager_id = int(manager["user"]["id"])
    post(base, "/approval-rules", admin, status=201, payload={
        "name": "M5 异常步骤竞争 " + intent()[:8], "type": "GENERAL", "departmentId": None,
        "minAmountCents": None, "maxAmountCents": None, "priority": 50, "enabled": True,
        "steps": [{"name": name, "assigneeId": user_id} for name, user_id in
                  (("第一步", manager_id), ("第二步", finance_id), ("第三步", target))],
    })
    raced = post(base, "/requests", token, payload=general("M5 模拟异常步骤竞争"), key=intent(), status=201)

    def exception():
        return next(row for row in page_all(base, "/workflow-exceptions", admin) if row["requestId"] == raced["id"])

    isolated_sql(f"UPDATE sys_users SET enable=2 WHERE id={finance_id};")
    try:
        observed = exception()
        assert observed["step"] == 2
        isolated_sql(f"UPDATE sys_users SET enable=CASE id WHEN {finance_id} THEN 1 ELSE 2 END WHERE id IN ({finance_id},{target});")
        changed = exception()
        assert changed["step"] == 3 and changed["version"] == observed["version"] and changed["taskId"] == observed["taskId"]
        before = request_detail(base, raced["id"], token)
        post(base, f"/workflow-exceptions/{observed['taskId']}/handoff", admin,
             payload={"step": observed["step"], "version": observed["version"], "targetUserId": manager_id,
                      "reason": "M5 模拟过期异常步骤"}, status=409, error="STALE_TASK")
        assert request_detail(base, raced["id"], token) == before
        assert exception() == changed
    finally:
        isolated_sql(f"UPDATE sys_users SET enable=1 WHERE id IN ({finance_id},{target});")
    print("PASS M5 unavailable current/future repair, stale version, protected snapshots and deferred access/outbox")


def accept_config_audits(base, admin, viewer, employee):
    for kind in ("LEAVE_TYPE", "LEAVE_CALENDAR", "APPROVAL_RULE"):
        route = f"/audit-events?type={kind}"
        assert any(row["type"] == kind for row in page_all(base, route, admin))
        for token in (viewer, employee):
            reply = send(base, route, token)
            assert reply.status == 403 or (reply.status == 200 and reply.data["total"] == 0)
    print("PASS M5 configuration changes audited for HR_ADMIN only")


def accept_real_account_handoffs(base, admin, applicant, manager):
    token, manager_id = applicant["token"], manager["user"]["id"]

    def approver(name):
        record = create_employee(base, admin, name)
        username, password = "m5-" + intent()[:16], secrets.token_hex(20)
        created = account_action(base, admin, record["id"], "CREATE", username=username, password=password, role="APPROVER")
        return record["id"], int(created["account"]["id"]), login_password(base, username, password)["token"]

    source_employee, source_id, source_token = approver("M5 模拟规则原审批人")
    override_employee, override_id, override_token = approver("M5 模拟异常接手人")
    _, successor, successor_token = approver("M5 模拟正常交接人")
    post(base, "/approval-rules", admin, status=201, payload={
        "name": "M5 模拟正常交接规则 " + intent()[:8], "type": "GENERAL", "departmentId": None,
        "minAmountCents": None, "maxAmountCents": None, "priority": 200, "enabled": True,
        "steps": [{"name": "初审", "assigneeId": manager_id}, {"name": "复核", "assigneeId": source_id}],
    })
    from_rule, from_override = (post(base, "/requests", token, status=201, key=intent(), payload=general(label))
                                for label in ("M5 模拟规则未来节点", "M5 模拟异常覆盖后交接"))
    isolated_sql(f"UPDATE sys_users SET enable=2 WHERE id={source_id};")
    try:
        exception = next(row for row in page_all(base, "/workflow-exceptions", admin) if row["requestId"] == from_override["id"])
        post(base, f"/workflow-exceptions/{exception['taskId']}/handoff", admin,
             payload={"step": exception["step"], "version": exception["version"],
                      "targetUserId": override_id, "reason": "M5 模拟未来覆盖"})
    finally:
        isolated_sql(f"UPDATE sys_users SET enable=1 WHERE id={source_id};")

    for employee_id, old_token, action_name in ((source_employee, source_token, "OFFBOARD"),
                                                (override_employee, override_token, "DISABLE")):
        account_action(base, admin, employee_id, action_name, status=409, error="HANDOFF_REQUIRED")
        updated = account_action(base, admin, employee_id, action_name, handoffUserId=successor)
        assert not updated["account"]["enabled"]
        call(base, "/auth/me", old_token, status=401)

    for request in (from_rule, from_override):
        request_id = int(request["id"])
        assert request["route"][1]["assigneeId"] == source_id
        saved = request_detail(base, request_id, token)
        assert saved["route"] == request["route"]
        assert saved["history"][-1]["action"] == "HANDOFF" and saved["history"][-1]["taskId"] is None
        assert saved["history"][-1]["targetUserId"] == successor
        call(base, f"/requests/{request_id}", successor_token, status=404)
        notifications = f"SELECT COUNT(*) FROM oa_notification_outbox WHERE request_id={request_id} AND recipient_id={successor};"
        assert isolated_sql(notifications) == "0"
        task = saved["tasks"][0]
        post(base, action_route(task), manager["token"], key=intent(), payload=action(task))
        assert request_detail(base, request_id, successor_token)["tasks"][-1]["assigneeId"] == successor
        assert isolated_sql(notifications) == "1"
    new = post(base, "/requests", token, status=201, key=intent(), payload=general("M5 模拟规则交接后提交"))
    assert new["route"][1]["assigneeId"] == successor
    print("PASS M5 real OFFBOARD/DISABLE moves rule and override future routes, with access only upon assignment")


def main():
    project = os.environ.get("OA_QA_PROJECT", "")
    if not project.startswith("hr-oa-m5-qa-"):
        raise SystemExit("Run with OA_QA_MILESTONE=5 scripts/oa-m4-isolated.sh")
    base = local_base(os.environ["OA_QA_BASE"])
    admin = login(base, "hr_admin", "HR_ADMIN")["token"]
    applicant = login(base, "employee", "EMPLOYEE")
    manager = login(base, "manager", "APPROVER")
    finance = login(base, "finance", "APPROVER")
    delegate = login(base, "delegate", "APPROVER")
    viewer = login(base, "hr_viewer", "HR_VIEWER")["token"]
    accept_legacy(base, applicant, {user["user"]["id"]: user["token"] for user in (manager, finance)})
    accept_leave(base, admin, applicant, manager["token"], finance["token"])
    accept_half_day_contention(base, admin, applicant, manager["token"], finance["token"])
    accept_nonbalance_leap_day(base, admin, applicant)
    accept_rules_and_dashboard(base, admin, applicant, manager, finance, viewer)
    accept_workflow_exceptions(base, admin, applicant, manager, finance, delegate, viewer)
    accept_config_audits(base, admin, viewer, applicant["token"])
    accept_real_account_handoffs(base, admin, applicant, manager)


if __name__ == "__main__":
    main()
