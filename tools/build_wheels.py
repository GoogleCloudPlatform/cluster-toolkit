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
"""Packages prebuilt gcluster binaries as platform wheels for PyPI.

Each wheel installs the gcluster binary onto PATH and the example blueprints
to <prefix>/share/cluster-toolkit/. Run it from a clean checkout of the commit
the binaries were built from; each binary's embedded Go build info (release
tag, commit, OS/architecture) must match.

Usage:
  python3 tools/build_wheels.py --name <package-name> --version 1.108.0 \\
      --binary linux_amd64=gcluster_linux_amd64 --binary darwin_arm64=gcluster_darwin_arm64
"""

import argparse
import base64
import csv
import hashlib
import io
import os
import re
import struct
import subprocess
import sys
import time
import zipfile

# The binary is static (CGO_ENABLED=0), so one Linux wheel covers glibc and musl.
LINUX_TAGS = {
    "linux_amd64": ["manylinux2014_x86_64", "musllinux_1_2_x86_64"],
    "linux_arm64": ["manylinux2014_aarch64", "musllinux_1_2_aarch64"],
}
MACOS_ARCH = {"darwin_amd64": "x86_64", "darwin_arm64": "arm64"}
WINDOWS_TAGS = {"windows_amd64": ["win_amd64"]}
PLATFORMS = sorted([*LINUX_TAGS, *MACOS_ARCH, *WINDOWS_TAGS])

EXAMPLE_DIRS = ["examples", "community/examples"]
SKIP_NAMES = {".DS_Store", "__pycache__"}
SHARE_DIR = "share/cluster-toolkit"
# Timestamp for wheel entries when neither SOURCE_DATE_EPOCH nor git is available.
DEFAULT_DATE_TIME = (2026, 1, 1, 0, 0, 0)
ZIP_MIN_EPOCH = 315532800  # 1980-01-01, the earliest time a zip entry can store

SUMMARY = "Google Cloud Cluster Toolkit command line tool (gcluster)"
HOMEPAGE = "https://cloud.google.com/cluster-toolkit"
REPO = "https://github.com/GoogleCloudPlatform/cluster-toolkit"
DESCRIPTION = f"""\
# Google Cloud Cluster Toolkit

Cluster Toolkit is open-source software offered by Google Cloud which makes it
easy for customers to deploy AI/ML and HPC environments on Google Cloud.

This package installs the `gcluster` command and the example blueprints. The
examples are installed to `<install prefix>/share/cluster-toolkit/` (with uv:
`$(uv tool dir)/{{name}}/share/cluster-toolkit/`).

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
    """Returns the name as used in wheel file names (google_cloud_cluster_toolkit)."""
    if not _NAME_RE.match(name):
        raise ValueError(f"invalid package name {name!r}")
    return re.sub(r"[-_.]+", "_", name).lower()


def _git(repo_root, *args):
    """Runs git in repo_root and returns stdout, or None if git fails."""
    try:
        return subprocess.run(["git", "-C", repo_root, *args], capture_output=True,
                              check=True).stdout
    except (OSError, subprocess.CalledProcessError):
        return None


def is_git_root(repo_root):
    """True if repo_root is the top level of a git checkout (not just inside one)."""
    top = _git(repo_root, "rev-parse", "--show-toplevel")
    return top is not None and (os.path.realpath(top.decode().strip())
                                == os.path.realpath(repo_root))


# --- Go build info (what `go version -m <binary>` prints) ---

_BUILDINFO_MAGIC = b"\xff Go buildinf:"


def _read_uvarint_string(data, pos):
    length, shift = 0, 0
    while True:
        if pos >= len(data):
            raise ValueError("truncated Go build info")
        b = data[pos]
        pos += 1
        length |= (b & 0x7F) << shift
        if not b & 0x80:
            break
        shift += 7
    return data[pos:pos + length], pos + length


def read_build_info(binary):
    """Returns the build settings Go embeds in a binary.

    For example {"go": "go1.26.10", "GOOS": "linux", "GOARCH": "amd64",
    "CGO_ENABLED": "0", "-ldflags": "..."}. Supports the Go 1.18+ format.
    """
    with open(binary, "rb") as f:
        data = f.read()
    i = data.find(_BUILDINFO_MAGIC)
    while i >= 0:
        if len(data) >= i + 32 and data[i + 15] & 0x2:  # strings stored inline
            go, pos = _read_uvarint_string(data, i + 32)
            if go.startswith(b"go"):
                mod, _ = _read_uvarint_string(data, pos)
                if len(mod) >= 33 and mod[-17:-16] == b"\n":
                    mod = mod[16:-16]  # strip the 16-byte sentinels
                info = {"go": go.decode()}
                for line in mod.decode("utf-8", "replace").splitlines():
                    if line.startswith("build\t"):
                        key, _, value = line[len("build\t"):].partition("=")
                        info[key] = value
                return info
        i = data.find(_BUILDINFO_MAGIC, i + 1)
    raise ValueError(f"{binary}: no Go build info found (not a gcluster binary?)")


def _ldflag_var(ldflags, var):
    values = re.findall(rf"-X\s+'?main\.{var}=([^'\s\"\\]+)", ldflags)
    return values[-1] if values else None  # the Go linker uses the last -X for a name


def check_binary(platform, binary, version=None, commit=None):
    """Checks that a binary was built for the platform (and release, if given)."""
    info = read_build_info(binary)
    goos, goarch = platform.split("_", 1)
    if (info.get("GOOS"), info.get("GOARCH")) != (goos, goarch):
        raise ValueError(f"{binary} is built for {info.get('GOOS')}_{info.get('GOARCH')}, "
                         f"not {platform}")
    if goos == "linux" and info.get("CGO_ENABLED") != "0":
        raise ValueError(f"{binary} must be built with CGO_ENABLED=0 (static), because "
                         "the Linux wheel is tagged for both glibc and musl")
    if version is None:
        return
    ldflags = info.get("-ldflags", "")
    tag = _ldflag_var(ldflags, "gitTagVersion")
    built_from = _ldflag_var(ldflags, "gitCommitHash")
    if not tag or not built_from:
        raise ValueError(f"{binary} has no release version in its build info (built "
                         "without -X main.gitTagVersion/main.gitCommitHash, or with "
                         "-trimpath); use --skip-version-check only for test builds")
    if tag != f"v{version}":
        raise ValueError(f"--version {version} does not match {binary} ({tag})")
    if built_from != commit:
        raise ValueError(f"{binary} was built from commit {built_from}, but --repo-root "
                         f"is at {commit}")
    if (_ldflag_var(ldflags, "gitCommitInfo") or "").endswith("-dirty"):
        raise ValueError(f"{binary} was built from a tree with uncommitted changes")


# --- Platform checks on the binary file itself ---

_LC_VERSION_MIN_MACOSX = 0x24
_LC_BUILD_VERSION = 0x32


def macos_min_version(binary):
    """Returns the minimum macOS (major, minor) recorded in a 64-bit Mach-O binary."""
    with open(binary, "rb") as f:
        header = f.read(32)
        if len(header) < 32 or struct.unpack_from("<I", header)[0] != 0xFEEDFACF:
            raise ValueError(f"{binary} is not a 64-bit Mach-O binary")
        ncmds, sizeofcmds = struct.unpack_from("<II", header, 16)
        cmds = f.read(sizeofcmds)
    off = 0
    try:
        for _ in range(ncmds):
            if off + 16 > len(cmds):
                break
            cmd, cmdsize = struct.unpack_from("<II", cmds, off)
            if cmd == _LC_BUILD_VERSION:
                ver = struct.unpack_from("<I", cmds, off + 12)[0]  # after platform
                return ver >> 16, (ver >> 8) & 0xFF
            if cmd == _LC_VERSION_MIN_MACOSX:
                ver = struct.unpack_from("<I", cmds, off + 8)[0]
                return ver >> 16, (ver >> 8) & 0xFF
            if cmdsize < 8:
                break
            off += cmdsize
    except struct.error as e:
        raise ValueError(f"{binary} is not a valid Mach-O binary: {e}") from e
    raise ValueError(f"{binary} has no minimum macOS version (LC_BUILD_VERSION)")


def macos_min_for(binary, override=None):
    """Returns the minimum macOS for a wheel: the override if given, else the binary's."""
    needed = macos_min_version(binary)
    if override is None:
        return needed
    if override < needed:
        raise ValueError(f"--macos-min {override[0]}.{override[1]} is lower than the "
                         f"{needed[0]}.{needed[1]} that {binary} needs")
    return override


def parse_macos_min(value):
    """Parses a --macos-min value ('12' or '12.0') into (major, minor)."""
    m = re.match(r"^(\d+)(?:\.(\d+))?$", value)
    if not m:
        raise ValueError(f"--macos-min must look like 12 or 12.0, got {value!r}")
    major, minor = int(m.group(1)), int(m.group(2) or 0)
    if major >= 11 and minor:
        # pip and uv only match <major>_0 tags on macOS 11 and later.
        raise ValueError(f"--macos-min {value}: use {major}.0; macOS 11+ wheels are "
                         "tagged by major version only")
    return major, minor


def _macos_tag_version(major, minor):
    if major >= 11 and minor:
        return major + 1, 0  # a <major>_0 tag would also match older minors
    return major, minor


def has_authenticode_signature(binary):
    """True if a Windows PE binary has an Authenticode certificate table entry.

    Only checks that a signature is present, not that it is valid.
    """
    with open(binary, "rb") as f:
        data = f.read()
    if data[:2] != b"MZ" or len(data) < 0x40:
        raise ValueError(f"{binary} is not a Windows executable")
    try:
        pe = struct.unpack_from("<I", data, 0x3C)[0]
        if data[pe:pe + 4] != b"PE\0\0":
            raise ValueError(f"{binary} is not a Windows executable")
        opt = pe + 24
        magic = struct.unpack_from("<H", data, opt)[0]
        if magic not in (0x10B, 0x20B):
            raise ValueError(f"{binary} has an unknown PE optional header")
        dirs = opt + (112 if magic == 0x20B else 96)
        if struct.unpack_from("<I", data, dirs - 4)[0] <= 4:
            return False
        off, size = struct.unpack_from("<II", data, dirs + 4 * 8)  # entry 4: certificates
        if size < 8 or off + size > len(data):
            return False
        length, _, cert_type = struct.unpack_from("<IHH", data, off)
    except struct.error as e:
        raise ValueError(f"{binary} is not a valid Windows executable: {e}") from e
    return length >= 8 and cert_type == 2  # WIN_CERT_TYPE_PKCS_SIGNED_DATA


def platform_tags(platform, macos_min=None):
    """Returns the wheel platform tags for a gcluster build target.

    macos_min is the minimum macOS as (major, minor); required for macOS.
    """
    if platform in LINUX_TAGS:
        return LINUX_TAGS[platform]
    if platform in WINDOWS_TAGS:
        return WINDOWS_TAGS[platform]
    if platform in MACOS_ARCH:
        major, minor = _macos_tag_version(*macos_min)
        return [f"macosx_{major}_{minor}_{MACOS_ARCH[platform]}"]
    raise ValueError(
        f"unknown platform {platform!r}; expected one of {', '.join(PLATFORMS)}")


# --- Wheel contents ---

def zip_date_time(repo_root):
    """Returns the timestamp for wheel entries, constant for a given commit.

    Uses SOURCE_DATE_EPOCH if set (https://reproducible-builds.org/specs/source-date-epoch/),
    else the last commit time of repo_root, else DEFAULT_DATE_TIME.
    """
    epoch = os.environ.get("SOURCE_DATE_EPOCH", "").strip()
    if not epoch:
        out = _git(repo_root, "log", "-1", "--format=%ct") if is_git_root(repo_root) else None
        epoch = out.decode().strip() if out else ""
        if not epoch:
            return DEFAULT_DATE_TIME
    if not epoch.isdigit():
        raise ValueError(f"SOURCE_DATE_EPOCH must be a Unix timestamp, got {epoch!r}")
    return time.gmtime(max(int(epoch), ZIP_MIN_EPOCH))[:6]


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
        ("Keywords", "gcluster,cluster-toolkit,hpc-toolkit,hpc,ai,ml,slurm,gke,"
                     "google-cloud,terraform"),
        ("Classifier", "Development Status :: 5 - Production/Stable"),
        ("Classifier", "Environment :: Console"),
        ("Classifier", "Intended Audience :: Developers"),
        ("Classifier", "Intended Audience :: System Administrators"),
        ("Classifier", "License :: OSI Approved :: Apache Software License"),
        ("Classifier", "Operating System :: MacOS"),
        # Add "Operating System :: Microsoft :: Windows" once a Windows wheel ships.
        ("Classifier", "Operating System :: POSIX :: Linux"),
        ("Classifier", "Topic :: System :: Clustering"),
        ("Classifier", "Topic :: System :: Distributed Computing"),
        # The wheel has no Python code, so this only limits which interpreters pip
        # installs it into. 3.9 keeps plain pip working with the macOS system
        # Python, RHEL/Rocky 9 and Ubuntu 22.04; uv and pipx pick their own Python.
        ("Requires-Python", ">=3.9"),
        ("Description-Content-Type", "text/markdown"),
    ]
    header = "".join(f"{k}: {v}\n" for k, v in fields)
    body = DESCRIPTION.format(name=name, version=version)
    return (header + "\n" + body).encode("utf-8")


def _skipped(rel):
    return any(part in SKIP_NAMES for part in rel.split("/"))


def _example_files(repo_root):
    """Returns sorted (absolute path, path relative to repo root, mode) tuples.

    In a git checkout, the tracked files and their git modes are used, so the
    result doesn't depend on the build OS or core.fileMode.
    """
    files = []
    if is_git_root(repo_root):
        out = _git(repo_root, "ls-files", "-s", "-z", "--", *EXAMPLE_DIRS) or b""
        for entry in out.decode().split("\0"):
            if not entry:
                continue
            meta, rel = entry.split("\t", 1)
            git_mode = meta.split(" ", 1)[0]
            if git_mode not in ("100644", "100755") or _skipped(rel):
                continue  # symlinks and submodules aren't packaged
            full = os.path.join(repo_root, *rel.split("/"))
            if not os.path.isfile(full):
                raise ValueError(f"tracked example file is missing: {rel}")
            files.append((full, rel, 0o755 if git_mode == "100755" else 0o644))
    else:
        for top in EXAMPLE_DIRS:
            for root, dirs, names in os.walk(os.path.join(repo_root, top)):
                dirs[:] = [d for d in dirs if d not in SKIP_NAMES]
                for n in names:
                    full = os.path.join(root, n)
                    rel = os.path.relpath(full, repo_root).replace(os.sep, "/")
                    if not _skipped(rel):
                        mode = 0o755 if os.stat(full).st_mode & 0o111 else 0o644
                        files.append((full, rel, mode))
    for top in EXAMPLE_DIRS:
        if not any(rel.startswith(top + "/") for _, rel, _ in files):
            raise ValueError(f"no example files found in {os.path.join(repo_root, top)}")
    return sorted(files, key=lambda f: f[1])


def _read(path):
    with open(path, "rb") as f:
        return f.read()


def build_wheel(name, version, platform, binary, out_dir, repo_root, macos_min=None):
    """Builds one platform wheel and returns its path.

    For macOS, macos_min (major, minor) defaults to the version in the binary and
    can't be lower than it.
    """
    if not os.path.isfile(binary):
        raise ValueError(f"binary for {platform} not found: {binary}")
    if platform in MACOS_ARCH:
        macos_min = macos_min_for(binary, macos_min)
    tags = platform_tags(platform, macos_min)

    name_ver = f"{wheel_name_part(name)}-{version}"
    data_dir = f"{name_ver}.data"
    dist_info = f"{name_ver}.dist-info"
    exe = "gcluster.exe" if platform.startswith("windows_") else "gcluster"
    path = os.path.join(out_dir, f"{name_ver}-py3-none-{'.'.join(tags)}.whl")

    entries = [(f"{data_dir}/scripts/{exe}", _read(binary), 0o755)]
    for full, rel, mode in _example_files(repo_root):
        entries.append((f"{data_dir}/data/{SHARE_DIR}/{rel}", _read(full), mode))
    entries.append((f"{dist_info}/LICENSE", _read(os.path.join(repo_root, "LICENSE")), 0o644))
    entries.append((f"{dist_info}/METADATA", _metadata(name, version), 0o644))
    wheel_txt = ("Wheel-Version: 1.0\n"
                 "Generator: cluster-toolkit tools/build_wheels.py\n"
                 "Root-Is-Purelib: false\n")
    wheel_txt += "".join(f"Tag: py3-none-{t}\n" for t in tags)
    entries.append((f"{dist_info}/WHEEL", wheel_txt.encode(), 0o644))

    record = io.StringIO()
    writer = csv.writer(record, lineterminator="\n")  # pip reads RECORD as CSV
    for p, d, _ in entries:
        writer.writerow([p, _record_hash(d), len(d)])
    writer.writerow([f"{dist_info}/RECORD", "", ""])
    entries.append((f"{dist_info}/RECORD", record.getvalue().encode(), 0o644))

    os.makedirs(out_dir, exist_ok=True)
    date_time = zip_date_time(repo_root)
    tmp = path + ".tmp"
    try:
        with zipfile.ZipFile(tmp, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for p, d, mode in entries:
                info = zipfile.ZipInfo(p, date_time=date_time)
                info.create_system = 3  # UNIX, so the output is the same on any build OS
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                z.writestr(info, d)
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            os.remove(tmp)
    return path


def main(argv=None):
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--name", required=True,
                    help="PyPI package name, e.g. google-cloud-cluster-toolkit")
    ap.add_argument("--version", required=True,
                    help="release version, e.g. 1.108.0 or v1.108.0")
    ap.add_argument("--binary", action="append", required=True, metavar="PLATFORM=PATH",
                    help="gcluster binary for a platform; PLATFORM is one of: "
                    + ", ".join(PLATFORMS) + ". Repeat for each platform.")
    ap.add_argument("--out", default="dist",
                    help="output directory, must not contain wheels yet (default: dist)")
    ap.add_argument("--repo-root", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
                    help="cluster-toolkit checkout the binaries were built from")
    ap.add_argument("--skip-version-check", action="store_true",
                    help="don't require the binaries' release tag and commit to match "
                    "--version and --repo-root (for test builds only)")
    ap.add_argument("--macos-min",
                    help="minimum macOS version for macOS wheels, e.g. 12.0 "
                    "(default: read from each macOS binary)")
    ap.add_argument("--allow-unsigned-windows", action="store_true",
                    help="build a Windows wheel from an unsigned gcluster.exe "
                    "(for test builds only)")
    args = ap.parse_args(argv)

    try:
        version = normalize_version(args.version)
        wheel_name_part(args.name)
        macos_min = parse_macos_min(args.macos_min) if args.macos_min else None
        if os.path.isdir(args.out) and any(f.endswith(".whl") for f in os.listdir(args.out)):
            raise ValueError(f"{args.out} already contains wheels; use an empty --out")
        commit = None
        if not args.skip_version_check:
            if not is_git_root(args.repo_root):
                raise ValueError(f"--repo-root {args.repo_root} must be the top level of a "
                                 "git checkout (or pass --skip-version-check)")
            head = _git(args.repo_root, "rev-parse", "HEAD")
            if not head:
                raise ValueError(f"cannot read the current commit of {args.repo_root}")
            commit = head.decode().strip()
            if _git(args.repo_root, "status", "--porcelain", "--", *EXAMPLE_DIRS, "LICENSE"):
                raise ValueError("examples or LICENSE have uncommitted changes in --repo-root")
        specs = []
        for spec in args.binary:
            platform, sep, binary = spec.partition("=")
            if not sep or not binary:
                raise ValueError(f"--binary must be PLATFORM=PATH, got {spec!r}")
            platform_tags(platform, (12, 0))  # validate the platform name
            if not os.path.isfile(binary):
                raise ValueError(f"binary for {platform} not found: {binary}")
            check_binary(platform, binary, None if args.skip_version_check else version,
                         commit)
            if platform in WINDOWS_TAGS and not has_authenticode_signature(binary):
                if not args.allow_unsigned_windows:
                    raise ValueError(f"{binary} is not code-signed; Windows wheels need a "
                                     "signed gcluster.exe (--allow-unsigned-windows for tests)")
                print(f"warning: {binary} is not code-signed", file=sys.stderr)
            # Read the macOS minimum before building anything, so a bad binary
            # doesn't leave a half-filled --out.
            plat_min = macos_min_for(binary, macos_min) if platform in MACOS_ARCH else None
            specs.append((platform, binary, plat_min))
        if len({p for p, _, _ in specs}) != len(specs):
            raise ValueError("each platform may be given only once")
        for platform, binary, plat_min in specs:
            path = build_wheel(args.name, version, platform, binary, args.out,
                               args.repo_root, plat_min)
            print(f"{path}  ({os.path.getsize(path) / 1e6:.1f} MB)")
    except (ValueError, OSError) as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
