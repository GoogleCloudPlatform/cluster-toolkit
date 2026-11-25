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


def _load_image():
    stubs = _common_stubs()
    google = _package("google")
    stubs.update({
        "django": _package("django"),
        "django.conf": _module("django.conf", settings=mock.MagicMock()),
        "google": google,
        "google.api_core": _package("google.api_core"),
        "google.api_core.exceptions": _module(
            "google.api_core.exceptions", NotFound=Exception
        ),
        "google.cloud": _module("google.cloud", compute_v1=mock.MagicMock()),
    })
    return _load("image", stubs)


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
image_module = _load_image()

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
        # One private service access per network, shared by its Cloud SQL
        # clusters; the firewall rule uses its range, which gcluster needs.
        self.assertEqual(
            [m["source"] for m in modules],
            ["modules/network/vpc", "modules/network/private-service-access",
             "modules/network/firewall-rules"],
        )
        self.assertEqual(modules[1]["use"], ["network"])
        self.assertNotIn("settings", modules[1])  # default psconnect-<network>
        self.assertEqual(modules[2]["use"], ["network"])
        self.assertIn(
            "$(private_service_access.cidr_range)",
            modules[2]["settings"]["ingress_rules"][0]["source_ranges"],
        )
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

    def test_longest_vpc_name_keeps_every_resource_name_within_63(self):
        vpc, subnet = _fake_vpc()
        vpc.name = "a" * 63  # the longest name the model accepts
        subnet.id = 99999
        self._start(vpc)
        network = vpc.cloud_id
        self.assertLessEqual(len(network), 42)
        derived = [
            network,
            subnet.cloud_id,
            f"psconnect-{network}",
            f"{network}-router",
            # The vpc module's own rules, the longest suffixes.
            f"{network}-fw-allow-iap-ingress",
            f"{network}-fw-allow-ssh-ingress",
        ]
        bp = yaml.safe_load((self.dir / "vpc.yaml").read_text())
        rules = bp["deployment_groups"][0]["modules"][2]["settings"]
        derived += [r["name"] for r in rules["ingress_rules"]]
        for name in derived:
            with self.subTest(name=name):
                self.assertLessEqual(len(name), 63)

    def test_existing_network_name_is_kept(self):
        vpc, _ = _fake_vpc(cloud_state="m")
        vpc.cloud_id = "net-abc123"
        self._start(vpc)
        self.assertEqual(vpc.cloud_id, "net-abc123")

    def test_destroy_uses_gcluster(self):
        vpc, _ = _fake_vpc(cloud_state="m")
        vpc.cloud_id = "net-abc123"
        (self.dir / "net-abc123").mkdir()
        with mock.patch.object(utils, "run_gcluster") as run_gcluster, \
                mock.patch.object(utils, "run_terraform") as tf:
            vpc_module.destroy_vpc(vpc)
        self.assertEqual(run_gcluster.call_args.args[1:3],
                         ("destroy", "net-abc123"))
        tf.assert_not_called()
        self.assertEqual(vpc.cloud_state, "xm")

    def test_log_file_is_the_gcluster_log_the_page_shows(self):
        self.assertEqual(vpc_module.get_log_file(_fake_vpc()[0]),
                         self.dir / "gcluster.log")

    def test_psa_blocked_destroy_is_recognised_from_the_log(self):
        vpc, _ = _fake_vpc()
        log = self.dir / "gcluster.log"
        blocked = ("\n$ gcluster destroy net --auto-approve\nError: Unable to "
                   "remove Service Networking Connection ... reason: "
                   "FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION\n")
        cases = {
            "no log": None,
            "blocked destroy": "\n$ gcluster deploy net\nok\n" + blocked,
            "other destroy error": "\n$ gcluster destroy net\nError: quota\n",
            # A later command supersedes the blocked destroy.
            "deployed since": blocked + "\n$ gcluster deploy net\nok\n",
        }
        expected = {"no log": False, "blocked destroy": True,
                    "other destroy error": False, "deployed since": False}
        for name, text in cases.items():
            with self.subTest(case=name):
                if text is None:
                    log.unlink(missing_ok=True)
                else:
                    log.write_text(text)
                self.assertEqual(vpc_module.destroy_blocked_by_psa(vpc),
                                 expected[name])

    def test_destroy_failure_marks_vpc_unknown_so_destroy_is_offered(self):
        # e.g. the network still used by a leftover PSA address.
        vpc, _ = _fake_vpc(cloud_state="dm")
        vpc.cloud_id = "net-abc123"
        (self.dir / "net-abc123").mkdir()
        failed = subprocess.CalledProcessError(1, ["gcluster", "destroy"])
        with mock.patch.object(utils, "run_gcluster", side_effect=failed):
            with self.assertRaises(subprocess.CalledProcessError):
                vpc_module.destroy_vpc(vpc)
        self.assertEqual(vpc.cloud_state, "um")
        vpc.save.assert_called()

    def test_destroy_of_never_applied_vpc_just_marks_it_destroyed(self):
        # Created, then destroyed without "Apply Cloud Changes": no cloud_id
        # and no deployment folder, so there is nothing for gcluster to do.
        for cloud_id in (None, "net-abc123"):
            with self.subTest(cloud_id=cloud_id):
                vpc, _ = _fake_vpc(cloud_state="nm")
                vpc.cloud_id = cloud_id
                with mock.patch.object(utils, "run_gcluster") as run_gcluster:
                    vpc_module.destroy_vpc(vpc)
                run_gcluster.assert_not_called()
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

    def test_successful_start_pings_controller_after_last_save(self):
        # A redeploy of a cluster whose controller is already up gets no new
        # "ready" message; the PING's answer stands in for it.
        info = _cluster_info("m")
        info.cluster.id = 18
        info._deploy_cluster = mock.MagicMock()
        calls = mock.MagicMock()
        info.cluster.save = calls.save
        with mock.patch.object(clusterinfo, "c2") as c2:
            c2.ping_cluster = calls.ping_cluster
            info.start_cluster(None)
        names = [c[0] for c in calls.mock_calls]
        self.assertEqual(names[-1], "ping_cluster")
        calls.ping_cluster.assert_called_once_with(18)

    def test_failed_start_does_not_ping(self):
        info = _cluster_info("nm")
        info._run_ghpc.side_effect = RuntimeError("gcluster failed")
        with mock.patch.object(clusterinfo, "c2") as c2:
            with self.assertRaises(RuntimeError):
                info.start_cluster(None)
        c2.ping_cluster.assert_not_called()

    def test_destroy_failure_marks_cluster_error_so_destroy_is_offered(self):
        # e.g. Service Networking refusing to drop a connection still in use:
        # the cluster must not stay in "Terminating", which offers no Destroy.
        info = _cluster_info("m")
        failed = subprocess.CalledProcessError(1, ["gcluster", "destroy"])
        with mock.patch.object(utils, "run_gcluster", side_effect=failed):
            with self.assertRaises(subprocess.CalledProcessError):
                info.stop_cluster()
        self.assertEqual(info.cluster.status, "e")
        self.assertEqual(info.cluster.cloud_state, "um")
        info.cluster.save.assert_called()


class ImageCloudStateTests(unittest.TestCase):
    """Images report cloud_state/cloud_id like the other resources."""

    def _backend(self):
        image = mock.MagicMock()
        image.name = "img"
        image.cloud_state = "nm"
        image.cloud_id = None
        backend = image_module.ImageBackend.__new__(image_module.ImageBackend)
        backend.image = image
        return backend, image

    def test_status_sets_cloud_state(self):
        for status, cloud_state in (("c", "cm"), ("r", "m"), ("e", "um")):
            with self.subTest(status=status):
                backend, image = self._backend()
                backend.update_image_status(status)
                self.assertEqual(image.status, status)
                self.assertEqual(image.cloud_state, cloud_state)
                image.save.assert_called()

    def test_ready_image_records_its_gcp_name(self):
        backend, image = self._backend()
        backend.update_image_status("r")
        self.assertEqual(image.cloud_id, "image-img")

    def test_failed_image_has_no_cloud_id(self):
        backend, image = self._backend()
        backend.update_image_status("e")
        self.assertIsNone(image.cloud_id)


if __name__ == "__main__":
    unittest.main()
