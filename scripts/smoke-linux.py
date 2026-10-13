"""Isolated Linux integration check: native CA, real Caddy, restart and draft isolation."""
import http.server
import json
import os
from pathlib import Path
import signal
import socket
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

root = Path(__file__).resolve().parents[1]
data = Path(tempfile.mkdtemp(prefix="netlite-smoke-"))
caddy = os.environ["NETLITE_TEST_CADDY"]
base = "http://127.0.0.1:19780"
process = None
token = ""

class Upstream(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = ("upstream:" + self.path).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        pass

def api(path, method="GET", body=None, authenticated=True):
    headers = {"Content-Type": "application/json"}
    if authenticated:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(base + "/api" + path, None if body is None else json.dumps(body).encode(), headers, method=method)
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.load(response)

def wait(check):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (OSError, urllib.error.URLError, json.JSONDecodeError):
            pass
        time.sleep(0.1)
    raise RuntimeError("timeout; logs: " + str(data))

def start():
    global process, token
    process = subprocess.Popen([str(root / "bin/netlite"), "--auth-mode", "token", "--listen", "127.0.0.1:19780", "--data", str(data), "--web", str(root / "web/dist"), "--caddy", caddy, "--caddy-admin", "127.0.0.1:21190", "--acme-http-listen", "127.0.0.1:19882"], stdout=log, stderr=log)
    wait(lambda: (data / "admin-token").exists())
    token = (data / "admin-token").read_text().strip()
    wait(lambda: api("/status")["proxy"]["running"])

def stop():
    if process and process.poll() is None:
        process.send_signal(signal.SIGTERM)
        process.wait(timeout=20)

for port in (19780, 21190, 19881, 19081, 19444, 19882):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", port))
upstream = http.server.ThreadingHTTPServer(("127.0.0.1", 19881), Upstream)
threading.Thread(target=upstream.serve_forever, daemon=True).start()
with (data / "backend.log").open("w") as log:
    try:
        start()
        try:
            api("/config", authenticated=False)
            raise AssertionError("unauthenticated request accepted")
        except urllib.error.HTTPError as error:
            assert error.code == 401
        config = api("/config")
        config["certificates"] = [{"id": "local", "domains": ["localhost"], "mode": "internal", "email": "", "staging": False}]
        config["routes"] = [
            {"id": "plain", "domain": "plain.localhost", "scheme": "http", "port": 19081, "upstream": "http://127.0.0.1:19881", "certificate_id": "", "enabled": True},
            {"id": "secure", "domain": "localhost", "scheme": "https", "port": 19444, "upstream": "http://127.0.0.1:19881", "certificate_id": "local", "enabled": True},
        ]
        saved = api("/config", "PUT", config)
        try:
            api("/config", "PUT", config)
            raise AssertionError("stale update accepted")
        except urllib.error.HTTPError as error:
            assert error.code == 409
        wait(lambda: api("/certificates/")["certificates"]["local"]["status"] == "ready")
        api("/proxy/validate", "POST")
        api("/proxy/apply", "POST")
        request = urllib.request.Request(base + "/api/certificates/root", headers={"Authorization": "Bearer " + token})
        with urllib.request.urlopen(request) as response:
            ca = response.read()
        assert b"PRIVATE KEY" not in ca
        ca_path = data / "root.pem"
        ca_path.write_bytes(ca)
        with urllib.request.urlopen("https://localhost:19444/secure", context=ssl.create_default_context(cafile=str(ca_path))) as response:
            assert response.read() == b"upstream:/secure"
        saved["routes"][0]["enabled"] = False
        api("/config", "PUT", saved)
        stop()
        start()
        request = urllib.request.Request("http://127.0.0.1:19081/restart", headers={"Host": "plain.localhost"})
        with urllib.request.urlopen(request) as response:
            assert response.read() == b"upstream:/restart"
        assert not api("/config")["routes"][0]["enabled"]
        print("PASS: authentication, revision conflict, native CA, trusted HTTPS, applied restart and draft isolation")
    finally:
        stop()
        upstream.shutdown()
