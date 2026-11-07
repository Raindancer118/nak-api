# Certificate + proxy host for a friend's naknak on NPMplus (run on the proxy host).
#
#   python3 npm-friend.py add|remove <fqdn> <api> <user> <forward host> <forward port> < password
#
# Safe to run again. remove keeps the certificate: deleting one makes NPMplus
# revoke it at Let's Encrypt.
import json
import ssl
import sys
import urllib.request
from http.cookiejar import CookieJar

action, fqdn, api, user, fhost, fport = sys.argv[1:7]
password = sys.stdin.readline().rstrip("\n")

ctx = ssl.create_default_context()
ctx.check_hostname = False  # the admin API on localhost:81 has a self-signed certificate
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=ctx), urllib.request.HTTPCookieProcessor(CookieJar()))


def call(method, path, body=None):
    req = urllib.request.Request(f"{api}/{path}", method=method, headers={"Content-Type": "application/json"},
                                 data=None if body is None else json.dumps(body).encode())
    with opener.open(req) as r:
        raw = r.read()
    return json.loads(raw) if raw.strip() else {}


def fail(msg):
    print("ERR", msg)
    sys.exit(1)


try:
    call("POST", "tokens", {"identity": user, "secret": password})["expires"]
except Exception:
    fail("NPM login failed")

host = next((h["id"] for h in call("GET", "nginx/proxy-hosts") if h["domain_names"] == [fqdn]), None)
cert = next((c["id"] for c in call("GET", "nginx/certificates") if c["domain_names"] == [fqdn]), None)

if action == "remove":
    if host:
        call("DELETE", f"nginx/proxy-hosts/{host}")
        print(f"host {host} removed")
    if cert:
        print(f"certificate {cert} kept (deleting revokes it)")
    sys.exit(0)

if not cert:
    try:
        cert = call("POST", "nginx/certificates", {"provider": "letsencrypt", "nice_name": fqdn, "domain_names": [fqdn],
                                                   "meta": {"dns_challenge": False}})["id"]
    except Exception:
        fail(f"certificate for {fqdn} failed (DNS pointing here?)")
print(f"certificate {cert}")

body = dict(forward_scheme="http", forward_host=fhost, forward_port=int(fport), certificate_id=cert, ssl_forced=True,
            http2_support=True, block_exploits=True, allow_websocket_upgrade=True)
if not host:
    try:
        host = call("POST", "nginx/proxy-hosts", dict(body, domain_names=[fqdn]))["id"]
    except Exception:
        fail("proxy host failed")
# NPMplus applies HSTS and noindex only on update
call("PUT", f"nginx/proxy-hosts/{host}", dict(body, hsts_enabled=True, npmplus_noindex=True))
print(f"host {host}")
