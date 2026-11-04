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
import email.parser
import hashlib
import os
import platform
import subprocess
import sys
import tempfile
import unittest
import zipfile

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
import build_wheels as bw

NAME = "google-cloud-cluster-toolkit"
WHEEL_NAME = "google_cloud_cluster_toolkit"


def _write(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    mode = "wb" if isinstance(data, bytes) else "w"
    with open(path, mode) as f:
        f.write(data)


class _FakeRepo(unittest.TestCase):
    """Creates a small fake cluster-toolkit checkout and a fake binary."""

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
        self.binary = os.path.join(self._tmp.name, "gcluster")
        _write(self.binary, b"\x7fELF fake gcluster binary")

    def build(self, plat="linux_amd64", version="1.107.0", macos_min="12.0"):
        return bw.build_wheel(NAME, version, plat, self.binary, self.out, self.root, macos_min)


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

    def test_macos_tag_follows_go_version(self):
        self.assertEqual(bw.min_macos_for_go("1.26"), "12.0")
        self.assertEqual(bw.min_macos_for_go("1.27"), "13.0")
        with self.assertRaises(ValueError):
            bw.min_macos_for_go("9.99")

    def test_platform_tags(self):
        self.assertEqual(bw.platform_tags("darwin_arm64", "12.0"), ["macosx_12_0_arm64"])
        self.assertEqual(bw.platform_tags("darwin_amd64", "12.0"), ["macosx_12_0_x86_64"])
        self.assertEqual(bw.platform_tags("windows_amd64", "12.0"), ["win_amd64"])
        self.assertEqual(bw.platform_tags("linux_arm64", "12.0"),
                         ["manylinux2014_aarch64", "musllinux_1_2_aarch64"])
        with self.assertRaises(ValueError):
            bw.platform_tags("solaris_sparc", "12.0")


class TestGoVersion(_FakeRepo):

    def test_reads_go_mod(self):
        self.assertEqual(bw.go_version(self.root), "1.26")

    def test_reads_toolkit_version(self):
        self.assertIsNone(bw.toolkit_version(self.root))
        _write(os.path.join(self.root, "pkg", "config", "config.go"),
               'const (\n\tlatestToolkitVersion     = "v1.107.0"\n)\n')
        self.assertEqual(bw.toolkit_version(self.root), "1.107.0")


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

    def test_windows_binary_is_exe(self):
        with zipfile.ZipFile(self.build("windows_amd64")) as z:
            self.assertIn(f"{WHEEL_NAME}-1.107.0.data/scripts/gcluster.exe", z.namelist())

    def test_record_hashes_match_contents(self):
        with zipfile.ZipFile(self.build()) as z:
            record = z.read(f"{WHEEL_NAME}-1.107.0.dist-info/RECORD").decode()
            rows = [r.split(",") for r in record.splitlines()]
            self.assertEqual(len(rows), len(z.namelist()))
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
        self.assertEqual(meta["Requires-Python"], ">=3.8")
        self.assertEqual(meta["Description-Content-Type"], "text/markdown")
        self.assertIn(f"uv tool install {NAME}", meta.get_payload())

    def test_same_inputs_give_identical_wheels(self):
        with open(self.build(), "rb") as f:
            first = f.read()
        with open(self.build(), "rb") as f:
            second = f.read()
        self.assertEqual(first, second)

    def test_missing_binary_is_an_error(self):
        os.remove(self.binary)
        with self.assertRaises(ValueError):
            self.build()


class TestMain(_FakeRepo):

    def run_main(self, *binaries):
        args = ["--name", NAME, "--version", "v1.107.0", "--out", self.out,
                "--repo-root", self.root]
        for b in binaries:
            args += ["--binary", b]
        return bw.main(args)

    def test_builds_one_wheel_per_platform(self):
        rc = self.run_main(f"linux_amd64={self.binary}", f"darwin_amd64={self.binary}",
                           f"windows_amd64={self.binary}")
        self.assertEqual(rc, 0)
        self.assertEqual(len(os.listdir(self.out)), 3)
        self.assertTrue(any("macosx_12_0_x86_64" in f for f in os.listdir(self.out)))

    def test_version_must_match_gcluster_version(self):
        _write(os.path.join(self.root, "pkg", "config", "config.go"),
               'latestToolkitVersion = "v1.108.0"\n')
        self.assertEqual(self.run_main(f"linux_amd64={self.binary}"), 1)
        args = ["--name", NAME, "--version", "1.107.0", "--out", self.out,
                "--repo-root", self.root, "--binary", f"linux_amd64={self.binary}",
                "--skip-version-check"]
        self.assertEqual(bw.main(args), 0)

    def test_rejects_bad_input(self):
        for spec in ["linux_amd64", "plan9_amd64=" + self.binary]:
            with self.subTest(spec=spec):
                self.assertEqual(self.run_main(spec), 1)
        self.assertEqual(self.run_main(f"linux_amd64={self.binary}",
                                       f"linux_amd64={self.binary}"), 1)


def _host_platform():
    machine = platform.machine().lower()
    if sys.platform.startswith("linux") and machine in ("x86_64", "amd64"):
        return "linux_amd64"
    if sys.platform.startswith("linux") and machine in ("aarch64", "arm64"):
        return "linux_arm64"
    return None


@unittest.skipUnless(_host_platform(), "end-to-end install test runs on Linux only")
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
