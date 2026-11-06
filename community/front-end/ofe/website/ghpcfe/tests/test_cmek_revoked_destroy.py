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
"""A cluster whose key was revoked can still be destroyed.

`terraform destroy` refreshes its state first, and the refresh reads back the
Slurm bucket objects the controller module wrote with the cluster's key. Once
the key is revoked that read fails (KEY_DESTROY_SCHEDULED), so the destroy
failed before removing anything and the cluster stayed "terminating" for ever,
with no reason shown. A revoked cluster is now destroyed without the refresh,
and any failed destroy leaves "terminating" with its reason recorded.
"""
import asyncio
import subprocess
import tempfile
from contextlib import ExitStack
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from django.template.loader import render_to_string
from django.test import RequestFactory, SimpleTestCase

from ..cluster_manager import clusterinfo
from ..cluster_manager import cloud_info
from ..cluster_manager import cmek
from ..views import cmek as v
from .test_cmek_applied_flows import _cluster, _cluster_info
from .test_cmek_stale_flags import stored

KMS_ERROR = (
    'Error: Error when reading or editing Storage Bucket Object '
    '"bucket/slurm-gcp-devel.zip": googleapi: Error 400: Cloud KMS key is '
    'disabled, destroyed, or scheduled to be destroyed, KEY_DESTROY_SCHEDULED')


def terraform(commands, error=None, during=None):
    """A subprocess.run stand-in for run_terraform().

    Records each terraform command line in `commands`; the destroy writes
    `error` to its stderr log and fails, as terraform does. `during` runs
    while the destroy does.
    """
    def run(cmdline, **kwargs):
        if cmdline[0] != "terraform":
            return  # the gcloud auth context run_terraform() sets up
        commands.append(cmdline)
        if during:
            during()
        if error:
            kwargs["stderr"].write(error.encode())
            raise subprocess.CalledProcessError(1, cmdline)
    return run


class DestroyTestCase(SimpleTestCase):

    def _destroy(self, db, error=None, during=None, revoke_queued=False,
                 status="r", cloud_state="m", tf_dir_holds=(".terraform",),
                 **fields):
        """Run stop_cluster() on a stored cluster; returns the commands run
        and what the caller saw raised.

        The Terraform directory holds the entries named in `tf_dir_holds`
        (an initialised one by default); None means it does not exist.
        """
        from ..cluster_manager import clusterinfo
        nodes = SimpleNamespace(all=lambda: SimpleNamespace(delete=lambda: None))
        cluster = stored(_cluster(
            enabled=True, status=status, cloud_state=cloud_state,
            cmek_applied=True, controller_node=None, login_nodes=nodes,
            **fields), db)
        if revoke_queued:
            db["cmek_revoked"] = True

        def reload(**kwargs):
            fresh = stored(_cluster(controller_node=None,
                                    login_nodes=nodes), {})
            fresh._db = db
            fresh.refresh_from_db()
            return fresh

        info = _cluster_info(cluster)
        tf_dir = Path(tempfile.mkdtemp()) / "primary"
        if tf_dir_holds is not None:
            tf_dir.mkdir()
            for name in tf_dir_holds:
                if name.startswith("."):
                    (tf_dir / name).mkdir()
                else:
                    (tf_dir / name).write_text("{}")
        commands, raised = [], None
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(
                clusterinfo.utils.subprocess, "run",
                side_effect=terraform(commands, error, during)))
            stack.enter_context(mock.patch.object(
                info, "get_terraform_dir", return_value=tf_dir))
            stack.enter_context(mock.patch.object(
                info, "_get_credentials_file",
                return_value=str(tf_dir / "cloud_credentials")))
            # the clean-up after the destroy, kept for the tests to check
            self.cleanup = SimpleNamespace(
                sweep=stack.enter_context(mock.patch.object(
                    info, "_sweep_orphaned_compute_nodes")),
                registries=stack.enter_context(mock.patch.object(
                    clusterinfo, "ContainerRegistry")),
                subscription=stack.enter_context(mock.patch.object(
                    clusterinfo.c2, "delete_cluster_subscription")))
            model = stack.enter_context(mock.patch.object(
                clusterinfo, "Cluster"))
            model.objects.get.side_effect = reload
            if error:
                stack.enter_context(self.assertLogs(level="ERROR"))
            try:
                info.stop_cluster()
            except subprocess.CalledProcessError as err:
                raised = err
        return commands, raised


class RefreshFreeDestroyTests(DestroyTestCase):

    def test_a_revoked_cluster_is_destroyed_without_a_refresh(self):
        db = {}
        commands, _ = self._destroy(db, cmek_revoked=True)
        self.assertEqual(commands, [[
            "terraform", "destroy", "-no-color", "-input=false",
            "-refresh=false", "-auto-approve"]])
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))

    def test_a_revoke_after_the_destroy_was_queued_counts(self):
        # loaded when the destroy was queued, revoked before it ran
        db = {}
        commands, _ = self._destroy(db, revoke_queued=True,
                                    cmek_revoked=False)
        self.assertIn("-refresh=false", commands[0])

    def test_any_other_cluster_keeps_the_same_command(self):
        db = {}
        commands, _ = self._destroy(db, cmek_revoked=False)
        self.assertEqual(commands, [[
            "terraform", "destroy", "-no-color", "-input=false",
            "-auto-approve"]])
        self.assertEqual((db["status"], db["cloud_state"]), ("d", "xm"))
        self.assertEqual(db["status_message"], "")


class FailedDestroyTests(DestroyTestCase):

    def test_a_failed_destroy_leaves_terminating_with_its_reason(self):
        for revoked in (True, False):
            with self.subTest(revoked=revoked):
                db = {}
                _, raised = self._destroy(db, error=KMS_ERROR + "\n",
                                          cmek_revoked=revoked)
                self.assertEqual(db["status"], "e")
                # part of it may be gone: still being destroyed, not "m"
                self.assertEqual(db["cloud_state"], "dm")
                self.assertTrue(
                    db["status_message"].startswith("Teardown failed: "))
                self.assertIn("KEY_DESTROY_SCHEDULED", db["status_message"])
                # still raised, so the task records the failure as before
                self.assertIsInstance(raised, subprocess.CalledProcessError)

    def test_the_reason_is_kept_short(self):
        db = {}
        self._destroy(db, error="Error: " + "x" * 5000)
        self.assertLessEqual(len(db["status_message"]), 320)
        self.assertTrue(db["status_message"].endswith("..."))

    def test_a_revoke_during_a_failed_destroy_is_not_lost(self):
        db = {}

        def revoke():
            db["cmek_revoked"] = True

        self._destroy(db, error=KMS_ERROR, during=revoke)
        self.assertEqual(db["status"], "e")
        self.assertIs(db["cmek_revoked"], True)
        self.assertIs(db["cmek_applied"], True)


# What terraform -no-color writes to stderr: each diagnostic in a box.
BOXED_KMS_ERROR = (
    "╷\n"
    "│ Warning: Deprecated attribute\n"
    "│ \n"
    "│ The attribute \"self_link\" is deprecated.\n"
    "╵\n"
    "╷\n"
    "│ Error: Error when reading or editing Storage Bucket Object "
    "\"bucket/slurm-gcp-devel.zip\": googleapi: Error 400: Cloud KMS key is "
    "disabled, destroyed, or scheduled to be destroyed, "
    "KEY_DESTROY_SCHEDULED\n"
    "│ \n"
    "│   with module.slurm_files.google_storage_bucket_object.devel[0],\n"
    "│   on modules/slurm_files/main.tf line 120, in resource "
    "\"google_storage_bucket_object\" \"devel\":\n"
    "│  120: resource \"google_storage_bucket_object\" \"devel\" {\n"
    "│ \n"
    "╵\n")


class DestroyErrorTextTests(DestroyTestCase):

    def test_the_reason_is_read_from_terraforms_boxed_output(self):
        db = {}
        self._destroy(db, error=BOXED_KMS_ERROR)
        self.assertEqual(
            db["status_message"],
            "Teardown failed: Error: Error when reading or editing Storage "
            "Bucket Object \"bucket/slurm-gcp-devel.zip\": googleapi: Error "
            "400: Cloud KMS key is disabled, destroyed, or scheduled to be "
            "destroyed, KEY_DESTROY_SCHEDULED")

    def test_a_long_boxed_reason_is_kept_short(self):
        db = {}
        self._destroy(db, error="╷\n│ Error: " + "x" * 5000 + "\n╵\n")
        self.assertIn("Error: xxx", db["status_message"])
        self.assertLessEqual(len(db["status_message"]), 320)
        self.assertTrue(db["status_message"].endswith("..."))


class RetriedDestroyTests(DestroyTestCase):
    """A failed destroy is finished by destroying the cluster again."""

    def test_a_retry_finishes_and_clears_the_reason(self):
        for revoked in (True, False):
            with self.subTest(revoked=revoked):
                db = {}
                self._destroy(db, error=BOXED_KMS_ERROR,
                              cmek_revoked=revoked)
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("e", "dm"))
                commands, raised = self._destroy(
                    db, status=db["status"], cloud_state=db["cloud_state"],
                    status_message=db["status_message"],
                    cmek_revoked=revoked)
                self.assertIsNone(raised)
                self.assertEqual(
                    "-refresh=false" in commands[0], revoked)
                self.assertEqual((db["status"], db["cloud_state"]),
                                 ("d", "xm"))
                self.assertEqual(db["status_message"], "")


class StartAfterFailedDestroyTests(DestroyTestCase):
    """A cluster whose destroy failed is not started again over it."""

    STEPS = ("_create_cluster_dir", "_set_credentials", "update",
             "_run_ghpc", "_initialize_terraform", "_apply_terraform")

    def _start(self, cluster):
        info = _cluster_info(cluster)
        with ExitStack() as stack:
            steps = [stack.enter_context(mock.patch.object(info, name))
                     for name in self.STEPS]
            stack.enter_context(mock.patch.object(
                clusterinfo.grafana, "create_cluster_dashboard",
                return_value={}))
            try:
                info.start_cluster("{}")
            except Exception as err:  # pylint: disable=broad-except
                return err, steps
        return None, steps

    def test_start_is_refused_after_a_failed_destroy(self):
        db = {}
        self._destroy(db, error=BOXED_KMS_ERROR, cmek_revoked=False)
        cluster = stored(_cluster(
            enabled=True, status=db["status"],
            cloud_state=db["cloud_state"], cmek_applied=True,
            status_message=db["status_message"]), db)
        raised, steps = self._start(cluster)
        self.assertIsInstance(raised, RuntimeError)
        self.assertEqual(db["status_message"], clusterinfo.TEARDOWN_UNFINISHED)
        self.assertEqual((db["status"], db["cloud_state"]), ("e", "dm"))
        for step in steps:
            step.assert_not_called()

    def test_any_other_failed_cluster_still_starts(self):
        db = {}
        cluster = stored(_cluster(enabled=True, status="e",
                                  cloud_state="nm", status_message=""), db)
        raised, steps = self._start(cluster)
        self.assertIsNone(raised)
        self.assertEqual(db["status_message"], "")
        for step in steps:
            step.assert_called_once()

    def test_a_cluster_being_destroyed_still_counts_as_deployed(self):
        cluster = _cluster(enabled=True, status="e", cloud_state="dm")
        self.assertIs(cmek.is_frozen(cluster), True)


class ClusterBackendGuardTests(SimpleTestCase):
    """The start and destroy URLs refuse a cluster in the wrong state, with a
    message and no task started."""

    def _call(self, view, method, row):
        from ..views import clusters as v
        created = mock.AsyncMock()
        with mock.patch.object(v.Cluster.objects, "get", return_value=row), \
                mock.patch.object(view, "test_user_is_cluster_admin",
                                  mock.AsyncMock()), \
                mock.patch.object(view, "create_task", created), \
                mock.patch("django.contrib.messages.add_message") as message:
            view.request = mock.Mock()
            response = asyncio.run(
                getattr(view, method)(view.request, 3))
        self.assertEqual(response.status_code, 302)
        return created, message

    def test_start_is_refused_after_a_failed_destroy(self):
        from ..views import clusters as v
        row = mock.MagicMock(status="e", cloud_state="dm")
        created, message = self._call(v.BackendStartCluster(), "get", row)
        created.assert_not_awaited()
        message.assert_called_once()
        self.assertEqual(message.call_args.args[2],
                         clusterinfo.TEARDOWN_UNFINISHED)
        row.save.assert_not_called()

    def test_start_after_a_failed_start_is_not(self):
        from ..views import clusters as v
        row = mock.MagicMock(status="e", cloud_state="nm")
        created, message = self._call(v.BackendStartCluster(), "get", row)
        created.assert_awaited_once()
        message.assert_not_called()

    def test_a_destroyed_cluster_is_not_destroyed_again(self):
        from ..views import clusters as v
        row = mock.MagicMock(status="d", cloud_state="xm")
        created, message = self._call(v.BackendDestroyCluster(), "post", row)
        created.assert_not_awaited()
        message.assert_called_once()
        self.assertIn("already destroyed", message.call_args.args[2])
        row.save.assert_not_called()

    def test_any_other_cluster_is_destroyed(self):
        from ..views import clusters as v
        for status, cloud_state in (("r", "m"), ("e", "dm"), ("e", "nm"),
                                    ("n", "nm"), ("s", "m"), ("i", "m"),
                                    ("t", "dm"), ("c", "nm")):
            with self.subTest(status=status, cloud_state=cloud_state):
                row = mock.MagicMock(status=status, cloud_state=cloud_state)
                created, message = self._call(
                    v.BackendDestroyCluster(), "post", row)
                created.assert_awaited_once()
                message.assert_not_called()


class DetailPageAfterFailedDestroyTests(SimpleTestCase):
    """The cluster page offers Delete, not Retry, after a failed destroy."""

    def _page(self, status, cloud_state):
        cluster = _cluster(
            enabled=False, status=status, cloud_state=cloud_state,
            status_message="Teardown failed: Error: boom",
            get_status_display=lambda: "Error",
            container_registry_relations=SimpleNamespace(exists=lambda: False),
            authorised_users=SimpleNamespace(all=lambda: []),
            mount_points=SimpleNamespace(all=lambda: []))
        request = RequestFactory().get("/cluster/1")
        request.user = mock.MagicMock(is_authenticated=True)
        html = render_to_string("cluster/detail.html", {
            "object": cluster, "cluster": cluster, "admin_view": 1,
            "navtab": "cluster"}, request=request)
        return " ".join(html.split())

    def test_a_failed_destroy_points_to_delete(self):
        page = self._page("e", "dm")
        self.assertNotIn(">Retry</a>", page)
        self.assertIn(">Delete</a>", page)
        self.assertIn("A teardown of this cluster was started and did not "
                      "finish.", page)
        self.assertIn("Teardown failed: Error: boom", page)

    def test_a_failed_start_still_offers_retry(self):
        page = self._page("e", "nm")
        self.assertIn(">Retry</a>", page)
        self.assertIn(">Delete</a>", page)
        self.assertNotIn("A teardown of this cluster", page)

    def test_a_cluster_being_destroyed_renders(self):
        page = self._page("t", "dm")
        self.assertNotIn(">Retry</a>", page)
        self.assertNotIn(">Delete</a>", page)

class RevokePageTests(SimpleTestCase):
    """The cluster revoke page says how the cluster is destroyed afterwards."""

    WARNING = ("an administrator must remove those Slurm bucket objects "
               "before the cluster can be deleted")

    def _page(self, kind):
        row = SimpleNamespace(pk=1, name="r1",
                              cloud_credential=SimpleNamespace(detail="{}"))
        path = ("projects/proj-a/locations/europe-west4/keyRings/ring-a"
                f"/cryptoKeys/{kind}-abc")
        route = {"cluster": "cluster-detail", "filestore": "fs-detail",
                 "image": "image-view"}[kind]
        view = v.CmekKeyDestroyView()
        view.setup(RequestFactory().get("/"), kind=kind, pk=1)
        with mock.patch.object(
                view, "_target",
                return_value=(row, kind.title(), route, path,
                              {"project": "proj-a",
                               "location": "europe-west4",
                               "ring": "ring-a", "key": f"{kind}-abc"})), \
                mock.patch.object(
                    cloud_info, "crypto_key_destroy_scheduled_duration",
                    return_value="2592000s"):
            context = view.get_context_data()
        return " ".join(render_to_string("cmek/key_destroy.html",
                                         context).split())

    def test_the_cluster_page_warns_how_it_is_destroyed_after(self):
        page = self._page("cluster")
        self.assertIn("OFE cannot restore the key", page)
        self.assertIn("skips Terraform's refresh", page)
        self.assertIn(self.WARNING, page)
        self.assertIn("30 days", page)

    def test_the_other_pages_do_not(self):
        for kind in ("filestore", "image"):
            with self.subTest(kind=kind):
                page = self._page(kind)
                self.assertNotIn("Slurm bucket", page)
                self.assertNotIn("refresh", page)
                self.assertIn("30 days", page)
