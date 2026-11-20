# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Unit tests for tools/build_wheels.py."""

import base64
import contextlib
import csv
import email.parser
import hashlib
import io
import os
import platform
import struct
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
import build_wheels as bw

NAME = "google-cloud-cluster-toolkit"
WHEEL_NAME = "google_cloud_cluster_toolkit"
COMMIT = "810c1ba62619ac6b21f37491a7ea42793e97ee69"


def _write(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    mode = "wb" if isinstance(data, bytes) else "w"
    with open(path, mode) as f:
        f.write(data)


def _uvarint(n):
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


def _build_info(goos, goarch, cgo="0", tag="v1.107.0", commit=COMMIT, extra_ldflags=""):
    """Returns a Go 1.18+ build info block, as embedded by the Go linker."""
    lines = ["path\thpc-toolkit", "mod\thpc-toolkit\t(devel)\t"]
    if tag:
        lines.append(f"build\t-ldflags=\"-X 'main.gitTagVersion={tag}' "
                     f"-X 'main.gitCommitHash={commit}'{extra_ldflags}\"")
    lines += [f"build\tCGO_ENABLED={cgo}", f"build\tGOARCH={goarch}", f"build\tGOOS={goos}"]
    mod = b"\x30" * 16 + ("\n".join(lines) + "\n").encode() + b"\xf9" * 16
    go = b"go1.26.10"
    header = bw._BUILDINFO_MAGIC + bytes([8, 2]) + b"\0" * 16
    return header + _uvarint(len(go)) + go + _uvarint(len(mod)) + mod


def _macho(minos=(12, 0)):
    lc = struct.pack("<IIIIII", 0x32, 24, 1, (minos[0] << 16) | (minos[1] << 8), 0, 0)
    header = struct.pack("<IiiIIIII", 0xFEEDFACF, 0x0100000C, 0, 2, 1, len(lc), 0, 0)
    return header + lc


def _pe(signed):
    """Returns a minimal PE32+ file, optionally with a certificate table entry."""
    opt = bytearray(240)
    struct.pack_into("<H", opt, 0, 0x20B)
    struct.pack_into("<I", opt, 108, 16)  # NumberOfRvaAndSizes
    data = bytearray(b"MZ" + b"\0" * 58 + struct.pack("<I", 64))
    data += b"PE\0\0" + struct.pack("<HHIIIHH", 0x8664, 0, 0, 0, 0, len(opt), 0) + opt
    if signed:
        cert = struct.pack("<IHH", 16, 0x0200, 2) + b"\0" * 8
        struct.pack_into("<II", data, 64 + 24 + 112 + 32, len(data), len(cert))
        data += cert
    return bytes(data)


def fake_binary(path, platform_name, signed=True, minos=(12, 0), **build_info):
    """Writes a fake gcluster binary for a platform, with Go build info."""
    goos, goarch = platform_name.split("_", 1)
    head = {"darwin": _macho(minos), "windows": _pe(signed)}.get(goos, b"\x7fELF" + b"\0" * 12)
    _write(path, head + b"\0" * 16 + _build_info(goos, goarch, **build_info))
    return path


def _git_available():
    try:
        subprocess.run(["git", "--version"], check=True, capture_output=True)
        return True
    except (OSError, subprocess.CalledProcessError):
        return False


def git(root, *args, **env):
    return subprocess.run(
        ["git", "-C", root, "-c", "user.name=t", "-c", "user.email=t@example.com",
         "-c", "commit.gpgsign=false", *args],
        check=True, capture_output=True, text=True, env={**os.environ, **env}).stdout.strip()


class _FakeRepo(unittest.TestCase):
    """Creates a small fake cluster-toolkit checkout and fake binaries."""

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = os.path.join(self._tmp.name, "repo")
        self.out = os.path.join(self._tmp.name, "dist")
        _write(os.path.join(self.root, "go.mod"), "module hpc-toolkit\n\ngo 1.26.0\n")
        _write(os.path.join(self.root, "LICENSE"), "Apache License 2.0\n")
        _write(os.path.join(self.root, "examples", "hpc-slurm.yaml"), "blueprint_name: hpc-slurm\n")
        _write(os.path.join(self.root, "examples", "sub", "README.md"), "# sub\n")
        _write(os.path.join(self.root, "examples", ".DS_Store"), "junk")
        _write(os.path.join(self.root, "community", "examples", "c.yaml"), "blueprint_name: c\n")
        self.script = os.path.join(self.root, "examples", "sub", "build.sh")
        _write(self.script, "#!/bin/bash\n")
        os.chmod(self.script, 0o755)
        self.binary = fake_binary(os.path.join(self._tmp.name, "gcluster"), "linux_amd64")

    def binary_for(self, plat, name=None, **kw):
        return fake_binary(os.path.join(self._tmp.name, "bin", name or plat), plat, **kw)

    def build(self, plat="linux_amd64", version="1.107.0", macos_min=None):
        binary = self.binary if plat == "linux_amd64" else self.binary_for(plat)
        return bw.build_wheel(NAME, version, plat, binary, self.out, self.root, macos_min)

    def init_git(self):
        if not _git_available():
            self.skipTest("git not available")
        git(self.root, "init", "-q")
        git(self.root, "add", "-A")
        git(self.root, "commit", "-q", "-m", "x", GIT_COMMITTER_DATE="2025-03-04T05:06:08Z")
        return git(self.root, "rev-parse", "HEAD")

    def no_source_date_epoch(self):
        env = {k: v for k, v in os.environ.items() if k != "SOURCE_DATE_EPOCH"}
        return mock.patch.dict(os.environ, env, clear=True)


class TestHelpers(unittest.TestCase):

    def test_normalize_version_strips_leading_v(self):
        self.assertEqual(bw.normalize_version("v1.107.0"), "1.107.0")
        self.assertEqual(bw.normalize_version("1.107.0rc1"), "1.107.0rc1")

    def test_normalize_version_rejects_invalid(self):
        for bad in ["", "1.107.0-dirty", "latest", "1.107.0+local"]:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                bw.normalize_version(bad)

    def test_wheel_name_part_normalizes_separators(self):
        self.assertEqual(bw.wheel_name_part("google-cloud-cluster-toolkit"), WHEEL_NAME)
        self.assertEqual(bw.wheel_name_part("Cluster.Toolkit"), "cluster_toolkit")
        with self.assertRaises(ValueError):
            bw.wheel_name_part("-bad-")

    def test_platform_tags(self):
        self.assertEqual(bw.platform_tags("darwin_arm64", (12, 0)), ["macosx_12_0_arm64"])
        self.assertEqual(bw.platform_tags("darwin_amd64", (12, 0)), ["macosx_12_0_x86_64"])
        self.assertEqual(bw.platform_tags("darwin_amd64", (10, 15)), ["macosx_10_15_x86_64"])
        self.assertEqual(bw.platform_tags("darwin_arm64", (12, 3)), ["macosx_13_0_arm64"])
        self.assertEqual(bw.platform_tags("windows_amd64"), ["win_amd64"])
        self.assertEqual(bw.platform_tags("linux_arm64"),
                         ["manylinux2014_aarch64", "musllinux_1_2_aarch64"])
        with self.assertRaises(ValueError):
            bw.platform_tags("solaris_sparc")

    def test_parse_macos_min(self):
        self.assertEqual(bw.parse_macos_min("13"), (13, 0))
        self.assertEqual(bw.parse_macos_min("12.0"), (12, 0))
        self.assertEqual(bw.parse_macos_min("10.15"), (10, 15))
        for bad in ["12.3", "12.0.1", "abc", "12.", ""]:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                bw.parse_macos_min(bad)


class TestBinaryChecks(unittest.TestCase):

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)

    def fake(self, plat, **kw):
        return fake_binary(os.path.join(self._tmp.name, plat), plat, **kw)

    def test_reads_build_info(self):
        info = bw.read_build_info(self.fake("linux_arm64"))
        self.assertEqual((info["go"], info["GOOS"], info["GOARCH"], info["CGO_ENABLED"]),
                         ("go1.26.10", "linux", "arm64", "0"))
        self.assertIn("main.gitTagVersion=v1.107.0", info["-ldflags"])

    def test_skips_a_magic_header_that_does_not_parse(self):
        real = _build_info("linux", "amd64")
        bad_utf8 = real[:32] + _uvarint(3) + b"go\xff"
        too_long = real[:32] + _uvarint(2) + b"go" + _uvarint(10 ** 9)
        for decoy in [bad_utf8, too_long]:
            with self.subTest(decoy=decoy[32:40]):
                path = os.path.join(self._tmp.name, "decoy")
                _write(path, b"\x7fELF" + decoy + b"\0" * 16 + real)
                self.assertEqual(bw.read_build_info(path)["GOOS"], "linux")

    def test_truncated_build_info_is_an_error(self):
        with self.assertRaisesRegex(ValueError, "truncated"):
            bw._read_uvarint_string(_uvarint(10) + b"short", 0)
        with self.assertRaisesRegex(ValueError, "invalid length"):
            bw._read_uvarint_string(b"\xff" * 20, 0)

    def test_non_go_file_is_an_error(self):
        path = os.path.join(self._tmp.name, "script.sh")
        _write(path, "#!/bin/sh\n")
        with self.assertRaises(ValueError):
            bw.read_build_info(path)

    def test_accepts_matching_binary(self):
        bw.check_binary("linux_amd64", self.fake("linux_amd64"), "1.107.0", COMMIT)
        bw.check_binary("darwin_arm64", self.fake("darwin_arm64"), "1.107.0", COMMIT)

    def test_rejects_binary_for_another_platform(self):
        with self.assertRaisesRegex(ValueError, "built for darwin_arm64"):
            bw.check_binary("linux_amd64", self.fake("darwin_arm64"))

    def test_rejects_dynamic_linux_binary(self):
        with self.assertRaisesRegex(ValueError, "CGO_ENABLED=0"):
            bw.check_binary("linux_amd64", self.fake("linux_amd64", cgo="1"))

    def test_rejects_version_and_commit_mismatch(self):
        binary = self.fake("linux_amd64")
        with self.assertRaisesRegex(ValueError, "does not match"):
            bw.check_binary("linux_amd64", binary, "1.108.0", COMMIT)
        with self.assertRaisesRegex(ValueError, "built from commit"):
            bw.check_binary("linux_amd64", binary, "1.107.0", "0" * 40)

    def test_rejects_binary_without_release_ldflags(self):
        binary = self.fake("linux_amd64", tag=None)  # e.g. built with -trimpath
        bw.check_binary("linux_amd64", binary)  # fine when the version isn't checked
        with self.assertRaisesRegex(ValueError, "no release version"):
            bw.check_binary("linux_amd64", binary, "1.107.0", COMMIT)

    def test_rejects_binary_built_from_a_dirty_tree(self):
        binary = self.fake("linux_amd64",
                           extra_ldflags=" -X 'main.gitCommitInfo=v1.107.0-0-g810c1ba-dirty'")
        with self.assertRaisesRegex(ValueError, "uncommitted changes"):
            bw.check_binary("linux_amd64", binary, "1.107.0", COMMIT)

    def test_last_ldflag_wins_like_the_go_linker(self):
        binary = self.fake("linux_amd64", extra_ldflags=" -X 'main.gitTagVersion=v9.9.9'")
        with self.assertRaisesRegex(ValueError, r"v9\.9\.9"):
            bw.check_binary("linux_amd64", binary, "1.107.0", COMMIT)

    def test_reads_every_ldflag_spelling(self):
        # As recorded in the build info for each spelling go build accepts.
        for ldflags in ["\"-X main.gitTagVersion=v1.2.3\"",
                        "\"-X 'main.gitTagVersion=v1.2.3'\"",
                        "\"-X \\\"main.gitTagVersion=v1.2.3\\\"\"",
                        "-X=main.gitTagVersion=v1.2.3"]:
            with self.subTest(ldflags=ldflags):
                self.assertEqual(bw._ldflag_var(ldflags, "gitTagVersion"), "v1.2.3")

    def test_reads_minimum_macos(self):
        self.assertEqual(bw.macos_min_version(self.fake("darwin_amd64")), (12, 0))
        with self.assertRaises(ValueError):
            bw.macos_min_version(self.fake("linux_amd64"))

    def test_reads_legacy_version_min_command(self):
        path = os.path.join(self._tmp.name, "legacy")
        lc = struct.pack("<IIII", 0x24, 16, (10 << 16) | (13 << 8), 0)
        _write(path, struct.pack("<IiiIIIII", 0xFEEDFACF, 0x01000007, 3, 2, 1, len(lc), 0, 0) + lc)
        self.assertEqual(bw.macos_min_version(path), (10, 13))

    def test_oversized_mach_o_load_commands_are_an_error(self):
        path = os.path.join(self._tmp.name, "huge")
        _write(path, struct.pack("<IiiIIIII", 0xFEEDFACF, 0x0100000C, 0, 2, 1, 2 ** 32 - 1, 0, 0))
        with self.assertRaisesRegex(ValueError, "truncated"):
            bw.macos_min_version(path)

    def test_macos_min_override_cannot_go_below_the_binary(self):
        binary = self.fake("darwin_arm64")  # needs macOS 12.0
        self.assertEqual(bw.macos_min_for(binary), (12, 0))
        self.assertEqual(bw.macos_min_for(binary, (13, 0)), (13, 0))
        with self.assertRaisesRegex(ValueError, "lower than"):
            bw.macos_min_for(binary, (11, 0))

    def test_windows_signature(self):
        self.assertTrue(bw.has_authenticode_signature(self.fake("windows_amd64", signed=True)))
        self.assertFalse(bw.has_authenticode_signature(self.fake("windows_amd64", signed=False)))
        with self.assertRaises(ValueError):
            bw.has_authenticode_signature(self.fake("linux_amd64"))

    def test_missing_certificate_table_is_unsigned(self):
        # Offset 0 means no table; the DOS header there must not be read as a certificate.
        data = bytearray(_pe(signed=False))
        struct.pack_into("<H", data, 6, 2)  # read at offset 0: length 0x5A4D ("MZ"), type 2
        data += b"\0" * (0x5A4D - len(data))
        struct.pack_into("<II", data, 64 + 24 + 112 + 32, 0, 0x5A4D)
        path = os.path.join(self._tmp.name, "crafted.exe")
        _write(path, bytes(data))
        self.assertFalse(bw.has_authenticode_signature(path))

    def test_certificate_longer_than_its_table_is_unsigned(self):
        data = bytearray(_pe(signed=True))
        off, size = struct.unpack_from("<II", data, 64 + 24 + 112 + 32)
        struct.pack_into("<I", data, off, size + 1)  # WIN_CERTIFICATE.dwLength
        path = os.path.join(self._tmp.name, "crafted.exe")
        _write(path, bytes(data))
        self.assertFalse(bw.has_authenticode_signature(path))

    def test_truncated_windows_binary_is_a_value_error(self):
        path = os.path.join(self._tmp.name, "truncated.exe")
        _write(path, b"MZ" + b"\0" * 58 + struct.pack("<I", 64) + b"PE\0\0" + b"\0" * 10)
        with self.assertRaises(ValueError):
            bw.has_authenticode_signature(path)


class TestBuildWheel(_FakeRepo):

    def test_file_name_uses_normalized_name_and_compressed_tags(self):
        path = self.build("linux_amd64")
        self.assertEqual(
            os.path.basename(path),
            f"{WHEEL_NAME}-1.107.0-py3-none-manylinux2014_x86_64.musllinux_1_2_x86_64.whl")

    def test_contents_layout(self):
        with zipfile.ZipFile(self.build()) as z:
            names = set(z.namelist())
            exe = z.getinfo(f"{WHEEL_NAME}-1.107.0.data/scripts/gcluster")
            self.assertEqual((exe.external_attr >> 16) & 0o777, 0o755)
        share = f"{WHEEL_NAME}-1.107.0.data/data/share/cluster-toolkit"
        self.assertIn(f"{share}/examples/hpc-slurm.yaml", names)
        self.assertIn(f"{share}/examples/sub/README.md", names)
        self.assertIn(f"{share}/community/examples/c.yaml", names)
        self.assertNotIn(f"{share}/examples/.DS_Store", names)
        for f in ["LICENSE", "METADATA", "WHEEL", "RECORD"]:
            self.assertIn(f"{WHEEL_NAME}-1.107.0.dist-info/{f}", names)

    def test_entries_are_marked_unix(self):
        # zipfile marks entries as made on Windows when it runs there.
        with mock.patch.object(sys, "platform", "win32"), zipfile.ZipFile(self.build()) as z:
            self.assertEqual({i.create_system for i in z.infolist()}, {3})

    def _example_modes(self):
        share = f"{WHEEL_NAME}-1.107.0.data/data/share/cluster-toolkit/examples"
        with zipfile.ZipFile(self.build()) as z:
            mode = lambda n: (z.getinfo(f"{share}/{n}").external_attr >> 16) & 0o777
            return mode("sub/build.sh"), mode("hpc-slurm.yaml")

    def test_executable_examples_keep_exec_bit(self):
        self.assertEqual(self._example_modes(), (0o755, 0o644))

    def test_modes_come_from_git_not_the_working_tree(self):
        self.init_git()
        git(self.root, "config", "core.fileMode", "false")
        os.chmod(self.script, 0o644)  # e.g. a checkout on Windows
        self.assertEqual(self._example_modes(), (0o755, 0o644))

    def test_only_git_tracked_examples_in_a_git_checkout(self):
        self.init_git()
        _write(os.path.join(self.root, "examples", "untracked.yaml"), "x: 1\n")
        with zipfile.ZipFile(self.build()) as z:
            names = [n.split("/share/cluster-toolkit/", 1)[1] for n in z.namelist()
                     if "/share/cluster-toolkit/" in n]
        self.assertEqual(names, ["community/examples/c.yaml", "examples/hpc-slurm.yaml",
                                 "examples/sub/README.md", "examples/sub/build.sh"])

    def test_tree_inside_another_git_repo_is_not_filtered(self):
        if not _git_available():
            self.skipTest("git not available")
        outer = self._tmp.name
        _write(os.path.join(outer, "other.txt"), "x\n")
        git(outer, "init", "-q")
        git(outer, "add", "other.txt")
        git(outer, "commit", "-q", "-m", "x", GIT_COMMITTER_DATE="2024-01-01T00:00:00Z")
        with self.no_source_date_epoch(), zipfile.ZipFile(self.build()) as z:
            names = [n for n in z.namelist() if "/share/cluster-toolkit/" in n]
            times = {i.date_time for i in z.infolist()}
        self.assertEqual(len(names), 4)
        self.assertEqual(times, {bw.DEFAULT_DATE_TIME})  # not the outer repo's commit time

    def test_symlinked_example_is_an_error(self):
        link = os.path.join(self.root, "examples", "link.yaml")
        try:
            os.symlink("hpc-slurm.yaml", link)
        except (OSError, NotImplementedError):
            self.skipTest("symlinks not supported")
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.build()  # not a git checkout: os.walk
        self.init_git()
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.build()  # git checkout: git ls-files

    def test_empty_example_directory_is_an_error(self):
        os.remove(os.path.join(self.root, "community", "examples", "c.yaml"))
        with self.assertRaisesRegex(ValueError, "no example files"):
            self.build()

    def test_windows_binary_is_exe(self):
        with zipfile.ZipFile(self.build("windows_amd64")) as z:
            self.assertIn(f"{WHEEL_NAME}-1.107.0.data/scripts/gcluster.exe", z.namelist())

    def test_macos_tag_comes_from_the_binary(self):
        path = self.build("darwin_amd64")
        self.assertTrue(path.endswith("-py3-none-macosx_12_0_x86_64.whl"), path)
        path = self.build("darwin_amd64", macos_min=(14, 0))
        self.assertTrue(path.endswith("-py3-none-macosx_14_0_x86_64.whl"), path)

    def test_macos_tag_follows_a_newer_binary_minimum(self):
        binary = self.binary_for("darwin_amd64", minos=(13, 0))
        path = bw.build_wheel(NAME, "1.107.0", "darwin_amd64", binary, self.out, self.root)
        self.assertTrue(path.endswith("-py3-none-macosx_13_0_x86_64.whl"), path)

    def test_record_hashes_match_contents(self):
        _write(os.path.join(self.root, "examples", "a,b.yaml"), "x: 1\n")
        with zipfile.ZipFile(self.build()) as z:
            record = z.read(f"{WHEEL_NAME}-1.107.0.dist-info/RECORD").decode()
            rows = list(csv.reader(record.splitlines()))
            self.assertEqual(len(rows), len(z.namelist()))
            self.assertIn(f"{WHEEL_NAME}-1.107.0.data/data/share/cluster-toolkit/examples/a,b.yaml",
                          [r[0] for r in rows])
            for path, digest, size in rows:
                if path.endswith("/RECORD"):
                    self.assertEqual((digest, size), ("", ""))
                    continue
                data = z.read(path)
                want = "sha256=" + base64.urlsafe_b64encode(
                    hashlib.sha256(data).digest()).rstrip(b"=").decode()
                self.assertEqual((digest, int(size)), (want, len(data)), path)

    def test_wheel_and_metadata_files(self):
        with zipfile.ZipFile(self.build("darwin_arm64")) as z:
            wheel = z.read(f"{WHEEL_NAME}-1.107.0.dist-info/WHEEL").decode()
            meta = email.parser.Parser().parsestr(
                z.read(f"{WHEEL_NAME}-1.107.0.dist-info/METADATA").decode())
        self.assertIn("Root-Is-Purelib: false", wheel)
        self.assertIn("Tag: py3-none-macosx_12_0_arm64", wheel)
        self.assertEqual(meta["Name"], NAME)
        self.assertEqual(meta["Version"], "1.107.0")
        self.assertEqual(meta["Requires-Python"], ">=3.9")
        self.assertIn("cluster-toolkit", meta["Keywords"].split(","))
        self.assertIn("hpc-toolkit", meta["Keywords"].split(","))
        self.assertNotIn("Operating System :: Microsoft :: Windows", meta.get_all("Classifier"))
        self.assertEqual(meta["Description-Content-Type"], "text/markdown")
        self.assertIn(f"uv tool install {NAME}", meta.get_payload())

    def test_same_inputs_give_identical_wheels(self):
        with open(self.build(), "rb") as f:
            first = f.read()
        with open(self.build(), "rb") as f:
            second = f.read()
        self.assertEqual(first, second)

    def test_failed_write_leaves_no_wheel(self):
        with mock.patch.object(zipfile.ZipFile, "writestr", side_effect=OSError("disk full")):
            with self.assertRaises(OSError):
                self.build()
        self.assertEqual(os.listdir(self.out), [])

    def _entry_times(self):
        with zipfile.ZipFile(self.build()) as z:
            return {i.date_time for i in z.infolist()}

    def test_entry_times_use_source_date_epoch(self):
        # 1700000000 = 2023-11-14 22:13:20 UTC
        with mock.patch.dict(os.environ, {"SOURCE_DATE_EPOCH": "1700000000"}):
            self.assertEqual(self._entry_times(), {(2023, 11, 14, 22, 13, 20)})

    def test_entry_times_use_last_commit_time(self):
        self.init_git()
        with self.no_source_date_epoch():
            self.assertEqual(self._entry_times(), {(2025, 3, 4, 5, 6, 8)})

    def test_entry_times_fall_back_without_git_or_env(self):
        with self.no_source_date_epoch():
            self.assertEqual(self._entry_times(), {bw.DEFAULT_DATE_TIME})

    def test_invalid_source_date_epoch_is_an_error(self):
        with mock.patch.dict(os.environ, {"SOURCE_DATE_EPOCH": "yesterday"}):
            with self.assertRaises(ValueError):
                self.build()

    def test_out_of_range_source_date_epoch_is_an_error(self):
        with mock.patch.dict(os.environ, {"SOURCE_DATE_EPOCH": "9" * 30}):
            with self.assertRaisesRegex(ValueError, "out of range"):
                self.build()

    def test_missing_binary_is_an_error(self):
        os.remove(self.binary)
        with self.assertRaises(ValueError):
            self.build()


class TestMain(_FakeRepo):

    def setUp(self):
        super().setUp()
        self.head = self.init_git()

    def run_main(self, *binaries, extra=()):
        args = ["--name", NAME, "--version", "v1.107.0", "--out", self.out,
                "--repo-root", self.root, *extra]
        for b in binaries:
            args += ["--binary", b]
        return bw.main(args)

    def bin(self, plat, **kw):
        return f"{plat}={self.binary_for(plat, commit=self.head, **kw)}"

    def test_builds_one_wheel_per_platform(self):
        rc = self.run_main(self.bin("linux_amd64"), self.bin("darwin_amd64"),
                           self.bin("windows_amd64"))
        self.assertEqual(rc, 0)
        self.assertEqual(len(os.listdir(self.out)), 3)
        self.assertTrue(any("macosx_12_0_x86_64" in f for f in os.listdir(self.out)))

    def test_version_must_match_the_binary(self):
        self.assertEqual(self.run_main(self.bin("linux_amd64", tag="v1.106.0")), 1)
        self.assertEqual(self.run_main(self.bin("linux_amd64", tag="v1.106.0"),
                                       extra=["--skip-version-check"]), 0)

    def test_commit_must_match_the_checkout(self):
        binary = self.binary_for("linux_amd64", commit="0" * 40)
        self.assertEqual(self.run_main(f"linux_amd64={binary}"), 1)

    def test_uncommitted_example_changes_are_an_error(self):
        _write(os.path.join(self.root, "examples", "hpc-slurm.yaml"), "changed: true\n")
        self.assertEqual(self.run_main(self.bin("linux_amd64")), 1)

    def test_uncommitted_license_change_is_an_error(self):
        _write(os.path.join(self.root, "LICENSE"), "changed\n")
        self.assertEqual(self.run_main(self.bin("linux_amd64")), 1)

    def test_repo_root_must_be_a_git_checkout(self):
        subprocess.run(["rm", "-rf", os.path.join(self.root, ".git")], check=True)
        self.assertEqual(self.run_main(self.bin("linux_amd64")), 1)
        self.assertEqual(self.run_main(self.bin("linux_amd64"),
                                       extra=["--skip-version-check"]), 0)

    def test_binary_for_another_platform_is_an_error(self):
        darwin = self.binary_for("darwin_arm64", commit=self.head)
        self.assertEqual(self.run_main(f"linux_amd64={darwin}"), 1)

    def test_output_directory_with_wheels_is_refused(self):
        _write(os.path.join(self.out, "old-1.0.0-py3-none-any.whl"), b"x")
        self.assertEqual(self.run_main(self.bin("linux_amd64")), 1)

    def test_unsigned_windows_binary_is_refused(self):
        self.assertEqual(self.run_main(self.bin("windows_amd64", signed=False)), 1)
        self.assertEqual(self.run_main(self.bin("windows_amd64", signed=False),
                                       extra=["--allow-unsigned-windows"]), 0)

    def test_macos_min_override(self):
        self.assertEqual(self.run_main(self.bin("darwin_arm64"), extra=["--macos-min", "12.3"]), 1)
        self.assertEqual(self.run_main(self.bin("darwin_arm64"), extra=["--macos-min", "11"]), 1)
        self.assertEqual(self.run_main(self.bin("darwin_arm64"), extra=["--macos-min", "13"]), 0)
        self.assertTrue(any("macosx_13_0_arm64" in f for f in os.listdir(self.out)))

    def test_bad_macos_binary_fails_before_any_wheel_is_written(self):
        broken = os.path.join(self._tmp.name, "bin", "broken")
        _write(broken, b"\xcf\xfa\xed\xfe" + b"\0" * 28 + _build_info("darwin", "arm64",
                                                                       commit=self.head))
        self.assertEqual(self.run_main(self.bin("linux_amd64"), f"darwin_arm64={broken}"), 1)
        self.assertFalse(os.path.exists(self.out) and os.listdir(self.out))

    def test_repo_root_inside_another_git_repo_is_an_error(self):
        nested = os.path.join(self.root, "nested")
        for top in bw.EXAMPLE_DIRS:
            _write(os.path.join(nested, top, "x.yaml"), "x: 1\n")
        _write(os.path.join(nested, "LICENSE"), "Apache License 2.0\n")
        git(self.root, "add", "-A")
        git(self.root, "commit", "-q", "-m", "nested")
        args = ["--name", NAME, "--version", "1.107.0", "--out", self.out,
                "--repo-root", nested, "--binary", self.bin("linux_amd64")]
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            self.assertEqual(bw.main(args), 1)
        self.assertIn("must be the top level", err.getvalue())
        self.assertEqual(bw.main(args + ["--skip-version-check"]), 0)

    def test_rejects_bad_input(self):
        for spec in ["linux_amd64", "plan9_amd64=" + self.binary]:
            with self.subTest(spec=spec):
                self.assertEqual(self.run_main(spec), 1)
        self.assertEqual(self.run_main(self.bin("linux_amd64"), self.bin("linux_amd64")), 1)


def _host_platform():
    machine = platform.machine().lower()
    if sys.platform.startswith("linux") and machine in ("x86_64", "amd64"):
        return "linux_amd64"
    if sys.platform.startswith("linux") and machine in ("aarch64", "arm64"):
        return "linux_arm64"
    return None


def _has_ensurepip():
    try:
        import ensurepip  # noqa: F401  (python -m venv needs it to install pip)
        return True
    except ImportError:
        return False


@unittest.skipUnless(_host_platform(), "end-to-end install test runs on Linux only")
@unittest.skipUnless(_has_ensurepip(), "needs ensurepip (python3-venv on Debian/Ubuntu)")
class TestPipInstall(_FakeRepo):
    """Installs a built wheel with pip into a fresh virtual environment."""

    def test_pip_installs_binary_and_examples(self):
        wheel = self.build(_host_platform())
        venv = os.path.join(self._tmp.name, "venv")
        subprocess.run([sys.executable, "-m", "venv", venv], check=True)
        bindir = os.path.join(venv, "bin")
        subprocess.run([os.path.join(bindir, "python"), "-m", "pip", "install",
                        "--no-index", "--no-deps", "--quiet", wheel], check=True)
        exe = os.path.join(bindir, "gcluster")
        self.assertTrue(os.access(exe, os.X_OK), "gcluster not installed as executable")
        self.assertTrue(os.path.isfile(os.path.join(
            venv, "share", "cluster-toolkit", "examples", "hpc-slurm.yaml")))


if __name__ == "__main__":
    unittest.main()
