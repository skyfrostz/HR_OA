"""Real MySQL and private file smoke from a freshly seeded, disposable project."""
import argparse
from datetime import datetime, timedelta
import hashlib
from io import BytesIO
import os
import re
import subprocess
import xml.etree.ElementTree as ET
from zipfile import ZipFile
from zoneinfo import ZoneInfo

from oa_api import api, download, fixture_pdf, local_base, upload
from oa_m2_api import action, action_route, assert_replay, call as post, intent, login, request_detail, send
from oa_m3_api import binary_workbook, call, fill_workbook, page_all, preview, send as request, workbook_rows
from oa_m4_api import account_action


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


def restricted_attachment(base, admin, viewer, employee, contract_id):
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
    for suffix in ("/preview", "/download"):
        assert request(base, file_path + suffix, viewer).status == 404
    for actor in (admin, employee):
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
    assert request(base, file_path + "/preview", viewer).body == body
    print("PASS CI attachment classification, hidden restricted bytes, private preview and optimistic version", flush=True)


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
    restricted_attachment(base, admin, viewer, employee, contract["id"])
    employee_import_history(base, admin, viewer, profile["department"], users["hr_viewer"]["user"]["id"])

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
