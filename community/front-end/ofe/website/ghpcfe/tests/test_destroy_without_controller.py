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

"""Tearing down a cluster whose start failed before it recorded a
controller, or whose Terraform folder is missing.

start_cluster() can fail before the cluster has a controller_node. The
teardown must still finish (status "d", cloud state "xm") and clean up the
container registries, login nodes and C2 subscription, rather than stop at
the missing controller and leave the cluster in "t".

The cluster's Terraform folder (<deployment folder>/primary) can be
missing: a create that stopped before gcluster wrote it, or a server rebuilt
with a restored database. Terraform can destroy only from that folder, so
the cluster's state in the state bucket decides. With state objects that
hold no resources, or no state object for a cluster that never started an
apply, the teardown finishes without running Terraform and still deletes the
cluster's C2 subscription (none, NotFound, counts as gone), and sweeps the
compute nodes of a cluster that was deployed. With resources, a state that
cannot be read, or no state object for a cluster that was deployed, it is
refused and the cluster keeps its status. A cluster counts as deployed once
a Terraform apply has started for it (terraform_started, which nothing
clears), even when the apply failed part way. A Retry of the create keeps
a cloud state ("m", or "dm" after a destroy that started) that says its
resources may exist.

Offline: the real ClusterInfo destroy runs with the destroy blocker tests'
stand-in Terraform on PATH, and the Cloud Storage client is a fake state
bucket at the google.cloud.storage.Client boundary.
"""

import json
import os
import shutil
import subprocess
from unittest import mock

from django.db import connection
from django.db.models.signals import pre_save
from django.test import TestCase
from django.test.utils import CaptureQueriesContext
from google.api_core.exceptions import Forbidden, NotFound

from ghpcfe.cluster_manager import credentials, utils
from ghpcfe.cluster_manager.clusterinfo import ClusterInfo
from ghpcfe.models import Cluster, ComputeInstance, Credential
from ghpcfe.tests import test_destroy_blockers as blockers
from ghpcfe.tests.test_blueprint_identities import PROFILE_CASES, make_world
from ghpcfe.tests.test_credentials import PROJECT, make_key_json
from ghpcfe.tests.test_preflight import FakeGoogle

# tests/fixtures/test_settings_config.yaml
STATE_BUCKET = "ofe-test-state-bucket"
# Why a cluster whose terraform_started is True counts as deployed: a set
# flag and a row from before the flag (True by db_default) look the same
FLAG_REASON = (
    "a Terraform apply was started for it, or the cluster predates the "
    "started-flag check, so OFE treats it as deployed"
)


def fixture_state(name):
    return (blockers.FIXTURES / name).read_bytes()


def state_document(*resources):
    """A Terraform state (format version 4) holding resources"""
    return json.dumps({
        "version": 4,
        "terraform_version": "1.16.1",
        "serial": 3,
        "lineage": "6a1f6c2e-3b7d-4f0a-9c1e-2d5b8e7f4a10",
        "outputs": {},
        "resources": list(resources),
        "check_results": None,
    }).encode("utf-8")


def resource(mode="managed", type_="google_compute_network", instances=1):
    return {
        "mode": mode,
        "type": type_,
        "name": "r",
        "provider": 'provider["registry.terraform.io/hashicorp/google"]',
        "instances": [{"attributes": {}} for _ in range(instances)],
    }


class FakeBlob:
    def __init__(self, name, data):
        self.name = name
        self._data = data

    def download_as_bytes(self):
        return self._data


class FakeStateBucket:
    """Cloud Storage clients over the state bucket holding objects
    ({name: bytes}), whose listing fails with error when it is set.

    Records the arguments each client was made with and each listing.
    """

    def __init__(self, objects=None, error=None):
        self.objects = dict(objects or {})
        self.error = error
        self.clients = []
        self.listed = []

    def client(self, *args, **kwargs):
        self.clients.append(kwargs)
        client = mock.Mock(name="storage")
        client.list_blobs.side_effect = self.list_blobs
        return client

    def list_blobs(self, bucket, prefix=None):
        self.listed.append((bucket, prefix))
        return self._listing(bucket, prefix or "")

    def _listing(self, bucket, prefix):
        # Like the client's HTTP iterator, a listing fails only when read
        if self.error is not None:
            raise self.error
        for name, data in sorted(self.objects.items()):
            if bucket == STATE_BUCKET and name.startswith(prefix):
                yield FakeBlob(name, data)

    def storage_patch(self):
        """FakeGoogle's Cloud Storage client replaced by this bucket's"""
        bucket = self

        def storage(unused_google, *args, **kwargs):
            return bucket.client(*args, **kwargs)

        return mock.patch.object(FakeGoogle, "storage", storage)


class DestroyWithoutControllerTests(TestCase):
    """ClusterInfo.stop_cluster for a cluster with no controller_node"""

    # The teardown set-up and stand-in Terraform of the destroy blocker
    # tests, without running those tests again
    setUp = blockers.DestroyBlockerTests.setUp
    destroy = blockers.DestroyBlockerTests.destroy

    def test_teardown_without_controller_node_reaches_deleted(self):
        controller = self.cluster.controller_node
        self.cluster.controller_node = None
        self.cluster.status = "e"
        self.cluster.save()
        controller.delete()
        self.cluster.refresh_from_db()
        self.assertIsNone(self.cluster.controller_node)

        instances, delete_subscription = self.destroy(blockers.DESTROYED)

        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "d")
        self.assertEqual(self.cluster.cloud_state, "xm")
        self.assertFalse(
            ComputeInstance.objects.filter(pk=self.login.pk).exists()
        )
        self.registry.refresh_from_db()
        self.assertEqual(self.registry.status, "d")
        instances.return_value.aggregated_list.assert_called_once()
        # The subscription is still deleted, with no controller account
        delete_subscription.assert_called_once_with(self.cluster.id, None)


class DestroyWithoutDeploymentFolderTests(TestCase):
    """ClusterInfo.stop_cluster for a cluster whose Terraform folder is
    missing: the state in the bucket decides"""

    blockers_setUp = blockers.DestroyBlockerTests.setUp
    destroy_with_terraform = blockers.DestroyBlockerTests.destroy
    terraform_runs = blockers.DestroyBlockerTests.terraform_runs

    def setUp(self):
        self.blockers_setUp()
        self.prefix = utils.terraform_state_prefix("cluster", self.cluster.id)
        self.state_name = f"{self.prefix}/default.tfstate"
        self.deployment_dir = self.info.cluster_dir / self.cluster.cloud_id
        self.bucket = FakeStateBucket()

    def without_deployment_folder(self, status="e", cloud_state="nm"):
        """The cluster as a create that failed before gcluster ran leaves
        it: its working directory, but no deployment folder, controller or
        login node"""
        controller = self.cluster.controller_node
        self.cluster.controller_node = None
        self.cluster.status = status
        self.cluster.cloud_state = cloud_state
        self.cluster.save()
        controller.delete()
        shutil.rmtree(self.deployment_dir)
        self.assertTrue(self.info.cluster_dir.is_dir())
        self.cluster.refresh_from_db()

    def destroy(self, *runs):
        """The destroy blocker tests' destroy(), with the fake state
        bucket"""
        with self.bucket.storage_patch():
            return self.destroy_with_terraform(*runs)

    def destroy_with_subscription_error(self, error):
        """destroy() with no Terraform run expected, whose C2 subscription
        delete raises error; returns the mocks destroy() returns"""
        env = {
            "PATH": self.path,
            "FAKE_TF_COUNT": str(self.count_file),
            "FAKE_TF_RUNS": "[]",
        }
        with self.bucket.storage_patch(), mock.patch.dict(
            os.environ, env
        ), mock.patch(
            "google.cloud.compute_v1.InstancesClient"
        ) as instances, mock.patch(
            "ghpcfe.cluster_manager.c2.delete_cluster_subscription",
            side_effect=error,
        ) as delete_subscription:
            FakeGoogle(blockers.ALL_CLUSTER_PERMISSIONS).run(
                self.info.stop_cluster
            )
        return instances, delete_subscription

    def assert_deleted_without_terraform(
        self, instances, delete_subscription, swept=False
    ):
        """Deleted without Terraform; swept says whether the cluster was
        deployed, so its compute nodes were swept"""
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "d")
        self.assertEqual(self.cluster.cloud_state, "xm")
        self.assertEqual(self.terraform_runs(), 0)
        self.assertFalse(self.deployment_dir.exists())
        self.registry.refresh_from_db()
        self.assertEqual(self.registry.status, "d")
        self.assertFalse(
            ComputeInstance.objects.filter(pk=self.login.pk).exists()
        )
        # A cluster that never started an apply made no node to sweep; one
        # that did can have nodes slurm-gcp made outside its state. The C2
        # subscription (made just before the first apply) is still deleted:
        # a server rebuilt with a restored database can have lost the
        # deployment folder of a cluster that has one
        if swept:
            self.assert_swept(instances)
        else:
            instances.return_value.aggregated_list.assert_not_called()
        delete_subscription.assert_called_once_with(self.cluster.id, None)
        # The state was listed once, under the cluster's own prefix
        self.assertEqual(
            self.bucket.listed, [(STATE_BUCKET, f"{self.prefix}/")]
        )

    def assert_swept(self, instances):
        """The node sweep listed the instances labelled with the cluster's
        Slurm cluster name, in its project"""
        instances.return_value.aggregated_list.assert_called_once()
        request = instances.return_value.aggregated_list.call_args.kwargs[
            "request"
        ]
        self.assertEqual(request.project, self.cluster.project_id)
        self.assertEqual(
            request.filter,
            f"labels.slurm_cluster_name={self.cluster.slurm_cluster_name}",
        )

    def refused(self, missing="is missing"):
        """destroy(), which must be refused before anything changes and
        before Terraform runs; returns the refusal's text"""
        before = (self.cluster.status, self.cluster.cloud_state)
        with self.assertRaises(credentials.CredentialError) as caught, \
                mock.patch.object(
                    utils, "run_terraform", wraps=utils.run_terraform
                ) as run_terraform, \
                self.assertLogs("ghpcfe.cluster_manager.clusterinfo", "ERROR"):
            self.destroy()
        self.assertEqual(self.terraform_runs(), 0)
        run_terraform.assert_not_called()
        self.cluster.refresh_from_db()
        self.assertEqual(
            (self.cluster.status, self.cluster.cloud_state), before
        )
        self.registry.refresh_from_db()
        self.assertEqual(self.registry.status, "r")
        self.assertTrue(
            ComputeInstance.objects.filter(pk=self.login.pk).exists()
        )
        refusal = str(caught.exception)
        self.assertTrue(refusal.startswith(
            "Cannot destroy cluster alpha (alpha-0a1b2c3d): its Terraform "
            f"folder {self.terraform_dir} {missing}, and "
        ), refusal)
        self.assertIn(f"gs://{STATE_BUCKET}/{self.prefix}/", refusal)
        self.assertTrue(refusal.endswith("The cluster keeps its status."))
        # Also on the cluster's log page
        log = credentials.shown_log_file(self.info.get_terraform_dir())
        self.assertIsNotNone(log)
        self.assertIn(f"OFE: {refusal}", log.read_text(encoding="utf-8"))
        return refusal

    def test_no_state_reaches_deleted_without_terraform(self):
        # A create that failed before its first apply started: "nm", no
        # controller and no apply started, so no state object means nothing
        # was deployed
        self.without_deployment_folder(status="e", cloud_state="nm")
        self.assertFalse(self.cluster.terraform_started)
        with self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "INFO"
        ) as logs:
            instances, delete_subscription = self.destroy()
        self.assert_deleted_without_terraform(instances, delete_subscription)
        self.assertIn(
            f"it has no Terraform state in gs://{STATE_BUCKET}/{self.prefix}/ "
            "and never started a Terraform apply, so nothing is left to "
            "destroy",
            "\n".join(logs.output),
        )

    def assert_no_state_refusal(self, refusal, evidence):
        self.assertIn(
            f"no Terraform state was found under "
            f"gs://{STATE_BUCKET}/{self.prefix}/, although the cluster may "
            f"have been deployed ({evidence}), so its resources may still "
            "exist",
            refusal,
        )
        for cause in (
            "never migrated to the state bucket",
            "deployed under another deployment name",
            "restored with a new state namespace",
            "\"When a resource's state is not in the bucket\" in "
            "docs/admin_guide.md",
        ):
            self.assertIn(cause, refusal)

    def test_no_state_for_a_managed_cluster_is_refused(self):
        # A deployed cluster whose state is not where this server looks:
        # unmigrated local state lost with a disk, another deployment
        # name's bucket, or a restored database's new namespace
        self.without_deployment_folder(status="r", cloud_state="m")
        self.assert_no_state_refusal(
            self.refused(), 'its cloud state is "m"'
        )

    def test_no_state_after_a_started_destroy_is_refused(self):
        self.without_deployment_folder(status="t", cloud_state="dm")
        self.assert_no_state_refusal(
            self.refused(), 'its cloud state is "dm"'
        )

    def test_no_state_for_a_cluster_with_a_controller_is_refused(self):
        # Only a finished apply records the controller, whatever the
        # cloud state says
        controller = self.cluster.controller_node
        self.without_deployment_folder(status="e", cloud_state="nm")
        controller.save()
        self.cluster.controller_node = controller
        self.cluster.save()
        self.assert_no_state_refusal(
            self.refused(), "it has a controller recorded"
        )
        self.assertTrue(
            ComputeInstance.objects.filter(pk=controller.pk).exists()
        )

    def failed_apply(self):
        """Run the real _apply_terraform of a create whose `terraform apply`
        fails part way (a stand-in at the run_terraform boundary); returns
        the terraform_started the database held when each Terraform command
        started, as (command, flag) pairs"""
        seen = []

        def run_terraform(target_dir, command, *args, **kwargs):
            seen.append((
                command,
                Cluster.objects.get(pk=self.cluster.pk).terraform_started,
            ))
            raise subprocess.CalledProcessError(1, ["terraform", command])

        with mock.patch.object(
            utils, "run_terraform", side_effect=run_terraform
        ), mock.patch(
            "ghpcfe.cluster_manager.c2.create_cluster_subscription"
        ), self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "ERROR"
        ), self.assertRaises(subprocess.CalledProcessError):
            self.info._apply_terraform()  # pylint: disable=protected-access
        return seen

    def test_the_flag_is_set_before_the_first_apply(self):
        self.without_deployment_folder(status="c", cloud_state="nm")
        self.assertFalse(self.cluster.terraform_started)
        self.assertEqual(self.failed_apply(), [("apply", True)])
        self.cluster.refresh_from_db()
        self.assertTrue(self.cluster.terraform_started)
        # The failed apply leaves the cloud state as it was
        self.assertEqual(self.cluster.cloud_state, "nm")

    def test_no_state_after_a_partial_apply_is_refused(self):
        # An apply that failed part way can leave resources, while the
        # cloud state stays "nm" and no controller is recorded
        self.without_deployment_folder(status="c", cloud_state="nm")
        self.failed_apply()
        self.cluster.status = "e"
        self.cluster.save()
        self.assert_no_state_refusal(self.refused(), FLAG_REASON)
        self.cluster.refresh_from_db()
        self.assertTrue(self.cluster.terraform_started)

    def test_a_stale_copy_saved_after_the_apply_started_keeps_the_flag(self):
        # A copy of the row loaded before the apply started (a page or task
        # holding it) and saved in full afterwards must not clear the flag,
        # or this destroy would take the missing state to mean nothing was
        # deployed and remove the cluster without Terraform
        self.without_deployment_folder(status="c", cloud_state="nm")
        stale = Cluster.objects.get(pk=self.cluster.pk)
        self.failed_apply()
        self.assertFalse(stale.terraform_started)
        stale.status = "e"
        stale.save()
        self.assertTrue(stale.terraform_started)
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")
        self.assertTrue(self.cluster.terraform_started)
        self.assert_no_state_refusal(self.refused(), FLAG_REASON)

    def test_setting_the_flag_writes_no_other_field(self):
        # The apply's copy is stale too: what another copy saved meanwhile
        # is kept
        self.without_deployment_folder(status="c", cloud_state="nm")
        other = Cluster.objects.get(pk=self.cluster.pk)
        other.status = "e"
        other.save()
        self.failed_apply()
        self.assertTrue(self.cluster.terraform_started)
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual((row.status, row.terraform_started), ("e", True))

    def test_no_state_for_a_cluster_from_before_the_flag_is_refused(self):
        # The migration gives rows from before the flag True (db_default),
        # so a failed create from then counts as deployed; the refusal
        # cannot tell that from a started apply, so it names both
        field = Cluster._meta.get_field("terraform_started")
        self.assertIs(field.default, False)
        self.assertIs(field.db_default, True)
        self.without_deployment_folder(status="e", cloud_state="nm")
        Cluster.objects.filter(pk=self.cluster.pk).update(
            terraform_started=True
        )
        self.cluster.refresh_from_db()
        refusal = self.refused()
        self.assert_no_state_refusal(refusal, FLAG_REASON)
        self.assertIn(
            "predates the started-flag check, so OFE treats it as deployed",
            refusal,
        )

    def test_empty_state_after_a_partial_apply_sweeps_its_nodes(self):
        # The state holds nothing, but slurm-gcp's compute nodes were never
        # in it
        self.bucket.objects = {self.state_name: state_document()}
        self.without_deployment_folder(status="e", cloud_state="nm")
        Cluster.objects.filter(pk=self.cluster.pk).update(
            terraform_started=True
        )
        self.cluster.refresh_from_db()
        self.assert_deleted_without_terraform(*self.destroy(), swept=True)

    def test_missing_primary_folder_reads_the_state_bucket(self):
        # The deployment folder is there, but not the group folder that
        # Terraform runs in: the bucket decides, as with no deployment
        # folder, instead of a destroy failing in a missing directory
        self.bucket.objects = {self.state_name: state_document()}
        shutil.rmtree(self.terraform_dir)
        self.assertTrue(self.deployment_dir.is_dir())
        instances, delete_subscription = self.destroy()
        self.cluster.refresh_from_db()
        self.assertEqual(
            (self.cluster.status, self.cluster.cloud_state), ("d", "xm")
        )
        self.assertEqual(self.terraform_runs(), 0)
        # A deployed cluster: its compute nodes are swept
        self.assert_swept(instances)
        delete_subscription.assert_called_once_with(
            self.cluster.id, blockers.CONTROLLER_ACCOUNT
        )
        self.assertEqual(
            self.bucket.listed, [(STATE_BUCKET, f"{self.prefix}/")]
        )

    def test_missing_primary_folder_of_a_deployed_cluster_without_state_is_refused(self):
        shutil.rmtree(self.terraform_dir)
        self.assertTrue(self.deployment_dir.is_dir())
        self.assert_no_state_refusal(
            self.refused(), 'its cloud state is "m"'
        )

    def test_file_at_the_primary_path_is_not_a_terraform_folder(self):
        self.bucket.objects = {self.state_name: state_document(resource())}
        shutil.rmtree(self.terraform_dir)
        self.terraform_dir.write_text("not a folder\n", encoding="utf-8")
        refusal = self.refused(missing="is not a folder")
        self.assertIn("still holds 1 resource,", refusal)
        self.assertEqual(
            self.terraform_dir.read_text(encoding="utf-8"), "not a folder\n"
        )

    def test_no_subscription_still_reaches_deleted(self):
        # A create that stopped before gcluster ran made no subscription
        self.without_deployment_folder()
        with self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "INFO"
        ) as logs:
            instances, delete_subscription = (
                self.destroy_with_subscription_error(
                    NotFound("Resource not found (resource=cluster_1).")
                )
            )
        self.assert_deleted_without_terraform(instances, delete_subscription)
        logged = "\n".join(logs.output)
        self.assertIn(
            f"Cluster {self.cluster.id} has no C2 subscription to delete",
            logged,
        )
        self.assertIn(f"Cluster {self.cluster.id} removed", logged)

    def test_failed_subscription_delete_is_logged_and_raised(self):
        # As on the normal path: the cluster is deleted, and the error
        # reaches the destroy task
        self.without_deployment_folder()
        error = RuntimeError("Pub/Sub is unavailable")
        with self.assertRaises(RuntimeError) as caught, self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "ERROR"
        ) as logs:
            self.destroy_with_subscription_error(error)
        self.assertIs(caught.exception, error)
        self.assertIn(
            f"Could not delete the C2 subscription of cluster "
            f"{self.cluster.id}",
            "\n".join(logs.output),
        )
        self.cluster.refresh_from_db()
        self.assertEqual(
            (self.cluster.status, self.cluster.cloud_state), ("d", "xm")
        )
        self.assertEqual(self.terraform_runs(), 0)

    def test_empty_state_reaches_deleted_without_terraform(self):
        # The state `terraform init` leaves, and its lock
        self.bucket.objects = {
            self.state_name: state_document(),
            f"{self.prefix}/default.tflock": b'{"ID": "x"}',
        }
        self.without_deployment_folder()
        self.assert_deleted_without_terraform(*self.destroy())

    def test_state_a_finished_destroy_left_reaches_deleted(self):
        self.bucket.objects = {
            self.state_name: fixture_state("destroyed.state.json")
        }
        self.without_deployment_folder()
        self.assert_deleted_without_terraform(*self.destroy())

    def test_state_with_resources_is_refused(self):
        self.bucket.objects = {
            self.state_name: fixture_state("template_left.state.json")
        }
        self.without_deployment_folder()
        refusal = self.refused()
        self.assertIn("still holds 1 resource,", refusal)
        self.assertIn(
            "Restore the deployment folder and destroy the cluster again, or "
            "have an administrator destroy the resources from that state.",
            refusal,
        )

    def test_rebuilt_server_with_a_running_cluster_is_refused(self):
        # A server rebuilt with a restored database: no cluster directory
        # at all, and the cluster as it was, running
        self.bucket.objects = {
            self.state_name: fixture_state(
                "ctl_roles_and_compute_left.state.json"
            )
        }
        shutil.rmtree(self.info.cluster_dir)
        refusal = self.refused()
        self.assertIn("still holds 9 resources,", refusal)
        self.cluster.refresh_from_db()
        self.assertEqual(
            (self.cluster.status, self.cluster.cloud_state), ("r", "m")
        )
        self.assertIsNotNone(self.cluster.controller_node)

    def test_retry_after_a_failed_attempt_is_still_refused(self):
        # An earlier teardown that failed left "t"/"dm"
        self.bucket.objects = {
            self.state_name: state_document(resource()),
            f"{self.prefix}/other.tfstate": state_document(),
        }
        self.without_deployment_folder(status="t", cloud_state="dm")
        self.assertIn("still holds 1 resource,", self.refused())
        self.cluster.refresh_from_db()
        self.assertEqual(
            (self.cluster.status, self.cluster.cloud_state), ("t", "dm")
        )

    def test_unreadable_bucket_is_refused(self):
        self.bucket.error = Forbidden(
            "provisioner does not have storage.objects.list access to the "
            f"Google Cloud Storage bucket {STATE_BUCKET}"
        )
        self.without_deployment_folder()
        refusal = self.refused()
        self.assertIn(
            f"could not be read as credential key-{PROJECT}: 403 provisioner "
            "does not have storage.objects.list access",
            refusal,
        )
        self.assertIn("Fix the error and destroy the cluster again", refusal)

    def test_state_that_is_not_json_is_refused(self):
        self.bucket.objects = {self.state_name: b"{not json"}
        self.without_deployment_folder()
        self.assertIn(
            f"gs://{STATE_BUCKET}/{self.state_name} is not a Terraform state "
            "document",
            self.refused(),
        )

    def test_the_preflight_still_comes_first(self):
        self.without_deployment_folder()
        with self.bucket.storage_patch(), self.assertRaises(
            credentials.PermissionGapError
        ):
            FakeGoogle(set()).run(self.info.stop_cluster)
        self.assertEqual(self.bucket.listed, [])
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")

    def test_state_is_read_as_an_impersonation_credential(self):
        # A keyless cluster credential: the bucket is read as its account,
        # by impersonation from the server's own identity (here a stand-in
        # for Application Default Credentials)
        from google.auth import impersonated_credentials  # pylint: disable=import-outside-toplevel
        from ghpcfe.tests.test_credentials import ACCOUNT  # pylint: disable=import-outside-toplevel

        # Owned by the cluster's owner: another user would need Role rows
        self.cluster.cloud_credential = Credential.objects.create(
            name="keyless-cred",
            owner=self.cluster.owner,
            kind=credentials.KIND_IMPERSONATE,
            detail=ACCOUNT,
            project_id=PROJECT,
        )
        self.cluster.save()
        self.bucket.objects = {self.state_name: state_document()}
        self.without_deployment_folder(status="r", cloud_state="m")
        with mock.patch(
            "google.auth.default",
            return_value=(mock.Mock(name="server-identity"), None),
        ) as server_identity:
            self.assert_deleted_without_terraform(
                *self.destroy(), swept=True
            )
        (made,) = self.bucket.clients
        self.assertEqual(made["project"], PROJECT)
        creds = made["credentials"]
        self.assertIsInstance(creds, impersonated_credentials.Credentials)
        # pylint: disable=protected-access
        self.assertEqual(creds._target_principal, ACCOUNT)
        # Impersonated from the server's own identity, never from a key
        server_identity.assert_called()

    def test_state_is_read_as_the_clusters_credential(self):
        self.without_deployment_folder()
        made_from = []
        make_client = self.bucket.client

        def client(*args, **kwargs):
            # The key the client's Python credentials were made from
            from google.oauth2 import service_account  # pylint: disable=import-outside-toplevel

            made_from.append(
                service_account.Credentials.from_service_account_info.call_args
            )
            return make_client(*args, **kwargs)

        self.bucket.client = client
        self.destroy()
        (made,) = self.bucket.clients
        self.assertEqual(made["project"], PROJECT)
        self.assertIsNotNone(made["credentials"])
        info = made_from[0].args[0]
        key = json.loads(credentials.key_json(self.cluster.cloud_credential))
        self.assertEqual(info["client_email"], key["client_email"])
        self.assertEqual(info["private_key"], key["private_key"])

    def test_deployment_folder_present_runs_the_normal_destroy(self):
        # Resources in the state do not matter: Terraform destroys them
        self.bucket.objects = {self.state_name: state_document(resource())}
        instances, delete_subscription = self.destroy(blockers.DESTROYED)
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "d")
        self.assertEqual(self.cluster.cloud_state, "xm")
        self.assertEqual(self.terraform_runs(), 1)
        instances.return_value.aggregated_list.assert_called_once()
        delete_subscription.assert_called_once_with(
            self.cluster.id, blockers.CONTROLLER_ACCOUNT
        )
        self.assertEqual(self.bucket.listed, [])


class StateContentsTests(TestCase):
    """utils.state_contents against a fake state bucket"""

    EMAIL = f"cluster-owner@{PROJECT}.iam.gserviceaccount.com"
    PREFIX = "ns/clusters/cluster_1"

    def setUp(self):
        self.credential = Credential.objects.create(
            name="cluster-key",
            owner=make_world()[0],
            detail=make_key_json(email=self.EMAIL),
        )
        self.bucket = FakeStateBucket()
        patcher = mock.patch(
            "google.cloud.storage.Client", side_effect=self.bucket.client
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def contents(self, prefix=PREFIX):
        return utils.state_contents(STATE_BUCKET, prefix, self.credential)

    def test_reads_as_the_credential(self):
        self.assertEqual(self.contents(), (0, 0))
        (made,) = self.bucket.clients
        # The credential's own key, not the server's identity
        self.assertEqual(made["project"], PROJECT)
        self.assertEqual(made["credentials"].service_account_email, self.EMAIL)

    def test_counts_every_state_and_resource_entry(self):
        self.bucket.objects = {
            f"{self.PREFIX}/default.tfstate": state_document(
                resource(instances=3),
                resource(mode="data", type_="google_project"),
            ),
            f"{self.PREFIX}/staging.tfstate": state_document(
                resource(instances=0)
            ),
            f"{self.PREFIX}/default.tflock": b"not a state",
            # Another cluster's state, whose prefix starts with this one's
            f"{self.PREFIX}0/default.tfstate": state_document(resource()),
        }
        self.assertEqual(self.contents(), (2, 3))
        self.assertEqual(self.contents(f"{self.PREFIX}/"), (2, 3))
        self.assertEqual(
            self.bucket.listed, [(STATE_BUCKET, f"{self.PREFIX}/")] * 2
        )

    def test_a_state_nested_below_the_prefix_is_counted(self):
        # Never missed, wherever below the prefix it is
        self.bucket.objects = {
            f"{self.PREFIX}/env/old/default.tfstate": state_document(
                resource(), resource()
            ),
        }
        contents = self.contents()
        self.assertEqual(contents.states, 1)
        self.assertEqual(contents.resources, 2)

    def test_no_state_is_told_from_states_with_no_resources(self):
        self.assertEqual(self.contents(), (0, 0))
        # An empty object is no state, as Terraform reads it, and a lock
        # alone is no state either
        self.bucket.objects = {
            f"{self.PREFIX}/empty.tfstate": b"",
            f"{self.PREFIX}/default.tflock": b'{"ID": "x"}',
        }
        self.assertEqual(self.contents(), (0, 0))
        # The state `terraform init` leaves, or a finished destroy
        self.bucket.objects[f"{self.PREFIX}/default.tfstate"] = (
            state_document()
        )
        self.assertEqual(self.contents(), (1, 0))

    def test_read_and_parse_failures_raise(self):
        self.bucket.error = Forbidden("denied")
        with self.assertRaises(Forbidden):
            self.contents()
        self.bucket.error = None
        name = f"{self.PREFIX}/default.tfstate"
        for data in (
            b"{not json",
            b"[]",
            b"\xff\xfe",
            json.dumps({"version": 3, "modules": []}).encode(),
            json.dumps({"version": 4, "resources": {}}).encode(),
        ):
            self.bucket.objects = {name: data}
            with self.subTest(data=data), self.assertRaisesRegex(
                RuntimeError,
                f"gs://{STATE_BUCKET}/{name} is not a Terraform state",
            ):
                self.contents()


class StartKeepsCloudStateTests(TestCase):
    """ClusterInfo.start_cluster (a Retry) keeps a cloud state that says
    the cluster's resources may exist"""

    setUp = blockers.DestroyBlockerTests.setUp

    def start_refused(self, cloud_state):
        """start_cluster from cloud_state, refused by its permission check
        before gcluster runs (no SSH key or blueprint is made); returns the
        cloud state it leaves"""
        self.cluster.status = "e"
        self.cluster.cloud_state = cloud_state
        self.cluster.save()
        with mock.patch.object(
            ClusterInfo, "_create_ssh_key"
        ), mock.patch.object(ClusterInfo, "update"), self.assertRaises(
            credentials.PermissionGapError
        ):
            FakeGoogle(set()).run(self.info.start_cluster)
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")
        return self.cluster.cloud_state

    def test_a_started_destroy_is_kept(self):
        # A destroy that failed part way leaves "dm": resources may be left
        self.assertEqual(self.start_refused("dm"), "dm")

    def test_a_managed_cluster_is_kept(self):
        self.assertEqual(self.start_refused("m"), "m")

    def test_a_destroyed_or_new_cluster_becomes_new(self):
        for cloud_state in ("xm", "nm"):
            with self.subTest(cloud_state=cloud_state):
                self.assertEqual(self.start_refused(cloud_state), "nm")


class StartedFlagSaveTests(TestCase):
    """Cluster.save() never writes False over a terraform_started the
    database holds as True"""

    def setUp(self):
        self.cluster = PROFILE_CASES["minimal"](make_world())

    def flag_in_database(self):
        return Cluster.objects.get(pk=self.cluster.pk).terraform_started

    def test_a_new_row_saves_false(self):
        self.assertFalse(self.cluster.terraform_started)
        self.assertFalse(self.flag_in_database())
        # And a later save of it keeps False
        self.cluster.status = "c"
        self.cluster.save()
        self.assertFalse(self.flag_in_database())

    def test_a_stale_full_save_keeps_true(self):
        stale = Cluster.objects.get(pk=self.cluster.pk)
        Cluster.objects.filter(pk=self.cluster.pk).update(
            terraform_started=True
        )
        stale.status = "e"
        stale.save()
        self.assertTrue(stale.terraform_started)
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual((row.status, row.terraform_started), ("e", True))

    def test_a_stale_save_naming_the_flag_keeps_true(self):
        stale = Cluster.objects.get(pk=self.cluster.pk)
        Cluster.objects.filter(pk=self.cluster.pk).update(
            terraform_started=True
        )
        stale.save(update_fields=["status", "terraform_started"])
        self.assertTrue(self.flag_in_database())

    def test_a_save_leaving_the_flag_out_reads_nothing(self):
        stale = Cluster.objects.get(pk=self.cluster.pk)
        Cluster.objects.filter(pk=self.cluster.pk).update(
            terraform_started=True
        )
        stale.status = "e"
        # No query reads or writes the flag
        with CaptureQueriesContext(connection) as queries:
            stale.save(update_fields=["status"])
        self.assertEqual(
            [
                query["sql"]
                for query in queries.captured_queries
                if "terraform_started" in query["sql"]
            ],
            [],
        )
        self.assertFalse(stale.terraform_started)
        self.assertTrue(self.flag_in_database())

    def test_update_fields_given_as_an_iterator_are_all_written(self):
        # Django's save() takes any iterable as update_fields; a generator
        # can be read only once, so reading it to find the flag must leave
        # every named field for the save to write
        for names in (["status"], ["status", "terraform_started"]):
            with self.subTest(update_fields=names):
                Cluster.objects.filter(pk=self.cluster.pk).update(
                    status="c", terraform_started=False
                )
                stale = Cluster.objects.get(pk=self.cluster.pk)
                Cluster.objects.filter(pk=self.cluster.pk).update(
                    terraform_started=True
                )
                stale.status = "e"
                stale.save(update_fields=(name for name in names))
                row = Cluster.objects.get(pk=self.cluster.pk)
                self.assertEqual(
                    (row.status, row.terraform_started), ("e", True)
                )

    def set_flag_just_before_the_update(self):
        """Another process sets the flag after a save of this cluster has
        begun: from the pre_save signal, which Django sends just before it
        writes the row"""

        def set_flag(sender, instance, **kwargs):
            Cluster.objects.filter(pk=instance.pk).update(
                terraform_started=True
            )

        pre_save.connect(set_flag, sender=Cluster, weak=False)
        self.addCleanup(pre_save.disconnect, set_flag, sender=Cluster)

    def test_a_flag_set_during_a_stale_full_save_is_kept(self):
        # A save that read the flag before writing would have read False
        # and then written it over the True
        stale = Cluster.objects.get(pk=self.cluster.pk)
        self.set_flag_just_before_the_update()
        stale.status = "e"
        stale.save()
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual((row.status, row.terraform_started), ("e", True))
        # The copy reads the column back after the save
        self.assertTrue(stale.terraform_started)

    def test_a_flag_set_during_a_stale_save_naming_it_is_kept(self):
        stale = Cluster.objects.get(pk=self.cluster.pk)
        self.set_flag_just_before_the_update()
        stale.status = "e"
        stale.save(update_fields=["status", "terraform_started"])
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual((row.status, row.terraform_started), ("e", True))

    def test_a_stale_save_of_a_deferred_copy_keeps_true(self):
        # Only the loaded fields are written, as Django saves a deferred
        # copy; the deferred name is not read or written
        stale = Cluster.objects.defer("name").get(pk=self.cluster.pk)
        Cluster.objects.filter(pk=self.cluster.pk).update(
            name="renamed", terraform_started=True
        )
        stale.status = "e"
        with CaptureQueriesContext(connection) as queries:
            stale.save()
        sql = [query["sql"] for query in queries.captured_queries]
        # Both columns are in the cluster's own table (its parent table,
        # ghpcfe_cloudresource, is updated separately)
        updates = [
            text for text in sql if text.startswith('UPDATE "ghpcfe_cluster" ')
        ]
        self.assertEqual(len(updates), 1, sql)
        self.assertIn('"status" = ', updates[0])
        # The UPDATE leaves out the deferred column and the flag
        self.assertNotIn('"name"', updates[0])
        self.assertNotIn('"terraform_started"', updates[0])
        # No query reads the deferred column
        self.assertEqual(
            [text for text in sql if '"ghpcfe_cluster"."name"' in text], []
        )
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual(
            (row.name, row.status, row.terraform_started),
            ("renamed", "e", True),
        )

    def test_a_save_holding_true_writes_true(self):
        # Not narrowed: a copy that holds True writes it with the rest
        copy = Cluster.objects.get(pk=self.cluster.pk)
        copy.terraform_started = True
        copy.status = "e"
        copy.save()
        row = Cluster.objects.get(pk=self.cluster.pk)
        self.assertEqual((row.status, row.terraform_started), ("e", True))
