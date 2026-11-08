"""Real MySQL and private file smoke from a freshly seeded, disposable project."""
import argparse
from datetime import datetime, timedelta
import hashlib
from io import BytesIO
import os
import re
import secrets
import subprocess
import xml.etree.ElementTree as ET
from zipfile import ZipFile
from zoneinfo import ZoneInfo

from oa_api import api, download, fixture_pdf, local_base, upload
from oa_m2_api import action, action_route, assert_replay, call as post, intent, login, request_detail, send
from oa_m3_api import binary_workbook, call, employee_input, fill_workbook, page_all, preview, race, send as request, workbook_rows
from oa_m4_api import account_action, login_password


def ci_sql(sql):
    project = os.environ.get("OA_CI_PROJECT", "")
    if not re.fullmatch(r"hr-oa-ci-[0-9a-f]{12}", project):
        raise RuntimeError("SQL fixture changes require a disposable CI project")
    result = subprocess.run(
        ["docker", "compose", "--env-file", "/dev/null", "--project-name", project,
         "--file", "deploy/oa/integration.compose.yaml", "exec", "-T", "mysql", "sh", "-ec",
         'test "$MYSQL_DATABASE" = hr_oa_ci; MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"'],
        input=sql, text=True, capture_output=True, timeout=20,
    )
    if result.returncode:
        raise RuntimeError("Isolated CI SQL fixture failed")
    return result.stdout.strip()


def restricted_attachment(base, admin, viewer, employee, other, contract_id):
    body, boundary = fixture_pdf(), intent()
    multipart = (f'--{boundary}\r\nContent-Disposition: form-data; name="classification"\r\n\r\nRESTRICTED\r\n'
                 f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="restricted.pdf"\r\n'
                 'Content-Type: application/pdf\r\n\r\n').encode() + body + f"\r\n--{boundary}--\r\n".encode()
    uploaded = request(base, f"/contracts/{contract_id}/files", admin, "POST", multipart, f"multipart/form-data; boundary={boundary}")
    assert uploaded.status == 201
    file = uploaded.data
    assert file["classification"] == "RESTRICTED" and file["version"] >= 1
    file_path = f"/files/{file['id']}"
    assert file["id"] not in {row["id"] for row in call(base, f"/contracts/{contract_id}", viewer)["files"]}
    call(base, f"/contracts/{contract_id}", other, status=404)
    for suffix in ("/preview", "/download"):
        for actor in (viewer, other):
            assert request(base, file_path + suffix, actor).status == 404
    for actor in (viewer, employee, other):
        call(base, file_path + "/classification", actor, "PUT", {
            "classification": "STANDARD", "version": file["version"], "reason": "CI 尝试越权修改范围",
        }, 403)
    for actor in (admin, employee):
        assert file["id"] in {row["id"] for row in call(base, f"/contracts/{contract_id}", actor)["files"]}
        assert request(base, file_path + "/download", actor).body == body
        opened = request(base, file_path + "/preview", actor)
        assert opened.status == 200 and opened.body == body
        assert opened.headers["Content-Disposition"].lower().startswith("attachment")
        assert opened.headers["X-Content-Type-Options"] == "nosniff"
        assert "no-store" in opened.headers["Cache-Control"]
        assert "sandbox" in opened.headers["Content-Security-Policy"]
    updated = call(base, file_path + "/classification", admin, "PUT", {
        "classification": "STANDARD", "version": file["version"], "reason": "CI 模拟授权",
    })
    assert updated["version"] == file["version"] + 1
    call(base, file_path + "/classification", admin, "PUT", {
        "classification": "RESTRICTED", "version": file["version"], "reason": "CI 模拟过期版本",
    }, 409, "STALE_RECORD")
    for actor in (admin, viewer, employee):
        assert file["id"] in {row["id"] for row in call(base, f"/contracts/{contract_id}", actor)["files"]}
        assert request(base, file_path + "/preview", actor).body == body
        assert request(base, file_path + "/download", actor).body == body
    for suffix in ("/preview", "/download"):
        assert request(base, file_path + suffix, other).status == 404
    print("PASS CI four-identity attachment metadata, preview, download and classification authorization", flush=True)


def employee_import_history(base, admin, viewer, department, viewer_id):
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    suffix = intent()[:9]
    valid = ("CI-" + suffix, "模拟导入", department, "1990-01-01", "2022-01-01", "1234.50")
    invalid = ("CX-" + suffix, "=1+1", department, "1990-02-30", "2022-01-01", "1234.50")
    workbook = fill_workbook(template, [valid, invalid])
    reply = preview(base, admin, workbook)
    assert reply.status == 201
    identifier = reply.data["id"]
    route = f"/employee-imports/{identifier}"
    errors = call(base, route + "?errorsOnly=true", admin)
    assert errors["totalRows"] == 2 and errors["errorRows"] == 1 and [row["row"] for row in errors["rows"]] == [3]
    export = binary_workbook(base, route + "/errors.xlsx", admin)
    with ZipFile(BytesIO(export)) as archive:
        sheet = ET.fromstring(archive.read("xl/worksheets/sheet1.xml"))
        assert all(cell.tag.rsplit("}", 1)[-1] != "f" for cell in sheet.iter())
    assert any("=1+1" in row for row in workbook_rows(export)[1:])
    assert request(base, route + "/errors.xlsx", viewer).status in (403, 404)
    viewer_id = int(viewer_id)
    assert ci_sql(f"UPDATE sys_users SET authority_id=9001 WHERE id={viewer_id} AND authority_id=9002; SELECT ROW_COUNT();") == "1"
    try:
        other_admin = login(base, "hr_viewer", "HR_ADMIN")["token"]
        for suffix in ("", "/errors.xlsx", "/result"):
            call(base, route + suffix, other_admin, status=404)
        assert page_all(base, "/employee-imports", other_admin) == []
    finally:
        ci_sql(f"UPDATE sys_users SET authority_id=9002 WHERE id={viewer_id};")
    assert re.fullmatch(r"[a-zA-Z0-9_-]{16,128}", identifier)
    assert ci_sql(f"UPDATE oa_employee_imports SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 MINUTE WHERE id='{identifier}'; SELECT ROW_COUNT();") == "1"
    expired = call(base, route, admin)
    assert expired["status"] == "EXPIRED" and expired["rows"] == []
    call(base, route + "/confirm", admin, "POST", {}, 410, "PREVIEW_EXPIRED")
    call(base, route + "/errors.xlsx", admin, status=410, error="PREVIEW_EXPIRED")

    candidate = preview(base, admin, fill_workbook(template, [valid]))
    assert candidate.status == 201
    committed_route = f"/employee-imports/{candidate.data['id']}"
    result = call(base, committed_route + "/confirm", admin, "POST", {})
    assert result["importedCount"] == 1
    assert call(base, committed_route + "/result", admin) == result
    assert call(base, committed_route + "/confirm", admin, "POST", {}) == result
    rows = {row["id"]: row for row in page_all(base, "/employee-imports", admin)}
    assert rows[identifier]["status"] == "EXPIRED" and rows[candidate.data["id"]]["status"] == "COMMITTED"
    assert "rows" not in rows[identifier] and "rows" not in rows[candidate.data["id"]]
    print("PASS CI import error-only export, literal spreadsheet cell, expiry, history and durable result", flush=True)


def department_scope(base, admin, viewer, employee):
    suffix = intent()[:8]
    root = call(base, "/departments", admin, "POST", {"name": "QA 父部门 " + suffix}, 201)
    child = call(base, "/departments", admin, "POST", {"name": "QA 子部门 " + suffix, "parentId": root["id"]}, 201)
    grandchild = call(base, "/departments", admin, "POST", {"name": "QA 孙部门 " + suffix, "parentId": child["id"]}, 201)
    outside = call(base, "/departments", admin, "POST", {"name": "QA 其他部门 " + suffix}, 201)
    fixtures = [call(base, "/employees", admin, "POST",
                     employee_input(f"UI{index}-{suffix}", "界面验收合成员工 " + suffix, departmentId=row["id"]), 201)
                for index, row in enumerate((root, child, grandchild, outside))]
    ids = [row["id"] for row in fixtures]
    numbers = [row["employeeNo"] for row in fixtures]
    query = {"departmentId": root["id"], "departmentScope": "SUBTREE", "query": suffix}
    for token in (admin, viewer):
        first = call(base, "/employees/search", token, "POST", dict(query, page=1, pageSize=2))
        second = call(base, "/employees/search", token, "POST", dict(query, page=2, pageSize=2))
        assert first["total"] == second["total"] == 3
        assert len(first["list"]) == 2 and len(second["list"]) == 1
        assert {r["id"] for r in first["list"] + second["list"]} == set(ids[:3])
        exported = request(base, "/employees/export.xlsx", token, "POST", query)
        assert exported.status == 200 and {row[0] for row in workbook_rows(exported.body)[1:]} == set(numbers[:3])
    for direct in ({"departmentId": root["id"]}, {"departmentId": root["id"], "departmentScope": "DIRECT"},
                   {"departmentId": grandchild["id"], "departmentScope": "SUBTREE"}):
        found = call(base, "/employees/search", admin, "POST", direct)
        target = ids[0] if direct["departmentId"] == root["id"] else ids[2]
        assert found["total"] == 1 and [r["id"] for r in found["list"]] == [target]
    assert call(base, "/employees/search", employee, "POST", query)["total"] == 0
    exported = request(base, "/employees/export.xlsx", employee, "POST", query)
    assert exported.status == 200 and not workbook_rows(exported.body)[1:]
    for invalid in ({"departmentScope": "SUBTREE"}, {"departmentScope": "descendant", "departmentId": root["id"]}):
        call(base, "/employees/search", admin, "POST", invalid, 400)
        assert request(base, "/employees/export.xlsx", admin, "POST", invalid).status == 400
    print("PASS UI/UX CI department subtree: descendants, totals, paging, export, legacy default and own-record scope", flush=True)


def structured_import_errors(base, admin, department, existing_number):
    suffix = intent()[:8]
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    future = (datetime.now(ZoneInfo("Asia/Shanghai")).date() + timedelta(days=400)).isoformat()

    def row(number, birth="1990-01-01", hire="2022-01-01"):
        return (number, "界面验收合成员工", department, birth, hire, "1234.50")

    entries = [
        row("UI-V-" + suffix, hire=future), row("UI-F-" + suffix, "1990/01/01"),
        row("UI-N-" + suffix, "1990-02-30"), row("UI-Y-" + suffix, "0999-01-01"),
        row("UI-B-" + suffix, future, future), row("UI-H-" + suffix, hire="1980-01-01"),
        row("UI-D-" + suffix), row("UI-D-" + suffix), row(existing_number),
    ]
    response = preview(base, admin, fill_workbook(template, entries))
    assert response.status == 201
    result = response.data
    actual = {r["row"]: {e["code"] for e in r["errors"] or []} for r in result["rows"]}
    assert actual[2] == set(), actual
    for number, code in enumerate(("DATE_FORMAT", "DATE_NOT_FOUND", "DATE_OUT_OF_RANGE", "FUTURE_BIRTH_DATE",
                                   "HIRE_BEFORE_BIRTH", "DUPLICATE_IN_WORKBOOK", "DUPLICATE_IN_WORKBOOK",
                                   "EMPLOYEE_NO_EXISTS"), 3):
        assert code in actual[number], actual
    assert not result["canConfirm"] and result["validRows"] == 1
    call(base, f"/employee-imports/{result['id']}/confirm", admin, "POST", {}, 409, "IMPORT_INVALID")
    assert call(base, "/employees/search", admin, "POST", {"query": suffix})["total"] == 0
    print("PASS UI/UX CI Excel: seven structured error codes, future-hire compatibility and all-or-nothing confirm", flush=True)


def discarded_import(base, admin, viewer, department, viewer_id):
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    prefix = "UD-" + intent()[:8]
    entries = [(f"{prefix}-{index}", "界面验收替换预览", department, "1990-01-01", "2022-01-01", "1234.50")
               for index in range(2)]
    for iteration, row in enumerate(entries):
        reply = preview(base, admin, fill_workbook(template, [row]))
        assert reply.status == 201 and reply.data["canConfirm"]
        identifier = reply.data["id"]
        route = f"/employee-imports/{identifier}"
        if iteration == 0:
            assert request(base, route + "/discard", viewer, "POST", {}).status == 403
            viewer_id = int(viewer_id)
            assert ci_sql(f"UPDATE sys_users SET authority_id=9001 WHERE id={viewer_id} AND authority_id=9002; SELECT ROW_COUNT();") == "1"
            try:
                other_admin = login(base, "hr_viewer", "HR_ADMIN")["token"]
                call(base, route + "/discard", other_admin, "POST", {}, 404)
            finally:
                ci_sql(f"UPDATE sys_users SET authority_id=9002 WHERE id={viewer_id};")
            expired = call(base, route + "/discard", admin, "POST", {})
            assert call(base, route + "/discard", admin, "POST", {}) == expired
        else:
            discard, confirm = race(lambda: request(base, route + "/discard", admin, "POST", {}),
                                    lambda: request(base, route + "/confirm", admin, "POST", {}))
            assert (discard.status, confirm.status) in ((200, 410), (409, 200))
            if confirm.status == 200:
                assert confirm.data["importedCount"] == 1
                call(base, route + "/discard", admin, "POST", {}, 409, "INVALID_STATE")
                assert call(base, "/employees/search", admin, "POST", {"query": row[0]})["total"] == 1
                continue
            expired = discard.data
        assert expired["status"] == "EXPIRED" and expired["rows"] == [] and not expired["canConfirm"]
        assert call(base, route, admin) == expired
        assert next(r for r in page_all(base, "/employee-imports", admin) if r["id"] == identifier)["status"] == "EXPIRED"
        call(base, route + "/confirm", admin, "POST", {}, 410, "PREVIEW_EXPIRED")
        call(base, route + "/errors.xlsx", admin, status=410, error="PREVIEW_EXPIRED")
        assert re.fullmatch(r"[a-zA-Z0-9_-]{16,128}", identifier)
        assert ci_sql(f"SELECT COUNT(*) FROM oa_employee_imports WHERE id='{identifier}' AND validated IS NULL "
                      "AND JSON_LENGTH(JSON_EXTRACT(preview, '$.rows'))=0 AND purged_at IS NOT NULL;") == "1"
        assert call(base, "/employees/search", admin, "POST", {"query": row[0]})["total"] == 0
    print("PASS UI/UX CI import discard: ownership, durable purge, retry and mutual exclusion with confirm", flush=True)


def test_fresh_stack(base):
    assert api(base, "/health")["database"] == "ok"
    assert ci_sql("SELECT COUNT(*) FROM oa_schema_migrations WHERE version=6 "
                  "AND UNIX_TIMESTAMP(applied_at)>0 AND applied_at<=UTC_TIMESTAMP();") == "1"
    assert ci_sql("SELECT column_name FROM information_schema.statistics WHERE table_schema=DATABASE() "
                  "AND table_name='oa_notification_outbox' AND index_name='ix_oa_outbox_dispatch' ORDER BY seq_in_index;").splitlines() == [
        "delivery_mode", "delivery_status", "available_at",
    ]
    users = {name: login(base, name, role) for name, role in (
        ("hr_admin", "HR_ADMIN"), ("hr_viewer", "HR_VIEWER"), ("employee", "EMPLOYEE"),
        ("manager", "APPROVER"), ("finance", "APPROVER"),
    )}
    admin, viewer, employee, manager, finance = [user["token"] for user in users.values()]
    employee_id = users["employee"]["user"]["employeeId"]
    profile = call(base, f"/employees/{employee_id}", employee)
    assert "salaryCents" in profile
    assert "salaryCents" not in call(base, f"/employees/{employee_id}", viewer)
    assert [row["id"] for row in page_all(base, "/employees", employee)] == [employee_id]

    day = datetime.now(ZoneInfo("Asia/Shanghai")).date()
    contract = call(base, f"/employees/{employee_id}/contracts", admin, "POST", {
        "contractNo": "CI-" + intent()[:12], "startDate": str(day - timedelta(days=365)),
        "endDate": str(day + timedelta(days=365)),
        "status": "DRAFT", "reason": "CI 模拟合同",
    }, 201)
    attachment = upload(base, contract["id"], admin, "ci-fixture.pdf", fixture_pdf())
    assert download(base, attachment["id"], admin) == hashlib.sha256(fixture_pdf()).hexdigest()
    signed = call(base, f"/contracts/{contract['id']}/actions", admin, "POST", {
        "action": "SIGN", "version": contract["version"], "reason": "CI 模拟签署", "effectiveDate": str(day),
    })
    assert signed["status"] == "SIGNED" and signed["version"] == contract["version"] + 1
    assert any(event["action"] == "SIGN" for event in page_all(base, f"/audit-events?contractId={contract['id']}", admin))
    print("PASS CI seeded roles, scoped salary, contract lifecycle and private attachment bytes", flush=True)
    unrelated = call(base, "/employees", admin, "POST",
                     employee_input("CI-OTHER-" + intent()[:10], "无关合同验收员工", departmentId=profile["departmentId"]), 201)
    username, password = "ci-other-" + intent()[:12], secrets.token_hex(20)
    account_action(base, admin, unrelated["id"], "CREATE", username=username, password=password, role="EMPLOYEE")
    other = login_password(base, username, password)["token"]
    assert call(base, "/auth/me", other)["user"]["role"] == "EMPLOYEE"
    restricted_attachment(base, admin, viewer, employee, other, contract["id"])
    renewed = call(base, f"/employees/{employee_id}/contracts", admin, "POST", {
        "contractNo": "CI-RENEW-" + intent()[:10], "startDate": str(day + timedelta(days=366)),
        "endDate": str(day + timedelta(days=730)), "status": "DRAFT", "previousContractId": contract["id"],
        "reason": "CI 验收续约关联草稿",
    }, 201)
    assert renewed["status"] == "DRAFT" and renewed["previousContractId"] == contract["id"]
    old = call(base, f"/contracts/{contract['id']}", admin)
    assert old["endDate"] == contract["endDate"] and old["status"] == "SIGNED"
    assert attachment["id"] in {item["id"] for item in old["files"]}
    assert download(base, attachment["id"], admin) == hashlib.sha256(fixture_pdf()).hexdigest()
    print("PASS CI renewal creates a linked draft and preserves original dates and attachments", flush=True)
    employee_import_history(base, admin, viewer, profile["department"], users["hr_viewer"]["user"]["id"])
    department_scope(base, admin, viewer, employee)
    structured_import_errors(base, admin, profile["department"], profile["employeeNo"])
    discarded_import(base, admin, viewer, profile["department"], users["hr_viewer"]["user"]["id"])

    while day.isoweekday() > 5:
        day += timedelta(days=1)
    annual = f"/employees/{employee_id}/annual-leave/{day.year}"
    seeded = next((row for row in page_all(base, f"/employees/{employee_id}/annual-leave", admin) if row["year"] == day.year), None)
    call(base, annual, admin, "PUT", {"version": seeded["version"] if seeded else 0,
         "entitledHalfDays": 4, "usedHalfDays": 1, "note": "CI 模拟额度", "reason": "CI 模拟验收"})
    leave_type, = [row for row in call(base, "/leave-types", employee)["list"] if row["usesBalance"]]
    period = {"leaveTypeId": leave_type["id"], "startDate": str(day), "endDate": str(day),
              "startHalf": "AM", "endHalf": "AM"}
    preview = post(base, "/leave-preview", employee, payload=period)
    assert preview["canSubmit"] and preview["calculation"]["totalHalfDays"] == 1
    calculation = preview["calculation"]
    body = {"type": "LEAVE", "title": "CI 模拟半天 " + intent()[:8], "form": dict(period, reason="CI 合成资料",
            leaveTypeVersion=calculation["leaveTypeVersion"], calendarVersion=calculation["calendarVersion"])}
    key = intent()
    first = send(base, "/requests", employee, body, key)
    assert first.status == 201
    assert_replay(first, send(base, "/requests", employee, body, key))
    request_id = first.data["id"]
    for approver in (manager, finance):
        task = request_detail(base, request_id, employee)["tasks"][-1]
        post(base, action_route(task), approver, payload=action(task), key=intent())
    assert request_detail(base, request_id, employee)["status"] == "APPROVED"
    post(base, f"/requests/{request_id}/actions", admin, payload={"action": "CANCEL", "reason": "CI 模拟撤销"}, key=intent())
    assert request_detail(base, request_id, employee)["status"] == "CANCELLED"
    ledger = page_all(base, f"/employees/{employee_id}/leave-ledger", employee)
    assert [(row["kind"], row["halfDays"]) for row in ledger] == [("REVERSE", 1), ("CONSUME", 1), ("RESERVE", 1)]
    balance, = [row for row in page_all(base, f"/employees/{employee_id}/annual-leave", employee) if row["year"] == day.year]
    assert (balance["usedHalfDays"], balance["approvalUsedHalfDays"], balance["reservedHalfDays"],
            balance["remainingHalfDays"]) == (1, 0, 0, 3)
    print("PASS CI half-day preview, idempotent reserve, final approval and cancellation reversal", flush=True)

    account_action(base, admin, employee_id, "DISABLE")
    call(base, "/auth/me", employee, status=401)
    account_action(base, admin, employee_id, "ENABLE")
    call(base, "/auth/me", employee, status=401)
    login(base, "employee", "EMPLOYEE")
    print("PASS CI account disable and enable never revives an old token", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    test_fresh_stack(parser.parse_args().base)
