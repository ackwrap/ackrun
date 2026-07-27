#!/usr/bin/env python3
"""Trigger the Ackwrap OpenWrt release workflow."""

from __future__ import annotations

import re
import shutil
import subprocess
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parent
REPOSITORY = "ackwrap/ackrun"
WORKFLOW = "build-openwrt.yml"
RELEASE_REF = "main"
VERSION_PATTERN = re.compile(
    r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$"
)


def read_version() -> str:
    if len(sys.argv) > 2:
        raise SystemExit(f"Usage: {Path(sys.argv[0]).name} [version]")
    version = sys.argv[1] if len(sys.argv) == 2 else input("Release version: ")
    version = version.strip()
    if version.startswith("v"):
        version = version[1:]
    if not VERSION_PATTERN.fullmatch(version):
        raise SystemExit("Invalid version; use a value such as 0.1.5 or 0.1.5-beta.1")
    return version


def main() -> int:
    version = read_version()
    gh = shutil.which("gh")
    if gh is None:
        raise SystemExit("GitHub CLI (gh) was not found in PATH")

    command = [
        gh,
        "workflow",
        "run",
        WORKFLOW,
        "--repo",
        REPOSITORY,
        "--ref",
        RELEASE_REF,
        "--raw-field",
        f"version={version}",
    ]
    print(f"> {subprocess.list2cmdline(command)}", flush=True)
    subprocess.run(command, cwd=PROJECT_ROOT, check=True)
    print(f"Triggered v{version}: https://github.com/{REPOSITORY}/actions")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode) from error
