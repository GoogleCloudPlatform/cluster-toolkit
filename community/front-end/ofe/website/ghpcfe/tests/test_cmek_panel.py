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
"""The encryption panel on a resource form.

Exercised through the mixin rather than a full ModelForm, so these stay fast and
do not need a database: what matters is the gating and the cleaning, both of
which are the mixin's.

Enforcement wins over the per-resource toggle. That is the one rule here with a
security consequence, so it is checked from both directions.
"""
import contextlib
import json
import pathlib
import types
from unittest import mock

from django import forms
from django.template.loader import render_to_string
from django.test import SimpleTestCase

from ..cluster_manager import cmek
from ..forms import CMEK_PANEL_FIELDS, CMEK_PANEL_WIDGETS, CmekPanelMixin


class PanelForm(CmekPanelMixin, forms.Form):
    """A plain form carrying the panel's fields, standing in for a resource.

    Uses CMEK_PANEL_WIDGETS, the same widgets the real forms declare, so a
    stand-in never tests something the product does not do.
    """

    cmek_enabled = forms.BooleanField(
        required=False, widget=CMEK_PANEL_WIDGETS["cmek_enabled"])
    # Not a real CMEK field: present because the guard reads location from
    # cleaned data, the same way ClusterForm/FilestoreForm/ImageForm do.
    cloud_zone = forms.CharField(required=False)
    cmek_key_name = forms.CharField(
        required=False, widget=CMEK_PANEL_WIDGETS["cmek_key_name"])
    cmek_key_project_id = forms.CharField(
        required=False, widget=CMEK_PANEL_WIDGETS["cmek_key_project_id"])
    cmek_key_ring_name = forms.CharField(
        required=False, widget=CMEK_PANEL_WIDGETS["cmek_key_ring_name"])
    cmek_deletion_policy = forms.ChoiceField(
        required=False, choices=[(p, p) for p in cmek.DELETION_POLICIES],
        widget=CMEK_PANEL_WIDGETS["cmek_deletion_policy"])
    cmek_protection_level = forms.ChoiceField(
        required=False,
        choices=[("", "default")] + [(p, p) for p in cmek.PROTECTION_LEVELS],
        widget=CMEK_PANEL_WIDGETS["cmek_protection_level"])
    cmek_rotation_period = forms.CharField(
        required=False, widget=CMEK_PANEL_WIDGETS["cmek_rotation_period"])
    cmek_encryption_service_account = forms.CharField(
        required=False,
        widget=CMEK_PANEL_WIDGETS["cmek_encryption_service_account"])

    def __init__(self, *args, settings=None, **kwargs):
        super().__init__(*args, **kwargs)
        self.setup_cmek_panel(settings=settings)

    def clean(self):
        return self.clean_cmek_panel(super().clean())


def settings_for(**kwargs):
    base = {"enforcement": "preferred", "key_project_id": "keys-proj",
            "key_ring_name": "ofe-cmek"}
    base.update(kwargs)
    return cmek.CmekSettings(**base)


def bound(data, **kwargs):
    form = PanelForm(data, **kwargs)
    form.is_valid()
    return form


class GatingTests(SimpleTestCase):
    def test_disabled_deployment_drops_the_fields(self):
        form = PanelForm(settings=settings_for(enforcement="disabled"))
        for name in CMEK_PANEL_FIELDS:
            with self.subTest(field=name):
                self.assertNotIn(name, form.fields)

    def test_preferred_offers_the_toggle_unlocked(self):
        form = PanelForm(settings=settings_for(enforcement="preferred"))
        self.assertIn("cmek_enabled", form.fields)
        self.assertFalse(form.fields["cmek_enabled"].disabled)

    def test_required_locks_the_toggle_on(self):
        form = PanelForm(settings=settings_for(enforcement="required"))
        field = form.fields["cmek_enabled"]
        self.assertTrue(field.disabled)
        self.assertTrue(field.initial)

    def test_required_encrypts_even_when_the_post_says_otherwise(self):
        # a disabled control submits nothing, and a hand-made POST must not win
        form = bound({},
                     settings=settings_for(enforcement="required"))
        self.assertTrue(form.cleaned_data["cmek_enabled"])

    def test_preferred_honours_an_unticked_toggle(self):
        form = bound({},
                     settings=settings_for(enforcement="preferred"))
        self.assertFalse(form.cleaned_data["cmek_enabled"])


class ClearingTests(SimpleTestCase):
    def test_turning_encryption_off_clears_the_rest(self):
        form = bound({
            "cmek_key_name": "leftover", "cmek_key_ring_name": "ring",
            "cmek_encryption_service_account": "me@p.iam.gserviceaccount.com",
        }, settings=settings_for())
        cleaned = form.cleaned_data
        self.assertFalse(cleaned["cmek_enabled"])
        self.assertEqual(cleaned["cmek_key_name"], "")
        self.assertEqual(cleaned["cmek_key_ring_name"], "")
        self.assertEqual(cleaned["cmek_encryption_service_account"], "")


class KeyNamingTests(SimpleTestCase):
    """There is one way a resource gets a key: OFE creates it. The name,
    project and key ring are optional overrides on top of the deployment
    defaults, not a separate mode.
    """

    def test_nothing_needs_filling_in(self):
        form = bound({"cmek_enabled": "on"}, settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_the_teardown_policy_defaults_to_abandon(self):
        # kms-key has no default, so OFE must choose one, and OFE does not
        # destroy key material unless it is asked to
        form = bound({"cmek_enabled": "on"}, settings=settings_for())
        self.assertEqual(form.cleaned_data["cmek_deletion_policy"],
                         cmek.DELETION_POLICY_ABANDON)

    def test_a_blank_key_name_is_left_for_ofe_to_derive(self):
        form = bound({"cmek_enabled": "on"}, settings=settings_for())
        self.assertEqual(form.cleaned_data["cmek_key_name"], "")

    def test_an_invalid_key_name_is_rejected(self):
        form = bound({"cmek_enabled": "on", "cmek_key_name": "no spaces"},
                     settings=settings_for())
        self.assertIn("cmek_key_name", form.errors)

    def test_an_invalid_key_ring_is_rejected(self):
        form = bound({"cmek_enabled": "on", "cmek_key_name": "ok",
                      "cmek_key_ring_name": "bad ring!"},
                     settings=settings_for())
        self.assertIn("cmek_key_ring_name", form.errors)

    def test_a_valid_name_and_key_ring_are_accepted(self):
        form = bound({"cmek_enabled": "on", "cmek_key_name": "mine",
                      "cmek_key_ring_name": "my-ring"},
                     settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)


class DuplicateKeyGuardTests(SimpleTestCase):
    """The duplicate-key guard must fire on Create, not only on Update.

    Live-found bug (2026-08-24): two Filestores were given the same key name
    and both deployed successfully with it -- the guard never fired. Cause:
    it read self._cmek_location, set once at __init__ from self.instance's
    cloud_zone/cloud_region. On every Create form self.instance is a fresh,
    unsaved row with neither set yet -- the operator is choosing the zone in
    the very submission being validated. The guard silently no-opped on every
    create, which is exactly where a duplicate name first gets typed; it only
    ever worked on Update, where a saved instance already carries its zone.

    PanelForm mirrors this exactly: it declares cloud_zone as a plain form
    field (as ClusterForm/FilestoreForm/ImageForm all do) and passes no
    location= at construction, matching every real Create view.
    """

    def test_a_duplicate_name_is_rejected_using_the_submitted_zone(self):
        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=["Filestore 'fsone'"]) as sharing:
            form = bound({
                "cmek_enabled": "on", "cloud_zone": "europe-west4-a",
                "cmek_key_name": "dup-test-key",
            }, settings=settings_for())
        self.assertFalse(form.is_valid())
        self.assertIn("cmek_key_name", form.errors)
        self.assertIn("fsone", str(form.errors["cmek_key_name"]))
        # and it must have actually been asked about the zone from this
        # submission, not an empty location
        sharing.assert_called_once()
        (path,), kwargs = sharing.call_args
        self.assertIn("europe-west4", path)

    def test_no_sharer_means_no_error(self):
        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=[]):
            form = bound({
                "cmek_enabled": "on", "cloud_zone": "europe-west4-a",
                "cmek_key_name": "fresh-key",
            }, settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_a_blank_zone_is_not_silently_treated_as_a_pass(self):
        # if the zone truly cannot be resolved there is nothing to compare
        # against, but this must be because location is blank, not because
        # the guard read the wrong field
        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=["Filestore 'fsone'"]) as sharing:
            form = bound({
                "cmek_enabled": "on", "cmek_key_name": "dup-test-key",
            }, settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)
        sharing.assert_not_called()


    # Live-found bug (2026-10-06, manual test plan A4): with no key project in
    # the deployment settings -- the default -- the guard returned before
    # comparing, so a second Filestore was accepted with another's key name.
    # The deploy resolves that project from the credential
    # (cmek.resolve_created_key); the guard must resolve it the same way.
    CRED_PATH = ("projects/cred-proj/locations/europe-west4/keyRings/ofe-cmek"
                 "/cryptoKeys/dup-test-key")

    @staticmethod
    def _credential():
        return types.SimpleNamespace(
            detail=json.dumps({"project_id": "cred-proj"}))

    def test_a_blank_key_project_uses_the_submitted_credential(self):
        credential = self._credential()

        class CreateForm(PanelForm):
            cloud_credential = forms.CharField(required=False)

            def clean_cloud_credential(self):
                return credential

        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=["Filestore 'fsone'"]) as sharing:
            form = CreateForm({
                "cmek_enabled": "on", "cloud_zone": "europe-west4-a",
                "cmek_key_name": "dup-test-key", "cloud_credential": "1",
            }, settings=settings_for(key_project_id=""))
            valid = form.is_valid()
        self.assertFalse(valid)
        self.assertIn("cmek_key_name", form.errors)
        (path,), _ = sharing.call_args
        self.assertEqual(path, self.CRED_PATH)

    def test_a_blank_key_project_uses_the_instance_credential(self):
        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=["Filestore 'fsone'"]) as sharing:
            form = PanelForm({
                "cmek_enabled": "on", "cloud_zone": "europe-west4-a",
                "cmek_key_name": "dup-test-key",
            }, settings=settings_for(key_project_id=""))
            form.instance = types.SimpleNamespace(
                pk=1, cloud_credential=self._credential())
            valid = form.is_valid()
        self.assertFalse(valid)
        self.assertIn("cmek_key_name", form.errors)
        (path,), _ = sharing.call_args
        self.assertEqual(path, self.CRED_PATH)

class ServiceAccountTests(SimpleTestCase):
    def test_rejects_a_principal_string(self):
        form = bound({"cmek_enabled": "on",                       "cmek_encryption_service_account":
                          "serviceAccount:me@p.iam.gserviceaccount.com"},
                     settings=settings_for())
        self.assertIn("cmek_encryption_service_account", form.errors)
        self.assertIn("serviceAccount:",
                      str(form.errors["cmek_encryption_service_account"]))

    def test_rejects_a_non_email(self):
        form = bound({"cmek_enabled": "on",                       "cmek_encryption_service_account": "someone"},
                     settings=settings_for())
        self.assertIn("cmek_encryption_service_account", form.errors)

    def test_accepts_a_bare_email(self):
        form = bound({"cmek_enabled": "on",                       "cmek_encryption_service_account":
                          "me@p.iam.gserviceaccount.com"},
                     settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_rejects_a_user_principal_with_a_hint(self):
        form = bound({"cmek_enabled": "on",
                      "cmek_encryption_service_account":
                          "user:me@p.iam.gserviceaccount.com"},
                     settings=settings_for())
        self.assertIn("cmek_encryption_service_account", form.errors)
        # the message echoes the value, so check the hint itself
        self.assertIn("drop the type prefix",
                      str(form.errors["cmek_encryption_service_account"]))


class PanelDefinitionTests(SimpleTestCase):
    def test_every_field_has_a_widget(self):
        self.assertEqual(set(CMEK_PANEL_FIELDS), set(CMEK_PANEL_WIDGETS))

    def test_the_three_resource_forms_all_carry_the_panel(self):
        from ..forms import ClusterForm, FilestoreForm, ImageForm
        for form_class in (ClusterForm, FilestoreForm, ImageForm):
            with self.subTest(form=form_class.__name__):
                self.assertTrue(issubclass(form_class, CmekPanelMixin))
                for name in CMEK_PANEL_FIELDS:
                    self.assertIn(name, form_class.Meta.fields)

    def test_the_cleaned_panel_feeds_the_emitter(self):
        form = bound({"cmek_enabled": "on",                       "cmek_key_name": "my-key",
                      "cmek_deletion_policy": "ABANDON"},
                     settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)
        out = cmek.key_modules_yaml(
            service="filestore", project_id="wl", location="europe-west4",
            settings=settings_for(),
            **cmek.resource_settings(
                type("R", (), form.cleaned_data)(), settings_for()))
        self.assertIn('key_name: "my-key"', out)
        self.assertIn('deletion_policy: "ABANDON"', out)


class RenderTests(SimpleTestCase):
    """The panel template, rendered for real.

    Database-free like the rest of the suite: this app commits no migrations
    (they are generated at deploy), so a DB-backed TestCase cannot run here.
    Nothing in the panel queries any more -- the reuse picker, which was the
    one part that did, is gone.
    """

    def render(self, enforcement):
        form = PanelForm(settings=settings_for(enforcement=enforcement))
        return render_to_string("cmek/_panel.html", {"form": form})

    def test_disabled_renders_nothing(self):
        self.assertEqual(self.render("disabled").strip(), "")

    def test_preferred_renders_the_panel(self):
        html = self.render("preferred")
        self.assertIn("Encryption (CMEK)", html)
        self.assertIn("Key configuration", html)

    def test_no_key_reuse_picker_is_offered(self):
        # sharing a key lets one resource's teardown revoke the other's
        # access, so the panel must not invite it
        html = self.render("preferred")
        self.assertNotIn("cmek-known-key", html)
        self.assertNotIn("Keys OFE already knows about", html)

    def test_the_panel_says_the_key_belongs_to_this_resource(self):
        self.assertIn("creates a key for this resource alone",
                      self.render("preferred"))

    def test_every_panel_field_is_rendered(self):
        html = self.render("preferred")
        for name in CMEK_PANEL_FIELDS:
            with self.subTest(field=name):
                self.assertIn(f'name="{name}"', html)

    def test_teardown_is_outside_advanced(self):
        html = self.render("preferred")
        teardown = html.index('id="cmek-teardown"')
        advanced = html.index('id="cmek-advanced"')
        self.assertLess(teardown, advanced,
                        "tear-down must not be inside Advanced")

    def test_the_service_account_field_shows_an_example(self):
        self.assertIn("name@project.iam.gserviceaccount.com",
                      self.render("preferred"))

    def test_required_says_why_the_toggle_is_locked(self):
        self.assertIn("requires a customer-managed encryption key",
                      self.render("required"))

    def test_abandoned_keys_are_explained_where_teardown_is_chosen(self):
        # ABANDON is now the default, so the panel has to say what happens to
        # the keys left behind and where to deal with them
        html = self.render("preferred")
        self.assertIn("Keys left in place cost nothing to keep", html)
        self.assertIn("console.cloud.google.com/security/kms", html)


class KeyPropertyTests(SimpleTestCase):
    """Protection level and rotation period, which OFE now sets."""

    def test_blank_leaves_the_module_default(self):
        form = bound({"cmek_enabled": "on"}, settings=settings_for())
        self.assertEqual(form.cleaned_data["cmek_protection_level"], "")
        self.assertEqual(form.cleaned_data["cmek_rotation_period"], "")

    def test_hsm_is_accepted(self):
        form = bound({"cmek_enabled": "on",                       "cmek_protection_level": "HSM"}, settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_an_unknown_protection_level_is_rejected(self):
        form = bound({"cmek_enabled": "on",                       "cmek_protection_level": "QUANTUM"},
                     settings=settings_for())
        self.assertIn("cmek_protection_level", form.errors)

    def test_a_valid_rotation_period_is_accepted(self):
        form = bound({"cmek_enabled": "on",                       "cmek_rotation_period": "2592000s"},
                     settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_a_rotation_period_without_seconds_is_rejected(self):
        form = bound({"cmek_enabled": "on",                       "cmek_rotation_period": "90d"}, settings=settings_for())
        self.assertIn("cmek_rotation_period", form.errors)

    def test_a_rotation_period_under_a_day_is_rejected(self):
        # kms-key requires at least 86400s
        form = bound({"cmek_enabled": "on",                       "cmek_rotation_period": "3600s"}, settings=settings_for())
        self.assertIn("cmek_rotation_period", form.errors)



class LiveFoundBugTests(SimpleTestCase):
    """Three defects the first live run surfaced, pinned so they cannot return.

    None was caught by the earlier tests: the first two only appear on a real
    ModelForm and in a real render, and the third only when nothing has been
    configured yet.
    """

    def test_the_teardown_policy_offers_no_blank_choice(self):
        # a blank=True model field makes ModelForm prepend an empty
        # "---------" choice, which reads as a fourth, meaningless option
        from ..forms import ClusterForm
        choices = list(ClusterForm().fields["cmek_deletion_policy"].choices)
        self.assertNotIn("", [value for value, _ in choices])

    def test_abandon_is_selected_by_default(self):
        from ..forms import ClusterForm
        self.assertEqual(
            ClusterForm().initial.get("cmek_deletion_policy"),
            cmek.DELETION_POLICY_ABANDON)

    def test_no_template_comment_leaks_into_the_page(self):
        # Django's short comment syntax does not span lines, so a multi-line
        # one is rendered to the page as literal text
        form = PanelForm(settings=settings_for())
        html = render_to_string("cmek/_panel.html", {"form": form})
        self.assertNotIn("{#", html)
        self.assertNotIn("{% comment", html)

    def test_encryption_works_with_nothing_configured(self):
        # a fresh deployment has no cmek: section at all; refusing to create a
        # key until someone visits the settings page makes the feature unusable
        settings = cmek.CmekSettings.from_config({})
        out = cmek.key_modules_yaml(
            service="filestore", project_id="wl",
            location="europe-west4", settings=settings, key_name="fs-1")
        self.assertIn(f'key_ring_name: "{cmek.DEFAULT_KEY_RING}"', out)

    def test_the_slider_css_is_global_not_per_page(self):
        """The toggle rendered as a bare checkbox on the Filestore form.

        Its CSS lived inline in cluster/update_form.html, so only that page had
        it. The panel is included by three forms, so the rules belong in the
        stylesheet every page loads.
        """
        css = (pathlib.Path(__file__).resolve().parents[1]
               / "static/css/styles.css").read_text()
        for rule in (".slider-switch", ".slider:before", ".grid-container"):
            with self.subTest(rule=rule):
                self.assertIn(rule, css)

    def test_teardown_choices_name_the_value_being_sent(self):
        """The options read as prose only, hiding which value was chosen."""
        form = PanelForm(settings=settings_for())
        labels = [str(label) for _, label in
                  form.fields["cmek_deletion_policy"].choices]
        self.assertTrue(any(l.startswith("DELETE") for l in labels), labels)
        self.assertTrue(any(l.startswith("ABANDON") for l in labels), labels)


class FilestoreCmekTierTests(SimpleTestCase):
    """CMEK is only valid on some Filestore tiers, and OFE must say so.

    Live-found (2026-08-24): a BASIC_HDD Filestore with encryption on saved
    happily, generated a blueprint, ran init and plan, and only then failed --
    the filestore module has a Terraform precondition allowing kms_key_name
    solely on ZONAL/HIGH_SCALE_SSD, REGIONAL and ENTERPRISE. The operator got
    an opaque provider error after several minutes of apply for something
    knowable at form time.

    Checked here against the tier codes rather than the module's labels, since
    that mapping is exactly what went wrong.
    """

    from ..forms import (CMEK_CAPABLE_FILESTORE_TIERS,
                         filestore_cmek_tier_error)

    def test_the_basic_tiers_are_rejected(self):
        from ..forms import filestore_cmek_tier_error
        for tier, label in (("bh", "BASIC_HDD"), ("bs", "BASIC_SSD")):
            with self.subTest(tier=tier):
                err = filestore_cmek_tier_error(tier, True)
                self.assertIsNotNone(err)
                self.assertIn(label, err)
                # names a way forward, not just a refusal
                self.assertIn("ENTERPRISE", err)

    def test_the_cmek_capable_tiers_are_accepted(self):
        from ..forms import filestore_cmek_tier_error
        for tier in ("hs", "en"):
            with self.subTest(tier=tier):
                self.assertIsNone(filestore_cmek_tier_error(tier, True))

    def test_an_unencrypted_basic_tier_is_fine(self):
        from ..forms import filestore_cmek_tier_error
        self.assertIsNone(filestore_cmek_tier_error("bh", False))

    def test_the_allowed_set_matches_the_modules_precondition(self):
        # the module allows ZONAL/HIGH_SCALE_SSD, REGIONAL and ENTERPRISE;
        # of the tiers OFE offers, that is exactly HIGH_SCALE_SSD and ENTERPRISE
        from pathlib import Path
        from ..forms import CMEK_CAPABLE_FILESTORE_TIERS
        from ..models import GCPFilestoreFilesystem
        main_tf = (Path(__file__).resolve().parents[6]
                   / "modules/file-system/filestore/main.tf")
        if not main_tf.exists():
            self.skipTest(f"module not present at {main_tf}")
        cond = main_tf.read_text()
        labels = dict(GCPFilestoreFilesystem.FILESTORE_TIER)
        for tier in CMEK_CAPABLE_FILESTORE_TIERS:
            with self.subTest(tier=tier):
                self.assertIn(f'"{labels[tier]}"', cond)
        for tier, label in labels.items():
            if tier not in CMEK_CAPABLE_FILESTORE_TIERS:
                with self.subTest(tier=tier):
                    self.assertNotIn(f'"{label}"', cond)


class FilestoreCapacityRuleTests(SimpleTestCase):
    """Capacity floors must match what the Filestore API actually accepts.

    Live-found (2026-08-24): a HIGH_SCALE_SSD Filestore at 1024 GiB passed
    OFE's form, generated a blueprint, and failed the apply with
    "capacity must be at least 10240 GiB". HIGH_SCALE_SSD is the legacy name
    for the *high-scale* zonal band (10 TiB floor, 2.5 TiB steps), not zonal
    in general -- OFE's rule had the smaller band's numbers.
    """

    def test_high_scale_ssd_floor_is_ten_tib(self):
        from ..forms import filestore_capacity_error
        self.assertIsNotNone(filestore_capacity_error("hs", 1024))
        self.assertIsNotNone(filestore_capacity_error("hs", 10239))
        self.assertIsNone(filestore_capacity_error("hs", 10240))

    def test_high_scale_ssd_steps_in_2560_gib(self):
        from ..forms import filestore_capacity_error
        self.assertIsNotNone(filestore_capacity_error("hs", 10240 + 256))
        self.assertIsNone(filestore_capacity_error("hs", 10240 + 2560))

    def test_the_other_tiers_keep_their_documented_floors(self):
        from ..forms import filestore_capacity_error
        for tier, floor in (("bh", 1024), ("bs", 2560), ("en", 1024)):
            with self.subTest(tier=tier):
                self.assertIsNotNone(filestore_capacity_error(tier, floor - 1))
                self.assertIsNone(filestore_capacity_error(tier, floor))

    def test_enterprise_is_the_cheapest_cmek_capable_entry_point(self):
        # both CMEK-capable tiers must have a usable floor; ENTERPRISE's is
        # an order of magnitude lower, which is what test runs should use
        from ..forms import (CMEK_CAPABLE_FILESTORE_TIERS,
                             FILESTORE_CAPACITY_RULES)
        floors = {t: FILESTORE_CAPACITY_RULES[t]["min"]
                  for t in CMEK_CAPABLE_FILESTORE_TIERS}
        self.assertEqual(min(floors, key=floors.get), "en")


class LockedToggleOnRealFormsTests(SimpleTestCase):
    """The locked toggle must render on in the forms the product really uses.

    The views build these ModelForms with an instance, and a ModelForm starts
    from the instance's values (the model default is off) in preference to
    the field's own initial. The plain PanelForm above has no instance, so it
    cannot show the difference.
    """

    def render_toggle(self, form_class, enforcement, **fields):
        model = form_class._meta.model
        kwargs = {}
        patches = [mock.patch.object(
            cmek.CmekSettings, "from_config",
            return_value=settings_for(enforcement=enforcement))]
        if form_class.__name__ == "FilestoreForm":
            creds = types.SimpleNamespace(detail="{}")
            kwargs["initial"] = {"cloud_credential": creds}
            patches.append(mock.patch(
                "ghpcfe.forms.cloud_info.get_region_zone_info",
                return_value={"europe-west4": ["europe-west4-a"]}))
            # the vpc picker is filtered by credential; nothing here needs a
            # database, so stand in for the lookup
            patches.append(mock.patch("ghpcfe.forms.VirtualNetwork"))
        with contextlib.ExitStack() as stack:
            for patch in patches:
                stack.enter_context(patch)
            form = form_class(instance=model(**fields), **kwargs)
        return str(form["cmek_enabled"])

    def real_forms(self):
        from .. import forms as ghpcfe_forms
        return (ghpcfe_forms.FilestoreForm, ghpcfe_forms.ClusterForm,
                ghpcfe_forms.ImageForm)

    def test_required_renders_the_toggle_on_and_locked(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                html = self.render_toggle(form_class, "required")
                self.assertIn("checked", html)
                self.assertIn("disabled", html)

    def test_preferred_does_not_render_the_toggle_on(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                html = self.render_toggle(form_class, "preferred")
                self.assertNotIn("checked", html)
                self.assertNotIn("disabled", html)

    def test_a_deployed_filestore_shows_its_key_locked_when_disabled(self):
        # Filestore only: a saved Cluster reads its many-to-many fields from
        # the database, which these tests do not have
        from .. import forms as ghpcfe_forms
        html = self.render_toggle(ghpcfe_forms.FilestoreForm, "disabled",
                                  pk=1, cloud_state="m", cmek_enabled=True)
        self.assertIn("checked", html)
        self.assertIn("disabled", html)


class FilestoreLocationIsFixedOnceAppliedTests(SimpleTestCase):
    """A Filestore's key lives in its zone's region, so once a key has been
    applied the zone, network, share and tier stay as they were, even after a
    failed create has put the record back to "not managed"."""

    LOCKED = ("vpc", "cloud_zone", "share_name", "performance_tier")

    def build(self, **fields):
        from .. import forms as ghpcfe_forms
        from ..models import GCPFilestoreFilesystem
        creds = types.SimpleNamespace(detail="{}")
        with mock.patch.object(cmek.CmekSettings, "from_config",
                               return_value=settings_for()), \
                mock.patch("ghpcfe.forms.cloud_info.get_region_zone_info",
                           return_value={"europe-west4": ["europe-west4-a"]}), \
                mock.patch("ghpcfe.forms.VirtualNetwork"):
            return ghpcfe_forms.FilestoreForm(
                instance=GCPFilestoreFilesystem(**fields),
                initial={"cloud_credential": creds})

    def test_an_applied_filestore_back_at_not_managed_is_locked(self):
        form = self.build(pk=1, cloud_state="nm", cmek_applied=True)
        for name in self.LOCKED:
            self.assertTrue(form.fields[name].disabled, name)

    def test_a_filestore_never_applied_can_still_move(self):
        form = self.build(pk=1, cloud_state="nm", cmek_applied=False)
        for name in self.LOCKED:
            self.assertFalse(form.fields[name].disabled, name)

    def test_a_deployed_filestore_is_locked_as_before(self):
        form = self.build(pk=1, cloud_state="m")
        for name in self.LOCKED:
            self.assertTrue(form.fields[name].disabled, name)


class InvalidStoredSettingsTests(SimpleTestCase):
    """A deployment whose stored encryption settings no longer validate.

    Reading them raised from every resource form's constructor, so every
    create and edit page failed with a server error. Now the forms still build,
    a resource not yet deployed is refused with an error naming the page that
    fixes it, and a deployed one keeps its stored choice and stays editable.
    """

    BAD = cmek.CmekConfigError(
        "cmek.key_project_id 'abcde' is not a valid Google Cloud project id")

    def build(self, form_class, data=None, **fields):
        model = form_class._meta.model
        kwargs = {}
        patches = [mock.patch.object(cmek.CmekSettings, "from_config",
                                     side_effect=self.BAD)]
        if form_class.__name__ == "FilestoreForm":
            creds = types.SimpleNamespace(detail="{}")
            kwargs["initial"] = {"cloud_credential": creds}
            patches.append(mock.patch(
                "ghpcfe.forms.cloud_info.get_region_zone_info",
                return_value={"europe-west4": ["europe-west4-a"]}))
            patches.append(mock.patch("ghpcfe.forms.VirtualNetwork"))
        with contextlib.ExitStack() as stack:
            for patch in patches:
                stack.enter_context(patch)
            return form_class(data, instance=model(**fields), **kwargs)

    def real_forms(self):
        from .. import forms as ghpcfe_forms
        return (ghpcfe_forms.FilestoreForm, ghpcfe_forms.ClusterForm,
                ghpcfe_forms.ImageForm)

    def test_the_forms_still_build_without_the_panel(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                form = self.build(form_class)
                for name in CMEK_PANEL_FIELDS:
                    self.assertNotIn(name, form.fields)

    def test_saving_a_new_resource_names_the_settings_page(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                form = self.build(form_class, data={})
                self.assertFalse(form.is_valid())
                errors = " ".join(form.non_field_errors())
                self.assertIn("encryption settings are invalid", errors)
                self.assertIn("'abcde'", errors)
                self.assertIn("Encryption settings page", errors)

    def test_a_deployed_resource_keeps_its_stored_choice(self):
        # Filestore only: a saved Cluster or Image reads its many-to-many
        # fields from the database, which these tests do not have
        from .. import forms as ghpcfe_forms
        for fields in ({"cloud_state": "m"},
                       {"cloud_state": "nm", "cmek_applied": True}):
            with self.subTest(**fields):
                form = self.build(ghpcfe_forms.FilestoreForm, pk=1,
                                  cmek_enabled=True, cmek_key_name="fs-key",
                                  **fields)
                for name in CMEK_PANEL_FIELDS:
                    self.assertTrue(form.fields[name].disabled, name)
                self.assertIn("checked", str(form["cmek_enabled"]))
                cleaned = {"cmek_enabled": True, "cmek_key_name": "fs-key"}
                self.assertEqual(form.clean_cmek_panel(dict(cleaned)), cleaned)

    def test_a_deployed_panel_form_is_still_valid(self):
        with mock.patch.object(cmek.CmekSettings, "from_config",
                               side_effect=self.BAD):
            form = DeployedPanelForm({"cmek_key_name": "other"},
                                     instance=deployed())
        self.assertTrue(form.is_valid(), form.errors)
        self.assertEqual(form.cleaned_data["cmek_key_name"], "fs-key")

    def test_settings_passed_in_are_used_as_before(self):
        with mock.patch.object(cmek.CmekSettings, "from_config",
                               side_effect=self.BAD) as from_config:
            form = bound({"cmek_enabled": "on"}, settings=settings_for())
        from_config.assert_not_called()
        self.assertTrue(form.is_valid(), form.errors)


class KeyProjectTests(SimpleTestCase):
    """The key project is written into the blueprint, so it is checked."""

    def test_a_project_carrying_a_module_is_refused(self):
        form = bound({"cmek_enabled": "on", "cmek_key_project_id": (
            "p\n  - id: evil\n    source: git::https://example.com/x.git")},
                     settings=settings_for())
        self.assertIn("cmek_key_project_id", form.errors)

    def test_a_malformed_project_is_refused(self):
        form = bound({"cmek_enabled": "on",
                      "cmek_key_project_id": "Bad_Project!"},
                     settings=settings_for())
        self.assertIn("cmek_key_project_id", form.errors)

    def test_a_domain_scoped_project_is_accepted(self):
        form = bound({"cmek_enabled": "on",
                      "cmek_key_project_id": "example.com:proj-1"},
                     settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)

    def test_list_syntax_in_the_account_is_refused(self):
        form = bound({"cmek_enabled": "on",
                      "cmek_encryption_service_account":
                          "a],[b@p.iam.gserviceaccount.com"},
                     settings=settings_for())
        self.assertIn("cmek_encryption_service_account", form.errors)


class DeployedPanelForm(PanelForm):
    """PanelForm editing a saved resource, as an Update view does."""

    def __init__(self, *args, instance=None, **kwargs):
        self.instance = instance
        super().__init__(*args, **kwargs)


def deployed(**kwargs):
    base = {"pk": 1, "cloud_state": "m", "cmek_enabled": True,
            "cmek_key_name": "fs-key", "cmek_key_project_id": "keys-proj",
            "cmek_key_ring_name": "ofe-cmek", "cmek_deletion_policy": "DELETE",
            "cmek_protection_level": "", "cmek_rotation_period": "",
            "cmek_encryption_service_account": ""}
    base.update(kwargs)
    return types.SimpleNamespace(**base)


class FrozenPanelTests(SimpleTestCase):
    """Encryption is fixed once a resource is deployed: the panel shows the
    stored choice, and nothing posted can change it.
    """

    def test_an_unticking_post_is_ignored(self):
        form = DeployedPanelForm({"cmek_key_name": "other"},
                                 instance=deployed(),
                                 settings=settings_for())
        self.assertTrue(form.is_valid(), form.errors)
        self.assertTrue(form.cleaned_data["cmek_enabled"])
        self.assertEqual(form.cleaned_data["cmek_key_name"], "fs-key")
        self.assertEqual(form.cleaned_data["cmek_deletion_policy"], "DELETE")

    def test_an_unencrypted_resource_stays_so_under_required(self):
        form = DeployedPanelForm({"cmek_enabled": "on"},
                                 instance=deployed(cmek_enabled=False,
                                                   cmek_key_name=""),
                                 settings=settings_for(enforcement="required"))
        self.assertTrue(form.is_valid(), form.errors)
        self.assertFalse(form.cleaned_data["cmek_enabled"])

    def test_disabled_enforcement_still_shows_the_panel_locked(self):
        form = DeployedPanelForm(instance=deployed(),
                                 settings=settings_for(enforcement="disabled"))
        for name in CMEK_PANEL_FIELDS:
            with self.subTest(field=name):
                self.assertIn(name, form.fields)
                self.assertTrue(form.fields[name].disabled)
        html = render_to_string("cmek/_panel.html", {"form": form})
        self.assertIn("Encryption is fixed once a resource is deployed", html)
        self.assertIn("checked", str(form["cmek_enabled"]))

    def test_an_applied_resource_stays_locked_after_a_failed_start(self):
        # a failed start put the record back to "nm"; the stored flag still
        # says the choice was applied once, so disabled must not drop it
        form = DeployedPanelForm(
            {"cmek_enabled": ""},
            instance=deployed(cloud_state="nm", cmek_applied=True),
            settings=settings_for(enforcement="disabled"))
        for name in CMEK_PANEL_FIELDS:
            with self.subTest(field=name):
                self.assertIn(name, form.fields)
                self.assertTrue(form.fields[name].disabled)
        self.assertTrue(form.is_valid(), form.errors)
        self.assertTrue(form.cleaned_data["cmek_enabled"])
        self.assertEqual(form.cleaned_data["cmek_key_name"], "fs-key")

    def test_an_applied_filestore_keeps_its_panel_when_disabled(self):
        from .. import forms as ghpcfe_forms
        html = LockedToggleOnRealFormsTests().render_toggle(
            ghpcfe_forms.FilestoreForm, "disabled",
            pk=1, cloud_state="nm", cmek_enabled=True, cmek_applied=True)
        self.assertIn("checked", html)
        self.assertIn("disabled", html)

    def test_an_undeployed_resource_still_follows_enforcement(self):
        new = deployed(cloud_state="nm", cmek_enabled=False)
        form = DeployedPanelForm({}, instance=new,
                                 settings=settings_for(enforcement="required"))
        self.assertTrue(form.is_valid(), form.errors)
        self.assertTrue(form.cleaned_data["cmek_enabled"])
        form = DeployedPanelForm(instance=new,
                                 settings=settings_for(enforcement="disabled"))
        self.assertNotIn("cmek_enabled", form.fields)


class DuplicateGuardResolvesOtherResourcesTests(SimpleTestCase):
    def test_the_guard_passes_the_settings_to_the_scan(self):
        # without them the scan compares raw columns, which are blank on a
        # resource not yet deployed, so a second one with the same typed name
        # slipped through
        settings = settings_for()
        with mock.patch("ghpcfe.forms.cmek.resources_sharing_key",
                        return_value=[]) as sharing:
            bound({"cmek_enabled": "on", "cloud_zone": "europe-west4-a",
                   "cmek_key_name": "dup"}, settings=settings)
        self.assertIs(sharing.call_args.kwargs.get("settings"), settings)


class EnforcementLabelTests(SimpleTestCase):
    def test_preferred_says_the_toggle_starts_off(self):
        from ..forms import CmekSettingsForm
        labels = dict(CmekSettingsForm().fields["enforcement"].choices)
        self.assertEqual(
            labels[cmek.ENFORCEMENT_PREFERRED],
            "Preferred — offered on each resource, off unless the user "
            "turns it on")


class KeyRingFieldNamesTests(SimpleTestCase):
    """The key ring and its project are named as Cloud KMS and the toolkit's
    kms-key module name them (key_ring_name, project_id), on the model, on
    each real resource form and in the fields the panel posts."""

    NEW = ("cmek_key_ring_name", "cmek_key_project_id")
    OLD = ("cmek_keychain", "cmek_key_project")

    def build(self, form_class):
        model = form_class._meta.model
        kwargs = {}
        patches = [mock.patch.object(cmek.CmekSettings, "from_config",
                                     return_value=settings_for())]
        if form_class.__name__ == "FilestoreForm":
            creds = types.SimpleNamespace(detail="{}")
            kwargs["initial"] = {"cloud_credential": creds}
            patches.append(mock.patch(
                "ghpcfe.forms.cloud_info.get_region_zone_info",
                return_value={"europe-west4": ["europe-west4-a"]}))
            patches.append(mock.patch("ghpcfe.forms.VirtualNetwork"))
        with contextlib.ExitStack() as stack:
            for patch in patches:
                stack.enter_context(patch)
            return form_class(instance=model(), **kwargs)

    def real_forms(self):
        from .. import forms as ghpcfe_forms
        return (ghpcfe_forms.FilestoreForm, ghpcfe_forms.ClusterForm,
                ghpcfe_forms.ImageForm)

    def test_the_model_fields_use_the_new_names(self):
        from django.core.exceptions import FieldDoesNotExist
        from ..models import (
            Cluster, CmekResourceMixin, GCPFilestoreFilesystem, Image,
        )
        for model in (CmekResourceMixin, Cluster, GCPFilestoreFilesystem,
                      Image):
            with self.subTest(model=model.__name__):
                for name in self.NEW:
                    self.assertEqual(model._meta.get_field(name).name, name)
                for name in self.OLD:
                    with self.assertRaises(FieldDoesNotExist):
                        model._meta.get_field(name)

    def test_the_real_forms_carry_the_new_names(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                form = self.build(form_class)
                for name in self.NEW:
                    self.assertIn(name, form.fields)
                for name in self.OLD:
                    self.assertNotIn(name, form.fields)

    def test_the_panel_posts_the_new_names(self):
        for form_class in self.real_forms():
            with self.subTest(form=form_class.__name__):
                html = render_to_string("cmek/_panel.html",
                                        {"form": self.build(form_class)})
                for name in self.NEW:
                    self.assertIn(f'name="{name}"', html)
                for name in self.OLD:
                    self.assertNotIn(f'name="{name}"', html)

    def test_an_invalid_key_ring_is_called_a_key_ring(self):
        form = bound({"cmek_enabled": "on",
                      "cmek_key_ring_name": "bad ring!"},
                     settings=settings_for())
        self.assertIn("Cloud KMS key ring may use",
                      str(form.errors["cmek_key_ring_name"]))
