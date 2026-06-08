#!/bin/bash
# Update mathgame from master and restart services.
# Run from the repo root. Idempotent (re-run after any change).
set -euo pipefail

SERVICES=(
    mathgame-api
    mathgame-web
    mathgame-compress-events
    mathgame-check-disabled-videos
    mathgame-update-statistics
    mathgame-trim-recently-shown-problems
)
TIMERS=(
    mathgame-compress-events
    mathgame-check-disabled-videos
    mathgame-update-statistics
    mathgame-trim-recently-shown-problems
)

# Rebuild from whatever is currently checked out.
make

# Sync systemd unit files to /etc/systemd/system.
for s in "${SERVICES[@]}"; do
    sudo cp "deploy/${s}.service" /etc/systemd/system/
done
for t in "${TIMERS[@]}"; do
    sudo cp "deploy/${t}.timer" /etc/systemd/system/
done

sudo systemctl daemon-reload

# Restart long-running services.
sudo systemctl restart mathgame-api
sudo systemctl restart mathgame-web

# Restart timers (in case schedule changed).
for t in "${TIMERS[@]}"; do
    sudo systemctl restart "${t}.timer"
done

echo "Update complete."
