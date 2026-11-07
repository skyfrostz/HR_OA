"""Black-box employee filter and export checks on the seeded local MySQL stack."""
import argparse
import json
import os
from pathlib import Path

from oa_api import local_base
from oa_m2_api import intent, login
from oa_m3_api import EXPORT, HEADERS, XLSX, call, employee_input, send, workbook_rows

SEARCH = "/employees/search"
DOWNLOAD = "/employees/export.xlsx"


def result(base, token, **filters):
    page = call(base, SEARCH, token, "POST", filters)
    assert set(page) == {"list", "total", "page", "pageSize"}
    return page


def export(base, token, filters=None):
    reply = send(base, DOWNLOAD, token, "GET" if filters is None else "POST", filters)
    assert reply.status == 200
    assert reply.headers.get_content_type() == XLSX
    assert reply.headers["Content-Disposition"].lower().startswith("attachment")
    assert reply.headers["X-Content-Type-Options"] == "nosniff"
    assert "no-store" in reply.headers["Cache-Control"]
    return workbook_rows(reply.body)


def check(base):
    users = {name: login(base, name, role) for name, role in (
        ("hr_admin", "HR_ADMIN"), ("hr_viewer", "HR_VIEWER"), ("employee", "EMPLOYEE"), ("manager", "APPROVER"),
    )}
    admin, viewer, employee, manager = (users[name]["token"] for name in users)
    assert call(base, "/employees", admin) == result(base, admin, query="")
    for body in (
        {"query": "研" * 101}, {"query": ["bad"]}, {"departmentId": 0},
        {"departmentId": -1}, {"departmentId": 1.5}, {"departmentId": "1"},
        {"page": 0}, {"pageSize": 101}, {"page": 9223372036854775807, "pageSize": 100}, {"badField": True},
    ):
        call(base, SEARCH, admin, "POST", body, 400)
    call(base, SEARCH, admin, "POST", {"query": "研" * 100})
    call(base, DOWNLOAD, admin, "POST", {"pageSize": 1}, 400)

    tag = "QX" + intent()[:12].upper()
    department = call(base, "/departments", admin, "POST", {"name": tag + " 本部"}, 201)
    child = call(base, "/departments", admin, "POST", {"name": tag + " 下级", "parentId": department["id"]}, 201)
    fixtures = []
    for index, (name, dep) in enumerate((
        (f"{tag} 研发%_'一", department), (f"={tag} 研发AA'二", department),
        (f"{tag} 研发XY三", department), (f"{tag} 研发%_'下级", child),
    )):
        body = employee_input(f"{tag}-{index}", name, departmentId=dep["id"])
        fixtures.append(call(base, "/employees", admin, "POST", body, 201))
    expected = [employee["employeeNo"] for employee in fixtures]

    def numbers(page):
        return [row["employeeNo"] for row in page["list"]]

    assert numbers(result(base, admin, query=f" {tag}-0 ")) == expected[:1]
    assert numbers(result(base, admin, query="研发%_'", departmentId=department["id"])) == expected[:1]
    assert numbers(result(base, admin, query="%", departmentId=department["id"])) == expected[:1]
    assert numbers(result(base, admin, query="_", departmentId=department["id"])) == expected[:1]
    assert numbers(result(base, admin, query="'", departmentId=department["id"])) == expected[:2]
    assert numbers(result(base, admin, query="%' OR 1=1 --", departmentId=department["id"])) == []
    assert numbers(result(base, admin, query=child["name"])) == expected[3:]
    filters = {"query": tag, "departmentId": department["id"]}
    first = result(base, admin, **filters, pageSize=2)
    second = result(base, admin, **filters, page=2, pageSize=2)
    third = result(base, admin, **filters, page=3, pageSize=2)
    assert first["total"] == second["total"] == third["total"] == 3
    assert numbers(first) + numbers(second) == expected[:3] and third["list"] == []
    assert first["pageSize"] == 2 and second["page"] == 2
    for size in (20, 50, 100):
        assert result(base, admin, **filters, pageSize=size)["pageSize"] == size
    read = result(base, viewer, **filters)
    assert numbers(read) == expected[:3] and all("salaryCents" not in row for row in read["list"])
    assert all("salaryCents" in row for row in first["list"])

    def workbook(rows, salary):
        assert rows[0] == EXPORT + ((HEADERS[-1],) if salary else ())
        assert all(len(row) == (8 if salary else 7) for row in rows)
        return [row[0] for row in rows[1:]]

    admin_rows = export(base, admin, filters)
    assert workbook(admin_rows, True) == expected[:3]
    assert admin_rows[2][1] == fixtures[1]["name"]
    assert workbook(export(base, viewer, filters), False) == expected[:3]
    empty_filter = {"query": intent(), "departmentId": department["id"]}
    empty = result(base, viewer, **empty_filter)
    assert empty["list"] == [] and empty["total"] == 0
    assert workbook(export(base, viewer, empty_filter), False) == []
    assert set(expected) <= set(workbook(export(base, viewer), False))
    for name, token in (("employee", employee), ("manager", manager)):
        assert result(base, token, **filters)["total"] == 0
        assert workbook(export(base, token, filters), True) == []
        own_id = users[name]["user"]["employeeId"]
        own = call(base, f"/employees/{own_id}", token)
        assert result(base, token, query=own["employeeNo"])["list"] == [own]
        assert workbook(export(base, token, {"query": own["employeeNo"]}), True) == [own["employeeNo"]]
    print("PASS real MySQL literal search, strict inputs, pagination, four-role scope and filtered XLSX", flush=True)
    return {"query": tag, "department": department, "rows": expected[:3], "name": fixtures[0]["name"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/commercial/state.json"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD"):
        parser.error("Export OA_SEED_PASSWORD from the local development environment")
    state = check(args.base)
    args.state.parent.mkdir(parents=True, exist_ok=True)
    args.state.write_text(json.dumps(state, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
