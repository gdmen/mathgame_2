#!/usr/bin/env python3
"""Generate the frontend config from the backend conf.json.

The React app (web/src/index.js, web/src/play.js) does require("./conf"), and
Create React App inlines imported JSON *wholesale* into the public JS bundle.
The backend conf.json holds secrets (openai_api_key, youtube_api_key, mysql_pass,
ntfy_topic, DB creds), so it must never be the file the frontend imports -- doing
so publishes those secrets in the world-readable bundle. Instead we copy only the
public fields the frontend actually reads into web/src/conf.json. See issue #219.

Usage: gen_frontend_conf.py <backend_conf.json> <frontend_conf.json>
"""
import json
import os
import sys

# The only fields web/src/*.js reads. Keep in sync with usages of `conf.` there.
PUBLIC_FIELDS = (
    "api_host",
    "api_port",
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
