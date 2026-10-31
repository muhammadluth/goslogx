#!/usr/bin/env python3
"""Merge Go profiles by source block, then enforce production-module coverage.

Dependency code exercised by a plugin's tests belongs to its defining module.
Repeated blocks from multiple test binaries count once in the denominator. The
patch figure is a local statement-block estimate, not Codecov's hosted status.
"""
import argparse
import collections
import json
import pathlib
import re
import subprocess

PREFIX = "github.com/muhammadluth/goslogx/"


def merge_profiles(paths):
    blocks = {}
    for path in paths:
        for line in path.read_text().splitlines()[1:]:
            location, statements, count = line.split()
            if not location.startswith(PREFIX):
                continue
            key = location.removeprefix(PREFIX)
            statements, count = int(statements), int(count)
            previous = blocks.get(key)
            if previous and previous[0] != statements:
                raise ValueError(f"incompatible coverage profiles: {key}")
            blocks[key] = (statements, count + (previous[1] if previous else 0))
    return blocks


def changed_lines(root, base):
    diff = subprocess.check_output(
        ["git", "diff", "--no-ext-diff", "--unified=0", base, "--", "*.go"], cwd=root, text=True
    )
    changed = collections.defaultdict(set)
    filename = None
    for line in diff.splitlines():
        if line.startswith("+++ b/"):
            filename = line[6:]
        elif filename and (match := re.match(r"@@ .* \+(\d+)(?:,(\d+))? @@", line)):
            start, length = int(match[1]), int(match[2] or 1)
            changed[filename].update(range(start, start + length))
    untracked = subprocess.check_output(
        ["git", "ls-files", "--others", "--exclude-standard", "-z"], cwd=root
    ).decode().split("\0")
    for filename in untracked:
        if filename.endswith(".go"):
            changed[filename].update(range(1, len((root / filename).read_text().splitlines()) + 1))
    return changed


def percentage(covered, total):
    return 100.0 * covered / total if total else 100.0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profiles", type=pathlib.Path)
    parser.add_argument("--minimum", type=float, default=90)
    parser.add_argument("--base", default="HEAD")
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parent.parent
    manifests = sorted(p for p in root.rglob("go.mod") if "vendor" not in p.parts and ".git" not in p.parts)
    modules = [str(p.parent.relative_to(root)) for p in manifests]
    # Require the current inventory. Do not silently reuse a partial sweep.
    paths = [args.profiles / (module.replace("/", "_") + ".out") for module in modules]
    blocks = merge_profiles(paths)
    changed = changed_lines(root, args.base)
    totals = {module: [0, 0] for module in modules}
    patch = [0, 0]
    project = [0, 0]
    for key, (statements, count) in blocks.items():
        filename, span = key.rsplit(":", 1)
        if filename.startswith("examples/"):
            continue
        owners = [module for module in modules if module == "." or filename.startswith(module + "/")]
        owner = max(owners, key=len)
        covered = statements if count else 0
        totals[owner][0] += covered
        totals[owner][1] += statements
        project[0] += covered
        project[1] += statements
        start, end = (int(part.split(".")[0]) for part in span.split(","))
        if changed[filename].intersection(range(start, end + 1)):
            patch[0] += covered
            patch[1] += statements
    results = {}
    failed = False
    for module, (covered, total) in sorted(totals.items()):
        if module.startswith("examples/"):
            continue
        if not total:
            raise SystemExit(f"no production coverage recorded for {module}")
        value = percentage(covered, total)
        results[module] = {"covered": covered, "statements": total, "percent": value}
        print(f"{module:56s} {value:7.2f}% ({covered}/{total})")
        failed |= value < args.minimum
    for name, (covered, total) in (("project", project), ("patch (local block estimate)", patch)):
        value = percentage(covered, total)
        results[name] = {"covered": covered, "statements": total, "percent": value}
        print(f"{name:56s} {value:7.2f}% ({covered}/{total})")
        failed |= value < args.minimum
    (args.profiles / "summary.json").write_text(json.dumps(results, indent=2) + "\n")
    (args.profiles / "merged.cover").write_text(
        "mode: atomic\n" + "".join(f"{PREFIX}{key} {n} {count}\n" for key, (n, count) in sorted(blocks.items()))
    )
    if failed:
        raise SystemExit(f"coverage below {args.minimum:g}%")


if __name__ == "__main__":
    main()
