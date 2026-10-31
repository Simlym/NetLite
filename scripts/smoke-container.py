"""Black-box Docker check: local accounts, native DDNS, certificates and persistence."""
import argparse
import http.cookiejar
import json
import ssl
import subprocess
import time
import urllib.error
import urllib.request
import uuid


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()


def eventually(check, timeout=60):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (OSError, ValueError, subprocess.CalledProcessError) as error:
            last_error = error
        time.sleep(0.5)
    raise RuntimeError(f"Timed out waiting for container: {last_error}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", default="netlite:smoke")
    args = parser.parse_args()
    name = "netlite-smoke-" + uuid.uuid4().hex
    volume = name + "-data"
    token = ""
    csrf = ""
    base = ""
    password = "container-smoke-" + uuid.uuid4().hex
    cookies = http.cookiejar.CookieJar()
    # Ignore developer/runner HTTP proxy variables for these loopback requests.
    client = urllib.request.build_opener(
        urllib.request.ProxyHandler({}),
        urllib.request.HTTPCookieProcessor(cookies),
        urllib.request.HTTPSHandler(context=ssl._create_unverified_context()),
    )

    def request(path, method="GET", body=None, authorized=True, url=None, host=None):
        headers = {}
        if authorized:
            headers["Origin"] = base
            headers["X-CSRF-Token"] = csrf
        if host:
            headers["Host"] = host
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request((url or base) + path, data=data, headers=headers, method=method)
        with client.open(req, timeout=5) as response:
            return response.read()

    def api(path, method="GET", body=None):
        return json.loads(request("/api/" + path, method, body))

    def port(number):
        return docker("port", name, f"{number}/tcp").splitlines()[0].rsplit(":", 1)[1]

    def launch():
        nonlocal base
        docker(
            "run", "-d", "--name", name, "--init", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges:true",
            "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m,mode=1777",
            "--mount", f"type=volume,src={volume},dst=/data",
            "-p", "127.0.0.1::18780", "-p", "127.0.0.1::18080",
            "-p", "127.0.0.1::18443", args.image,
        )
        base = "http://127.0.0.1:" + port(18780)
        eventually(lambda: json.loads(request("/healthz", authorized=False))["status"] == "ok")

    def healthy():
        return docker("inspect", "--format", "{{.State.Health.Status}}", name) == "healthy"

    def proxy_works():
        plain = "http://127.0.0.1:" + port(18080)
        secure = "https://localhost:" + port(18443)
        # SNI must match the certificate policy; use curl inside the container for TLS.
        assert json.loads(request("/healthz", url=plain, host="plain.localhost", authorized=False))["status"] == "ok"
        result = docker("exec", name, "curl", "-fkSs", "--resolve", "localhost:18443:127.0.0.1",
                        "https://localhost:18443/healthz")
        assert json.loads(result)["status"] == "ok"
        # The published TLS port must also accept a TLS handshake.
        assert json.loads(request("/healthz", url=secure, host="localhost", authorized=False))["status"] == "ok"
        return True

    def ddns_observed():
        state = api("dns/")["tasks"][0]["state"]
        return (state["result_known"] and state["ipv4_result"] == "ip_failed"
                and state["provider_requests"] == 0 and state["update_requests"] == 0)

    docker("volume", "create", volume)
    try:
        launch()
        token = docker("exec", name, "cat", "/data/admin-token")
        assert len(token) >= 32
        assert not api("auth/state")["initialized"]
        api("auth/setup", "POST", dict(username="admin", password=password, code=token))
        csrf = api("auth/me")["csrf_token"]
        eventually(lambda: api("status")["proxy"]["running"])
        assert docker("exec", name, "id", "-u") == "10001"
        # Direct Go builds can report (devel); check the required runtime module.
        modules = docker("exec", name, "/opt/netlite/bin/caddy", "list-modules")
        assert "http.handlers.reverse_proxy" in modules
        cookies.clear()
        try:
            request("/api/config", authorized=False)
            raise AssertionError("Unauthenticated API accepted")
        except urllib.error.HTTPError as error:
            assert error.code == 401
        api("auth/login", "POST", dict(username="admin", password=password))
        csrf = api("auth/me")["csrf_token"]
        assert b"<html" in request("/", authorized=False).lower()
        config = api("config")
        config["routes"] = [
            dict(id="http", domain="plain.localhost", scheme="http", port=18080,
                 upstream="http://127.0.0.1:18780", certificate_id="", enabled=True),
            dict(id="https", domain="localhost", scheme="https", port=18443,
                 upstream="http://127.0.0.1:18780", certificate_id="local", enabled=True),
        ]
        config["certificates"] = [dict(id="local", domains=["localhost"], mode="internal")]
        saved = api("config", "PUT", config)
        eventually(lambda: api("certificates/")["certificates"]["local"]["status"] == "ready")
        api("proxy/validate", "POST")
        api("proxy/apply", "POST")
        eventually(proxy_works)
        # Leave an unapplied draft and ensure recreation restores only the applied rules.
        saved["routes"][0]["enabled"] = False
        api("config", "PUT", saved)
        api("dns/accounts/smoke", "PUT", dict(
            id="smoke", name="Container test", provider="dnspod",
            credential_id="not-real", secret="not-a-real-secret",
        ))
        # This endpoint returns JSON without an IP; the engine must never contact DNS.
        api("dns/tasks/smoke", "PUT", dict(
            id="smoke", name="Container IP failure", account_id="smoke",
            ipv4_domains=["smoke.example.invalid"], ipv6_domains=[],
            ipv4_url="http://127.0.0.1:18780/healthz", ipv6_url="",
            interval_seconds=300, ttl="600", enabled=True,
        ))
        eventually(ddns_observed)
        eventually(healthy)
        key_hash = docker("exec", name, "sha256sum", "/data/dns/master.key")
        root_hash = docker("exec", name, "sha256sum", "/data/certificates/internal-ca.json")
        docker("stop", "--time", "30", name)
        assert docker("inspect", "--format", "{{.State.ExitCode}}", name) == "0"
        docker("rm", name)
        launch()
        assert docker("exec", name, "cat", "/data/admin-token") == token
        cookies.clear()
        api("auth/login", "POST", dict(username="admin", password=password))
        csrf = api("auth/me")["csrf_token"]
        assert docker("exec", name, "sha256sum", "/data/dns/master.key") == key_hash
        eventually(proxy_works)
        assert docker("exec", name, "sha256sum", "/data/certificates/internal-ca.json") == root_hash
        assert api("config")["routes"][0]["enabled"] is False
        # Successful startup of the saved task requires decrypting its saved account.
        eventually(ddns_observed)
        eventually(healthy)
        docker("stop", "--time", "30", name)
        assert docker("inspect", "--format", "{{.State.ExitCode}}", name) == "0"
        print("PASS: non-root/read-only container, local account, embedded frontend, HTTP/HTTPS, "
              "native DDNS without DNS writes, graceful stop, recreation preserves account/key/CA/draft/applied config")
    except Exception:
        subprocess.run(["docker", "logs", "--tail", "80", name], check=False)
        raise
    finally:
        subprocess.run(["docker", "rm", "-f", name], check=False, stdout=subprocess.DEVNULL)
        subprocess.run(["docker", "volume", "rm", volume], check=False, stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
