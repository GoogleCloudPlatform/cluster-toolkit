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
"""The Encryption dropdown on a resource detail page.

Everything about a resource's encryption is in that one section, and nothing
about it leaks onto the rest of the page. The other rule worth pinning is that a
setting OFE left alone must read as the module's default rather than as a value
OFE chose.
"""
from types import SimpleNamespace

from django.template.loader import render_to_string
from django.test import SimpleTestCase

from ..cluster_manager import cmek


def settings_for(**kwargs):
    base = {"enforcement": "preferred", "key_project_id": "keys-proj",
            "key_ring_name": "ofe-cmek"}
    base.update(kwargs)
    return cmek.CmekSettings(**base)


def resource(**kwargs):
    base = {"cmek_enabled": True,
            "cmek_key_name": "cluster-abc", "cmek_key_project_id": "",
            "cmek_key_ring_name": "",
            "cmek_deletion_policy": cmek.DELETION_POLICY_DELETE,
            "cmek_protection_level": "", "cmek_rotation_period": "",
            "cmek_encryption_service_account": "",
            "cloud_zone": "europe-west4-a", "project_id": "workload-proj"}
    base.update(kwargs)
    return SimpleNamespace(**base)


def render(res, settings=None):
    detail = cmek.describe(res, settings or settings_for())
    return render_to_string("cmek/_detail.html", {"cmek_detail": detail})




class DescribeTests(SimpleTestCase):
    def test_unencrypted_resource(self):
        got = cmek.describe(resource(cmek_enabled=False), settings_for())
        self.assertFalse(got["encrypted"])

    def test_disabled_deployment(self):
        got = cmek.describe(resource(), settings_for(enforcement="disabled"))
        self.assertFalse(got["encrypted"])

    def test_ofe_handled_key_path_is_derived(self):
        got = cmek.describe(resource(), settings_for())
        self.assertEqual(
            got["key_path"],
            "projects/keys-proj/locations/europe-west4/keyRings/ofe-cmek"
            "/cryptoKeys/cluster-abc")

    def test_delete_teardown_says_what_it_costs(self):
        got = cmek.describe(resource(cmek_deletion_policy="DELETE"),
                            settings_for())
        self.assertIn("unreadable", got["teardown"])

    def test_abandon_teardown_says_the_key_survives(self):
        got = cmek.describe(resource(cmek_deletion_policy="ABANDON"),
                            settings_for())
        self.assertIn("left in place", got["teardown"])

    def test_access_is_not_reported_because_it_never_varies(self):
        # OFE creates every key and grants on every key, so a row saying so
        # would restate an invariant rather than report a choice
        self.assertNotIn("access", cmek.describe(resource(), settings_for()))

    def test_unset_properties_are_labelled_as_module_defaults(self):
        # OFE can set them now, but blank must not read as though it had
        got = cmek.describe(resource(), settings_for())
        self.assertIn("module default", got["protection_level"])
        self.assertIn("module default", got["rotation_period"])

    def test_chosen_properties_are_reported_plainly(self):
        got = cmek.describe(
            resource(cmek_protection_level="HSM",
                     cmek_rotation_period="2592000s"), settings_for())
        self.assertEqual(got["protection_level"], "HSM")
        self.assertEqual(got["rotation_period"], "2592000s")

    def test_the_key_ring_and_its_project_use_cloud_kms_names(self):
        got = cmek.describe(resource(cmek_key_ring_name="ofe-cmek-v1",
                                     cmek_key_project_id="keys-proj"),
                            settings_for())
        self.assertEqual(got["key_ring_name"], "ofe-cmek-v1")
        self.assertEqual(got["key_project_id"], "keys-proj")
        self.assertNotIn("keychain", got)
        self.assertNotIn("key_project", got)

    def test_abandon_teardown_says_the_key_stays(self):
        got = cmek.describe(
            resource(cmek_deletion_policy=cmek.DELETION_POLICY_ABANDON),
            settings_for())
        self.assertIn("left in place", got["teardown"])


class RevokedResourceTests(SimpleTestCase):
    """A revoked key ends the resource's use, not the record of its key."""

    def test_the_key_is_still_described(self):
        revoked = resource(pk=1, name="fsone", cloud_state="nm",
                           cmek_revoked=True)
        got = cmek.describe(revoked, settings_for(enforcement="disabled"))
        self.assertTrue(got["encrypted"])
        self.assertEqual(got["key_name"], "cluster-abc")
        self.assertIn("cluster-abc", render(
            revoked, settings_for(enforcement="disabled")))


class RenderTests(SimpleTestCase):
    def test_nothing_renders_without_the_context(self):
        self.assertEqual(render_to_string("cmek/_detail.html", {}).strip(), "")

    def test_encrypted_resource_shows_its_key(self):
        html = render(resource())
        self.assertIn("Encryption", html)
        self.assertIn("cluster-abc", html)
        self.assertIn("Full key path", html)

    def test_the_section_is_collapsed_by_default(self):
        html = render(resource())
        body = html.index('id="cmek-detail-body"')
        self.assertIn('style="display:none;"', html[body:body + 120])

    def test_the_header_says_the_status_without_opening(self):
        self.assertIn("customer-managed key", render(resource()))
        self.assertIn("Google-managed", render(resource(cmek_enabled=False)))

    def test_unencrypted_resource_says_not_applicable(self):
        html = render(resource(cmek_enabled=False))
        self.assertIn("Google-managed encryption", html)
        self.assertIn("Not applicable", html)

    def test_no_encryption_detail_escapes_the_section(self):
        html = render(resource(
            cmek_encryption_service_account="sa@p.iam.gserviceaccount.com"))
        body_start = html.index('id="cmek-detail-body"')
        body_end = html.index("<script>")
        outside = html[:body_start] + html[body_end:]
        for term in ("Key ring", "Tear-down", "Full key path", "Key access",
                     "sa@p.iam.gserviceaccount.com"):
            with self.subTest(term=term):
                self.assertNotIn(term, outside)

    def test_the_service_account_override_is_shown_when_set(self):
        self.assertIn("sa@p.iam.gserviceaccount.com",
                      render(resource(cmek_encryption_service_account=
                                      "sa@p.iam.gserviceaccount.com")))

    def test_it_explains_what_module_default_means(self):
        self.assertIn("left it", render(resource()))

    def test_a_chosen_protection_level_is_rendered(self):
        html = render(resource(cmek_protection_level="HSM",
                               cmek_rotation_period="2592000s"))
        self.assertIn("HSM", html)
        self.assertIn("every 2592000s", html)


class DeployedKeyIsRecordedTests(SimpleTestCase):
    """The detail page showed no Key or Full key path.

    OFE derives the name when it writes the blueprint, but nothing stored it, so
    describe() had nothing to report: the first live Filestore's page said
    "customer-managed key" with no name and no path. resolve_created_key() is
    now the single place that decides name, ring and project, used by both the
    emitter and the record written back to the resource.
    """

    def test_resolution_prefers_the_resource_then_the_deployment_default(self):
        settings = settings_for(key_ring_name="default-ring",
                                key_project_id="default-proj")
        own = cmek.resolve_created_key(
            {"key_name": "k", "key_ring_name": "mine",
             "key_project_id": "my-proj"}, settings, "resource-proj")
        self.assertEqual(own["key_ring_name"], "mine")
        self.assertEqual(own["key_project_id"], "my-proj")

        fallback = cmek.resolve_created_key(
            {"key_name": "k"}, settings, "resource-proj")
        self.assertEqual(fallback["key_ring_name"], "default-ring")
        self.assertEqual(fallback["key_project_id"], "default-proj")

    def test_the_resource_project_is_the_last_resort(self):
        settings = settings_for(key_project_id="")
        got = cmek.resolve_created_key({"key_name": "k"}, settings, "res-proj")
        self.assertEqual(got["key_project_id"], "res-proj")

    def test_a_recorded_key_gives_the_page_a_name_and_a_path(self):
        # what the generators now write back
        res = resource(cmek_key_name="filestore-fsone",
                       cmek_key_ring_name="ofe-cmek-v1",
                       cmek_key_project_id="keys-proj")
        got = cmek.describe(res, settings_for())
        self.assertEqual(got["key_name"], "filestore-fsone")
        self.assertEqual(
            got["key_path"],
            "projects/keys-proj/locations/europe-west4"
            "/keyRings/ofe-cmek-v1/cryptoKeys/filestore-fsone")

    def test_the_page_names_the_key_ring_and_its_project(self):
        html = render(resource(cmek_key_name="filestore-fsone",
                               cmek_key_ring_name="ofe-cmek-v1",
                               cmek_key_project_id="keys-proj"))
        self.assertIn(
            '<tr><th scope="row">Key ring</th><td>ofe-cmek-v1</td></tr>', html)
        self.assertIn(
            '<tr><th scope="row">Key project</th><td>keys-proj</td></tr>',
            html)
        self.assertNotIn("Keychain", html)

    def test_the_page_renders_both_rows(self):
        html = render(resource(cmek_key_name="filestore-fsone",
                              cmek_key_ring_name="ofe-cmek-v1",
                              cmek_key_project_id="p"))
        self.assertIn("Full key path", html)
        self.assertIn("filestore-fsone", html)

    def test_the_generators_all_record_it(self):
        import pathlib as _p
        base = _p.Path(__file__).resolve().parents[1] / "cluster_manager"
        for name in ("filesystem.py", "clusterinfo.py", "image.py"):
            with self.subTest(module=name):
                src = (base / name).read_text()
                self.assertIn("resolve_created_key", src)
                self.assertIn("cmek_key_name", src)
