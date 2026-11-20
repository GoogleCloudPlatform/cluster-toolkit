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
"""Errors about the encryption setup are reported, not guessed around.

Whether a key ring exists decides between key_ring_name and key_ring_id in the
blueprint, so an answer Cloud KMS could not give is an error with a reason
rather than a "no". And deployment settings that fail to parse are named on
each detail page rather than dropping its Encryption section.
"""
from types import SimpleNamespace
from unittest import mock

import httplib2
from django.template.loader import render_to_string
from django.test import SimpleTestCase
from googleapiclient.errors import HttpError

from ..cluster_manager import cloud_info, utils
from ..views import view_utils

RING = "projects/keys-proj/locations/europe-west4/keyRings/ofe-cmek"


def http_error(status, reason="Forbidden"):
    return HttpError(httplib2.Response({"status": status, "reason": reason}),
                     b"{}")


class KeyRingExistsTests(SimpleTestCase):
    """kms_key_ring_exists against a Cloud KMS client double."""

    def check(self, outcome):
        client = mock.MagicMock()
        get = client.projects.return_value.locations.return_value \
            .keyRings.return_value.get
        get.return_value.execute.side_effect = outcome
        with mock.patch.object(cloud_info, "_get_gcp_client",
                               return_value=("p", client)):
            result = cloud_info.kms_key_ring_exists(
                "{}", "keys-proj", "europe-west4", "ofe-cmek")
        get.assert_called_once_with(name=RING)
        return result

    def test_a_ring_that_is_there_exists(self):
        self.assertIs(self.check([{"name": RING}]), True)

    def test_only_a_404_means_it_does_not(self):
        self.assertIs(self.check(http_error(404, "Not Found")), False)

    def test_a_403_says_what_to_fix(self):
        with self.assertRaises(cloud_info.KeyRingCheckError) as ctx:
            self.check(http_error(403))
        self.assertEqual(ctx.exception.status, 403)
        self.assertEqual(
            str(ctx.exception),
            f"Cannot check the Cloud KMS key ring {RING}: the Cloud KMS API "
            "is disabled in project keys-proj or this credential lacks "
            "cloudkms.keyRings.get. Enable the API / grant the permission, "
            "or ask an administrator.")

    def test_a_server_error_names_its_status(self):
        with self.assertRaises(cloud_info.KeyRingCheckError) as ctx:
            self.check(http_error(500, "Internal Server Error"))
        self.assertEqual(ctx.exception.status, 500)
        self.assertIn(f"Cannot check the Cloud KMS key ring {RING}",
                      str(ctx.exception))
        self.assertIn("HTTP 500", str(ctx.exception))

    def test_anything_but_an_http_error_propagates(self):
        with self.assertRaises(ConnectionError):
            self.check(ConnectionError("no route"))


class InvalidSettingsDetailTests(SimpleTestCase):
    """The detail pages' Encryption section when the settings cannot be read."""

    BAD = {"cmek": {"key_project_id": "Bad_Project!"}}

    def detail(self, resource, config):
        with mock.patch.object(utils, "load_config",
                               return_value=config):
            return view_utils.cmek_detail_context(resource)

    def test_the_section_says_the_settings_are_invalid(self):
        detail = self.detail(SimpleNamespace(cmek_enabled=True), self.BAD)
        self.assertIsNone(detail["encrypted"])
        self.assertIn("Bad_Project!", detail["invalid_settings"])
        page = " ".join(render_to_string(
            "cmek/_detail.html", {"cmek_detail": detail}).split())
        self.assertIn("Encryption settings for this deployment are invalid: "
                      f"{detail['invalid_settings']}. An administrator must "
                      "correct them on the Encryption settings page.",
                      page.replace("&#x27;", "'"))
        self.assertNotIn("Google-managed", page)

    def test_a_resource_that_cannot_carry_a_key_still_shows_nothing(self):
        self.assertIsNone(self.detail(SimpleNamespace(), self.BAD))
        self.assertEqual(
            render_to_string("cmek/_detail.html",
                             {"cmek_detail": None}).strip(), "")
