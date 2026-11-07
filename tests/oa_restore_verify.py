"""Read the original synthetic M1 and M2 records from an isolated restored API."""
import argparse
import json
from pathlib import Path

from oa_api import api, local_base, login, verify_saved
from oa_m2_api import request_detail
from oa_m3_api import page_all


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, required=True)
    parser.add_argument("--employee-state", type=Path, required=True)
    parser.add_argument("--approval-state", type=Path, required=True)
    parser.add_argument("--m3-state", type=Path)
    args = parser.parse_args()
    assert api(args.base, "/health")["database"] == "ok"
    admin, _ = login(args.base, "hr_admin", "HR_ADMIN")
    verify_saved(args.base, admin, json.loads(args.employee_state.read_text()))
    employee, _ = login(args.base, "employee", "EMPLOYEE")
    approval = json.loads(args.approval_state.read_text())
    flow = approval["flow"]
    assert request_detail(args.base, flow["id"], employee) == flow["final"]
    rejected = request_detail(args.base, approval["resubmission"]["rejected"], employee)
    resubmitted = request_detail(args.base, approval["resubmission"]["resubmitted"], employee)
    assert rejected["status"] == "REJECTED" and resubmitted["previousRequestId"] == rejected["id"]
    print("PASS isolated restore: original employee, contract, byte-identical attachment and approval IDs")
    if args.m3_state:
        state = json.loads(args.m3_state.read_text())
        department = state["departmentRecord"]
        assert department in page_all(args.base, "/departments", admin)
        assert state["leaveRecord"] in page_all(args.base, f"/employees/{state['targetId']}/annual-leave", admin)
        print("PASS isolated restore: M3 department and annual-leave IDs, versions and values")


if __name__ == "__main__":
    main()
