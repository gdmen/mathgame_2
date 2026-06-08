#!/bin/bash
# Alert via ntfy.sh on sustained "OpenAI error after retries" in the journal.
#
# Run hourly by mathgame-openai-watchdog.timer. greps the last hour of the
# mathgame-api journal and pushes a phone notification if the error count
# crosses THRESHOLD. The retry layer already filters transient blips, so a
# sustained count (not a single error) is the real signal. See issue #200.
#
# The ntfy topic comes from "ntfy_topic" in conf.json (gitignored, alongside
# the other secrets). It is effectively a shared secret: anyone who knows the
# topic can push to it. Empty/absent -> the watchdog quietly no-ops.
set -euo pipefail

THRESHOLD=5
MATCH="OpenAI error after retries"
CONF="${1:-/home/ubuntu/mathgame_2/conf.json}"

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

if [ "$errors" -ge "$THRESHOLD" ]; then
    body=$(journalctl -u mathgame-api --since "1 hour ago" --no-pager | grep "$MATCH" | tail -3)
    curl -s \
        -H "Title: Mathgame: $errors OpenAI errors in last hour" \
        -H "Priority: high" \
        -H "Tags: warning" \
        -d "$body" \
        "ntfy.sh/$topic"
fi
