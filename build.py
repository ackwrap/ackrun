#!/usr/bin/env python3
import argparse
import gzip
import io
import os
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parent
FRONTEND = ROOT / "frontend"
BACKEND = ROOT / "backend"
OPENWRT = ROOT / "openwrt"


def run(command: list[str], cwd: Path, env: dict[str, str] | None = None) -> None:
    print(f"[{cwd.name}] {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=cwd, env=env, check=True)


def npm_command() -> str:
    command = "npm.cmd" if os.name == "nt" else "npm"
    if shutil.which(command) is None:
        raise RuntimeError("npm was not found in PATH")
    return command


def build_frontend() -> None:
    npm = npm_command()
    if not (FRONTEND / "node_modules").exists():
        run([npm, "ci"], FRONTEND)
    run([npm, "run", "build"], FRONTEND)


def run_checks() -> None:
    run(["go", "test", "./..."], BACKEND)
    run(["go", "vet", "./..."], BACKEND)


def build_binary(target: str, arch: str, version: str, output_dir: Path) -> Path:
    goos = "windows" if target == "windows" else "linux"
    extension = ".exe" if target == "windows" else ""
    output = output_dir / f"ackwrap-{target}-{arch}{extension}"
    env = os.environ.copy()
    env.update({"GOOS": goos, "GOARCH": arch, "CGO_ENABLED": "0"})
    run(
        [
            "go",
            "build",
            "-trimpath",
            "-ldflags=-s -w "
            f"-X github.com/ackwrap/ackrun/internal/buildinfo.Version={version}",
            "-o",
            str(output),
            "./cmd/server",
        ],
        BACKEND,
        env,
    )
    return output


def executable_path(path: Path) -> bool:
    value = path.as_posix()
    return (
        "/etc/init.d/" in value
        or "/etc/uci-defaults/" in value
        or "/usr/libexec/" in value
        or path.name in {"preinst", "postinst", "prerm", "postrm"}
        or value.endswith("/usr/bin/ackwrap")
    )


def tar_gz_directory(root: Path) -> bytes:
    output = io.BytesIO()
    with gzip.GzipFile(fileobj=output, mode="wb", mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w") as archive:
            for source in sorted(root.rglob("*")):
                relative = source.relative_to(root).as_posix()
                info = archive.gettarinfo(str(source), arcname=relative)
                info.uid = 0
                info.gid = 0
                info.uname = "root"
                info.gname = "root"
                info.mtime = 0
                info.mode = 0o755 if source.is_dir() or executable_path(source) else 0o644
                if source.is_file():
                    with source.open("rb") as file:
                        archive.addfile(info, file)
                else:
                    archive.addfile(info)
    return output.getvalue()


def write_ipk_archive(path: Path, members: list[tuple[str, bytes]]) -> None:
    # iStoreOS/OpenWrt opkg uses the v0 IPK layout: a gzipped tar containing
    # debian-binary, control.tar.gz, and data.tar.gz.
    with path.open("wb") as file:
        with gzip.GzipFile(fileobj=file, mode="wb", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for name, data in members:
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.uid = 0
                    info.gid = 0
                    info.uname = "root"
                    info.gname = "root"
                    info.mtime = 0
                    info.mode = 0o644
                    archive.addfile(info, io.BytesIO(data))


def package_control(
    name: str,
    version: str,
    architecture: str,
    depends: str,
    description: str,
) -> str:
    return "\n".join(
        [
            f"Package: {name}",
            f"Version: {version}",
            f"Architecture: {architecture}",
            f"Depends: {depends}",
            "Maintainer: Ackwrap",
            "Section: net",
            "Priority: optional",
            f"Description: {description}",
            "",
        ]
    )


def build_ipk(
    output: Path,
    data_root: Path,
    control_text: str,
    scripts_dir: Path | None = None,
    conffiles: list[str] | None = None,
) -> None:
    with tempfile.TemporaryDirectory(prefix="ackwrap-control-") as temp:
        control_root = Path(temp)
        (control_root / "control").write_text(control_text, encoding="utf-8", newline="\n")
        if scripts_dir and scripts_dir.exists():
            for script in scripts_dir.iterdir():
                if script.is_file():
                    shutil.copy2(script, control_root / script.name)
        if conffiles:
            (control_root / "conffiles").write_text(
                "\n".join(conffiles) + "\n", encoding="utf-8", newline="\n"
            )
        write_ipk_archive(
            output,
            [
                ("debian-binary", b"2.0\n"),
                ("control.tar.gz", tar_gz_directory(control_root)),
                ("data.tar.gz", tar_gz_directory(data_root)),
            ],
        )


def build_openwrt_packages(
    binary: Path, arch: str, version: str, output_dir: Path
) -> list[Path]:
    package_arch = {"amd64": "x86_64", "arm64": "aarch64_generic"}[arch]
    package_version = f"{version}-1"
    for pattern in ("luci-app-ackwrap_*.ipk", "app-meta-ackwrap_*.ipk"):
        for stale_package in output_dir.glob(pattern):
            stale_package.unlink()
    with tempfile.TemporaryDirectory(prefix="ackwrap-openwrt-") as temp:
        staging = Path(temp)

        package_root = staging / "ackwrap"
        shutil.copytree(OPENWRT / "core", package_root)
        shutil.copytree(OPENWRT / "luci", package_root, dirs_exist_ok=True)
        shutil.copytree(OPENWRT / "meta", package_root, dirs_exist_ok=True)

        binary_target = package_root / "usr" / "bin" / "ackwrap"
        binary_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(binary, binary_target)

        template = package_root / "usr" / "lib" / "opkg" / "meta" / "ackwrap.json.in"
        rendered = template.with_suffix("")
        rendered.write_text(
            template.read_text(encoding="utf-8").replace("@VERSION@", version),
            encoding="utf-8",
            newline="\n",
        )
        template.unlink()
        icon_target = package_root / "www" / "luci-static" / "resources" / "app-icons" / "ackwrap.png"
        icon_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(FRONTEND / "public" / "favicon.png", icon_target)

        output = output_dir / f"ackwrap_{package_version}_{package_arch}.ipk"
        build_ipk(
            output,
            package_root,
            package_control(
                "ackwrap",
                package_version,
                package_arch,
                "libc, ca-bundle, kmod-tun, firewall4, luci-base, luci-compat, "
                "kmod-nfnetlink-queue, kmod-nft-queue",
                "Ackwrap sing-box management service with LuCI and iStoreOS integration.",
            ),
            OPENWRT / "control" / "ackwrap",
            ["/etc/config/ackwrap"],
        )
    return [output]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build Ackwrap with the embedded frontend for Windows, Linux, and OpenWrt."
    )
    parser.add_argument(
        "--target",
        choices=("all", "windows", "linux", "openwrt"),
        default="all",
    )
    parser.add_argument("--arch", choices=("amd64", "arm64"), default="amd64")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "dist")
    parser.add_argument("--version", default="0.1.0")
    parser.add_argument("--skip-checks", action="store_true")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if shutil.which("go") is None:
        raise RuntimeError("go was not found in PATH")
    build_frontend()
    if not args.skip_checks:
        run_checks()
    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    targets = ("windows", "linux", "openwrt") if args.target == "all" else (args.target,)
    built: dict[str, Path] = {}
    for target in targets:
        output = build_binary(target, args.arch, args.version, output_dir)
        built[target] = output
        print(f"Built {output}")
    if "openwrt" in built:
        for package in build_openwrt_packages(
            built["openwrt"], args.arch, args.version, output_dir
        ):
            print(f"Built {package}")


if __name__ == "__main__":
    main()
