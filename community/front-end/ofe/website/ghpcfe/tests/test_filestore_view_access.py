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
"""Who can reach the filesystem pages, and for which filesystems.

The Filestore detail, create and edit pages checked only that someone was
signed in, so any user could create or change a Filestore (its encryption,
key ring, network and zone) by typing its URL, although every other
filesystem page is for administrators only. The imported-filesystem edit page
looked a filesystem up by its pk alone, so it rewrote a Filestore's zone,
address and share name through a form meant for imported filesystems.
"""
import functools
from unittest import mock

from django.contrib.auth.models import AnonymousUser
from django.core.handlers.exception import convert_exception_to_response
from django.http import Http404
from django.test import RequestFactory, SimpleTestCase

from ..models import (
    Filesystem, FilesystemExport, FilesystemImpl, GCPFilestoreFilesystem,
)
from ..permissions import SuperUserRequiredMixin
from ..views import filesystems
from ..views import gcpfilestore

FILESTORE_PAGES = (
    (gcpfilestore.GCPFilestoreFilesystemDetailView,
     "/filesystem/filestore/detail/3", {"pk": 3}),
    (gcpfilestore.GCPFilestoreFilesystemUpdateView,
     "/filesystem/filestore/edit/3", {"pk": 3}),
    (gcpfilestore.GCPFilestoreFilesystemCreateView,
     "/filesystem/filestore/create/", {"credential": 1}),
)


def _user(superuser):
    return mock.Mock(is_authenticated=True, is_superuser=superuser)


class FilestorePagesAreForAdministratorsTests(SimpleTestCase):

    def _dispatch(self, view, path, kwargs, method, user,
                  stub_handler=False, refused=False):
        """Run a request through the view as Django serves it, with every
        lookup and write it could make recorded instead."""
        request = getattr(RequestFactory(), method)(
            path, {"name": "renamed", "share_name": "/other",
                   "cmek_enabled": "on"})
        request.user = user
        touched = {}
        patches = [
            mock.patch.object(view, "get_object"),
            mock.patch.object(gcpfilestore, "get_object_or_404"),
            mock.patch.object(GCPFilestoreFilesystem, "save"),
            mock.patch.object(FilesystemExport, "save"),
            mock.patch.object(gcpfilestore.FilestoreForm, "save"),
        ]
        if stub_handler:
            patches.append(mock.patch.object(
                view, method, return_value=mock.Mock(status_code=200)))
        mocks = [patcher.start() for patcher in patches]
        try:
            # as the request handler wraps a view: PermissionDenied is a 403
            handler = convert_exception_to_response(
                functools.partial(view.as_view(), **kwargs))
            if refused:
                # Django logs each refusal as a warning
                with self.assertLogs("django.request", "WARNING"):
                    response = handler(request)
            else:
                response = handler(request)
        finally:
            for patcher in patches:
                patcher.stop()
        for patcher, recorded in zip(patches, mocks):
            touched[patcher.attribute] = recorded
        return response, touched

    def test_a_signed_in_user_who_is_not_an_administrator_is_refused(self):
        for view, path, kwargs in FILESTORE_PAGES:
            for method in ("get", "post"):
                with self.subTest(view=view.__name__, method=method):
                    response, touched = self._dispatch(
                        view, path, kwargs, method, _user(superuser=False),
                        refused=True)
                    self.assertEqual(response.status_code, 403)
                    # nothing was looked up, let alone saved
                    for name, recorded in touched.items():
                        with self.subTest(touched=name):
                            recorded.assert_not_called()

    def test_an_anonymous_request_goes_to_the_login_page(self):
        for view, path, kwargs in FILESTORE_PAGES:
            for method in ("get", "post"):
                with self.subTest(view=view.__name__, method=method):
                    response, touched = self._dispatch(
                        view, path, kwargs, method, AnonymousUser())
                    self.assertEqual(response.status_code, 302)
                    self.assertIn("/login", response["Location"])
                    self.assertIn("next=", response["Location"])
                    for name, recorded in touched.items():
                        with self.subTest(touched=name):
                            recorded.assert_not_called()

    def test_an_administrator_reaches_the_page(self):
        for view, path, kwargs in FILESTORE_PAGES:
            for method in ("get", "post"):
                with self.subTest(view=view.__name__, method=method):
                    if not hasattr(view, method):
                        # the detail page takes no POST from anyone
                        response, _ = self._dispatch(
                            view, path, kwargs, method,
                            _user(superuser=True), refused=True)
                        self.assertEqual(response.status_code, 405)
                        continue
                    response, touched = self._dispatch(
                        view, path, kwargs, method, _user(superuser=True),
                        stub_handler=True)
                    self.assertEqual(response.status_code, 200)
                    touched[method].assert_called_once()

    def test_the_check_runs_before_the_view(self):
        for view, _, _ in FILESTORE_PAGES:
            with self.subTest(view=view.__name__):
                mro = view.__mro__
                self.assertLess(mro.index(SuperUserRequiredMixin),
                                mro.index(gcpfilestore.generic.View))


class FakeQuerySet(list):
    """The few queryset calls an UpdateView makes, over a list of rows."""

    model = Filesystem

    def all(self):
        return self

    def filter(self, **conditions):
        return FakeQuerySet(row for row in self if all(
            getattr(row, name) == value
            for name, value in conditions.items()))

    def get(self):
        if len(self) != 1:
            raise Filesystem.DoesNotExist
        return self[0]


class ImportedFilesystemEditTests(SimpleTestCase):

    ROWS = {
        FilesystemImpl.BUILT_IN: 2,
        FilesystemImpl.GCPFILESTORE: 3,
        FilesystemImpl.IMPORTED: 4,
    }

    def _object(self, pk):
        rows = FakeQuerySet(
            Filesystem(pk=row_pk, name=f"fs{row_pk}", impl_type=impl)
            for impl, row_pk in self.ROWS.items())
        view = filesystems.FilesystemImportUpdateView()
        view.setup(RequestFactory().get(f"/filesystem/import/{pk}/edit"),
                   pk=pk)
        view.model = mock.Mock(_default_manager=rows)
        return view.get_object()

    def test_an_imported_filesystem_is_found(self):
        found = self._object(self.ROWS[FilesystemImpl.IMPORTED])
        self.assertEqual(found.impl_type, FilesystemImpl.IMPORTED)

    def test_any_other_filesystem_is_not_found(self):
        for impl in (FilesystemImpl.GCPFILESTORE, FilesystemImpl.BUILT_IN):
            with self.subTest(impl=impl.label):
                with self.assertRaises(Http404):
                    self._object(self.ROWS[impl])

    def test_the_page_filters_on_the_imported_type(self):
        view = filesystems.FilesystemImportUpdateView()
        sql = str(view.get_queryset().query)
        self.assertIn(" WHERE ", sql)
        where = sql.split(" WHERE ", 1)[1]
        self.assertIn("impl_type", where)
        self.assertIn(str(int(FilesystemImpl.IMPORTED)), where)
