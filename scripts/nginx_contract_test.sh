#!/usr/bin/env bash
# Front-door contract tests (`make test-nginx`); scope and rationale in
# docs/ops-runbook.md ("The front door"). Runs deploy/nginx/mikeymath.conf
# itself against a fixture build dir and a stub API upstream, substituting
# only ports, cert paths, the three content roots and the proxy target.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF_SRC="$REPO_ROOT/deploy/nginx/mikeymath.conf"
PAGE_SRC="$REPO_ROOT/deploy/maintenance.html"

HTTP_PORT=18080
HTTPS_PORT=18443
API_PORT=18081

if ! command -v nginx >/dev/null; then
    echo "nginx not installed (macOS: brew install nginx; Ubuntu: sudo apt-get install -y nginx)" >&2
    exit 1
fi

WORK="$(mktemp -d)"
NGINX_PID=""
API_PID=""
cleanup() {
    if [ -n "$NGINX_PID" ]; then kill "$NGINX_PID" 2>/dev/null || true; fi
    if [ -n "$API_PID" ]; then
        kill "$API_PID" 2>/dev/null || true
        wait "$API_PID" 2>/dev/null || true
    fi
    rm -rf "$WORK"
}
trap cleanup EXIT

failures=0
fail() {
    echo "FAIL: $*" >&2
    failures=$((failures + 1))
}
pass() { echo "ok: $*"; }

# ---------------------------------------------------------------- staging ---

STAGED="$WORK/mikeymath.conf"
cp "$CONF_SRC" "$STAGED"

# Each substitution must find its pattern, so a rename in the deploy copy
# breaks the test loudly instead of quietly making it vacuous.
sub() {
    grep -qF -- "$1" "$STAGED" || {
        echo "FAIL: $CONF_SRC no longer contains '$1'; update this test alongside it" >&2
        exit 1
    }
    python3 -c 'import sys, pathlib
p = pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace(sys.argv[2], sys.argv[3]))' \
        "$STAGED" "$1" "$2"
}

# A stand-in for web/build with the pieces the routing contract touches.
mkdir -p "$WORK/build/static/js" "$WORK/build/static/css" "$WORK/www" \
    "$WORK/tmp" "$WORK/acme/.well-known/acme-challenge"
echo "<title>LANDING-MARKER</title>" > "$WORK/build/index.html"
echo "<title>APP-SHELL-MARKER</title>" > "$WORK/build/app.html"
echo "<title>PRIVACY-MARKER</title>" > "$WORK/build/privacy.html"
echo "<title>NOT-FOUND-MARKER</title>" > "$WORK/build/404.html"
python3 -c "print('BUNDLE-MARKER();' * 200)" > "$WORK/build/static/js/main.js"
python3 -c "print('.bundle-marker{}' * 200)" > "$WORK/build/static/css/main.css"
cp "$PAGE_SRC" "$WORK/www/maintenance.html"
echo -n "ACME-TOKEN-VALUE" > "$WORK/acme/.well-known/acme-challenge/token"

openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
    -keyout "$WORK/key.pem" -out "$WORK/cert.pem" \
    -subj "/CN=mikeymath.org" \
    -addext "subjectAltName=DNS:mikeymath.org,DNS:www.mikeymath.org" 2>/dev/null

sub "listen 80;" "listen 127.0.0.1:$HTTP_PORT;"
sub "listen 443 ssl http2;" "listen 127.0.0.1:$HTTPS_PORT ssl http2;"
sub "proxy_pass http://127.0.0.1:8080;" "proxy_pass http://127.0.0.1:$API_PORT;"
sub "/etc/letsencrypt/live/mikeymath.org/fullchain.pem" "$WORK/cert.pem"
sub "/etc/letsencrypt/live/mikeymath.org/privkey.pem" "$WORK/key.pem"
sub "/var/www/mathgame/build" "$WORK/build"
sub "/var/www/mathgame" "$WORK/www"
sub "/var/www/html" "$WORK/acme"

# The inverse guard: a prod-only path or listener ADDED to the config would
# run here unsubstituted, silently shrinking coverage. Fail on any residue
# (ignoring comments and the staged $WORK paths the sub() calls just wrote).
residue=$(grep -vE "^[[:space:]]*#" "$STAGED" | grep -Fv "$WORK" |
    grep -E "/etc/|/var/|/home/|listen|proxy_pass" | grep -v "listen 127.0.0.1" |
    grep -v "proxy_pass http://127.0.0.1:$API_PORT;" || true)
if [ -n "$residue" ]; then
    printf 'FAIL: unsubstituted prod path/listener; add a sub() for it:\n%s\n' "$residue" >&2
    exit 1
fi

# Everything nginx writes is redirected under $WORK: an unprivileged run cannot
# create the compiled-in defaults (/var/lib/nginx, /run/nginx.pid).
cat > "$WORK/nginx.conf" <<EOF
pid $WORK/nginx.pid;
error_log $WORK/error.log warn;
events {}
http {
    access_log $WORK/access.log;
    client_body_temp_path $WORK/tmp/client;
    proxy_temp_path $WORK/tmp/proxy;
    fastcgi_temp_path $WORK/tmp/fastcgi;
    uwsgi_temp_path $WORK/tmp/uwsgi;
    scgi_temp_path $WORK/tmp/scgi;
    # The modern .js mapping (nginx >=1.21.5), which is what CI's nginx uses;
    # prod's 1.18 maps application/javascript, so gzip_types must carry both.
    types { text/html html; text/javascript js; text/css css; }
    default_type application/octet-stream;
    include $STAGED;
}
EOF

# A stand-in for apiserver: echoes the request path and the proxy headers
# back as JSON, so the tests can assert what nginx actually forwarded.
cat > "$WORK/api_stub.py" <<'EOF'
from http.server import BaseHTTPRequestHandler, HTTPServer
import json, sys

class Echo(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({
            "path": self.path,
            "host": self.headers.get("Host", ""),
            "proto": self.headers.get("X-Forwarded-Proto", ""),
        }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), Echo).serve_forever()
EOF
python3 "$WORK/api_stub.py" "$API_PORT" &
API_PID=$!

nginx -p "$WORK" -c "$WORK/nginx.conf" -g "daemon off;" &
NGINX_PID=$!

# wait_ready <url> [extra curl args...] -> polls until the URL answers 200
wait_ready() {
    local url=$1
    shift
    for _ in $(seq 1 50); do
        if [ "$(curl -sk -o /dev/null -w '%{http_code}' --max-time 1 \
            "$@" "$url" || true)" = 200 ]; then
            return 0
        fi
        sleep 0.2
    done
    return 1
}

if ! wait_ready "http://127.0.0.1:$API_PORT/ready"; then
    echo "FAIL: the API stub never became ready" >&2
    exit 1
fi
if ! wait_ready "https://mikeymath.org:$HTTPS_PORT/" \
    --resolve "mikeymath.org:$HTTPS_PORT:127.0.0.1"; then
    echo "FAIL: nginx never became ready; error log follows" >&2
    cat "$WORK/error.log" >&2 || true
    exit 1
fi

# ------------------------------------------------------------------ helpers --

# get <scheme> <host> <path> [extra curl args...] -> sets STATUS; headers land
# in $WORK/h, body in $WORK/b. curl transport failures leave STATUS=000 for
# the assertions to report rather than aborting the suite via set -e.
get() {
    local scheme=$1 host=$2 path=$3 port=$HTTPS_PORT
    shift 3
    [ "$scheme" = http ] && port=$HTTP_PORT
    STATUS=$(curl -sk -o "$WORK/b" -D "$WORK/h" -w '%{http_code}' \
        --resolve "$host:$port:127.0.0.1" "$@" "$scheme://$host:$port$path" || true)
}

header() { grep -i "^$1:" "$WORK/h" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r'; }

# body_is <marker> -> the last response body contains the marker
body_is() { grep -q "$1" "$WORK/b"; }

# ------------------------------------------------------------------- tests ---

get https www.mikeymath.org "/play?x=1"
if [ "$STATUS" = 301 ] && [ "$(header location)" = "https://mikeymath.org/play?x=1" ]; then
    pass "www on 443 redirects to the apex, path and query preserved"
else
    fail "www on 443: got $STATUS $(header location), want 301 https://mikeymath.org/play?x=1"
fi

for host in mikeymath.org www.mikeymath.org; do
    get http "$host" "/play?x=1"
    if [ "$STATUS" = 301 ] && [ "$(header location)" = "https://mikeymath.org/play?x=1" ]; then
        pass "$host on 80 redirects to https on the apex"
    else
        fail "$host on 80: got $STATUS $(header location), want 301 https://mikeymath.org/play?x=1"
    fi
done

get http mikeymath.org "/.well-known/acme-challenge/token"
if [ "$STATUS" = 200 ] && body_is "ACME-TOKEN-VALUE"; then
    pass "the ACME challenge path is served on 80, not redirected"
else
    fail "acme challenge: got $STATUS, want the token served with 200 (webroot renewal would break)"
fi

get https mikeymath.org "/"
if [ "$STATUS" = 200 ] && body_is "LANDING-MARKER"; then
    pass "the landing page is the document at /"
else
    fail "landing: got $STATUS, want 200 with index.html"
fi

# /play/ and /PLAY exercise the non-strict, case-insensitive match; React
# Router routes are case-insensitive, so the front door must be too.
for route in /login /play /settings /progress /admin /admin/deep /pin/target /play/ /PLAY; do
    get https mikeymath.org "$route"
    if [ "$STATUS" = 200 ] && body_is "APP-SHELL-MARKER"; then
        pass "$route serves the React shell"
    else
        fail "$route: got $STATUS, want 200 with app.html"
    fi
done

get https mikeymath.org "/api/v1/problems/42?x=1"
if [ "$STATUS" = 200 ] && body_is '"path": "/api/v1/problems/42?x=1"'; then
    pass "/api/ is proxied to the API upstream, path and query preserved"
else
    fail "/api/ proxy: got $STATUS body $(cat "$WORK/b"), want the upstream echo of the full path"
fi
if body_is '"host": "mikeymath.org"' && body_is '"proto": "https"'; then
    pass "the proxy forwards Host and X-Forwarded-Proto"
else
    fail "proxy headers: got $(cat "$WORK/b"), want Host mikeymath.org and X-Forwarded-Proto https"
fi

get https mikeymath.org "/privacy"
if [ "$STATUS" = 200 ] && body_is "PRIVACY-MARKER"; then
    pass "extensionless static pages resolve via their .html"
else
    fail "/privacy: got $STATUS, want 200 with privacy.html"
fi

for asset in /static/js/main.js /static/css/main.css; do
    get https mikeymath.org "$asset" -H "Accept-Encoding: gzip"
    if [ "$STATUS" = 200 ] && [ "$(header content-encoding)" = gzip ] &&
        gzip -dc < "$WORK/b" 2>/dev/null | grep -qi "bundle-marker"; then
        pass "$asset is served gzipped"
    else
        fail "$asset gzip: got $STATUS content-encoding=$(header content-encoding), want a gzipped 200"
    fi
done

get https mikeymath.org "/static/js/main.js"
if [ "$(header cache-control)" = "public, max-age=31536000, immutable" ]; then
    pass "hashed assets are cacheable forever"
else
    fail "asset cache-control: got '$(header cache-control)', want immutable max-age"
fi

get https mikeymath.org "/play"
if [ "$(header cache-control)" = no-cache ]; then
    pass "the shell revalidates on every load"
else
    fail "shell cache-control: got '$(header cache-control)', want no-cache (a stale shell requests deleted hashed bundles)"
fi

for path in /no-such-page /static/; do
    get https mikeymath.org "$path"
    if [ "$STATUS" = 404 ] && body_is "NOT-FOUND-MARKER"; then
        pass "$path is a real 404 with the branded page"
    else
        fail "$path: got $STATUS, want 404 with 404.html"
    fi
done

touch "$WORK/www/maintenance.on"
get https mikeymath.org "/play"
if [ "$STATUS" = 503 ] && [ "$(header retry-after)" = 120 ] &&
    [ "$(header cache-control)" = no-store ] && body_is "Mikey Math"; then
    pass "the maintenance flag serves the branded page as a 503"
else
    fail "maintenance flag: got $STATUS retry-after=$(header retry-after) cache-control=$(header cache-control), want 503 with the maintenance page"
fi
get https mikeymath.org "/api/v1/problems/42"
if [ "$STATUS" = 503 ]; then
    pass "the maintenance flag covers API requests too"
else
    fail "maintenance flag on /api/: got $STATUS, want 503 (the deploy window must close the API as well)"
fi
rm "$WORK/www/maintenance.on"

get https mikeymath.org "/play"
if [ "$STATUS" = 200 ] && body_is "APP-SHELL-MARKER"; then
    pass "removing the flag restores normal serving"
else
    fail "flag removal: got $STATUS, want 200 with app.html"
fi

if [ "$failures" -gt 0 ]; then
    echo "$failures nginx contract assertion(s) failed" >&2
    exit 1
fi
echo "nginx front-door contract OK"
