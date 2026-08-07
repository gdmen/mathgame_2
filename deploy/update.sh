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

sudo rm -f "$MAINTENANCE_FLAG"

# Smoke-check through the real front door; a failure re-raises the flag so a
# broken deploy ends on the maintenance page, not on errors.
for path in / /play; do
    if ! curl -fsS -o /dev/null --resolve mikeymath.org:443:127.0.0.1 \
        "https://mikeymath.org${path}"; then
        sudo touch "$MAINTENANCE_FLAG"
        echo "smoke check failed on ${path}; maintenance page re-raised" >&2
        exit 1
    fi
done

echo "Update complete."
