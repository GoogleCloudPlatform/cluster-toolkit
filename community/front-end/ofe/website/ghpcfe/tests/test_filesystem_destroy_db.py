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
"""The conditional "dm" update of a Filestore destroy, on a real database.

A destroy request sets "dm" only if the row still holds what the request
loaded, so a request whose get_orm thread ran on after a client disconnect
cannot undo a later destroy or re-create a deleted record. Filesystem and
GCPFilestoreFilesystem inherit from CloudResource through separate tables,
and an update through the Filesystem manager selects the ids first and
then updates by id, losing the condition. These tests run the real
queries; Terraform, the state-lock read and the request are replaced.

The Delete page's refusal is tested here too, as it reads a field that
only the Filestore subclass's table holds.
"""
import json
import subprocess
from unittest import mock

from django.contrib.auth import get_user_model
from django.db import connection
from django.test import RequestFactory, TestCase
from django.test.utils import CaptureQueriesContext
from django.urls import reverse

from ..models import (CloudResource, Credential, Filesystem,
                      FilesystemExport, GCPFilestoreFilesystem)
from ..views import filesystems as v

GET_ORM = v.BackendDestroyFilesystem.__dict__["get_orm"].func
CHANGED = ("This Filestore changed while the request was being handled; "
           "reload the page and try again.")


class MarkDestroyingDbTests(TestCase):
    """_mark_destroying and the destroy flow against real tables."""

    def setUp(self):
        admin = get_user_model().objects.create_superuser(
            "admin", "admin@example.invalid", "pw")
        credential = Credential.objects.create(
            name="c", owner=admin, detail=json.dumps({"project_id": "proj"}))
        filestore = GCPFilestoreFilesystem.objects.create(
            name="fs1", cloud_credential=credential, cloud_state="m",
            status_message="Teardown failed: an earlier run.",
            cloud_region="europe-west4", cloud_zone="europe-west4-a",
            capacity=2048, performance_tier="en")
        FilesystemExport.objects.create(filesystem=filestore,
                                        export_name="/share")
        self.pk = filestore.pk
        self.subclass = self.subclass_fields()

    def load(self):
        """The record as get_orm loads it: through the Filesystem manager."""
        return Filesystem.objects.get(pk=self.pk)

    def row(self):
        """The stored (cloud_state, status_message), or None once deleted."""
        return (CloudResource.objects.filter(pk=self.pk)
                .values_list("cloud_state", "status_message").first())

    def subclass_fields(self):
        filestore = GCPFilestoreFilesystem.objects.get(pk=self.pk)
        return (filestore.name, filestore.cloud_region, filestore.cloud_zone,
                filestore.capacity, filestore.performance_tier,
                list(filestore.exports.values_list("export_name", flat=True)))

    def save_as_another_request(self, cloud_state, status_message):
        """Another request's cmd: a separate instance, saved in full."""
        other = self.load()
        other.cloud_state = cloud_state
        other.status_message = status_message
        other.save()

    # (a) an unchanged row

    def test_an_unchanged_row_is_marked_in_one_conditional_update(self):
        fs = self.load()
        with CaptureQueriesContext(connection) as queries:
            self.assertTrue(v.BackendDestroyFilesystem._mark_destroying(fs))
        sqls = [q["sql"] for q in queries.captured_queries]
        self.assertEqual(len(sqls), 1, sqls)
        qn = connection.ops.quote_name
        table = qn(CloudResource._meta.db_table)
        self.assertTrue(sqls[0].startswith(f"UPDATE {table} SET "), sqls)
        set_part, where = sqls[0].split(" WHERE ", 1)
        self.assertEqual(set_part, f"UPDATE {table} SET "
                                   f"{qn('cloud_state')} = 'dm'")
        for condition in (
                f"{table}.{qn('id')} = {self.pk}",
                f"{table}.{qn('cloud_state')} = 'm'",
                f"{table}.{qn('status_message')} = "
                "'Teardown failed: an earlier run.'"):
            self.assertIn(condition, where)
        self.assertEqual(fs.cloud_state, "dm")
        self.assertEqual(self.row(),
                         ("dm", "Teardown failed: an earlier run."))
        self.assertEqual(self.subclass_fields(), self.subclass)

    # (b) a row changed after the load

    def check_not_marked_after(self, cloud_state, status_message):
        fs = self.load()
        self.save_as_another_request(cloud_state, status_message)
        self.assertFalse(v.BackendDestroyFilesystem._mark_destroying(fs))
        self.assertEqual(fs.cloud_state, "m")  # the loaded copy too
        self.assertEqual(self.row(), (cloud_state, status_message))
        self.assertEqual(self.subclass_fields(), self.subclass)

    def test_a_changed_cloud_state_is_not_overwritten(self):
        # A later destroy finished while this request read the lock.
        self.check_not_marked_after("xm", "")

    def test_a_changed_status_message_is_not_overwritten(self):
        # Same state, new message: a later destroy failed in the same way.
        self.check_not_marked_after("m", "Teardown failed: a later run.")

    def test_a_change_just_before_the_update_runs_is_not_overwritten(self):
        # Another request's save lands after any SELECT but before the
        # UPDATE: only a condition inside the UPDATE itself refuses then.
        fs = self.load()
        landed = []

        def change_first(execute, sql, params, many, context):
            if not landed and sql.startswith("UPDATE"):
                landed.append(sql)
                self.save_as_another_request("xm", "")
            return execute(sql, params, many, context)

        with connection.execute_wrapper(change_first):
            marked = v.BackendDestroyFilesystem._mark_destroying(fs)
        self.assertEqual(len(landed), 1)
        self.assertFalse(marked)
        self.assertEqual(self.row(), ("xm", ""))
        self.assertEqual(fs.cloud_state, "m")

    # (c) a row deleted after the load

    def test_a_deleted_row_is_not_recreated(self):
        fs = self.load()
        counts = (CloudResource.objects.count(), Filesystem.objects.count(),
                  GCPFilestoreFilesystem.objects.count())
        self.load().delete()  # the Delete page, in another request
        self.assertFalse(v.BackendDestroyFilesystem._mark_destroying(fs))
        self.assertIsNone(self.row())
        self.assertFalse(Filesystem.objects.filter(pk=self.pk).exists())
        self.assertFalse(
            GCPFilestoreFilesystem.objects.filter(pk=self.pk).exists())
        self.assertEqual(
            (CloudResource.objects.count(), Filesystem.objects.count(),
             GCPFilestoreFilesystem.objects.count()),
            tuple(n - 1 for n in counts))

    # (d) the late accept, through the real get_orm and cmd

    def get_orm(self, during_lock_read=None):
        """The real get_orm body in this thread, so it sees the test data.

        during_lock_read runs inside the state-lock read, standing for what
        happens while a cancelled request's thread waits there.
        """
        view = v.BackendDestroyFilesystem()
        view.request = mock.Mock()
        shown = []

        def read_lock(unused_fs):
            if during_lock_read:
                during_lock_read()
            return None  # no Terraform operation holds the state

        with mock.patch.object(v.cm_fs, "running_state_lock", read_lock), \
                mock.patch.object(v.messages, "add_message",
                                  lambda req, lvl, text: shown.append(text)):
            (fs,) = GET_ORM(view, self.pk)
        return view, fs, shown

    @staticmethod
    def cmd(fs, error=None):
        """Run the real cmd; return what it raised, or None."""
        view = v.BackendDestroyFilesystem()
        with mock.patch.object(v.cm_fs, "destroy_filesystem",
                               side_effect=error):
            try:
                view.cmd(None, None, fs)
            except Exception as err:  # pylint: disable=broad-except
                return err  # compared by run_late
        return None

    def run_late(self, error=None):
        """A loads the row; B is accepted and its destroy ends; A resumes.

        B runs inside A's state-lock read, which get_orm's lock check wraps
        in a catch of every exception, so an assertion failing there would
        only be logged. B records what it saw instead, and this asserts it
        once A's get_orm has returned.
        """
        seen = {}

        def request_b():
            view, fs, shown = self.get_orm()
            seen["accepted"] = (view.refused, shown, self.row()[0])
            seen["raised"] = self.cmd(fs, error)
            seen["finished"] = True

        with mock.patch.object(v, "_running_destroys",
                               v._RunningDestroys()), \
                self.assertLogs(v.logger, "INFO") as logs:
            view, fs, shown = self.get_orm(during_lock_read=request_b)
        self.assertEqual(seen.get("accepted"), (False, [], "dm"), seen)
        self.assertIs(seen.get("raised"), error)
        self.assertIs(seen.get("finished"), True, seen)
        self.assertNotIn("WARNING", [r.levelname for r in logs.records])
        self.assertTrue(view.refused)
        self.assertEqual(shown, [CHANGED])
        self.assertEqual(fs.cloud_state, "m")
        self.assertIn(
            f"Refused the destroy of filesystem {self.pk}: it changed while "
            "the request was being handled",
            [r.getMessage() for r in logs.records])
        self.assertEqual(self.subclass_fields(), self.subclass)
        return self.row()

    def test_a_late_accept_after_a_successful_destroy_changes_nothing(self):
        self.assertEqual(self.run_late(), ("xm", ""))

    def test_a_late_accept_after_a_failed_destroy_keeps_its_message(self):
        state, message = self.run_late(
            subprocess.CalledProcessError(1, "terraform"))
        self.assertEqual(state, "um")
        self.assertTrue(message.startswith("Teardown failed: "), message)
        self.assertIn("destroy it again", message)


class DeleteGuardDbTests(TestCase):
    """The Delete page's refusal, with the record loaded from real tables.

    cmek_applied is a field of GCPFilestoreFilesystem, not of Filesystem,
    so the refusal depends on get_object loading the subclass.
    """

    def setUp(self):
        self.admin = get_user_model().objects.create_superuser(
            "admin", "admin@example.invalid", "pw")
        self.credential = Credential.objects.create(
            name="c", owner=self.admin,
            detail=json.dumps({"project_id": "proj"}))

    def filestore(self, applied):
        """A Filestore whose first apply failed, or that never started."""
        filestore = GCPFilestoreFilesystem.objects.create(
            name="fs1", cloud_credential=self.credential, cloud_state="nm",
            cloud_region="europe-west4", cloud_zone="europe-west4-a",
            capacity=2048, performance_tier="en", cmek_applied=applied)
        return filestore.pk

    def call(self, method, pk):
        request = getattr(RequestFactory(), method)(
            reverse("fs-delete", kwargs={"pk": pk}))
        request.user = self.admin
        with mock.patch.object(v.messages, "error") as error, \
                mock.patch.object(v.messages, "success"):
            response = v.FilesystemDeleteView.as_view()(request, pk=pk)
        return response, error

    def test_the_record_is_loaded_as_its_filestore_subclass(self):
        pk = self.filestore(applied=True)
        view = v.FilesystemDeleteView()
        view.setup(RequestFactory().get("/"), pk=pk)
        obj = view.get_object()
        self.assertIsInstance(obj, GCPFilestoreFilesystem)
        self.assertIs(obj.cmek_applied, True)

    def test_a_failed_first_apply_is_neither_offered_nor_deleted(self):
        pk = self.filestore(applied=True)
        for method in ("get", "post"):
            with self.subTest(method=method):
                response, error = self.call(method, pk)
                self.assertEqual(response.status_code, 302)
                self.assertEqual(response.url,
                                 reverse("fs-detail", kwargs={"pk": pk}))
                self.assertIn("Cannot delete fs1", error.call_args[0][1])
                self.assertIn("Destroy it instead", error.call_args[0][1])
                self.assertTrue(
                    GCPFilestoreFilesystem.objects.filter(pk=pk).exists())
                self.assertTrue(CloudResource.objects.filter(pk=pk).exists())

    def test_a_record_that_never_started_an_apply_is_deleted(self):
        # The same request deletes when nothing may exist in the cloud, so
        # the refusal above is the guard's doing.
        pk = self.filestore(applied=False)
        response, error = self.call("post", pk)
        self.assertEqual(response.status_code, 302)
        self.assertEqual(response.url, reverse("filesystems"))
        error.assert_not_called()
        self.assertFalse(CloudResource.objects.filter(pk=pk).exists())
