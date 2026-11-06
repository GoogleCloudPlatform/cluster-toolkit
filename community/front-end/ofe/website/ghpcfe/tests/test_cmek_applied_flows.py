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
"""What the cluster, Filestore and image flows record about encryption.

- Each generator stores the choice it emitted, not the box as typed, so a key
  forced by enforcement stays in later blueprints and is listed on the keys
  page, and a key enforcement refused is not added later.
- The choice is marked applied just before the first apply runs, so a
  failed first apply, a failure after the apply (reading its outputs) and a
  later failed start or reconfigure all leave it fixed; a key revoked in the
  meantime is refused and not marked. An image is marked once it is built.
- A resource deployed before the flag existed gets it stored when a start,
  reconfigure or create begins, so a failure there does not unfreeze it.
- A resource whose key was revoked cannot be started, changed or re-applied,
  but can still be destroyed.
- Start, reconfigure and image builds run only from the states they are for.
- An image blueprint that cannot be generated stops the build; a failed
  Packer build is recorded and the builder env is still destroyed.
- A key ring that cannot be checked is reported on the resource's page.
- The login node's use: list carries the key, not the filesystems.
"""
import asyncio
import json
import subprocess
import tempfile
from contextlib import ExitStack
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import yaml
from django.test import SimpleTestCase

from ..cluster_manager import cloud_info, cmek

CREDENTIAL = SimpleNamespace(detail=json.dumps({"project_id": "proj-abcde"}))
ENFORCEMENTS = ("disabled", "preferred", "required")


def _config(enforcement):
    return {"cmek": {"enforcement": enforcement, "key_ring_name": "ofe-cmek"},
            "server": {"gcs_bucket": "bucket"}}


class Row(SimpleNamespace):
    """A model row that records the fields each save() wrote.

    Nothing else writes to it, so re-reading it changes nothing.
    """

    def __init__(self, **kwargs):
        super().__init__(saves=[], refreshes=[], **kwargs)

    def save(self, update_fields=None):
        self.saves.append(update_fields)

    def refresh_from_db(self, fields=None):
        self.refreshes.append(fields)

    def wrote(self, field):
        """Whether any save so far wrote this field to the database."""
        return any(fields is None or field in fields for fields in self.saves)


def _cmek_fields(enabled, **extra):
    fields = dict(
        cmek_enabled=enabled, cmek_key_name="", cmek_key_ring_name="",
        cmek_key_project_id="", cmek_deletion_policy="ABANDON",
        cmek_protection_level="", cmek_rotation_period="",
        cmek_encryption_service_account="", cmek_applied=False,
        cmek_revoked=False, cloud_credential=CREDENTIAL, status_message="")
    fields.update(extra)
    return fields


def _cluster(enabled=False, status="n", cloud_state="nm", **extra):
    fields = dict(
        id=1, pk=1, name="c", cloud_id="c-abcd", project_id="proj-abcde",
        cloud_zone="europe-west4-a", cloud_region="europe-west4",
        status=status, cloud_state=cloud_state, use_bigquery=False,
        use_containers=False, use_cloudsql=False, enable_slurm_auth=True,
        subnet=SimpleNamespace(
            cloud_id="sub", vpc=SimpleNamespace(cloud_id="net")),
        controller_instance_type="n2-standard-2",
        controller_disk_type="pd-standard", controller_disk_size=120,
        num_login_nodes=0, login_node_instance_type="n2-standard-2",
        login_node_disk_type="pd-standard", login_node_disk_size=120,
        controller_node_image=None, login_node_image=None,
        mount_points=SimpleNamespace(order_by=lambda field: []),
        partitions=SimpleNamespace(all=lambda: []), shared_fs=None,
        grafana_dashboard_url="")
    fields.update(_cmek_fields(enabled))
    fields.update(extra)
    return Row(**fields)


def _filestore(enabled=False, cloud_state="nm", **extra):
    fields = dict(
        id=5, pk=5, name="home", cloud_zone="europe-west4-a",
        cloud_region="europe-west4", capacity=1024,
        vpc=SimpleNamespace(cloud_id="net"), cloud_state=cloud_state,
        exports=SimpleNamespace(first=lambda: SimpleNamespace(
            export_name="/share")),
        get_performance_tier_display=lambda: "ENTERPRISE")
    fields.update(_cmek_fields(enabled))
    fields.update(extra)
    return Row(**fields)


def _filestore_parent(child, cloud_state="nm"):
    """The Filesystem row a view or start_filesystem() is given."""
    from ..models import FilesystemImpl
    return Row(id=child.id, pk=child.pk, name=child.name,
               cloud_state=cloud_state, status_message="",
               impl_type=FilesystemImpl.GCPFILESTORE,
               cloud_credential=CREDENTIAL, gcpfilestorefilesystem=child)


def _image(enabled=False, status="n", **extra):
    fields = dict(
        id=7, pk=7, name="img", family="fam", cloud_zone="europe-west4-a",
        cloud_region="europe-west4", source_image_project="p",
        source_image_family="f", enable_os_login="TRUE",
        block_project_ssh_keys="TRUE", status=status,
        startup_script=SimpleNamespace(all=lambda: []))
    fields.update(_cmek_fields(enabled))
    fields.update(extra)
    return Row(**fields)


def _cluster_info(cluster, enforcement="preferred"):
    from django.template import engines
    from ..cluster_manager import clusterinfo
    info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
    info.config = _config(enforcement)
    info.cluster = cluster
    info.cluster_dir = Path(tempfile.mkdtemp())
    info.env = engines["django"]
    return info


def _image_backend(img, enforcement="preferred"):
    from ..cluster_manager import image
    backend = image.ImageBackend.__new__(image.ImageBackend)
    backend.config = _config(enforcement)
    backend.image = img
    backend.image_dir = Path(tempfile.mkdtemp())
    backend.blueprint_name = f"image_{img.id}"
    backend.credentials_file = backend.image_dir / "cloud_credentials"
    backend._cmek_in_use = False
    return backend


# Each generator, run once: returns whether the blueprint carries a key.

def _generate_cluster(cluster, enforcement):
    info = _cluster_info(cluster, enforcement)
    with mock.patch.object(cmek, "ring_already_exists", return_value=True):
        return cmek.KEY_IAM_MODULE_ID in info._cmek_modules_yaml()


def _generate_filestore(fs, enforcement):
    from ..cluster_manager import filesystem
    target = Path(tempfile.mkdtemp())
    with mock.patch.object(filesystem.utils, "load_config",
                           return_value=_config(enforcement)), \
            mock.patch.object(cmek, "ring_already_exists",
                              return_value=True):
        filesystem.write_filestore_yaml(fs, target)
    return cmek.KEY_IAM_MODULE_ID in (target / "filesystem.yaml").read_text()


def _generate_image(img, enforcement):
    backend = _image_backend(img, enforcement)
    with mock.patch.object(cmek, "ring_already_exists", return_value=True):
        backend._create_blueprint()
    return cmek.KEY_IAM_MODULE_ID in (
        backend.image_dir / "image.yaml").read_text()


def _deploy_cluster(cluster):
    cluster.status, cluster.cloud_state = "r", "m"


def _deploy_filestore(fs):
    fs.cloud_state = "m"


def _deploy_image(img):
    # an image has no deployed state of its own: only the flag freezes it
    img.cmek_applied = True


KINDS = {
    "cluster": (_cluster, _generate_cluster, _deploy_cluster),
    "filestore": (_filestore, _generate_filestore, _deploy_filestore),
    "image": (_image, _generate_image, _deploy_image),
}


class EffectiveChoiceIsStoredTests(SimpleTestCase):
    """The generators store the choice they emitted."""

    def test_a_key_forced_by_enforcement_is_stored_and_kept(self):
        for kind, (make, generate, deploy) in KINDS.items():
            with self.subTest(kind=kind):
                resource = make(enabled=False)
                self.assertTrue(generate(resource, "required"))
                self.assertIs(resource.cmek_enabled, True)
                # written to the row, so the keys page, which lists rows
                # with cmek_enabled=True, lists this key and can revoke it
                self.assertTrue(resource.wrote("cmek_enabled"))
                deploy(resource)
                for enforcement in ENFORCEMENTS:
                    with self.subTest(after_deploy=enforcement):
                        self.assertTrue(generate(resource, enforcement))
                        self.assertIs(resource.cmek_enabled, True)

    def test_a_key_refused_by_enforcement_is_not_added_later(self):
        for kind, (make, generate, deploy) in KINDS.items():
            with self.subTest(kind=kind):
                resource = make(enabled=True)
                self.assertFalse(generate(resource, "disabled"))
                self.assertIs(resource.cmek_enabled, False)
                self.assertTrue(resource.wrote("cmek_enabled"))
                deploy(resource)
                for enforcement in ENFORCEMENTS:
                    with self.subTest(after_deploy=enforcement):
                        self.assertFalse(generate(resource, enforcement))
                        self.assertIs(resource.cmek_enabled, False)

    def test_a_frozen_resource_writes_nothing_new(self):
        for kind, (make, generate, deploy) in KINDS.items():
            with self.subTest(kind=kind):
                resource = make(enabled=False)
                deploy(resource)
                self.assertFalse(generate(resource, "required"))
                self.assertEqual(resource.saves, [])

    def test_an_undeployed_resource_edited_again_follows_enforcement(self):
        for kind, (make, generate, _deploy) in KINDS.items():
            with self.subTest(kind=kind):
                resource = make(enabled=False)
                self.assertTrue(generate(resource, "required"))
                # edited again before any apply: the box is unticked and
                # enforcement relaxed, so the next generation has no key
                resource.cmek_enabled = False
                self.assertFalse(generate(resource, "preferred"))
                self.assertIs(resource.cmek_enabled, False)
                # and enforcement made stricter adds it back
                self.assertTrue(generate(resource, "required"))
                self.assertIs(resource.cmek_enabled, True)


def _cluster_terraform(stack, info, apply_error=None):
    """Patch what _apply_terraform() reaches beyond its own bookkeeping."""
    from ..cluster_manager import clusterinfo
    tf_dir = Path(tempfile.mkdtemp())
    (tf_dir / "terraform.tfstate").write_text("{}")

    def terraform(directory, command, **kwargs):
        if command == "apply" and apply_error:
            raise apply_error

    stack.enter_context(mock.patch.object(
        clusterinfo.c2, "create_cluster_subscription"))
    stack.enter_context(mock.patch.object(
        clusterinfo.utils, "run_terraform", side_effect=terraform))
    stack.enter_context(mock.patch.object(
        clusterinfo.grafana, "create_cluster_dashboard",
        return_value={"url": ""}))
    for name, value in (("get_terraform_dir", tf_dir),
                        ("_get_credentials_file", "/dev/null"),
                        ("_create_model_instances_from_tf_state", []),
                        ("_get_service_accounts", [])):
        stack.enter_context(mock.patch.object(info, name,
                                              return_value=value))
    for name in ("extract_and_update_registry_info",
                 "_apply_service_account_permissions",
                 "_configure_spack_install_loc", "_create_cluster_dir",
                 "_set_credentials", "update", "_run_ghpc",
                 "_initialize_terraform"):
        stack.enter_context(mock.patch.object(info, name))


APPLY_FAILED = subprocess.CalledProcessError(1, "terraform apply")


class AppliedIsSetWhenTheApplyStartsTests(SimpleTestCase):
    """cmek_applied records the first apply, on or off, even a failed one."""

    def _cluster_run(self, method, apply_error=None, **fields):
        info = _cluster_info(_cluster(**fields))
        with ExitStack() as stack:
            _cluster_terraform(stack, info, apply_error)
            if apply_error:
                stack.enter_context(self.assertRaises(type(apply_error)))
                stack.enter_context(self.assertLogs(level="ERROR"))
            method(info)
        return info.cluster

    @staticmethod
    def _start(info):
        info.start_cluster("{}")

    @staticmethod
    def _reconfigure(info):
        info.reconfigure_cluster()

    def test_a_cluster_start_and_reconfigure(self):
        for method in (self._start, self._reconfigure):
            for enabled in (True, False):
                with self.subTest(method=method.__name__, enabled=enabled):
                    cluster = self._cluster_run(method, enabled=enabled)
                    self.assertIs(cluster.cmek_applied, True)
                    cluster = self._cluster_run(method, APPLY_FAILED,
                                                enabled=enabled)
                    self.assertIs(cluster.cmek_applied, True)

    def test_a_failed_reconfigure_after_an_apply_keeps_the_choice(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                info = _cluster_info(_cluster(enabled=enabled))
                with ExitStack() as stack:
                    _cluster_terraform(stack, info)
                    info.start_cluster("{}")
                cluster = info.cluster
                with ExitStack() as stack:
                    _cluster_terraform(stack, info, APPLY_FAILED)
                    with self.assertRaises(subprocess.CalledProcessError), \
                            self.assertLogs(level="ERROR"):
                        info.reconfigure_cluster()
                self.assertEqual((cluster.status, cluster.cloud_state),
                                 ("e", "nm"))
                self.assertIs(cluster.cmek_applied, True)
                for enforcement in ENFORCEMENTS:
                    settings = cmek.CmekSettings.from_config(
                        _config(enforcement))
                    self.assertEqual(
                        bool(cmek.resource_settings(cluster, settings)),
                        enabled)

    def _start_filestore(self, apply_error=None, output_error=None,
                         child=None):
        from ..cluster_manager import filesystem
        target = Path(tempfile.mkdtemp())
        (target / "filesystem.yaml").write_text("blueprint")
        out = target / "output.json"
        out.write_text(json.dumps(
            {"network_storage_home": {"value": {"server_ip": "10.0.0.2"}}}))

        def terraform(directory, command, **kwargs):
            if command == "apply" and apply_error:
                raise apply_error
            if command == "output" and output_error:
                raise output_error
            return (out, None)

        child = child or _filestore(enabled=True)
        parent = _filestore_parent(child)
        error = apply_error or output_error
        with ExitStack() as stack:
            for name, value in (("_base_dir_for_fs", target),
                                ("_tf_dir_for_fs", target),
                                ("_get_credentials_file", "/dev/null")):
                stack.enter_context(mock.patch.object(
                    filesystem, name, return_value=value))
            stack.enter_context(mock.patch.object(filesystem, "_run_ghpc"))
            stack.enter_context(mock.patch.object(
                filesystem.utils, "run_terraform", side_effect=terraform))
            if error:
                stack.enter_context(self.assertRaises(type(error)))
                stack.enter_context(self.assertLogs(level="ERROR"))
            filesystem.start_filesystem(parent)
        return parent, child

    def test_a_filestore_start(self):
        parent, child = self._start_filestore()
        self.assertEqual(parent.cloud_state, "m")
        # on the child row: a save of the parent does not write its columns
        self.assertIs(child.cmek_applied, True)
        self.assertIn(["cmek_applied"], child.saves)
        parent, child = self._start_filestore(APPLY_FAILED)
        self.assertEqual(parent.cloud_state, "nm")
        self.assertIs(child.cmek_applied, True)

    def _build_image(self, build_error=None):
        from ..cluster_manager import image

        def packer(directory, command, **kwargs):
            if command == "build" and build_error:
                raise build_error

        backend = _image_backend(_image(enabled=True, status="c"))
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(
                image.utils, "run_packer", side_effect=packer))
            if build_error:
                # recorded on the image, not raised: prepare() still has the
                # builder env to destroy
                stack.enter_context(self.assertLogs(level="ERROR"))
            backend._create_image()
        return backend.image

    def test_an_image_build(self):
        img = self._build_image()
        self.assertEqual(img.status, "r")
        self.assertIs(img.cmek_applied, True)
        self.assertIn(["cmek_applied"], img.saves)
        for error in (subprocess.CalledProcessError(1, "packer build"),
                      RuntimeError("Packer command failed: exit status 1")):
            with self.subTest(error=type(error).__name__):
                img = self._build_image(error)
                self.assertEqual(img.status, "e")
                self.assertEqual(img.status_message,
                                 f"The Packer build failed: {error}")
                self.assertIs(img.cmek_applied, False)


class AppliedIsStoredBeforeAnythingElseCanFailTests(SimpleTestCase):
    """A failure after a successful apply cannot lose the applied choice."""

    def test_a_cluster_whose_state_cannot_be_read(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                info = _cluster_info(_cluster(enabled=enabled))
                with ExitStack() as stack:
                    _cluster_terraform(stack, info)
                    stack.enter_context(mock.patch.object(
                        info, "extract_and_update_registry_info",
                        side_effect=RuntimeError("unreadable state")))
                    with self.assertRaises(RuntimeError):
                        info.start_cluster("{}")
                cluster = info.cluster
                self.assertEqual((cluster.status, cluster.cloud_state),
                                 ("e", "nm"))
                self.assertIs(cluster.cmek_applied, True)
                self.assertIn(["cmek_applied"], cluster.saves)
                cluster._cmek_deployed = False
                for enforcement in ENFORCEMENTS:
                    with self.subTest(enforcement=enforcement):
                        self.assertEqual(
                            _generate_cluster(cluster, enforcement), enabled)

    def test_a_filestore_whose_outputs_cannot_be_read(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                parent, child = AppliedIsSetWhenTheApplyStartsTests()\
                    ._start_filestore(
                        output_error=subprocess.CalledProcessError(
                            1, "terraform output"),
                        child=_filestore(enabled=enabled))
                self.assertEqual(parent.cloud_state, "nm")
                self.assertIs(child.cmek_applied, True)
                self.assertIn(["cmek_applied"], child.saves)
                for enforcement in ENFORCEMENTS:
                    with self.subTest(enforcement=enforcement):
                        self.assertEqual(
                            _generate_filestore(child, enforcement), enabled)

    def test_an_image_whose_status_cannot_be_saved(self):
        from ..cluster_manager import image
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                backend = _image_backend(_image(enabled=enabled, status="c"))
                with mock.patch.object(image.utils, "run_packer"), \
                        mock.patch.object(
                            backend, "update_image_status",
                            side_effect=RuntimeError("database is locked")), \
                        self.assertLogs(level="ERROR"):
                    backend._create_image()
                img = backend.image
                self.assertEqual(img.status, "e")
                self.assertIn(["cmek_applied"], img.saves)
                for enforcement in ENFORCEMENTS:
                    with self.subTest(enforcement=enforcement):
                        self.assertEqual(
                            _generate_image(img, enforcement), enabled)


class FailedFirstApplyLocksTheChoiceTests(SimpleTestCase):
    """A first apply that fails can leave encrypted (or unencrypted) disks,
    buckets, a share or the key behind, so a retry must apply the same
    choice whatever the enforcement has become."""

    def test_a_cluster_retry_generates_the_same_choice(self):
        from ..cluster_manager import clusterinfo
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                info = _cluster_info(_cluster(enabled=enabled))
                seen = []

                def terraform(directory, command, **kwargs):
                    if command == "apply":
                        # already stored, on its own, when the apply runs
                        seen.append((info.cluster.cmek_applied,
                                     ["cmek_applied"] in info.cluster.saves))
                        raise APPLY_FAILED

                with ExitStack() as stack:
                    _cluster_terraform(stack, info)
                    stack.enter_context(mock.patch.object(
                        clusterinfo.utils, "run_terraform",
                        side_effect=terraform))
                    with self.assertRaises(subprocess.CalledProcessError), \
                            self.assertLogs(level="ERROR"):
                        info.start_cluster("{}")
                cluster = info.cluster
                self.assertEqual(seen, [(True, True)])
                self.assertEqual((cluster.status, cluster.cloud_state),
                                 ("e", "nm"))
                # what the Retry loads: the stored flag, no snapshot
                cluster._cmek_deployed = False
                self.assertIs(cmek.is_frozen(cluster), True)
                for enforcement in ("required", "disabled"):
                    with self.subTest(retry_under=enforcement):
                        self.assertEqual(
                            _generate_cluster(cluster, enforcement), enabled)
                        self.assertIs(cluster.cmek_enabled, enabled)

    def test_a_filestore_retry_generates_the_same_choice(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                parent, child = AppliedIsSetWhenTheApplyStartsTests()\
                    ._start_filestore(APPLY_FAILED,
                                      child=_filestore(enabled=enabled))
                self.assertEqual(parent.cloud_state, "nm")
                self.assertIs(child.cmek_applied, True)
                self.assertIn(["cmek_applied"], child.saves)
                self.assertIs(cmek.is_frozen(child), True)
                for enforcement in ("required", "disabled"):
                    with self.subTest(retry_under=enforcement):
                        self.assertEqual(
                            _generate_filestore(child, enforcement), enabled)
                        self.assertIs(child.cmek_enabled, enabled)

    def test_a_cluster_revoked_before_the_apply_is_not_marked(self):
        from ..cluster_manager import clusterinfo
        # revoked after the start's own check, before the apply
        info = _cluster_info(_cluster(enabled=True, cmek_revoked=True))
        with ExitStack() as stack:
            _cluster_terraform(stack, info)
            with self.assertRaises(cmek.CmekConfigError):
                info._apply_terraform()
            commands = [c.args[1] for c in
                        clusterinfo.utils.run_terraform.call_args_list]
        self.assertNotIn("apply", commands)
        self.assertIs(info.cluster.cmek_applied, False)
        self.assertFalse(info.cluster.wrote("cmek_applied"))

    def test_a_filestore_revoked_before_the_apply_is_not_marked(self):
        from ..cluster_manager import filesystem
        target = Path(tempfile.mkdtemp())
        (target / "filesystem.yaml").write_text("blueprint")
        child = _filestore(enabled=True)
        parent = _filestore_parent(child)
        commands = []

        def terraform(directory, command, **kwargs):
            commands.append(command)
            if command == "plan":
                # an administrator revokes the key while the plan runs
                child.cmek_revoked = True

        with ExitStack() as stack:
            for name, value in (("_base_dir_for_fs", target),
                                ("_tf_dir_for_fs", target),
                                ("_get_credentials_file", "/dev/null")):
                stack.enter_context(mock.patch.object(
                    filesystem, name, return_value=value))
            stack.enter_context(mock.patch.object(filesystem, "_run_ghpc"))
            stack.enter_context(mock.patch.object(
                filesystem.utils, "run_terraform", side_effect=terraform))
            stack.enter_context(self.assertRaises(cmek.CmekConfigError))
            stack.enter_context(self.assertLogs(level="ERROR"))
            filesystem.start_filesystem(parent)
        self.assertNotIn("apply", commands)
        self.assertIs(child.cmek_applied, False)
        self.assertFalse(child.wrote("cmek_applied"))
        self.assertIn("was revoked", parent.status_message)


class LegacyDeployedResourcesStayFrozenTests(SimpleTestCase):
    """Deployed before cmek_applied existed: frozen only by their state.

    The flag is stored when a flow that can reset that state begins, so a
    failure in it leaves the resource frozen.
    """

    def assertFrozen(self, resource, generate, enabled):
        # what a later request loads: the flag, not this flow's snapshot
        resource._cmek_deployed = False
        self.assertIs(resource.cmek_applied, True)
        self.assertIn(["cmek_applied"], resource.saves)
        self.assertIs(cmek.is_frozen(resource), True)
        for enforcement in ENFORCEMENTS:
            with self.subTest(enforcement=enforcement):
                self.assertEqual(generate(resource, enforcement), enabled)

    def test_a_failed_reconfigure(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                info = _cluster_info(_cluster(
                    enabled=enabled, status="r", cloud_state="m"))
                # as BackendReconfigureCluster runs it
                with mock.patch.object(info, "_prepare_ghpc_yaml"), \
                        mock.patch.object(info, "_prepare_bootstrap_gcs"):
                    info.update()
                with ExitStack() as stack:
                    _cluster_terraform(stack, info, APPLY_FAILED)
                    with self.assertRaises(subprocess.CalledProcessError), \
                            self.assertLogs(level="ERROR"):
                        info.reconfigure_cluster()
                cluster = info.cluster
                self.assertEqual((cluster.status, cluster.cloud_state),
                                 ("e", "nm"))
                self.assertFrozen(cluster, _generate_cluster, enabled)

    def test_a_failed_start(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                info = _cluster_info(_cluster(
                    enabled=enabled, status="e", cloud_state="m"))
                with ExitStack() as stack:
                    _cluster_terraform(stack, info, APPLY_FAILED)
                    with self.assertRaises(subprocess.CalledProcessError), \
                            self.assertLogs(level="ERROR"):
                        info.start_cluster("{}")
                cluster = info.cluster
                self.assertEqual((cluster.status, cluster.cloud_state),
                                 ("e", "nm"))
                self.assertFrozen(cluster, _generate_cluster, enabled)

    def test_a_filestore_create_that_fails(self):
        from ..cluster_manager import filesystem
        from ..views import filesystems as v
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                child = _filestore(enabled=enabled, cloud_state="m")
                parent = _filestore_parent(child, cloud_state="m")
                with mock.patch.object(
                        filesystem, "create_filesystem",
                        side_effect=RuntimeError("cannot write")), \
                        self.assertRaises(RuntimeError):
                    v.BackendCreateFilesystem().cmd(None, None, parent)
                # a later failed start leaves the record "nm"
                child.cloud_state = "nm"
                self.assertFrozen(child, _generate_filestore, enabled)

    def test_a_filestore_start_that_fails(self):
        for enabled in (True, False):
            with self.subTest(enabled=enabled):
                parent, child = AppliedIsSetWhenTheApplyStartsTests()\
                    ._start_filestore(APPLY_FAILED, child=_filestore(
                        enabled=enabled, cloud_state="m"))
                self.assertEqual(parent.cloud_state, "nm")
                child.cloud_state = "nm"
                self.assertFrozen(child, _generate_filestore, enabled)

    def test_a_resource_never_deployed_is_not_marked(self):
        info = _cluster_info(_cluster(status="n", cloud_state="nm"))
        with mock.patch.object(info, "_prepare_ghpc_yaml"), \
                mock.patch.object(info, "_prepare_bootstrap_gcs"):
            info.update()
        self.assertIs(info.cluster.cmek_applied, False)
        self.assertEqual(info.cluster.saves, [])


class RevokedIsRefusedTests(SimpleTestCase):
    """A revoked resource is refused everywhere but teardown."""

    def assertRefused(self, resource):
        self.assertIn("was revoked", resource.status_message)

    def test_cluster_update(self):
        info = _cluster_info(_cluster(cmek_revoked=True))
        with mock.patch.object(info, "_prepare_ghpc_yaml") as generate, \
                self.assertRaises(cmek.CmekConfigError):
            info.update()
        generate.assert_not_called()

    def test_cluster_start_leaves_the_record_as_it_was(self):
        info = _cluster_info(_cluster(status="e", cloud_state="m",
                                      cmek_revoked=True))
        with mock.patch.object(info, "_create_cluster_dir") as create, \
                self.assertRaises(cmek.CmekConfigError):
            info.start_cluster("{}")
        create.assert_not_called()
        cluster = info.cluster
        self.assertEqual((cluster.status, cluster.cloud_state), ("e", "m"))
        self.assertTrue(cluster.status_message.startswith("Start failed: "))
        self.assertRefused(cluster)
        self.assertEqual(cluster.saves, [["status_message"]])

    def test_cluster_reconfigure_says_why(self):
        from ..cluster_manager import clusterinfo
        from ..views import clusters as v
        cluster = _cluster(status="r", cloud_state="m", cmek_revoked=True)
        with mock.patch.object(clusterinfo.utils, "load_config",
                               return_value={"baseDir": Path("/nonexistent")}), \
                mock.patch.object(clusterinfo.ClusterInfo,
                                  "_prepare_ghpc_yaml") as generate, \
                mock.patch.object(clusterinfo.ClusterInfo,
                                  "reconfigure_cluster") as apply, \
                self.assertRaises(cmek.CmekConfigError):
            v.BackendReconfigureCluster().cmd(None, None, cluster)
        generate.assert_not_called()
        apply.assert_not_called()
        self.assertTrue(
            cluster.status_message.startswith("Reconfigure failed: "))
        self.assertRefused(cluster)
        self.assertEqual((cluster.status, cluster.cloud_state), ("r", "m"))

    def _update_form(self, revoked):
        from ..views import clusters as v
        view = v.ClusterUpdateView()
        view.object = mock.MagicMock(status="r", cmek_revoked=revoked)
        form = mock.MagicMock()
        form.instance.use_containers = False
        with mock.patch.object(view, "get_context_data", return_value={
                "mountpoints_formset": mock.MagicMock(),
                "cluster_partitions_formset": mock.MagicMock(),
                "container_registry_formset": mock.MagicMock(forms=[])}), \
                mock.patch.object(v.cloud_info, "get_machine_types",
                                  return_value={}), \
                mock.patch.object(v.cloud_info, "get_disk_types",
                                  return_value=[]), \
                mock.patch.object(view, "form_invalid",
                                  return_value="invalid"):
            self.assertEqual(view.form_valid(form), "invalid")
        return [c.args for c in form.add_error.call_args_list]

    def test_the_cluster_update_form(self):
        errors = self._update_form(revoked=True)
        self.assertEqual(len(errors), 1)
        self.assertIsNone(errors[0][0])
        self.assertIn("was revoked", errors[0][1])
        # not revoked: past that check, to the next one (no disk types here)
        self.assertEqual(self._update_form(revoked=False),
                         [("controller_disk_type", "Invalid Disk Type")])

    def test_cluster_reconfigure_itself(self):
        # BackendReconfigureCluster refuses first; this is the second check
        info = _cluster_info(_cluster(status="r", cloud_state="m",
                                      cmek_revoked=True))
        with mock.patch.object(info, "_run_ghpc") as run_ghpc, \
                mock.patch.object(info, "_apply_terraform") as apply, \
                self.assertRaises(cmek.CmekConfigError):
            info.reconfigure_cluster()
        run_ghpc.assert_not_called()
        apply.assert_not_called()
        self.assertEqual((info.cluster.status, info.cluster.cloud_state),
                         ("r", "m"))
        self.assertEqual(info.cluster.saves, [])

    def _edit_filestore(self, revoked):
        from ..views import gcpfilestore as v
        view = v.GCPFilestoreFilesystemUpdateView()
        form = mock.MagicMock()
        form.instance = _filestore(enabled=True, cloud_state="m",
                                   cmek_revoked=revoked)
        with mock.patch.object(view, "form_invalid",
                               return_value="invalid"), \
                mock.patch.object(view, "get_success_url",
                                  return_value="/done"):
            response = view.form_valid(form)
        return response, form

    def test_the_filestore_edit_form(self):
        response, form = self._edit_filestore(revoked=True)
        self.assertEqual(response, "invalid")
        form.save.assert_not_called()
        self.assertEqual(len(form.add_error.call_args_list), 1)
        field, message = form.add_error.call_args.args
        self.assertIsNone(field)
        self.assertIn("was revoked", message)
        # not revoked: saved as before
        response, form = self._edit_filestore(revoked=False)
        self.assertEqual(response.status_code, 302)
        form.save.assert_called_once_with(commit=False)
        form.add_error.assert_not_called()

    def test_filestore_update_files_says_why(self):
        from ..cluster_manager import filesystem
        from ..views import filesystems as v
        child = _filestore(enabled=True, cloud_state="m", cmek_revoked=True)
        parent = _filestore_parent(child, cloud_state="m")
        with mock.patch.object(filesystem, "create_filesystem") as create, \
                self.assertRaises(cmek.CmekConfigError):
            v.BackendUpdateFilesystem().cmd(None, None, parent)
        create.assert_not_called()
        self.assertEqual(parent.cloud_state, "m")
        self.assertRefused(parent)
        self.assertEqual(parent.saves, [["status_message"]])

    def test_filestore_generation(self):
        fs = _filestore(enabled=True, cmek_revoked=True)
        with self.assertRaises(cmek.CmekConfigError):
            _generate_filestore(fs, "preferred")
        self.assertEqual(fs.saves, [])

    def test_filestore_create_keeps_a_deployed_filestore_deployed(self):
        from ..cluster_manager import filesystem
        from ..views import filesystems as v
        child = _filestore(enabled=True, cloud_state="m", cmek_revoked=True)
        parent = _filestore_parent(child, cloud_state="m")
        with mock.patch.object(filesystem, "create_filesystem") as create, \
                self.assertRaises(cmek.CmekConfigError):
            v.BackendCreateFilesystem().cmd(None, None, parent)
        create.assert_not_called()
        # still "m", so the page keeps offering Destroy
        self.assertEqual(parent.cloud_state, "m")
        self.assertRefused(parent)
        self.assertEqual(parent.saves, [["status_message"]])

    def test_filestore_start(self):
        from ..cluster_manager import filesystem
        child = _filestore(enabled=True, cloud_state="m", cmek_revoked=True)
        parent = _filestore_parent(child, cloud_state="m")
        with mock.patch.object(filesystem, "_run_ghpc") as run_ghpc, \
                self.assertRaises(cmek.CmekConfigError):
            filesystem.start_filesystem(parent)
        run_ghpc.assert_not_called()
        self.assertEqual(parent.cloud_state, "m")
        self.assertRefused(parent)

    def test_image_build(self):
        backend = _image_backend(_image(enabled=True, cmek_revoked=True))
        with mock.patch.object(backend, "_create_image_dir") as create, \
                self.assertRaises(cmek.CmekConfigError):
            backend.prepare()
        create.assert_not_called()
        self.assertEqual(backend.image.status, "n")
        self.assertRefused(backend.image)

    def test_teardown_is_not_refused(self):
        from ..cluster_manager import clusterinfo, filesystem, image

        class Reached(Exception):
            pass

        info = _cluster_info(_cluster(status="r", cloud_state="m",
                                      cmek_revoked=True))
        tf_dir = Path(tempfile.mkdtemp())
        (tf_dir / ".terraform").mkdir()
        with mock.patch.object(clusterinfo.utils, "run_terraform",
                               side_effect=Reached) as terraform, \
                mock.patch.object(info, "get_terraform_dir",
                                  return_value=tf_dir), \
                mock.patch.object(info, "_get_credentials_file"), \
                self.assertRaises(Reached):
            info.stop_cluster()
        self.assertEqual(terraform.call_args.args[1], "destroy")

        parent = _filestore_parent(
            _filestore(enabled=True, cloud_state="m", cmek_revoked=True),
            cloud_state="m")
        with mock.patch.object(filesystem, "_tf_dir_for_fs"), \
                mock.patch.object(filesystem, "_get_credentials_file"), \
                mock.patch.object(filesystem.utils,
                                  "run_terraform") as terraform:
            filesystem.destroy_filesystem(parent)
        self.assertEqual(terraform.call_args.args[1], "destroy")

        backend = _image_backend(_image(enabled=True, status="r",
                                        cmek_revoked=True))
        with mock.patch.object(image.compute_v1, "ImagesClient") as client, \
                mock.patch.dict(image.os.environ):
            self.assertIs(backend.delete_image(), True)
        client.return_value.delete.assert_called_once()


class StatusGuardTests(SimpleTestCase):
    """Hand-typed backend URLs run only from the state they are for."""

    def _get(self, view, model, row):
        created = mock.AsyncMock()
        with mock.patch.object(model.objects, "get", return_value=row), \
                mock.patch.object(view, "test_user_is_cluster_admin",
                                  mock.AsyncMock()), \
                mock.patch.object(view, "create_task", created), \
                mock.patch("django.contrib.messages.add_message") as message:
            view.request = mock.Mock()
            response = asyncio.run(view.get(view.request, 3))
        return response, created, message

    def _check(self, make_view, model, allowed, refused, make_row):
        for status in allowed + refused:
            with self.subTest(view=make_view.__name__, status=status):
                row = make_row(status)
                response, created, message = self._get(
                    make_view(), model, row)
                self.assertEqual(response.status_code, 302)
                if status in allowed:
                    created.assert_awaited_once()
                    message.assert_not_called()
                else:
                    created.assert_not_awaited()
                    message.assert_called_once()
                    # nothing else about the row was recorded
                    row.save.assert_not_called()

    def test_start_and_reconfigure_a_cluster(self):
        from ..views import clusters as v

        def make_row(status):
            return mock.MagicMock(status=status)

        self._check(v.BackendStartCluster, v.Cluster, ("n", "e"),
                    ("c", "i", "r", "re", "s", "t", "d"), make_row)
        self._check(v.BackendReconfigureCluster, v.Cluster, ("r",),
                    ("n", "c", "i", "re", "s", "t", "e", "d"), make_row)

    def test_build_an_image(self):
        from ..views import images as v

        def make_row(status):
            return mock.MagicMock(status=status)

        def claim(pk, status):
            # the claim updates the row only while it is still new
            return mock.Mock(update=mock.Mock(
                return_value=int(rows[-1].status == status)))

        rows = []

        def make_claimed_row(status):
            rows.append(make_row(status))
            return rows[-1]

        with mock.patch.object(v.Image.objects, "filter",
                               side_effect=claim):
            self._check(v.BackendCreateImage, v.Image, ("n",),
                        ("c", "r", "e"), make_claimed_row)


class ImageBlueprintFailureStopsTheBuildTests(SimpleTestCase):
    """A blueprint that cannot be generated ends the build there."""

    STEPS = ("_run_ghpc", "_create_cmek_env", "_create_builder_env",
             "_create_image", "_destroy_builder_env")

    def _prepare(self, backend, ring_error=None):
        with ExitStack() as stack:
            steps = {name: stack.enter_context(
                mock.patch.object(backend, name)) for name in self.STEPS}
            stack.enter_context(mock.patch.object(
                cmek, "ring_already_exists", return_value=True,
                side_effect=ring_error))
            if ring_error:
                stack.enter_context(self.assertRaises(type(ring_error)))
                stack.enter_context(self.assertLogs(level="ERROR"))
            backend.prepare()
        return steps

    def test_a_generation_error_stops_before_any_step(self):
        backend = _image_backend(_image(enabled=True))
        stale = backend.image_dir / "image.yaml"
        stale.write_text("from a previous attempt")
        error = cloud_info.KeyRingCheckError("Cannot check the ring", 403)
        steps = self._prepare(backend, error)
        for name, step in steps.items():
            with self.subTest(step=name):
                step.assert_not_called()
        self.assertEqual(backend.image.status, "e")
        self.assertEqual(
            backend.image.status_message,
            "Could not generate the image blueprint: Cannot check the ring")
        self.assertFalse(stale.exists())
        self.assertIs(backend._cmek_in_use, False)

    def test_a_failed_packer_build_still_destroys_the_builder_env(self):
        from ..cluster_manager import image
        for error in (subprocess.CalledProcessError(1, "packer build"),
                      RuntimeError("Packer command failed: exit status 1")):
            with self.subTest(error=type(error).__name__):
                backend = _image_backend(_image(enabled=True))
                steps = ("_run_ghpc", "_create_cmek_env",
                         "_create_builder_env", "_destroy_builder_env")
                with ExitStack() as stack:
                    mocks = {name: stack.enter_context(
                        mock.patch.object(backend, name)) for name in steps}
                    stack.enter_context(mock.patch.object(
                        cmek, "ring_already_exists", return_value=True))
                    stack.enter_context(mock.patch.object(
                        image.utils, "run_packer", side_effect=error))
                    stack.enter_context(self.assertLogs(level="ERROR"))
                    backend.prepare()
                mocks["_destroy_builder_env"].assert_called_once()
                self.assertEqual(backend.image.status, "e")
                self.assertEqual(backend.image.status_message,
                                 f"The Packer build failed: {error}")

    def test_a_directory_error_stops_the_build(self):
        backend = _image_backend(_image())
        # a file where the directory should be
        backend.image_dir.rmdir()
        backend.image_dir.write_text("")
        with mock.patch.object(backend, "_create_blueprint") as blueprint, \
                self.assertRaises(OSError), self.assertLogs(level="ERROR"):
            backend.prepare()
        blueprint.assert_not_called()
        self.assertEqual(backend.image.status, "e")
        self.assertTrue(backend.image.status_message.startswith(
            "Could not create the image directory: "))

    def test_a_successful_run_clears_the_last_reason(self):
        backend = _image_backend(_image(
            enabled=True, status_message="Could not generate it"))
        steps = self._prepare(backend)
        for name, step in steps.items():
            with self.subTest(step=name):
                step.assert_called_once()
        self.assertEqual(backend.image.status_message, "")
        self.assertIn(cmek.KEY_IAM_MODULE_ID,
                      (backend.image_dir / "image.yaml").read_text())


class KeyRingCheckErrorIsShownTests(SimpleTestCase):
    """A ring that cannot be checked is said on the resource's own page."""

    ERROR = cloud_info.KeyRingCheckError("Cannot check the ring", 403)

    def test_filestore_create(self):
        from ..cluster_manager import filesystem
        from ..views import filesystems as v
        target = Path(tempfile.mkdtemp())
        parent = _filestore_parent(_filestore(enabled=True))
        with mock.patch.object(filesystem.utils, "load_config",
                               return_value=_config("preferred")), \
                mock.patch.object(filesystem, "_base_dir_for_fs",
                                  return_value=target), \
                mock.patch.object(filesystem, "_get_credentials_file",
                                  return_value=target / "credentials"), \
                mock.patch.object(cmek, "ring_already_exists",
                                  side_effect=self.ERROR), \
                self.assertRaises(cloud_info.KeyRingCheckError):
            v.BackendCreateFilesystem().cmd(None, None, parent)
        self.assertEqual(parent.cloud_state, "nm")
        self.assertIn("Cannot check the ring", parent.status_message)
        self.assertFalse((target / "filesystem.yaml").exists())

    def _filestore_files(self, view, cloud_state, config=None, **patches):
        from ..cluster_manager import filesystem
        target = Path(tempfile.mkdtemp())
        parent = _filestore_parent(
            _filestore(enabled=True, cloud_state=cloud_state),
            cloud_state=cloud_state)
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(
                filesystem.utils, "load_config",
                return_value=config or _config("preferred")))
            stack.enter_context(mock.patch.object(
                filesystem, "_base_dir_for_fs", return_value=target))
            stack.enter_context(mock.patch.object(
                filesystem, "_get_credentials_file",
                return_value=target / "credentials"))
            stack.enter_context(mock.patch.object(
                cmek, "ring_already_exists", **patches))
            stack.enter_context(self.assertRaises(Exception))
            view().cmd(None, None, parent)
        self.assertFalse((target / "filesystem.yaml").exists())
        return parent

    def test_filestore_create_on_a_deployed_filestore(self):
        from ..views import filesystems as v
        # a create-files URL typed by hand for a Filestore that is deployed:
        # nothing was generated, so it keeps its state and its Destroy button
        for state in ("m", "i"):
            with self.subTest(cloud_state=state):
                parent = self._filestore_files(
                    v.BackendCreateFilesystem, state, side_effect=self.ERROR)
                self.assertEqual(parent.cloud_state, state)
                self.assertEqual(
                    parent.status_message,
                    "Could not generate the blueprint: Cannot check the ring")

    def test_filestore_update_files(self):
        from ..views import filesystems as v
        parent = self._filestore_files(
            v.BackendUpdateFilesystem, "m", side_effect=self.ERROR)
        self.assertEqual(parent.cloud_state, "m")
        self.assertEqual(
            parent.status_message,
            "Could not generate the blueprint: Cannot check the ring")
        self.assertEqual(parent.saves, [["status_message"]])

    def test_filestore_update_files_with_invalid_settings(self):
        from ..views import filesystems as v
        config = {"cmek": {"key_project_id": 123456789012}}
        # never deployed, so the settings decide its key
        parent = self._filestore_files(
            v.BackendUpdateFilesystem, "nm", config=config,
            return_value=False)
        self.assertEqual(parent.cloud_state, "nm")
        self.assertEqual(
            parent.status_message,
            "Could not generate the blueprint: cmek.key_project_id must be "
            "text, not a number: quote it in configuration.yaml")

    def test_cluster_start(self):
        info = _cluster_info(_cluster(enabled=True, status="e"))
        with mock.patch.object(info, "_create_cluster_dir"), \
                mock.patch.object(info, "_set_credentials"), \
                mock.patch.object(info, "_prepare_bootstrap_gcs"), \
                mock.patch.object(info, "_run_ghpc") as run_ghpc, \
                mock.patch.object(cmek, "ring_already_exists",
                                  side_effect=self.ERROR), \
                self.assertLogs(level="ERROR"), \
                self.assertRaises(cloud_info.KeyRingCheckError):
            info.start_cluster("{}")
        run_ghpc.assert_not_called()
        cluster = info.cluster
        self.assertEqual((cluster.status, cluster.cloud_state), ("e", "nm"))
        self.assertEqual(cluster.status_message,
                         "Start failed: Cannot check the ring")


class LoginNodeUsesOnlyTheKeyTests(SimpleTestCase):
    """The login module takes no network_storage, so no filesystem in use:."""

    def _login_block(self, enabled):
        export = SimpleNamespace(export_name="/home", server_name="10.0.0.2")
        mount = SimpleNamespace(id=4, export=export, mount_path="/home",
                                mount_options="defaults", fstype_name="nfs")
        cluster = _cluster(
            enabled=enabled,
            mount_points=SimpleNamespace(order_by=lambda field: [mount]),
            shared_fs=SimpleNamespace(exports=SimpleNamespace(
                all=lambda: [])))
        info = _cluster_info(cluster)
        with mock.patch.object(cmek, "ring_already_exists",
                               return_value=True):
            info._prepare_ghpc_yaml()
        blueprint = yaml.safe_load(
            (info.cluster_dir / "cluster.yaml").read_text())
        found = {}
        for group in blueprint["deployment_groups"]:
            for module in group["modules"]:
                found[module["id"]] = module
        self.assertIn("mount_num_4", found)
        self.assertIn("mount_num_4", found["slurm_controller"]["use"])
        return found["slurm_login"]

    def test_an_encrypted_cluster_with_a_filestore(self):
        login = self._login_block(enabled=True)
        self.assertEqual(login["use"], [cmek.KEY_IAM_MODULE_ID])

    def test_an_unencrypted_cluster_with_a_filestore(self):
        login = self._login_block(enabled=False)
        self.assertNotIn("use", login)
