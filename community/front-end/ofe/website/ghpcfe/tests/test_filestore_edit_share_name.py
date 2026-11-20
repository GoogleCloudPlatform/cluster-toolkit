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
"""The Filestore edit page takes the share name from the form, not the POST.

Once a Filestore is deployed the form disables its share name, so a browser
does not send it: reading the raw POST failed with a server error after the
record was already saved, and a hand-made POST could rename the export of a
deployed Filestore.
"""
import contextlib
from types import SimpleNamespace as NS
from unittest import mock

from django.db import models as django_models
from django.test import RequestFactory, SimpleTestCase

from ..cluster_manager import cmek
from ..models import Credential, GCPFilestoreFilesystem, VirtualNetwork
from ..views import gcpfilestore as v
from .test_cmek_panel import settings_for


def queryset_of(obj):
    """A stand-in queryset for a choice field that finds `obj` by its pk."""
    queryset = mock.MagicMock(model=type(obj), **{"get.return_value": obj})
    queryset.all.return_value = queryset
    return queryset


class FilestoreEditShareNameTests(SimpleTestCase):
    """Drive the real update view and the real FilestoreForm, no database."""

    def post(self, cloud_state, data):
        creds = Credential(pk=1, name="c", detail="{}")
        vpc = VirtualNetwork(pk=2, name="net")
        filestore = GCPFilestoreFilesystem(
            pk=3, name="fs", cloud_credential=creds, vpc=vpc,
            cloud_zone="europe-west4-a", cloud_state=cloud_state,
            capacity=1024, performance_tier="bh")
        export = mock.Mock(export_name="/data")
        post = {"name": "fs", "cloud_credential": "1", "vpc": "2",
                "cloud_zone": "europe-west4-a", "capacity": "1024",
                "performance_tier": "bh", **data}

        view = v.GCPFilestoreFilesystemUpdateView()
        view.setup(RequestFactory().post("/edit/3", post), pk=3)
        view.object = filestore
        patches = (
            mock.patch.object(view, "get_object", return_value=filestore),
            mock.patch.object(GCPFilestoreFilesystem, "exports",
                              NS(first=lambda: export)),
            mock.patch.object(GCPFilestoreFilesystem, "save"),
            mock.patch("ghpcfe.forms.utils.load_config", return_value={}),
            mock.patch.object(cmek.CmekSettings, "from_config",
                              return_value=settings_for()),
            mock.patch("ghpcfe.forms.cloud_info.get_region_zone_info",
                       return_value={"europe-west4": ["europe-west4-a"]}),
            mock.patch("ghpcfe.forms.VirtualNetwork"),
            # the foreign keys' existence checks are database lookups
            mock.patch.object(django_models.ForeignKey, "validate"),
        )
        with contextlib.ExitStack() as stack:
            saved = [stack.enter_context(p) for p in patches][2]
            form = view.get_form()
            form.fields["cloud_credential"].queryset = queryset_of(creds)
            form.fields["vpc"].queryset = queryset_of(vpc)
            self.assertTrue(form.is_valid(), form.errors)
            response = view.form_valid(form)
        return response, export, saved

    def test_a_browser_post_without_the_disabled_share_name_saves(self):
        response, export, saved = self.post("m", {})
        self.assertEqual(response.status_code, 302)
        saved.assert_called_once()
        self.assertEqual(export.export_name, "/data")
        export.save.assert_called_once()

    def test_a_forged_share_name_on_a_deployed_filestore_is_ignored(self):
        response, export, _ = self.post("m", {"share_name": "/forged"})
        self.assertEqual(response.status_code, 302)
        self.assertEqual(export.export_name, "/data")

    def test_an_undeployed_filestore_takes_its_share_name_from_the_post(self):
        response, export, _ = self.post("nm", {"share_name": "/renamed"})
        self.assertEqual(response.status_code, 302)
        self.assertEqual(export.export_name, "/renamed")
        export.save.assert_called_once()
