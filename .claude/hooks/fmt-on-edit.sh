#!/bin/sh
# PostToolUse(Edit|Write): format the edited file via the Makefile, so the
# formatter definition lives in exactly one place (make fmt-file / fmt-web-file).
# Best-effort and never blocks the edit — always exits 0.

file=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)
[ -z "$file" ] && exit 0

case "$file" in
  */server/api/*.generated.go) : ;;                               # generated — never touch
  *.go)                make -s fmt-file FILE="$file" >/dev/null 2>&1 ;;
  */web/src/*.js|*/web/src/*.scss) make -s fmt-web-file FILE="$file" >/dev/null 2>&1 ;;
esac
exit 0
