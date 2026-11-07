"""Read synthetic M1–M6 records from an upgraded or isolated restored API."""
import argparse
import hashlib
import json
from pathlib import Path

from oa_api import api, download, fixture_pdf, local_base, login, verify_saved
from oa_m2_api import request_detail
from oa_m3_api import call, page_all, send


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    parser.add_argument("--employee-state", type=Path, required=True)
    parser.add_argument("--approval-state", type=Path, required=True)
    parser.add_argument("--m3-state", type=Path)
    parser.add_argument("--m4-state", type=Path)
    parser.add_argument("--m5-state", type=Path)
    parser.add_argument("--m6-state", type=Path)
    args = parser.parse_args()
    assert api(args.base, "/health")["database"] == "ok"
    admin, _ = login(args.base, "hr_admin", "HR_ADMIN")
    verify_saved(args.base, admin, json.loads(args.employee_state.read_text()))
    employee, _ = login(args.base, "employee", "EMPLOYEE")
    approval = json.loads(args.approval_state.read_text())
    flow = approval["flow"]
    historical_flow = request_detail(args.base, flow["id"], employee)
    assert {key: historical_flow[key] for key in flow["final"]} == flow["final"]
    assert historical_flow.get("leaveCalculation") is None and historical_flow.get("rule") is None
    rejected = request_detail(args.base, approval["resubmission"]["rejected"], employee)
    resubmitted = request_detail(args.base, approval["resubmission"]["resubmitted"], employee)
    assert rejected["status"] == "REJECTED" and resubmitted["previousRequestId"] == rejected["id"]
    print("PASS isolated restore: original employee, contract, byte-identical attachment and approval IDs")
    if args.m3_state:
        state = json.loads(args.m3_state.read_text())
        department = state["departmentRecord"]
        assert department in page_all(args.base, "/departments", admin)
        annual = state["leaveRecord"]
        actual, = [row for row in page_all(args.base, f"/employees/{state['targetId']}/annual-leave", admin)
                   if row["year"] == annual["year"]]
        assert {key: actual[key] for key in annual} == annual
        assert actual.get("approvalUsedHalfDays", 0) == actual.get("reservedHalfDays", 0) == 0
        print("PASS isolated restore: M3 department and annual-leave IDs, versions and values")
    if args.m4_state:
        state = json.loads(args.m4_state.read_text())
        identifier = state["contract"]["id"]
        contract = call(args.base, f"/contracts/{identifier}", admin)
        assert contract["status"] == "TERMINATED" and contract["terminatedDate"] == state["contract"]["terminated"]
        assert contract["employeeId"] == state["employeeId"]
        attachment, = [file for file in contract["files"] if file["originalName"] == "m4-fixture.pdf"]
        assert download(args.base, attachment["id"], admin) == hashlib.sha256(fixture_pdf()).hexdigest()
        due = state["contract"]["due"]
        assert call(args.base, f"/contracts/{due['id']}", admin)["contractNo"] == due["contractNo"]
        events = page_all(args.base, f"/audit-events?contractId={identifier}", admin)
        assert [event["action"] for event in events] == ["TERMINATE", "SIGN", "UPDATE", "CREATE"]
        account = call(args.base, f"/employees/{state['employeeId']}/account", admin)
        assert account["account"]["enabled"] and account["account"]["role"] == "APPROVER"
        print("PASS isolated restore: M4 lifecycle account, original contract status, byte-identical attachment and audit history")
    if args.m5_state:
        state = json.loads(args.m5_state.read_text())
        request_id, employee_id, year = (state[key] for key in ("requestId", "employeeId", "year"))
        restored = request_detail(args.base, request_id, employee)
        assert restored["status"] == "CANCELLED" and restored["leaveCalculation"] == state["calculation"]
        ledger = page_all(args.base, f"/employees/{employee_id}/leave-ledger?year={year}", employee)
        assert [(row["kind"], row["halfDays"]) for row in ledger if row["requestId"] == request_id] == [
            ("REVERSE", 1), ("CONSUME", 1), ("RESERVE", 1),
        ]
        annual, = [row for row in page_all(args.base, f"/employees/{employee_id}/annual-leave", employee)
                   if row["year"] == year]
        assert {key: annual[key] for key in state["annual"]} == state["annual"]
        print("PASS isolated restore: M5 leave snapshot, cancellation, ledger and annual counters")
    if args.m6_state:
        state = json.loads(args.m6_state.read_text())
        viewer, _ = login(args.base, "hr_viewer", "HR_VIEWER")
        contract = call(args.base, f"/contracts/{state['contractId']}", admin)
        file, = [row for row in contract["files"] if row["id"] == state["fileId"]]
        assert file["classification"] == "RESTRICTED" and contract["employeeId"] == state["employeeId"]
        file_path = f"/files/{file['id']}"
        assert send(args.base, file_path + "/preview", viewer).status == 404
        assert download(args.base, file["id"], admin) == state["sha256"]
        result = call(args.base, f"/employee-imports/{state['importId']}/result", admin)
        assert result["importedCount"] == 1
        print("PASS isolated restore: M6 restricted file bytes and scope, durable import result")


if __name__ == "__main__":
    main()
