#!/usr/bin/env python3
"""Build and deploy the current Ackwrap OpenWrt amd64 package."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parent
PACKAGE_PATH = PROJECT_ROOT / "dist" / "ackwrap_0.1.0-1_x86_64.ipk"
REMOTE_PACKAGE = "/tmp/ackwrap-deploy.ipk"


def run(command: list[str], *, cwd: Path | None = None) -> None:
    print(f"\n> {subprocess.list2cmdline(command)}", flush=True)
    subprocess.run(command, cwd=cwd, check=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build, upload, and reinstall Ackwrap on the OpenWrt test VM."
    )
    parser.add_argument("--host", default="192.168.59.2", help="OpenWrt host")
    parser.add_argument("--user", default="root", help="SSH user")
    parser.add_argument(
        "--key",
        type=Path,
        default=Path.home() / ".ssh" / "id_ed25519",
        help="SSH private key",
    )
    parser.add_argument(
        "--skip-build",
        action="store_true",
        help="Deploy the existing IPK without rebuilding",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    key = args.key.expanduser().resolve()
    if not key.is_file():
        raise SystemExit(f"SSH private key not found: {key}")

    if not args.skip_build:
        run(
            [sys.executable, "build.py", "--target", "openwrt", "--arch", "amd64"],
            cwd=PROJECT_ROOT,
        )
    if not PACKAGE_PATH.is_file():
        raise SystemExit(f"OpenWrt package not found: {PACKAGE_PATH}")

    destination = f"{args.user}@{args.host}"
    ssh_options = ["-i", str(key), "-o", "BatchMode=yes"]
    run(["scp", *ssh_options, str(PACKAGE_PATH), f"{destination}:{REMOTE_PACKAGE}"])

    install_command = (
        "if opkg status ackwrap >/dev/null 2>&1; then "
        f"opkg install --force-reinstall {REMOTE_PACKAGE}; "
        "else "
        f"opkg install {REMOTE_PACKAGE}; "
        "fi && "
        f"rm -f {REMOTE_PACKAGE} && "
        "sleep 2 && /etc/init.d/ackwrap status"
    )
    run(["ssh", *ssh_options, destination, install_command])
    print("\nOpenWrt deployment completed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode) from error
