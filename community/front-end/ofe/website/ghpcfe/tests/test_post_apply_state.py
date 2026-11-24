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

"""A failure after a successful `gcluster deploy` keeps the resource managed.

Once the deploy succeeds the resource exists in the cloud, so a later failure
(reading the state back from the bucket, or bookkeeping after it) must not
mark it "New" or leave it "Creating": either hides the actions that manage or
destroy it, and the resource keeps running and billing.

Like test_tf_state_backend.py, these tests import no Django: vpc.py and
clusterinfo.py are loaded by path with their Django-dependent imports stubbed.
"""

import importlib.util
import subprocess
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

import yaml

OFE_DIR = Path(__file__).resolve().parents[3]
CLUSTER_MANAGER = OFE_DIR / "website" / "ghpcfe" / "cluster_manager"


def _module(name, **attrs):
    module = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    return module


def _package(name):
    package = _module(name)
    package.__path__ = []
    return package


def _load(name, stubs):
    """Load cluster_manager/<name>.py inside a throwaway package."""
    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location(
            f"ofe_stub.cluster_manager.{name}", CLUSTER_MANAGER / f"{name}.py"
        )
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
    return module


def _load_utils():
    spec = importlib.util.spec_from_file_location(
        "ofe_post_apply_utils", CLUSTER_MANAGER / "utils.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


utils = _load_utils()


def _common_stubs(**models):
    manager = _package("ofe_stub.cluster_manager")
    manager.utils = utils
    stubs = {
        "ofe_stub": _package("ofe_stub"),
        "ofe_stub.cluster_manager": manager,
        "ofe_stub.cluster_manager.utils": utils,
        "ofe_stub.models": _module("ofe_stub.models", **models),
    }
    return stubs


def _load_vpc():
    stubs = _common_stubs(
        VirtualNetwork=mock.MagicMock(), VirtualSubnet=mock.MagicMock()
    )
    stubs.update(
        {
            "website": _package("website"),
            "website.settings": _module(
                "website.settings", SITE_NAME="test-site"
            ),
        }
    )
    return _load("vpc", stubs)


def _load_clusterinfo():
    stubs = _common_stubs(
        Cluster=mock.MagicMock(),
        ApplicationInstallationLocation=mock.MagicMock(),
        ComputeInstance=mock.MagicMock(),
        ContainerRegistry=mock.MagicMock(),
    )
    for name in ("c2", "cloud_info"):
        stubs[f"ofe_stub.cluster_manager.{name}"] = mock.MagicMock()
    stubs["ofe_stub.grafana"] = mock.MagicMock()
    stubs.update(
        {
            "django": _package("django"),
            "django.template": _module(
                "django.template", engines=mock.MagicMock()
            ),
            "django.utils": _module(
                "django.utils", timezone=mock.MagicMock()
            ),
            "google": _package("google"),
            "google.api_core": _package("google.api_core"),
            "google.api_core.exceptions": _module(
                "google.api_core.exceptions",
                PermissionDenied=type("PermissionDenied", (Exception,), {}),
            ),
            "website": _package("website"),
            "website.settings": _module(
                "website.settings", SITE_NAME="test-site"
            ),
        }
    )
    return _load("clusterinfo", stubs)


vpc_module = _load_vpc()
clusterinfo = _load_clusterinfo()

STATE_PULL_FAILED = subprocess.CalledProcessError(
    1, ["terraform", "state", "pull"]
)
DEPLOY_FAILED = subprocess.CalledProcessError(1, ["gcluster", "deploy"])


def _fake_vpc(cloud_state="cm"):
    vpc = mock.MagicMock()
    vpc.id = 7
    vpc.name = "net"
    vpc.cloud_id = ""
    vpc.cloud_region = "europe-west4"
    vpc.cloud_state = cloud_state
    vpc.cloud_credential.detail = '{"project_id": "p1"}'
    subnet = mock.MagicMock()
    subnet.id = 3
    subnet.is_managed = True
    subnet.cloud_region = "europe-west4"
    subnet.cidr = "10.0.0.0/24"
    subnet.private_google_access_enabled = True
    vpc.subnets.all.return_value = [subnet]
    return vpc, subnet


class StartVpcTests(unittest.TestCase):
    """Networks are deployed with gcluster and never left "Creating"."""

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self._tmp.name)
        self.addCleanup(self._tmp.cleanup)
        for name, value in (
            ("get_terraform_state_bucket", "bucket"),
            ("terraform_state_prefix", "ns/vpcs/vpc_7"),
        ):
            patcher = mock.patch.object(utils, name, return_value=value)
            patcher.start()
            self.addCleanup(patcher.stop)
        patcher = mock.patch.object(
            vpc_module, "_base_dir", return_value=self.dir
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def _start(self, vpc, deploy_error=None):
        def run(base_dir, *args, credentials_file=None):
            if args[0] == "deploy" and deploy_error:
                raise deploy_error

        with mock.patch.object(
            utils, "run_gcluster", side_effect=run
        ) as run_gcluster, mock.patch.object(utils, "run_terraform") as tf:
            vpc_module.start_vpc(vpc)
        tf.assert_not_called()
        return run_gcluster

    def test_deploy_failure_marks_vpc_unknown_not_creating(self):
        vpc, _ = _fake_vpc()
        with self.assertRaises(subprocess.CalledProcessError):
            self._start(vpc, deploy_error=DEPLOY_FAILED)
        self.assertEqual(vpc.cloud_state, "um")
        vpc.save.assert_called()

    def test_success_creates_then_deploys_and_records_names(self):
        vpc, subnet = _fake_vpc()
        run_gcluster = self._start(vpc)
        commands = [c.args[1] for c in run_gcluster.call_args_list]
        self.assertEqual(commands, ["create", "deploy"])
        self.assertEqual(vpc.cloud_state, "m")
        self.assertTrue(vpc.cloud_id.startswith("net-"))
        self.assertEqual(subnet.cloud_id, f"{vpc.cloud_id}-subnet-3")
        self.assertEqual(subnet.cloud_state, "m")

    def test_blueprint_lists_subnets(self):
        vpc, _ = _fake_vpc()
        self._start(vpc)
        bp = yaml.safe_load((self.dir / "vpc.yaml").read_text())
        self.assertEqual(
            bp["terraform_backend_defaults"]["configuration"]["prefix"],
            "ns/vpcs/vpc_7",
        )
        modules = bp["deployment_groups"][0]["modules"]
        # Private service access lives in the Cloud SQL cluster blueprint.
        self.assertEqual(
            [m["source"] for m in modules],
            ["modules/network/vpc", "modules/network/firewall-rules"],
        )
        self.assertEqual(modules[1]["use"], ["network"])
        subnets = modules[0]["settings"]["subnetworks"]
        self.assertEqual(
            subnets,
            [{
                "subnet_name": f"{vpc.cloud_id}-subnet-3",
                "subnet_region": "europe-west4",
                "subnet_ip": "10.0.0.0/24",
                "subnet_private_access": True,
            }],
        )

    def test_existing_network_name_is_kept(self):
        vpc, _ = _fake_vpc(cloud_state="m")
        vpc.cloud_id = "net-abc123"
        self._start(vpc)
        self.assertEqual(vpc.cloud_id, "net-abc123")

    def test_destroy_uses_gcluster(self):
        vpc, _ = _fake_vpc(cloud_state="m")
        vpc.cloud_id = "net-abc123"
        with mock.patch.object(utils, "run_gcluster") as run_gcluster, \
                mock.patch.object(utils, "run_terraform") as tf:
            vpc_module.destroy_vpc(vpc)
        self.assertEqual(run_gcluster.call_args.args[1:3],
                         ("destroy", "net-abc123"))
        tf.assert_not_called()
        self.assertEqual(vpc.cloud_state, "xm")


def _cluster_info(cloud_state):
    """A ClusterInfo with every step that touches disk or the cloud mocked."""
    info = object.__new__(clusterinfo.ClusterInfo)
    info.cluster = mock.MagicMock()
    info.cluster.cloud_state = cloud_state
    info.cluster.name = "c1"
    info.cluster_dir = Path("/x")
    for step in (
        "_create_cluster_dir",
        "_set_credentials",
        "update",
        "_run_ghpc",
    ):
        setattr(info, step, mock.MagicMock())
    return info


class ClusterPostApplyTests(unittest.TestCase):
    """A cluster stays managed once its apply has succeeded."""

    def _start(self, info, state_error=STATE_PULL_FAILED):
        with mock.patch.object(utils, "run_gcluster"), mock.patch.object(
            utils, "get_terraform_state", side_effect=state_error
        ):
            info.start_cluster(None)

    def test_state_read_failure_after_create_keeps_cluster_managed(self):
        for error in (STATE_PULL_FAILED, RuntimeError("bad state")):
            with self.subTest(error=type(error).__name__):
                info = _cluster_info("nm")
                with self.assertRaises(type(error)):
                    self._start(info, state_error=error)
                self.assertEqual(info.cluster.cloud_state, "m")
                self.assertEqual(info.cluster.status, "e")
                self.assertEqual(info.cluster.internal_name, "c1")

    def test_failure_before_apply_leaves_new_cluster_unmanaged(self):
        info = _cluster_info("nm")
        info._run_ghpc.side_effect = RuntimeError("gcluster failed")
        with self.assertRaises(RuntimeError):
            info.start_cluster(None)
        self.assertEqual(info.cluster.cloud_state, "nm")
        self.assertEqual(info.cluster.status, "e")

    def test_retry_of_running_cluster_keeps_it_managed(self):
        info = _cluster_info("m")
        info._run_ghpc.side_effect = RuntimeError("gcluster failed")
        with self.assertRaises(RuntimeError):
            info.start_cluster(None)
        self.assertEqual(info.cluster.cloud_state, "m")
        self.assertEqual(info.cluster.status, "e")

    def test_reconfigure_failure_keeps_running_cluster_managed(self):
        for failing_step in ("update", "_run_ghpc"):
            with self.subTest(step=failing_step):
                info = _cluster_info("m")
                getattr(info, failing_step).side_effect = RuntimeError("x")
                with self.assertRaises(RuntimeError):
                    info.reconfigure_cluster()
                self.assertEqual(info.cluster.cloud_state, "m")
                self.assertEqual(info.cluster.status, "e")

    def test_reconfigure_state_read_failure_keeps_cluster_managed(self):
        info = _cluster_info("m")
        with mock.patch.object(utils, "run_gcluster"), mock.patch.object(
            utils, "get_terraform_state", side_effect=STATE_PULL_FAILED
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                info.reconfigure_cluster()
        self.assertEqual(info.cluster.cloud_state, "m")
        self.assertEqual(info.cluster.status, "e")


if __name__ == "__main__":
    unittest.main()
