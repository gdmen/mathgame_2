#!/usr/bin/env python3
"""Generate the frontend config from the backend conf.json.

The React app (web/src/index.js, web/src/play.js) imports "./conf.json", and the
bundler inlines imported JSON *wholesale* into the public JS bundle.
The backend conf.json holds secrets (openai_api_key, youtube_api_key, mysql_pass,
ntfy_topic, DB creds), so it must never be the file the frontend imports -- doing
so publishes those secrets in the world-readable bundle. Instead we copy only the
public fields the frontend actually reads into web/src/conf.json.

Usage: gen_frontend_conf.py <backend_conf.json> <frontend_conf.json>
"""
import json
import os
import re
import sys

# The only fields web/src/*.js reads. Keep in sync with usages of `conf.` there.
PUBLIC_FIELDS = (
    "api_host",
    "event_reporting_interval",
    "auth0_audience",
    "auth0_clientId",
    "auth0_domain",
    "debug_quickplay",
)


def generate(src, dst):
    if not os.path.exists(src):
        sys.exit(f"{src} not found; copy conf.json_ to {src} and fill it in.")
    with open(src) as f:
        backend = json.load(f)
    frontend = {k: backend[k] for k in PUBLIC_FIELDS if k in backend}

    # api_host is the full origin the bundle dials; a wrong-but-parseable
    # value (like the stale portless "http://localhost") bakes a dead URL
    # with no other signal.
    api_host = frontend.get("api_host", "")
    if not re.match(r"^https?://[^/]+$", api_host) or api_host == "http://localhost":
        sys.exit(
            f"{src}: api_host {api_host!r} must be the full API origin the "
            "bundle calls, e.g. http://localhost:8080 (dev) or "
            "https://mikeymath.org (prod); see conf.json_"
        )

    # The previous build symlinked dst -> backend conf.json. Unlink first so we
    # never follow that symlink and truncate the real backend config.
    if os.path.lexists(dst):
        os.remove(dst)
    with open(dst, "w") as f:
        json.dump(frontend, f, indent=2)
        f.write("\n")


def main(argv):
    if len(argv) != 3:
        sys.exit("usage: gen_frontend_conf.py <backend_conf> <frontend_conf>")
    generate(argv[1], argv[2])


if __name__ == "__main__":
    main(sys.argv)
