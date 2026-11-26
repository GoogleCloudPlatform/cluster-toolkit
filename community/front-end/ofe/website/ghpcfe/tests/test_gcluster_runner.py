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

"""OFE deploys its blueprints through gcluster, not Terraform directly.

Like test_tf_state_backend.py, these tests import no Django.
"""

import importlib.util
import re
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

OFE_DIR = Path(__file__).resolve().parents[3]
CLUSTER_MANAGER = OFE_DIR / "website" / "ghpcfe" / "cluster_manager"


def _load_utils():
    spec = importlib.util.spec_from_file_location(
        "ofe_gcluster_runner_utils", CLUSTER_MANAGER / "utils.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


utils = _load_utils()


class RunGclusterTests(unittest.TestCase):
    """run_gcluster logs to <base>/gcluster.log and fails like the old helpers."""

    def _run(self, returncode, output="out\n"):
        def fake_run(cmdline, cwd, env, stdout, stderr, check):
            stdout.write(output)
            self.calls.append((cmdline, cwd, env))
            return subprocess.CompletedProcess(cmdline, returncode)

        self.calls = []
        with mock.patch.object(utils.subprocess, "run", side_effect=fake_run):
            return utils.run_gcluster(
                self.dir, "deploy", "dep", "--auto-approve",
                credentials_file=self.dir / "cloud_credentials",
            )

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self._tmp.name)
        self.addCleanup(self._tmp.cleanup)

    def test_runs_gcluster_in_base_dir_as_the_credential(self):
        log = self._run(0)
        cmdline, cwd, env = self.calls[0]
        self.assertEqual(
            cmdline,
            [utils.GCLUSTER_PATH, "--no-color", "deploy", "dep",
             "--auto-approve"],
        )
        self.assertEqual(cwd, self.dir)
        self.assertEqual(env["GOOGLE_APPLICATION_CREDENTIALS"],
                         str(self.dir / "cloud_credentials"))
        self.assertNotIn("SSH_AUTH_SOCK", env)
        self.assertTrue(env["PATH"].startswith("/usr/bin:"))
        self.assertEqual(log, self.dir / "gcluster.log")

    def test_gcluster_is_the_binary_at_the_toolkit_root(self):
        # OFE lives at <root>/community/front-end/ofe; make builds
        # <root>/gcluster (ghpc is only a symlink to it).
        self.assertEqual(Path(utils.GCLUSTER_PATH),
                         OFE_DIR.parents[2] / "gcluster")

    def test_appends_every_command_to_one_log(self):
        self._run(0, "first\n")
        self._run(0, "second\n")
        text = (self.dir / "gcluster.log").read_text()
        self.assertEqual(text.count("$ gcluster deploy dep --auto-approve"), 2)
        self.assertLess(text.index("first"), text.index("second"))

    def test_logged_command_is_shell_quoted(self):
        def fake_run(cmdline, cwd, env, stdout, stderr, check):
            return subprocess.CompletedProcess(cmdline, 0)

        with mock.patch.object(utils.subprocess, "run", side_effect=fake_run):
            utils.run_gcluster(self.dir, "create", "my blueprint.yaml", "-w")
        self.assertIn("$ gcluster create 'my blueprint.yaml' -w",
                      (self.dir / "gcluster.log").read_text())

    def test_failure_raises_called_process_error_with_log_tail(self):
        with self.assertRaises(subprocess.CalledProcessError) as ctx:
            self._run(3, "Error: quota exceeded\n")
        self.assertEqual(ctx.exception.returncode, 3)
        self.assertIn(b"quota exceeded", ctx.exception.output)


class NoDirectTerraformTests(unittest.TestCase):
    """Blueprint resources never run Terraform or Packer themselves."""

    CALL = re.compile(r"\butils\.(run_terraform|run_packer)\(")

    def test_blueprint_resources_use_gcluster(self):
        for name in ("clusterinfo.py", "filesystem.py", "image.py", "vpc.py"):
            with self.subTest(module=name):
                text = (CLUSTER_MANAGER / name).read_text()
                self.assertIsNone(self.CALL.search(text))
                self.assertIn("utils.run_gcluster(", text)

if __name__ == "__main__":
    unittest.main()
