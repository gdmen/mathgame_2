#!/usr/bin/env python3
"""Write a canary conf from a template: every string field set to a sentinel.

Lets the bundle secret scan (check_bundle_secrets.py) run in CI and locally
without real secrets -- the scan then verifies that none of these sentinels leak
into the public web bundle. Uses conf.json_ (the committed template) for the
field set, so it never touches a real conf.json.

Usage: gen_canary_conf.py <template_conf> <out_conf>
"""
import json
import sys


def generate(src, dst):
    with open(src) as f:
        conf = json.load(f)
    for k, v in conf.items():
        if isinstance(v, str):
            conf[k] = f"CANARY-{k}-d34db33f"
    with open(dst, "w") as f:
        json.dump(conf, f, indent=2)
        f.write("\n")


def main(argv):
    if len(argv) != 3:
        sys.exit("usage: gen_canary_conf.py <template_conf> <out_conf>")
    generate(argv[1], argv[2])


if __name__ == "__main__":
    main(sys.argv)
