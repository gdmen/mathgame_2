#!/bin/bash
# Watchdog: alert via ntfy.sh on sustained error patterns in the journal.
#
# Run every 5 minutes by mathgame-watchdog.timer. For each entry in WATCHES, it
# greps the last hour of the UNIT journal and pushes a phone notification if the
# match count crosses that entry's threshold. The retry / self-heal layers
# already absorb transient blips, so a sustained count (not a single hit) is the
# real signal. See issue #200.
#
# To watch another error, add a line to WATCHES:
#     "slug|threshold|label|grep pattern"
#   slug       unique id; names the per-watch cooldown stamp file
#   threshold  minimum hits in the last hour before alerting
#   label      human text for the notification title ("N <label> in last hour")
#   pattern    grep pattern to count (may contain spaces; must not contain '|')
#
# Each watch is rate-limited independently to at most one notification per
# COOLDOWN window (1 hour) via $STATE_DIR/mathgame-watchdog-<slug>.last.
#
# The ntfy topic comes from "ntfy_topic" in conf.json (gitignored, alongside
# the other secrets). It is effectively a shared secret: anyone who knows the
# topic can push to it. Empty/absent -> the watchdog quietly no-ops.
set -euo pipefail

UNIT=mathgame-api
COOLDOWN=3600  # seconds; at most one notification per watch per hour
CONF="${1:-/home/ubuntu/mathgame_2/conf.json}"
STATE_DIR="${STATE_DIR:-/run}"

WATCHES=(
    "openai|5|OpenAI errors|OpenAI error"
)

if [ ! -f "$CONF" ]; then
    echo "config $CONF not found; skipping watchdog." >&2
    exit 0
fi

topic=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("ntfy_topic",""))' "$CONF")
if [ -z "$topic" ]; then
    echo "ntfy_topic not set in $CONF; skipping watchdog." >&2
    exit 0
fi

# Read the window once and grep it per watch, rather than re-querying journalctl.
log=$(journalctl -u "$UNIT" --since "1 hour ago" --no-pager)
now=$(date +%s)

for watch in "${WATCHES[@]}"; do
    IFS='|' read -r slug threshold label pattern <<< "$watch"

    # grep -c exits non-zero when the count is zero; don't let that trip set -e.
    count=$(grep -c "$pattern" <<< "$log" || true)
    if [ "$count" -lt "$threshold" ]; then
        continue
    fi

    # Rate-limit per watch: skip if we already paged for this slug this hour.
    stamp="$STATE_DIR/mathgame-watchdog-${slug}.last"
    if [ -f "$stamp" ]; then
        last=$(cat "$stamp" 2>/dev/null || echo 0)
        if [ $(( now - last )) -lt "$COOLDOWN" ]; then
            continue
        fi
    fi

    body=$(grep "$pattern" <<< "$log" | tail -3)
    # Only stamp the cooldown on a successful send, so a failed push retries.
    if curl -fsS \
        -H "Title: Mathgame: $count $label in last hour" \
        -H "Priority: high" \
        -H "Tags: warning" \
        -d "$body" \
        "ntfy.sh/$topic"; then
        echo "$now" > "$stamp"
    fi
done
