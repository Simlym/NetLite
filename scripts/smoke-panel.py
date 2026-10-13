"""Linux account, embedded frontend and management TLS integration."""
import http.cookiejar
import json
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

root = Path(__file__).resolve().parents[1]
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    port = sock.getsockname()[1]
with tempfile.TemporaryDirectory(prefix='netlite-panel-') as directory:
    data = Path(directory)
    binary = str(root / 'bin/netlite')
    subprocess.run([binary, 'init-admin', '--data', directory, '--temporary'], input='temporary long password\n', text=True, check=True)
    args = [binary, '--no-caddy', '--listen', f'127.0.0.1:{port}', '--data', directory]
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    csrf = ''
    base = f'http://localhost:{port}'

    def api(path, method='GET', body=None):
        headers = {'Content-Type': 'application/json', 'X-CSRF-Token': csrf, 'Origin': base}
        req = urllib.request.Request(base + '/api' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
        with opener.open(req, timeout=5) as response:
            return json.load(response)

    def denied(path, status, method='GET', body=None):
        try:
            api(path, method, body)
            raise AssertionError(f'{path}: unexpectedly authorized')
        except urllib.error.HTTPError as error:
            assert error.code == status, (path, error.code)

    def wait(fn):
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            try:
                value = fn()
                if value:
                    return value
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(.1)
        raise AssertionError('service not ready')

    with (data / 'backend.log').open('w') as log:
        process = subprocess.Popen(args, stdout=log, stderr=log)
        try:
            wait(lambda: api('/auth/state'))
            with opener.open(base, timeout=5) as response:
                html = response.read().decode()
                assert '/assets/' in html and '/src/main.ts' not in html, 'release frontend missing'
            denied('/config', 401)
            api('/auth/login', 'POST', {'username':'admin','password':'temporary long password'})
            me = api('/auth/me')
            csrf = me['csrf_token']
            assert me['must_change_password']
            denied('/config', 403)
            api('/auth/password', 'POST', {'old_password':'temporary long password','new_password':'personal long password'})
            csrf = api('/auth/me')['csrf_token']
            config = api('/config')
            config['certificates'] = [{'id':'panel','domains':['localhost'],'mode':'internal','staging':False,'email':''}]
            api('/config', 'PUT', config)
            wait(lambda: api('/certificates/')['certificates']['panel']['status'] == 'ready')
            with opener.open(base + '/api/certificates/root', timeout=5) as response:
                ca = response.read().decode()
            api('/panel', 'PUT', {'enabled':True,'domain':'localhost','certificate_id':'panel','redirect':True})
            assert api('/panel')['pending']
            denied('/panel/confirm', 422, 'POST')
            base = f'https://localhost:{port}'
            opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), urllib.request.HTTPSHandler(context=ssl.create_default_context(cadata=ca)))
            api('/auth/login', 'POST', {'username':'admin','password':'personal long password'})
            csrf = api('/auth/me')['csrf_token']
            api('/panel/confirm', 'POST')
            assert not api('/panel')['pending']
            config = api('/config')
            config['certificates'] = []
            denied('/config', 400, 'PUT', config)
            process.terminate()
            process.wait(timeout=20)
            process = subprocess.Popen(args, stdout=log, stderr=log)
            assert wait(lambda: api('/panel'))['enabled']
            subprocess.run([binary, 'reset-password', '--data', directory], input='recovered long password\n', text=True, check=True)
            denied('/config', 401)
            api('/auth/login', 'POST', {'username':'admin','password':'recovered long password'})
            csrf = api('/auth/me')['csrf_token']
            api('/auth/logout', 'POST')
            denied('/config', 401)
            print('Linux local authentication, embedded frontend and HTTPS: passed')
        finally:
            process.terminate()
            process.wait(timeout=20)
