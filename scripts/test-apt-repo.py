#!/usr/bin/env python3
"""验证版本升级保留旧包、索引选取 Debian 最新版本以及无效包拒绝。"""

import gzip
import hashlib
from pathlib import Path
import runpy
import subprocess
import tempfile


build = runpy.run_path("scripts/build-apt-repo.py")["build"]
with tempfile.TemporaryDirectory(prefix="v2term-apt-test-") as temporary:
    root = Path(temporary)
    inputs = root / "input"
    for version in ["0.9.0", "0.10.0"]:
        release = inputs / version
        release.mkdir(parents=True)
        checksums = []
        for arch in ["amd64", "arm64"]:
            control = root / f"package-{version}-{arch}/DEBIAN"
            control.mkdir(parents=True)
            (control / "control").write_text(
                f"Package: v2term\nVersion: {version}\nArchitecture: {arch}\n"
                "Maintainer: test <test@example.invalid>\nDescription: APT test fixture\n"
            )
            package = release / f"v2term_{version}_{arch}.deb"
            subprocess.run(["dpkg-deb", "--build", "--root-owner-group", str(control.parent), str(package)], check=True, capture_output=True)
            checksums.append(f"{hashlib.sha256(package.read_bytes()).hexdigest()}  {package.name}\n")
        (release / "checksums.txt").write_text("".join(checksums))
    output = root / "site"
    build(inputs, output)
    for version in ["0.9.0", "0.10.0"]:
        for arch in ["amd64", "arm64"]:
            name = f"v2term_{version}_{arch}.deb"
            assert (output / "apt/pool/main/v/v2term" / name).read_bytes() == (inputs / version / name).read_bytes()
    for arch in ["amd64", "arm64"]:
        index = output / f"apt/dists/stable/main/binary-{arch}/Packages"
        text = index.read_text()
        assert "Version: 0.10.0\n" in text and "Version: 0.9.0\n" not in text
        assert f"Architecture: {arch}\n" in text
        assert gzip.decompress(index.with_suffix(".gz").read_bytes()) == index.read_bytes()
    bad = inputs / "0.9.0/v2term_0.9.0_amd64.deb"
    bad.write_bytes(b"tampered")
    try:
        build(inputs, root / "invalid")
    except ValueError as error:
        assert "SHA256" in str(error)
    else:
        raise AssertionError("篡改后的安装包未被拒绝")
print("APT 升级测试通过：保留旧包、正确选择版本、双架构索引和校验失败拒绝。")
