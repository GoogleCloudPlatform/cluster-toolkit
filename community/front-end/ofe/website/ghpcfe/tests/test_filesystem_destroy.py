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
"""Destroying a filesystem that a cluster or workbench still mounts is refused.

Live-found 2026-10-21: confirming the destroy page for a Filestore that a
running cluster had mounted deleted it and left the cluster's mount hanging,
because the backend view never checked for users.
"""
import ast
import asyncio
import json
import subprocess
import tempfile
import threading
from pathlib import Path
from types import SimpleNamespace as NS
from unittest import mock

from asgiref.sync import ThreadSensitiveContext, sync_to_async
from django.test import SimpleTestCase

from ..views import filesystems as v

ROOT = Path(__file__).resolve().parents[1]


def fs_double(clusters=(), workbenches=()):
    fs = mock.MagicMock(cloud_state="m")
    fs.active_mounts.return_value = [
        NS(cluster=NS(name=n)) for n in clusters]
    fs.active_workbench_mounts.return_value = [
        NS(workbench=NS(name=n)) for n in workbenches]
    fs.in_use.return_value = bool(clusters or workbenches)
    return fs


def conditional_update(rows=1):
    """A stand-in whose filter(...).update(...) returns rows.

    Tests patch CloudResource.objects.filter with its filter, never
    CloudResource.objects itself: Filesystem.objects resolves through that
    class attribute, so replacing it would replace Filesystem.objects too.
    """
    objects = mock.Mock()
    objects.filter.return_value.update.return_value = rows
    return objects


class DestroyRefusalTests(SimpleTestCase):
    def run_get_orm(self, fs, lock=None, rows=1, held=(7, 8)):
        """Run get_orm for filesystem 7 with the claims in held taken.

        get() holds the claim of 7 around get_orm, and 8 stands for another
        Filestore's destroy. get_orm neither takes nor releases a claim, so
        the set must hold exactly what it held before.
        """
        fs.id = fs.pk = 7
        view = v.BackendDestroyFilesystem()
        view.request = mock.Mock()
        reader = (mock.Mock(side_effect=lock) if isinstance(lock, Exception)
                  else mock.Mock(return_value=lock))
        running = v._RunningDestroys()
        for fs_id in held:
            self.assertTrue(running.claim(fs_id))
        self.objects = conditional_update(rows)
        with mock.patch.object(v.Filesystem.objects, "get", return_value=fs), \
                mock.patch.object(v.CloudResource.objects, "filter",
                                  self.objects.filter), \
                mock.patch.object(v.cm_fs, "running_state_lock", reader), \
                mock.patch.object(v, "_running_destroys", running), \
                mock.patch.object(v.cm_fs, "_tf_dir_for_fs",
                                  return_value=Path("/fs dir/primary")), \
                mock.patch.object(v.cm_fs, "_get_credentials_file",
                                  return_value=Path("/fs dir/creds")), \
                mock.patch.object(v.messages, "add_message") as msg:
            asyncio.run(view.get_orm(7))
        self.assertEqual({fs_id: fs_id in running for fs_id in (7, 8)},
                         {fs_id: fs_id in held for fs_id in (7, 8)})
        return view, msg

    def test_no_outcome_takes_or_releases_a_claim(self):
        # Each way get_orm ends, with the claim of 7 held (as get() holds it)
        # and not held: a claim taken would show in the second, a release in
        # the first. run_get_orm compares the set.
        outcomes = {
            "mounted": (["c1"], {}),
            "locked": ([], {"lock": {"ID": "17", "Who": "ofe@server",
                                     "Created": "t"}}),
            "changed": ([], {"rows": 0}),
            "allowed": ([], {}),
            "unreadable lock": ([], {"lock": OSError("bucket unreachable")}),
        }
        with self.assertLogs(v.logger, "INFO"):
            for name, (clusters, kwargs) in outcomes.items():
                for held in ((7, 8), (8,)):
                    with self.subTest(outcome=name, held=held):
                        view, _ = self.run_get_orm(
                            fs_double(clusters=clusters), held=held, **kwargs)
                        self.assertEqual(
                            view.refused,
                            name in ("mounted", "locked", "changed"))

    def assert_marked_destroying(self, fs, loaded_state):
        """"dm" was set by one conditional update, never by save()."""
        self.objects.filter.assert_called_once_with(
            pk=fs.pk, cloud_state=loaded_state,
            status_message=fs.status_message)
        self.objects.filter.return_value.update.assert_called_once_with(
            cloud_state="dm")
        fs.save.assert_not_called()

    def test_refused_when_a_live_cluster_mounts_it(self):
        fs = fs_double(clusters=["c1"])
        view, msg = self.run_get_orm(fs)
        self.assertTrue(view.refused)
        self.assertEqual(fs.cloud_state, "m")  # state unchanged
        fs.save.assert_not_called()
        self.objects.filter.assert_not_called()
        self.assertIn("c1", msg.call_args[0][2])

    def test_refused_when_a_workbench_mounts_it(self):
        fs = fs_double(workbenches=["wb1"])
        view, msg = self.run_get_orm(fs)
        self.assertTrue(view.refused)
        fs.save.assert_not_called()
        self.objects.filter.assert_not_called()
        self.assertIn("wb1", msg.call_args[0][2])

    def test_allowed_when_nothing_live_mounts_it(self):
        # the model query excludes destroyed users, so none are returned
        fs = fs_double()
        view, msg = self.run_get_orm(fs)
        self.assertFalse(view.refused)
        self.assertEqual(fs.cloud_state, "dm")
        self.assert_marked_destroying(fs, "m")
        msg.assert_not_called()

    def test_refused_when_the_row_changed_after_the_load(self):
        # The conditional update matched no row: a later destroy finished,
        # or the record was deleted, while this request read the lock.
        fs = fs_double()
        with self.assertLogs(v.logger, "INFO") as logs:
            view, msg = self.run_get_orm(fs, rows=0)
        self.assertTrue(view.refused)
        self.assertEqual(fs.cloud_state, "m")  # the in-memory copy too
        fs.save.assert_not_called()
        self.objects.filter.return_value.update.assert_called_once_with(
            cloud_state="dm")
        self.assertEqual(
            msg.call_args[0][2],
            "This Filestore changed while the request was being handled; "
            "reload the page and try again.")
        self.assertIn(
            f"Refused the destroy of filesystem {fs.id}: it changed while "
            "the request was being handled", logs.output[-1])

    def test_refused_while_a_destroy_holds_the_state_lock(self):
        fs = fs_double()
        fs.cloud_state = "dm"
        view, msg = self.run_get_orm(fs, lock={
            "ID": "17", "Who": "ofe@server", "Created": "2026-11-23T10:00:00Z"})
        self.assertTrue(view.refused)
        self.assertEqual(fs.cloud_state, "dm")  # state unchanged
        fs.save.assert_not_called()
        self.objects.filter.assert_not_called()
        text = msg.call_args[0][2]
        self.assertTrue(text.startswith(
            "A Terraform operation is already running for this Filestore:"),
            text)
        self.assertIn("ofe@server", text)
        self.assertIn("2026-11-23T10:00:00Z", text)
        self.assertIn("(lock ID 17)", text)
        self.assertIn("stale state lock", text)
        # The command the admin guide says to run, quoted as the log quotes it
        self.assertTrue(text.endswith(
            "cd '/fs dir/primary' && "
            "GOOGLE_APPLICATION_CREDENTIALS='/fs dir/creds' "
            "terraform force-unlock -force 17"), text)

    def test_the_force_unlock_command_matches_the_one_the_log_gives(self):
        fs = fs_double()
        with mock.patch.object(v.cm_fs, "_tf_dir_for_fs",
                               return_value=Path("/fs dir/primary")), \
                mock.patch.object(v.cm_fs, "_get_credentials_file",
                                  return_value=Path("/fs dir/creds")):
            command = v.cm_fs.force_unlock_command(fs, "17")
        logged = v.cm_fs.utils._state_lock_message(  # pylint: disable=protected-access
            Path("/fs dir/primary"), {"ID": "17"}, Path("/fs dir/creds"),
            "reason")
        self.assertIn(f"  {command}\n", logged)

    def test_a_stuck_destroy_with_no_live_lock_is_accepted(self):
        fs = fs_double()
        fs.cloud_state = "dm"
        view, msg = self.run_get_orm(fs, lock=None)
        self.assertFalse(view.refused)
        self.assertEqual(fs.cloud_state, "dm")
        self.assert_marked_destroying(fs, "dm")
        msg.assert_not_called()

    def test_a_lock_that_cannot_be_read_does_not_refuse(self):
        fs = fs_double()
        with self.assertLogs(v.logger, "WARNING") as logs:
            view, msg = self.run_get_orm(
                fs, lock=OSError("bucket unreachable"))
        self.assertFalse(view.refused)
        self.assertEqual(fs.cloud_state, "dm")
        self.assert_marked_destroying(fs, "m")
        msg.assert_not_called()
        self.assertIn("bucket unreachable", logs.output[0])

    def test_get_creates_no_task_when_refused(self):
        tree = ast.parse((ROOT / "views/filesystems.py").read_text())
        get = next(
            n for n in ast.walk(tree)
            if isinstance(n, ast.AsyncFunctionDef) and n.name == "get"
            and any("Destroy Filesystem" in ast.unparse(x)
                    for x in ast.walk(n)))
        src = ast.unparse(get)
        self.assertLess(src.index("self.refused"), src.index("create_task"))


class InUseQueryTests(SimpleTestCase):
    def test_destroyed_users_are_ignored_and_workbenches_counted(self):
        src = (ROOT / "models.py").read_text()
        start = src.index("def active_mounts")
        # VirtualNetwork has an in_use() too, earlier in the module
        body = src[start:src.index("def in_use", start)]
        self.assertEqual(body.count('"xm"'), 2)
        self.assertIn("WorkbenchMountPoint", body)


class InUseLogicTests(SimpleTestCase):
    """The real in_use(), with the two mount queries replaced."""

    def check(self, clusters, workbenches):
        from ..models import Filesystem
        with mock.patch.object(
                Filesystem, "active_mounts",
                return_value=mock.Mock(exists=lambda: clusters)), \
                mock.patch.object(
                    Filesystem, "active_workbench_mounts",
                    return_value=mock.Mock(exists=lambda: workbenches)):
            return Filesystem().in_use()

    def test_only_a_cluster_mounts_it(self):
        self.assertTrue(self.check(True, False))

    def test_only_a_workbench_mounts_it(self):
        self.assertTrue(self.check(False, True))

    def test_both_mount_it(self):
        self.assertTrue(self.check(True, True))

    def test_nothing_mounts_it(self):
        self.assertFalse(self.check(False, False))


class DestroyGetBehaviourTests(SimpleTestCase):
    """Run the view's get() itself, not its source text."""

    def run_get(self, refused):
        view = v.BackendDestroyFilesystem()
        view.request = mock.Mock()

        async def fake_get_orm(pk):
            view.refused = refused
            return ("fs",)

        created = mock.AsyncMock()
        # A fresh set: get() claims, and no real task here releases it.
        with mock.patch.object(v, "_running_destroys",
                               v._RunningDestroys()), \
                mock.patch.object(view, "get_orm", fake_get_orm), \
                mock.patch.object(view, "test_user_is_cluster_admin",
                                  mock.AsyncMock()), \
                mock.patch.object(view, "create_task", created):
            response = asyncio.run(view.get(mock.Mock(), 7))
        return response, created

    def test_a_refusal_redirects_to_the_detail_page_and_creates_no_task(self):
        response, created = self.run_get(True)
        self.assertEqual(response.status_code, 302)
        self.assertTrue(response.url.endswith("/filesystem/detail/7"))
        created.assert_not_awaited()

    def test_an_allowed_destroy_creates_the_task(self):
        response, created = self.run_get(False)
        created.assert_awaited_once()
        self.assertEqual(response.status_code, 302)


class DestroyFlowMixin:
    """Real get(), get_orm(), create_task() and cmd() on one event loop.

    Only the database, the lock read and Terraform are replaced. The first
    destroy's Terraform blocks until the test lets it finish, standing for
    its `terraform init` and state pull, when no state lock is held yet.
    """

    def setUp(self):
        super().setUp()
        self.fs = fs_double()
        self.fs.id = 7
        self.lock = None
        self.go = threading.Event()
        # Whether each destroy's wait for go ended because go was set. cmd
        # catches every exception, so an assertion here would only fail the
        # destroy; wait_for_tasks asserts these instead.
        self.woken = []
        self.error = None
        self.records = []
        self.tasks = []
        self.messages = []
        spawn = asyncio.create_task

        def track(coro, **kwargs):
            task = spawn(coro, **kwargs)
            if getattr(coro, "__name__", "") == "_cmd":  # the destroy task
                self.tasks.append(task)
            return task

        def destroy(fs):
            self.woken.append(self.go.wait(10))
            if self.error:
                raise self.error

        async def record(user, title):
            self.records.append(title)
            n = len(self.records)
            return mock.Mock(pk=n, id=n, title=title, data={})

        self.objects = conditional_update()
        patches = [
            mock.patch.object(v, "_running_destroys", v._RunningDestroys()),
            mock.patch.object(v.Filesystem.objects, "get",
                              return_value=self.fs),
            mock.patch.object(v.CloudResource.objects, "filter",
                              self.objects.filter),
            mock.patch.object(v.cm_fs, "running_state_lock",
                              lambda fs: self.lock),
            mock.patch.object(v.cm_fs, "force_unlock_command",
                              return_value="the unlock command"),
            mock.patch.object(v.cm_fs, "destroy_filesystem", destroy),
            mock.patch.object(v.messages, "add_message",
                              lambda req, lvl, text: self.messages.append(
                                  text)),
            mock.patch.object(v.BackendDestroyFilesystem,
                              "test_user_is_cluster_admin", mock.AsyncMock()),
            mock.patch.object(v.BackendDestroyFilesystem, "get_user_token",
                              mock.AsyncMock(return_value="token")),
            mock.patch.object(v.BackendDestroyFilesystem,
                              "make_task_record", side_effect=record),
            mock.patch.object(asyncio, "create_task", track),
        ]
        for patch in patches:
            patch.start()
            self.addCleanup(patch.stop)

    def track_get_orm(self):
        """Patch get_orm to set an event when each call's body has ended.

        A cancelled request leaves its get_orm thread running. A test waits
        on that call's event before reading what the thread changed and
        before it ends, so the thread never runs on after the test's patches
        are removed. Returns the events, one per call in the order the calls
        started; the real body runs in the same thread as before.
        """
        finished = []
        body = v.BackendDestroyFilesystem.__dict__["get_orm"].func

        def tracked(view, fs_id):
            done = threading.Event()
            finished.append(done)
            try:
                return body(view, fs_id)
            finally:
                done.set()

        patch = mock.patch.object(v.BackendDestroyFilesystem, "get_orm",
                                  sync_to_async(tracked))
        patch.start()
        self.addCleanup(patch.stop)
        return finished

    async def request(self):
        """One Destroy request, as Django handles it: a new view each time."""
        view = v.BackendDestroyFilesystem()
        view.request = mock.Mock()
        response = await view.get(view.request, 7)
        self.assertEqual(response.status_code, 302)
        return view.refused

    async def within(self, awaitable, what):
        """Await awaitable, failing the test if it takes over 10 seconds."""
        try:
            return await asyncio.wait_for(awaitable, 10)
        except TimeoutError:
            self.fail(f"{what} did not happen within 10 seconds")

    async def wait_for_tasks(self, tasks):
        """Wait for the destroy tasks to end; each was woken by go."""
        self.assertTrue(tasks, "no destroy task was started")
        _, pending = await asyncio.wait(tasks, timeout=10)
        self.assertEqual(pending, set(),
                         "a destroy task did not end within 10 seconds")
        self.assertEqual(self.woken, [True] * len(tasks))

    async def finish_first(self):
        self.go.set()
        await self.wait_for_tasks(self.tasks[:1])


class SecondDestroyTests(DestroyFlowMixin, SimpleTestCase):
    """A second Destroy, and every early exit, leave the next one possible."""

    def test_a_refused_request_creates_no_task_and_no_task_record(self):
        self.lock = {"ID": "17", "Who": "ofe@server", "Created": "t"}

        async def scenario():
            return await self.request()

        self.assertTrue(asyncio.run(scenario()))
        self.assertEqual(self.records, [])
        self.assertEqual(self.tasks, [])
        self.assertEqual(self.fs.cloud_state, "m")
        self.assertNotIn(7, v._running_destroys)
        self.assertIn("(lock ID 17)", self.messages[0])

    def test_a_second_request_while_the_first_runs_is_refused(self):
        async def scenario():
            first = await self.request()
            second = await self.request()  # no lock held yet
            await self.finish_first()
            return first, second

        self.assertEqual(asyncio.run(scenario()), (False, True))
        self.assertEqual(self.records, ["Destroy Filesystem"])
        self.assertEqual(self.messages, [
            "A Terraform operation is already running for this Filestore. "
            "Wait for it to finish."])

    def check_accepted_again_after_the_first_ends(self, error, state):
        self.error = error

        async def scenario():
            first = await self.request()
            await self.finish_first()
            self.assertNotIn(7, v._running_destroys)
            self.assertEqual(self.fs.cloud_state, state)
            again = await self.request()
            self.go.set()
            await self.wait_for_tasks(self.tasks)
            return first, again

        self.assertEqual(asyncio.run(scenario()), (False, False))
        self.assertEqual(len(self.records), 2)
        self.assertNotIn(7, v._running_destroys)

    def test_a_request_after_a_successful_destroy_is_accepted(self):
        self.check_accepted_again_after_the_first_ends(None, "xm")

    def test_a_request_after_a_failed_destroy_is_accepted(self):
        with self.assertLogs("ghpcfe.views.asyncview", "INFO"):
            self.check_accepted_again_after_the_first_ends(
                subprocess.CalledProcessError(1, "terraform"), "um")

    def test_a_request_refused_by_the_lock_does_not_block_the_next(self):
        self.lock = {"ID": "17", "Who": "ofe@server", "Created": "t"}

        async def scenario():
            refused = await self.request()
            self.lock = None  # the other operation finished
            accepted = await self.request()
            await self.finish_first()
            return refused, accepted

        self.assertEqual(asyncio.run(scenario()), (True, False))

    def test_a_failure_to_start_the_task_does_not_block_the_next(self):
        async def scenario():
            with mock.patch.object(v.BackendDestroyFilesystem,
                                   "get_user_token",
                                   mock.AsyncMock(side_effect=LookupError)):
                with self.assertRaises(LookupError):
                    await self.request()
            self.assertNotIn(7, v._running_destroys)
            self.assertEqual(self.tasks, [])
            accepted = await self.request()
            await self.finish_first()
            return accepted

        self.assertFalse(asyncio.run(scenario()))

    def test_an_error_while_accepting_does_not_block_the_next(self):
        self.objects.filter.return_value.update.side_effect = [
            RuntimeError("database"), 1]

        async def scenario():
            with self.assertRaises(RuntimeError):
                await self.request()
            self.assertNotIn(7, v._running_destroys)
            accepted = await self.request()
            await self.finish_first()
            return accepted

        self.assertFalse(asyncio.run(scenario()))

    def test_a_stuck_destroy_after_a_restart_is_accepted(self):
        # A restart leaves "dm" in the database, an empty set and no lock.
        self.fs.cloud_state = "dm"

        async def scenario():
            accepted = await self.request()
            await self.finish_first()
            return accepted

        self.assertFalse(asyncio.run(scenario()))
        self.assertEqual(self.fs.cloud_state, "xm")

    def test_a_disconnect_during_the_lock_read_does_not_block_the_next(self):
        # Django cancels the view when the client disconnects; get() gets
        # CancelledError at once while get_orm's thread runs on.
        entered = threading.Event()
        unblock = threading.Event()
        saved = threading.Event()

        def mark(**values):
            saved.set()
            return 1

        self.objects.filter.return_value.update.side_effect = mark
        finished = self.track_get_orm()
        # The lock read runs inside get_orm's catch of every exception, so
        # it records whether unblock was set and the test asserts it later.
        woke = []

        def slow_lock(fs):
            entered.set()
            woke.append(unblock.wait(10))
            return None

        async def scenario():
            with mock.patch.object(v.cm_fs, "running_state_lock", slow_lock):
                first = asyncio.ensure_future(self.request())
                self.assertTrue(await asyncio.to_thread(entered.wait, 10))
                first.cancel()
                with self.assertRaises(asyncio.CancelledError):
                    await self.within(first, "the cancel")
                # Released at once, while the thread still reads the lock.
                self.assertNotIn(7, v._running_destroys)
                unblock.set()
                # The abandoned thread still sets "dm", but starts no task.
                self.assertTrue(await asyncio.to_thread(saved.wait, 10))
                # The update returns before get_orm sets "dm" on the loaded
                # object, so wait for the whole call to end before reading
                # it.
                self.assertTrue(
                    await asyncio.to_thread(finished[0].wait, 10),
                    "the abandoned get_orm thread did not finish")
                self.assertEqual(woke, [True])
                self.assertEqual(self.fs.cloud_state, "dm")
                self.assertEqual(self.tasks, [])
                self.assertNotIn(7, v._running_destroys)
                accepted = await self.request()
                await self.finish_first()
                return accepted

        self.assertFalse(asyncio.run(scenario()))
        self.assertEqual(self.records, ["Destroy Filesystem"])
        self.assertEqual(self.messages, [])
        self.assertEqual(self.fs.cloud_state, "xm")
        self.assertNotIn(7, v._running_destroys)

    def test_a_disconnect_while_starting_the_task_does_not_block_the_next(
            self):
        async def scenario():
            entered = asyncio.Event()

            async def slow_token(user):
                entered.set()
                await asyncio.Event().wait()  # until the cancel

            with mock.patch.object(v.BackendDestroyFilesystem,
                                   "get_user_token",
                                   mock.AsyncMock(side_effect=slow_token)):
                first = asyncio.ensure_future(self.request())
                self.assertTrue(await self.within(
                    entered.wait(), "the request reaching the task start"))
                first.cancel()
                with self.assertRaises(asyncio.CancelledError):
                    await self.within(first, "the cancel")
            self.assertNotIn(7, v._running_destroys)
            self.assertEqual(self.tasks, [])
            accepted = await self.request()
            await self.finish_first()
            return accepted

        self.assertFalse(asyncio.run(scenario()))
        self.assertEqual(self.records, ["Destroy Filesystem"])

    def test_a_refusal_because_it_is_mounted_leaves_no_claim(self):
        self.fs.in_use.return_value = True
        self.fs.active_mounts.return_value = [NS(cluster=NS(name="c1"))]

        async def scenario():
            refused = await self.request()
            self.assertNotIn(7, v._running_destroys)
            # The cluster is destroyed, so nothing mounts the Filestore now.
            self.fs.in_use.return_value = False
            self.fs.active_mounts.return_value = []
            accepted = await self.request()
            await self.finish_first()
            return refused, accepted

        self.assertEqual(asyncio.run(scenario()), (True, False))
        self.assertEqual(self.messages, [
            "Cannot destroy - Filesystem is still mounted by: c1 (see below)"])
        self.assertEqual(self.records, ["Destroy Filesystem"])

    def test_claims_and_refusals_are_logged(self):
        async def scenario():
            await self.request()  # accepted
            await self.request()  # refused: the first is still running
            await self.finish_first()
            self.lock = {"ID": "17", "Who": "ofe@server", "Created": "t"}
            await self.request()  # refused: the state lock is held

        with self.assertLogs(v.logger, "INFO") as logs:
            asyncio.run(scenario())
        self.assertEqual([r.levelname for r in logs.records], ["INFO"] * 6)
        self.assertEqual([r.getMessage() for r in logs.records], [
            "Claimed the destroy of filesystem 7",
            "Refused the destroy of filesystem 7: a destroy started from "
            "this server is still running",
            "Released the destroy claim of filesystem 7",
            "Claimed the destroy of filesystem 7",
            "Refused the destroy of filesystem 7: its Terraform state is "
            "locked by ofe@server since t (lock ID 17)",
            "Released the destroy claim of filesystem 7",
        ])

    def test_a_refusal_because_it_is_mounted_is_logged(self):
        self.fs.in_use.return_value = True
        self.fs.active_mounts.return_value = [NS(cluster=NS(name="c1"))]
        self.fs.active_workbench_mounts.return_value = [
            NS(workbench=NS(name="wb1"))]

        with self.assertLogs(v.logger, "INFO") as logs:
            self.assertTrue(asyncio.run(self.request()))
        self.assertEqual([r.levelname for r in logs.records], ["INFO"] * 3)
        self.assertEqual([r.getMessage() for r in logs.records], [
            "Claimed the destroy of filesystem 7",
            "Refused the destroy of filesystem 7: still mounted by c1, wb1",
            "Released the destroy claim of filesystem 7",
        ])

    def test_an_error_while_accepting_logs_its_type_before_the_release(self):
        with mock.patch.object(v.BackendDestroyFilesystem, "get_user_token",
                               mock.AsyncMock(side_effect=LookupError)), \
                self.assertLogs(v.logger, "INFO") as logs, \
                self.assertRaises(LookupError):
            asyncio.run(self.request())
        self.assertEqual([r.getMessage() for r in logs.records], [
            "Claimed the destroy of filesystem 7",
            "The destroy request for filesystem 7 ended with LookupError "
            "before its task started",
            "Released the destroy claim of filesystem 7",
        ])
        self.assertEqual(logs.records[1].levelname, "INFO")

    def test_a_disconnect_logs_the_cancel_before_the_release(self):
        entered = threading.Event()
        unblock = threading.Event()
        finished = self.track_get_orm()
        woke = []  # asserted outside get_orm's catch of every exception

        def slow_lock(fs):
            entered.set()
            woke.append(unblock.wait(10))
            return None

        async def scenario():
            with mock.patch.object(v.cm_fs, "running_state_lock", slow_lock):
                first = asyncio.ensure_future(self.request())
                self.assertTrue(await asyncio.to_thread(entered.wait, 10))
                first.cancel()
                with self.assertRaises(asyncio.CancelledError):
                    await self.within(first, "the cancel")
                unblock.set()
                # The abandoned thread runs on; it must end inside the test,
                # while the patches it uses are still in place.
                self.assertTrue(
                    await asyncio.to_thread(finished[0].wait, 10),
                    "the abandoned get_orm thread did not finish")
                self.assertEqual(woke, [True])

        with self.assertLogs(v.logger, "INFO") as logs:
            asyncio.run(scenario())
        self.assertEqual([r.getMessage() for r in logs.records][:3], [
            "Claimed the destroy of filesystem 7",
            "The destroy request for filesystem 7 ended with CancelledError "
            "before its task started",
            "Released the destroy claim of filesystem 7",
        ])
        self.assertEqual(logs.records[1].levelname, "INFO")


class FilestoreRow:
    """The database row of filesystem 7, as the view's queries see it.

    get() returns a new instance on each call, as Filesystem.objects.get
    does. save() acts as Model.save: an UPDATE of the fields, and an INSERT
    when no row exists and update_fields is not given. update() acts as the
    conditional UPDATE: it changes the row only if every condition matches.
    """

    def __init__(self):
        self.fields = {"cloud_state": "m", "status_message": ""}
        self.exists = True
        self.inserts = 0
        self.lock = threading.Lock()

    def get(self, pk):
        with self.lock:
            fs = fs_double()
            fs.id = fs.pk = pk
            fs.cloud_state = self.fields["cloud_state"]
            fs.status_message = self.fields["status_message"]
        fs.save.side_effect = (
            lambda update_fields=None: self.save(fs, update_fields))
        return fs

    def save(self, fs, update_fields):
        with self.lock:
            if not self.exists:
                if update_fields:
                    raise RuntimeError("save() with update_fields did not "
                                       "affect any rows")
                self.exists = True
                self.inserts += 1
            for name in update_fields or self.fields:
                self.fields[name] = getattr(fs, name)

    def filter(self, pk, **conditions):
        def update(**values):
            with self.lock:
                if (not self.exists or pk != 7 or any(
                        self.fields[k] != want
                        for k, want in conditions.items())):
                    return 0
                self.fields.update(values)
                return 1
        return NS(update=update)


class LateAcceptTests(DestroyFlowMixin, SimpleTestCase):
    """A cancelled request's get_orm ending after a later destroy finished.

    Request A is cancelled while its get_orm thread reads the lock; the
    claim is released, request B is accepted and its destroy ends. Then A's
    thread resumes: it must change nothing, so B's result stands.
    """

    def setUp(self):
        super().setUp()
        self.row = FilestoreRow()
        self.entered = threading.Event()
        self.unblock = threading.Event()
        self.late_refusal = threading.Event()
        self.lock_reads = 0
        # Whether A's stalled read ended because unblock was set; run_late
        # asserts it, as the read runs inside get_orm's catch of every
        # exception.
        self.woke = []

        def lock(fs):
            self.lock_reads += 1
            if self.lock_reads == 1:  # A's read stalls
                self.entered.set()
                self.woke.append(self.unblock.wait(10))
            return None

        def show(req, lvl, text):
            self.messages.append(text)
            if text.startswith("This Filestore changed"):
                self.late_refusal.set()

        self.released = threading.Event()
        release = v._running_destroys.release

        def release_and_signal(fs_id):
            release(fs_id)
            self.released.set()

        patches = [
            mock.patch.object(v._running_destroys, "release",
                              release_and_signal),
            mock.patch.object(v.Filesystem.objects, "get", self.row.get),
            mock.patch.object(v.CloudResource.objects, "filter",
                              self.row.filter),
            mock.patch.object(v.cm_fs, "running_state_lock", lock),
            mock.patch.object(v.messages, "add_message", show),
        ]
        for patch in patches:
            patch.start()
            self.addCleanup(patch.stop)
        self.finished = self.track_get_orm()  # [0] is A's call

    async def request(self):
        # A context per request, as Django's ASGI handler gives each one,
        # so B's get_orm thread does not wait behind A's stalled one.
        async with ThreadSensitiveContext():
            return await super().request()

    def run_late(self, after_b=None):
        async def scenario():
            first = asyncio.ensure_future(self.request())
            self.assertTrue(await asyncio.to_thread(self.entered.wait, 10))
            first.cancel()
            # get() releases the claim on the cancel, while A's thread waits.
            self.assertTrue(await asyncio.to_thread(self.released.wait, 10))
            self.assertNotIn(7, v._running_destroys)
            accepted = await self.request()  # B
            await self.finish_first()
            if after_b:
                after_b()
            before = (dict(self.row.fields), self.row.exists)
            self.unblock.set()  # A's lock read returns
            self.assertTrue(
                await asyncio.to_thread(self.late_refusal.wait, 10),
                "A's late get_orm did not refuse")
            # The refusal message is shown before get_orm returns; wait for
            # A's call to end so its thread does not outlive the test.
            self.assertTrue(
                await asyncio.to_thread(self.finished[0].wait, 10),
                "A's abandoned get_orm thread did not finish")
            self.assertEqual(self.woke, [True])
            with self.assertRaises(asyncio.CancelledError):
                await self.within(first, "the cancel of A")
            self.assertEqual((dict(self.row.fields), self.row.exists),
                             before)
            return accepted

        with self.assertLogs(v.logger, "INFO") as logs:
            self.assertFalse(asyncio.run(scenario()))
        self.assertEqual(len(self.tasks), 1)  # only B's
        self.assertNotIn(7, v._running_destroys)
        self.assertIn(
            "Refused the destroy of filesystem 7: it changed while the "
            "request was being handled",
            [r.getMessage() for r in logs.records])
        return self.row

    def test_a_late_accept_after_a_successful_destroy_changes_nothing(self):
        row = self.run_late()
        self.assertEqual(row.fields, {"cloud_state": "xm",
                                      "status_message": ""})

    def test_a_late_accept_after_a_failed_destroy_keeps_its_message(self):
        self.error = subprocess.CalledProcessError(1, "terraform")
        with self.assertLogs("ghpcfe.views.asyncview", "INFO"):
            row = self.run_late()
        self.assertEqual(row.fields["cloud_state"], "um")
        self.assertTrue(row.fields["status_message"].startswith(
            "Teardown failed: "), row.fields)

    def test_a_late_accept_after_the_record_is_deleted_recreates_nothing(
            self):
        def delete():  # the Delete page, once B left it at "xm"
            self.assertEqual(self.row.fields["cloud_state"], "xm")
            self.row.exists = False

        row = self.run_late(after_b=delete)
        self.assertFalse(row.exists)
        self.assertEqual(row.inserts, 0)


class DestroyCommandTests(SimpleTestCase):
    """The destroy task: a failure leaves the Filestore retryable."""

    def run_cmd(self, error=None, status_message=""):
        fs = mock.MagicMock(cloud_state="dm", status_message=status_message)
        view = v.BackendDestroyFilesystem()
        with mock.patch.object(v.cm_fs, "destroy_filesystem",
                               side_effect=error):
            if error:
                with self.assertRaises(type(error)) as raised:
                    view.cmd(None, None, fs)
                self.assertIs(raised.exception, error)  # re-raised as is
            else:
                view.cmd(None, None, fs)
        return fs

    def check_failure_says_why(self, error):
        fs = self.run_cmd(error)
        self.assertEqual(fs.cloud_state, "um")
        # The underlying error, then what to do about it
        self.assertEqual(
            fs.status_message,
            f"Teardown failed: {error} Part of the Filestore may already be "
            "gone; destroy it again to finish.")
        self.assertIn(str(error), fs.status_message)
        fs.save.assert_called_once_with(
            update_fields=["cloud_state", "status_message"])

    def test_a_failed_destroy_can_be_retried_and_says_why(self):
        self.check_failure_says_why(
            subprocess.CalledProcessError(1, "terraform"))

    def test_a_failure_that_is_not_a_terraform_error_says_why(self):
        # Not every failure is a failed command: an unreadable credentials
        # file, a missing directory or a bug raise other exceptions.
        for error in (OSError("credentials file unreadable"),
                      RuntimeError("an unexpected failure")):
            with self.subTest(error=type(error).__name__):
                self.check_failure_says_why(error)

    def test_a_successful_destroy_clears_an_earlier_failure(self):
        fs = self.run_cmd(status_message="Teardown failed: an earlier run.")
        self.assertEqual((fs.cloud_state, fs.status_message), ("xm", ""))
        fs.save.assert_called_once_with()

    def check_released_after_the_save(self, error, state):
        fs = mock.MagicMock(id=7, cloud_state="dm", status_message="")
        running = v._RunningDestroys()
        self.assertTrue(running.claim(7))
        # What a request accepted at the moment of each save would see.
        seen = []
        fs.save.side_effect = lambda *a, **kw: seen.append(
            (fs.cloud_state, 7 in running))
        view = v.BackendDestroyFilesystem()
        with mock.patch.object(v, "_running_destroys", running), \
                mock.patch.object(v.cm_fs, "destroy_filesystem",
                                  side_effect=error):
            if error:
                with self.assertRaises(type(error)):
                    view.cmd(None, None, fs)
            else:
                view.cmd(None, None, fs)
        self.assertEqual(seen, [(state, True)])  # still claimed while saving
        self.assertNotIn(7, running)  # released once saved

    def test_the_claim_is_released_after_a_success_is_saved(self):
        self.check_released_after_the_save(None, "xm")

    def test_the_claim_is_released_after_a_failure_is_saved(self):
        self.check_released_after_the_save(
            subprocess.CalledProcessError(1, "terraform"), "um")

    def test_the_claim_is_released_after_any_other_failure_is_saved(self):
        self.check_released_after_the_save(
            OSError("credentials file unreadable"), "um")


CREDS = {"GOOGLE_APPLICATION_CREDENTIALS": "/dev/null"}


class DestroyNothingAppliedTests(SimpleTestCase):
    """A start that never applied anything leaves nothing to destroy."""

    def destroy(self, applied, tf_dir=None):
        from ..cluster_manager import filesystem
        fs = mock.MagicMock()
        if tf_dir is None:
            tf_dir = self.enterContext(tempfile.TemporaryDirectory())
        with mock.patch.object(filesystem, "_tf_dir_for_fs",
                               return_value=tf_dir), \
                mock.patch.object(filesystem, "_get_credentials_file",
                                  return_value="/dev/null"), \
                mock.patch.object(filesystem.utils,
                                  "terraform_may_have_applied",
                                  return_value=applied) as check, \
                mock.patch.object(filesystem.utils, "run_terraform") as run:
            filesystem.destroy_filesystem(fs)
        return run, check, tf_dir

    def test_nothing_applied_skips_terraform_destroy(self):
        run, _, tf_dir = self.destroy(False)
        # init still runs, so the check reads the bucket; no destroy follows
        self.assertEqual(run.call_args_list,
                         [mock.call(tf_dir, "init", extra_env=CREDS)])

    def test_anything_applied_is_destroyed(self):
        run, _, tf_dir = self.destroy(True)
        self.assertEqual(run.call_args_list, [
            mock.call(tf_dir, "init", extra_env=CREDS),
            mock.call(tf_dir, "destroy", extra_env=CREDS)])

    def test_a_missing_directory_still_skips(self):
        run, check, _ = self.destroy(True, tf_dir="/nowhere/primary")
        run.assert_not_called()
        check.assert_not_called()


class DestroyInitFirstTests(SimpleTestCase):
    """`terraform init` runs before the may-have-applied check.

    A failed start retry runs `ghpc create -w`, which moves the group
    directory, `.terraform` with it, into a backup. If it then fails before
    `terraform init`, the new directory has no record of the GCS backend,
    the check falls back to the local-state rule and says nothing was
    applied, and the destroy was skipped while the share still existed.
    """

    def setUp(self):
        from ..cluster_manager import filesystem
        self.filesystem = filesystem
        self.tf_dir = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.enterContext(mock.patch.object(
            filesystem, "_tf_dir_for_fs", return_value=self.tf_dir))
        self.enterContext(mock.patch.object(
            filesystem, "_get_credentials_file", return_value="/dev/null"))

    @staticmethod
    def fake_terraform(init_error=None):
        def run(target_dir, command, extra_env=None):
            if command == "init":
                if init_error:
                    raise init_error
                # What a real init records for the blueprint's GCS backend
                marker = Path(target_dir) / ".terraform" / "terraform.tfstate"
                marker.parent.mkdir()
                marker.write_text(json.dumps({"backend": {
                    "type": "gcs",
                    "config": {"bucket": "state", "prefix": "fs/4"}}}))
            return (None, None)
        return mock.Mock(side_effect=run)

    def test_a_directory_without_its_backend_record_is_destroyed(self):
        run = self.fake_terraform()
        state = {"resources": [{"type": "google_filestore_instance"}]}
        with mock.patch.object(self.filesystem.utils, "run_terraform", run), \
                mock.patch.object(self.filesystem.utils,
                                  "get_terraform_state",
                                  return_value=state) as pull:
            self.filesystem.destroy_filesystem(mock.MagicMock())
        pull.assert_called_once()  # the bucket's state was read
        self.assertEqual([c.args[1] for c in run.call_args_list],
                         ["init", "destroy"])

    def test_a_failed_init_fails_the_destroy_and_is_never_skipped(self):
        err = subprocess.CalledProcessError(1, "terraform init")
        run = self.fake_terraform(init_error=err)
        with mock.patch.object(self.filesystem.utils, "run_terraform", run), \
                mock.patch.object(self.filesystem.utils,
                                  "terraform_may_have_applied") as check:
            with self.assertRaises(subprocess.CalledProcessError):
                self.filesystem.destroy_filesystem(mock.MagicMock())
        check.assert_not_called()
        self.assertEqual([c.args[1] for c in run.call_args_list], ["init"])

    def test_a_failed_init_leaves_the_filestore_retryable(self):
        # Through the view's task: the failure is shown, not marked destroyed
        err = subprocess.CalledProcessError(1, "terraform init")
        fs = mock.MagicMock(cloud_state="dm", status_message="")
        with mock.patch.object(self.filesystem.utils, "run_terraform",
                               self.fake_terraform(init_error=err)):
            with self.assertRaises(subprocess.CalledProcessError):
                v.BackendDestroyFilesystem().cmd(None, None, fs)
        self.assertEqual(fs.cloud_state, "um")
        self.assertIn("Teardown failed", fs.status_message)


class RunningStateLockTests(SimpleTestCase):
    """Which state locks refuse a second destroy, read from the bucket."""

    LOCK = {"ID": "17", "Who": "ofe@server.example", "Created": "now"}

    def setUp(self):
        from ..cluster_manager import filesystem
        self.filesystem = filesystem
        self.fs = NS(id=4, name="home")
        for name, value in (("_filestore", self.fs),
                            ("_get_credentials_file", "/creds"),
                            ("_tf_dir_for_fs", "/fs/home/primary")):
            self.enterContext(mock.patch.object(filesystem, name,
                                                return_value=value))
        self.enterContext(mock.patch.object(
            filesystem.utils, "get_terraform_state_bucket",
            return_value="state"))
        self.enterContext(mock.patch.object(
            filesystem.utils, "terraform_state_prefix",
            return_value="ofe/fs/4"))
        self.enterContext(mock.patch.object(
            filesystem.socket, "gethostname", return_value="server"))

    def lock(self, reads, running=False):
        with mock.patch.object(self.filesystem.utils, "_read_gcs_state_lock",
                               side_effect=reads) as read, \
                mock.patch.object(self.filesystem.utils,
                                  "_terraform_running_in",
                                  return_value=running) as scan:
            result = self.filesystem.running_state_lock(self.fs)
        return result, read, scan

    def test_a_free_lock_is_none(self):
        result, read, _ = self.lock([None])
        self.assertIsNone(result)
        read.assert_called_once_with("state", "ofe/fs/4", "/creds")

    def test_a_lock_held_here_by_a_running_terraform_is_returned(self):
        result, _, scan = self.lock([self.LOCK], running=True)
        self.assertEqual(result, self.LOCK)
        scan.assert_called_once_with("/fs/home/primary")

    def test_a_lock_taken_on_another_machine_is_returned(self):
        other = dict(self.LOCK, Who="ofe@old-server")
        result, _, scan = self.lock([other])
        self.assertEqual(result, other)
        scan.assert_not_called()

    def test_a_stale_lock_left_by_a_killed_run_is_none(self):
        with self.assertLogs(self.filesystem.logger, "WARNING"):
            result, read, _ = self.lock([self.LOCK, dict(self.LOCK)])
        self.assertIsNone(result)
        self.assertEqual(read.call_count, 2)

    def test_a_lock_released_during_the_check_is_none(self):
        result, _, _ = self.lock([self.LOCK, None])
        self.assertIsNone(result)

    def test_a_lock_taken_again_during_the_check_is_returned(self):
        newer = dict(self.LOCK, ID="18")
        result, _, _ = self.lock([self.LOCK, newer])
        self.assertEqual(result, newer)

    def test_a_filesystem_that_is_not_a_filestore_has_no_lock(self):
        with mock.patch.object(self.filesystem, "_filestore",
                               return_value=None):
            result, read, _ = self.lock([self.LOCK])
        self.assertIsNone(result)
        read.assert_not_called()

    def test_a_read_failure_is_raised_for_the_view_to_handle(self):
        with self.assertRaises(OSError):
            self.lock(OSError("bucket unreachable"))


def filestore_row(cloud_state, applied):
    return NS(pk=4, id=4, name="home", cloud_state=cloud_state,
              cmek_applied=applied)


class DeleteGuardTests(SimpleTestCase):
    """Deleting only the record is refused while cloud resources may exist."""

    def test_which_records_may_hold_cloud_resources(self):
        cases = {("nm", False): False, ("nm", True): True,
                 ("i", False): False, ("xm", True): False,
                 ("cm", True): True, ("m", True): True,
                 ("dm", True): True, ("um", True): True,
                 # deployed before the flag existed
                 ("m", False): True}
        for (state, applied), want in cases.items():
            with self.subTest(state=state, applied=applied):
                self.assertIs(v.may_hold_cloud_resources(
                    filestore_row(state, applied)), want)

    def call(self, method, row):
        from django.test import RequestFactory
        request = getattr(RequestFactory(), method)("/")
        view = v.FilesystemDeleteView()
        view.setup(request, pk=row.pk)
        with mock.patch.object(view, "get_object", return_value=row), \
                mock.patch.object(v.messages, "error") as error, \
                mock.patch.object(v.DeleteView, method,
                                  return_value="deleted") as base:
            response = getattr(view, method)(request, pk=row.pk)
        return response, base, error

    def test_a_failed_apply_is_not_deleted_from_the_database(self):
        for method in ("get", "post"):
            with self.subTest(method=method):
                response, base, error = self.call(
                    method, filestore_row("nm", True))
                base.assert_not_called()
                self.assertEqual(response.status_code, 302)
                self.assertTrue(response.url.endswith("/filesystem/detail/4"))
                self.assertIn("Destroy it instead", error.call_args[0][1])

    def test_a_never_applied_record_is_still_deleted(self):
        response, base, error = self.call("post", filestore_row("nm", False))
        self.assertEqual(response, "deleted")
        base.assert_called_once()
        error.assert_not_called()


class DetailButtonsTests(SimpleTestCase):
    """The detail page offers Destroy, not Delete, once an apply started."""

    def buttons(self, cloud_state, applied):
        from django.template import engines
        src = (ROOT / "templates/filesystem/filestore_detail.html").read_text()
        start = src.index("{% comment %}\n      Once an apply")
        end = src.index("<hr>", start)
        page = engines["django"].from_string(
            src[start:end]).render({"object": filestore_row(cloud_state,
                                                            applied)})
        return {name for name in ("Delete", "Start Filestore",
                                  "Destroy Filestore") if f">{name}<" in page}

    def notice(self, cloud_state):
        from django.template import engines
        src = (ROOT / "templates/filesystem/filestore_detail.html").read_text()
        start = src.index('{% comment %}\n      "dm" is a destroy')
        end = src.index("{% comment %}\n      Once an apply", start)
        page = engines["django"].from_string(src[start:end]).render(
            {"object": filestore_row(cloud_state, True)})
        return {name for name, mark in (("destroyed", "is destroyed"),
                                        ("Edit", ">Edit<")) if mark in page}

    def test_a_running_destroy_is_not_called_destroyed(self):
        cases = {"dm": set(), "xm": {"destroyed"}, "m": {"Edit"},
                 "um": {"Edit"}, "nm": {"Edit"}}
        for state, want in cases.items():
            with self.subTest(state=state):
                self.assertEqual(self.notice(state), want)

    def test_the_comment_names_the_refusal_not_a_lock_failure(self):
        src = (ROOT / "templates/filesystem/filestore_detail.html").read_text()
        self.assertNotIn("fails\n      on the Terraform state lock", src)
        self.assertIn("refused while the Filestore's Terraform state lock",
                      src)
        # and the in-process guard, which covers the init before the lock
        self.assertIn("while a destroy started from this server runs", src)
        self.assertIn("so a restart clears it", src)

    def test_buttons_for_each_state(self):
        cases = {
            ("nm", False): {"Delete", "Start Filestore"},
            ("nm", True): {"Start Filestore", "Destroy Filestore"},
            ("m", True): {"Destroy Filestore"},
            ("um", True): {"Destroy Filestore"},
            ("dm", True): {"Destroy Filestore"},
            ("cm", True): set(),
            ("xm", True): set(),
        }
        for (state, applied), want in cases.items():
            with self.subTest(state=state, applied=applied):
                self.assertEqual(self.buttons(state, applied), want)
