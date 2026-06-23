#!/usr/bin/env python3
"""Registry-driven documentation checks.

Reads the Project Areas registry block from README.md and enforces:
  - integrity: the registry parses and every row is well-formed; reports which area docs
    exist vs. are still TODO.
  - doc-touched (--base REF): a change to an area's owned files (its globs) on this branch
    must also touch that area's doc. Enforced only for areas whose doc already EXISTS, so
    undocumented areas don't block work until their doc is generated (enforcement opts in
    per area as docs land).

Usage:
  python3 scripts/docs_check.py                        # integrity only
  python3 scripts/docs_check.py --base origin/master   # + doc-touched vs. base
"""
import argparse
import fnmatch
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
README = os.path.join(ROOT, "README.md")
BEGIN = "<!-- BEGIN PROJECT-AREA REGISTRY"
END = "<!-- END PROJECT-AREA REGISTRY"


def parse_registry(text):
    s = text.find(BEGIN)
    e = text.find(END)
    if s < 0 or e < 0 or e < s:
        sys.exit("PROJECT-AREA REGISTRY block missing from README.md")
    block = text[text.find("\n", s) + 1:e]
    areas = []
    cur = None
    for line in block.splitlines():
        t = line.strip()
        if not t or t.startswith("```"):
            continue
        m = re.match(r"^(\S+)\s+doc=(\S+)\s+type=(\S+)$", t)
        if m:
            cur = {"name": m.group(1), "doc": m.group(2), "type": m.group(3), "globs": []}
            areas.append(cur)
            continue
        g = re.match(r"^globs:\s*(.+)$", t)
        if g and cur is not None:
            cur["globs"] = [x.strip() for x in g.group(1).split(",") if x.strip()]
    return areas


def matches(path, glob):
    if glob.endswith("/**"):
        return path.startswith(glob[:-2])  # directory prefix
    return fnmatch.fnmatch(path, glob)


def changed_files(base):
    """Files changed on this branch since `base`, committed OR not — so the check
    is useful locally (uncommitted work) and in CI (the PR's commits)."""
    def diff(*args):
        out = subprocess.run(
            ["git", "diff", "--name-only", *args], cwd=ROOT, capture_output=True, text=True,
        )
        if out.returncode != 0:
            sys.exit(f"git diff failed: {out.stderr.strip()}")
        return [l.strip() for l in out.stdout.splitlines() if l.strip()]
    # committed PR changes (merge-base..HEAD) ∪ uncommitted (working tree vs HEAD)
    return set(diff(f"{base}...HEAD")) | set(diff("HEAD"))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", help="git ref to diff against for the doc-touched check")
    args = ap.parse_args()

    areas = parse_registry(open(README).read())
    if not areas:
        sys.exit("no areas parsed from the registry block")
    for a in areas:
        for k in ("doc", "type", "globs"):
            if not a.get(k):
                sys.exit(f"registry area {a.get('name')!r} missing {k}")
        if a["type"] not in ("anchored", "prose"):
            sys.exit(f"registry area {a['name']!r} has bad type {a['type']!r}")

    exists = {a["name"]: os.path.exists(os.path.join(ROOT, a["doc"])) for a in areas}
    todo = [a["name"] for a in areas if not exists[a["name"]]]
    print(f"registry: {len(areas)} areas; {len(areas) - len(todo)} documented, {len(todo)} TODO")
    if todo:
        print("  TODO (no doc yet): " + ", ".join(todo))

    if not args.base:
        print("OK (integrity). Pass --base <ref> to run the doc-touched check.")
        return

    changed = set(changed_files(args.base))
    violations = []
    for a in areas:
        if not exists[a["name"]] or a["doc"] in changed:
            continue  # enforcement opts in once the doc exists; skip if doc was touched
        hit = [f for f in changed if any(matches(f, g) for g in a["globs"])]
        if hit:
            violations.append((a["name"], a["doc"], hit))
    if violations:
        print("\nDOC-TOUCHED violations (area code changed without updating its doc):")
        for name, doc, hit in violations:
            extra = " …" if len(hit) > 5 else ""
            print(f"  [{name}] update {doc} — changed: {', '.join(hit[:5])}{extra}")
        sys.exit(1)
    print("OK (doc-touched).")


if __name__ == "__main__":
    main()
