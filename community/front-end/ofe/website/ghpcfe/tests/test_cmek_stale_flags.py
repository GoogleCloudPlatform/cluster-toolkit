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
"""A key revoked while a long flow runs stays revoked.

Start, reconfigure, destroy, a Filestore start and an image build load their
record once and save all of it when they finish, minutes later. Revoking a key
in between writes `cmek_revoked` through another instance, so without a fresh
read the flow's final save wrote the old value back and un-revoked the key.
The `cmek_applied` the flow itself stored must survive that read too.
"""
from contextlib import ExitStack
from types import SimpleNamespace
from unittest import mock

from django.core.exceptions import ObjectDoesNotExist
from django.test import SimpleTestCase

from ..cluster_manager import cmek
from .test_cmek_applied_flows import (
    APPLY_FAILED, _cluster, _cluster_info, _cluster_terraform, _filestore,
    _image, _image_backend,
)

FLAGS = ("cmek_revoked", "cmek_applied")
# what a full save() writes, of the columns these tests look at
COLUMNS = FLAGS + ("status", "cloud_state", "status_message")


class StoredRow(SimpleNamespace):
    """A row whose stored columns live in `db`, shared with other instances.

    save() writes what this instance holds, all of it or the fields named;
    refresh_from_db() reads the named fields back, as Django does.
    """

    def __init__(self, db, **fields):
        super().__init__(**fields)
        self._db = db
        db.update({name: fields[name] for name in COLUMNS if name in fields})

    def save(self, update_fields=None):
        for name in update_fields or COLUMNS:
            if hasattr(self, name):
                self._db[name] = getattr(self, name)

    def refresh_from_db(self, fields=None):
        for name in fields or COLUMNS:
            if name in self._db:
                setattr(self, name, self._db[name])


def stored(row, db):
    """The same row, backed by `db`."""
    fields = {k: v for k, v in vars(row).items()
              if k not in ("saves", "refreshes")}
    return StoredRow(db, **fields)


def revoke_during(command, db, error=None):
    """A run_terraform/run_packer stand-in: while `command` runs, an
    administrator revokes the key, as views/cmek.py records it (a targeted
    save through its own instance)."""
    def run(directory, cmd, **kwargs):
        if cmd == command:
            db["cmek_revoked"] = True
            if error:
                raise error
    return run


def just_run(directory, cmd, **kwargs):
    """The same stand-in with no revoke."""


class ClusterFlowsTests(SimpleTestCase):

    def _run(self, method, cluster, terraform, error=None):
        from ..cluster_manager import clusterinfo
        info = _cluster_info(cluster)
        with ExitStack() as stack:
            _cluster_terraform(stack, info)
            stack.enter_context(mock.patch.object(
                clusterinfo.utils, "run_terraform", side_effect=terraform))
            if error:
                stack.enter_context(self.assertRaises(type(error)))
                stack.enter_context(self.assertLogs(level="ERROR"))
            method(info)
        return info.cluster

    @staticmethod
    def _start(info):
        info.start_cluster("{}")

    @staticmethod
    def _reconfigure(info):
        info.reconfigure_cluster()

    def test_a_revoke_during_the_apply_survives_the_final_save(self):
        for method, status in ((self._start, "i"), (self._reconfigure, "r")):
            with self.subTest(method=method.__name__):
                db = {}
                cluster = stored(_cluster(enabled=True), db)
                self._run(method, cluster, revoke_during("apply", db))
                self.assertEqual(db["status"], status)
                self.assertIs(db["cmek_revoked"], True)
                self.assertIs(db["cmek_applied"], True)

    def test_a_revoke_during_a_failed_apply_survives_too(self):
        for method in (self._start, self._reconfigure):
            with self.subTest(method=method.__name__):
                db = {}
                cluster = stored(_cluster(enabled=True), db)
                self._run(method, cluster,
                          revoke_during("apply", db, APPLY_FAILED),
                          APPLY_FAILED)
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("e", "nm"))
                self.assertIs(db["cmek_revoked"], True)
                # stored before the apply ran: a failed apply locks it too
                self.assertIs(db["cmek_applied"], True)

    def test_without_a_revoke_the_flows_save_as_before(self):
        for method, status in ((self._start, "i"), (self._reconfigure, "r")):
            with self.subTest(method=method.__name__):
                db = {}
                cluster = stored(_cluster(enabled=True), db)
                self._run(method, cluster, just_run)
                self.assertEqual((db["status"], db["cloud_state"]),
                                 (status, "m"))
                self.assertEqual(db["status_message"], "")
                self.assertIs(db["cmek_revoked"], False)
                self.assertIs(db["cmek_applied"], True)

    def test_a_revoke_later_in_the_flow_survives_each_later_save(self):
        # Each save re-reads the flags, not just the first after the apply:
        # the revoke lands after that one here.
        from ..cluster_manager import clusterinfo

        def revoke_then(result, db):
            def run(*args, **kwargs):
                db["cmek_revoked"] = True
                return result
            return run

        cases = (
            # reading the nodes from the state, before the apply's last save
            (self._start, None, "_create_model_instances_from_tf_state", []),
            # the dashboard, before the start's own last save
            (self._start, clusterinfo.grafana, "create_cluster_dashboard",
             {"url": ""}),
            # anything in the apply, before the reconfigure's last save
            (self._reconfigure, None, "_apply_terraform", None),
        )
        for method, owner, where, result in cases:
            with self.subTest(method=method.__name__, where=where):
                db = {}
                info = _cluster_info(stored(_cluster(enabled=True), db))
                with ExitStack() as stack:
                    _cluster_terraform(stack, info)
                    stack.enter_context(mock.patch.object(
                        owner or info, where,
                        side_effect=revoke_then(result, db)))
                    method(info)
                self.assertIs(db["cmek_revoked"], True)

    def test_a_revoke_while_spack_is_located_survives_its_save(self):
        from ..cluster_manager import clusterinfo
        db = {}
        mount = SimpleNamespace(mount_path="/opt", export="export")
        cluster = stored(_cluster(
            enabled=True, spackdir="/opt/spack",
            mount_points=SimpleNamespace(order_by=lambda field: [mount])),
            db)
        info = _cluster_info(cluster)

        def found(**kwargs):
            db["cmek_revoked"] = True
            return ["install-location"]

        with mock.patch.object(clusterinfo,
                               "ApplicationInstallationLocation") as model:
            model.objects.filter.return_value.filter.side_effect = found
            info._configure_spack_install_loc()
        self.assertEqual(cluster.spack_install, "install-location")
        self.assertIs(db["cmek_revoked"], True)

    def test_a_revoke_after_the_start_was_queued_is_refused(self):
        # loaded when the start was queued, revoked before it ran
        db = {}
        cluster = stored(_cluster(enabled=True, status="e"), db)
        db["cmek_revoked"] = True
        info = _cluster_info(cluster)
        with mock.patch.object(info, "_create_cluster_dir") as create, \
                self.assertRaises(cmek.CmekConfigError):
            info.start_cluster("{}")
        create.assert_not_called()
        self.assertEqual((db["status"], db["cloud_state"]), ("e", "nm"))
        self.assertIs(db["cmek_revoked"], True)
        self.assertIn("was revoked", db["status_message"])

    def test_destroy(self):
        from ..cluster_manager import clusterinfo
        nodes = SimpleNamespace(
            all=lambda: SimpleNamespace(delete=lambda: None))
        # before the destroy began (its first save), or after it re-read the
        # row (its last save)
        for when in ("queued", "after the re-read"):
            with self.subTest(when=when):
                db = {}
                cluster = stored(_cluster(
                    enabled=True, status="r", cloud_state="m",
                    cmek_applied=True, controller_node=None,
                    login_nodes=nodes), db)
                if when == "queued":
                    db["cmek_revoked"] = True

                def reload(db=db, when=when, **kwargs):
                    fresh = stored(_cluster(controller_node=None,
                                            login_nodes=nodes), {})
                    fresh._db = db
                    fresh.refresh_from_db()
                    if when != "queued":
                        db["cmek_revoked"] = True
                    return fresh

                info = _cluster_info(cluster)
                with mock.patch.object(clusterinfo.utils, "run_terraform"), \
                        mock.patch.object(info, "get_terraform_dir"), \
                        mock.patch.object(info, "_get_credentials_file"), \
                        mock.patch.object(info,
                                          "_sweep_orphaned_compute_nodes"), \
                        mock.patch.object(clusterinfo, "ContainerRegistry"), \
                        mock.patch.object(clusterinfo, "Cluster") as model, \
                        mock.patch.object(clusterinfo.c2,
                                          "delete_cluster_subscription"):
                    model.objects.get.side_effect = reload
                    info.stop_cluster()
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("d", "xm"))
                self.assertIs(db["cmek_revoked"], True)
                self.assertIs(db["cmek_applied"], True)


class ImageBuildTests(SimpleTestCase):

    def _build(self, packer):
        from ..cluster_manager import image
        db = {}
        backend = _image_backend(stored(_image(enabled=True, status="c"), db))
        with mock.patch.object(image.utils, "run_packer",
                               side_effect=packer(db)):
            backend._create_image()
        return db

    def test_a_revoke_during_the_build_survives_the_status_save(self):
        db = self._build(lambda db: revoke_during("build", db))
        self.assertEqual(db["status"], "r")
        self.assertIs(db["cmek_revoked"], True)
        self.assertIs(db["cmek_applied"], True)

    def test_without_a_revoke_the_build_saves_as_before(self):
        db = self._build(lambda db: just_run)
        self.assertEqual(db["status"], "r")
        self.assertIs(db["cmek_revoked"], False)
        self.assertIs(db["cmek_applied"], True)


class FilestoreStartTests(SimpleTestCase):

    def _start(self, terraform, error=None, outputs=None, revoked=False):
        import json
        import tempfile
        from pathlib import Path
        from ..cluster_manager import filesystem
        from ..models import FilesystemImpl
        target = Path(tempfile.mkdtemp())
        (target / "filesystem.yaml").write_text("blueprint")
        out = target / "output.json"
        out.write_text(json.dumps(outputs if outputs is not None else
            {"network_storage_home": {"value": {"server_ip": "10.0.0.2"}}}))
        db = {}
        # the Filestore row itself, as a Filestore detail page passes it
        fs = stored(_filestore(enabled=True,
                               impl_type=FilesystemImpl.GCPFILESTORE), db)
        fs.gcpfilestorefilesystem = fs
        if revoked:
            # loaded when the start was queued, revoked before it ran
            db["cmek_revoked"] = True

        def run(directory, cmd, **kwargs):
            terraform(db)(directory, cmd, **kwargs)
            return (out, None)

        with ExitStack() as stack:
            for name, value in (("_base_dir_for_fs", target),
                                ("_tf_dir_for_fs", target),
                                ("_get_credentials_file", "/dev/null")):
                stack.enter_context(mock.patch.object(
                    filesystem, name, return_value=value))
            stack.enter_context(mock.patch.object(filesystem, "_run_ghpc"))
            stack.enter_context(mock.patch.object(
                filesystem.utils, "run_terraform", side_effect=run))
            if error:
                stack.enter_context(self.assertRaises(
                    error if isinstance(error, type) else type(error)))
            if error and not revoked:
                stack.enter_context(self.assertLogs(level="ERROR"))
            filesystem.start_filesystem(fs)
        return db

    def test_a_revoke_during_the_apply_survives_the_final_save(self):
        db = self._start(lambda db: revoke_during("apply", db))
        self.assertEqual(db["cloud_state"], "m")
        self.assertIs(db["cmek_revoked"], True)
        self.assertIs(db["cmek_applied"], True)

    def test_a_revoke_during_a_failed_apply_survives_too(self):
        db = self._start(lambda db: revoke_during("apply", db, APPLY_FAILED),
                         APPLY_FAILED)
        self.assertEqual(db["cloud_state"], "nm")
        self.assertIs(db["cmek_revoked"], True)
        # stored before the apply ran: a failed apply locks it too
        self.assertIs(db["cmek_applied"], True)

    def test_a_revoke_before_outputs_that_cannot_be_read_survives(self):
        # not a Terraform failure: the outputs lack the share
        db = self._start(lambda db: revoke_during("apply", db),
                         KeyError("network_storage_home"), outputs={})
        self.assertEqual(db["cloud_state"], "nm")
        self.assertIs(db["cmek_revoked"], True)
        self.assertIs(db["cmek_applied"], True)

    def test_a_revoke_after_the_start_was_queued_is_refused(self):
        db = self._start(lambda db: just_run, cmek.CmekConfigError,
                         revoked=True)
        self.assertEqual(db["cloud_state"], "nm")
        self.assertIs(db["cmek_revoked"], True)
        self.assertIn("was revoked", db["status_message"])

    def test_without_a_revoke_the_start_saves_as_before(self):
        db = self._start(lambda db: just_run)
        self.assertEqual((db["cloud_state"], db["status_message"]), ("m", ""))
        self.assertIs(db["cmek_revoked"], False)
        self.assertIs(db["cmek_applied"], True)


class KeepFlagsCurrentTests(SimpleTestCase):
    """What keep_flags_current() reads, and what it leaves alone."""

    def test_it_reads_only_the_two_flags(self):
        row = mock.Mock(pk=3, cmek_revoked=False)
        cmek.keep_flags_current(row)
        row.refresh_from_db.assert_called_once_with(
            fields=["cmek_revoked", "cmek_applied"])

    def test_a_filesystem_reads_its_filestore_row(self):
        child = mock.Mock(pk=3, cmek_revoked=False)
        parent = SimpleNamespace(pk=3, gcpfilestorefilesystem=child)
        cmek.keep_flags_current(parent)
        child.refresh_from_db.assert_called_once_with(
            fields=["cmek_revoked", "cmek_applied"])

    def test_nothing_to_read_is_left_alone(self):
        unsaved = mock.Mock(pk=None, cmek_revoked=False)
        for resource in (None, unsaved, SimpleNamespace(pk=3)):
            with self.subTest(resource=resource):
                cmek.keep_flags_current(resource)
        unsaved.refresh_from_db.assert_not_called()

    def test_a_deleted_row_is_left_alone(self):
        row = mock.Mock(pk=3, cmek_revoked=False)
        row.refresh_from_db.side_effect = ObjectDoesNotExist
        cmek.keep_flags_current(row)
        row.refresh_from_db.assert_called_once()
