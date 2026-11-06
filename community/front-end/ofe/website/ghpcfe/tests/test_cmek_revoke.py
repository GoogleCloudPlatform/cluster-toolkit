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
"""Revoking a key destroys exactly that key, and all of it.

The revoke page used to be addressed by the key's short name, which is unique
only within one key ring: two resources whose keys share a name in different
projects or key rings reached the same page, and the first match was the one
destroyed. It is now addressed by the resource, and the POST carries the full
key path the admin confirmed.

Database-free like the rest of the suite: this app commits no migrations, so the
model managers are replaced by an in-memory stand-in that applies the same
filter() and exclude() the view asks for.
"""
from types import SimpleNamespace as NS
from unittest import mock

from django.core.exceptions import PermissionDenied
from django.http import Http404
from django.template.loader import render_to_string
from django.test import RequestFactory, SimpleTestCase
from django.urls import resolve, reverse

from ..cluster_manager import cloud_info
from ..models import Cluster, GCPFilestoreFilesystem, Image
from ..views import cmek as v
from .test_cmek_panel import PanelForm, settings_for

ADMIN = NS(is_authenticated=True, is_superuser=True, is_staff=True)
STAFF = NS(is_authenticated=True, is_superuser=False, is_staff=True)

PATH_A = ("projects/proj-a/locations/europe-west4/keyRings/ring-a"
          "/cryptoKeys/cluster-abc")
PATH_B = ("projects/proj-b/locations/europe-west4/keyRings/ring-b"
          "/cryptoKeys/cluster-abc")


class Rows(list):
    """Just enough of a QuerySet for the live-row filter, over plain rows."""

    @staticmethod
    def _matches(row, lookups):
        return all(getattr(row, k) == want for k, want in lookups.items())

    def filter(self, **lookups):
        return Rows(r for r in self if self._matches(r, lookups))

    def exclude(self, **lookups):
        return Rows(r for r in self if not self._matches(r, lookups))

    def first(self):
        return self[0] if self else None


def cluster(pk, project, ring, key="cluster-abc", **fields):
    row = NS(pk=pk, name=f"c{pk}", cmek_enabled=True, cloud_state="m",
             status="r", cmek_key_name=key, cmek_key_ring_name=ring,
             cmek_key_project_id=project, cloud_zone="europe-west4-a",
             cloud_region="", cmek_deletion_policy="ABANDON",
             status_message="", cmek_revoked=False,
             cloud_credential=NS(detail="{}"),
             save=mock.Mock())
    for name, value in fields.items():
        setattr(row, name, value)
    return row


def kms_client():
    """A Cloud KMS client double, and its cryptoKeys() resource."""
    client = mock.MagicMock()
    keys = (client.projects.return_value.locations.return_value
            .keyRings.return_value.cryptoKeys.return_value)
    return client, keys


class ViewTestCase(SimpleTestCase):
    """Calls the views directly, with the managers and messages replaced."""

    def setUp(self):
        self.rows = [cluster(1, "proj-a", "ring-a"),
                     cluster(2, "proj-b", "ring-b")]
        for model, rows in ((Cluster, self.rows),
                            (GCPFilestoreFilesystem, []), (Image, [])):
            patcher = mock.patch.object(model, "objects", Rows(rows))
            patcher.start()
            self.addCleanup(patcher.stop)
        patcher = mock.patch.object(v, "messages")
        self.messages = patcher.start()
        self.addCleanup(patcher.stop)

    def get(self, kind, pk, user=ADMIN):
        request = RequestFactory().get("/")
        request.user = user
        with mock.patch.object(
                cloud_info, "crypto_key_destroy_scheduled_duration",
                return_value="2592000s"):
            return v.CmekKeyDestroyView.as_view()(request, kind=kind, pk=pk)

    def post(self, kind, pk, data, destroyed=("v1",)):
        request = RequestFactory().post("/", data)
        request.user = ADMIN
        with mock.patch.object(cloud_info, "destroy_crypto_key_versions",
                               return_value=list(destroyed)) as destroy:
            response = v.CmekKeyDestroyView.as_view()(
                request, kind=kind, pk=pk)
        return response, destroy


class ExactTargetTests(ViewTestCase):
    def test_the_url_names_the_resource(self):
        self.assertEqual(
            reverse("cmek-key-destroy", kwargs={"kind": "filestore", "pk": 3}),
            "/cmek/keys/filestore/3/destroy")

    def test_each_url_destroys_only_its_own_rows_key(self):
        # both keys are called cluster-abc; only the path tells them apart
        for row, path in ((self.rows[0], PATH_A), (self.rows[1], PATH_B)):
            with self.subTest(pk=row.pk):
                response, destroy = self.post("cluster", row.pk,
                                              {"path": path})
                self.assertEqual(response.status_code, 302)
                destroy.assert_called_once_with("{}", path)
                self.assertIn("cluster-abc", row.status_message)

    def test_the_listing_links_each_row_by_resource(self):
        request = RequestFactory().get("/")
        request.user = ADMIN
        response = v.CmekKeysView.as_view()(request)
        got = sorted((k["kind"], k["resource_pk"], k["path"])
                     for k in response.context_data["keys"])
        self.assertEqual(got, [("cluster", 1, PATH_A), ("cluster", 2, PATH_B)])

    def test_the_listing_names_each_key_ring(self):
        request = RequestFactory().get("/")
        request.user = ADMIN
        response = v.CmekKeysView.as_view()(request)
        rows = {k["resource_pk"]: k for k in response.context_data["keys"]}
        self.assertEqual(rows[2]["key_ring_name"], "ring-b")
        self.assertEqual(
            rows[2]["key_ring_url"],
            "https://console.cloud.google.com/security/kms/keyring/manage/"
            "europe-west4/ring-b?project=proj-b")
        self.assertNotIn("keychain", rows[2])
        page = render_to_string("cmek/keys.html", response.context_data)
        self.assertIn('<th scope="col">Key ring</th>', page)
        self.assertIn(f'<a href="{rows[2]["key_ring_url"]}"', page)
        self.assertIn(">ring-b</a>", page)
        self.assertNotIn("Keychain", page)

    def test_the_confirmation_page_names_the_key_ring(self):
        response = self.get("cluster", 2)
        self.assertEqual(response.context_data["key_ring_name"], "ring-b")
        page = render_to_string("cmek/key_destroy.html",
                                response.context_data)
        self.assertIn('<th scope="row">Key ring</th><td>ring-b</td>', page)
        self.assertNotIn("Keychain", page)

    def test_the_confirmation_page_carries_the_full_path(self):
        response = self.get("cluster", 2)
        self.assertEqual(response.context_data["key_path"], PATH_B)
        page = render_to_string("cmek/key_destroy.html",
                                response.context_data)
        self.assertIn(f'name="path" value="{PATH_B}"', page)


class StalePageTests(ViewTestCase):
    def test_a_path_that_no_longer_matches_is_refused(self):
        # the page was confirmed for one key; the resource now uses another
        response, destroy = self.post("cluster", 2, {"path": PATH_A})
        self.assertEqual(response.status_code, 302)
        destroy.assert_not_called()
        self.messages.error.assert_called_once()
        self.assertEqual(self.rows[1].status_message, "")

    def test_a_post_without_a_path_is_refused(self):
        _response, destroy = self.post("cluster", 2, {})
        destroy.assert_not_called()
        self.messages.error.assert_called_once()


class NotLiveTests(ViewTestCase):
    def assert_not_found(self, kind, pk):
        with self.assertRaises(Http404):
            self.get(kind, pk)
        with self.assertRaises(Http404):
            self.post(kind, pk, {"path": PATH_A})

    def test_a_torn_down_row_is_not_found(self):
        self.rows[0].cloud_state = "xm"
        self.assert_not_found("cluster", 1)

    def test_a_destroyed_cluster_is_not_found(self):
        self.rows[0].status = "d"
        self.assert_not_found("cluster", 1)

    def test_an_unencrypted_row_is_not_found(self):
        self.rows[0].cmek_enabled = False
        self.assert_not_found("cluster", 1)

    def test_a_row_with_no_recorded_key_is_not_found(self):
        self.rows[0].cmek_key_name = ""
        self.assert_not_found("cluster", 1)

    def test_a_pk_of_another_kind_is_not_found(self):
        # pk 1 is a cluster, not an image
        self.assert_not_found("image", 1)

    def test_an_unknown_kind_is_not_found(self):
        self.assertEqual(resolve("/cmek/keys/bucket/1/destroy").url_name,
                         "cmek-key-destroy")
        self.assert_not_found("bucket", 1)


class RevocationCompletenessTests(SimpleTestCase):
    """destroy_crypto_key_versions against a Cloud KMS client double."""

    def run_destroy(self, client):
        with mock.patch.object(cloud_info, "_get_gcp_client",
                               return_value=("p", client)):
            return cloud_info.destroy_crypto_key_versions("{}", PATH_A)

    def test_every_page_of_versions_is_destroyed(self):
        client, keys = kms_client()
        versions = keys.cryptoKeyVersions.return_value
        versions.list.return_value.execute.side_effect = [
            {"cryptoKeyVersions": [{"name": "v1", "state": "ENABLED"}],
             "nextPageToken": "page-2"},
            {"cryptoKeyVersions": [{"name": "v2", "state": "ENABLED"}]},
        ]
        self.assertEqual(self.run_destroy(client), ["v1", "v2"])
        self.assertEqual(versions.list.call_args_list[1],
                         mock.call(parent=PATH_A, pageToken="page-2"))

    def test_the_rotation_schedule_is_cleared_before_any_destroy(self):
        # otherwise the next rotation mints a new ENABLED primary
        client, keys = kms_client()
        versions = keys.cryptoKeyVersions.return_value
        versions.list.return_value.execute.return_value = {
            "cryptoKeyVersions": [{"name": "v1", "state": "ENABLED"}]}
        self.run_destroy(client)
        keys.patch.assert_called_once_with(
            name=PATH_A, updateMask="rotationPeriod,nextRotationTime",
            body={})
        calls = [name for name, _args, _kwargs in keys.mock_calls]
        self.assertLess(calls.index("patch().execute"),
                        calls.index("cryptoKeyVersions().destroy"))

    def test_any_failure_after_the_patch_says_what_was_scheduled(self):
        for listing, destroys, scheduled, failed in (
                ([RuntimeError("x")], [], [], "listing versions"),
                (None, [RuntimeError("x")], [], "v1"),
                (None, [{}, RuntimeError("x")], ["v1"], "v2")):
            with self.subTest(failed=failed, scheduled=scheduled):
                client, keys = kms_client()
                versions = keys.cryptoKeyVersions.return_value
                versions.list.return_value.execute.side_effect = listing or [{
                    "cryptoKeyVersions": [
                        {"name": "v1", "state": "ENABLED"},
                        {"name": "v2", "state": "ENABLED"}]}]
                versions.destroy.return_value.execute.side_effect = destroys
                with self.assertRaises(cloud_info.KeyDestructionError) as ctx:
                    self.run_destroy(client)
                self.assertEqual(ctx.exception.scheduled, scheduled)
                self.assertEqual(ctx.exception.failed, failed)

    def test_a_failed_patch_is_raised_as_it_is(self):
        client, keys = kms_client()
        keys.patch.return_value.execute.side_effect = RuntimeError("denied")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_destroy(client)
        self.assertNotIsInstance(ctx.exception,
                                 cloud_info.KeyDestructionError)
        keys.cryptoKeyVersions.return_value.list.assert_not_called()

    def test_already_scheduled_versions_are_skipped(self):
        client, keys = kms_client()
        versions = keys.cryptoKeyVersions.return_value
        versions.list.return_value.execute.return_value = {
            "cryptoKeyVersions": [
                {"name": "v1", "state": "DESTROY_SCHEDULED"},
                {"name": "v2", "state": "DESTROYED"},
                {"name": "v3", "state": "ENABLED"}]}
        self.assertEqual(self.run_destroy(client), ["v3"])


class PartialFailureTests(ViewTestCase):
    """Once rotation is cleared, every outcome is recorded on the resource.

    The key no longer rotates from that point, whether or not any version was
    scheduled, so the resource is marked revoked and its page says what is
    left to do. Only a failure to clear rotation leaves it untouched.
    """

    def post_with(self, outcomes=(), listing=None, patch=None):
        client, keys = kms_client()
        if patch is not None:
            keys.patch.return_value.execute.side_effect = patch
        versions = keys.cryptoKeyVersions.return_value
        versions.list.return_value.execute.side_effect = listing or [{
            "cryptoKeyVersions": [{"name": "v1", "state": "ENABLED"},
                                  {"name": "v2", "state": "ENABLED"},
                                  {"name": "v3", "state": "ENABLED"}]}]
        versions.destroy.return_value.execute.side_effect = outcomes
        request = RequestFactory().post("/", {"path": PATH_A})
        request.user = ADMIN
        with mock.patch.object(cloud_info, "_get_gcp_client",
                               return_value=("p", client)):
            return v.CmekKeyDestroyView.as_view()(request, kind="cluster",
                                                  pk=1)

    def assert_revoked(self):
        row = self.rows[0]
        self.assertIs(row.cmek_revoked, True)
        row.save.assert_called_once_with(
            update_fields=["status_message", "cmek_revoked"])
        self.assertIn("OFE will no longer start, change or re-apply it",
                      row.status_message)

    def test_the_resource_records_what_was_scheduled_and_what_failed(self):
        response = self.post_with([{}, RuntimeError("denied")])
        self.assertEqual(response.status_code, 302)
        message = self.rows[0].status_message
        self.assertIn("Scheduled for destruction: v1.", message)
        self.assertIn("Failed: v2.", message)
        self.assert_revoked()
        self.messages.error.assert_called_once()
        self.assertIn("denied", self.messages.error.call_args[0][1])

    def test_a_failure_on_the_first_destroy_is_still_recorded(self):
        # rotation is already off, so the key has changed even with nothing
        # scheduled
        self.post_with([RuntimeError("denied")])
        message = self.rows[0].status_message
        self.assertIn("Rotation was cleared but no version was scheduled for "
                      "destruction; failure: v1; revoke again to finish.",
                      message)
        self.assert_revoked()
        self.messages.error.assert_called_once()
        self.assertIn("denied", self.messages.error.call_args[0][1])

    def test_a_failure_on_a_later_destroy_names_both(self):
        self.post_with([{}, {}, RuntimeError("denied")])
        message = self.rows[0].status_message
        self.assertIn("Scheduled for destruction: v1, v2.", message)
        self.assertIn("Failed: v3.", message)
        self.assert_revoked()

    def test_a_failure_listing_the_versions_is_still_recorded(self):
        self.post_with(listing=[RuntimeError("list denied")])
        message = self.rows[0].status_message
        self.assertIn("Rotation was cleared but no version was scheduled for "
                      "destruction; failure: listing versions;", message)
        self.assert_revoked()
        self.assertIn("list denied", self.messages.error.call_args[0][1])

    def test_a_failure_on_a_later_page_of_versions_is_still_recorded(self):
        self.post_with(listing=[
            {"cryptoKeyVersions": [{"name": "v1", "state": "ENABLED"}],
             "nextPageToken": "page-2"},
            RuntimeError("page denied")])
        self.assertIn("failure: listing versions",
                      self.rows[0].status_message)
        self.assert_revoked()

    def test_a_key_with_no_live_versions_is_still_marked_revoked(self):
        self.post_with(listing=[{"cryptoKeyVersions": [
            {"name": "v1", "state": "DESTROY_SCHEDULED"}]}])
        self.assertIn("no live versions", self.rows[0].status_message)
        self.assert_revoked()
        self.messages.warning.assert_called_once()

    def test_success_marks_the_resource_revoked(self):
        self.post_with([{}, {}, {}])
        self.assertIn("scheduled for destruction", self.rows[0].status_message)
        self.assert_revoked()
        self.messages.success.assert_called_once()
        self.assertIn("Destroy this cluster and create a new one: OFE will no "
                      "longer start, change or re-apply it.",
                      self.messages.success.call_args[0][1])

    def test_a_failure_to_clear_rotation_leaves_the_row_alone(self):
        # nothing changed, so the resource still works and has nothing to say
        self.post_with(patch=RuntimeError("patch denied"))
        self.assertEqual(self.rows[0].status_message, "")
        self.assertIs(self.rows[0].cmek_revoked, False)
        self.rows[0].save.assert_not_called()
        self.messages.error.assert_called_once()
        self.assertIn("patch denied", self.messages.error.call_args[0][1])


class DestructionWindowTests(ViewTestCase):
    """Cloud KMS keeps destroyed versions for 30 days by default, not 24 hours.

    kms-key's destroy_scheduled_duration defaults to 2592000s, and OFE does
    not override it.
    """

    def test_the_fallback_wording_says_30_days(self):
        page = render_to_string("cmek/key_destroy.html", {
            "key_name": "cluster-abc", "key_path": PATH_A,
            "resource_route": "cluster-detail", "resource_pk": 1,
            "destroy_window": None})
        self.assertIn("30 days", page)
        self.assertNotIn("24 hours", page)

    def test_the_window_is_read_from_the_key(self):
        response = self.get("cluster", 1)
        self.assertEqual(response.context_data["destroy_window"], "30 days")
        page = render_to_string("cmek/key_destroy.html",
                                response.context_data)
        self.assertIn("30 days", page)
        self.assertNotIn("24 hours", page)

    def test_the_page_still_renders_when_the_key_cannot_be_read(self):
        request = RequestFactory().get("/")
        request.user = ADMIN
        with mock.patch.object(cloud_info, "_get_gcp_client",
                               side_effect=RuntimeError("no access")):
            response = v.CmekKeyDestroyView.as_view()(
                request, kind="cluster", pk=1)
        self.assertEqual(response.status_code, 200)
        self.assertIsNone(response.context_data["destroy_window"])

    def test_no_message_or_docstring_claims_24_hours(self):
        self.assertNotIn("24 hours", v.CmekKeyDestroyView.__doc__)
        self.assertIn("30 days", v.CmekKeyDestroyView.__doc__)


class SettingsPermissionTests(SimpleTestCase):
    """The deployment defaults are admin only, like the key pages."""

    def call(self, method, user):
        request = getattr(RequestFactory(), method)("/", {})
        request.user = user
        with mock.patch.object(v.utils, "load_config", return_value={}):
            return v.CmekSettingsView.as_view()(request)

    def test_staff_cannot_read_or_save_them(self):
        for method in ("get", "post"):
            with self.subTest(method=method), \
                    self.assertRaises(PermissionDenied):
                self.call(method, STAFF)

    def test_a_superuser_can(self):
        self.assertEqual(self.call("get", ADMIN).status_code, 200)

    def test_the_panel_links_them_only_for_a_superuser(self):
        def render(user):
            form = PanelForm(settings=settings_for(enforcement="preferred"))
            return render_to_string("cmek/_panel.html",
                                    {"form": form, "user": user})
        self.assertNotIn("Deployment defaults", render(STAFF))
        self.assertIn("Deployment defaults", render(ADMIN))
