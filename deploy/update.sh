#!/bin/bash
# Update mathgame from master and restart services.
# Run from the repo root. Idempotent (re-run after any change).
set -euo pipefail

SERVICES=(
    mathgame-api
    mathgame-compress-events
    mathgame-check-disabled-videos
    mathgame-update-statistics
    mathgame-trim-recently-shown-problems
    mathgame-watchdog
)
TIMERS=(
    mathgame-compress-events
    mathgame-check-disabled-videos
    mathgame-update-statistics
    mathgame-trim-recently-shown-problems
    mathgame-watchdog
)
MAINTENANCE_FLAG=/var/www/mathgame/maintenance.on
# The flag only works if nginx checks the same path this script touches.
grep -qF "$MAINTENANCE_FLAG" deploy/nginx/mikeymath.conf || {
    echo "deploy/nginx/mikeymath.conf no longer checks $MAINTENANCE_FLAG" >&2
    exit 1
}

# Preflight the two conf.json fields the front door depends on, before the
# build touches anything: api_host is baked into the bundle, api_port must
# match the front door's proxy target (docs/ops-runbook.md).
python3 - <<'EOF'
import json, re, sys
conf = json.load(open("conf.json"))
nginx = open("deploy/nginx/mikeymath.conf").read()
errs = []
m = re.search(r"proxy_pass http://127\.0\.0\.1:(\d+);", nginx)
if not m:
    errs.append("deploy/nginx/mikeymath.conf has no proxy_pass http://127.0.0.1:<port>;")
elif conf.get("api_port") != m.group(1):
    errs.append(f"conf.json api_port {conf.get('api_port')!r} != front-door proxy_pass port {m.group(1)!r}")
if conf.get("api_host") != "https://mikeymath.org":
    errs.append(f"conf.json api_host {conf.get('api_host')!r} != 'https://mikeymath.org' (the origin baked into the bundle)")
if errs:
    print("preflight failed:\n  " + "\n  ".join(errs), file=sys.stderr)
    sys.exit(1)
EOF

# Rebuild from whatever is currently checked out, before touching any
# service. build-web stages into web/build.next and swaps, so nginx keeps
# serving valid content through the whole build and a failed build
# (set -e aborts) leaves the live site untouched.
make

# Sync systemd unit files to /etc/systemd/system.
for s in "${SERVICES[@]}"; do
    sudo cp "deploy/${s}.service" /etc/systemd/system/
done
for t in "${TIMERS[@]}"; do
    sudo cp "deploy/${t}.timer" /etc/systemd/system/
done

sudo systemctl daemon-reload

# Publish the bundle where nginx serves it (www-data must not need traversal
# into /home/ubuntu, where conf.json lives), with the same staged swap as
# build-web so the live dir always holds a complete copy.
sudo mkdir -p /var/www/mathgame
sudo rm -rf /var/www/mathgame/build.next /var/www/mathgame/build.prev
sudo cp -a web/build /var/www/mathgame/build.next
if [ -d /var/www/mathgame/build ]; then
    sudo mv /var/www/mathgame/build /var/www/mathgame/build.prev
fi
sudo mv /var/www/mathgame/build.next /var/www/mathgame/build
sudo cp deploy/maintenance.html /var/www/mathgame/

# Sync the front door. A config nginx rejects must not stay in conf.d - it
# would pass unnoticed now (the running nginx keeps its in-memory config) and
# take the site down at the next reload, e.g. certbot's renewal deploy-hook.
# The .bak suffix is outside the conf.d/*.conf include glob.
if [ -f /etc/nginx/conf.d/mikeymath.conf ]; then
    sudo cp /etc/nginx/conf.d/mikeymath.conf /etc/nginx/conf.d/mikeymath.conf.bak
fi
sudo cp deploy/nginx/mikeymath.conf /etc/nginx/conf.d/
if ! sudo nginx -t; then
    if [ -f /etc/nginx/conf.d/mikeymath.conf.bak ]; then
        sudo mv /etc/nginx/conf.d/mikeymath.conf.bak /etc/nginx/conf.d/mikeymath.conf
    else
        sudo rm /etc/nginx/conf.d/mikeymath.conf
    fi
    echo "nginx rejected the new config; previous config restored" >&2
    exit 1
fi
sudo systemctl reload nginx

# The disruptive window: nginx 503s every request while the flag exists, and
# a failure below leaves it up (set -e), so users see "down for maintenance".
sudo touch "$MAINTENANCE_FLAG"

sudo systemctl restart mathgame-api

# Restart timers (in case schedule changed).
for t in "${TIMERS[@]}"; do
    sudo systemctl restart "${t}.timer"
done

# apiserver readiness, on loopback with the maintenance page still up (the
# flag only gates nginx): it was just restarted and runs migrations before it
# listens, so 000 (refused) retries; the app answers unauthenticated requests
# 401. The port comes from conf.json, which the preflight pinned to the
# front door's proxy target.
api_port=$(python3 -c 'import json; print(json.load(open("conf.json"))["api_port"])')
api_status=""
for _ in $(seq 1 150); do
    api_status=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
        "http://127.0.0.1:${api_port}/api/v1/problems/1" || true)
    [ "$api_status" = 000 ] || break
    sleep 2
done
if [ "$api_status" != 401 ]; then
    echo "apiserver readiness check failed (got ${api_status}, want 401); maintenance page left up" >&2
    exit 1
fi

sudo rm -f "$MAINTENANCE_FLAG"

RESOLVE="--resolve mikeymath.org:443:127.0.0.1"
smoke_fail() {
    sudo touch "$MAINTENANCE_FLAG"
    echo "smoke check failed on $1; maintenance page re-raised" >&2
    exit 1
}

# Smoke-check through the real front door; a failure re-raises the flag so a
# broken deploy ends on the maintenance page, not on errors.
for path in / /play; do
    if ! curl -fsS -o /dev/null --max-time 10 $RESOLVE \
        "https://mikeymath.org${path}"; then
        smoke_fail "${path}"
    fi
done

# The API wiring: apiserver is already up (readiness above), so anything but
# its 401 here is the proxy's failure.
api_status=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 $RESOLVE \
    "https://mikeymath.org/api/v1/problems/1" || true)
if [ "$api_status" != 401 ]; then
    smoke_fail "/api/v1/problems/1 (got ${api_status}, want 401)"
fi

echo "Update complete."
