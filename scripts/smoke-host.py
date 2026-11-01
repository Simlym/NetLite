"""Linux Docker host smoke: non-root ambient capabilities, 80/443 and dynamic ports."""
import json
import socket
import ssl
import subprocess
import time
import urllib.error
import urllib.request
import uuid


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()


def main():
    import argparse
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", default="netlite:smoke")
    args = parser.parse_args()
    name = "netlite-host-smoke-" + uuid.uuid4().hex
    volume = name + "-data"
    token = ""
    base = "http://127.0.0.1:19781"
    client = urllib.request.build_opener(
        urllib.request.ProxyHandler({}),
        urllib.request.HTTPSHandler(context=ssl._create_unverified_context()),
    )

    def request(path, method="GET", body=None, url=None, host=None):
        headers = {"Authorization": "Bearer " + token}
        if host:
            headers["Host"] = host
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request((url or base) + path, data=data, headers=headers, method=method)
        with client.open(req, timeout=5) as response:
            return json.load(response)

    def api(path, method="GET", body=None):
        return request("/api/" + path, method, body)

    # CI's unprivileged runner cannot bind 80/443 to check availability.
    # Probe existing listeners; Caddy's real bind remains the definitive check.
    for port in (80, 443, 19781, 21191, 19445):
        for address in ("127.0.0.1", "::1"):
            try:
                connection = socket.create_connection((address, port), timeout=0.2)
            except OSError:
                continue
            connection.close()
            raise AssertionError(f"test port already occupied: {address}:{port}")

    docker("volume", "create", volume)
    try:
        docker("run", "-d", "--name", name, "--network=host", "--user=0:0",
               "--read-only", "--cap-drop=ALL", "--cap-add=NET_BIND_SERVICE", "--cap-add=SETUID",
               "--cap-add=SETGID", "--cap-add=SETPCAP", "--security-opt=no-new-privileges:true",
               "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m,mode=1777",
               "--mount", f"type=volume,src={volume},dst=/data", "-e", "NETLITE_NETWORK_MODE=host",
               "--entrypoint", "/opt/netlite/bin/netlite-host", args.image,
               "--auth-mode", "token", "--listen", "127.0.0.1:19781", "--data", "/data",
               "--caddy", "/opt/netlite/bin/caddy", "--caddy-admin", "127.0.0.1:21191")
        for _ in range(60):
            try:
                if request("/healthz")["status"] == "ok":
                    break
            except (OSError, ValueError):
                time.sleep(0.25)
        else:
            raise AssertionError("host service did not start")
        token = docker("exec", "--user=10001:10001", name, "cat", "/data/admin-token")
        for _ in range(60):
            if api("status")["proxy"]["running"]:
                break
            time.sleep(0.25)
        else:
            raise AssertionError("Caddy did not start")
        process_dump = docker("exec", "--user=10001:10001", name, "sh", "-c",
                              'for p in /proc/[0-9]*; do '
                              'case "$(cat "$p/comm" 2>/dev/null)" in netlite|caddy) '
                              'cat "$p/status"; echo END_PROCESS;; esac; done')
        processes = []
        for block in process_dump.split("END_PROCESS"):
            state = dict(line.split(":", 1) for line in block.strip().splitlines() if ":" in line)
            if state:
                processes.append(state)
        assert {p["Name"].strip() for p in processes} == {"netlite", "caddy"}
        for state in processes:
            assert set(state["Uid"].split()) == {"10001"}, state
            assert set(state["Gid"].split()) == {"10001"}, state
            for key in ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
                assert int(state[key].strip(), 16) == 0x400, (key, state)
            assert state["NoNewPrivs"].strip() == "1", state
        cfg = api("config")
        cfg["certificates"] = [dict(id="local", domains=["localhost"], mode="internal", email="", staging=False)]
        cfg["routes"] = [
            dict(id="plain", domain="plain.localhost", port=80, scheme="http", upstream=base, certificate_id="", enabled=True),
            dict(id="secure", domain="localhost", port=443, scheme="https", upstream=base, certificate_id="local", enabled=True),
        ]
        cfg = api("config", "PUT", cfg)
        for _ in range(60):
            if api("certificates/")["certificates"]["local"]["status"] == "ready":
                break
            time.sleep(0.25)
        api("proxy/apply", "POST")
        assert request("/healthz", url="http://127.0.0.1", host="plain.localhost")["status"] == "ok"
        assert request("/healthz", url="https://localhost")["status"] == "ok"
        # No container recreation or new Docker port mapping is needed.
        cfg["routes"].append(dict(id="extra", domain="extra.localhost", port=19445, scheme="http",
                                  upstream=base, certificate_id="", enabled=True))
        api("config", "PUT", cfg)
        api("proxy/apply", "POST")
        assert request("/healthz", url="http://127.0.0.1:19445", host="extra.localhost")["status"] == "ok"
        state = api("status")
        assert state["deployment"]["mode"] == "host"
        assert {80, 443, 19445}.issubset(state["proxy"]["listening_ports"])
        docker("stop", "--time", "30", name)
        assert docker("inspect", "--format", "{{.State.ExitCode}}", name) == "0"
        print("PASS: host bootstrap drops to UID/GID 10001, only NET_BIND_SERVICE inherited by NetLite/Caddy; "
              "HTTP 80, HTTPS 443, dynamic port without container recreation, graceful exit")
    except Exception:
        subprocess.run(["docker", "logs", "--tail", "80", name], check=False)
        raise
    finally:
        subprocess.run(["docker", "rm", "-f", name], check=False, stdout=subprocess.DEVNULL)
        subprocess.run(["docker", "volume", "rm", volume], check=False, stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
