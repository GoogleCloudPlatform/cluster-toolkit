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

"""utils.run_packer runs Packer as the credential, never as the server.

The real run_packer runs a stand-in Packer that records its environment.
run_packer names Packer by its full path (/usr/bin/packer, since another
binary called packer can come first on PATH), so the tests send that one
path to the stand-in at the subprocess.run boundary; every other argument
and the environment reach the stand-in unchanged. The server's environment
holds a value for every ambient credential variable, none of which may
reach Packer.
"""

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
from pathlib import Path
from unittest import mock

from django.test import TestCase

from ghpcfe.cluster_manager import utils
from ghpcfe.tests.test_credentials import (
    ACCOUNT,
    KEY_JSON,
    PROJECT,
    impersonate_credential,
    key_credential,
)
from ghpcfe.tests.test_preflight import (
    AMBIENT_CREDENTIAL_VARIABLES,
    RECORD_ENVIRONMENT,
    ambient_credentials,
)

PACKER = "/usr/bin/packer"


class RunPackerCredentialTests(TestCase):
    """utils.run_packer with a stand-in Packer"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-fake-packer-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        self.fake = self.tmp / "packer"
        self.fake.write_text(RECORD_ENVIRONMENT.format(python=sys.executable))
        self.fake.chmod(stat.S_IRWXU)
        self.workdir = self.tmp / "work"
        self.workdir.mkdir()
        self.record = self.tmp / "record.json"
        self.ambient = ambient_credentials()
        patcher = mock.patch.dict(os.environ, self.ambient)
        patcher.start()
        self.addCleanup(patcher.stop)

        self.commands = []
        real_run = subprocess.run

        def run(cmdline, *args, **kwargs):
            self.commands.append(list(cmdline))
            if cmdline[0] == PACKER:
                cmdline = [str(self.fake), *cmdline[1:]]
            return real_run(cmdline, *args, **kwargs)

        patcher = mock.patch("subprocess.run", side_effect=run)
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_packer(self, credential, command="build", rc=0):
        before = dict(os.environ)
        utils.run_packer(
            self.workdir,
            command,
            arguments=["-var", "startup_script_file=startup_script.sh", "."],
            extra_env={"FAKE_RECORD": str(self.record), "FAKE_RC": str(rc)},
            credential=credential,
            operation="build image img-a",
        )
        # The server's own environment is unchanged
        self.assertEqual(dict(os.environ), before)

    def recorded(self):
        """What the stand-in Packer saw"""
        self.assertEqual(self.commands[0][:2], [PACKER, "build"])
        record = json.loads(self.record.read_text(encoding="utf-8"))
        self.assertEqual(
            record["argv"],
            ["build", "-var", "startup_script_file=startup_script.sh", "."],
        )
        return record

    def assert_no_ambient_credential(self, env):
        for name, value in self.ambient.items():
            self.assertNotEqual(env.get(name), value, name)

    def test_key_build_runs_as_the_key(self):
        self.run_packer(key_credential())
        record = self.recorded()
        env = record["env"]
        self.assert_no_ambient_credential(env)
        # Packer reads only a key file: a private copy of the key, removed
        # after the run
        self.assertEqual(env["GOOGLE_CREDENTIALS"], KEY_JSON)
        self.assertEqual(record["key_file_text"], KEY_JSON)
        self.assertEqual(record["key_file_mode"], 0o600)
        self.assertEqual(
            env["CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"],
            env["GOOGLE_APPLICATION_CREDENTIALS"],
        )
        self.assertFalse(Path(env["GOOGLE_APPLICATION_CREDENTIALS"]).exists())
        # gcloud gets a configuration directory of its own and the
        # credential's project
        self.assertFalse(Path(env["CLOUDSDK_CONFIG"]).exists())
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], PROJECT)
        for name in (
            "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
            "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT",
            "GOOGLE_OAUTH_ACCESS_TOKEN",
            "CLOUDSDK_CORE_ACCOUNT",
        ):
            self.assertNotIn(name, env)

    def test_impersonate_build_runs_as_the_account(self):
        self.run_packer(impersonate_credential())
        env = self.recorded()["env"]
        self.assert_no_ambient_credential(env)
        self.assertEqual(env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertEqual(
            env["CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT
        )
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], PROJECT)
        for name in (
            "GOOGLE_CREDENTIALS",
            "GOOGLE_APPLICATION_CREDENTIALS",
            "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
        ):
            self.assertNotIn(name, env)

    def test_failed_key_build_still_removes_the_key_file(self):
        with self.assertRaises(RuntimeError):
            self.run_packer(key_credential(), rc=1)
        env = self.recorded()["env"]
        self.assert_no_ambient_credential(env)
        path = Path(env["GOOGLE_APPLICATION_CREDENTIALS"])
        self.assertFalse(path.exists())
        self.assertFalse(path.parent.exists())

    def test_run_without_a_credential_gets_no_cloud_identity(self):
        # Such as `packer init`, which needs no cloud access
        self.run_packer(None)
        env = self.recorded()["env"]
        # The pinned names (test_preflight checks they are the code's)
        for name in AMBIENT_CREDENTIAL_VARIABLES:
            # Not assertNotIn: its failure message would print the whole
            # environment into the test log
            self.assertFalse(name in env, f"{name} reached Packer")
        self.assertEqual(env["FAKE_RECORD"], str(self.record))
