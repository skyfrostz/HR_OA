"""Synthetic HTTP provider for the disposable M6 Compose project."""
from collections import Counter
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from threading import Lock
from urllib.parse import parse_qs, urlencode, urlsplit

CORP = "oa-ci-corp"
AGENT = 1000001
BUCKET = "oa-ci-private"
ACCESS_KEY = "oa-ci-access-key"
PREFIX = "qa/"
MAX_BODY = 12 * 1024 * 1024


class Provider(BaseHTTPRequestHandler):
    lock = Lock()
    counts = Counter()
    objects = {}
    messages = []
    used_codes = set()
    issued_token = ""
    switches = {"expire_next_token": False, "next_code": "member-a", "send_mode": "ok", "oss_mode": "ok"}

    def log_message(self, *_):
        # Authorization codes, DSNs, signatures and tokens never enter mock logs.
        return

    def respond(self, status, body=b"", *, content_type="application/octet-stream", headers=None):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        for key, value in (headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def json_reply(self, status=200, **data):
        self.respond(status, json.dumps(data).encode(), content_type="application/json")

    def xml_error(self, status, code):
        self.respond(status, f"<Error><Code>{code}</Code><Message>synthetic provider</Message></Error>".encode(),
                     content_type="application/xml")

    def body(self):
        size = int(self.headers.get("Content-Length", "0"))
        if size < 0 or size > MAX_BODY:
            raise ValueError("invalid synthetic request size")
        return self.rfile.read(size)

    def wecom(self, path, query):
        kind = path.rsplit("/", 1)[-1]
        self.counts[kind] += 1
        if kind == "gettoken":
            if self.command != "GET" or query != {"corpid": [CORP], "corpsecret": [os.environ["OA_MOCK_WECOM_SECRET"]]}:
                return self.json_reply(errcode=40001)
            Provider.issued_token = "oa-ci-token-" + str(self.counts[kind])
            return self.json_reply(errcode=0, access_token=self.issued_token, expires_in=7200)
        if query.get("access_token") != [self.issued_token] or not self.issued_token:
            return self.json_reply(errcode=40014)
        if self.switches["expire_next_token"]:
            self.switches["expire_next_token"] = False
            return self.json_reply(errcode=42001)
        if kind == "getuserinfo" and self.command == "GET":
            code = query.get("code", [""])[0]
            if code in self.used_codes:
                return self.json_reply(errcode=40029)
            self.used_codes.add(code)
            member = {"member-a": "Synthetic-A", "member-b": "Synthetic-B"}.get(code.split(":", 1)[0])
            return self.json_reply(errcode=0, userid=member) if member else self.json_reply(errcode=0, openid="synthetic-external")
        if kind == "send" and self.command == "POST" and self.headers.get("Content-Type") == "application/json":
            message = json.loads(self.body())
            required = {"touser", "msgtype", "agentid", "text", "enable_duplicate_check", "duplicate_check_interval"}
            if (set(message) != required or message["agentid"] != AGENT or message["msgtype"] != "text"
                    or message["enable_duplicate_check"] != 1 or message["duplicate_check_interval"] <= 0
                    or set(message["text"]) != {"content"}):
                return self.json_reply(errcode=40003)
            self.messages.append({"recipient": message["touser"], "text": message["text"]["content"]})
            mode = self.switches["send_mode"]
            if mode == "invalid":
                return self.json_reply(errcode=0, invaliduser=message["touser"])
            if mode == "rate_limit":
                return self.json_reply(errcode=45009)
            if mode == "unknown":
                return self.respond(502)
            return self.json_reply(errcode=0)
        return self.json_reply(405, errcode=40001)

    def authorize(self, query):
        target = urlsplit(query.get("redirect_uri", [""])[0])
        state = query.get("state", [""])[0]
        h5 = query.get("scope") == ["snsapi_base"] and query.get("response_type") == ["code"]
        pc = query.get("login_type") == ["CorpApp"] and "scope" not in query and "response_type" not in query
        if (query.get("appid") != [CORP] or query.get("agentid") != [str(AGENT)] or
                not (h5 or pc) or not state or
                target.scheme != "http" or target.hostname != "127.0.0.1" or
                target.path != "/api/oa/v1/auth/wecom/callback" or target.query or target.fragment):
            return self.respond(400)
        self.counts["authorize"] += 1
        code = self.switches["next_code"] + ":" + str(self.counts["authorize"])
        self.respond(302, headers={"Location": target.geturl() + "?" + urlencode({"state": state, "code": code})})

    def oss(self, path, query):
        auth = self.headers.get("Authorization", "")
        if not (auth.startswith("OSS4-HMAC-SHA256 Credential=" + ACCESS_KEY + "/") or auth.startswith("OSS " + ACCESS_KEY + ":")):
            return self.xml_error(403, "SignatureDoesNotMatch")
        marker = "/" + BUCKET + "/"
        host = self.headers.get("Host", "").split(":", 1)[0]
        if path.rstrip("/") == "/" + BUCKET and self.command == "GET":
            bucket_checks = {
                "acl": "<AccessControlPolicy><AccessControlList><Grant>private</Grant></AccessControlList></AccessControlPolicy>",
                "publicAccessBlock": "<PublicAccessBlockConfiguration><BlockPublicAccess>true</BlockPublicAccess></PublicAccessBlockConfiguration>",
                "versioning": "<VersioningConfiguration/>",
            }
            match = next((key for key in bucket_checks if key in query), None)
            if match:
                self.counts["oss_bucket_" + match] += 1
                return self.respond(200, bucket_checks[match].encode(), content_type="application/xml")
        if path.startswith(marker):
            key = path.removeprefix(marker)
        elif host.startswith(BUCKET + "."):
            key = path.lstrip("/")
        else:
            return self.xml_error(404, "NoSuchBucket")
        if not key.startswith(PREFIX) or ".." in key.split("/") or key.endswith("/"):
            return self.xml_error(400, "InvalidObjectName")
        self.counts["oss_" + self.command.lower()] += 1
        if self.switches["oss_mode"] == "unavailable":
            return self.xml_error(503, "ServiceUnavailable")
        if self.command == "PUT":
            if (self.headers.get("x-oss-object-acl") != "private" or
                    self.headers.get("x-oss-forbid-overwrite") != "true" or
                    self.headers.get("x-oss-server-side-encryption") != "AES256" or
                    self.headers.get("Cache-Control") != "no-store" or
                    self.headers.get("Content-Disposition") != "attachment"):
                return self.xml_error(400, "InvalidArgument")
            if key in self.objects and (self.headers.get("If-None-Match") == "*" or self.headers.get("x-oss-forbid-overwrite") == "true"):
                return self.xml_error(409, "FileAlreadyExists")
            self.objects[key] = self.body()
            data = self.objects[key]
            return self.respond(200, headers={"ETag": '"' + hashlib.md5(data).hexdigest() + '"'})
        if self.command == "DELETE":
            self.objects.pop(key, None)
            return self.respond(204)
        if key not in self.objects:
            return self.xml_error(404, "NoSuchKey")
        data = self.objects[key]
        if self.switches["oss_mode"] == "corrupt" and self.command == "GET":
            data = b"synthetic corruption"
        return self.respond(200, data, headers={"ETag": '"' + hashlib.md5(data).hexdigest() + '"'})

    def control(self, path):
        if self.headers.get("X-QA-Control") != os.environ["OA_MOCK_CONTROL"]:
            return self.respond(404)
        if self.command == "GET" and path == "/__mock/state":
            return self.json_reply(counts=dict(self.counts), objectCount=len(self.objects), messages=self.messages)
        if self.command == "POST" and path == "/__mock/mode":
            updates = json.loads(self.body())
            validators = {"expire_next_token": (True, False), "next_code": ("member-a", "member-b", "external"),
                          "send_mode": ("ok", "invalid", "rate_limit", "unknown"), "oss_mode": ("ok", "unavailable", "corrupt")}
            if set(updates) - set(validators) or any(value not in validators[name] for name, value in updates.items()):
                return self.respond(400)
            self.switches.update(updates)
            return self.json_reply(ok=True)
        if self.command == "POST" and path == "/__mock/seed-object":
            seed = json.loads(self.body())
            key = seed["key"]
            if not isinstance(key, str) or not key.startswith(PREFIX) or key in self.objects:
                return self.respond(400)
            self.objects[key] = bytes.fromhex(seed["contentHex"])
            return self.json_reply(ok=True)
        return self.respond(404)

    def handle_method(self):
        parsed = urlsplit(self.path)
        query = parse_qs(parsed.query, keep_blank_values=True)
        with self.lock:
            try:
                if parsed.path == "/health" and self.command == "GET":
                    return self.json_reply(ok=True)
                if parsed.path.startswith("/__mock/"):
                    return self.control(parsed.path)
                if parsed.path == "/authorize" and self.command == "GET":
                    return self.authorize(query)
                if parsed.path in ("/cgi-bin/gettoken", "/cgi-bin/auth/getuserinfo", "/cgi-bin/message/send"):
                    return self.wecom(parsed.path, query)
                return self.oss(parsed.path, query)
            except (ValueError, TypeError, KeyError):
                return self.respond(400)

    do_GET = do_POST = do_PUT = do_HEAD = do_DELETE = handle_method


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Provider).serve_forever()
