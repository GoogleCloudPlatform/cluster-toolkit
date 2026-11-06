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
runs.
"""
import subprocess

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
                    "-auto-approve"]])
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("d", "xm"))
