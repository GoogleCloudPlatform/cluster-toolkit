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
"""The deployment-wide encryption defaults page.

Three settings, superusers only, off the sidebar. The form validates through the same
code that reads the file, so it cannot save something the engine would later
refuse.
"""
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import yaml
from django.test import RequestFactory, SimpleTestCase
from django.urls import reverse

from ..cluster_manager import cmek, utils
from ..forms import CmekSettingsForm
from ..views import cmek as cmek_views

ADMIN = SimpleNamespace(is_authenticated=True, is_superuser=True,
                        is_staff=True)


class FormTests(SimpleTestCase):
    def test_accepts_the_three_settings(self):
        form = CmekSettingsForm({"enforcement": "required",
                                 "key_project_id": "keys-proj",
                                 "key_ring_name": "ofe-cmek"})
        self.assertTrue(form.is_valid(), form.errors)
        self.assertEqual(form.settings.enforcement, "required")

    def test_only_three_settings_are_offered(self):
        # everything else is a per-resource choice or a kms-key default
        self.assertEqual(
            set(CmekSettingsForm().fields),
            {"enforcement", "key_project_id", "key_ring_name"})

    def test_rejects_an_invalid_key_ring_name(self):
        form = CmekSettingsForm({"enforcement": "preferred",
                                 "key_ring_name": "not a ring!"})
        self.assertFalse(form.is_valid())
        self.assertIn("key_ring_name", form.errors)

    def test_rejects_an_unknown_enforcement_mode(self):
        form = CmekSettingsForm({"enforcement": "sideways"})
        self.assertFalse(form.is_valid())

    def test_an_empty_key_project_is_omitted_from_the_file(self):
        # blank means "each resource's own project", so writing "" would be a
        # value that means nothing
        form = CmekSettingsForm({"enforcement": "preferred"})
        self.assertTrue(form.is_valid(), form.errors)
        self.assertNotIn("key_project_id", form.settings.to_config_section())

    def test_the_key_ring_always_lands_in_the_file(self):
        # it falls back to a default so encryption works on a fresh deployment;
        # writing the effective name keeps the file self-describing rather than
        # leaving the answer only in the code
        form = CmekSettingsForm({"enforcement": "preferred"})
        self.assertTrue(form.is_valid(), form.errors)
        self.assertEqual(form.settings.to_config_section()["key_ring_name"],
                         cmek.DEFAULT_KEY_RING)

    def test_the_section_round_trips_through_the_reader(self):
        form = CmekSettingsForm({"enforcement": "required",
                                 "key_project_id": "keys-proj",
                                 "key_ring_name": "r"})
        self.assertTrue(form.is_valid(), form.errors)
        again = cmek.CmekSettings.from_config(
            {"cmek": form.settings.to_config_section()})
        self.assertEqual(again, form.settings)


class PersistenceTests(SimpleTestCase):
    """The write path, against a temporary file rather than the real config."""

    def test_it_writes_only_the_cmek_section(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {
                "server": {"gcp_project": "keep-me"},
                "cmek": {"enforcement": "disabled"},
            }}))
            settings = cmek.CmekSettings(enforcement="required",
                                         key_ring_name="ofe-cmek")
            utils.update_config_file(
                lambda section: section.__setitem__(
                    "cmek", settings.to_config_section()),
                config_file=path)
            got = yaml.safe_load(path.read_text())["config"]
            self.assertEqual(got["server"]["gcp_project"], "keep-me")
            self.assertEqual(got["cmek"]["enforcement"], "required")
            self.assertEqual(got["cmek"]["key_ring_name"], "ofe-cmek")

    def test_a_stale_setting_does_not_survive_the_write(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {"cmek": {
                "enforcement": "required",
                "provisioning": "autokey",     # from the old implementation
                "key_ring_name": "old-ring",
            }}}))
            settings = cmek.CmekSettings(enforcement="preferred")
            utils.update_config_file(
                lambda section: section.__setitem__(
                    "cmek", settings.to_config_section()),
                config_file=path)
            got = yaml.safe_load(path.read_text())["config"]["cmek"]
            self.assertNotIn("provisioning", got)
            self.assertEqual(got["enforcement"], "preferred")
            self.assertEqual(got["key_ring_name"], cmek.DEFAULT_KEY_RING)


class InvalidStoredSettingsTests(SimpleTestCase):
    """A stored value that fails to parse is shown, so it can be corrected."""

    STORED = {"cmek": {"enforcement": "required",
                       "key_project_id": "Bad_Project!",
                       "key_ring_name": "stored-ring"}}

    def call(self, method, data=None, config=None):
        request = getattr(RequestFactory(), method)("/", data or {})
        request.user = ADMIN
        with mock.patch.object(cmek_views.utils, "load_config",
                               return_value=config or self.STORED), \
                mock.patch.object(cmek_views.utils,
                                  "update_config_file") as update, \
                mock.patch.object(cmek_views, "messages") as messages:
            response = cmek_views.CmekSettingsView.as_view()(request)
        return response, update, messages

    def test_the_stored_values_are_offered_for_correction(self):
        response, _update, messages = self.call("get")
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.context_data["form"].initial, {
            "enforcement": "required", "key_project_id": "Bad_Project!",
            "key_ring_name": "stored-ring"})
        self.assertIn('value="Bad_Project!"',
                      response.render().content.decode())
        messages.warning.assert_called_once()
        self.assertIn("Bad_Project!", messages.warning.call_args[0][1])

    def test_an_unknown_enforcement_falls_back_to_the_default(self):
        response, _update, _messages = self.call("get", config={"cmek": {
            "enforcement": "sometimes", "key_project_id": "keys-proj"}})
        initial = response.context_data["form"].initial
        self.assertEqual(initial["enforcement"], cmek.ENFORCEMENT_PREFERRED)
        self.assertEqual(initial["key_project_id"], "keys-proj")
        self.assertEqual(initial["key_ring_name"], cmek.DEFAULT_KEY_RING)

    def test_a_section_that_is_not_a_mapping_still_renders(self):
        response, _update, messages = self.call(
            "get", config={"cmek": "required"})
        self.assertEqual(response.status_code, 200)
        defaults = cmek.CmekSettings()
        self.assertEqual(response.context_data["form"].initial, {
            "enforcement": defaults.enforcement,
            "key_project_id": defaults.key_project_id,
            "key_ring_name": defaults.key_ring_name})
        response.render()
        messages.warning.assert_called_once()
        self.assertIn("the cmek section of configuration.yaml",
                      messages.warning.call_args[0][1])

    def test_a_valid_replacement_is_written(self):
        response, update, _messages = self.call("post", {
            "enforcement": "required", "key_project_id": "good-project",
            "key_ring_name": "stored-ring"})
        self.assertEqual(response.status_code, 302)
        update.assert_called_once()
        section = {"cmek": self.STORED["cmek"]}
        update.call_args[0][0](section)
        self.assertEqual(section["cmek"], {
            "enforcement": "required", "key_project_id": "good-project",
            "key_ring_name": "stored-ring"})


class RoutingTests(SimpleTestCase):
    def test_the_page_has_a_url(self):
        self.assertEqual(reverse("cmek-settings"), "/cmek/")

    def test_it_is_not_in_the_sidebar(self):
        # encryption is part of creating a resource, not a section of its own
        base = Path(__file__).resolve().parents[1] / "templates/base_generic.html"
        self.assertNotIn("cmek-settings", base.read_text())

    def test_the_view_is_admin_only(self):
        from ..permissions import SuperUserRequiredMixin
        from ..views.cmek import CmekSettingsView
        self.assertTrue(issubclass(CmekSettingsView, SuperUserRequiredMixin))


class KeysOverviewTests(SimpleTestCase):
    """The admin key overview.

    Admin only, because revoking a key is offered from it. OFE reads what the
    deploys recorded and links out to the Cloud Console; it creates and edits
    nothing. Revocation is the one action it does offer, since that is the
    reason a customer wants CMEK and it cannot be reached from a resource's own
    page.
    """

    def _view_class(self, name):
        import ast
        src = (Path(__file__).resolve().parents[1] / "views/cmek.py").read_text()
        return next(n for n in ast.walk(ast.parse(src))
                    if isinstance(n, ast.ClassDef) and n.name == name)

    def test_the_pages_have_urls(self):
        self.assertEqual(reverse("cmek-keys"), "/cmek/keys/")
        self.assertEqual(
            reverse("cmek-key-destroy", kwargs={"kind": "cluster", "pk": 7}),
            "/cmek/keys/cluster/7/destroy")

    def test_both_views_are_admin_only(self):
        from ..permissions import SuperUserRequiredMixin
        from ..views.cmek import CmekKeyDestroyView, CmekKeysView
        for view in (CmekKeysView, CmekKeyDestroyView):
            with self.subTest(view=view.__name__):
                self.assertTrue(issubclass(view, SuperUserRequiredMixin))

    def test_it_is_not_in_the_sidebar(self):
        # encryption is part of creating a resource, not a section of its own
        base = Path(__file__).resolve().parents[1] / "templates/base_generic.html"
        self.assertNotIn("cmek-keys", base.read_text())

    def test_the_overview_creates_and_edits_nothing(self):
        import ast
        cls = self._view_class("CmekKeysView")
        bases = [ast.unparse(b) for b in cls.bases]
        self.assertIn("TemplateView", bases)
        for forbidden in ("CreateView", "UpdateView", "FormView"):
            self.assertNotIn(forbidden, bases)
        body = ast.unparse(cls)
        for forbidden in (".save(", ".create("):
            self.assertNotIn(forbidden, body)

    def test_the_overview_only_lists_live_resources(self):
        import ast
        src = (Path(__file__).resolve().parents[1] / "views/cmek.py").read_text()
        func = next(n for n in ast.walk(ast.parse(src))
                    if isinstance(n, ast.FunctionDef)
                    and n.name == "_live_rows")
        body = ast.unparse(func)
        # a torn-down resource's key is not this deployment's concern, and
        # offering to revoke it would destroy a key nothing uses
        self.assertIn("cloud_state", body)
        self.assertIn("'xm'", body.replace('"xm"', "'xm'"))


class KeyRevocationTests(SimpleTestCase):
    """Revoking a key schedules its versions for destruction.

    A CryptoKey cannot be deleted -- Cloud KMS keeps the name for ever, which is
    why a name can never be reused -- so this schedules every live
    version for destruction instead. The resource stops working, which is the
    intended outcome and matches what kms-key already does on a DELETE teardown.
    """

    def _function(self, path, name):
        import ast
        src = (Path(__file__).resolve().parents[1] / path).read_text()
        node = next(n for n in ast.walk(ast.parse(src))
                    if isinstance(n, (ast.FunctionDef, ast.ClassDef))
                    and n.name == name)
        return ast.unparse(node)

    def test_only_post_destroys_anything(self):
        # irreversible once the scheduled window passes, so a GET must never do
        # more than show what will break
        cls = self._function("views/cmek.py", "CmekKeyDestroyView")
        get_ctx = cls[cls.index("def get_context_data"):cls.index("def post")]
        self.assertNotIn("destroy_crypto_key_versions", get_ctx)
        post = cls[cls.index("def post"):]
        self.assertIn("destroy_crypto_key_versions", post)

    def test_it_schedules_versions_rather_than_deleting_the_key(self):
        body = self._function(
            "cluster_manager/cloud_info.py", "destroy_crypto_key_versions")
        self.assertIn("cryptoKeyVersions", body)
        self.assertIn("destroy", body)

    def test_versions_already_gone_are_skipped(self):
        # so revoking twice is not an error
        body = self._function(
            "cluster_manager/cloud_info.py", "destroy_crypto_key_versions")
        self.assertIn("DESTROYED", body)
        self.assertIn("DESTROY_SCHEDULED", body)

    def test_the_resource_is_told_why_it_stopped_working(self):
        post = self._function("views/cmek.py", "CmekKeyDestroyView")
        self.assertIn("status_message", post)

    def test_the_resource_pages_show_that_message(self):
        templates = Path(__file__).resolve().parents[1] / "templates"
        for name in ("cluster/detail.html", "image/image-view.html",
                     "filesystem/filestore_detail.html"):
            with self.subTest(template=name):
                self.assertIn("object.status_message",
                              (templates / name).read_text())

    def test_the_confirmation_names_what_breaks(self):
        page = (Path(__file__).resolve().parents[1]
                / "templates/cmek/key_destroy.html").read_text()
        self.assertIn("no longer be usable", page)
        self.assertIn("permanently unreadable", page)
        self.assertIn("csrf_token", page)


class ConsoleLinkTests(SimpleTestCase):
    """Cloud Console deep links, built from the recorded key path.

    OFE never destroys a key it abandoned, so the operator needs somewhere to go
    to do it themselves; these are the links that take them there.
    """

    PATH = ("projects/proj-a/locations/europe-west4/keyRings/ofe-cmek-v1"
            "/cryptoKeys/filestore-fsone")

    def test_the_key_link_names_the_key(self):
        parts = cmek.parse_key_path(self.PATH)
        url = cmek.console_key_url(
            parts["project"], parts["location"], parts["ring"], parts["key"])
        self.assertEqual(
            url,
            "https://console.cloud.google.com/security/kms/key/manage/"
            "europe-west4/ofe-cmek-v1/filestore-fsone?project=proj-a")

    def test_the_key_ring_link_names_the_ring(self):
        parts = cmek.parse_key_path(self.PATH)
        url = cmek.console_keyring_url(
            parts["project"], parts["location"], parts["ring"])
        self.assertEqual(
            url,
            "https://console.cloud.google.com/security/kms/keyring/manage/"
            "europe-west4/ofe-cmek-v1?project=proj-a")

    def test_the_links_are_built_from_a_recorded_path(self):
        # the overview has only the three write-back fields to work from, so a
        # path it cannot parse must not become a broken link
        row = SimpleNamespace(
            cmek_key_name="filestore-fsone", cmek_key_ring_name="ofe-cmek-v1",
            cmek_key_project_id="proj-a", cloud_zone="europe-west4-a",
            cloud_region="")
        self.assertEqual(cmek.resource_key_path(row), self.PATH)
