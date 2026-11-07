"""Run against a local seeded OA instance backed by the development MySQL volume."""
import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import hashlib
import hmac
import json
from pathlib import Path
import os
import re
import secrets
import subprocess
import time
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

PREFIX = "/api/oa/v1"
MIB = 1024 * 1024


class NoRedirects(HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        return None


OPENER = build_opener(ProxyHandler({}), NoRedirects())


def local_base(value):
    parsed = urlsplit(value)
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost"):
        raise argparse.ArgumentTypeError("Use a loopback HTTP development instance")
    if parsed.username or parsed.password or parsed.path not in ("", "/") or parsed.query or parsed.fragment:
        raise argparse.ArgumentTypeError("Use a bare loopback HTTP origin")
    return value.rstrip("/")


def fixture_pdf(padding=0):
    data = b"%PDF-1.4\n" + b" " * padding
    offsets = [0]
    objects = (
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] >>",
    )
    for index, content in enumerate(objects, 1):
        offsets.append(len(data))
        data += f"{index} 0 obj\n".encode() + content + b"\nendobj\n"
    start = len(data)
    data += b"xref\n0 4\n0000000000 65535 f \n"
    for offset in offsets[1:]:
        data += f"{offset:010d} 00000 n \n".encode()
    return data + f"trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n{start}\n%%EOF\n".encode()


def request(base, path, method="GET", token=None, payload=None, content_type=None):
    headers = {}
    if token:
        headers["x-token"] = token
    if content_type:
        headers["Content-Type"] = content_type
    elif payload is not None:
        headers["Content-Type"] = "application/json"
        payload = json.dumps(payload).encode()
    req = Request(base + path, data=payload, headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=30)
    except HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


def api(base, path, status=200, **kwargs):
    actual, _, body = request(base, PREFIX + path, **kwargs)
    assert actual == status, f"{kwargs.get('method', 'GET')} {path} expected {status}, got {actual}"
    envelope = json.loads(body)
    assert envelope["code"] == (0 if status < 400 else 7), f"{path}: invalid envelope"
    if status >= 400:
        assert envelope["data"]["errorCode"], f"{path}: missing error code"
    return envelope["data"]


def login(base, username, role):
    data = api(base, "/auth/login", method="POST", payload={
        "username": username,
        "password": os.environ["OA_SEED_PASSWORD"],
    })
    assert data["user"]["role"] == role
    identity = api(base, "/auth/me", token=data["token"])
    assert identity["user"] == data["user"]
    assert identity["permissions"] == data["permissions"]
    return data["token"], data["user"]["employeeId"]


def expired_token(token):
    header, payload, _ = token.split(".")
    claims = json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))
    now = int(time.time())
    claims.update(iat=now - 120, nbf=now - 120, exp=now - 60)
    encoded = base64.urlsafe_b64encode(json.dumps(claims).encode()).rstrip(b"=").decode()
    data = f"{header}.{encoded}".encode()
    signature = hmac.digest(os.environ["OA_JWT_SECRET"].encode(), data, "sha256")
    return data.decode() + "." + base64.urlsafe_b64encode(signature).rstrip(b"=").decode()


def mysql(sql):
    # Send only fixed SQL over stdin. The development DB credential stays inside MySQL's container.
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "mysql", "sh", "-ec",
         'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u "$MYSQL_USER" "$MYSQL_DATABASE"'],
        input=sql, text=True, capture_output=True, timeout=20,
    )
    if result.returncode:
        raise RuntimeError("development MySQL account check failed")
    return result.stdout.strip()


def verify_live_identity(base, employee, target_id):
    scope = "username = 'employee' AND deleted_at IS NULL"
    assert mysql(f"SELECT authority_id, enable FROM sys_users WHERE {scope}") == "9003\t1"
    try:
        assert mysql(f"UPDATE sys_users SET authority_id = 9002 WHERE {scope}; SELECT ROW_COUNT()") == "1"
        session = api(base, "/auth/me", token=employee)
        assert session["user"]["role"] == "HR_VIEWER"
        assert "salaryCents" not in api(base, f"/employees/{target_id}", token=employee)
        assert mysql(f"UPDATE sys_users SET enable = 2 WHERE {scope}; SELECT ROW_COUNT()") == "1"
        api(base, "/auth/me", 401, token=employee)
        api(base, "/auth/login", 401, method="POST", payload={
            "username": "employee", "password": os.environ["OA_SEED_PASSWORD"],
        })
    finally:
        mysql(f"UPDATE sys_users SET authority_id = 9003, enable = 1 WHERE {scope}")
    assert api(base, "/auth/me", token=employee)["user"]["role"] == "EMPLOYEE"
    api(base, f"/employees/{target_id}", 404, token=employee)


def upload(base, contract_id, token, name, content, status=201):
    boundary = secrets.token_hex(16)
    payload = (
        f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\n'
        "Content-Type: application/pdf\r\n\r\n"
    ).encode() + content + f"\r\n--{boundary}--\r\n".encode()
    return api(base, f"/contracts/{contract_id}/files", status, method="POST",
               token=token, payload=payload, content_type=f"multipart/form-data; boundary={boundary}")


def multipart_temp_files():
    return subprocess.check_output(
        ["docker", "compose", "exec", "-T", "api", "sh", "-c",
         'for file in /tmp/multipart-*; do if test -e "$file"; then printf "%s\\n" "$file"; fi; done'],
        text=True, timeout=20,
    ).splitlines()


def download(base, file_id, token, content_type="application/pdf"):
    status, headers, body = request(base, f"{PREFIX}/files/{file_id}/download", token=token)
    assert status == 200, f"download expected 200, got {status}"
    assert headers.get_content_type() == content_type
    assert headers["Content-Disposition"].lower().startswith("attachment")
    assert headers["X-Content-Type-Options"] == "nosniff"
    assert "no-store" in headers["Cache-Control"]
    return hashlib.sha256(body).hexdigest()


def verify_saved(base, admin, state):
    employee_id = state["employee_id"]
    detail = api(base, f"/employees/{employee_id}", token=admin)
    assert detail["employeeNo"] == state["employee_no"]
    assert detail["salaryCents"] == state["salary_cents"]
    contracts = api(base, f"/employees/{employee_id}/contracts", token=admin)["list"]
    contract = next(row for row in contracts if row["id"] == state["contract_id"])
    assert contract["contractNo"] == state["contract_no"]
    assert [item["id"] for item in contract["files"]] == state["file_ids"]
    assert download(base, state["file_id"], admin) == state["sha256"]


def check(base):
    api(base, "/employees", 401)
    admin, _ = login(base, "hr_admin", "HR_ADMIN")
    api(base, "/auth/me", 401, token="invalid.jwt.value")
    api(base, "/auth/me", 401, token=expired_token(admin))
    viewer, _ = login(base, "hr_viewer", "HR_VIEWER")
    employee, own_id = login(base, "employee", "EMPLOYEE")
    assert isinstance(own_id, int) and own_id > 0
    own = api(base, "/employees", token=employee)
    assert own["total"] == 1 and [item["id"] for item in own["list"]] == [own_id]
    assert "salaryCents" in api(base, f"/employees/{own_id}", token=employee)
    suffix = secrets.token_hex(5).upper()
    employee_input = {
        "employeeNo": f"QA{suffix}", "name": "模拟验收员工", "department": "测试部门",
        "birthDate": "1990-01-15", "hireDate": "2021-07-01", "salaryCents": 1234567,
    }
    before = api(base, "/employees", token=admin)["total"]
    for change in ({"birthDate": "1990-02-30"}, {"hireDate": "01/01/2021"},
                   {"salaryCents": -1}, {"salaryCents": 1.5}, {"salaryCents": 2**53}):
        api(base, "/employees", 400, token=admin, method="POST", payload=dict(employee_input, **change))
    assert api(base, "/employees", token=admin)["total"] == before
    for token in (viewer, employee):
        api(base, "/employees", 403, token=token, method="POST", payload=employee_input)
    detail = api(base, "/employees", 201, token=admin, method="POST", payload=employee_input)
    api(base, "/employees", 409, token=admin, method="POST", payload=employee_input)
    concurrent_input = dict(employee_input, employeeNo=f"QC{suffix}")
    with ThreadPoolExecutor(max_workers=2) as pool:
        writes = [pool.submit(request, base, PREFIX + "/employees", "POST", admin, concurrent_input)
                  for _ in range(2)]
        assert sorted(write.result()[0] for write in writes) == [201, 409], "concurrent employee uniqueness"
    target_id = detail["id"]
    for path in ("/employees", f"/employees/{target_id}"):
        data = api(base, path, token=viewer)
        assert all("salaryCents" not in row for row in data.get("list", [data]))
    api(base, f"/employees/{target_id}", 404, token=employee)
    api(base, f"/employees/{target_id}/contracts", 404, token=employee)
    verify_live_identity(base, employee, target_id)
    print("PASS missing, invalid and expired JWT, immediate DB role change and disable", flush=True)
    print("PASS employee roles, row scope, salary omission, unique and concurrent writes", flush=True)

    contract_input = {"contractNo": f"QA{suffix}", "startDate": "2024-01-01",
                      "endDate": "2027-12-31", "status": "SIGNED"}
    path = f"/employees/{target_id}/contracts"
    for token in (viewer, employee):
        api(base, path, 403, token=token, method="POST", payload=contract_input)
    contract = api(base, path, 201, token=admin, method="POST", payload=contract_input)
    api(base, path, 409, token=admin, method="POST", payload=contract_input)
    own_contract = api(base, f"/employees/{own_id}/contracts", 201, token=admin, method="POST",
                       payload=dict(contract_input, contractNo=f"QO{suffix}"))
    assert own_contract["id"] in {
        row["id"] for row in api(base, f"/employees/{own_id}/contracts", token=employee)["list"]
    }
    for change in ({"startDate": "2024-02-30"}, {"endDate": "2020-01-01"},
                   {"previousContractId": own_contract["id"]}, {"status": "UNKNOWN"}):
        api(base, path, 400, token=admin, method="POST",
            payload=dict(contract_input, contractNo=f"QI{suffix}", **change))
    assert [row["id"] for row in api(base, path, token=admin)["list"]] == [contract["id"]]
    contract_id = contract["id"]
    pdf = fixture_pdf()
    file = upload(base, contract_id, admin, "fixture.pdf", pdf)
    own_file = upload(base, own_contract["id"], admin, "fixture.pdf", pdf)
    digest = hashlib.sha256(pdf).hexdigest()
    assert set(file) == {"id", "originalName", "size", "contentType", "createdAt"}
    assert file["size"] == len(pdf)
    assert download(base, file["id"], viewer) == digest
    assert download(base, own_file["id"], employee) == digest
    api(base, f'/files/{file["id"]}/download', 404, token=employee)
    api(base, f'/files/{file["id"]}/download', 401)
    for token in (viewer, employee):
        upload(base, own_contract["id"], token, "fixture.pdf", pdf, 403)
    upload(base, contract_id, admin, "fixture.txt", pdf, 400)
    upload(base, contract_id, admin, "fixture.pdf", b"invalid PDF payload", 400)
    ids = [file["id"]]
    for filename, mime in (("pixel.png", "image/png"), ("pixel.jpg", "image/jpeg")):
        content = (Path(__file__).parent / "fixtures" / filename).read_bytes()
        image = upload(base, contract_id, admin, filename, content)
        assert image["contentType"] == mime and image["size"] == len(content)
        assert download(base, image["id"], admin, mime) == hashlib.sha256(content).hexdigest()
        upload(base, contract_id, admin, "mismatch.pdf", content, 400)
        ids.append(image["id"])
    # xref byte offsets gain one decimal digit at the boundary PDF's startxref.
    padding = 10 * MIB - len(pdf)
    padding -= len(fixture_pdf(padding)) - 10 * MIB
    boundary_pdf = fixture_pdf(padding)
    assert len(boundary_pdf) == 10 * MIB
    boundary_file = upload(base, contract_id, admin, "boundary.pdf", boundary_pdf)
    assert boundary_file["size"] == 10 * MIB
    ids.append(boundary_file["id"])
    upload(base, contract_id, admin, "too-large.pdf", fixture_pdf(padding + 1), 413)
    temporary_before = multipart_temp_files()
    truncated = (b'--fixture\r\nContent-Disposition: form-data; name="file"; filename="truncated.pdf"'
                 b'\r\nContent-Type: application/pdf\r\n\r\n' + fixture_pdf(2 * MIB))
    api(base, f"/contracts/{contract_id}/files", 400, method="POST", token=admin, payload=truncated,
        content_type="multipart/form-data; boundary=fixture")
    assert multipart_temp_files() == temporary_before, "temporary multipart file leaked"
    print("PASS truncated multipart rejected without temporary-file leak", flush=True)
    print("PASS contract scope, authorized downloads, PDF/JPEG/PNG, mismatch, 10 MiB and 10 MiB+1", flush=True)

    routes = (
        ("POST", "/init/checkdb"), ("POST", "/init/initdb"),
        ("POST", "/autoCode/createTemp"), ("POST", "/autoCode/installPlugin"),
        ("POST", "/autoCode/removePlugin"), ("POST", "/autoCode/llmAuto"),
        ("POST", "/fileUploadAndDownload/upload"),
        ("POST", "/fileUploadAndDownload/breakpointContinueFinish"),
        ("GET", "/uploads/file/fixture.pdf"), ("GET", "/public/uploads/fixture.pdf"),
    )
    for method, route in routes:
        for prefix in ("", "/api", PREFIX):
            actual, _, _ = request(base, prefix + route, method=method, token=admin)
            assert actual == 404, f"{method} {prefix + route} expected 404 at web ingress, got {actual}"
            options = ["--post-data={}"] if method == "POST" else []
            direct = subprocess.run(
                ["docker", "compose", "exec", "-T", "api", "wget", "-S", "-O", "/dev/null",
                 *options, "http://127.0.0.1:8888" + prefix + route],
                capture_output=True, text=True, timeout=20,
            )
            assert re.search(r"HTTP/1\.[01] 404\b", direct.stderr), (
                f"{method} {prefix + route} expected 404 directly at API"
            )
    print("PASS upstream route probes through web ingress and directly at API", flush=True)

    for _ in range(41):
        status, headers, body = request(base, PREFIX + "/auth/login", method="POST", payload={})
        if status == 429:
            assert int(headers["Retry-After"]) > 0
            assert json.loads(body)["data"]["errorCode"] == "RATE_LIMITED"
            break
        assert status == 400, f"invalid login body expected 400 or 429, got {status}"
    else:
        raise AssertionError("login endpoint did not enforce its rate limit")
    print("PASS login limiter returns 429 with Retry-After", flush=True)

    state = {
        "employee_id": target_id, "employee_no": detail["employeeNo"], "salary_cents": detail["salaryCents"],
        "contract_id": contract_id, "contract_no": contract["contractNo"],
        "file_id": file["id"], "file_ids": ids, "sha256": digest, "own_contract_no": own_contract["contractNo"],
    }
    verify_saved(base, admin, state)
    return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("create", "verify"))
    parser.add_argument("--base", type=local_base, default="http://127.0.0.1:8080")
    parser.add_argument("--state", type=Path, default=Path("tests/results/oa-state.json"))
    args = parser.parse_args()
    if not os.environ.get("OA_SEED_PASSWORD") or (args.mode == "create" and not os.environ.get("OA_JWT_SECRET")):
        parser.error("Export the local OA_SEED_PASSWORD and OA_JWT_SECRET before running")
    assert api(args.base, "/health") == {
        "status": "ok", "database": "ok", "integrationMode": "local-development",
    }
    if args.mode == "create":
        state = check(args.base)
        args.state.parent.mkdir(parents=True, exist_ok=True)
        args.state.write_text(json.dumps(state) + "\n")
        print("PASS fixture readback before restart")
    else:
        admin, _ = login(args.base, "hr_admin", "HR_ADMIN")
        verify_saved(args.base, admin, json.loads(args.state.read_text()))
        print("PASS fixture readback with byte-identical attachment")


if __name__ == "__main__":
    main()
