#!/bin/bash
# Update mathgame from master and restart services.
# Run from the repo root. Idempotent (re-run after any change).
set -euo pipefail

SERVICES=(
    mathgame-api
    mathgame-web
    mathgame-maintenance
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

# Rebuild from whatever is currently checked out, before touching any
# service. build-web stages into web/build.next and swaps, so the running
# web server keeps serving valid content through the whole build and a
# failed build (set -e aborts) leaves the live site untouched.
make

# Sync systemd unit files to /etc/systemd/system.
for s in "${SERVICES[@]}"; do
    sudo cp "deploy/${s}.service" /etc/systemd/system/
done
for t in "${TIMERS[@]}"; do
    sudo cp "deploy/${t}.timer" /etc/systemd/system/
done

sudo systemctl daemon-reload

# Serve the maintenance page during the disruptive window (Conflicts= in the
# unit stops mathgame-web). If anything below fails, set -e exits with the
# maintenance page still up - users see "down for maintenance", not errors.
sudo systemctl start mathgame-maintenance

sudo systemctl restart mathgame-api

# Restart timers (in case schedule changed).
for t in "${TIMERS[@]}"; do
    sudo systemctl restart "${t}.timer"
done

# Back to the real web server (Conflicts= stops the maintenance page).
sudo systemctl start mathgame-web

echo "Update complete."
