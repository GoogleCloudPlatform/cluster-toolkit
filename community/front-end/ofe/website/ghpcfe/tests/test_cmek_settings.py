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
import fcntl
import os
import re
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


def _directory_is_locked(directory):
    """Whether another descriptor holds the exclusive lock on `directory`.

    A flock lock belongs to the open file description, so a descriptor
    opened here is refused while update_config_file holds its own, even in
    the same process. The probe asks for a shared lock, which only an
    exclusive lock refuses, so a shared lock taken by mistake reads as
    not locked.
    """
    probe = os.open(directory, os.O_RDONLY)
    try:
        fcntl.flock(probe, fcntl.LOCK_SH | fcntl.LOCK_NB)
    except BlockingIOError:
        return True
    finally:
        os.close(probe)    # also releases the probe's lock if it got one
    return False


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

    def setUp(self):
        # update_config_file drops the process-wide config cache, which holds
        # the real configuration; leave it alone, so no later test reads a
        # configuration other than the one it set up.
        patcher = mock.patch.object(utils, "invalidate_config_cache")
        self.invalidate = patcher.start()
        self.addCleanup(patcher.stop)

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
            # the running server reads the new settings, not its cached copy
            self.invalidate.assert_called_once_with()

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

    def test_the_read_the_change_and_the_rename_hold_the_lock(self):
        # Two admins saving at once, from any server process: the second
        # writer waits on the config directory's lock, so each of the read,
        # the change and the rename is checked to run while it is held. A
        # writer can then neither read the file before the previous writer's
        # rename nor rename after the next writer has read it.
        real_load, real_replace = yaml.safe_load, os.replace
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {}}))
            held = {}

            def load(stream):
                held["read"] = _directory_is_locked(tmp)
                return real_load(stream)

            def replace(src, dst):
                held["rename"] = _directory_is_locked(tmp)
                return real_replace(src, dst)

            def mutate(section):
                held["change"] = _directory_is_locked(tmp)
                section["b"] = 2

            with mock.patch("yaml.safe_load", side_effect=load), \
                    mock.patch("os.replace", side_effect=replace):
                utils.update_config_file(mutate, config_file=path)
            self.assertEqual(held,
                             {"read": True, "change": True, "rename": True})
            self.assertFalse(_directory_is_locked(tmp))
            self.assertEqual(yaml.safe_load(path.read_text()),
                             {"config": {"b": 2}})

    def test_concurrent_writers_lose_neither_change(self):
        # Writers started together all succeed, each change is kept, and no
        # writer leaves its own temporary file behind.
        import threading
        import time
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {}}))
            errors = []
            # bounded, so a writer that never releases the lock fails the
            # test instead of hanging it
            start = threading.Barrier(4, timeout=10)

            def write(name):
                def mutate(section):
                    section[name] = True
                try:
                    start.wait()
                    utils.update_config_file(mutate, config_file=path)
                except Exception as err:  # pylint: disable=broad-except
                    errors.append(err)

            threads = [threading.Thread(target=write, args=(f"k{i}",),
                                        daemon=True)
                       for i in range(4)]
            for thread in threads:
                thread.start()
            deadline = time.monotonic() + 15  # one bound for all writers
            for thread in threads:
                thread.join(timeout=max(0, deadline - time.monotonic()))
            self.assertEqual([t.name for t in threads if t.is_alive()], [],
                             "a writer did not finish")
            self.assertEqual(errors, [])
            got = yaml.safe_load(path.read_text())["config"]
            self.assertEqual(sorted(got), ["k0", "k1", "k2", "k3"])
            self.assertEqual([p.name for p in Path(tmp).iterdir()],
                             ["configuration.yaml"])

    def test_a_failed_write_leaves_the_file_and_no_temporary_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            original = yaml.safe_dump({"config": {"server": {"a": 1}}})
            path.write_text(original)
            with mock.patch.object(utils.yaml, "safe_dump",
                                   side_effect=OSError("disk full")), \
                    self.assertRaises(OSError):
                utils.update_config_file(
                    lambda section: section.__setitem__("b", 2),
                    config_file=path)
            self.assertEqual(path.read_text(), original)
            self.assertEqual([p.name for p in Path(tmp).iterdir()],
                             ["configuration.yaml"])
            self.invalidate.assert_not_called()  # nothing new to read

    def test_the_temporary_file_is_made_beside_the_config_file(self):
        # os.replace is atomic only within one file system, so the new file
        # is written in the config file's own directory, never in TMPDIR.
        real_replace = os.replace
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {}}))
            renamed = []

            def replace(src, dst):
                renamed.append((Path(os.fspath(src)), Path(os.fspath(dst))))
                return real_replace(src, dst)

            with mock.patch("os.replace", side_effect=replace):
                utils.update_config_file(
                    lambda section: section.__setitem__("b", 2),
                    config_file=path)
            self.assertEqual(len(renamed), 1)
            src, dst = renamed[0]
            self.assertEqual(dst, path)
            self.assertEqual(src.parent, path.parent)
            self.assertNotEqual(src.name, path.name)
            self.assertEqual(yaml.safe_load(path.read_text()),
                             {"config": {"b": 2}})

    def test_the_file_keeps_its_permissions(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "configuration.yaml"
            path.write_text(yaml.safe_dump({"config": {}}))
            path.chmod(0o640)
            utils.update_config_file(
                lambda section: section.__setitem__("b", 2),
                config_file=path)
            self.assertEqual(path.stat().st_mode & 0o777, 0o640)


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
            "key_ring_name": "stored-ring",
            "loaded_digest": cmek_views._section_digest(self.STORED)})
        self.assertEqual(response.status_code, 302)
        update.assert_called_once()
        section = {"cmek": self.STORED["cmek"]}
        update.call_args[0][0](section)
        self.assertEqual(section["cmek"], {
            "enforcement": "required", "key_project_id": "good-project",
            "key_ring_name": "stored-ring"})


class StaleSaveTests(SimpleTestCase):
    """A save from a page loaded before the settings last changed is refused.

    A save replaces the whole section, so without this check an administrator
    whose page still shows the old values would silently undo another's save.
    These run the view against a real file under the real lock; the server's
    cached copy of the configuration is stood in for by load_config.
    """

    INITIAL = {"server": {"gcp_project": "keep-me"},
               "cmek": {"enforcement": "preferred",
                        "key_ring_name": "ring-a"}}

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.path = Path(tmp.name) / "configuration.yaml"
        self.write(self.INITIAL)
        # the cache, re-read on every call unless a test says otherwise
        self.load_config = self.patch(
            cmek_views.utils, "load_config", side_effect=self.read)
        self.patch(cmek_views.utils, "config_file_path",
                   return_value=self.path)
        # never refresh the process-wide configuration from the test's file
        self.invalidate = self.patch(cmek_views.utils,
                                     "invalidate_config_cache")
        self.patch(cmek_views, "messages")

    def patch(self, target, name, **kwargs):
        patcher = mock.patch.object(target, name, **kwargs)
        self.addCleanup(patcher.stop)
        return patcher.start()

    def write(self, config):
        self.path.write_text(yaml.safe_dump({"config": config}))

    def read(self, *_args, **_kwargs):
        return yaml.safe_load(self.path.read_text())["config"]

    @staticmethod
    def call(method, data=None):
        request = getattr(RequestFactory(), method)("/cmek/", data or {})
        request.user = ADMIN
        return cmek_views.CmekSettingsView.as_view()(request)

    def load_page(self):
        """GET the page and return the digest its form carries."""
        response = self.call("get")
        self.assertEqual(response.status_code, 200)
        found = re.search(r'name="loaded_digest" value="([^"]*)"',
                          response.render().content.decode())
        self.assertIsNotNone(found, "the form carries no loaded_digest")
        return found[1]

    def save(self, digest, enforcement="preferred", key_ring_name="ring-a"):
        data = {"enforcement": enforcement, "key_project_id": "",
                "key_ring_name": key_ring_name}
        if digest is not None:
            data["loaded_digest"] = digest
        return self.call("post", data)

    def assertRefused(self, response, before):
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.context_data["form"].non_field_errors(),
                         [cmek_views.STALE_SETTINGS_MESSAGE])
        self.assertIn(cmek_views.STALE_SETTINGS_MESSAGE,
                      response.render().content.decode())
        self.assertEqual(self.path.read_bytes(), before)

    def test_a_stale_page_cannot_undo_another_admins_save(self):
        page_a = self.load_page()
        page_b = self.load_page()
        self.assertEqual(self.save(page_a, enforcement="required")
                         .status_code, 302)
        after_a = self.path.read_bytes()
        self.invalidate.reset_mock()
        # B's page still shows "preferred"; B changes only the key ring
        response = self.save(page_b, enforcement="preferred",
                             key_ring_name="ring-b")
        self.assertRefused(response, after_a)
        self.assertEqual(self.read()["cmek"]["enforcement"], "required")
        self.assertEqual(self.read()["cmek"]["key_ring_name"], "ring-a")
        # so the reload the message asks for shows the current settings
        self.invalidate.assert_called_once_with()

    def test_the_refused_page_cannot_be_resubmitted_over_the_change(self):
        page_b = self.load_page()
        self.save(self.load_page(), enforcement="required")
        after_a = self.path.read_bytes()
        refused = self.save(page_b, key_ring_name="ring-b")
        found = re.search(r'name="loaded_digest" value="([^"]*)"',
                          refused.render().content.decode())
        self.assertEqual(found[1], page_b)
        self.assertRefused(self.save(found[1], key_ring_name="ring-b"),
                           after_a)

    def test_a_page_reloaded_after_the_refusal_saves(self):
        page_b = self.load_page()
        self.assertEqual(self.save(self.load_page(), enforcement="required")
                         .status_code, 302)
        after_a = self.path.read_bytes()
        self.assertRefused(self.save(page_b, key_ring_name="ring-b"), after_a)
        response = self.save(self.load_page(), enforcement="required",
                             key_ring_name="ring-b")
        self.assertEqual(response.status_code, 302)
        self.assertEqual(self.read()["cmek"]["key_ring_name"], "ring-b")

    def test_saves_from_fresh_pages_succeed_one_after_another(self):
        self.assertEqual(self.save(self.load_page(), enforcement="required")
                         .status_code, 302)
        self.assertEqual(self.save(self.load_page(), enforcement="disabled",
                                   key_ring_name="ring-c").status_code, 302)
        got = self.read()
        self.assertEqual(got["cmek"], {"enforcement": "disabled",
                                       "key_ring_name": "ring-c"})
        self.assertEqual(got["server"], {"gcp_project": "keep-me"})

    def test_the_first_save_with_no_section_yet_succeeds(self):
        self.write({"server": {"gcp_project": "keep-me"}})
        response = self.save(self.load_page(), enforcement="required")
        self.assertEqual(response.status_code, 302)
        self.assertEqual(self.read()["cmek"]["enforcement"], "required")

    def test_adding_an_empty_section_makes_a_page_stale(self):
        # A missing section has a digest of its own, so a page loaded before
        # an empty `cmek:` section was added cannot overwrite it.
        self.assertNotEqual(cmek_views._section_digest({}),
                            cmek_views._section_digest({"cmek": {}}))
        self.write({"server": {"gcp_project": "keep-me"}})
        page = self.load_page()
        self.write({"server": {"gcp_project": "keep-me"}, "cmek": {}})
        before = self.path.read_bytes()
        self.assertRefused(self.save(page, enforcement="required"), before)

    def test_a_missing_or_garbled_loaded_value_is_refused(self):
        before = self.path.read_bytes()
        for digest in (None, "", "not-a-digest", "é" * 64):
            with self.subTest(digest=digest):
                self.assertRefused(self.save(digest, enforcement="required"),
                                   before)

    def test_the_check_is_against_the_file_not_the_cached_copy(self):
        # The cache still holds what the page showed, but the file has
        # changed underneath it; only the read made under the lock sees that.
        self.load_config.side_effect = None
        self.load_config.return_value = self.read()
        page = self.load_page()
        changed = dict(self.INITIAL,
                       cmek={"enforcement": "required",
                             "key_ring_name": "ring-a"})
        self.write(changed)
        before = self.path.read_bytes()
        self.assertRefused(self.save(page, key_ring_name="ring-b"), before)


class MixedKeyTypeSectionTests(SimpleTestCase):
    """A hand-edited `cmek:` mapping whose keys are of mixed types.

    json.dumps cannot sort such keys, so the digest used to raise and the
    page returned a 500, leaving the settings unreachable.
    """

    MIXED = {"server": {"gcp_project": "keep-me"},
             "cmek": {1: "a", "b": "c", "enforcement": "sometimes"}}
    INITIAL = StaleSaveTests.INITIAL

    # the file-backed page helpers of StaleSaveTests, without its tests
    setUp = StaleSaveTests.setUp
    patch = StaleSaveTests.patch
    read = StaleSaveTests.read
    call = StaleSaveTests.__dict__["call"]
    load_page = StaleSaveTests.load_page
    save = StaleSaveTests.save
    assertRefused = StaleSaveTests.assertRefused

    def write(self, config):
        # safe_dump sorts keys by default, which mixed types cannot do
        self.path.write_text(yaml.safe_dump({"config": config},
                                            sort_keys=False))

    def test_the_digest_is_computed_and_stable(self):
        digest = cmek_views._section_digest(self.MIXED)
        self.assertRegex(digest, r"^[0-9a-f]{64}$")
        self.assertEqual(digest, cmek_views._section_digest(
            {"cmek": dict(reversed(list(self.MIXED["cmek"].items())))}))
        # a fixed value, so it cannot depend on the process (hash seed)
        self.assertEqual(
            cmek_views._section_digest({"cmek": {1: "a", "b": "c"}}),
            "bc20373d7bbb36dfbe08d7cf0869547df46a8beb6f5d6b2367ed42630e74cac7")

    def test_different_sections_give_different_digests(self):
        digests = {cmek_views._section_digest({"cmek": section}) for section
                   in ({1: "a", "b": "c"}, {"1": "a", "b": "c"},
                       {1: "a", "b": "d"}, {1: "a"}, None, "required")}
        self.assertEqual(len(digests), 6)

    def test_a_section_containing_itself_has_a_digest(self):
        section = {"enforcement": "required"}
        section["self"] = section
        self.assertRegex(cmek_views._section_digest({"cmek": section}),
                         r"^[0-9a-f]{64}$")

    def test_the_page_renders_with_the_section_shown_as_invalid(self):
        self.write(self.MIXED)
        messages = self.patch(cmek_views, "messages")
        response = self.call("get")
        self.assertEqual(response.status_code, 200)
        self.assertIn('name="loaded_digest"',
                      response.render().content.decode())
        messages.warning.assert_called_once()
        self.assertIn("sometimes", messages.warning.call_args[0][1])

    def test_a_save_from_that_page_replaces_the_section(self):
        self.write(self.MIXED)
        response = self.save(self.load_page(), enforcement="required")
        self.assertEqual(response.status_code, 302)
        self.assertEqual(self.read()["cmek"], {"enforcement": "required",
                                               "key_ring_name": "ring-a"})
        self.assertEqual(self.read()["server"], {"gcp_project": "keep-me"})

    def test_a_save_from_a_different_page_is_refused_as_stale(self):
        self.write(self.MIXED)
        before = self.path.read_bytes()
        self.assertRefused(
            self.save(cmek_views._section_digest(self.INITIAL),
                      enforcement="required"), before)


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
