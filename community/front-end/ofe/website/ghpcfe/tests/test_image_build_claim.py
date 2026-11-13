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
"""An image is built once, by one request, and never left "being created".

Nothing set the "being created" status, so an image stayed "newly
configured" for its whole build and a second request for the build page
(a reload, a second tab) started another build into the same directory. The
build page now claims the image in one conditional UPDATE, and every way the
build can end without a ready image leaves it failed, with a reason.
"""
import asyncio
import subprocess
import threading
from contextlib import ExitStack
from types import SimpleNamespace
from unittest import mock

from django.test import SimpleTestCase

from ..cluster_manager import cmek
from ..models import Image
from ..views import images as v
from .test_cmek_applied_flows import _image, _image_backend

STATUS_TEXT = dict(Image.IMAGE_STATUS)


class StoredImage:
    """Image.objects over one stored image.

    Each UPDATE checks its condition and writes under one lock, as the
    database runs a single UPDATE ... WHERE. `before_read` runs on each
    read, so a test can line two requests up there.
    """

    def __init__(self, status, before_read=None):
        self.status = status
        self.before_read = before_read
        self.lock = threading.Lock()
        self.conditions = []

    def filter(self, pk, status):
        stored = self
        self.conditions.append(status)

        class Rows:
            def update(self, **values):
                with stored.lock:
                    if stored.status != status:
                        return 0
                    stored.status = values["status"]
                    return 1

        return Rows()

    def get(self, pk):
        if self.before_read:
            self.before_read()
        status = self.status
        return SimpleNamespace(
            pk=pk, status=status, cloud_credential=None,
            get_status_display=lambda: STATUS_TEXT[status])


def _claim(stored):
    """What one request for the build page decides: (refused, message)."""
    view = v.BackendCreateImage()
    view.request = mock.Mock()
    with mock.patch("django.contrib.messages.add_message") as message:
        # the function under sync_to_async, so threads really overlap
        v.BackendCreateImage.__dict__["get_orm"].func(view, 7)
    text = message.call_args.args[2] if message.called else None
    return view.refused, text


class ClaimTests(SimpleTestCase):

    def setUp(self):
        patcher = mock.patch.object(v.Image, "objects")
        self.objects = patcher.start()
        self.addCleanup(patcher.stop)

    def _use(self, stored):
        self.objects.filter.side_effect = stored.filter
        self.objects.get.side_effect = stored.get

    def test_the_claim_is_one_conditional_update(self):
        stored = StoredImage("n")
        self._use(stored)
        self.assertEqual(_claim(stored), (False, None))
        self.assertEqual(stored.conditions, ["n"])
        self.assertEqual(stored.status, "c")

    def test_a_second_request_after_the_first_is_refused(self):
        stored = StoredImage("n")
        self._use(stored)
        self.assertEqual(_claim(stored), (False, None))
        refused, text = _claim(stored)
        self.assertIs(refused, True)
        self.assertEqual(
            text, "Cannot build - the image is not newly configured "
            f"({STATUS_TEXT['c']}).")
        self.assertEqual(stored.status, "c")

    def test_two_requests_at_once_start_one_build(self):
        # both requests read the image before either has decided
        both_reading = threading.Barrier(2, timeout=5)
        stored = StoredImage("n", before_read=both_reading.wait)
        self._use(stored)
        results = []

        def request():
            results.append(_claim(stored))

        threads = [threading.Thread(target=request) for _ in range(2)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(10)
        self.assertEqual(len(results), 2)
        self.assertEqual(sorted(refused for refused, _ in results),
                         [False, True])
        self.assertEqual(stored.status, "c")

    def test_two_requests_at_once_without_a_pause_start_one_build(self):
        for _ in range(20):
            stored = StoredImage("n")
            self._use(stored)
            start = threading.Barrier(2, timeout=5)
            results = []

            def request(stored=stored, start=start, results=results):
                start.wait()
                results.append(_claim(stored))

            threads = [threading.Thread(target=request) for _ in range(2)]
            for thread in threads:
                thread.start()
            for thread in threads:
                thread.join(10)
            self.assertEqual(sorted(refused for refused, _ in results),
                             [False, True])

    def test_an_image_in_any_other_state_is_not_claimed(self):
        for status in ("c", "r", "e"):
            with self.subTest(status=status):
                stored = StoredImage(status)
                self._use(stored)
                refused, text = _claim(stored)
                self.assertIs(refused, True)
                self.assertIn(STATUS_TEXT[status], text)
                self.assertEqual(stored.status, status)

    def test_a_task_that_cannot_be_started_releases_the_claim(self):
        stored = StoredImage("n")
        self._use(stored)
        view = v.BackendCreateImage()
        request = mock.Mock()
        request.user.is_authenticated = True
        view.request = request
        with mock.patch.object(view, "test_user_is_cluster_admin",
                               mock.AsyncMock()), \
                mock.patch.object(view, "create_task", mock.AsyncMock(
                    side_effect=RuntimeError("no API token"))), \
                self.assertRaises(RuntimeError):
            asyncio.run(view.get(request, 7))
        # no build runs, so it can be built again
        self.assertEqual(stored.status, "n")


class NeverLeftBeingCreatedTests(SimpleTestCase):
    """Every build that does not end ready ends failed, with a reason."""

    STEPS = ("_create_image_dir", "_create_blueprint", "_run_ghpc",
             "_create_cmek_env", "_create_builder_env", "_create_image",
             "_destroy_builder_env")

    def _prepare(self, failing_step, error):
        backend = _image_backend(_image(status="c"))
        with ExitStack() as stack:
            for name in self.STEPS:
                stack.enter_context(mock.patch.object(
                    backend, name,
                    side_effect=error if name == failing_step else None))
            stack.enter_context(self.assertRaises(type(error)))
            backend.prepare()
        return backend.image

    def test_an_unexpected_error_in_any_step(self):
        for step in self.STEPS:
            with self.subTest(step=step):
                img = self._prepare(step, RuntimeError("disk full"))
                self.assertEqual(img.status, "e")
                self.assertEqual(img.status_message,
                                 "The image build failed: disk full")

    def test_a_tool_failure_that_only_set_the_status(self):
        # gcluster and terraform failures mark the image failed but say
        # nothing on its page
        from ..cluster_manager import image
        backend = _image_backend(_image(status="c"))
        backend.ghpc_path = "gcluster"
        with ExitStack() as stack:
            for name in ("_create_image_dir", "_create_blueprint"):
                stack.enter_context(mock.patch.object(backend, name))
            stack.enter_context(mock.patch.object(
                image.subprocess, "run",
                side_effect=subprocess.CalledProcessError(1, "gcluster")))
            stack.enter_context(self.assertRaises(
                subprocess.CalledProcessError))
            stack.enter_context(self.assertLogs(level="ERROR"))
            backend.prepare()
        img = backend.image
        self.assertEqual(img.status, "e")
        self.assertTrue(img.status_message.startswith(
            "The image build failed: "))

    def test_a_reason_a_step_gave_is_kept(self):
        backend = _image_backend(_image(status="c"))

        def fail_with_reason():
            backend._fail("Could not create the image directory: denied")
            raise OSError("denied")

        with ExitStack() as stack:
            for name in self.STEPS:
                stack.enter_context(mock.patch.object(backend, name))
            backend._create_image_dir.side_effect = fail_with_reason
            stack.enter_context(self.assertRaises(OSError))
            backend.prepare()
        self.assertEqual(backend.image.status, "e")
        self.assertEqual(backend.image.status_message,
                         "Could not create the image directory: denied")

    def test_a_backend_that_cannot_be_set_up(self):
        img = _image(status="c")
        with mock.patch.object(v, "ImageBackend",
                               side_effect=KeyError("baseDir")), \
                self.assertRaises(KeyError):
            v.BackendCreateImage().cmd(None, None, img, None)
        self.assertEqual(img.status, "e")
        self.assertEqual(img.status_message,
                         "The image build failed: 'baseDir'")
        self.assertIn(["status", "status_message"], img.saves)

    def test_a_revoked_image_is_released_not_failed(self):
        backend = _image_backend(_image(enabled=True, status="c",
                                        cmek_revoked=True))
        with mock.patch.object(backend, "_create_image_dir") as create, \
                self.assertRaises(cmek.CmekConfigError):
            backend.prepare()
        create.assert_not_called()
        img = backend.image
        # refused and nothing changed: newly configured again
        self.assertEqual(img.status, "n")
        self.assertIn("was revoked", img.status_message)
        self.assertEqual(img.saves, [["status", "status_message"]])

    def test_an_unexpected_error_in_the_revoked_check(self):
        backend = _image_backend(_image(status="c"))
        with mock.patch.object(cmek, "assert_not_revoked",
                               side_effect=RuntimeError("db gone")), \
                mock.patch.object(backend, "_create_image_dir") as create, \
                self.assertRaises(RuntimeError):
            backend.prepare()
        create.assert_not_called()
        self.assertEqual(backend.image.status, "e")
        self.assertEqual(backend.image.status_message,
                         "The image build failed: db gone")
