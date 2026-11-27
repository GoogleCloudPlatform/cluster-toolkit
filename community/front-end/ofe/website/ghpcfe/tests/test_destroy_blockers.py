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

"""Cluster teardown when Terraform destroy fails only on custom IAM roles.

Offline: Terraform is a stand-in script on PATH that replays each destroy run
from fixtures/destroy_blockers (the stderr Terraform printed with -no-color and
the state it left), and the Google client libraries are replaced at their own
module boundaries.
"""

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import textwrap
from pathlib import Path
from unittest import mock

from django.test import TestCase

from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager.clusterinfo import ClusterInfo
from ghpcfe.models import ComputeInstance, ContainerRegistry, Credential
from ghpcfe.tests.test_blueprint_identities import PROFILE_CASES, make_world
from ghpcfe.tests.test_credentials import PROJECT, make_key_json
from ghpcfe.tests.test_preflight import ALL_CLUSTER_PERMISSIONS, FakeGoogle

FIXTURES = Path(__file__).resolve().parent / "fixtures" / "destroy_blockers"

# The controller's roles in PROFILE_CASES["minimal"]: deployment alpha-0a1b2c3d,
# account ctl, role_id_suffix a1b2c3
CTL_ROLE = "alpha_0a1b2c3d.ctl.a1b2c3"
CTL_SCOPED_ROLE = "alpha_0a1b2c3d.ctl.a1b2c3.scoped"
CONTROLLER_ACCOUNT = f"alpha-0a1b2c3d-ctl@{PROJECT}.iam.gserviceaccount.com"

# Replays destroy runs: run n (counted in FAKE_TF_COUNT) takes the n-th
# [stderr, state, rc] of FAKE_TF_RUNS, copies state (if any) to
# ./terraform.tfstate, prints stderr (if any) and exits rc. A run beyond the
# list fails, so an extra destroy shows. `state pull` prints the state the
# last destroy left, as the state bucket would return it.
FAKE_TERRAFORM = textwrap.dedent(
    """\
    #!{python}
    import json
    import os
    import shutil
    import sys

    if sys.argv[1:3] == ["state", "pull"]:
        if os.path.exists("terraform.tfstate"):
            with open("terraform.tfstate", encoding="utf-8") as fp:
                sys.stdout.write(fp.read())
        sys.exit(0)
    if sys.argv[1:2] != ["destroy"]:
        sys.exit("the stand-in terraform only replays destroy")
    count_file = os.environ["FAKE_TF_COUNT"]
    with open(count_file, encoding="utf-8") as fp:
        run = int(fp.read() or "0")
    with open(count_file, "w", encoding="utf-8") as fp:
        fp.write(str(run + 1))
    runs = json.loads(os.environ["FAKE_TF_RUNS"])
    if run >= len(runs):
        sys.exit("the stand-in terraform was run more often than expected")
    stderr, state, rc = runs[run]
    if state:
        shutil.copyfile(state, "terraform.tfstate")
    if stderr:
        with open(stderr, encoding="utf-8") as fp:
            sys.stderr.write(fp.read())
    sys.exit(rc)
    """
)

# The state a full destroy leaves
DESTROYED = (None, "destroyed.state.json", 0)
# The fixed module's controller roles failed to delete. Nothing else of the
# cluster depends on them, so only they and their input checks are left.
ROLES_UNAVAILABLE = ("ctl_roles_unavailable.stderr", "ctl_roles_only_left.state.json", 1)
ROLES_DENIED = ("ctl_roles_denied.stderr", "ctl_roles_only_left.state.json", 1)


def credential_for(owner, project=PROJECT):
    """A key credential for project, owned by make_world's user (a second
    user would get a default role, which needs Role rows)"""
    return Credential.objects.create(
        name=f"key-{project}", owner=owner, detail=make_key_json(project=project)
    )


class DestroyBlockerTests(TestCase):
    """ClusterInfo.stop_cluster with a replayed Terraform destroy"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-destroy-blockers-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        bindir = self.tmp / "bin"
        bindir.mkdir()
        fake = bindir / "terraform"
        fake.write_text(FAKE_TERRAFORM.format(python=sys.executable))
        fake.chmod(stat.S_IRWXU)
        self.path = f"{bindir}{os.pathsep}{os.environ.get('PATH', '')}"

        world = make_world()
        self.owner = world[0]
        self.cluster = PROFILE_CASES["minimal"](world)
        self.cluster.cloud_credential = credential_for(self.owner)
        self.cluster.status = "r"
        self.cluster.cloud_state = "m"
        controller = ComputeInstance.objects.create(
            cloud_credential=self.cluster.cloud_credential,
            instance_type="n2-standard-2",
            service_account=CONTROLLER_ACCOUNT,
        )
        self.cluster.controller_node = controller
        self.cluster.save()
        self.login = ComputeInstance.objects.create(
            cloud_credential=self.cluster.cloud_credential,
            instance_type="n2-standard-2",
            cluster_login=self.cluster,
        )
        self.registry = ContainerRegistry.objects.create(
            cluster=self.cluster, status="r"
        )

        self.info = ClusterInfo(self.cluster)
        # Cluster ids restart in each test's database, so a directory can be
        # left by an earlier test or run
        shutil.rmtree(self.info.cluster_dir, ignore_errors=True)
        self.terraform_dir = self.info.get_terraform_dir()
        self.terraform_dir.mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.info.cluster_dir, ignore_errors=True)
        self.count_file = self.tmp / "terraform-runs"
        self.count_file.write_text("0")

    def destroy(self, *runs):
        """Destroy the cluster with a stand-in Terraform whose n-th run
        prints the n-th (stderr, state, rc) fixture's stderr, leaves its
        state and exits rc. Returns the mocks for the node sweep's client and
        the C2 subscription delete."""
        env = {
            "PATH": self.path,
            "FAKE_TF_COUNT": str(self.count_file),
            "FAKE_TF_RUNS": json.dumps([
                [
                    str(FIXTURES / stderr) if stderr else None,
                    str(FIXTURES / state) if state else None,
                    rc,
                ]
                for stderr, state, rc in runs
            ]),
        }
        with mock.patch.dict(os.environ, env), mock.patch(
            "google.cloud.compute_v1.InstancesClient"
        ) as instances, mock.patch(
            "ghpcfe.cluster_manager.c2.delete_cluster_subscription"
        ) as delete_subscription:
            instances.return_value.aggregated_list.return_value = []
            FakeGoogle(ALL_CLUSTER_PERMISSIONS).run(self.info.stop_cluster)
        return instances, delete_subscription

    def destroy_fails(self, *runs, error=subprocess.CalledProcessError):
        """destroy(), which must log and raise error as before"""
        with self.assertRaises(error) as caught, self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "ERROR"
        ) as logs:
            self.destroy(*runs)
        self.assertIn("Terraform destroy failed", "\n".join(logs.output))
        return caught.exception

    def terraform_runs(self):
        return int(self.count_file.read_text())

    def terraform_log(self):
        return (self.terraform_dir / "terraform_destroy_log.stderr").read_text()

    def assert_torn_down(self, instances, delete_subscription):
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "d")
        self.assertEqual(self.cluster.cloud_state, "xm")
        self.assertIsNone(self.cluster.controller_node)
        self.assertFalse(ComputeInstance.objects.filter(pk=self.login.pk).exists())
        self.registry.refresh_from_db()
        self.assertEqual(self.registry.status, "d")
        instances.return_value.aggregated_list.assert_called_once()
        delete_subscription.assert_called_once_with(
            self.cluster.id, CONTROLLER_ACCOUNT
        )

    def assert_not_torn_down(self):
        """The state a failed destroy leaves today"""
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "t")
        self.assertEqual(self.cluster.cloud_state, "dm")
        self.assertIsNotNone(self.cluster.controller_node)
        self.assertTrue(ComputeInstance.objects.filter(pk=self.login.pk).exists())
        self.registry.refresh_from_db()
        self.assertEqual(self.registry.status, "r")
        self.assertNotIn("OFE: Cluster", self.terraform_log())

    def assert_role_message(self, text):
        """The OFE line naming the roles left and how to delete them"""
        self.assertTrue(text.startswith("Cluster alpha (alpha-0a1b2c3d) is "))
        self.assertNotIn("\n", text.rstrip("\n"))
        self.assertIn(f"{CTL_ROLE}, {CTL_SCOPED_ROLE} in project {PROJECT}", text)
        self.assertIn("have no members", text)
        self.assertIn("which Google Cloud did not delete (the errors are above)", text)
        self.assertIn(
            f"gcloud iam roles delete {CTL_ROLE} --project={PROJECT}; "
            f"gcloud iam roles delete {CTL_SCOPED_ROLE} --project={PROJECT}",
            text,
        )

    def test_successful_destroy_is_unchanged(self):
        instances, delete_subscription = self.destroy(DESTROYED)
        self.assert_torn_down(instances, delete_subscription)
        self.assertEqual(self.terraform_log(), "")
        self.assertEqual(self.terraform_runs(), 1)

    def test_roles_left_finish_the_teardown_and_say_how_to_delete_them(self):
        with self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "WARNING"
        ) as logs:
            instances, delete_subscription = self.destroy(ROLES_UNAVAILABLE)
        self.assert_torn_down(instances, delete_subscription)
        # No retry: one destroy only
        self.assertEqual(self.terraform_runs(), 1)
        self.assertIn("Cluster alpha (alpha-0a1b2c3d) is destroyed", "\n".join(logs.output))
        log = self.terraform_log()
        # The Terraform log page shows the destroy's errors, then the message
        self.assertTrue(log.startswith(
            (FIXTURES / "ctl_roles_unavailable.stderr").read_text()
        ))
        message = log.rsplit("\nOFE: ", 1)[1]
        self.assert_role_message(message)
        self.assertNotIn("gcloud storage rm", message)

    def test_roles_denied_with_a_403_are_still_only_roles(self):
        instances, delete_subscription = self.destroy(ROLES_DENIED)
        self.assert_torn_down(instances, delete_subscription)
        self.assertEqual(self.terraform_runs(), 1)
        log = self.terraform_log()
        self.assertIn("lacks permission iam.roles.delete", log)
        self.assert_role_message(log.rsplit("\nOFE: ", 1)[1])

    def test_roles_and_the_slurm_bucket_left_say_how_to_delete_both(self):
        # The profiles' input checks name the Slurm files bucket, so a role
        # that cannot be deleted keeps it
        instances, delete_subscription = self.destroy(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_slurm_bucket_left.state.json", 1)
        )
        self.assert_torn_down(instances, delete_subscription)
        self.assertEqual(self.terraform_runs(), 1)
        message = self.terraform_log().rsplit("\nOFE: ", 1)[1]
        self.assert_role_message(message)
        self.assertIn("Slurm files bucket(s) slurm-alpha-0a1b2c3d-5e6f7a8b,", message)
        self.assertIn(
            "gcloud storage rm --recursive gs://slurm-alpha-0a1b2c3d-5e6f7a8b",
            message,
        )

    def test_roles_and_the_registry_left_say_how_to_delete_both(self):
        # The controller profile's artifact_registry_bindings name the
        # cluster's registry, so a role that cannot be deleted keeps it
        instances, delete_subscription = self.destroy(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_registry_left.state.json", 1)
        )
        self.assert_torn_down(instances, delete_subscription)
        self.assertEqual(self.terraform_runs(), 1)
        message = self.terraform_log().rsplit("\nOFE: ", 1)[1]
        self.assert_role_message(message)
        self.assertIn(
            "container registry repository alpha-0a1b2c3d-registry-1-1a2b (europe-west4)",
            message,
        )
        self.assertIn(
            "gcloud artifacts repositories delete alpha-0a1b2c3d-registry-1-1a2b "
            f"--location=europe-west4 --project={PROJECT} --quiet",
            message,
        )
        self.assertNotIn("gcloud secrets delete", message)

    def test_roles_and_the_registry_with_its_secret_left_say_how_to_delete_them(self):
        instances, delete_subscription = self.destroy(
            (
                "ctl_roles_unavailable.stderr",
                "ctl_roles_and_registry_with_secret_left.state.json",
                1,
            )
        )
        self.assert_torn_down(instances, delete_subscription)
        message = self.terraform_log().rsplit("\nOFE: ", 1)[1]
        self.assert_role_message(message)
        self.assertIn("registry password secret alpha-0a1b2c3d-registry-1-password", message)
        self.assertIn(
            f"gcloud secrets delete alpha-0a1b2c3d-registry-1-password --project={PROJECT} --quiet",
            message,
        )
        self.assertIn("gcloud artifacts repositories delete", message)

    def test_roles_and_another_repository_left_fail_as_before(self):
        # Only the blueprint's registry modules are expected to stay
        self.destroy_fails(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_other_repository_left.state.json", 1)
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_roles_and_another_bucket_left_fail_as_before(self):
        # Only the blueprint's Slurm files bucket is expected to stay
        self.destroy_fails(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_other_bucket_left.state.json", 1)
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_role_and_another_resource_fail_as_before(self):
        # The state alone would pass: the template's error decides
        self.destroy_fails(
            ("role_and_template.stderr", "ctl_roles_only_left.state.json", 1)
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_another_resource_fails_as_before(self):
        self.destroy_fails(("template_in_use.stderr", "template_left.state.json", 1))
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_403_on_another_resource_fails_as_before(self):
        error = self.destroy_fails(
            ("template_denied.stderr", "template_left.state.json", 1),
            error=credentials.PermissionDeniedRunError,
        )
        self.assertIn("compute.instanceTemplates.delete", str(error))
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_role_read_error_is_not_a_delete_failure(self):
        # Refresh failed before anything was destroyed. The state alone would
        # pass: the error's summary decides.
        self.destroy_fails(
            ("role_read_denied.stderr", "ctl_roles_only_left.state.json", 1),
            error=credentials.PermissionDeniedRunError,
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_roles_that_kept_other_resources_fail_as_before(self):
        # A cluster created before the module's roles stopped depending on
        # the act-as accounts: Terraform keeps the compute account and its
        # bindings when the roles fail, so the teardown must not finish.
        self.destroy_fails(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_compute_left.state.json", 1)
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_roles_and_a_service_account_left_fail_as_before(self):
        # A service account is never "creates nothing in the cloud"
        self.destroy_fails(
            ("ctl_roles_unavailable.stderr", "ctl_roles_and_account_left.state.json", 1)
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_roles_and_a_service_account_binding_left_fail_as_before(self):
        self.destroy_fails(
            (
                "ctl_roles_unavailable.stderr",
                "ctl_roles_and_account_binding_left.state.json",
                1,
            )
        )
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_without_state_fails_as_before(self):
        self.destroy_fails(("ctl_roles_unavailable.stderr", None, 1))
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()

    def test_roles_in_another_project_fail_as_before(self):
        # The command would name the credential's project, not the roles'
        self.cluster.cloud_credential = credential_for(
            self.owner, project="other-project"
        )
        self.cluster.save()
        self.destroy_fails(ROLES_UNAVAILABLE)
        self.assertEqual(self.terraform_runs(), 1)
        self.assert_not_torn_down()
