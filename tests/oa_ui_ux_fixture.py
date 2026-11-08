"""Fingerprint read-only synthetic records used by paired OA browser captures."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import time

from oa_api import local_base
from oa_m2_api import expect, send as auth_request
from oa_m3_api import binary_workbook, call, fill_workbook, send, workbook_rows


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False,
                                     separators=(",", ":")).encode()).hexdigest()


def token_for(base, username, role):
    for attempt in range(5):
        result = auth_request(base, "/auth/login", payload={
            "username": username, "password": os.environ["OA_SEED_PASSWORD"],
        })
        if result.status != 429 or attempt == 4:
            identity = expect(result, 200)
            assert identity["user"]["role"] == role
            return identity["token"]
        delay = int(result.headers.get("Retry-After", "0"))
        assert 0 < delay <= 30
        time.sleep(delay + .1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    args = parser.parse_args()
    base = args.base
    admin = token_for(base, "hr_admin", "HR_ADMIN")
    employee = token_for(base, "employee", "EMPLOYEE")
    manager = token_for(base, "manager", "APPROVER")
    history = Path(__file__).parent / "results"
    m5 = json.loads((history / "m5/browser-state.json").read_text())
    m6 = json.loads((history / "m6/browser-state.json").read_text())
    employee_id = m6["employeeId"]
    outputs = {}

    def record(label, data):
        entry = {"sha256": digest(data)}
        if isinstance(data, dict) and "list" in data:
            entry.update({"total": data.get("total"), "pageItems": len(data["list"] or [])})
        outputs[label] = entry

    def get(label, path, token=admin):
        result = call(base, path, token)
        record(label, result)
        return result

    for page in (1, 2):
        record(f"employee-roster-page-{page}", call(base, "/employees/search", admin, "POST",
                                                  {"page": page, "pageSize": 20}))
    record("employee-no-match", call(base, "/employees/search", admin, "POST",
                                     {"query": "UIUX-不可能匹配-20261108", "page": 1, "pageSize": 20}))
    person = get("fixed-employee-detail", f"/employees/{employee_id}")
    record("fixed-employee-search", call(base, "/employees/search", admin, "POST",
                                         {"query": person["employeeNo"], "page": 1, "pageSize": 20}))
    get("fixed-employee-contracts", f"/employees/{employee_id}/contracts?page=1&pageSize=20")
    get("fixed-employee-account", f"/employees/{employee_id}/account")
    get("fixed-employee-audit", f"/audit-events?employeeId={employee_id}&page=1&pageSize=20")
    get("fixed-contract", f"/contracts/{m6['contractId']}")
    file = send(base, f"/files/{m6['fileId']}/download", admin)
    assert file.status == 200 and hashlib.sha256(file.body).hexdigest() == m6["sha256"]
    outputs["fixed-attachment-bytes"] = {"sha256": m6["sha256"]}
    for token, label in ((admin, "admin"), (employee, "employee")):
        get("notices-" + label, "/notifications?page=1&pageSize=20", token)
        get("reminders-" + label, "/contract-reminders?days=30&page=1&pageSize=20", token)
        get("dashboard-" + label, "/dashboard?days=30", token)
    get("manager-history", "/tasks/history?page=1&pageSize=20", manager)
    get("manager-pending", "/tasks?page=1&pageSize=20", manager)
    get("employee-requests", "/requests?page=1&pageSize=20", employee)
    get("employee-templates", "/request-templates", employee)
    get("departments", "/departments/tree")
    get("calendar", "/leave-calendar")
    get("rules", "/approval-rules?page=1&pageSize=20")
    get("approver-candidates", "/approval-rules/assignees?page=1&pageSize=20")
    get("leave-types", "/leave-types")
    get("approved-leaves", "/approved-leaves?page=1&pageSize=20")
    get("workflow-exceptions", "/workflow-exceptions?page=1&pageSize=20")
    get("annual-leave", f"/employees/{employee_id}/annual-leave?page=1&pageSize=20", employee)
    get("leave-ledger", f"/employees/{employee_id}/leave-ledger?page=1&pageSize=20", employee)
    get("fixed-leave-snapshot", f"/requests/{m5['requestId']}", employee)
    get("fixed-import-result", f"/employee-imports/{m6['importId']}/result")
    template = binary_workbook(base, "/employees/import-template.xlsx", admin)
    workbook = fill_workbook(template, [
        ("UIUX-V", "界面验收专用合成员工", person["department"], "1990-01-01", "2022-01-01", "1500.50"),
        ("UIUX-D", "界面验收专用错误日期", person["department"], "1990-02-30", "2022-01-01", "1500.50"),
    ])
    record("import-fixture-cells", workbook_rows(workbook))
    outputs["import-fixture-raw-bytes"] = {"sha256": hashlib.sha256(workbook).hexdigest()}
    print(json.dumps(outputs, sort_keys=True, indent=2))


if __name__ == "__main__":
    main()
