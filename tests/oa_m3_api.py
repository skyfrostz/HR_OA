"""M3 MySQL API acceptance using only generated synthetic fixtures."""
import argparse
from contextlib import contextmanager
from datetime import datetime, timedelta
from io import BytesIO
import json
import os
import re
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request
import xml.etree.ElementTree as ET
from zipfile import ZipFile
from zoneinfo import ZoneInfo

from oa_api import OPENER, PREFIX, local_base, mysql, verify_saved
from oa_m2_api import Response, expect, intent, login, race

XLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
XML = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
HEADERS = ("工号", "姓名", "部门", "出生日期", "入职日期", "约定工资（元）")
EXPORT = ("工号", "姓名", "部门", "出生日期", "年龄", "入职日期", "工龄")


def send(base, route, token, method="GET", payload=None, content_type="application/json"):
    data = json.dumps(payload, ensure_ascii=False).encode() if isinstance(payload, dict) else payload
    headers = {"x-token": token}
    if data is not None:
        headers["Content-Type"] = content_type
    request = Request(base + PREFIX + route, data=data, headers=headers, method=method)
    try:
        response = OPENER.open(request, timeout=30)
    except HTTPError as error:
        response = error
    with response:
        return Response(response.status, response.headers, response.read())


def call(base, route, token, method="GET", payload=None, status=200, error=None):
    response = send(base, route, token, method, payload)
    assert response.status == status, f"{method} {route} expected {status}, got {response.status}"
    return expect(response, status, error)


def page_all(base, route, token):
    separator = "&" if "?" in route else "?"
    records = []
    page = 1
    while True:
        result = call(base, f"{route}{separator}page={page}&pageSize=100", token)
        records.extend(result["list"])
        if len(records) >= result["total"]:
            return records
        assert result["list"], "list pagination made no progress"
        page += 1


def employee_input(number, name, **department):
    return {"employeeNo": number, "name": name, "birthDate": "1990-01-01",
            "hireDate": "2022-01-01", "salaryCents": 846321, **department}


def assert_single_winner(outcomes):
    assert sorted(response.status for response in outcomes) == [200, 409]
    winner = next(response.data for response in outcomes if response.status == 200)
    expect(next(response for response in outcomes if response.status == 409), 409, "STALE_RECORD")
    return winner


def baseline(base, state):
    admin = login(base, "hr_admin", "HR_ADMIN")["token"]
    verify_saved(base, admin, state)
    return call(base, f"/employees/{state['employee_id']}", admin)


def departments(base, users, before, suffix):
    admin, viewer, employee, manager = (users[name]["token"] for name in ("hr_admin", "hr_viewer", "employee", "manager"))
    before_id = before["id"]
    current = call(base, f"/employees/{before_id}", admin)
    assert all(current[field] == value for field, value in before.items())
    assert current["version"] >= 1
    departments = {row["id"]: row for row in page_all(base, "/departments", viewer)}
    assert departments[current["departmentId"]]["name"] == current["department"]
    for token in (employee, manager):
        call(base, "/departments", token, status=403)
    call(base, "/departments", viewer, "POST", {"name": "越权部门"}, 403)

    root = call(base, "/departments", admin, "POST", {"name": "模拟主部门" + suffix}, 201)
    child = call(base, "/departments", admin, "POST", {"name": "模拟子部门" + suffix, "parentId": root["id"]}, 201)
    root_route = f"/departments/{root['id']}"
    root_edit = {"name": root["name"], "parentId": child["id"], "version": root["version"]}
    call(base, root_route, admin, "PUT", root_edit, 400)
    call(base, root_route, admin, "PUT", dict(root_edit, parentId=root["id"]), 400)
    payload = employee_input("M3" + suffix, "模拟版本员工", departmentId=child["id"])
    record = call(base, "/employees", admin, "POST", payload, 201)
    route = f"/employees/{record['id']}"
    assert record["departmentId"] == child["id"] and record["department"] == child["name"]
    call(base, "/employees", admin, "POST",
         employee_input("MM" + suffix, "模拟部门不一致", departmentId=child["id"], department=root["name"]), 400)

    child_route = f"/departments/{child['id']}"
    new_department = call(base, child_route, admin, "PUT",
                          {"name": "模拟更名部门" + suffix, "parentId": root["id"], "version": child["version"]})
    call(base, child_route, admin, "PUT",
         {"name": child["name"], "parentId": root["id"], "version": child["version"]}, 409, "STALE_RECORD")
    updated = call(base, route, admin)
    assert updated["department"] == new_department["name"] and updated["version"] == record["version"] + 1
    base_update = dict(payload, version=updated["version"])
    winner = assert_single_winner(race(
        lambda: send(base, route, admin, "PUT", dict(base_update, name="模拟并发甲", salaryCents=11)),
        lambda: send(base, route, admin, "PUT", dict(base_update, name="模拟并发乙", salaryCents=22)),
    ))
    assert winner["version"] == updated["version"] + 1 and call(base, route, admin) == winner
    assert (winner["name"], winner["salaryCents"]) in (("模拟并发甲", 11), ("模拟并发乙", 22))
    call(base, route, viewer, "PUT", base_update, 403)
    print("PASS M3 migration preserves original employee, department hierarchy and concurrent employee version", flush=True)
    return winner["id"], new_department


def leave(base, users, target):
    admin, viewer, employee, manager = (users[name]["token"] for name in ("hr_admin", "hr_viewer", "employee", "manager"))
    own = users["employee"]["user"]["employeeId"]
    list_route = f"/employees/{target}/annual-leave"
    year = 2026
    route = f"{list_route}/{year}"
    payload = {"version": 0, "entitledHalfDays": 17, "usedHalfDays": 3, "note": " 模拟年假 "}
    call(base, route, admin, "PUT", dict(payload, usedHalfDays=18), 400)
    call(base, route, viewer, "PUT", payload, 403)
    created = assert_single_winner(race(
        lambda: send(base, route, admin, "PUT", payload),
        lambda: send(base, route, admin, "PUT", payload),
    ))
    assert created["remainingHalfDays"] == 14 and created["note"] == "模拟年假"
    updated = assert_single_winner(race(
        lambda: send(base, route, admin, "PUT", dict(payload, version=created["version"], usedHalfDays=4)),
        lambda: send(base, route, admin, "PUT", dict(payload, version=created["version"], usedHalfDays=5)),
    ))
    assert updated["version"] == created["version"] + 1
    assert updated["remainingHalfDays"] == 17 - updated["usedHalfDays"]
    assert page_all(base, list_route, viewer) == [updated]
    call(base, list_route, employee, status=404)
    call(base, list_route, manager, status=404)
    assert isinstance(page_all(base, f"/employees/{own}/annual-leave", employee), list)
    print("PASS M3 annual-leave create/update concurrency, half-day calculation and row scope", flush=True)
    return updated


def workbook_rows(data):
    with ZipFile(BytesIO(data)) as archive:
        sheet = ET.fromstring(archive.read("xl/worksheets/sheet1.xml"))
        strings = []
        if "xl/sharedStrings.xml" in archive.namelist():
            table = ET.fromstring(archive.read("xl/sharedStrings.xml"))
            strings = ["".join(item.itertext()) for item in table]
        result = []
        for row in sheet.findall(f"{XML}sheetData/{XML}row"):
            cells = {}
            for cell in row.findall(f"{XML}c"):
                assert cell.find(f"{XML}f") is None, "downloaded workbook contains an executable formula"
                column = "".join(filter(str.isalpha, cell.attrib["r"]))
                text = cell.findtext(f"{XML}v", "")
                if cell.get("t") == "s":
                    text = strings[int(text)]
                if cell.get("t") == "inlineStr":
                    text = "".join(cell.find(f"{XML}is").itertext())
                cells[column] = text
            if any(cells.values()):
                result.append(tuple(cells.get(col, "") for col in "ABCDEFGH"[:len(cells)]))
        return result


def fill_workbook(template, rows, formula=False):
    buffer = BytesIO()
    with ZipFile(BytesIO(template)) as original, ZipFile(buffer, "w") as output:
        for item in original.infolist():
            data = original.read(item.filename)
            if item.filename == "xl/worksheets/sheet1.xml":
                sheet = ET.fromstring(data)
                sheet_data = sheet.find(f"{XML}sheetData")
                for row in list(sheet_data)[1:]:
                    sheet_data.remove(row)
                for index, values in enumerate(rows, 2):
                    row = ET.SubElement(sheet_data, f"{XML}row", {"r": str(index)})
                    for column, value in zip("ABCDEF", values):
                        cell = ET.SubElement(row, f"{XML}c", {"r": f"{column}{index}"})
                        if formula and column == "F":
                            ET.SubElement(cell, f"{XML}f").text = "1+2"
                            ET.SubElement(cell, f"{XML}v").text = "3"
                        else:
                            cell.set("t", "inlineStr")
                            ET.SubElement(ET.SubElement(cell, f"{XML}is"), f"{XML}t").text = value
                dimension = sheet.find(f"{XML}dimension")
                if dimension is not None:
                    dimension.set("ref", f"A1:F{len(rows) + 1}")
                data = ET.tostring(sheet, encoding="utf-8", xml_declaration=True)
            output.writestr(item, data)
    return buffer.getvalue()


def binary_workbook(base, path, token):
    response = send(base, path, token)
    assert response.status == 200, f"{path} expected workbook, got {response.status}"
    assert response.headers.get_content_type() == XLSX
    assert response.headers["Content-Disposition"].lower().startswith("attachment")
    assert response.headers["X-Content-Type-Options"] == "nosniff"
    assert "no-store" in response.headers["Cache-Control"]
    return response.body


def preview(base, token, workbook, filename="synthetic.xlsx"):
    boundary = intent()
    multipart = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\n'
                 f"Content-Type: {XLSX}\r\n\r\n").encode() + workbook + f"\r\n--{boundary}--\r\n".encode()
    return send(base, "/employee-imports/preview", token, "POST", multipart,
                f"multipart/form-data; boundary={boundary}")


def external_workbook(template):
    buffer = BytesIO()
    with ZipFile(BytesIO(template)) as original, ZipFile(buffer, "w") as output:
        for item in original.infolist():
            output.writestr(item, original.read(item.filename))
        output.writestr("xl/externalLinks/externalLink1.xml", "<externalLink/>")
    return buffer.getvalue()


@contextmanager
def change_test_account(name, identifier, field, original, temporary):
    assert (name, field, original, temporary) in (
        ("hr_viewer", "authority_id", 9002, 9001), ("hr_admin", "enable", 1, 2),
    )
    scope = f"id={int(identifier)} AND username='{name}' AND deleted_at IS NULL"
    assert mysql(f"SELECT {field} FROM sys_users WHERE {scope}") == str(original)
    try:
        assert mysql(f"UPDATE sys_users SET {field}={temporary} WHERE {scope}; SELECT ROW_COUNT()") == "1"
        yield
    finally:
        mysql(f"UPDATE sys_users SET {field}={original} WHERE {scope}")


def imports_and_exports(base, users, department, suffix):
    admin, viewer, employee, manager = (users[name]["token"] for name in ("hr_admin", "hr_viewer", "employee", "manager"))
    for name, identity in users.items():
        privileges = identity["permissions"]
        assert privileges["canExportEmployees"] is True
        for key in ("canManageDepartments", "canManageAnnualLeave", "canImportEmployees"):
            assert privileges[key] is (name == "hr_admin")
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    assert workbook_rows(template) == [HEADERS]
    call(base, "/employees/import-template.xlsx", viewer, status=403)

    def row(number, name="模拟 Excel 员工", date="1990-01-01", department_name=department):
        return (number, name, department_name, date, "2022-01-01", "1234.50")

    duplicate = "XD" + suffix
    bad = fill_workbook(template, [row(duplicate), row(duplicate), row("XF" + suffix, date="1990-02-30"),
                                  row("XN" + suffix, department_name="不存在的模拟部门" + suffix)])
    invalid = expect(preview(base, admin, bad), 201)
    assert invalid["totalRows"] == 4 and invalid["errorRows"] >= 3 and not invalid["canConfirm"]
    assert {item["row"] for item in invalid["rows"]} == {2, 3, 4, 5}
    assert all(item["errors"] for item in invalid["rows"] if item["row"] in (4, 5))
    call(base, f"/employee-imports/{invalid['id']}/confirm", admin, "POST", {}, 409, "IMPORT_INVALID")
    valid_book = fill_workbook(template, [row("XI" + suffix, "=模拟文字"), row("XJ" + suffix)])
    expect(preview(base, viewer, valid_book), 403)
    expect(preview(base, admin, valid_book, "invalid.xls"), 400)
    expect(preview(base, admin, b"invalid xlsx"), 400)
    expect(preview(base, admin, external_workbook(valid_book)), 400)
    expect(preview(base, admin, b"x" * (2 * 1024 * 1024 + 1)), 413)
    formula = preview(base, admin, fill_workbook(template, [row("XP" + suffix)], formula=True))
    if formula.status == 201:
        assert not formula.data["canConfirm"] and formula.data["errorRows"] > 0
    else:
        expect(formula, 400)

    valid = expect(preview(base, admin, valid_book), 201)
    route = f"/employee-imports/{valid['id']}"
    assert valid["totalRows"] == valid["validRows"] == 2 and valid["canConfirm"]
    assert call(base, route, admin) == valid
    with change_test_account("hr_viewer", users["hr_viewer"]["user"]["id"], "authority_id", 9002, 9001):
        call(base, route, viewer, status=404)
        call(base, route + "/confirm", viewer, "POST", {}, 404)
    results = race(
        lambda: send(base, route + "/confirm", admin, "POST", {}),
        lambda: send(base, route + "/confirm", admin, "POST", {}),
    )
    first, second = (expect(item, 200) for item in results)
    assert first == second and first["importedCount"] == 2 and len(set(first["employeeIds"])) == 2
    assert call(base, route + "/confirm", admin, "POST", {}) == first
    assert call(base, route, admin)["status"] == "COMMITTED"
    with change_test_account("hr_admin", users["hr_admin"]["user"]["id"], "enable", 1, 2):
        call(base, route + "/confirm", admin, "POST", {}, 401)
    expired = expect(preview(base, admin, fill_workbook(template, [row("XZ" + suffix)])), 201)
    preview_id = expired["id"]
    assert re.fullmatch(r"[a-zA-Z0-9_-]{16,128}", preview_id)
    assert mysql(f"UPDATE oa_employee_imports SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 MINUTE WHERE id='{preview_id}'; SELECT ROW_COUNT()") == "1"
    call(base, f"/employee-imports/{preview_id}/confirm", admin, "POST", {}, 410, "PREVIEW_EXPIRED")

    contested, untouched = "XC" + suffix, "XT" + suffix
    conflict = expect(preview(base, admin, fill_workbook(template, [row(untouched), row(contested)])), 201)
    call(base, "/employees", admin, "POST", employee_input(contested, "模拟先写入", department=department), 201)
    call(base, f"/employee-imports/{conflict['id']}/confirm", admin, "POST", {}, 409, "IMPORT_CONFLICT")

    def exported(token):
        return workbook_rows(binary_workbook(base, "/employees/export.xlsx", token))

    admin_export, viewer_export, employee_export, manager_export = (
        exported(token) for token in (admin, viewer, employee, manager)
    )
    assert admin_export[0] == employee_export[0] == manager_export[0] == EXPORT + (HEADERS[-1],)
    assert viewer_export[0] == EXPORT and all(len(row) == 7 for row in viewer_export)
    for name, data in (("employee", employee_export), ("manager", manager_export)):
        own = call(base, f"/employees/{users[name]['user']['employeeId']}", users[name]["token"])
        assert len(data) == 2 and data[1][0] == own["employeeNo"]
    matching = [item for item in admin_export[1:] if item[0] in ("XI" + suffix, "XJ" + suffix)]
    assert len(matching) == 2 and any(item[1] == "=模拟文字" for item in matching)
    assert not any(item[0] == untouched for item in admin_export[1:]), "conflicting preview partially imported"
    print("PASS M3 XLSX row errors, formula/extension/size rejection, concurrent replay, atomic conflict, export isolation",
          flush=True)


def reminders(base, users, target, suffix):
    admin, viewer, employee, manager = (users[name]["token"] for name in ("hr_admin", "hr_viewer", "employee", "manager"))
    today = datetime.now(ZoneInfo("Asia/Shanghai")).date()

    def create(identifier, code, days, status="SIGNED", previous=None):
        finish = today + timedelta(days=days)
        payload = {"contractNo": code + suffix, "startDate": (finish - timedelta(days=90)).isoformat(),
                   "endDate": finish.isoformat(), "status": status}
        if previous:
            payload["previousContractId"] = previous["id"]
        return call(base, f"/employees/{identifier}/contracts", admin, "POST", payload, 201)

    due = create(target, "MT", 0)
    expired = create(target, "ME", -2, "EXPIRED")
    future = create(target, "MF", 31)
    draft = create(target, "MD", -1, "DRAFT")
    terminated = create(target, "MX", -1, "TERMINATED")
    superseded = create(target, "MO", -7)
    successor = create(target, "MS", 50, previous=superseded)
    retained = create(target, "MR", -8)
    draft_successor = create(target, "MP", 20, "DRAFT", retained)
    own = create(users["employee"]["user"]["employeeId"], "MU", 1)
    approver_own = create(users["manager"]["user"]["employeeId"], "MA", 1)
    for days in ("0", "91", "x"):
        call(base, f"/contract-reminders?days={days}", admin, status=400)
    default = page_all(base, "/contract-reminders", admin)
    assert [(item["endDate"], item["contractId"]) for item in default] == sorted(
        (item["endDate"], item["contractId"]) for item in default
    )
    by_id = {item["contractId"]: item for item in default}
    assert [by_id[item["id"]]["daysRemaining"] for item in (due, expired, retained)] == [0, -2, -8]
    for item in (future, draft, terminated, superseded, successor, draft_successor):
        assert item["id"] not in by_id
    assert by_id[expired["id"]]["status"] == "EXPIRED"
    assert all("salaryCents" not in item for item in default)
    wide = {row["contractId"] for row in page_all(base, "/contract-reminders?days=90", admin)}
    assert {future["id"], successor["id"]} <= wide and superseded["id"] not in wide
    assert page_all(base, "/contract-reminders", viewer) == default
    for token, contract in ((employee, own), (manager, approver_own)):
        scoped = page_all(base, "/contract-reminders", token)
        assert contract["id"] in {item["contractId"] for item in scoped}
        assert all(item["employeeId"] == contract["employeeId"] for item in scoped)
    records = {row["id"]: row for row in page_all(base, f"/employees/{target}/contracts", admin)}
    assert records[due["id"]]["status"] == "SIGNED" and records[expired["id"]]["status"] == "EXPIRED"
    print("PASS M3 reminder day boundaries, signed successor suppression, ordering and role scope", flush=True)
    return by_id[due["id"]]


def verify(base, before):
    users = {name: login(base, name, role) for name, role in (
        ("hr_admin", "HR_ADMIN"), ("hr_viewer", "HR_VIEWER"), ("employee", "EMPLOYEE"), ("manager", "APPROVER"),
    )}
    suffix = intent()[:10].upper()
    target, department = departments(base, users, before, suffix)
    leave_record = leave(base, users, target)
    imports_and_exports(base, users, department["name"], suffix)
    due = reminders(base, users, target, suffix)
    return {"targetId": target, "department": department["name"], "departmentRecord": department,
            "leaveRecord": leave_record, "due": due}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("baseline", "verify"))
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--m1-state", type=Path, default=Path("tests/results/oa-state.json"))
    parser.add_argument("--baseline", type=Path, default=Path("tests/results/m3/pre-migration.json"))
    parser.add_argument("--results", type=Path, default=Path("tests/results/m3/state.json"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export the local OA_SEED_PASSWORD before running")
    if args.mode == "baseline":
        record = baseline(args.base, json.loads(args.m1_state.read_text()))
        args.baseline.parent.mkdir(parents=True, exist_ok=True)
        with args.baseline.open("x") as destination:
            json.dump(record, destination, ensure_ascii=False)
        print("PASS M1 original fixture captured before M3 migration")
    else:
        state = verify(args.base, json.loads(args.baseline.read_text()))
        args.results.parent.mkdir(parents=True, exist_ok=True)
        args.results.write_text(json.dumps(state, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
