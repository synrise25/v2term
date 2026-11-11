#!/usr/bin/env python3
"""根据发行归档及校验和生成 Homebrew 和 WinGet 清单。"""

import hashlib
import json
from pathlib import Path
import re
import sys
import zipfile


def generate(tag, directory):
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", tag):
        raise ValueError("版本必须是 v 开头的语义化版本号")
    version = tag[1:]
    repository = "https://github.com/synrise25/v2term"
    base = f"{repository}/releases/download/{tag}"
    checksums = {}
    for line in (directory / "checksums.txt").read_text().splitlines():
        digest, name = line.split()
        checksums[name.lstrip("*")] = digest

    def archive(os_name, arch):
        suffix = "zip" if os_name == "windows" else "tar.gz"
        name = f"v2term_{os_name}_{arch}.{suffix}"
        digest = hashlib.sha256((directory / name).read_bytes()).hexdigest()
        if digest != checksums[name]:
            raise ValueError(f"归档校验失败：{name}")
        return f"{base}/{name}", digest

    formula = [
        "class V2term < Formula",
        '  desc "V2EX 终端客户端"',
        f'  homepage "{repository}"',
        f'  version "{version}"',
        '  license "MIT"',
        "",
    ]
    for os_name, block in [("darwin", "macos"), ("linux", "linux")]:
        formula.append(f"  on_{block} do")
        for arch, cpu in [("arm64", "arm"), ("amd64", "intel")]:
            url, digest = archive(os_name, arch)
            formula.extend([
                f"    on_{cpu} do",
                f'      url "{url}"',
                f'      sha256 "{digest}"',
                "    end",
            ])
        formula.extend(["  end", ""])
    formula.extend([
        "  def install",
        '    bin.install "v2term"',
        "  end",
        "",
        "  test do",
        '    assert_match version.to_s, shell_output("#{bin}/v2term --version")',
        "  end",
        "end",
        "",
    ])
    (directory / "v2term.rb").write_text("\n".join(formula))

    # 使用 JSON 引号转义字符串；该子集也是合法的 YAML。
    quote = lambda value: json.dumps(value, ensure_ascii=False)
    common = [
        "PackageIdentifier: synrise25.v2term",
        f"PackageVersion: {quote(version)}",
    ]
    locale = common + [
        "PackageLocale: zh-CN",
        "Publisher: synrise25",
        "PackageName: v2term",
        "License: MIT",
        f"LicenseUrl: {repository}/blob/{tag}/LICENSE",
        "ShortDescription: V2EX 终端客户端",
        f"PackageUrl: {repository}",
        "ManifestType: defaultLocale",
        "ManifestVersion: 1.28.0",
        "",
    ]
    manifest = common + [
        "InstallerType: zip",
        "NestedInstallerType: portable",
        "NestedInstallerFiles:",
        "  - RelativeFilePath: v2term.exe",
        "    PortableCommandAlias: v2term",
        "Installers:",
    ]
    for arch, platform in [("amd64", "x64"), ("arm64", "arm64")]:
        url, digest = archive("windows", arch)
        manifest.extend([
            f"  - Architecture: {platform}",
            f"    InstallerUrl: {quote(url)}",
            f"    InstallerSha256: {quote(digest.upper())}",
        ])
    manifest.extend(["ManifestType: installer", "ManifestVersion: 1.28.0", ""])
    root = common + ["DefaultLocale: zh-CN", "ManifestType: version", "ManifestVersion: 1.28.0", ""]
    manifests = {
        "synrise25.v2term.yaml": ("version", root),
        "synrise25.v2term.installer.yaml": ("installer", manifest),
        "synrise25.v2term.locale.zh-CN.yaml": ("defaultLocale", locale),
    }
    with zipfile.ZipFile(directory / "v2term-winget-manifests.zip", "w", zipfile.ZIP_DEFLATED) as archive_file:
        for name, (kind, lines) in manifests.items():
            header = f"# yaml-language-server: $schema=https://aka.ms/winget-manifest.{kind}.1.28.0.schema.json\n\n"
            archive_file.writestr(f"manifests/s/synrise25/v2term/{version}/{name}", header + "\n".join(lines))
    for name in ["v2term.rb", "v2term-winget-manifests.zip"]:
        checksums[name] = hashlib.sha256((directory / name).read_bytes()).hexdigest()
    (directory / "checksums.txt").write_text(
        "".join(f"{digest}  {name}\n" for name, digest in sorted(checksums.items()))
    )


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("用法：python3 scripts/package-metadata.py v版本号")
    generate(sys.argv[1], Path("dist"))
