#!/usr/bin/env python3
"""Create an isolated workspace containing one module and its local dependencies.

This validates unreleased monorepo changes without publishing versions or adding
local replace directives to the modules distributed to consumers.
"""
import json
import pathlib
import re
import sys

root = pathlib.Path(__file__).resolve().parent.parent
module_dir = (root / sys.argv[1]).resolve()
output = pathlib.Path(sys.argv[2]).resolve()
modules = {}
for manifest in root.rglob("go.mod"):
    if ".git" in manifest.parts or "vendor" in manifest.parts:
        continue
    source = manifest.read_text()
    name = re.search(r"^module\s+(\S+)", source, re.M).group(1)
    version = re.search(r"^go\s+([\d.]+)", source, re.M).group(1)
    modules[name] = (manifest.parent.resolve(), version, source)

selected = next(value for value in modules.values() if value[0] == module_dir)
pending = [selected]
included = {}
while pending:
    directory, version, source = pending.pop()
    if directory in included:
        continue
    included[directory] = version
    for name, dependency in modules.items():
        if re.search(r"(?m)^\s*(?:require\s+)?" + re.escape(name) + r"\s+v", source):
            pending.append(dependency)

target_version = selected[1]
version_key = lambda version: tuple(map(int, version.split(".")))
for directory, version in included.items():
    if version_key(version) > version_key(target_version):
        raise SystemExit(f"{directory} requires Go {version}, above {target_version} declared by {module_dir}")
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(
    f"go {target_version}\n\nuse (\n"
    + "".join("\t" + json.dumps(str(directory)) + "\n" for directory in sorted(included))
    + ")\n"
)
print(target_version)
