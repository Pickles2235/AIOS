"""Owned TLS Git fixture exercising an external machine credential helper."""
from contextlib import contextmanager
import http.cookiejar
import http.server
import json
import os
from pathlib import Path
import shlex
import ssl
import subprocess
import threading
import urllib.error
import urllib.parse
import urllib.request


@contextmanager
def credential_remote(root, source):
    root = Path(root)
    root.mkdir(mode=0o700)
    bare = root / 'fixture.git'
    clean = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1')
    subprocess.run(['git', 'clone', '--bare', str(source), str(bare)], env=clean,
                   check=True, capture_output=True, timeout=30)
    subprocess.run(['git', '-C', str(bare), 'update-server-info'], env=clean,
                   check=True, capture_output=True, timeout=30)
    config = root / 'certificate.conf'
    config.write_text('[req]\nprompt=no\ndistinguished_name=dn\nx509_extensions=ext\n'
                      '[dn]\nCN=127.0.0.1\n[ext]\nsubjectAltName=IP:127.0.0.1\n'
                      'basicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,digitalSignature,keyEncipherment\n')
    certificate, key = root / 'ca.pem', root / 'key.pem'
    subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
                    '-days', '1', '-config', str(config), '-keyout', str(key),
                    '-out', str(certificate)], check=True, capture_output=True, timeout=30)
    marker = root / 'helper-called'
    helper = root / 'credential-helper'
    helper.write_text('#!/bin/sh\nif [ "$1" = get ]; then\n'
                      'printf called > ' + shlex.quote(str(marker)) + '\n'
                      "printf 'username=fixture\\npassword=fixture-only-password\\n\\n'\nfi\n")
    helper.chmod(0o700)
    profile = root / 'machine.gitconfig'
    subprocess.run(['git', 'config', '--file', str(profile), 'credential.helper',
                    '!'+shlex.quote(str(helper))], env=clean, check=True, timeout=10)
    profile.chmod(0o600)
    import base64
    expected = 'Basic ' + base64.b64encode(b'fixture:fixture-only-password').decode()
    accepted = threading.Event(); accepted.set()
    successful = threading.Event()

    class Handler(http.server.SimpleHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=str(root), **kwargs)

        def do_GET(self):
            if not accepted.is_set() or self.headers.get('Authorization') != expected:
                self.send_response(401); self.send_header('WWW-Authenticate', 'Basic realm="fixture"')
                self.end_headers(); return
            successful.set()
            super().do_GET()

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(certificate, key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    try:
        yield {'url': f'https://127.0.0.1:{server.server_port}/fixture.git',
               'environment': {'GIT_CONFIG_GLOBAL': str(profile), 'GIT_SSL_CAINFO': str(certificate)},
               'marker': marker, 'successful': successful, 'accept': accepted}
    finally:
        server.shutdown(); server.server_close(); thread.join(timeout=5)


def authenticated_api(launch_url):
    """Authenticate the actual supplied origin without proxies or DNS substitution."""
    parsed = urllib.parse.urlsplit(launch_url)
    origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
    if parsed.scheme != 'http' or not parsed.port:
        raise AssertionError('explicit local HTTP port required')
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                        urllib.request.HTTPCookieProcessor(jar))
    csrf = ''

    def api(path, body=None, expected=200):
        headers = {'Origin': origin, 'X-CSRF-Token': csrf}
        payload = None if body is None else json.dumps(body).encode()
        if payload is not None: headers['Content-Type'] = 'application/json'
        try:
            response = opener.open(urllib.request.Request(origin+path, data=payload,
                                                          headers=headers), timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            if response.code != expected: raise AssertionError('authenticated HTTP status boundary: '+path)
            data = response.read(1048577)
            if len(data) > 1048576: raise AssertionError('bounded HTTP response')
            return json.loads(data)

    token = urllib.parse.parse_qs(parsed.fragment)['token'][0]
    csrf = api('/api/v1/session', {'token': token})['csrf_token']
    return api
