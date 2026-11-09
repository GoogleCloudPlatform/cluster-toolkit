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
"""A cluster whose start never reached Terraform can still be destroyed.

A start that failed in ghpc create, or before it, leaves no Terraform
directory, or one that was never initialised. `terraform destroy` there fails
every time, and a failed destroy keeps the cluster "being destroyed" and
refuses a start, so the cluster could neither be destroyed nor started. When
nothing can have been applied the destroy is skipped and only the clean-up
runs. With the state in the bucket, that is read from the state itself.

A destroy refused because the state is locked says so, rather than showing
an earlier run's error.
"""
import json
import subprocess
from unittest import mock

from ..cluster_manager import clusterinfo
from .test_cmek_revoked_destroy import BOXED_KMS_ERROR, DestroyTestCase


class NothingAppliedTests(DestroyTestCase):

    def assertDestroyed(self, db, commands, raised):
        self.assertEqual(commands, [])
        self.assertIsNone(raised)
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))
        self.assertEqual(db["status_message"], "")
        self.cleanup.sweep.assert_called_once()
        self.cleanup.registries.objects.filter.assert_called_once()
        self.cleanup.subscription.assert_called_once()

    def test_a_missing_directory(self):
        for revoked in (True, False):
            with self.subTest(revoked=revoked):
                db = {}
                commands, raised = self._destroy(
                    db, status="e", cloud_state="nm",
                    status_message="Start failed: ghpc create failed",
                    tf_dir_holds=None, cmek_revoked=revoked)
                self.assertDestroyed(db, commands, raised)

    def test_an_empty_directory(self):
        db = {}
        commands, raised = self._destroy(
            db, status="e", cloud_state="nm", tf_dir_holds=())
        self.assertDestroyed(db, commands, raised)

    def test_only_the_generated_files(self):
        # what ghpc create writes before any terraform init
        db = {}
        commands, raised = self._destroy(
            db, status="e", cloud_state="nm",
            tf_dir_holds=("main.tf", "variables.tf", "terraform.tfvars"))
        self.assertDestroyed(db, commands, raised)

    def test_a_failed_destroy_of_nothing_is_finished_by_a_retry(self):
        # a destroy that failed before this change left "dm" behind
        db = {}
        commands, raised = self._destroy(
            db, status="e", cloud_state="dm",
            status_message="Teardown failed: no such directory",
            tf_dir_holds=None)
        self.assertDestroyed(db, commands, raised)


class RegeneratedDirectoryTests(DestroyTestCase):
    """ghpc create replaces the group directory (and its `.terraform`), so a
    running cluster whose start or reconfigure stopped before `init` looks
    like nothing was applied. It must still be destroyed."""

    def test_an_applied_cluster_with_a_regenerated_directory(self):
        for cloud_state in ("m", "re"):
            with self.subTest(cloud_state=cloud_state):
                db = {}
                commands, raised = self._destroy(
                    db, status="e", cloud_state=cloud_state,
                    tf_dir_holds=("main.tf", "variables.tf"))
                self.assertIsNone(raised)
                self.assertEqual(len(commands), 1)
                self.assertEqual(commands[0][:2], ["terraform", "destroy"])
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("d", "xm"))


class SomethingMayHaveBeenAppliedTests(DestroyTestCase):

    def test_an_initialised_directory_whose_destroy_fails_stays_retryable(self):
        for revoked in (True, False):
            with self.subTest(revoked=revoked):
                db = {}
                commands, raised = self._destroy(
                    db, error=BOXED_KMS_ERROR, tf_dir_holds=(".terraform",),
                    cmek_revoked=revoked)
                self.assertEqual(len(commands), 1)
                self.assertEqual("-refresh=false" in commands[0], revoked)
                self.assertIsInstance(raised, subprocess.CalledProcessError)
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("e", "dm"))
                self.assertTrue(
                    db["status_message"].startswith("Teardown failed: "))
                self.cleanup.subscription.assert_not_called()

    def test_a_state_file_is_destroyed_as_before(self):
        for state in ("terraform.tfstate", "terraform.tfstate.backup"):
            with self.subTest(state=state):
                db = {}
                commands, raised = self._destroy(db, tf_dir_holds=(state,))
                self.assertIsNone(raised)
                self.assertEqual(commands, [[
                    "terraform", "destroy", "-no-color", "-input=false",
                    "-lock-timeout=45m", "-auto-approve"]])
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("d", "xm"))


GCS_BACKEND = (".terraform/terraform.tfstate", json.dumps({
    "backend": {"type": "gcs", "config": {
        "bucket": "ofe-state", "prefix": "20260101-0a1b2c3d/clusters/c_1"}}}))


class GcsStateTests(DestroyTestCase):
    """With the state in the bucket, the state itself says whether anything
    was applied: `init` alone, or an apply that created nothing, leaves an
    initialised directory and state with no resources."""

    def _destroy_with_state(self, db, state=None, error=None):
        pulls = []

        def pull(directory, extra_env=None):
            pulls.append(extra_env)
            if error:
                raise error
            return state

        with mock.patch.object(clusterinfo.utils, "get_terraform_state",
                               side_effect=pull):
            commands, raised = self._destroy(
                db, status="e", cloud_state="nm",
                tf_dir_holds=(GCS_BACKEND,))
        # read as the cluster's own credential, which the bucket requires
        self.assertEqual(pulls, [{"GOOGLE_APPLICATION_CREDENTIALS": str(
            self.tf_dir / "cloud_credentials")}])
        return commands, raised

    def test_state_with_resources_is_destroyed(self):
        db = {}
        commands, raised = self._destroy_with_state(
            db, {"version": 4, "resources": [{"type": "google_compute_disk"}]})
        self.assertIsNone(raised)
        self.assertEqual(len(commands), 1)
        self.assertEqual(commands[0][:2], ["terraform", "destroy"])
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))

    def test_state_without_resources_is_only_cleaned_up(self):
        db = {}
        commands, raised = self._destroy_with_state(
            db, {"version": 4, "resources": []})
        self.assertEqual(commands, [])
        self.assertIsNone(raised)
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))
        self.cleanup.subscription.assert_called_once()

    def test_unreadable_state_lets_the_destroy_run(self):
        db = {}
        with self.assertLogs(level="WARNING"):
            commands, raised = self._destroy_with_state(
                db, error=subprocess.CalledProcessError(
                    1, ["terraform", "state", "pull"]))
        self.assertIsNone(raised)
        self.assertEqual(len(commands), 1)
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))


class StateLockTests(DestroyTestCase):
    """A destroy refused for a state lock says so on the cluster's page."""

    def test_a_stale_lock_is_reported_not_an_earlier_log(self):
        message = ("Terraform state for /x/primary is locked. The lock was "
                   "taken on this server and no Terraform process on this "
                   "server holds it, so it is stale.\n  Lock ID:   17")
        refused = clusterinfo.utils.TerraformStateLockError(
            ["terraform", "destroy"], message)
        db = {}
        # refused before Terraform runs, so this log is an earlier run's
        earlier_log = ("terraform_destroy_log.stderr",
                       "Error: an earlier, unrelated failure\n")
        with mock.patch.object(clusterinfo.utils, "_check_stale_state_lock",
                               side_effect=refused), \
                self.assertLogs(level="ERROR"):
            commands, raised = self._destroy(
                db, tf_dir_holds=(".terraform", earlier_log))
        self.assertEqual(commands, [])
        self.assertIs(raised, refused)
        self.assertEqual((db["status"], db["cloud_state"]), ("e", "dm"))
        self.assertEqual(db["status_message"],
                         "Teardown failed: " + message.splitlines()[0])
