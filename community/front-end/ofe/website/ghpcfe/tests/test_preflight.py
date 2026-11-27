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

"""Permission checks before provisioning, and 403s mapped to the same message.

Offline: Google client libraries are replaced at their own module
boundaries, and Terraform by a stand-in script on PATH.
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

from django.contrib.messages import get_messages
from django.test import SimpleTestCase, TestCase
from django.urls import reverse
from google.api_core.exceptions import Forbidden, NotFound

from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager import filesystem as cm_filesystem
from ghpcfe.cluster_manager import utils
from ghpcfe.cluster_manager import vpc as cm_vpc
from ghpcfe.cluster_manager.clusterinfo import ClusterInfo
from ghpcfe.cluster_manager.image import ImageBackend
from ghpcfe.cluster_manager.workbenchinfo import WorkbenchInfo
from ghpcfe.models import (
    Credential,
    Filesystem,
    GCPFilestoreFilesystem,
    Image,
    Role,
    User,
    VirtualNetwork,
    Workbench,
)
from ghpcfe.tests.test_blueprint_identities import (
    PROFILE_CASES,
    make_world,
    scenario_minimal,
)
from ghpcfe.tests.test_credentials import (
    ACCOUNT,
    KEY_JSON,
    PROJECT,
    impersonate_credential,
    key_credential,
)

SERVER_PROJECT = "ofe-test-project"
SERVER_BUCKET = "ofe-test-bucket"
SERVER_TOPIC = "ofe-test-c2"


def granting(granted):
    """A tester that grants the permissions in granted"""
    return lambda creds, check: set(check.permissions) & set(granted)


class PreflightTests(TestCase):
    """credentials.preflight names identity, permission and operation"""

    def setUp(self):
        self.credential = impersonate_credential()

    def test_passes_when_everything_is_granted(self):
        credentials.preflight(
            self.credential,
            "create cluster alpha",
            ("compute.instances.create",),
            testers={"project": granting(["compute.instances.create"])},
            creds=object(),
        )

    def test_gap_names_identity_permission_operation_and_project(self):
        with self.assertRaises(credentials.PermissionGapError) as caught:
            credentials.preflight(
                self.credential,
                "create cluster alpha",
                ("compute.instances.create", "iam.roles.create"),
                testers={"project": granting(["compute.instances.create"])},
                creds=object(),
            )
        self.assertEqual(
            str(caught.exception),
            f"Identity {ACCOUNT} lacks permission iam.roles.create needed to "
            f"create cluster alpha in project {PROJECT}.",
        )
        self.assertEqual(caught.exception.permissions, ("iam.roles.create",))

    def test_gap_on_a_resource_names_the_resource(self):
        bucket = credentials.ResourceCheck(
            "bucket", SERVER_BUCKET, credentials.BUCKET_IAM_PERMISSIONS
        )
        with self.assertRaises(credentials.PermissionGapError) as caught:
            credentials.preflight(
                self.credential,
                "create cluster alpha",
                (),
                resources=[bucket],
                testers={"bucket": granting(["storage.buckets.getIamPolicy"])},
                creds=object(),
            )
        message = str(caught.exception)
        self.assertIn(f"Identity {ACCOUNT} lacks permission", message)
        self.assertIn("storage.buckets.setIamPolicy", message)
        self.assertIn("needed to create cluster alpha", message)
        self.assertIn(f"on resource gs://{SERVER_BUCKET}", message)

    def test_a_check_that_cannot_run_is_an_error_not_a_pass(self):
        def broken(creds, check):
            raise RuntimeError("cannot impersonate")

        with self.assertRaisesRegex(
            credentials.CredentialError,
            f"Could not check the permissions of identity {ACCOUNT} needed to "
            "create cluster alpha",
        ):
            credentials.preflight(
                self.credential,
                "create cluster alpha",
                ("compute.instances.create",),
                testers={"project": broken},
                creds=object(),
            )

    def test_profiles_operations_add_account_permissions(self):
        legacy = credentials.permissions_for("cluster_create")
        profiles = credentials.permissions_for("cluster_create", profiles=True)
        self.assertNotIn("iam.roles.create", legacy)
        for permission in (
            "iam.serviceAccounts.create",
            "iam.roles.create",
            "resourcemanager.projects.setIamPolicy",
        ):
            self.assertIn(permission, profiles)


class FakeGoogle:
    """Discovery, Storage and Pub/Sub clients that grant `granted`"""

    def __init__(self, granted, subscription_exists=True):
        self.granted = set(granted)
        self.subscription_exists = subscription_exists
        self.asked = []

    def _grant(self, resource, permissions):
        self.asked.append((resource, tuple(permissions)))
        return [p for p in permissions if p in self.granted]

    def discovery(self, *args, **kwargs):
        service = mock.Mock(name="cloudresourcemanager")

        def test_iam(resource, body):
            request = mock.Mock()
            request.execute.return_value = {
                "permissions": self._grant(
                    f"projects/{resource}", body["permissions"]
                )
            }
            return request

        service.projects.return_value.testIamPermissions.side_effect = test_iam
        return service

    def storage(self, *args, **kwargs):
        client = mock.Mock(name="storage")

        def bucket(name):
            handle = mock.Mock()
            handle.test_iam_permissions.side_effect = (
                lambda permissions: self._grant(f"gs://{name}", permissions)
            )
            return handle

        client.bucket.side_effect = bucket
        return client

    def pubsub(self, kind):
        def make(*args, **kwargs):
            client = mock.Mock(name=kind)

            def test_iam(request):
                if kind == "subscriber" and not self.subscription_exists:
                    raise NotFound("no subscription")
                return mock.Mock(
                    permissions=self._grant(
                        request["resource"], request["permissions"]
                    )
                )

            client.test_iam_permissions.side_effect = test_iam
            return client

        return make

    def patches(self):
        return [
            mock.patch(
                "google.oauth2.service_account.Credentials.from_service_account_info"
            ),
            mock.patch("googleapiclient.discovery.build", side_effect=self.discovery),
            mock.patch("google.cloud.storage.Client", side_effect=self.storage),
            mock.patch(
                "google.cloud.pubsub_v1.PublisherClient",
                side_effect=self.pubsub("publisher"),
            ),
            mock.patch(
                "google.cloud.pubsub_v1.SubscriberClient",
                side_effect=self.pubsub("subscriber"),
            ),
        ]

    def run(self, func):
        patches = self.patches()
        for patch in patches:
            patch.start()
        try:
            return func()
        finally:
            for patch in reversed(patches):
                patch.stop()


ALL_CLUSTER_PERMISSIONS = set(
    credentials.permissions_for("cluster_create", profiles=True)
    + credentials.permissions_for("cluster_destroy", profiles=True)
    + credentials.BUCKET_IAM_PERMISSIONS
    + credentials.TOPIC_IAM_PERMISSIONS
    + credentials.SUBSCRIPTION_IAM_PERMISSIONS
)


class ClusterPreflightTests(TestCase):
    """Clusters are checked before gcluster or Terraform runs"""

    def setUp(self):
        # The credential is for PROJECT; the server runs in SERVER_PROJECT
        self.world = make_world()

    def preflight(self, cluster, google, verb="create"):
        info = ClusterInfo(cluster)
        return google.run(lambda: info._preflight(verb))  # pylint: disable=protected-access

    def test_cross_project_profiles_cluster_checks_server_resources(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        google = FakeGoogle(ALL_CLUSTER_PERMISSIONS)
        self.preflight(cluster, google)
        resources = [resource for resource, _ in google.asked]
        subscription = (
            f"projects/{SERVER_PROJECT}/subscriptions/{SERVER_TOPIC}-cluster_{cluster.id}"
        )
        self.assertEqual(
            resources,
            [
                f"projects/{PROJECT}",
                f"gs://{SERVER_BUCKET}",
                f"projects/{SERVER_PROJECT}/topics/{SERVER_TOPIC}",
                subscription,
            ],
        )
        self.assertIn("iam.roles.create", google.asked[0][1])

    def test_missing_topic_permission_names_the_topic(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        google = FakeGoogle(
            ALL_CLUSTER_PERMISSIONS - {"pubsub.topics.setIamPolicy"}
        )
        with self.assertRaises(credentials.PermissionGapError) as caught:
            self.preflight(cluster, google)
        message = str(caught.exception)
        self.assertIn("lacks permission pubsub.topics.setIamPolicy", message)
        self.assertIn(
            f"on resource projects/{SERVER_PROJECT}/topics/{SERVER_TOPIC}", message
        )
        self.assertIn(f"create cluster {cluster.name}", message)

    def test_new_subscription_is_checked_through_the_server_project(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        google = FakeGoogle(ALL_CLUSTER_PERMISSIONS, subscription_exists=False)
        self.preflight(cluster, google)
        self.assertEqual(
            google.asked[-1],
            (
                f"projects/{SERVER_PROJECT}",
                credentials.SUBSCRIPTION_IAM_PERMISSIONS,
            ),
        )

    def test_legacy_cluster_checks_only_its_project(self):
        cluster = scenario_minimal(self.world, "legacy")
        google = FakeGoogle(ALL_CLUSTER_PERMISSIONS)
        self.preflight(cluster, google)
        self.assertEqual(len(google.asked), 1)
        self.assertNotIn("iam.roles.create", google.asked[0][1])

    def test_destroy_checks_delete_permissions_only_in_the_project(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        google = FakeGoogle(ALL_CLUSTER_PERMISSIONS)
        self.preflight(cluster, google, verb="destroy")
        self.assertEqual(len(google.asked), 1)
        self.assertIn("iam.roles.delete", google.asked[0][1])
        self.assertIn("compute.instances.delete", google.asked[0][1])

    def test_reconfigure_stops_before_gcluster_on_a_gap(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        info = ClusterInfo(cluster)
        google = FakeGoogle(set())
        # Only the check: no blueprint or bucket upload
        with mock.patch("subprocess.run") as run, mock.patch.object(
            ClusterInfo, "update"
        ):
            with self.assertRaises(credentials.PermissionGapError):
                google.run(info.reconfigure_cluster)
        run.assert_not_called()
        cluster.refresh_from_db()
        self.assertEqual(cluster.status, "e")


class TerraformOutputTests(SimpleTestCase):
    """A 403 in tool output becomes the same message"""

    credential = Credential(
        name="keyless",
        kind=credentials.KIND_IMPERSONATE,
        detail=ACCOUNT,
        project_id=PROJECT,
    )

    def gap(self, text):
        return credentials.permission_error_from_output(
            text, self.credential, "create network net-a"
        )

    def test_required_permission_message(self):
        gap = self.gap(
            "Error: Error creating Network: googleapi: Error 403: Required "
            "'compute.networks.create' permission for "
            "'projects/example-project/global/networks/net-a', forbidden"
        )
        self.assertEqual(gap.permissions, ("compute.networks.create",))
        self.assertEqual(
            str(gap),
            f"Identity {ACCOUNT} lacks permission compute.networks.create "
            "needed to create network net-a on resource "
            "projects/example-project/global/networks/net-a.",
        )

    def test_permission_denied_message(self):
        gap = self.gap(
            "Error 403: Permission 'iam.serviceAccounts.create' denied on "
            "resource (or it may not exist)., forbidden\n"
            "PERMISSION_DENIED: Permission iam.serviceAccounts.actAs is "
            "required to perform this operation"
        )
        self.assertEqual(
            gap.permissions,
            ("iam.serviceAccounts.create", "iam.serviceAccounts.actAs"),
        )
        self.assertIn(f"Identity {ACCOUNT} lacks permissions", str(gap))

    def test_unnamed_permission_keeps_googles_words(self):
        gap = self.gap("googleapi: Error 403: The caller does not have permission")
        self.assertEqual(gap.permissions, ())
        self.assertIn(f"Identity {ACCOUNT} lacks a permission", str(gap))
        self.assertIn("The caller does not have permission", str(gap))

    def test_other_failures_are_not_mapped(self):
        self.assertIsNone(self.gap("Error: Quota 'CPUS' exceeded. Limit: 24.0"))
        self.assertIsNone(self.gap(""))


FAKE_TERRAFORM = textwrap.dedent(
    """\
    #!{python}
    import json, os, stat, sys
    record = {{}}
    for name in ("GOOGLE_CREDENTIALS", "GOOGLE_APPLICATION_CREDENTIALS",
                 "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
                 "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
                 "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT", "CLOUDSDK_CONFIG"):
        record[name] = os.environ.get(name)
    path = os.environ.get("GOOGLE_APPLICATION_CREDENTIALS")
    if path:
        record["file_mode"] = stat.S_IMODE(os.stat(path).st_mode)
        record["dir_mode"] = stat.S_IMODE(os.stat(os.path.dirname(path)).st_mode)
        with open(path, encoding="utf-8") as fp:
            record["file_text"] = fp.read()
    with open(os.environ["FAKE_TF_RECORD"], "w", encoding="utf-8") as fp:
        json.dump(record, fp)
    sys.stderr.write(os.environ.get("FAKE_TF_STDERR", ""))
    sys.exit(int(os.environ.get("FAKE_TF_RC", "0")))
    """
)


class RunTerraformTests(TestCase):
    """utils.run_terraform with a stand-in terraform"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-fake-tf-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        bindir = self.tmp / "bin"
        bindir.mkdir()
        fake = bindir / "terraform"
        fake.write_text(FAKE_TERRAFORM.format(python=sys.executable))
        fake.chmod(stat.S_IRWXU)
        self.workdir = self.tmp / "work"
        self.workdir.mkdir()
        self.record = self.tmp / "record.json"
        # The test's own PATH; the code under test only copies os.environ
        patcher = mock.patch.dict(
            os.environ,
            {"PATH": f"{bindir}{os.pathsep}{os.environ.get('PATH', '')}"},
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_terraform(self, credential, command="apply", rc=0, stderr=""):
        utils.run_terraform(
            self.workdir,
            command,
            extra_env={
                "FAKE_TF_RECORD": str(self.record),
                "FAKE_TF_RC": str(rc),
                "FAKE_TF_STDERR": stderr,
            },
            credential=credential,
            operation="create network net-a",
        )

    def recorded(self):
        return json.loads(self.record.read_text(encoding="utf-8"))

    def test_key_apply_gets_a_private_file_removed_afterwards(self):
        before = dict(os.environ)
        self.run_terraform(key_credential())
        record = self.recorded()
        self.assertEqual(record["GOOGLE_CREDENTIALS"], KEY_JSON)
        self.assertEqual(record["file_text"], KEY_JSON)
        self.assertEqual(record["file_mode"], 0o600)
        self.assertEqual(record["dir_mode"], 0o700)
        self.assertEqual(
            record["CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"],
            record["GOOGLE_APPLICATION_CREDENTIALS"],
        )
        self.assertFalse(Path(record["GOOGLE_APPLICATION_CREDENTIALS"]).exists())
        self.assertEqual(dict(os.environ), before)
        self.assertEqual(sorted(os.listdir(self.workdir)), [
            "terraform_apply_log.stderr", "terraform_apply_log.stdout"
        ])

    def test_key_plan_gets_no_file(self):
        self.run_terraform(key_credential(), command="plan")
        record = self.recorded()
        self.assertEqual(record["GOOGLE_CREDENTIALS"], KEY_JSON)
        self.assertIsNone(record["GOOGLE_APPLICATION_CREDENTIALS"])

    def test_impersonate_apply_gets_impersonation_only(self):
        self.run_terraform(impersonate_credential())
        record = self.recorded()
        self.assertEqual(record["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertEqual(record["CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertIsNone(record["GOOGLE_CREDENTIALS"])
        self.assertIsNone(record["GOOGLE_APPLICATION_CREDENTIALS"])
        self.assertFalse(Path(record["CLOUDSDK_CONFIG"]).exists())

    def test_failed_key_run_still_removes_the_file(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_terraform(key_credential(), rc=1, stderr="Error: boom\n")
        path = Path(self.recorded()["GOOGLE_APPLICATION_CREDENTIALS"])
        self.assertFalse(path.exists())
        self.assertFalse(path.parent.exists())

    def test_403_is_raised_as_the_permission_message(self):
        with self.assertRaises(credentials.PermissionDeniedRunError) as caught:
            self.run_terraform(
                impersonate_credential(),
                rc=1,
                stderr=(
                    "Error: googleapi: Error 403: Required "
                    "'compute.networks.create' permission for "
                    "'projects/example-project/global/networks/net-a'\n"
                ),
            )
        error = caught.exception
        self.assertIsInstance(error, subprocess.CalledProcessError)
        self.assertIn(
            f"Identity {ACCOUNT} lacks permission compute.networks.create "
            "needed to create network net-a",
            str(error),
        )
        log = (self.workdir / "terraform_apply_log.stderr").read_text()
        self.assertTrue(log.rstrip().endswith(f"OFE: {error.gap}"))


# Records, in FAKE_RECORD, the arguments and the Google Cloud and gcloud
# variables of its environment, and the key file named by
# GOOGLE_APPLICATION_CREDENTIALS; then exits FAKE_RC
RECORD_ENVIRONMENT = textwrap.dedent(
    """\
    #!{python}
    import json, os, stat, sys
    record = {{
        "argv": sys.argv[1:],
        "env": {{
            name: value for name, value in os.environ.items()
            if name.startswith(("GOOGLE_", "GCLOUD_", "CLOUDSDK_", "FAKE_"))
        }},
    }}
    path = os.environ.get("GOOGLE_APPLICATION_CREDENTIALS")
    if path and os.path.isfile(path):
        with open(path, encoding="utf-8") as fp:
            record["key_file_text"] = fp.read()
        record["key_file_mode"] = stat.S_IMODE(os.stat(path).st_mode)
    with open(os.environ["FAKE_RECORD"], "w", encoding="utf-8") as fp:
        json.dump(record, fp)
    sys.exit(int(os.environ.get("FAKE_RC", "0")))
    """
)


# The server's ambient credential variables, none of which may reach a tool
# run given no credential. Pinned here, so a name dropped from
# credentials.AMBIENT_CREDENTIAL_VARIABLES fails a test
# (CredentiallessRunTests.test_the_ambient_variables_are_pinned).
AMBIENT_CREDENTIAL_VARIABLES = (
    "GOOGLE_APPLICATION_CREDENTIALS",
    "GOOGLE_CREDENTIALS",
    "GOOGLE_BACKEND_CREDENTIALS",
    "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
    "GOOGLE_CLOUD_KEYFILE_JSON",
    "GCLOUD_KEYFILE_JSON",
    "GOOGLE_OAUTH_ACCESS_TOKEN",
    "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CONFIG",
    "CLOUDSDK_AUTH_ACCESS_TOKEN_FILE",
    "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
    "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CORE_ACCOUNT",
    "CLOUDSDK_CORE_PROJECT",
)


def ambient_credentials():
    """A value for each ambient credential variable, as the server's own
    environment could hold"""
    return {
        name: f"ambient-{name.lower()}" for name in AMBIENT_CREDENTIAL_VARIABLES
    }


class CredentiallessRunTests(SimpleTestCase):
    """utils.run_terraform with no credential gets no cloud identity: none
    of the server's ambient credential variables"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-fake-tf-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        bindir = self.tmp / "bin"
        bindir.mkdir()
        fake = bindir / "terraform"
        fake.write_text(RECORD_ENVIRONMENT.format(python=sys.executable))
        fake.chmod(stat.S_IRWXU)
        self.workdir = self.tmp / "work"
        self.workdir.mkdir()
        self.record = self.tmp / "record.json"
        patcher = mock.patch.dict(os.environ, {
            "PATH": f"{bindir}{os.pathsep}{os.environ.get('PATH', '')}",
            **ambient_credentials(),
        })
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_the_ambient_variables_are_pinned(self):
        self.assertEqual(len(AMBIENT_CREDENTIAL_VARIABLES), 14)
        self.assertEqual(
            len(credentials.AMBIENT_CREDENTIAL_VARIABLES),
            len(set(credentials.AMBIENT_CREDENTIAL_VARIABLES)),
        )
        self.assertEqual(
            set(credentials.AMBIENT_CREDENTIAL_VARIABLES),
            set(AMBIENT_CREDENTIAL_VARIABLES),
        )

    def test_no_ambient_credential_reaches_terraform(self):
        before = dict(os.environ)
        utils.run_terraform(
            self.workdir,
            "init",
            extra_env={"FAKE_RECORD": str(self.record), "FAKE_EXTRA": "kept"},
        )
        record = json.loads(self.record.read_text(encoding="utf-8"))
        self.assertEqual(record["argv"][0], "init")
        for name in AMBIENT_CREDENTIAL_VARIABLES:
            # Not assertNotIn: its failure message would print the whole
            # environment into the test log
            self.assertFalse(name in record["env"], f"{name} reached Terraform")
        self.assertEqual(record["env"]["FAKE_EXTRA"], "kept")
        # The server's own environment is unchanged (compared without
        # printing it)
        self.assertTrue(dict(os.environ) == before, "os.environ changed")


class RefusedPreflightTests(TestCase):
    """Images, networks, workbenches and filesystems stop before any tool
    runs when the identity lacks a permission, and say why"""

    def setUp(self):
        self.owner, _, self.subnet = make_world()
        # A key for ACCOUNT, whose identity has no permission at all. Owned
        # by the world's owner: a second user would need Role rows.
        self.credential = Credential.objects.create(
            name="no-permissions", owner=self.owner, detail=KEY_JSON
        )
        self.google = FakeGoogle(set())
        patcher = mock.patch("subprocess.run")
        self.subprocess_run = patcher.start()
        self.addCleanup(patcher.stop)

    def refused(self, func):
        """Run func; return the PermissionGapError it raised"""
        with self.assertRaises(credentials.PermissionGapError) as caught:
            self.google.run(func)
        self.subprocess_run.assert_not_called()
        return str(caught.exception)

    def assert_names(self, message, permission, operation):
        self.assertIn(f"Identity {ACCOUNT} lacks permission", message)
        self.assertIn(permission, message)
        self.assertIn(f"needed to {operation}", message)

    # Images

    def make_image(self):
        return Image.objects.create(
            name="img-a",
            family="img-a-family",
            owner=self.owner,
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )

    def test_image_build(self):
        image = self.make_image()
        backend = ImageBackend(image)
        with self.assertLogs("ghpcfe.cluster_manager.image", level="ERROR"):
            message = self.refused(backend.prepare)
        self.assert_names(message, "compute.images.create", "build image img-a")
        self.assertEqual(Image.objects.get(pk=image.pk).status, "e")
        self.assertFalse(backend.image_dir.exists())

    def test_image_delete(self):
        backend = ImageBackend(self.make_image())
        with self.assertLogs("ghpcfe.cluster_manager.image", level="ERROR") as logs:
            # delete_image logs the error and raises it, so the caller keeps
            # the image
            message = self.refused(backend.delete_image)
        self.assert_names(message, "compute.images.delete", "delete image img-a")
        self.assertIn(message, "\n".join(logs.output))

    # Networks

    def make_vpc(self, cloud_state):
        return VirtualNetwork.objects.create(
            name="net-b",
            cloud_id="net-b",
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_state=cloud_state,
        )

    def test_vpc_create_writes_nothing(self):
        vpc = self.make_vpc("nm")
        message = self.refused(lambda: cm_vpc.create_vpc(vpc))
        self.assert_names(message, "compute.networks.create", "create network net-b")
        self.assertFalse(cm_vpc._tf_dir_for_vpc(vpc.id).exists())  # pylint: disable=protected-access

    def test_vpc_start_goes_back_to_new(self):
        vpc = self.make_vpc("cm")
        message = self.refused(lambda: cm_vpc.start_vpc(vpc))
        self.assert_names(message, "compute.firewalls.create", "create network net-b")
        self.assertEqual(VirtualNetwork.objects.get(pk=vpc.pk).cloud_state, "nm")

    def test_vpc_destroy_keeps_the_network(self):
        vpc = self.make_vpc("dm")
        message = self.refused(lambda: cm_vpc.destroy_vpc(vpc))
        self.assert_names(message, "compute.networks.delete", "destroy network net-b")
        self.assertEqual(VirtualNetwork.objects.get(pk=vpc.pk).cloud_state, "m")

    def test_imported_vpc_destroy_stays_imported(self):
        vpc = self.make_vpc("dm")
        # create_vpc leaves no main.tf for a network it imported; OFE only
        # removes that network's subnets
        target_dir = cm_vpc._tf_dir_for_vpc(vpc.id)  # pylint: disable=protected-access
        target_dir.mkdir(parents=True)
        self.addCleanup(shutil.rmtree, target_dir, ignore_errors=True)
        message = self.refused(lambda: cm_vpc.destroy_vpc(vpc))
        self.assert_names(message, "compute.subnetworks.delete", "destroy network net-b")
        self.assertEqual(VirtualNetwork.objects.get(pk=vpc.pk).cloud_state, "i")

    # Workbenches

    def make_workbench(self):
        return Workbench.objects.create(
            name="wb-a",
            owner=self.owner,
            trusted_user=self.owner,
            subnet=self.subnet,
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )

    def workbench_refused(self, method, action, status, expected_status):
        workbench = self.make_workbench()
        workbench.status = status
        workbench.save()
        info = WorkbenchInfo(workbench)
        with self.assertLogs(
            "ghpcfe.cluster_manager.workbenchinfo", level="ERROR"
        ) as logs:
            # The background task logs the reason
            self.google.run(getattr(info, method))
        self.subprocess_run.assert_not_called()
        self.assertEqual(
            Workbench.objects.get(pk=workbench.pk).status, expected_status
        )
        log = "\n".join(logs.output)
        self.assertIn(f"Cannot {action} workbench", log)
        return log

    def test_workbench_start(self):
        # A refused create marks the workbench failed
        log = self.workbench_refused("start", "deploy", "n", "e")
        self.assert_names(log, "notebooks.instances.create", "create workbench wb-a")

    def test_workbench_terminate(self):
        # A refused destroy leaves the running workbench as it was, as for
        # clusters
        log = self.workbench_refused("terminate", "destroy", "r", "r")
        self.assert_names(log, "notebooks.instances.delete", "destroy workbench wb-a")

    # Filesystems

    def make_filesystem(self, cloud_state):
        fs = GCPFilestoreFilesystem.objects.create(
            name="fs-a",
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            vpc=self.subnet.vpc,
            capacity=1024,
            cloud_state=cloud_state,
        )
        return Filesystem.objects.get(pk=fs.pk)

    def test_filesystem_start_goes_back_to_new(self):
        fs = self.make_filesystem("nm")
        message = self.refused(lambda: cm_filesystem.start_filesystem(fs))
        self.assert_names(message, "file.instances.create", "create filesystem fs-a")
        self.assertEqual(Filesystem.objects.get(pk=fs.pk).cloud_state, "nm")

    def test_filesystem_destroy_keeps_the_filesystem(self):
        fs = self.make_filesystem("dm")
        target_dir = cm_filesystem.get_terraform_dir(fs)
        target_dir.mkdir(parents=True)
        self.addCleanup(shutil.rmtree, target_dir.parent.parent, ignore_errors=True)
        message = self.refused(lambda: cm_filesystem.destroy_filesystem(fs))
        self.assert_names(message, "file.instances.delete", "destroy filesystem fs-a")
        self.assertEqual(Filesystem.objects.get(pk=fs.pk).cloud_state, "m")

    def test_imported_filesystem_destroy_stays_imported(self):
        fs = self.make_filesystem("dm")
        self.refused(lambda: cm_filesystem.destroy_filesystem(fs))
        self.assertEqual(Filesystem.objects.get(pk=fs.pk).cloud_state, "i")


class ImageDeleteViewTests(TestCase):
    """Deleting an image in the image list keeps its row, and says why,
    when the delete is refused or fails; Google Cloud and Terraform are
    replaced at their own boundaries"""

    RUN_TERRAFORM = "ghpcfe.cluster_manager.utils.run_terraform"
    IMAGES_CLIENT = "google.cloud.compute_v1.ImagesClient"

    def setUp(self):
        # Only a superuser may delete an image. A new user other than the
        # database's first gets the normal-user role, and user ids need not
        # start at 1 (an earlier test in the run can have used some), so the
        # role's row must exist
        Role.objects.get_or_create(id=Role.NORMALUSER)
        admin = User.objects.create_superuser(
            username="image-admin", password="admin-pass", email="a@example.com"
        )
        self.credential = Credential.objects.create(
            name="image-key", owner=admin, detail=KEY_JSON
        )
        self.image = Image.objects.create(
            name="img-a",
            family="img-a-family",
            owner=admin,
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            status="r",
            builder_started=True,
        )
        self.client.force_login(admin)
        self.images = mock.MagicMock(name="ImagesClient")
        patcher = mock.patch(self.IMAGES_CLIENT, return_value=self.images)
        patcher.start()
        self.addCleanup(patcher.stop)
        # The image as a finished build leaves it: its builder environment
        # folder, in a base directory of this test's own (image ids restart
        # in each test's database)
        base_dir = Path(tempfile.mkdtemp(prefix="ofe-image-delete-"))
        self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)
        patcher = mock.patch.dict(utils.g_config, {"baseDir": base_dir})
        patcher.start()
        self.addCleanup(patcher.stop)
        self.builder_dir = (
            base_dir / "images" / f"image_{self.image.id}"
            / f"image_{self.image.id}" / "builder-env"
        )
        self.builder_dir.mkdir(parents=True)

    def delete(self, granted, xhr=True):
        """POST the delete as the image list's script does (xhr), or as
        a plain form; returns (response, its JSON, the messages it queued
        for the next page)"""
        headers = {"HTTP_X_REQUESTED_WITH": "XMLHttpRequest"} if xhr else {}
        with self.assertLogs("ghpcfe.cluster_manager.image", "INFO") as logs:
            response = FakeGoogle(granted).run(
                lambda: self.client.post(
                    reverse("image-delete", args=[self.image.pk]),
                    **headers,
                )
            )
        shown = [str(m) for m in get_messages(response.wsgi_request)]
        self.logged = "\n".join(logs.output)
        return response, response.json(), shown

    def all_granted(self):
        return set(credentials.permissions_for("image_destroy", profiles=True))

    def assert_kept(self, response, body, shown, *expected):
        self.assertEqual(response.status_code, 200)
        self.assertFalse(body["success"])
        self.assertTrue(Image.objects.filter(pk=self.image.pk).exists())
        # The list's script shows the error in an alert; nothing is queued
        # to show it again on the next page
        self.assertEqual(shown, [])
        self.assertTrue(body["error"].startswith("Image img-a was not deleted: "))
        for text in expected:
            self.assertIn(text, body["error"])

    def test_refused_delete_without_xhr_queues_the_reason(self):
        # Any other caller gets no alert, so the next page shows it
        with mock.patch(self.RUN_TERRAFORM) as run:
            response, body, shown = self.delete(set(), xhr=False)
        self.assertFalse(body["success"])
        self.assertTrue(Image.objects.filter(pk=self.image.pk).exists())
        self.assertEqual(shown, [body["error"]])
        self.assertIn(f"Identity {ACCOUNT} lacks permission", body["error"])
        run.assert_not_called()

    def test_the_list_script_sends_the_xhr_header(self):
        # The view decides by this header, so the list page must send it
        template = (
            Path(__file__).resolve().parents[1] / "templates" / "image" / "list.html"
        ).read_text(encoding="utf-8")
        self.assertIn(
            "xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');",
            template,
        )
        self.assertIn("window.alert(response.error);", template)

    def test_refused_delete_keeps_the_image_and_says_why(self):
        with mock.patch(self.RUN_TERRAFORM) as run:
            response, body, shown = self.delete(set())
        self.assert_kept(
            response,
            body,
            shown,
            f"Identity {ACCOUNT} lacks permission",
            "compute.images.delete",
            "delete image img-a",
        )
        run.assert_not_called()
        self.images.delete.assert_not_called()

    def test_failed_builder_destroy_keeps_the_image_and_says_why(self):
        failure = subprocess.CalledProcessError(1, ["terraform", "destroy"])
        with mock.patch(self.RUN_TERRAFORM, side_effect=failure):
            response, body, shown = self.delete(self.all_granted())
        self.assert_kept(response, body, shown, "returned non-zero exit status 1")
        self.images.delete.assert_not_called()

    def test_failed_builder_destroy_keeps_the_image_status(self):
        # A failed delete is not a failed build: the image stays usable
        failure = subprocess.CalledProcessError(1, ["terraform", "destroy"])
        with mock.patch(self.RUN_TERRAFORM, side_effect=failure):
            response, body, shown = self.delete(self.all_granted())
        self.assert_kept(response, body, shown)
        self.image.refresh_from_db()
        self.assertEqual(self.image.status, "r")

    def test_failed_image_delete_keeps_the_image_and_says_why(self):
        self.images.delete.return_value.result.side_effect = RuntimeError(
            "operation failed"
        )
        with mock.patch(self.RUN_TERRAFORM):
            response, body, shown = self.delete(self.all_granted())
        self.assert_kept(response, body, shown, "operation failed")

    def test_successful_delete_removes_the_image(self):
        with mock.patch(self.RUN_TERRAFORM) as run:
            response, body, shown = self.delete(self.all_granted())
        self.assertEqual(response.status_code, 200)
        self.assertEqual(body, {"success": True})
        self.assertEqual(shown, [])
        self.assertFalse(Image.objects.filter(pk=self.image.pk).exists())
        self.assertEqual(run.call_args.args[1], "destroy")
        self.images.delete.assert_called_once_with(
            project=PROJECT, image="image-img-a"
        )

    def test_image_google_cloud_no_longer_has_counts_as_deleted(self):
        self.images.delete.side_effect = NotFound("no such image")
        with mock.patch(self.RUN_TERRAFORM):
            response, body, _ = self.delete(self.all_granted())
        self.assertEqual(body, {"success": True})
        self.assertFalse(Image.objects.filter(pk=self.image.pk).exists())

    # Without the builder environment folder (a build refused before
    # gcluster wrote it), the builder's state in the bucket decides

    @staticmethod
    def state_bucket():
        """The fake state bucket module of the cluster destroy tests,
        imported here because that module imports this one"""
        from ghpcfe.tests import test_destroy_without_controller  # pylint: disable=import-outside-toplevel

        return test_destroy_without_controller

    def delete_without_builder_folder(self, objects=None, error=None):
        """delete() with no builder environment folder and a fake state
        bucket holding objects ({name under the image's prefix: bytes}), or
        failing with error; Terraform must not run. Returns delete()'s
        result and the image's state location in the bucket."""
        fakes = self.state_bucket()
        shutil.rmtree(self.builder_dir.parent)
        bucket = fakes.FakeStateBucket(error=error)
        prefix = utils.terraform_state_prefix("image", self.image.id)
        bucket.objects = {
            f"{prefix}/{name}": data for name, data in (objects or {}).items()
        }
        with bucket.storage_patch(), mock.patch(self.RUN_TERRAFORM) as run:
            result = self.delete(self.all_granted())
        run.assert_not_called()
        self.assertFalse(self.builder_dir.exists())
        # The state was listed once, under the image's own prefix
        self.assertEqual(bucket.listed, [(fakes.STATE_BUCKET, f"{prefix}/")])
        return result, f"gs://{fakes.STATE_BUCKET}/{prefix}/"

    def test_missing_builder_folder_without_state_deletes_the_image(self):
        # A build that failed ("e") before its builder apply started wrote
        # no state object, and made nothing. A save never clears the flag,
        # so the row is set as such a build leaves it with an update.
        Image.objects.filter(pk=self.image.pk).update(
            status="e", builder_started=False
        )
        self.image.refresh_from_db()
        (response, body, shown), location = (
            self.delete_without_builder_folder()
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(body, {"success": True})
        self.assertEqual(shown, [])
        self.assertFalse(Image.objects.filter(pk=self.image.pk).exists())
        self.images.delete.assert_called_once_with(
            project=PROJECT, image="image-img-a"
        )
        self.assertIn(
            f"it has no Terraform state in {location} and no Terraform apply "
            "of its builder ever started, so no builder is left to destroy; "
            "skipping the builder destroy",
            self.logged,
        )

    def test_missing_builder_folder_without_state_after_a_build_keeps_the_image(self):
        # A finished build ("r"), or an image from before builder_started
        # existed (the migration gives those rows True), leaves the
        # builder's emptied state in the bucket; with none there, the state
        # is somewhere else and the builder's resources may still exist
        field = Image._meta.get_field("builder_started")
        self.assertIs(field.default, False)
        self.assertIs(field.db_default, True)
        self.assert_no_state_kept()

    def test_missing_builder_folder_without_state_after_a_failed_build_keeps_the_image(self):
        # A builder apply or Packer build that failed ("e") after the
        # builder apply started can have left the builder's resources
        self.image.status = "e"
        self.image.save()
        self.assert_no_state_kept()

    def assert_no_state_kept(self):
        """A delete without the builder folder or any state object, of an
        image whose builder apply started, is refused"""
        (response, body, shown), location = (
            self.delete_without_builder_folder()
        )
        self.assert_kept(
            response,
            body,
            shown,
            "Cannot delete image img-a: its builder environment folder "
            f"{self.builder_dir} is missing, and no Terraform state was "
            f"found under {location}, although a Terraform apply of its "
            "builder was started, or the image predates the started-flag "
            "check, so OFE treats its builder as deployed. Its builder's "
            "resources may still exist in Google Cloud.",
            "never migrated to the state bucket",
            "deployed under another deployment name",
            "restored with a new state namespace",
            "\"When a resource's state is not in the bucket\" in "
            "docs/admin_guide.md",
            "The image is kept.",
        )
        self.images.delete.assert_not_called()

    def test_missing_builder_folder_with_an_empty_state_deletes_the_image(self):
        # The state `terraform init` leaves, or a finished destroy
        (response, body, _), location = self.delete_without_builder_folder(
            {"default.tfstate": self.state_bucket().state_document()}
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(body, {"success": True})
        self.assertFalse(Image.objects.filter(pk=self.image.pk).exists())
        self.assertIn(
            f"its Terraform state in {location} holds no resources, so no "
            "builder is left to destroy",
            self.logged,
        )

    def test_missing_builder_folder_with_resources_keeps_the_image(self):
        fakes = self.state_bucket()
        (response, body, shown), location = self.delete_without_builder_folder(
            {
                "default.tfstate": fakes.state_document(
                    fakes.resource(), fakes.resource()
                )
            }
        )
        self.assert_kept(
            response,
            body,
            shown,
            "Cannot delete image img-a: its builder environment folder "
            f"{self.builder_dir} is missing, and its Terraform state in "
            f"{location} still holds 2 resources,",
            "Have an administrator destroy the resources from that state, "
            "then delete the image again.",
            "The image is kept.",
        )
        self.images.delete.assert_not_called()

    def test_missing_builder_folder_with_unreadable_state_keeps_the_image(self):
        (response, body, shown), location = self.delete_without_builder_folder(
            error=Forbidden("denied")
        )
        self.assert_kept(
            response,
            body,
            shown,
            f"its Terraform state in {location} could not be read as "
            "credential image-key: 403 denied.",
            "Fix the error and delete the image again.",
        )
        self.images.delete.assert_not_called()


class ImageBuilderStartedTests(TestCase):
    """builder_started is set, and saved, before the builder's first
    `terraform apply`, so a delete knows the builder may have resources
    even when the build failed ("e")"""

    def setUp(self):
        owner, _, _ = make_world()
        self.image = Image.objects.create(
            name="img-started",
            family="img-started",
            owner=owner,
            cloud_credential=Credential.objects.create(
                name="image-key", owner=owner, detail=KEY_JSON
            ),
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            status="c",
        )
        self.backend = ImageBackend(self.image)
        # Nothing is written under the configured base directory
        tmp = Path(tempfile.mkdtemp(prefix="ofe-builder-started-"))
        self.addCleanup(shutil.rmtree, tmp, ignore_errors=True)
        self.backend.image_dir = tmp / "image"
        (self.backend.image_dir / self.backend.blueprint_name / "builder-env").mkdir(
            parents=True
        )

    def build_failing_at(self, failing):
        """The real _create_builder_env, with a stand-in at the
        run_terraform boundary whose `terraform <failing>` fails; returns
        the builder_started the database held when each command started,
        as (command, flag) pairs"""
        seen = []

        def run_terraform(target_dir, command, *args, **kwargs):
            seen.append((
                command,
                Image.objects.get(pk=self.image.pk).builder_started,
            ))
            if command == failing:
                raise subprocess.CalledProcessError(1, ["terraform", command])

        with mock.patch.object(
            utils, "run_terraform", side_effect=run_terraform
        ), self.assertLogs(
            "ghpcfe.cluster_manager.image", "ERROR"
        ), self.assertRaises(subprocess.CalledProcessError):
            self.backend._create_builder_env()  # pylint: disable=protected-access
        self.image.refresh_from_db()
        self.assertEqual(self.image.status, "e")
        return seen

    def test_the_flag_is_set_before_the_builder_apply(self):
        self.assertFalse(self.image.builder_started)
        self.assertEqual(
            self.build_failing_at("apply"),
            [
                ("init", False),
                ("validate", False),
                ("plan", False),
                ("apply", True),
            ],
        )
        self.assertTrue(self.image.builder_started)

    def test_a_failure_before_the_apply_leaves_it_unset(self):
        self.assertEqual(
            self.build_failing_at("plan"),
            [("init", False), ("validate", False), ("plan", False)],
        )
        self.assertFalse(self.image.builder_started)

    def test_a_new_row_saves_false(self):
        self.assertFalse(
            Image.objects.get(pk=self.image.pk).builder_started
        )

    def test_a_stale_copy_saved_after_the_apply_started_keeps_the_flag(self):
        # A copy loaded before the build (a page or task holding the row)
        # and saved in full afterwards must not clear the flag, or a delete
        # without the builder folder would take a missing state to mean no
        # builder was made
        stale = Image.objects.get(pk=self.image.pk)
        self.build_failing_at("apply")
        self.assertFalse(stale.builder_started)
        stale.status = "e"
        stale.save()
        self.assertTrue(stale.builder_started)
        self.assertTrue(Image.objects.get(pk=self.image.pk).builder_started)

    def test_a_save_leaving_the_flag_out_reads_nothing(self):
        stale = Image.objects.get(pk=self.image.pk)
        self.build_failing_at("apply")
        stale.status = "r"
        # The UPDATE of status alone: the flag is not read
        with self.assertNumQueries(1):
            stale.save(update_fields=["status"])
        row = Image.objects.get(pk=self.image.pk)
        self.assertEqual((row.status, row.builder_started), ("r", True))
