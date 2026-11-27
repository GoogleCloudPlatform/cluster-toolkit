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

"""Whether a Terraform working directory may hold anything to destroy.

A cluster or image whose deployment failed before anything was applied must
stay deletable, so its teardown skips `terraform destroy` there. With the
state in the GCS bucket the directory holds no state file, so the answer
comes from the state itself; with local state (before the move to the
bucket) it comes from the files `init` and `apply` leave.

`terraform state pull` is answered by a stand-in for subprocess.run, so the
real get_terraform_state() parses what Terraform would print, and nothing is
run. Like test_tf_state_backend.py, this loads utils.py by path and imports
no Django, so it runs under `manage.py test` and under pytest.
"""

import importlib.util
import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

CLUSTER_MANAGER = Path(__file__).resolve().parents[1] / "cluster_manager"
CREDENTIALS = {"GOOGLE_APPLICATION_CREDENTIALS": "/run/creds.json"}


def _load_utils():
    spec = importlib.util.spec_from_file_location(
        "ofe_may_have_applied_utils", CLUSTER_MANAGER / "utils.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


utils = _load_utils()


def _backend_record(directory, backend_type, config=None):
    """What `terraform init` records about the directory's backend."""
    (directory / ".terraform").mkdir()
    (directory / ".terraform" / "terraform.tfstate").write_text(json.dumps(
        {"version": 3, "backend": {"type": backend_type,
                                   "config": config or {}}}))


class TerraformMayHaveAppliedTests(unittest.TestCase):

    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.dir = self.root / "primary"
        self.dir.mkdir()
        (self.dir / "main.tf").write_text("")

    def tearDown(self):
        shutil.rmtree(self.root)

    def _gcs(self):
        _backend_record(self.dir, "gcs",
                        {"bucket": "ofe-state", "prefix": "ns/clusters/c_1"})

    def _ask(self, pulled=None, error=None, directory=None):
        """Ask about `directory` (self.dir); `terraform state pull` prints
        `pulled` (bytes) or fails with `error`. Returns the answer and the
        pulls made."""
        pulls = []

        def run(cmdline, **kwargs):
            pulls.append((cmdline, kwargs))
            if error:
                raise error
            return subprocess.CompletedProcess(cmdline, 0, stdout=pulled,
                                               stderr=b"")

        with mock.patch.object(utils.subprocess, "run", side_effect=run):
            answer = utils.terraform_may_have_applied(
                str(directory or self.dir), CREDENTIALS)
        return answer, pulls

    # Nothing initialised here

    def test_no_directory(self):
        answer, pulls = self._ask(directory=self.root / "absent")
        self.assertIs(answer, False)
        self.assertEqual(pulls, [])

    def test_only_the_generated_files(self):
        answer, pulls = self._ask()
        self.assertIs(answer, False)
        self.assertEqual(pulls, [])

    # Local state: initialised before the state moved to the bucket

    def test_local_state_initialised(self):
        (self.dir / ".terraform").mkdir()
        answer, pulls = self._ask()
        self.assertIs(answer, True)
        self.assertEqual(pulls, [])

    def test_local_backend_recorded(self):
        _backend_record(self.dir, "local")
        self.assertIs(self._ask()[0], True)

    def test_local_state_files(self):
        for name in ("terraform.tfstate", "terraform.tfstate.backup"):
            with self.subTest(name=name):
                path = self.dir / name
                path.write_text("{}")
                self.assertIs(self._ask()[0], True)
                path.unlink()

    # GCS state: the state in the bucket decides

    def test_gcs_state_with_resources(self):
        self._gcs()
        state = {"version": 4, "resources": [{"type": "google_compute_disk"}]}
        answer, pulls = self._ask(json.dumps(state).encode())
        self.assertIs(answer, True)
        self.assertEqual(len(pulls), 1)
        cmdline, kwargs = pulls[0]
        self.assertEqual(cmdline, ["terraform", "state", "pull"])
        self.assertEqual(kwargs["cwd"], str(self.dir))
        # the bucket is read as the resource's own credential
        self.assertEqual(kwargs["env"]["GOOGLE_APPLICATION_CREDENTIALS"],
                         CREDENTIALS["GOOGLE_APPLICATION_CREDENTIALS"])

    def test_gcs_state_without_resources(self):
        # what `init` alone leaves, or a destroy that finished
        self._gcs()
        for state in ({"version": 4, "resources": []}, {"version": 4}):
            with self.subTest(state=state):
                self.assertIs(self._ask(json.dumps(state).encode())[0], False)

    def test_gcs_without_any_state(self):
        # `terraform state pull` prints nothing when there is no state
        self._gcs()
        for output in (b"", b"\n"):
            with self.subTest(output=output):
                self.assertIs(self._ask(output)[0], False)

    def test_gcs_state_that_cannot_be_read(self):
        # The destroy then runs and reports its own error, rather than the
        # record being dropped while resources may remain.
        self._gcs()
        for error in (subprocess.CalledProcessError(1, ["terraform"]),
                      FileNotFoundError("terraform")):
            with self.subTest(error=type(error).__name__), \
                    self.assertLogs(level="WARNING"):
                self.assertIs(self._ask(error=error)[0], True)

    def test_gcs_state_that_cannot_be_parsed(self):
        self._gcs()
        with self.assertLogs(level="WARNING"):
            self.assertIs(self._ask(b"not a state document")[0], True)

    def test_an_unreadable_backend_record(self):
        (self.dir / ".terraform").mkdir()
        (self.dir / ".terraform" / "terraform.tfstate").write_text("{")
        with self.assertLogs(level="WARNING"):
            answer, pulls = self._ask()
        self.assertIs(answer, True)
        self.assertEqual(pulls, [])


class RunTerraformStdinTests(unittest.TestCase):
    """Terraform never reads the server's standard input.

    It runs unattended in a background task; with -input=false it fails on
    a missing value instead of prompting, and with its standard input closed
    nothing it starts can wait for input either.
    """

    def test_every_command_gets_a_closed_stdin(self):
        for command in ("init", "validate", "plan", "apply", "output",
                        "destroy"):
            with self.subTest(command=command), \
                    tempfile.TemporaryDirectory() as tmp, \
                    mock.patch.object(utils.subprocess, "run") as run:
                utils.run_terraform(tmp, command)
                terraform = [c for c in run.call_args_list
                             if c.args[0][0] == "terraform"]
                self.assertEqual(len(terraform), 1)
                self.assertIs(terraform[0].kwargs["stdin"],
                              subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
