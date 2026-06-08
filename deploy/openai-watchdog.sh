#!/bin/bash
# Alert via ntfy.sh on sustained "OpenAI error after retries" in the journal.
#
# Run every 5 minutes by mathgame-openai-watchdog.timer so a breakage is caught
# within minutes. greps the last hour of the mathgame-api journal and pushes a
# phone notification if the error count crosses THRESHOLD. The retry layer
# already filters transient blips, so a sustained count (not a single error) is
# the real signal. See issue #200.
#
# To avoid paging every 5 minutes while an outage persists, a stamp file
# rate-limits notifications to at most one per COOLDOWN window (1 hour).
#
# The ntfy topic comes from "ntfy_topic" in conf.json (gitignored, alongside
# the other secrets). It is effectively a shared secret: anyone who knows the
# topic can push to it. Empty/absent -> the watchdog quietly no-ops.
set -euo pipefail

THRESHOLD=5
COOLDOWN=3600  # seconds; at most one notification per hour
MATCH="OpenAI error after retries"
CONF="${1:-/home/ubuntu/mathgame_2/conf.json}"
STAMP="${STATE_FILE:-/run/mathgame-openai-watchdog.last}"

if [ ! -f "$CONF" ]; then
    echo "config $CONF not found; skipping watchdog." >&2
    exit 0
fi

topic=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("ntfy_topic",""))' "$CONF")
if [ -z "$topic" ]; then
    echo "ntfy_topic not set in $CONF; skipping watchdog." >&2
    exit 0
fi

# grep -c exits non-zero when the count is zero; don't let that trip set -e.
errors=$(journalctl -u mathgame-api --since "1 hour ago" --no-pager | grep -c "$MATCH" || true)

if [ "$errors" -lt "$THRESHOLD" ]; then
    exit 0
fi

# Sustained failure detected. Rate-limit: skip if we already paged this hour.
now=$(date +%s)
if [ -f "$STAMP" ]; then
    last=$(cat "$STAMP" 2>/dev/null || echo 0)
    if [ $(( now - last )) -lt "$COOLDOWN" ]; then
        exit 0
    fi
fi

body=$(journalctl -u mathgame-api --since "1 hour ago" --no-pager | grep "$MATCH" | tail -3)
# Only stamp the cooldown on a successful send, so a failed push retries next tick.
if curl -fsS \
    -H "Title: Mathgame: $errors OpenAI errors in last hour" \
    -H "Priority: high" \
    -H "Tags: warning" \
    -d "$body" \
    "ntfy.sh/$topic"; then
    echo "$now" > "$STAMP"
fi
