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

"""manage.py ofe_terraform_as_credential: Terraform as a resource's credential.

script/migrate_tf_state_to_gcs.sh runs every Terraform command through it,
because the key files older releases kept beside each resource are deleted
at every server start. Each test runs the command as the script does, from
its command line, with Terraform itself replaced at subprocess.run.
"""

import io
import json
import os
import subprocess
from pathlib import Path
from unittest import mock

from django.conf import settings
from django.test import TestCase
from google.api_core.exceptions import PermissionDenied

from ghpcfe.cluster_manager import credentials
from ghpcfe.management.commands import ofe_terraform_as_credential
from ghpcfe.models import Credential, VirtualNetwork
from ghpcfe.tests.test_credentials import (
    ACCOUNT,
    KEY_JSON,
    PROJECT,
    impersonate_credential,
    key_credential,
)

COMMAND = "ofe_terraform_as_credential"
PLAN = ["-chdir=/srv/vpcs/vpc_1", "plan", "-detailed-exitcode", "-no-color"]


class FakeTerraform:
    """Stands in for subprocess.run; records each run and what its
    environment held while it ran"""

    def __init__(self, returncode=0, error=None):
        self.returncode = returncode
        # Raised instead of running, as subprocess.run raises when the
        # program cannot be started
        self.error = error
        self.calls = []

    def __call__(self, cmdline, **kwargs):
        if self.error is not None:
            raise self.error
        env = dict(kwargs["env"])
        config = Path(env["CLOUDSDK_CONFIG"])
        self.calls.append({
            "cmdline": list(cmdline),
            "kwargs": kwargs,
            "env": env,
            "config_existed": config.is_dir(),
            "config": config,
            "run_dir_files": sorted(os.listdir(config.parent)),
        })
        return subprocess.CompletedProcess(cmdline, self.returncode)


class TerraformAsCredentialTests(TestCase):
    """Runs Terraform only, as the stored credential, with Terraform's
    exit code"""

    AMBIENT = {
        "PATH": "/usr/bin:/bin",
        "GOOGLE_APPLICATION_CREDENTIALS": "/srv/vpcs/vpc_1/cloud_credentials",
        "GOOGLE_CREDENTIALS": "ambient key",
        "CLOUDSDK_CORE_ACCOUNT": "someone@example.com",
    }

    def make_vpc(self, credential):
        return VirtualNetwork.objects.create(
            name="net-a",
            cloud_id="net-a",
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_state="m",
        )

    def run_command(self, *argv, returncode=0, error=None):
        """Run the command from its command line.

        Returns (exit code, stderr, the FakeTerraform).
        """
        terraform = FakeTerraform(returncode, error)
        stderr = io.StringIO()
        command = ofe_terraform_as_credential.Command(
            stdout=io.StringIO(), stderr=stderr
        )
        exit_code = 0
        with mock.patch.dict(os.environ, self.AMBIENT, clear=True), \
                mock.patch("subprocess.run", side_effect=terraform):
            try:
                command.run_from_argv(["manage.py", COMMAND, *argv])
            except SystemExit as err:
                exit_code = err.code
        return exit_code, stderr.getvalue(), terraform

    def assert_refused(self, argv, *expected):
        exit_code, stderr, terraform = self.run_command(*argv)
        self.assertEqual(exit_code, 1, stderr)
        self.assertTrue(stderr.startswith("CommandError: "), stderr)
        for text in expected:
            self.assertIn(text, stderr)
        self.assertEqual(terraform.calls, [])
        return stderr

    # Running as the credential

    def test_impersonate_credential_gives_impersonation(self):
        vpc = self.make_vpc(impersonate_credential())
        exit_code, stderr, terraform = self.run_command(
            "vpc", str(vpc.id), "--", *PLAN
        )
        self.assertEqual(exit_code, 0, stderr)
        self.assertEqual(len(terraform.calls), 1)
        env = terraform.calls[0]["env"]
        self.assertEqual(env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], PROJECT)
        self.assertNotIn("GOOGLE_CREDENTIALS", env)
        self.assertNotIn("GOOGLE_APPLICATION_CREDENTIALS", env)
        self.assertNotIn("CLOUDSDK_CORE_ACCOUNT", env)
        self.assertEqual(env["PATH"], "/usr/bin:/bin")

    def test_key_credential_gives_key_in_memory_without_key_file(self):
        vpc = self.make_vpc(key_credential())
        exit_code, stderr, terraform = self.run_command(
            "vpc", str(vpc.id), "--", *PLAN
        )
        self.assertEqual(exit_code, 0, stderr)
        call = terraform.calls[0]
        env = call["env"]
        self.assertEqual(env["GOOGLE_CREDENTIALS"], KEY_JSON)
        self.assertNotIn("GOOGLE_APPLICATION_CREDENTIALS", env)
        self.assertNotIn("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", env)
        self.assertNotIn("GOOGLE_IMPERSONATE_SERVICE_ACCOUNT", env)
        # The run's own directory held no key file, and is gone afterwards
        self.assertTrue(call["config_existed"])
        self.assertEqual(call["run_dir_files"], ["gcloud"])
        self.assertFalse(call["config"].parent.exists())

    def test_only_terraform_runs_with_the_arguments_as_given(self):
        vpc = self.make_vpc(impersonate_credential())
        exit_code, _, terraform = self.run_command(
            "vpc", str(vpc.id), "--", *PLAN
        )
        self.assertEqual(exit_code, 0)
        self.assertEqual(len(terraform.calls), 1)
        call = terraform.calls[0]
        self.assertEqual(call["cmdline"], ["terraform", *PLAN])
        # No shell, and Terraform's output goes straight to the caller
        self.assertNotIn("shell", call["kwargs"])
        self.assertNotIn("stdout", call["kwargs"])
        self.assertNotIn("stderr", call["kwargs"])
        self.assertFalse(call["kwargs"]["check"])

    def test_argument_naming_another_program_is_refused(self):
        vpc = self.make_vpc(impersonate_credential())
        self.assert_refused(
            ["vpc", str(vpc.id), "--", "/bin/sh", "-c", "true"],
            "Terraform '/bin/sh' is not run by this command",
        )

    def test_each_allowed_subcommand_runs(self):
        vpc = self.make_vpc(impersonate_credential())
        for subcommand in ("version", "init", "validate", "plan", "show", "output"):
            for argv in (
                [subcommand, "-no-color"],
                ["-chdir=/srv/vpcs/vpc_1", subcommand, "-no-color"],
            ):
                with self.subTest(argv=argv):
                    exit_code, stderr, terraform = self.run_command(
                        "vpc", str(vpc.id), "--", *argv
                    )
                    self.assertEqual(exit_code, 0, stderr)
                    self.assertEqual(
                        terraform.calls[0]["cmdline"], ["terraform", *argv]
                    )

    def test_terraform_exit_code_is_the_command_exit_code(self):
        vpc = self.make_vpc(impersonate_credential())
        for returncode in (0, 1, 2):
            with self.subTest(returncode=returncode):
                exit_code, stderr, terraform = self.run_command(
                    "vpc", str(vpc.id), "--", *PLAN, returncode=returncode
                )
                self.assertEqual(exit_code, returncode)
                self.assertEqual(stderr, "")
                self.assertEqual(len(terraform.calls), 1)

    # Refusals

    def test_refuses_subcommands_that_change_resources_or_state(self):
        vpc = self.make_vpc(impersonate_credential())
        for argv in (
            ["apply", "-auto-approve"],
            ["-chdir=/srv/vpcs/vpc_1", "destroy", "-auto-approve"],
            ["-chdir=/srv/vpcs/vpc_1", "state", "rm", "google_compute_network.x"],
            ["import", "google_compute_network.x", "net-a"],
            ["refresh"],
            ["force-unlock", "-force", "1234"],
            ["taint", "google_compute_network.x"],
        ):
            with self.subTest(argv=argv):
                stderr = self.assert_refused(
                    ["vpc", str(vpc.id), "--", *argv],
                    "is not run by this command",
                    "version, init, validate, plan, show, output",
                    "apply and destroy need the key file",
                    "must go through TKFE",
                )
                subcommand = next(a for a in argv if not a.startswith("-"))
                self.assertIn(f"Terraform '{subcommand}'", stderr)

    def test_an_allowed_word_after_a_refused_subcommand_is_refused(self):
        # Only the first argument that is not an option is the subcommand;
        # an allowed word later on (a plan file named "plan", "state show")
        # does not make a refused subcommand run.
        vpc = self.make_vpc(impersonate_credential())
        for argv, subcommand in (
            (["destroy", "plan"], "destroy"),
            (["state", "show", "google_compute_network.x"], "state"),
            (["-chdir=/x", "apply", "plan"], "apply"),
            (["-chdir=/x", "destroy", "-auto-approve", "version"], "destroy"),
        ):
            with self.subTest(argv=argv):
                stderr = self.assert_refused(
                    ["vpc", str(vpc.id), "--", *argv],
                    f"Terraform '{subcommand}' is not run by this command",
                    "must go through TKFE",
                )
                self.assertNotIn("No Terraform subcommand", stderr)

    def test_refuses_arguments_without_a_subcommand(self):
        vpc = self.make_vpc(impersonate_credential())
        self.assert_refused(
            ["vpc", str(vpc.id), "--", "-chdir=/srv/vpcs/vpc_1", "-help"],
            "No Terraform subcommand was given; this command runs only",
        )

    def test_subcommand_is_checked_before_the_resource(self):
        # Nothing is read from the database for a refused subcommand
        self.assert_refused(
            ["vpc", "999", "--", "destroy"], "Terraform 'destroy' is not run"
        )

    def test_refuses_an_id_too_large_for_the_database(self):
        huge = "9" * 30
        stderr = self.assert_refused(
            ["vpc", huge, "--", "version"], f"There is no vpc with id {huge}"
        )
        self.assertNotIn("Traceback", stderr)

    def test_refuses_when_terraform_cannot_be_started(self):
        vpc = self.make_vpc(impersonate_credential())
        for error in (
            FileNotFoundError(2, "No such file or directory", "terraform"),
            PermissionError(13, "Permission denied", "terraform"),
        ):
            with self.subTest(error=type(error).__name__):
                exit_code, stderr, terraform = self.run_command(
                    "vpc", str(vpc.id), "--", *PLAN, error=error
                )
                self.assertEqual(exit_code, 1, stderr)
                self.assertTrue(stderr.startswith("CommandError: "), stderr)
                self.assertIn(f"vpc {vpc.id}: terraform cannot be run", stderr)
                self.assertIn(error.strerror, stderr)
                self.assertIn("Terraform is installed and on PATH", stderr)
                self.assertNotIn("Traceback", stderr)
                self.assertEqual(terraform.calls, [])

    def test_refuses_a_resource_that_does_not_exist(self):
        for kind in sorted(ofe_terraform_as_credential.MODELS):
            with self.subTest(kind=kind):
                self.assert_refused(
                    [kind, "999", "--", "version"], f"There is no {kind} with id 999"
                )
        self.assert_refused(
            ["vpc", "1;x", "--", "version"], "There is no vpc with id 1;x"
        )

    def test_refuses_a_resource_without_a_credential(self):
        unsaved = VirtualNetwork(name="net-b", cloud_region="us-central1")
        lookup = mock.Mock()
        lookup.return_value.first.return_value = unsaved
        with mock.patch.object(VirtualNetwork.objects, "filter", lookup):
            self.assert_refused(
                ["vpc", "7", "--", "version"],
                "vpc 7 has no credential to run Terraform as",
            )
        lookup.assert_called_once_with(pk=7)

    def test_refuses_a_resource_whose_credential_is_gone(self):
        dangling = VirtualNetwork(
            name="net-b", cloud_region="us-central1", cloud_credential_id=4242
        )
        lookup = mock.Mock()
        lookup.return_value.first.return_value = dangling
        with mock.patch.object(VirtualNetwork.objects, "filter", lookup):
            self.assert_refused(
                ["vpc", "7", "--", "version"],
                "vpc 7: its credential (id 4242) no longer exists",
            )

    def test_refuses_a_credential_that_cannot_be_read(self):
        unreadable = key_credential(name="broken-key", text="not a key")
        vpc = self.make_vpc(unreadable)
        self.assert_refused(
            ["vpc", str(vpc.id), "--", *PLAN],
            f"vpc {vpc.id}: its stored credential broken-key "
            f"(id {unreadable.id}) cannot be used",
            "does not hold a service account key",
        )

    def test_refuses_when_the_database_key_cannot_be_fetched(self):
        vpc = self.make_vpc(key_credential())
        with mock.patch.object(
            credentials,
            "key_json",
            side_effect=PermissionDenied("secretmanager.versions.access"),
        ):
            self.assert_refused(
                ["vpc", str(vpc.id), "--", *PLAN],
                f"vpc {vpc.id}: its stored credential key-cred",
                "secretmanager.versions.access",
            )

    def test_refuses_when_the_database_key_is_malformed(self):
        credential = key_credential()
        vpc = self.make_vpc(credential)
        malformed = "not-a-fernet-key"
        with self.settings(OFE_CREDENTIAL_ENCRYPTION_KEY=malformed):
            stderr = self.assert_refused(
                ["vpc", str(vpc.id), "--", *PLAN],
                f"vpc {vpc.id}: its stored credential key-cred "
                f"(id {credential.id}) cannot be used",
                "The database key in the OFE_CREDENTIAL_ENCRYPTION_KEY "
                "setting is missing or is not a Fernet key (ValueError)",
            )
        self.assertNotIn("Traceback", stderr)
        self.assertNotIn(malformed, stderr)
        self.assertNotIn("private_key", stderr)

    def test_refuses_a_stored_key_tkfe_did_not_write(self):
        # A row whose detail was written outside TKFE: it carries TKFE's
        # encrypted-key prefix, but is not what TKFE stores. Reading it
        # raises a ValueError, whose text may quote part of the key.
        server_key = credentials.Fernet(settings.OFE_CREDENTIAL_ENCRYPTION_KEY)
        for detail, error in (
            # A token that is not ASCII
            (f"{credentials.ENCRYPTED_PREFIX}{KEY_JSON}é",
             "UnicodeEncodeError"),
            # Key JSON encrypted with the server's key, but not as UTF-8
            (credentials.ENCRYPTED_PREFIX + server_key.encrypt(
                KEY_JSON.encode("utf-16")).decode("ascii"),
             "UnicodeDecodeError"),
        ):
            with self.subTest(error=error):
                credential = key_credential(name=f"foreign-{error}")
                Credential.objects.filter(pk=credential.pk).update(
                    detail=detail
                )
                vpc = self.make_vpc(credential)
                stderr = self.assert_refused(
                    ["vpc", str(vpc.id), "--", *PLAN],
                    f"vpc {vpc.id}: its stored credential foreign-{error} "
                    f"(id {credential.id}) cannot be used",
                    "its stored key was not written by TKFE and cannot be "
                    f"read ({error})",
                )
                self.assertNotIn("Traceback", stderr)
                self.assertNotIn("private_key", stderr)
                self.assertNotIn("é", stderr)
                self.assertNotIn(
                    json.loads(KEY_JSON)["private_key"], stderr
                )

    def test_refuses_when_the_credential_directory_cannot_be_made(self):
        credential = impersonate_credential()
        vpc = self.make_vpc(credential)
        with mock.patch.object(
            credentials.tempfile,
            "mkdtemp",
            side_effect=OSError(28, "No space left on device"),
        ):
            stderr = self.assert_refused(
                ["vpc", str(vpc.id), "--", *PLAN],
                f"vpc {vpc.id}: its stored credential keyless-cred "
                f"(id {credential.id}) cannot be used",
                "private credential directory cannot be made",
                "No space left on device",
            )
        self.assertNotIn("Traceback", stderr)
