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
from pathlib import Path
from types import SimpleNamespace as NS
from unittest import mock

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


class DestroyRefusalTests(SimpleTestCase):
    def run_get_orm(self, fs):
        view = v.BackendDestroyFilesystem()
        view.request = mock.Mock()
        with mock.patch.object(v.Filesystem.objects, "get", return_value=fs), \
                mock.patch.object(v.messages, "add_message") as msg:
            asyncio.run(view.get_orm(1))
        return view, msg

    def test_refused_when_a_live_cluster_mounts_it(self):
        fs = fs_double(clusters=["c1"])
        view, msg = self.run_get_orm(fs)
        self.assertTrue(view.refused)
        self.assertEqual(fs.cloud_state, "m")  # state unchanged
        fs.save.assert_not_called()
        self.assertIn("c1", msg.call_args[0][2])

    def test_refused_when_a_workbench_mounts_it(self):
        fs = fs_double(workbenches=["wb1"])
        view, msg = self.run_get_orm(fs)
        self.assertTrue(view.refused)
        fs.save.assert_not_called()
        self.assertIn("wb1", msg.call_args[0][2])

    def test_allowed_when_nothing_live_mounts_it(self):
        # the model query excludes destroyed users, so none are returned
        fs = fs_double()
        view, msg = self.run_get_orm(fs)
        self.assertFalse(view.refused)
        self.assertEqual(fs.cloud_state, "dm")
        fs.save.assert_called_once()
        msg.assert_not_called()

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
        with mock.patch.object(view, "get_orm", fake_get_orm), \
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
