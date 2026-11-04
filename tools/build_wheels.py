#!/usr/bin/env python3
# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Package prebuilt gcluster binaries as platform wheels for PyPI.

Users install the result with `uv tool install <name>`, `pipx install <name>`
or `pip install <name>`. Each wheel contains:

  - the gcluster binary (gcluster.exe on Windows), installed to the
    environment's bin/ (Scripts\\ on Windows) so it is on PATH
  - examples/ and community/examples/, installed to
    <prefix>/share/cluster-toolkit/
  - the LICENSE file

No Python code runs at install time or at run time: pip only copies files.
The script uses the Python standard library only.

Usage:
  tools/build_wheels.py --name google-cloud-cluster-toolkit \\
      --version 1.107.0 --out dist \\
      --binary linux_amd64=build/gcluster_linux_amd64 \\
      --binary darwin_arm64=build/gcluster_darwin_arm64 \\
      --binary windows_amd64=build/gcluster_windows_amd64.exe
"""

import argparse
import base64
import hashlib
import os
import re
import sys
import zipfile

# Wheel platform tags per gcluster build target (GOOS_GOARCH). The binary is a
# static Go build (CGO_ENABLED=0), so one Linux wheel covers glibc and musl
# distributions. macOS tags are filled in from the minimum macOS version.
LINUX_TAGS = {
    "linux_amd64": ["manylinux2014_x86_64", "musllinux_1_2_x86_64"],
    "linux_arm64": ["manylinux2014_aarch64", "musllinux_1_2_aarch64"],
}
MACOS_ARCH = {"darwin_amd64": "x86_64", "darwin_arm64": "arm64"}
WINDOWS_TAGS = {"windows_amd64": ["win_amd64"]}
PLATFORMS = sorted([*LINUX_TAGS, *MACOS_ARCH, *WINDOWS_TAGS])

# Minimum macOS version required by each Go release, from
# https://go.dev/wiki/MinimumRequirements. The macOS wheel tag must not be
# lower than this, or pip would install a binary that cannot run.
GO_MIN_MACOS = {
    "1.23": "11.0",
    "1.24": "11.0",
    "1.25": "12.0",
    "1.26": "12.0",
    "1.27": "13.0",
}

EXAMPLE_DIRS = ["examples", "community/examples"]
SKIP_NAMES = {".DS_Store", "__pycache__"}
SHARE_DIR = "share/cluster-toolkit"
# Fixed timestamp so the same inputs always produce the same wheel bytes.
ZIP_DATE_TIME = (2026, 1, 1, 0, 0, 0)

SUMMARY = "Google Cloud Cluster Toolkit command line tool (gcluster)"
HOMEPAGE = "https://cloud.google.com/cluster-toolkit"
REPO = "https://github.com/GoogleCloudPlatform/cluster-toolkit"
DESCRIPTION = f"""\
# Google Cloud Cluster Toolkit

Cluster Toolkit is open-source software offered by Google Cloud which makes it
easy for customers to deploy AI/ML and HPC environments on Google Cloud.

This package installs the `gcluster` command and the example blueprints.

```shell
uv tool install {{name}}           # or: pipx install {{name}}
uv tool install {{name}}=={{version}}  # install a specific version
uv tool upgrade {{name}}           # upgrade to the latest version
gcluster --version
```

- Documentation: {HOMEPAGE}
- Source code and issues: {REPO}
"""

# PEP 440 public version identifiers (simplified; no local versions on PyPI).
_VERSION_RE = re.compile(
    r"^\d+(\.\d+)*((a|b|rc)\d+)?(\.post\d+)?(\.dev\d+)?$")
# PEP 508 project names.
_NAME_RE = re.compile(r"^([A-Z0-9]|[A-Z0-9][A-Z0-9._-]*[A-Z0-9])$", re.I)


def normalize_version(version):
    """Strips a leading 'v' (git tags) and checks the PEP 440 format."""
    v = version[1:] if version.startswith("v") else version
    if not _VERSION_RE.match(v):
        raise ValueError(f"invalid version {version!r}; expected e.g. 1.107.0")
    return v


def wheel_name_part(name):
    """Returns the distribution name as written in wheel file names.

    The wheel spec requires runs of '-', '_' and '.' to become a single '_'
    (google-cloud-cluster-toolkit -> google_cloud_cluster_toolkit).
    """
    if not _NAME_RE.match(name):
        raise ValueError(f"invalid package name {name!r}")
    return re.sub(r"[-_.]+", "_", name).lower()


def go_version(repo_root):
    """Reads the Go language version ('1.26') from go.mod."""
    with open(os.path.join(repo_root, "go.mod"), encoding="utf-8") as f:
        for line in f:
            m = re.match(r"^go\s+(\d+)\.(\d+)", line)
            if m:
                return f"{m.group(1)}.{m.group(2)}"
    raise ValueError("no 'go' directive found in go.mod")


def toolkit_version(repo_root):
    """Returns the version gcluster reports (latestToolkitVersion), or None.

    `gcluster --version` prints this constant from pkg/config/config.go, so the
    wheel version must match it.
    """
    path = os.path.join(repo_root, "pkg", "config", "config.go")
    if not os.path.isfile(path):
        return None
    with open(path, encoding="utf-8") as f:
        m = re.search(r'latestToolkitVersion\s*=\s*"v?([^"]+)"', f.read())
    return m.group(1) if m else None


def min_macos_for_go(go_ver):
    """Returns the minimum macOS version for a Go release."""
    if go_ver not in GO_MIN_MACOS:
        raise ValueError(
            f"unknown minimum macOS version for Go {go_ver}; add it to "
            "GO_MIN_MACOS (see https://go.dev/wiki/MinimumRequirements) or "
            "pass --macos-min")
    return GO_MIN_MACOS[go_ver]


def platform_tags(platform, macos_min):
    """Returns the wheel platform tags for a gcluster build target."""
    if platform in LINUX_TAGS:
        return LINUX_TAGS[platform]
    if platform in WINDOWS_TAGS:
        return WINDOWS_TAGS[platform]
    if platform in MACOS_ARCH:
        major, _, minor = macos_min.partition(".")
        return [f"macosx_{major}_{minor or 0}_{MACOS_ARCH[platform]}"]
    raise ValueError(
        f"unknown platform {platform!r}; expected one of {', '.join(PLATFORMS)}")


def _record_hash(data):
    digest = hashlib.sha256(data).digest()
    return "sha256=" + base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def _metadata(name, version):
    fields = [
        ("Metadata-Version", "2.1"),
        ("Name", name),
        ("Version", version),
        ("Summary", SUMMARY),
        ("Home-page", HOMEPAGE),
        ("Author", "Google LLC"),
        ("License", "Apache-2.0"),
        ("Project-URL", f"Documentation, {HOMEPAGE}"),
        ("Project-URL", f"Source, {REPO}"),
        ("Project-URL", f"Issues, {REPO}/issues"),
        ("Project-URL", f"Releases, {REPO}/releases"),
        ("Keywords", "gcluster,hpc,ai,ml,slurm,gke,google-cloud,terraform"),
        ("Classifier", "Development Status :: 5 - Production/Stable"),
        ("Classifier", "Environment :: Console"),
        ("Classifier", "Intended Audience :: Developers"),
        ("Classifier", "Intended Audience :: System Administrators"),
        ("Classifier", "License :: OSI Approved :: Apache Software License"),
        ("Classifier", "Operating System :: MacOS"),
        ("Classifier", "Operating System :: Microsoft :: Windows"),
        ("Classifier", "Operating System :: POSIX :: Linux"),
        ("Classifier", "Topic :: System :: Clustering"),
        ("Classifier", "Topic :: System :: Distributed Computing"),
        ("Requires-Python", ">=3.8"),
        ("Description-Content-Type", "text/markdown"),
    ]
    header = "".join(f"{k}: {v}\n" for k, v in fields)
    body = DESCRIPTION.format(name=name, version=version)
    return (header + "\n" + body).encode("utf-8")


def _example_files(repo_root):
    """Yields (absolute path, path relative to repo root) in sorted order."""
    for top in EXAMPLE_DIRS:
        base = os.path.join(repo_root, top)
        if not os.path.isdir(base):
            raise ValueError(f"missing example directory {base}")
        for root, dirs, files in os.walk(base):
            dirs[:] = sorted(d for d in dirs if d not in SKIP_NAMES)
            for f in sorted(files):
                if f in SKIP_NAMES:
                    continue
                full = os.path.join(root, f)
                rel = os.path.relpath(full, repo_root).replace(os.sep, "/")
                yield full, rel


def _read(path):
    with open(path, "rb") as f:
        return f.read()


def build_wheel(name, version, platform, binary, out_dir, repo_root, macos_min):
    """Builds one platform wheel and returns its path."""
    tags = platform_tags(platform, macos_min)
    if not os.path.isfile(binary):
        raise ValueError(f"binary for {platform} not found: {binary}")

    name_ver = f"{wheel_name_part(name)}-{version}"
    data_dir = f"{name_ver}.data"
    dist_info = f"{name_ver}.dist-info"
    exe = "gcluster.exe" if platform.startswith("windows_") else "gcluster"
    path = os.path.join(out_dir, f"{name_ver}-py3-none-{'.'.join(tags)}.whl")

    entries = [(f"{data_dir}/scripts/{exe}", _read(binary), 0o755)]
    for full, rel in _example_files(repo_root):
        entries.append((f"{data_dir}/data/{SHARE_DIR}/{rel}", _read(full), 0o644))
    entries.append((f"{dist_info}/LICENSE", _read(os.path.join(repo_root, "LICENSE")), 0o644))
    entries.append((f"{dist_info}/METADATA", _metadata(name, version), 0o644))
    wheel_txt = ("Wheel-Version: 1.0\n"
                 "Generator: cluster-toolkit tools/build_wheels.py\n"
                 "Root-Is-Purelib: false\n")
    wheel_txt += "".join(f"Tag: py3-none-{t}\n" for t in tags)
    entries.append((f"{dist_info}/WHEEL", wheel_txt.encode(), 0o644))

    record = [f"{p},{_record_hash(d)},{len(d)}" for p, d, _ in entries]
    record.append(f"{dist_info}/RECORD,,")
    entries.append((f"{dist_info}/RECORD", ("\n".join(record) + "\n").encode(), 0o644))

    os.makedirs(out_dir, exist_ok=True)
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for p, d, mode in entries:
            info = zipfile.ZipInfo(p, date_time=ZIP_DATE_TIME)
            info.external_attr = (0o100000 | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, d)
    return path


def main(argv=None):
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--name", required=True,
                    help="PyPI package name, e.g. google-cloud-cluster-toolkit")
    ap.add_argument("--version", required=True,
                    help="release version, e.g. 1.107.0 or v1.107.0")
    ap.add_argument("--binary", action="append", required=True, metavar="PLATFORM=PATH",
                    help="gcluster binary for a platform; PLATFORM is one of: "
                    + ", ".join(PLATFORMS) + ". Repeat for each platform.")
    ap.add_argument("--out", default="dist", help="output directory (default: dist)")
    ap.add_argument("--repo-root", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
                    help="cluster-toolkit checkout with examples/ and go.mod")
    ap.add_argument("--skip-version-check", action="store_true",
                    help="allow --version to differ from latestToolkitVersion in "
                    "pkg/config/config.go (for test builds only)")
    ap.add_argument("--macos-min",
                    help="minimum macOS version for macOS wheels, e.g. 12.0 "
                    "(default: derived from the Go version in go.mod)")
    args = ap.parse_args(argv)

    try:
        version = normalize_version(args.version)
        wheel_name_part(args.name)
        reported = toolkit_version(args.repo_root)
        if reported and reported != version and not args.skip_version_check:
            raise ValueError(
                f"--version {version} does not match the version gcluster reports "
                f"({reported}, latestToolkitVersion in pkg/config/config.go)")
        macos_min = args.macos_min or min_macos_for_go(go_version(args.repo_root))
        specs = []
        for spec in args.binary:
            platform, sep, binary = spec.partition("=")
            if not sep or not binary:
                raise ValueError(f"--binary must be PLATFORM=PATH, got {spec!r}")
            platform_tags(platform, macos_min)  # validate before building
            specs.append((platform, binary))
        if len({p for p, _ in specs}) != len(specs):
            raise ValueError("each platform may be given only once")
        for platform, binary in specs:
            path = build_wheel(args.name, version, platform, binary, args.out,
                               args.repo_root, macos_min)
            print(f"{path}  ({os.path.getsize(path) / 1e6:.1f} MB)")
    except (ValueError, OSError) as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
