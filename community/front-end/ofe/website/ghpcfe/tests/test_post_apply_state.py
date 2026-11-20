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

"""A failure after a successful `terraform apply` keeps the resource managed.

Once the apply succeeds the resource exists in the cloud, so a later failure
(reading the state back from the bucket, or bookkeeping after it) must not
mark it "New" or leave it "Creating": either hides the actions that manage or
destroy it, and the resource keeps running and billing.

Like test_tf_state_backend.py, these tests import no Django: vpc.py and
clusterinfo.py are loaded by path with their Django-dependent imports stubbed.
"""

import importlib.util
import subprocess
import sys
import types
import unittest
from pathlib import Path
from unittest import mock

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
    return _load(
        "vpc",
        _common_stubs(
            VirtualNetwork=mock.MagicMock(), VirtualSubnet=mock.MagicMock()
        ),
    )


def _load_clusterinfo():
    stubs = _common_stubs(
        Cluster=mock.MagicMock(),
        ApplicationInstallationLocation=mock.MagicMock(),
        ComputeInstance=mock.MagicMock(),
        ContainerRegistry=mock.MagicMock(),
    )
    for name in ("c2", "cloud_info"):
        stubs[f"ofe_stub.cluster_manager.{name}"] = mock.MagicMock()
    # The encryption checks need the models; nothing here is encrypted.
    cmek = mock.MagicMock(unsafe=True)  # it has assert_* functions
    cmek.CmekConfigError = type("CmekConfigError", (Exception,), {})
    stubs["ofe_stub.cluster_manager.cmek"] = cmek
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
APPLY_FAILED = subprocess.CalledProcessError(1, ["terraform", "apply"])


def _fake_vpc(cloud_state="cm"):
    vpc = mock.MagicMock()
    vpc.id = 7
    vpc.cloud_state = cloud_state
    subnet = mock.MagicMock()
    subnet.id = 3
    subnet.is_managed = True
    vpc.subnets.all.return_value = [subnet]
    return vpc, subnet


class StartVpcTests(unittest.TestCase):
    """start_vpc never leaves a network stuck in "Creating"."""

    def _start(self, vpc, state=None, state_error=None, apply_error=None):
        with mock.patch.object(
            vpc_module, "_tf_dir_for_vpc", return_value=Path("/x")
        ), mock.patch.object(
            utils, "run_terraform", side_effect=apply_error
        ), mock.patch.object(
            utils,
            "get_terraform_state",
            return_value=state,
            side_effect=state_error,
        ):
            vpc_module.start_vpc(vpc)

    def test_state_read_failure_after_apply_keeps_vpc_managed(self):
        for error in (STATE_PULL_FAILED, RuntimeError("bad state")):
            with self.subTest(error=type(error).__name__):
                vpc, _ = _fake_vpc()
                with self.assertRaises(type(error)):
                    self._start(vpc, state_error=error)
                self.assertEqual(vpc.cloud_state, "m")
                vpc.save.assert_called()

    def test_missing_output_after_apply_keeps_vpc_managed(self):
        vpc, _ = _fake_vpc()
        with self.assertRaises(KeyError):
            self._start(vpc, state={"outputs": {}})
        self.assertEqual(vpc.cloud_state, "m")

    def test_apply_failure_marks_vpc_unknown_not_creating(self):
        vpc, _ = _fake_vpc()
        with self.assertRaises(subprocess.CalledProcessError):
            self._start(vpc, apply_error=APPLY_FAILED)
        self.assertEqual(vpc.cloud_state, "um")
        vpc.save.assert_called()

    def test_success_records_ids(self):
        vpc, subnet = _fake_vpc()
        state = {
            "outputs": {
                "vpc_id": {"value": "net-1"},
                "subnet-3": {"value": "sub-3"},
            }
        }
        self._start(vpc, state=state)
        self.assertEqual(vpc.cloud_state, "m")
        self.assertEqual(vpc.cloud_id, "net-1")
        self.assertEqual(subnet.cloud_id, "sub-3")
        self.assertEqual(subnet.cloud_state, "m")


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
        "_initialize_terraform",
    ):
        setattr(info, step, mock.MagicMock())
    return info


class ClusterPostApplyTests(unittest.TestCase):
    """A cluster stays managed once its apply has succeeded."""

    def _start(self, info, state_error=STATE_PULL_FAILED):
        with mock.patch.object(utils, "run_terraform"), mock.patch.object(
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
        for failing_step in ("update", "_run_ghpc", "_initialize_terraform"):
            with self.subTest(step=failing_step):
                info = _cluster_info("m")
                info.cluster.status = "r"
                getattr(info, failing_step).side_effect = RuntimeError("x")
                with self.assertRaises(RuntimeError):
                    info.reconfigure_cluster()
                self.assertEqual(info.cluster.cloud_state, "m")
                # A blueprint that cannot be regenerated stops the
                # reconfigure before anything runs: the cluster keeps running.
                self.assertEqual(info.cluster.status,
                                 "r" if failing_step == "update" else "e")

    def test_reconfigure_state_read_failure_keeps_cluster_managed(self):
        info = _cluster_info("m")
        with mock.patch.object(utils, "run_terraform"), mock.patch.object(
            utils, "get_terraform_state", side_effect=STATE_PULL_FAILED
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                info.reconfigure_cluster()
        self.assertEqual(info.cluster.cloud_state, "m")
        self.assertEqual(info.cluster.status, "e")


if __name__ == "__main__":
    unittest.main()
