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

"""Every Terraform and Packer run that reaches Google Cloud acts as the
resource's own credential.

Each test runs the real code path of one call site (cluster, network,
filesystem, image or workbench) with utils.run_terraform, run_packer and
get_terraform_state replaced, and checks that each run that contacts the
cloud or the state bucket gets credential=<the resource's Credential>.
Another credential (the one make_world gives the network the resources sit
in) is in the database, so a run that picked up some other credential, or
none, fails the test. Offline: the permission preflights are replaced too.
"""

import json
import shutil
import subprocess
import tempfile
from pathlib import Path
from unittest import mock

from django.test import TestCase

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
    VirtualNetwork,
    Workbench,
)
from ghpcfe.tests.test_blueprint_identities import PROFILE_CASES, make_world

RUN_TERRAFORM = "ghpcfe.cluster_manager.utils.run_terraform"
RUN_PACKER = "ghpcfe.cluster_manager.utils.run_packer"
GET_STATE = "ghpcfe.cluster_manager.utils.get_terraform_state"
PREFLIGHT = "ghpcfe.cluster_manager.credentials.preflight"


class StopAfterApply(Exception):
    """Raised by the replaced state read, to stop a create after its apply"""


class CallSiteTestBase(TestCase):
    """A resource credential, and another one that must never be used"""

    def setUp(self):
        self.world = make_world()
        self.owner, self.other_credential, self.subnet = self.world
        self.credential = Credential.objects.create(
            name="resource-credential",
            owner=self.owner,
            detail=json.dumps(
                {"type": "service_account", "project_id": "resource-project"}
            ),
        )
        self.assertNotEqual(self.credential.pk, self.other_credential.pk)
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-call-sites-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        patcher = mock.patch(PREFLIGHT)
        self.preflight = patcher.start()
        self.addCleanup(patcher.stop)

    def assert_runs_as_resource(self, run, commands):
        """run (a replaced run_terraform or run_packer) ran commands, in
        order, each as the resource's credential and naming an operation"""
        self.assertEqual([call.args[1] for call in run.call_args_list], commands)
        for call in run.call_args_list:
            with self.subTest(command=call.args[1]):
                credential = call.kwargs.get("credential")
                self.assertIsInstance(credential, Credential)
                self.assertEqual(credential.pk, self.credential.pk)
                self.assertTrue(call.kwargs.get("operation"))

    def assert_state_read_as_resource(self, get_state):
        get_state.assert_called_once()
        credential = get_state.call_args.kwargs.get("credential")
        self.assertIsInstance(credential, Credential)
        self.assertEqual(credential.pk, self.credential.pk)


class ClusterCallSiteTests(CallSiteTestBase):
    """clusterinfo.py: init, validate and plan, then apply"""

    def setUp(self):
        super().setUp()
        self.cluster = PROFILE_CASES["minimal"](self.world)
        self.cluster.cloud_credential = self.credential
        self.cluster.save()
        self.info = ClusterInfo(self.cluster)

    def test_init_validate_and_plan_run_as_the_cluster_credential(self):
        with mock.patch(RUN_TERRAFORM) as run:
            self.info._initialize_terraform()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["init", "validate", "plan"])

    def test_apply_and_state_read_run_as_the_cluster_credential(self):
        with mock.patch(RUN_TERRAFORM) as run, mock.patch(
            GET_STATE, side_effect=StopAfterApply
        ) as get_state, mock.patch(
            "ghpcfe.cluster_manager.c2.create_cluster_subscription"
        ), self.assertRaises(StopAfterApply):
            self.info._apply_terraform()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["apply"])
        self.assert_state_read_as_resource(get_state)

    def test_destroy_and_its_state_read_run_as_the_cluster_credential(self):
        # A destroy that fails reads the state it left from the state
        # bucket, to tell a failure on custom roles alone from any other.
        # The deployment folder is there, as after any apply. It is made in
        # a base directory of this test's own: cluster ids restart in each
        # test's database, so the shared one can hold this cluster id's
        # folder from an earlier test module.
        with mock.patch.dict(utils.g_config, {"baseDir": self.tmp / "base"}):
            self.info = ClusterInfo(self.cluster)
        terraform_dir = self.info.get_terraform_dir()
        self.assertTrue(terraform_dir.is_relative_to(self.tmp))
        terraform_dir.mkdir(parents=True)
        (terraform_dir / "terraform_destroy_log.stderr").write_text(
            "Error: Error deleting the custom project role\n", encoding="utf-8"
        )
        failure = subprocess.CalledProcessError(1, ["terraform", "destroy"])
        with mock.patch(RUN_TERRAFORM, side_effect=failure) as run, mock.patch(
            GET_STATE, side_effect=RuntimeError("state stand-in")
        ) as get_state, self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", "WARNING"
        ), self.assertRaises(subprocess.CalledProcessError):
            self.info.stop_cluster()
        self.assert_runs_as_resource(run, ["destroy"])
        self.assert_state_read_as_resource(get_state)
        self.assertEqual(get_state.call_args.args[0], terraform_dir)
        self.assertTrue(get_state.call_args.kwargs.get("operation"))


class NetworkCallSiteTests(CallSiteTestBase):
    """vpc.py: the credential _preflight hands every Terraform run"""

    def setUp(self):
        super().setUp()
        self.vpc = VirtualNetwork.objects.create(
            name="net-call-site",
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            cloud_state="nm",
        )
        self.target_dir = cm_vpc._tf_dir_for_vpc(self.vpc.id)  # pylint: disable=protected-access
        shutil.rmtree(self.target_dir, ignore_errors=True)
        self.addCleanup(shutil.rmtree, self.target_dir, ignore_errors=True)

    def test_create_runs_init_validate_and_plan_as_the_network_credential(self):
        source = self.tmp / "vpc_tf"
        source.mkdir()
        (source / "main.tf").write_text("# stand-in\n", encoding="utf-8")
        with mock.patch.object(
            cm_vpc, "_tf_source_dir_for_vpc", return_value=source
        ), mock.patch(RUN_TERRAFORM) as run:
            cm_vpc.create_vpc(self.vpc)
        self.assert_runs_as_resource(run, ["init", "validate", "plan"])

    def test_start_runs_apply_and_state_read_as_the_network_credential(self):
        state = {"outputs": {"vpc_id": {"value": "net-call-site-id"}}}
        with mock.patch(RUN_TERRAFORM) as run, mock.patch(
            GET_STATE, return_value=state
        ) as get_state:
            cm_vpc.start_vpc(self.vpc)
        self.assert_runs_as_resource(run, ["apply"])
        self.assert_state_read_as_resource(get_state)
        self.assertEqual(self.vpc.cloud_state, "m")

    def test_destroy_runs_as_the_network_credential(self):
        self.vpc.cloud_state = "dm"
        with mock.patch(RUN_TERRAFORM) as run:
            cm_vpc.destroy_vpc(self.vpc)
        self.assert_runs_as_resource(run, ["destroy"])


class FilesystemCallSiteTests(CallSiteTestBase):
    """filesystem.py: the cloud dict start_filesystem gives each run"""

    def setUp(self):
        super().setUp()
        created = GCPFilestoreFilesystem.objects.create(
            name="fscallsite",
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            vpc=self.subnet.vpc,
            capacity=1024,
            cloud_state="nm",
        )
        self.fs = Filesystem.objects.get(pk=created.pk)
        base_dir = cm_filesystem._base_dir_for_fs(self.fs)  # pylint: disable=protected-access
        self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)

    def test_start_runs_every_terraform_command_as_the_filesystem_credential(self):
        output = self.tmp / "terraform_output_log.stdout"
        output.write_text(
            json.dumps({
                "network_storage_fscallsite": {
                    "value": {"server_ip": "10.10.0.5"}
                }
            }),
            encoding="utf-8",
        )
        with mock.patch.object(
            cm_filesystem, "_run_ghpc"
        ) as run_ghpc, mock.patch(
            RUN_TERRAFORM, return_value=(output, self.tmp / "stderr")
        ) as run:
            cm_filesystem.start_filesystem(self.fs)
        self.assert_runs_as_resource(run, ["init", "plan", "apply", "output"])
        # gcluster's validators run as it too
        self.assertEqual(run_ghpc.call_args.args[1].pk, self.credential.pk)
        self.assertEqual(self.fs.cloud_state, "m")

    def test_destroy_runs_as_the_filesystem_credential(self):
        with mock.patch(RUN_TERRAFORM) as run:
            cm_filesystem.destroy_filesystem(self.fs)
        self.assert_runs_as_resource(run, ["destroy"])


class ImageCallSiteTests(CallSiteTestBase):
    """image.py: the credential _cloud gives the builder's Terraform runs
    and the Packer build"""

    def setUp(self):
        super().setUp()
        self.image = Image.objects.create(
            name="img-call-site",
            family="img-call-site",
            owner=self.owner,
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )
        self.backend = ImageBackend(self.image)
        # Nothing is written under the configured base directory
        self.backend.image_dir = self.tmp / "image"
        deployment = self.backend.image_dir / self.backend.blueprint_name
        self.builder_dir = deployment / "builder-env"
        self.builder_dir.mkdir(parents=True)
        (deployment / "packer-image" / "custom-image").mkdir(parents=True)

    def test_builder_env_runs_as_the_image_credential(self):
        (self.builder_dir / "terraform_output_log.stdout").write_text(
            "#!/bin/bash\necho ready\n", encoding="utf-8"
        )
        with mock.patch(RUN_TERRAFORM) as run, mock.patch.object(
            self.backend, "_run_ghpc_import_inputs"
        ):
            self.backend._create_builder_env()  # pylint: disable=protected-access
        self.assert_runs_as_resource(
            run, ["init", "validate", "plan", "apply", "output"]
        )

    def test_packer_validate_and_build_run_as_the_image_credential(self):
        with mock.patch(RUN_PACKER) as run:
            self.backend._create_image()  # pylint: disable=protected-access
        # packer init only installs plugins and needs no cloud identity
        init, *cloud_runs = run.call_args_list
        self.assertEqual(init.args[1], "init")
        self.assertIsNone(init.kwargs.get("credential"))
        run.call_args_list = cloud_runs
        self.assert_runs_as_resource(run, ["validate", "build"])
        self.image.refresh_from_db()
        self.assertEqual(self.image.status, "r")

    def test_builder_env_destroy_runs_as_the_image_credential(self):
        with mock.patch(RUN_TERRAFORM) as run:
            self.backend._destroy_builder_env()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["destroy"])


class WorkbenchCallSiteTests(CallSiteTestBase):
    """workbenchinfo.py: the credential _cloud gives each Terraform run"""

    def setUp(self):
        super().setUp()
        self.workbench = Workbench.objects.create(
            name="wb-call-site",
            owner=self.owner,
            trusted_user=self.owner,
            subnet=self.subnet,
            cloud_credential=self.credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )
        self.info = WorkbenchInfo(self.workbench)

    def test_init_validate_and_plan_run_as_the_workbench_credential(self):
        with mock.patch(RUN_TERRAFORM) as run:
            self.info._terraform_init()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["init", "validate", "plan"])

    def test_apply_and_state_read_run_as_the_workbench_credential(self):
        state = {"outputs": {"notebook_instance_name": {"value": "nb-1"}}}
        with mock.patch(RUN_TERRAFORM) as run, mock.patch(
            GET_STATE, return_value=state
        ) as get_state:
            self.info._terraform_create()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["apply"])
        self.assert_state_read_as_resource(get_state)
        self.assertEqual(self.workbench.internal_name, "nb-1")

    def test_destroy_runs_as_the_workbench_credential(self):
        with mock.patch(RUN_TERRAFORM) as run:
            self.info._terraform_destroy()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["destroy"])

    def test_proxy_uri_refresh_and_state_read_run_as_the_workbench_credential(self):
        state = {"outputs": {"notebook_proxy_uri": {"value": "https://nb.example"}}}
        with mock.patch(RUN_TERRAFORM) as run, mock.patch(
            GET_STATE, return_value=state
        ) as get_state:
            self.info._get_proxy_uri()  # pylint: disable=protected-access
        self.assert_runs_as_resource(run, ["apply"])
        # The apply only refreshes the state
        self.assertEqual(run.call_args.args[2], ["-refresh-only"])
        self.assert_state_read_as_resource(get_state)
        self.assertEqual(self.workbench.proxy_uri, "https://nb.example")
