#!/usr/bin/env python3
"""用经过校验的 Debian 包生成标准 APT 索引；签名单独由发布任务完成。"""

from datetime import datetime, timezone
from email.utils import format_datetime
import gzip
import hashlib
from pathlib import Path
import shutil
import subprocess
import sys


def build(packages, output):
    if output.exists() and any(output.iterdir()):
        raise ValueError("输出目录必须为空，避免遗留旧索引或旧安装包")
    repo = output / "apt"
    pool = repo / "pool/main/v/v2term"
    pool.mkdir(parents=True)
    checksums = dict((line.split()[1], line.split()[0]) for line in (packages / "checksums.txt").read_text().splitlines())
    versions = set()
    architectures = set()
    for package in sorted(packages.glob("*.deb")):
        if hashlib.sha256(package.read_bytes()).hexdigest() != checksums.get(package.name):
            raise ValueError(f"安装包缺少有效的 SHA256 校验：{package.name}")
        fields = subprocess.check_output(
            ["dpkg-deb", "--field", str(package), "Package", "Version", "Architecture"],
            text=True,
        )
        fields = dict(line.split(": ", 1) for line in fields.splitlines())
        if fields["Package"] != "v2term" or fields["Architecture"] not in {"amd64", "arm64"}:
            raise ValueError(f"不支持的安装包：{package.name}")
        versions.add(fields["Version"])
        if fields["Architecture"] in architectures:
            raise ValueError("同一架构只能包含一个发行版本")
        architectures.add(fields["Architecture"])
        shutil.copyfile(package, pool / package.name)
    if architectures != {"amd64", "arm64"} or len(versions) != 1:
        raise ValueError("必须提供同一版本的 amd64 和 arm64 安装包")

    release_dir = repo / "dists/stable"
    for arch in sorted(architectures):
        index = subprocess.check_output(["dpkg-scanpackages", "--arch", arch, "pool"], cwd=repo)
        target = release_dir / f"main/binary-{arch}"
        target.mkdir(parents=True)
        (target / "Packages").write_bytes(index)
        (target / "Packages.gz").write_bytes(gzip.compress(index, mtime=0))

    release = [
        "Origin: v2term",
        "Label: v2term",
        "Suite: stable",
        "Codename: stable",
        "Architectures: amd64 arm64",
        "Components: main",
        "Description: v2term APT repository",
        "Date: " + format_datetime(datetime.now(timezone.utc), usegmt=True),
    ]
    indexes = sorted(path for path in release_dir.rglob("*") if path.is_file())
    for label, algorithm in [("MD5Sum", "md5"), ("SHA256", "sha256"), ("SHA512", "sha512")]:
        release.append(label + ":")
        for path in indexes:
            data = path.read_bytes()
            digest = hashlib.new(algorithm, data).hexdigest()
            release.append(f" {digest} {len(data)} {path.relative_to(release_dir).as_posix()}")
    (release_dir / "Release").write_text("\n".join(release) + "\n")
    shutil.copyfile("packaging/apt/v2term.asc", repo / "v2term.asc")
    shutil.copyfile("packaging/apt/index.html", repo / "index.html")
    shutil.copyfile("packaging/apt/index.html", output / "index.html")
    (output / ".nojekyll").touch()


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("用法：python3 scripts/build-apt-repo.py 安装包目录 输出目录")
    build(Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve())
