#!/usr/bin/env python3
"""Fail if the built web bundle contains any secret.

Two independent checks, both fatal:

1. Value scan (primary): no value of a SECRET field from conf.json may appear in
   the build. "Secret" = any conf field NOT in gen_frontend_conf.PUBLIC_FIELDS,
   so this list never drifts from the frontend whitelist. This directly prevents
   recurrence of #219 (secrets symlinked/imported into the public bundle), and
   doubles as a pre-deploy check you can run locally against the real bundle:
       make build-web && make check-bundle-secrets

2. Pattern scan (defense in depth): no well-known secret format (OpenAI,
   Anthropic, Google API key, PEM private key) may appear, catching secrets that
   reach the bundle from outside conf.json (e.g. hardcoded in source).

Usage: check_bundle_secrets.py <conf.json> <build_dir>
"""
import json
import os
import re
import sys

from gen_frontend_conf import PUBLIC_FIELDS

# Below this length, config values are usually non-secret (ports, "root",
# hostnames) and short enough to occur by chance in minified JS, so the value
# scan skips them to avoid false positives. Real keys/passwords are far longer.
MIN_SECRET_LEN = 8

# High-signal formats only -- specific enough not to match minified JS by chance.
SECRET_PATTERNS = {
    "OpenAI project key": r"sk-proj-[A-Za-z0-9_-]{20,}",
    "OpenAI service-account key": r"sk-svcacct-[A-Za-z0-9_-]{20,}",
    "OpenAI key": r"sk-[A-Za-z0-9]{40,}",
    "Anthropic key": r"sk-ant-[A-Za-z0-9_-]{20,}",
    "Google API key": r"AIza[0-9A-Za-z_-]{35}",
    "PEM private key": r"-----BEGIN [A-Z ]*PRIVATE KEY-----",
}


def secret_variants(value):
    # webpack inlines imported JSON as escaped JS string literals, so a value
    # with quotes/backslashes/non-ASCII appears escaped in the bundle. Match
    # both the raw value and its JSON-escaped inner form.
    return {value, json.dumps(value)[1:-1]}


def iter_text_files(build_dir):
    for root, _, files in os.walk(build_dir):
        for name in files:
            path = os.path.join(root, name)
            try:
                with open(path, encoding="utf-8", errors="ignore") as f:
                    yield path, f.read()
            except OSError:
                continue


def find_leaks(conf, build_dir):
    secret_values = {
        k: v
        for k, v in conf.items()
        if k not in PUBLIC_FIELDS
        and isinstance(v, str)
        and len(v.strip()) >= MIN_SECRET_LEN
    }
    patterns = {name: re.compile(p) for name, p in SECRET_PATTERNS.items()}

    leaks = []
    for path, text in iter_text_files(build_dir):
        rel = os.path.relpath(path, build_dir)
        for field, value in secret_values.items():
            if any(variant in text for variant in secret_variants(value)):
                leaks.append(f"conf secret '{field}' -> {rel}")
        for name, pat in patterns.items():
            if pat.search(text):
                leaks.append(f"{name} pattern -> {rel}")
    return sorted(set(leaks)), len(secret_values), len(patterns)


def main(argv):
    if len(argv) != 3:
        sys.exit("usage: check_bundle_secrets.py <conf.json> <build_dir>")
    conf_path, build_dir = argv[1], argv[2]
    if not os.path.isdir(build_dir):
        sys.exit(f"{build_dir} not found; run `make build-web` first.")
    with open(conf_path) as f:
        try:
            conf = json.load(f)
        except json.JSONDecodeError as e:
            sys.exit(f"{conf_path}: invalid JSON: {e}")
    if not isinstance(conf, dict):
        sys.exit(f"{conf_path}: expected a JSON object")

    leaks, n_secrets, n_patterns = find_leaks(conf, build_dir)
    if leaks:
        print("SECRET LEAK in web build artifacts:", file=sys.stderr)
        for leak in leaks:
            print(f"  {leak}", file=sys.stderr)
        sys.exit(1)
    print(
        f"OK: no secrets in {build_dir} "
        f"({n_secrets} conf secret fields + {n_patterns} patterns checked)."
    )


if __name__ == "__main__":
    main(sys.argv)
