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
"""A deployed resource keeps its encryption through the flows that reset it.

- Starting a cluster and creating a Filestore put the record back to "nm"
  before the blueprint is regenerated, so the generator must still see a
  deployed resource and keep its stored choice whatever enforcement says.
- A running cluster's credential, name, subnet, region and zone cannot be
  changed by a forged POST.
- A rotation period too long for int() is a normal settings error.
- A successful start or reconfigure clears the last failure reason, and a
  failed start says why.
"""
import json
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from django.test import SimpleTestCase

from ..cluster_manager import cmek

CREDENTIAL = SimpleNamespace(detail=json.dumps({"project_id": "proj-abcde"}))


def _config(enforcement):
    return {"cmek": {"enforcement": enforcement, "key_ring_name": "ofe-cmek"},
            "server": {"gcs_bucket": "bucket"}}


def _cmek_fields(enabled):
    return dict(
        cmek_enabled=enabled, cmek_key_name="k1", cmek_key_ring_name="ring",
        cmek_key_project_id="proj-abcde", cmek_deletion_policy="ABANDON",
        cmek_protection_level="", cmek_rotation_period="",
        cmek_encryption_service_account="", cloud_credential=CREDENTIAL,
        save=lambda **kwargs: None)


# (enforcement, stored cmek_enabled): every pairing where enforcement would
# otherwise override the stored choice, and the ones where it agrees.
CASES = (("required", False), ("required", True), ("disabled", True),
         ("disabled", False), ("preferred", True), ("preferred", False))


class StartClusterKeepsItsEncryptionTests(SimpleTestCase):
    """start_cluster() resets the record before regenerating cluster.yaml."""

    def _cluster(self, enabled):
        return SimpleNamespace(
            id=1, pk=1, name="c", cloud_id="c-abcd", project_id="proj-abcde",
            cloud_zone="europe-west4-a", cloud_region="europe-west4",
            status="r", cloud_state="m", use_bigquery=False,
            use_containers=False, use_cloudsql=False, enable_slurm_auth=True,
            subnet=SimpleNamespace(
                cloud_id="sub", vpc=SimpleNamespace(cloud_id="net")),
            controller_instance_type="n2-standard-2",
            controller_disk_type="pd-standard", controller_disk_size=120,
            num_login_nodes=1, login_node_instance_type="n2-standard-2",
            login_node_disk_type="pd-standard", login_node_disk_size=120,
            controller_node_image=None, login_node_image=None,
            mount_points=SimpleNamespace(order_by=lambda field: []),
            partitions=SimpleNamespace(all=lambda: []), shared_fs=None,
            **_cmek_fields(enabled))

    def _start(self, enforcement, enabled):
        """Start a deployed cluster and return the cluster.yaml it wrote."""
        from django.template import engines
        from ..cluster_manager import clusterinfo
        info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
        info.config = _config(enforcement)
        info.cluster = self._cluster(enabled)
        info.cluster_dir = Path(tempfile.mkdtemp())
        info.env = engines["django"]
        # Stop after the blueprint is written: the rest is Terraform.
        with mock.patch.object(info, "_create_cluster_dir"), \
                mock.patch.object(info, "_set_credentials"), \
                mock.patch.object(info, "_prepare_bootstrap_gcs"), \
                mock.patch.object(info, "_run_ghpc",
                                  side_effect=RuntimeError("stop")), \
                mock.patch.object(cmek, "ring_already_exists",
                                  return_value=True), \
                self.assertRaises(RuntimeError):
            info.start_cluster("{}")
        return (info.cluster_dir / "cluster.yaml").read_text()

    def test_the_stored_choice_survives_the_reset(self):
        for enforcement, enabled in CASES:
            with self.subTest(enforcement=enforcement, enabled=enabled):
                text = self._start(enforcement, enabled)
                self.assertEqual(cmek.KEY_IAM_MODULE_ID in text, enabled)


class CreateFilestoreKeepsItsEncryptionTests(SimpleTestCase):
    """BackendCreateFilesystem resets the record before filesystem.yaml."""

    def _filestore(self, enabled, cloud_state):
        return SimpleNamespace(
            id=5, pk=5, name="home", cloud_zone="europe-west4-a",
            cloud_region="europe-west4", capacity=1024,
            vpc=SimpleNamespace(cloud_id="net"), cloud_state=cloud_state,
            exports=SimpleNamespace(first=lambda: SimpleNamespace(
                export_name="/share")),
            get_performance_tier_display=lambda: "ENTERPRISE",
            **_cmek_fields(enabled))

    def _create(self, enforcement, enabled):
        """Run the create flow on a deployed Filestore; its blueprint."""
        from ..cluster_manager import filesystem
        from ..models import FilesystemImpl
        from ..views import filesystems
        target = Path(tempfile.mkdtemp())
        parent = self._filestore(enabled, "m")
        parent.impl_type = FilesystemImpl.GCPFILESTORE
        # The generator works on the GCPFilestoreFilesystem row, a separate
        # instance Django loads after the reset has been saved.
        parent.gcpfilestorefilesystem = self._filestore(enabled, "nm")
        with mock.patch.object(filesystem.utils, "load_config",
                               return_value=_config(enforcement)), \
                mock.patch.object(filesystem, "_base_dir_for_fs",
                                  return_value=target), \
                mock.patch.object(filesystem, "_get_credentials_file",
                                  return_value=target / "credentials"), \
                mock.patch.object(cmek, "ring_already_exists",
                                  return_value=True):
            filesystems.BackendCreateFilesystem().cmd(None, None, parent)
        self.assertEqual(parent.cloud_state, "nm")
        return (target / "filesystem.yaml").read_text()

    def test_the_stored_choice_survives_the_reset(self):
        for enforcement, enabled in CASES:
            with self.subTest(enforcement=enforcement, enabled=enabled):
                text = self._create(enforcement, enabled)
                self.assertEqual(cmek.KEY_IAM_MODULE_ID in text, enabled)


class DeployedSnapshotTests(SimpleTestCase):
    """is_frozen() reads the snapshot the start and create flows take."""

    def test_the_snapshot_wins_over_the_state(self):
        from ..models import Cluster, GCPFilestoreFilesystem
        for resource in (Cluster(pk=1, cloud_state="nm", status="c"),
                         GCPFilestoreFilesystem(pk=1, cloud_state="nm")):
            with self.subTest(resource=type(resource).__name__):
                self.assertFalse(cmek.is_frozen(resource))
                resource._cmek_deployed = True
                self.assertTrue(cmek.is_frozen(resource))

    def test_the_snapshot_is_on_that_instance_only(self):
        from ..models import Cluster
        started = Cluster(pk=1, cloud_state="nm", status="c")
        started._cmek_deployed = True
        self.assertFalse(cmek.is_frozen(
            Cluster(pk=1, cloud_state="nm", status="c")))

    def test_a_false_snapshot_leaves_the_state_check(self):
        from ..models import Cluster
        cluster = Cluster(pk=1, cloud_state="m", status="r")
        cluster._cmek_deployed = False
        self.assertTrue(cmek.is_frozen(cluster))

    def test_a_cluster_being_destroyed_is_frozen(self):
        from ..models import Cluster
        self.assertTrue(cmek.is_frozen(
            Cluster(pk=1, cloud_state="dm", status="t")))

    def test_a_mock_is_not_frozen_by_its_auto_attributes(self):
        self.assertFalse(cmek.is_frozen(mock.MagicMock(
            pk=1, cloud_state="nm", status="n")))


class RunningClusterFormTests(SimpleTestCase):
    """A running cluster's placement fields ignore what a POST sends."""

    LOCKED = ("cloud_credential", "name", "subnet", "cloud_region",
              "cloud_zone")

    def _form(self, data=None):
        from ..forms import ClusterForm
        from ..models import Cluster
        # Unsaved, so the form reads no many-to-many fields from a database.
        cluster = Cluster(status="r", name="c1", cloud_region="europe-west4",
                          cloud_zone="europe-west4-a", spackdir="/opt/spack")
        with mock.patch.object(cmek.CmekSettings, "from_config",
                               return_value=cmek.CmekSettings(
                                   enforcement="preferred",
                                   key_project_id="keys-proj",
                                   key_ring_name="ofe-cmek")):
            return ClusterForm(data=data, instance=cluster)

    def test_the_fields_are_disabled_and_still_look_readonly(self):
        form = self._form()
        for name in self.LOCKED:
            with self.subTest(field=name):
                self.assertTrue(form.fields[name].disabled)
                self.assertTrue(form.fields[name].widget.attrs["readonly"])

    def test_a_forged_zone_is_ignored_and_other_edits_are_kept(self):
        form = self._form(data={"cloud_zone": "us-central1-a",
                                "name": "forged", "spackdir": "/srv/spack"})
        self.assertEqual(form["cloud_zone"].value(), "europe-west4-a")
        self.assertEqual(form["name"].value(), "c1")
        self.assertEqual(form["spackdir"].value(), "/srv/spack")
        self.assertNotIn("cloud_zone", form.changed_data)
        self.assertIn("spackdir", form.changed_data)
        form.is_valid()
        self.assertNotIn("cloud_zone", form.errors)
        self.assertEqual(form.cleaned_data["cloud_zone"], "europe-west4-a")
        self.assertEqual(form.cleaned_data["name"], "c1")
        self.assertEqual(form.cleaned_data["spackdir"], "/srv/spack")


class LongRotationPeriodTests(SimpleTestCase):
    """A rotation period too long for int() is refused like any other."""

    def test_thousands_of_digits_are_a_settings_error(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.assert_rotation_period("9" * 5000 + "s")

    def test_eleven_digits_are_refused_and_ten_accepted(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.assert_rotation_period("1" * 11 + "s")
        self.assertEqual(cmek.assert_rotation_period("1" * 10 + "s"),
                         "1" * 10 + "s")

    def test_a_valid_period_still_passes(self):
        self.assertEqual(cmek.assert_rotation_period("86400s"), "86400s")


class StatusMessageTests(SimpleTestCase):
    """A successful start or reconfigure clears the last failure reason."""

    def _info(self):
        from ..cluster_manager import clusterinfo
        info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
        info.cluster = SimpleNamespace(
            pk=None, status="e", cloud_state="nm",
            status_message="Reconfigure failed: no key",
            grafana_dashboard_url="", save=mock.Mock())
        return info

    def _run(self, method, apply_error=None):
        from ..cluster_manager import clusterinfo
        info = self._info()
        with mock.patch.object(info, "_create_cluster_dir"), \
                mock.patch.object(info, "_set_credentials"), \
                mock.patch.object(info, "update"), \
                mock.patch.object(info, "_run_ghpc"), \
                mock.patch.object(info, "_initialize_terraform"), \
                mock.patch.object(info, "_apply_terraform",
                                  side_effect=apply_error), \
                mock.patch.object(clusterinfo.grafana,
                                  "create_cluster_dashboard",
                                  return_value={"url": ""}):
            if apply_error:
                with self.assertRaises(type(apply_error)):
                    method(info)
            else:
                method(info)
        return info.cluster

    @staticmethod
    def _start(info):
        info.start_cluster("{}")

    @staticmethod
    def _reconfigure(info):
        info.reconfigure_cluster()

    def test_success_clears_it(self):
        for method in (self._start, self._reconfigure):
            with self.subTest(method=method.__name__):
                cluster = self._run(method)
                self.assertEqual(cluster.status_message, "")

    def test_a_failed_reconfigure_keeps_it(self):
        cluster = self._run(self._reconfigure, RuntimeError("apply failed"))
        self.assertEqual(cluster.status_message, "Reconfigure failed: no key")
        self.assertEqual(cluster.status, "e")

    def test_a_failed_start_says_why(self):
        cluster = self._run(self._start, RuntimeError("apply failed"))
        self.assertEqual(cluster.status_message, "Start failed: apply failed")
        self.assertEqual((cluster.status, cluster.cloud_state), ("e", "nm"))
