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
"""The blueprint blocks OFE emits for a resource's key.

These assert the contract with the three Cluster Toolkit modules, so they fail
if OFE starts sending something the modules would reject:

  * deletion_policy is always present -- kms-key has no default for it
  * the agents come from the resource kind, not from a guess
  * the agents are always granted -- OFE creates the key, so OFE administers it
  * every resource creates its own key, so no two share one grant
  * consumers reach the key through kms-key-iam, never the key module
"""
import re
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import yaml
from django.test import SimpleTestCase

from ..cluster_manager import cmek


def settings_for(**kwargs):
    base = {"enforcement": "preferred", "key_project_id": "keys-proj",
            "key_ring_name": "ofe-cmek"}
    base.update(kwargs)
    return cmek.CmekSettings(**base)


def sources(out):
    """The module sources in order.

    Compared as whole values, not substrings: the kms-key source is a prefix of
    the kms-key-iam source, so `assertIn` on it matches the wrong module.
    """
    return re.findall(r"^\s*source:\s*(\S+)\s*$", out, re.M)


def emit(**kwargs):
    args = {"service": "cluster",
            "project_id": "workload-proj", "location": "europe-west4",
            "settings": settings_for(), "key_name": "cluster-abc"}
    args.update(kwargs)
    return cmek.key_modules_yaml(**args)


class SettingsTests(SimpleTestCase):
    def test_defaults_when_no_section(self):
        s = cmek.CmekSettings.from_config({})
        self.assertEqual(s.enforcement, cmek.ENFORCEMENT_PREFERRED)
        self.assertTrue(s.offered)
        self.assertFalse(s.required)

    def test_disabled_is_not_offered(self):
        s = cmek.CmekSettings.from_config({"cmek": {"enforcement": "disabled"}})
        self.assertFalse(s.offered)

    def test_bad_enforcement_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.CmekSettings.from_config({"cmek": {"enforcement": "sideways"}})

    def test_bad_key_ring_name_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.CmekSettings.from_config(
                {"cmek": {"key_ring_name": "not a valid ring!"}})


class SettingsThatAreNotTextTests(SimpleTestCase):
    """YAML reads an unquoted number or yes/no as something other than text.

    Each is refused as a settings error, which every page already handles,
    rather than crashing whatever read the settings.
    """

    CASES = {
        "key_project_id": (123456789012, "a number"),
        "key_ring_name": (12345, "a number"),
        "enforcement": (True, "true/false"),
    }

    def test_each_setting_must_be_text(self):
        for name, (value, kind) in self.CASES.items():
            for bad, said in ((value, kind), (["a"], "a list"),
                              ({"a": 1}, "a mapping")):
                with self.subTest(name=name, value=bad):
                    with self.assertRaises(cmek.CmekConfigError) as caught:
                        cmek.CmekSettings.from_config({"cmek": {name: bad}})
                    self.assertEqual(
                        str(caught.exception),
                        f"cmek.{name} must be text, not {said}: quote it in "
                        "configuration.yaml")

    def test_the_section_must_be_a_mapping(self):
        for section in ("required", ["enforcement"], 5):
            with self.subTest(section=section):
                with self.assertRaises(cmek.CmekConfigError) as caught:
                    cmek.CmekSettings.from_config({"cmek": section})
                self.assertIn("the cmek section of configuration.yaml",
                              str(caught.exception))

    def test_quoted_values_are_read_as_before(self):
        got = cmek.CmekSettings.from_config({"cmek": {
            "enforcement": "required", "key_project_id": "keys-proj",
            "key_ring_name": "12345"}})
        self.assertEqual(got, cmek.CmekSettings(
            enforcement="required", key_project_id="keys-proj",
            key_ring_name="12345"))

    def test_the_settings_page_still_renders(self):
        from django.test import RequestFactory
        from ..views import cmek as cmek_views
        request = RequestFactory().get("/")
        request.user = SimpleNamespace(is_authenticated=True,
                                       is_superuser=True, is_staff=True)
        config = {"cmek": {"enforcement": "required",
                           "key_project_id": 123456789012}}
        with mock.patch.object(cmek_views.utils, "load_config",
                               return_value=config), \
                mock.patch.object(cmek_views, "messages") as messages:
            response = cmek_views.CmekSettingsView.as_view()(request)
            self.assertEqual(response.status_code, 200)
            response.render()
        self.assertIn("must be text, not a number",
                      messages.warning.call_args[0][1])
        self.assertEqual(response.context_data["form"].initial[
            "key_project_id"], "123456789012")

    def test_a_resource_form_still_builds_and_says_why(self):
        from .. import forms as ghpcfe_forms
        from ..models import Image
        config = {"cmek": {"key_project_id": 123456789012}}
        with mock.patch.object(ghpcfe_forms.utils, "load_config",
                               return_value=config):
            form = ghpcfe_forms.ImageForm({}, instance=Image())
            self.assertFalse(form.is_valid())
        self.assertNotIn("cmek_enabled", form.fields)
        errors = " ".join(form.non_field_errors())
        self.assertIn("must be text, not a number", errors)


class CreatedKeyTests(SimpleTestCase):
    def test_emits_kms_key_then_kms_key_iam(self):
        self.assertEqual(sources(emit()),
                         [cmek.KEY_MODULE_SOURCE, cmek.KEY_IAM_MODULE_SOURCE])

    def test_deletion_policy_is_always_present(self):
        # kms-key has no default for it, so an omitted value fails the apply
        self.assertRegex(emit(), r'deletion_policy: "(DELETE|ABANDON)"')
        self.assertIn('deletion_policy: "DELETE"',
                      emit(deletion_policy="DELETE"))

    def test_deletion_policy_is_validated(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(deletion_policy="KEEP")

    def test_consumers_reach_the_key_through_the_iam_module(self):
        out = emit()
        self.assertIn(f"use: [{cmek.KEY_MODULE_ID}]", out)
        # the key module itself uses nothing
        self.assertEqual(out.count("use: ["), 1)

    def test_zone_is_reduced_to_its_region(self):
        self.assertIn('location: "europe-west4"',
                      emit(location="europe-west4-a"))

    def test_multi_region_is_left_alone(self):
        self.assertIn('location: "europe"', emit(location="europe"))

    def test_key_lives_in_the_key_project_but_agents_in_the_workload_project(self):
        out = emit()
        key_block, iam_block = out.split("- id: " + cmek.KEY_IAM_MODULE_ID)
        self.assertIn('project_id: "keys-proj"', key_block)
        self.assertIn('project_id: "workload-proj"', iam_block)

    def test_a_typed_in_name_project_and_ring_override_the_defaults(self):
        out = emit(key_name="mine", key_project_id="my-proj",
                   key_ring_name="my-ring")
        self.assertIn('key_name: "mine"', out)
        self.assertIn('project_id: "my-proj"', out)
        self.assertIn('key_ring_name: "my-ring"', out)

    def test_a_blank_override_falls_back_to_the_deployment_default(self):
        out = emit(key_name="mine")
        self.assertIn('key_ring_name: "ofe-cmek"', out)

    def test_the_tear_down_policy_defaults_to_abandon(self):
        # OFE does not destroy key material unless asked: a key it deleted
        # takes any data still encrypted with it (the same hazard).
        self.assertIn('deletion_policy: "ABANDON"', emit())

    def test_a_key_name_is_required(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(key_name="")

    def test_an_invalid_key_name_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(key_name="no spaces allowed")

    def test_a_key_ring_is_required(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(settings=settings_for(key_ring_name=""))


class ServiceAgentTests(SimpleTestCase):
    def test_filestore_gets_the_filestore_agent(self):
        self.assertIn('service_agents: ["filestore"]',
                      emit(service="filestore"))

    def test_cluster_needs_compute_and_storage_agents(self):
        # compute for the controller/login/nodeset disks; storage for the
        # controller's GCS bucket, which schedmd-slurm-gcp-v6-controller
        # creates (and encrypts with this same key) by default. Reproduced
        # live: an OFE cluster failed at apply with kmsPermissionDenied on
        # the controller disk and a Cloud KMS 403 on the controller bucket
        # when this list was empty.
        self.assertIn('service_agents: ["compute", "storage"]',
                      emit(service="cluster"))

    def test_every_kind_the_forms_offer_is_known(self):
        for service in ("cluster", "filestore", "image"):
            with self.subTest(service=service):
                self.assertIsInstance(cmek.service_agents_for(service), list)

    def test_an_unknown_kind_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(service="quantum-widget")

    def test_extra_agents_are_added_after_the_kind_s_own(self):
        out = emit(service="cluster",
                   extra_service_agents=["cloudsql", "artifactregistry"])
        self.assertIn(
            'service_agents: ["compute", "storage", "cloudsql", '
            '"artifactregistry"]',
            out)

    def test_an_extra_agent_already_granted_is_not_repeated(self):
        out = emit(service="cluster", extra_service_agents=["storage"])
        self.assertIn('service_agents: ["compute", "storage"]', out)

    def test_an_unknown_extra_agent_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(service="cluster", extra_service_agents=["quantum-widget"])


class ClusterKeyReachesItsAttachmentsTests(SimpleTestCase):
    """A cluster's key is handed to Cloud SQL and Artifact Registry too.

    Live-found 2026-10-08: with CMEK on and Cloud SQL / containers enabled, the
    apply failed with "Insufficient permission to use KMS key" (Cloud SQL) and
    "Permission denied on Cloud KMS key" (Artifact Registry), because the key
    was granted only to the compute and storage agents.
    """

    def _modules(self, **flags):
        from ..cluster_manager import clusterinfo
        info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
        info.config = {"cmek": {"enforcement": "preferred",
                                "key_ring_name": "ofe-cmek"}}
        fields = dict(
            cmek_enabled=True, cmek_key_name="", cmek_key_ring_name="",
            cmek_key_project_id="", cmek_deletion_policy="ABANDON",
            cmek_protection_level="", cmek_rotation_period="",
            cmek_encryption_service_account="", cloud_id="abc", id=1,
            project_id="workload-proj", cloud_zone="europe-west4-a",
            cloud_region="europe-west4",
            cloud_credential=SimpleNamespace(detail="{}"),
            save=lambda **kwargs: None, use_cloudsql=False,
            use_containers=False)
        fields.update(flags)
        registries = fields.pop("registries", [])
        fields["container_registry_relations"] = SimpleNamespace(
            exclude=lambda **kwargs: [
                r for r in registries
                if getattr(r, "status", None) != kwargs.get("status")])
        info.cluster = SimpleNamespace(**fields)
        with mock.patch.object(cmek, "ring_already_exists",
                               return_value=False):
            return info._cmek_modules_yaml()

    def test_a_plain_cluster_grants_compute_and_storage_only(self):
        self.assertIn('service_agents: ["compute", "storage"]',
                      self._modules())

    def test_cloudsql_adds_its_agent(self):
        self.assertIn('service_agents: ["compute", "storage", "cloudsql"]',
                      self._modules(use_cloudsql=True))

    def test_containers_add_the_artifact_registry_agent(self):
        self.assertIn(
            'service_agents: ["compute", "storage", "artifactregistry"]',
            self._modules(use_containers=True))

    def test_a_registry_with_credentials_adds_the_secret_manager_agent(self):
        self.assertIn(
            'service_agents: ["compute", "storage", "artifactregistry", '
            '"secretmanager"]',
            self._modules(use_containers=True, registries=[
                SimpleNamespace(use_upstream_credentials=True)]))

    def test_a_registry_without_credentials_does_not(self):
        self.assertIn(
            'service_agents: ["compute", "storage", "artifactregistry"]',
            self._modules(use_containers=True, registries=[
                SimpleNamespace(use_upstream_credentials=False)]))

    def test_a_deleted_registry_with_credentials_does_not(self):
        self.assertIn(
            'service_agents: ["compute", "storage", "artifactregistry"]',
            self._modules(use_containers=True, registries=[
                SimpleNamespace(use_upstream_credentials=True, status="d")]))

    def test_registries_are_ignored_when_containers_are_off(self):
        self.assertNotIn(
            "secretmanager",
            self._modules(use_containers=False, registries=[
                SimpleNamespace(use_upstream_credentials=True)]))

    def test_both_attachments_add_both_agents(self):
        self.assertIn(
            'service_agents: ["compute", "storage", "cloudsql", '
            '"artifactregistry"]',
            self._modules(use_cloudsql=True, use_containers=True))


class OfeAlwaysGrantsTests(SimpleTestCase):
    """OFE creates the key, so OFE administers access to it.

    The old "someone else manages access to this key" option existed for a key
    a customer brought themselves, and went with the adopted-key mode. Nothing
    now emits skip_iam_role_grants, so a resource can never be deployed with a
    key nothing has been granted on.
    """

    def test_the_agents_are_always_granted(self):
        out = emit(service="filestore")
        self.assertIn('service_agents: ["filestore"]', out)

    def test_skip_iam_role_grants_is_never_emitted(self):
        self.assertNotIn("skip_iam_role_grants", emit())

    def test_no_generator_can_ask_for_it(self):
        # the option is gone from the emitter's signature, so a caller that
        # still passed it would fail loudly rather than silently grant nothing
        import inspect
        params = inspect.signature(cmek.key_modules_yaml).parameters
        self.assertNotIn("key_iam_external", params)


class ServiceAccountTests(SimpleTestCase):
    def test_an_encryption_account_becomes_a_custom_service_account(self):
        out = emit(encryption_service_account="me@p.iam.gserviceaccount.com")
        self.assertIn(
            'custom_service_accounts: ["me@p.iam.gserviceaccount.com"]', out)

    def test_a_principal_string_is_rejected(self):
        # kms-key-iam adds the prefix itself and rejects one that is present
        with self.assertRaises(cmek.CmekConfigError):
            emit(encryption_service_account=
                 "serviceAccount:me@p.iam.gserviceaccount.com")

    def test_a_non_email_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(encryption_service_account="just-a-name")

    def test_an_empty_account_is_omitted(self):
        self.assertNotIn("custom_service_accounts",
                         emit(encryption_service_account=""))

    def test_other_principal_prefixes_are_rejected(self):
        # the module would build "serviceAccount:user:..." and Terraform fails
        # partway through an apply, after the permanent key name is spent
        for prefix in ("user:", "group:", "domain:", "deleted:"):
            with self.subTest(prefix=prefix):
                with self.assertRaises(cmek.CmekConfigError):
                    emit(encryption_service_account=
                         prefix + "me@p.iam.gserviceaccount.com")

    def test_a_space_in_the_account_is_rejected(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(encryption_service_account="a b@p.iam.gserviceaccount.com")

    def test_agent_and_default_compute_accounts_are_accepted(self):
        for account in ("123-compute@developer.gserviceaccount.com",
                        "service-123@gcp-sa-foo.iam.gserviceaccount.com"):
            with self.subTest(account=account):
                self.assertIn(account,
                              emit(encryption_service_account=account))

    def test_none_is_treated_as_blank(self):
        self.assertEqual(cmek.assert_service_account_email(None), "")


class DeriveKeyNameTests(SimpleTestCase):
    def test_derives_from_the_cloud_id(self):
        self.assertEqual(cmek.derive_key_name("cluster", "abc123"),
                         "cluster-abc123")

    def test_sanitises_and_truncates(self):
        name = cmek.derive_key_name("cluster", "Has Spaces/And.Dots" * 10)
        self.assertTrue(cmek.KMS_NAME_RE.match(name), name)
        self.assertLessEqual(len(name), 63)

    def test_rejects_an_identity_with_nothing_usable(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.derive_key_name("", "///")


class ResourceSettingsTests(SimpleTestCase):
    def resource(self, **kwargs):
        base = {"cmek_enabled": True,
                "cmek_key_name": "", "cmek_key_project_id": "",
                "cmek_key_ring_name": "", "cmek_deletion_policy": "",
                "cmek_encryption_service_account": "",
                }
        base.update(kwargs)
        return SimpleNamespace(**base)

    def test_disabled_deployment_never_encrypts(self):
        self.assertIsNone(cmek.resource_settings(
            self.resource(), settings_for(enforcement="disabled")))

    def test_an_unticked_resource_is_not_encrypted(self):
        self.assertIsNone(cmek.resource_settings(
            self.resource(cmek_enabled=False), settings_for()))

    def test_required_enforcement_overrides_the_resource(self):
        got = cmek.resource_settings(
            self.resource(cmek_enabled=False),
            settings_for(enforcement="required"))
        self.assertIsNotNone(got)

    def test_passes_the_resource_choices_through(self):
        got = cmek.resource_settings(
            self.resource(cmek_key_name="mine", cmek_key_ring_name="my-ring"),
            settings_for())
        self.assertEqual(got["key_name"], "mine")
        self.assertEqual(got["key_ring_name"], "my-ring")

    def test_the_result_feeds_the_emitter(self):
        got = cmek.resource_settings(self.resource(cmek_key_name="k1"),
                                     settings_for())
        out = cmek.key_modules_yaml(
            service="filestore", project_id="wl", location="europe-west4",
            settings=settings_for(), **got)
        self.assertIn('key_name: "k1"', out)
        self.assertIn('service_agents: ["filestore"]', out)


class YamlShapeTests(SimpleTestCase):
    def test_indents_for_a_deployment_group(self):
        out = emit(indent=2)
        self.assertTrue(out.startswith("  - id: "), repr(out[:20]))
        for line in out.splitlines():
            if line.strip():
                self.assertTrue(line.startswith("  "), line)

    def test_no_blank_settings_are_emitted(self):
        for line in emit().splitlines():
            if ":" in line and line.strip().startswith(("project_id", "key_")):
                self.assertNotRegex(line, r":\s*$", line)


class NoOrphanedReferencesTests(SimpleTestCase):
    """Nothing calls a function the CMEK removal deleted.

    The first live Filestore failed with
        NameError: name '_apply_created_key_first' is not defined
    because the removal took the function but left one call site. Python does
    not resolve names until the line runs, and no unit test reaches
    start_filesystem, so nothing caught it until a real apply.

    pyflakes does catch it, in well under a second. It is not in
    requirements.txt, so this skips when it is absent rather than pretending to
    have checked.
    """

    def test_no_undefined_names_in_the_generators(self):
        import subprocess
        import sys
        from pathlib import Path

        try:
            import pyflakes  # noqa: F401
        except ImportError:
            self.skipTest("pyflakes is not installed")

        package = Path(__file__).resolve().parents[1]
        result = subprocess.run(
            [sys.executable, "-m", "pyflakes",
             str(package / "cluster_manager"), str(package / "views")],
            capture_output=True, text=True, check=False)
        undefined = [line for line in result.stdout.splitlines()
                     if "undefined name '" in line]
        self.assertEqual(undefined, [], "\n".join(undefined))


class FailureIsVisibleTests(SimpleTestCase):
    """A failed filesystem start must not look like one still in progress.

    The first live Filestore raised before terraform apply and the record kept
    cloud_state "cm" for good: the page said "being created" indefinitely and
    the reason was only in django.log. Clusters already handled the general
    case; filesystems caught only CalledProcessError.
    """

    def test_start_filesystem_handles_more_than_terraform_failures(self):
        import ast
        from pathlib import Path

        src = (Path(__file__).resolve().parents[1]
               / "cluster_manager/filesystem.py").read_text()
        tree = ast.parse(src)
        func = next(n for n in tree.body
                    if isinstance(n, ast.FunctionDef)
                    and n.name == "start_filesystem")
        handled = {ast.unparse(h.type) for t in ast.walk(func)
                   if isinstance(t, ast.Try) for h in t.handlers if h.type}
        self.assertIn("Exception", handled,
                      "a non-Terraform failure would leave the record stuck "
                      "in the creating state")

    def test_the_handler_records_a_reason_and_reraises(self):
        import ast
        from pathlib import Path

        src = (Path(__file__).resolve().parents[1]
               / "cluster_manager/filesystem.py").read_text()
        tree = ast.parse(src)
        func = next(n for n in tree.body
                    if isinstance(n, ast.FunctionDef)
                    and n.name == "start_filesystem")
        for t in ast.walk(func):
            if not isinstance(t, ast.Try):
                continue
            for h in t.handlers:
                if h.type is None or ast.unparse(h.type) != "Exception":
                    continue
                body = ast.unparse(h)
                self.assertIn("status_message", body)
                self.assertIn("cloud_state", body)
                self.assertIn("raise", body)


class KeyRingAdoptionTests(SimpleTestCase):
    """Cloud KMS key rings can never be deleted, so a ring shared by several
    resources exists already for every resource after the first.

    `kms-key` takes exactly one of key_ring_name (create) and key_ring_id
    (adopt), so which one OFE emits decides whether the apply succeeds.
    """

    def test_a_new_ring_is_created_by_name(self):
        out = emit(ring_exists=False)
        self.assertIn('key_ring_name: "ofe-cmek"', out)
        self.assertNotIn("key_ring_id:", out)

    def test_an_existing_ring_is_adopted_by_id(self):
        out = emit(ring_exists=True)
        self.assertIn(
            'key_ring_id: "projects/keys-proj/locations/europe-west4'
            '/keyRings/ofe-cmek"', out)
        # exactly one of the two, or the module rejects the call at plan time
        self.assertNotIn("key_ring_name:", out)

    def test_the_adopted_ring_agrees_with_project_and_location(self):
        # kms-key validates that key_ring_id's project and location match
        # project_id and location, so a disagreement fails the plan
        out = emit(ring_exists=True,
                   key_project_id="keys-proj", key_ring_name="shared",
                   location="europe-west4-a")
        ring = re.search(r'key_ring_id: "([^"]+)"', out).group(1)
        parts = ring.split("/")
        self.assertIn(f'project_id: "{parts[1]}"', out)
        self.assertIn(f'location: "{parts[3]}"', out)

    def test_the_check_is_made_against_cloud_kms(self):
        # OFE's own records cannot answer it: the ring outlives every
        # deployment that used it and may have been created outside OFE
        import ast
        from pathlib import Path

        src = (Path(__file__).resolve().parents[1]
               / "cluster_manager/cmek.py").read_text()
        func = next(n for n in ast.parse(src).body
                    if isinstance(n, ast.FunctionDef)
                    and n.name == "ring_already_exists")
        self.assertIn("cloud_info.kms_key_ring_exists", ast.unparse(func))

    def test_every_generator_decides_which_to_emit(self):
        # a generator that forgets this reverts to the earlier failure where
        # the generated key ring already existed for every resource after the
        # first, so it would pass a single-run test
        from pathlib import Path

        root = Path(__file__).resolve().parents[1] / "cluster_manager"
        for name in ("filesystem.py", "clusterinfo.py", "image.py"):
            src = (root / name).read_text()
            self.assertIn("ring_already_exists", src, name)
            self.assertIn("ring_exists=ring_exists", src, name)


class KeyRingCheckFailuresAreExplainedTests(SimpleTestCase):
    """Whatever stops the ring check, the caller gets a message to show."""

    DEPLOYED = {"key_project_id": "keys-proj", "key_ring_name": "ofe-cmek"}

    def check(self, **kwargs):
        from ..cluster_manager import cloud_info
        with mock.patch.object(cloud_info, "kms_key_ring_exists",
                               **kwargs) as exists:
            result = cmek.ring_already_exists("{}", self.DEPLOYED,
                                              "europe-west4-a")
        exists.assert_called_once_with(
            "{}", "keys-proj", "europe-west4", "ofe-cmek")
        return result

    def test_found_and_not_found_are_answered_as_before(self):
        self.assertIs(self.check(return_value=True), True)
        self.assertIs(self.check(return_value=False), False)

    def test_an_error_before_cloud_kms_answers_is_explained(self):
        from google.auth.exceptions import DefaultCredentialsError
        from ..cluster_manager import cloud_info
        for cause in (DefaultCredentialsError("No credentials"),
                      ValueError("Service account info was not in the "
                                 "expected format"),
                      OSError("Network is unreachable")):
            with self.subTest(cause=type(cause).__name__):
                with self.assertRaises(cloud_info.KeyRingCheckError) as caught:
                    self.check(side_effect=cause)
                self.assertEqual(
                    str(caught.exception),
                    "Cannot check the Cloud KMS key ring projects/keys-proj/"
                    f"locations/europe-west4/keyRings/ofe-cmek: {cause}")
                self.assertIs(caught.exception.__cause__, cause)

    def test_a_cloud_kms_answer_is_passed_on_unchanged(self):
        from ..cluster_manager import cloud_info
        for error in (cloud_info.KeyRingCheckError("HTTP 403", 403),
                      cmek.CmekConfigError("bad setting")):
            with self.subTest(error=type(error).__name__):
                with self.assertRaises(type(error)) as caught:
                    self.check(side_effect=error)
                self.assertIs(caught.exception, error)


class AdminCannotChangeTheFlagsTests(SimpleTestCase):
    """Un-revoking or unfreezing a resource by hand is not offered."""

    def test_every_model_with_the_flags_shows_them_read_only(self):
        from django.contrib import admin
        from ..models import CmekResourceMixin
        models = [model for model in admin.site._registry
                  if issubclass(model, CmekResourceMixin)]
        self.assertEqual(sorted(model.__name__ for model in models),
                         ["Cluster", "GCPFilestoreFilesystem", "Image"])
        for model in models:
            with self.subTest(model=model.__name__):
                readonly = admin.site._registry[model].get_readonly_fields(
                    None)
                self.assertIn("cmek_applied", readonly)
                self.assertIn("cmek_revoked", readonly)

    def test_the_cluster_admin_keeps_its_inlines_and_columns(self):
        from django.contrib import admin
        from ..models import Cluster
        cluster_admin = admin.site._registry[Cluster]
        self.assertEqual(len(cluster_admin.inlines), 3)
        self.assertIn("_controller_node", cluster_admin.list_display)


class NoShadowedDjangoSettingsTests(SimpleTestCase):
    """`settings` is django.conf.settings in image.py, and it is used for
    MEDIA_ROOT after the CMEK block. A local of that name shadows it, and the
    AttributeError only fires for an image that has a startup script.
    """

    def test_image_py_does_not_rebind_settings(self):
        import ast
        from pathlib import Path

        src = (Path(__file__).resolve().parents[1]
               / "cluster_manager/image.py").read_text()
        for node in ast.walk(ast.parse(src)):
            if isinstance(node, ast.Assign):
                for t in node.targets:
                    if isinstance(t, ast.Name) and t.id == "settings":
                        self.fail("image.py rebinds `settings`, shadowing "
                                  "django.conf.settings used for MEDIA_ROOT")


class ResourceKeyPathTests(SimpleTestCase):
    """resource_key_path() rebuilds the path of the key a resource created.

    It is what the create-time uniqueness check and the read-only keys overview
    both read, so a resource whose path cannot be rebuilt is invisible to both.
    fsone in the live shared-key validation is the case that matters: it
    created its own key, and the path exists only as the three fields the
    deploy wrote back.
    """

    def _row(self, **kwargs):
        base = {"cmek_key_name": "", "cmek_key_ring_name": "",
                "cmek_key_project_id": "", "cloud_zone": "",
                "cloud_region": ""}
        base.update(kwargs)
        return SimpleNamespace(**base)

    def test_a_key_the_resource_created_for_itself_resolves(self):
        row = self._row(cmek_key_name="filestore-fsone",
                        cmek_key_ring_name="ofe-cmek-v1",
                        cmek_key_project_id="keys-proj",
                        cloud_zone="europe-west4-a")
        self.assertEqual(
            cmek.resource_key_path(row),
            "projects/keys-proj/locations/europe-west4"
            "/keyRings/ofe-cmek-v1/cryptoKeys/filestore-fsone")

    def test_falls_back_to_cloud_region_when_there_is_no_zone(self):
        # Image and Cluster records may carry only a region
        row = self._row(cmek_key_name="k", cmek_key_ring_name="r",
                        cmek_key_project_id="p", cloud_region="europe-west4")
        self.assertEqual(
            cmek.resource_key_path(row),
            "projects/p/locations/europe-west4/keyRings/r/cryptoKeys/k")

    def test_a_resource_with_no_key_recorded_has_no_path(self):
        self.assertEqual(cmek.resource_key_path(self._row()), "")

    def test_a_created_key_with_no_location_cannot_be_resolved(self):
        # a location-less path would be unusable and unparsable, so it must not
        # be offered to the uniqueness check as though it were comparable
        row = self._row(cmek_key_name="k", cmek_key_ring_name="r",
                        cmek_key_project_id="p")
        self.assertEqual(cmek.resource_key_path(row), "")

    def test_the_path_round_trips_through_parse_key_path(self):
        # the keys overview splits the path back into its parts to build the
        # Cloud Console links, so the two must agree
        row = self._row(cmek_key_name="k", cmek_key_ring_name="r",
                        cmek_key_project_id="p", cloud_region="europe-west4")
        parts = cmek.parse_key_path(cmek.resource_key_path(row))
        self.assertEqual(
            (parts["project"], parts["location"], parts["ring"], parts["key"]),
            ("p", "europe-west4", "r", "k"))


class NoMultilineDjangoCommentsTests(SimpleTestCase):
    """A `{# ... #}` spanning multiple lines is not a Django comment tag at
    all -- Django's template language only recognizes it on a single line --
    so the literal text, backticks included, renders straight into the
    blueprint and breaks the YAML. Found live: cluster_config.yaml.j2:41
    leaked exactly this into a cluster blueprint (BF: `{#` at char 1183).
    {% comment %}...{% endcomment %} is required for anything longer.
    """

    def test_no_template_has_a_multiline_hash_comment(self):
        import re
        from pathlib import Path

        root = Path(__file__).resolve().parents[1] / "templates"
        offenders = []
        for pattern in ("*.j2", "*.html"):
            for path in root.rglob(pattern):
                text = path.read_text()
                for m in re.finditer(r"\{#.*?#\}", text, re.S):
                    if "\n" in m.group(0):
                        offenders.append(str(path.relative_to(root)))
        self.assertEqual(offenders, [])


class LoginNodeReceivesTheKeyTests(SimpleTestCase):
    """The login node's boot disk must be encrypted too, not just the
    controller's.

    clusterinfo.py's _prepare_ghpc_yaml() already builds a `login_uses`
    context variable from cmek_refs (only those: the login module has no
    network_storage input for a filesystem to match), but the template never
    rendered it into the login module's block -- it had no `use:` list at
    all. Reproduced live: a real cluster's login node disk showed
    diskEncryptionKey: null while the controller's disk was correctly
    encrypted with the cluster's key, the two modules having been wired
    asymmetrically since this file was rebuilt.
    """

    def test_context_builds_a_login_uses_value(self):
        import ast
        from pathlib import Path

        src = (Path(__file__).resolve().parents[1]
               / "cluster_manager/clusterinfo.py").read_text()
        func = next(n for n in ast.walk(ast.parse(src))
                    if isinstance(n, ast.FunctionDef)
                    and n.name == "_prepare_ghpc_yaml")
        self.assertIn("'login_uses'", ast.unparse(func))

    def test_the_template_actually_renders_login_uses(self):
        # the missing half of the wiring: computing the value is not enough
        # if the template never places it in the login module's block
        from pathlib import Path

        tpl = (Path(__file__).resolve().parents[1]
               / "templates/blueprint/cluster_config.yaml.j2").read_text()
        login_block = tpl[tpl.index("id: slurm_login"):]
        self.assertIn("login_uses", login_block)
        self.assertIn("use:", login_block)


class OneKeyPerResourceTests(SimpleTestCase):
    """Two resources must never end up on one key.

    kms-key-iam's service agents are project-scoped, so two resources sharing a
    key hold the identical (member, role) grant, and either one's teardown
    revokes it for both. Reproduced live: deleting fsthree, which had adopted
    fsone's key, left fsone's key with an empty IAM policy while fsone was still
    READY and its own Terraform state still believed the grant existed.

    The adoption path that made that easy is gone, and the derived key name is
    unique per resource, so the only route left is typing one resource's key
    name into another. That is refused at creation, before anything is
    deployed. resources_sharing_key() needs a database to scan real rows, so
    these check the wiring at the source level rather than exercising the query.
    """

    def _source(self, path):
        from pathlib import Path
        return (Path(__file__).resolve().parents[1] / path).read_text()

    def _function(self, path, funcname):
        import ast
        func = next(n for n in ast.walk(ast.parse(self._source(path)))
                    if isinstance(n, ast.FunctionDef) and n.name == funcname)
        return ast.unparse(func)

    def test_the_form_refuses_a_key_another_resource_already_uses(self):
        body = self._function("forms.py", "_reject_a_key_another_resource_uses")
        self.assertIn("resources_sharing_key", body)
        self.assertIn("add_error", body)

    def test_the_panel_runs_that_check(self):
        self.assertIn("_reject_a_key_another_resource_uses",
                      self._function("forms.py", "clean_cmek_panel"))

    def test_the_scan_covers_all_three_cmek_resource_kinds(self):
        body = self._function("cluster_manager/cmek.py",
                              "resources_sharing_key")
        for model in ("Cluster", "GCPFilestoreFilesystem", "Image"):
            self.assertIn(model, body)

    def test_the_scan_ignores_torn_down_resources(self):
        # a destroyed resource keeps its key fields, and counting it would
        # refuse a key that is genuinely free
        body = self._function("cluster_manager/cmek.py",
                              "resources_sharing_key")
        self.assertIn("cloud_state", body)
        self.assertIn("'xm'", body.replace('"xm"', "'xm'"))

    def test_no_key_path_means_no_query(self):
        # a resource with no resolvable key has nothing to compare against and
        # nothing worth a database round trip for
        self.assertEqual(cmek.resources_sharing_key(""), [])

    def test_no_adoption_path_remains(self):
        # pre-existing-kms-key adopts a key another resource may already use,
        # which is the collision this whole class exists to prevent
        src = self._source("cluster_manager/cmek.py")
        self.assertNotIn("pre-existing-kms-key", src)
        self.assertFalse(hasattr(cmek, "MODE_EXISTING"))


class ImageKeyOutlivesTheBuilderEnvTests(SimpleTestCase):
    """The image's key must not be in the group destroyed after a build.

    kms-key and kms-key-iam used to sit in builder-env, which prepare()
    destroys as the last step of a *successful* build. That removed the grant
    Compute Engine needs to decrypt the image when it boots a node from it,
    and under DELETE destroyed the key itself, making the image unusable. The
    key belongs to the image, so it is torn down with the image instead.
    """

    def _source(self):
        from pathlib import Path
        return (Path(__file__).resolve().parents[1]
                / "cluster_manager/image.py").read_text()

    def test_the_key_has_its_own_deployment_group(self):
        src = self._source()
        self.assertIn('CMEK_GROUP = "image-cmek"', src)
        self.assertIn("{cmek_group}- group: builder-env", src)

    def test_the_builder_env_group_no_longer_carries_the_key(self):
        src = self._source()
        self.assertNotIn("{key_modules}  - id: network1", src)

    def test_the_key_group_is_applied_before_the_builder_env(self):
        import ast
        func = next(n for n in ast.walk(ast.parse(self._source()))
                    if isinstance(n, ast.FunctionDef) and n.name == "prepare")
        body = ast.unparse(func)
        self.assertLess(body.index("_create_cmek_env"),
                        body.index("_create_builder_env"))

    def test_the_key_is_destroyed_with_the_image_not_after_the_build(self):
        import ast
        tree = ast.parse(self._source())
        prepare = next(n for n in ast.walk(tree) if isinstance(
            n, ast.FunctionDef) and n.name == "prepare")
        delete = next(n for n in ast.walk(tree) if isinstance(
            n, ast.FunctionDef) and n.name == "delete_image")
        self.assertNotIn("_destroy_cmek_env", ast.unparse(prepare))
        self.assertIn("_destroy_cmek_env", ast.unparse(delete))


class ImageFormDoesNotRenderCmekFieldsTwiceTests(SimpleTestCase):
    """image-create.html looped over form.visible_fields generically, then
    included cmek/_panel.html right after -- rendering every cmek_* field
    twice, once raw and once through the panel. Since Django gives each
    field a fixed id (id_cmek_enabled, ...), the duplicate meant
    getElementById in _panel_script.html's show/hide JS grabbed the first,
    unstyled copy, leaving the properly-styled toggle in the panel with no
    listener attached at all -- reported live as "the toggle does nothing".
    filestore and cluster never hit this because their templates name each
    non-CMEK field individually rather than looping over every visible field.
    """

    def test_the_generic_field_loop_excludes_cmek_fields(self):
        from pathlib import Path

        tpl = (Path(__file__).resolve().parents[1]
               / "templates/image/image-create.html").read_text()
        loop_start = tpl.index("for field in form.visible_fields")
        guard = tpl[loop_start:loop_start + 200]
        self.assertIn("cmek_", guard)
        self.assertTrue("!=" in guard or "not in" in guard,
                        "the loop must skip cmek_-prefixed fields, or they "
                        "render a second time alongside cmek/_panel.html")

    def test_the_panel_is_still_included_exactly_once(self):
        from pathlib import Path

        tpl = (Path(__file__).resolve().parents[1]
               / "templates/image/image-create.html").read_text()
        self.assertEqual(tpl.count('include "cmek/_panel.html"'), 1)


class DeploymentGrantsKmsAccessTests(SimpleTestCase):
    """The deploy tooling must grant the credential Cloud KMS access.

    OFE creates a CryptoKey per encrypted resource, grants the service agents on
    it through kms-key-iam, and schedules its versions for destruction when an
    admin revokes one. All three need cloudkms.admin on the service account
    registered as OFE's credential.

    This regressed silently once already: CMEK was added while
    script/service_account.sh's role list was not, so every deployment landed
    unable to create a key, and the failure surfaced as an opaque Terraform
    permission error at apply rather than anything naming KMS.
    """

    def _role_script(self):
        from pathlib import Path
        # ofe/website/ghpcfe/tests -> ofe
        return (Path(__file__).resolve().parents[3]
                / "script/service_account.sh")

    def test_the_service_account_script_grants_cloudkms_admin(self):
        self.assertIn("'cloudkms.admin'", self._role_script().read_text())

    def test_it_is_inside_the_role_array_the_script_applies(self):
        # a role named only in a comment would grant nothing
        src = self._role_script().read_text()
        array = src[src.index("SA_ROLES=("):src.index(")", src.index("SA_ROLES=("))]
        self.assertIn("'cloudkms.admin'", array)

    def test_no_encrypter_decrypter_role_is_granted(self):
        # separation of duties: OFE administers keys, and must not be able to
        # decrypt the data they protect. kms-key-iam grants that to Google's own
        # service agents, never to OFE's credential.
        src = self._role_script().read_text()
        self.assertNotIn("cryptoKeyEncrypterDecrypter", src)

    def _function(self, name):
        """Body of a shell function in service_account.sh."""
        src = self._role_script().read_text()
        start = src.index(f"{name}() {{")
        return src[start:src.index("\n}\n", start)]

    def test_update_is_wired_to_its_own_function_which_checks_the_account(self):
        src = self._role_script().read_text()
        start = src.index('"update")')
        case = src[start:src.index(";;", start)]
        self.assertIn('update_service_account_roles "${project}" "${account}"',
                      case)
        body = self._function("update_service_account_roles")
        self.assertIn('service_account_exists "${project}" "${account}"', body)
        self.assertIn(
            'grant_service_account_roles "${project}" "${account}" || return 1',
            body)
        # a missing account stops update before anything is granted
        self.assertIn(
            'if ! service_account_exists "${project}" "${account}"; then\n'
            '\t\terror "Error: Service account [${account}] not found in '
            'project [${project}]."\n\t\treturn 1\n\tfi\n', body)
        self.assertLess(body.index("service_account_exists"),
                        body.index("grant_service_account_roles"))
        # and the case hands update's exit status back to the caller
        self.assertIn(
            'update_service_account_roles "${project}" "${account}"\n\trc=$?',
            case)

    def test_update_looks_the_account_up_by_exact_email(self):
        # the default gcloud table puts the display name in the first column, so
        # matching on it misses any account that has one (every account made in
        # the console), and an unanchored match can hit the wrong account
        body = self._function("service_account_exists")
        self.assertIn("--format='value(email)'", body)
        self.assertIn('--project="${project}"', body)
        self.assertIn('sa_expand "${project}" "${account}"', body)
        # the whole email, as a fixed string, matched as a whole line, and the
        # pipeline's status is the lookup's result
        self.assertTrue(body.rstrip().endswith('grep -Fxq "${sa_fullname}"'))
        self.assertNotIn("awk", body)

    def test_create_and_update_share_one_role_loop_that_only_adds_roles(self):
        src = self._role_script().read_text()
        self.assertEqual(src.count('for role in "${SA_ROLES[@]}"'), 1)
        grant = self._function("grant_service_account_roles")
        self.assertIn('for role in "${SA_ROLES[@]}"', grant)
        self.assertIn("add-iam-policy-binding", grant)
        self.assertNotIn("remove-iam-policy-binding", grant)
        # a role that cannot be granted must fail the command
        self.assertRegex(grant, r'failed\. Exiting\."\n\t\t\treturn 1')
        self.assertIn(
            'grant_service_account_roles "${project}" "${account}" || return 1',
            self._function("create_service_account"))

    def test_deploy_updates_roles_when_it_reuses_an_account(self):
        # an account made before encryption was added lacks cloudkms.admin, and
        # the deploy script assumed a reused account was right and never added it
        deploy = (self._role_script().parents[1] / "deploy.sh").read_text()
        start = deploy.index("Using existing service account")
        window = deploy[start:deploy.index("regenerate a credential", start)]
        self.assertIn("Grant any missing required roles to it? [Y/n]", window)
        self.assertIn("[Nn]*)", window)  # n skips
        call = window[window.index('service_account.sh" update'):]
        # project first, then account
        self.assertRegex(
            call, r'update \\\n\s+"\$\{project\}" "\$\{service_account\}"')
        self.assertIn("exit 1", call)  # a failed update aborts the deploy
        # and only the reuse branch updates roles
        self.assertEqual(deploy.count('service_account.sh" update'), 1)

    def test_the_docs_role_list_matches_the_script(self):
        import re
        root = self._role_script().parents[1]
        src = self._role_script().read_text()
        array = src[src.index("SA_ROLES=("):src.index("\n)", src.index("SA_ROLES=("))]
        script_roles = set(re.findall(r"'([A-Za-z.]+)'", array))
        docs = (root / "docs/AdvancedAdmin.md").read_text()
        first = docs.index("for roleid in")
        loop = docs[first:docs.index("; do", first)]
        docs_roles = set(re.findall(r"[A-Za-z]+(?:\.[A-Za-z]+)+", loop))
        self.assertEqual(docs_roles, script_roles)


class ImageBridgeExportsEveryGroupItImportsTests(SimpleTestCase):
    """import-inputs reads an exported outputs file per group packer `use`s.

    Live-found 2026-10-08: an encrypted image never started building.
    `ghpc import-inputs <dep>/packer-image` failed with "image-cmek_outputs.tfvars
    could not be read", because only the builder env's outputs were exported and
    the key group, applied with plain terraform, writes no export artifact.
    """

    def _commands(self, cmek_in_use):
        from ..cluster_manager import image
        backend = image.ImageBackend.__new__(image.ImageBackend)
        backend.image_dir = Path(tempfile.mkdtemp())
        backend.blueprint_name = "image_9"
        backend.ghpc_path = "ghpc"
        backend.image = SimpleNamespace(id=9)
        backend._cmek_in_use = cmek_in_use
        backend._get_credentials_file = lambda: "/dev/null"
        calls = []
        with mock.patch.object(
                image.subprocess, "run",
                side_effect=lambda cmd, **kwargs: calls.append(cmd)):
            backend._run_ghpc_import_inputs()
        return calls

    def test_an_unencrypted_image_exports_only_the_builder_env(self):
        self.assertEqual(self._commands(False), [
            ["ghpc", "export-outputs", "image_9/builder-env"],
            ["ghpc", "import-inputs", "image_9/packer-image"]])

    def test_an_encrypted_image_also_exports_the_key_group_first(self):
        self.assertEqual(self._commands(True), [
            ["ghpc", "export-outputs", "image_9/builder-env"],
            ["ghpc", "export-outputs", "image_9/image-cmek"],
            ["ghpc", "import-inputs", "image_9/packer-image"]])


class VpcDetailToleratesDraftClustersTests(SimpleTestCase):
    """Visiting "Create cluster" saves a cluster row before its form is posted.

    Live-found 2026-10-08: that draft has no subnet, so /vpc/<id> raised
    AttributeError ('NoneType' object has no attribute 'vpc') for every admin
    until the draft was finished or deleted.
    """

    def test_a_draft_cluster_is_skipped_not_fatal(self):
        from ..views import vpc as vpc_views
        network = object()
        draft = SimpleNamespace(subnet=None)
        attached = SimpleNamespace(subnet=SimpleNamespace(vpc=network))
        elsewhere = SimpleNamespace(subnet=SimpleNamespace(vpc=object()))
        view = vpc_views.VPCDetailView()
        view.kwargs = {"pk": 1}
        with mock.patch(
                "django.views.generic.DetailView.get_context_data",
                return_value={}), \
                mock.patch.object(vpc_views, "get_object_or_404",
                                  return_value=network), \
                mock.patch.object(vpc_views.VirtualSubnet, "objects"), \
                mock.patch.object(vpc_views.Cluster, "objects") as clusters, \
                mock.patch.object(vpc_views.Filesystem, "objects") as fss, \
                mock.patch.object(vpc_views.Workbench, "objects") as wbs:
            clusters.exclude.return_value = [draft, attached, elsewhere]
            fss.exclude.return_value = []
            wbs.exclude.return_value = []
            context = view.get_context_data()
        self.assertEqual(context["used_in_clusters"], [attached])



class RegistryCredentialSecretIsEncryptedTests(SimpleTestCase):
    """The registry password's Secret Manager secret must use the cluster key.

    Live-found 2026-10-08: with CMEK on and a registry that has upstream
    credentials, the repository was encrypted with the cluster key but its
    credential secret used Google-managed automatic replication, and the key was
    not granted to the Secret Manager agent.
    """

    def _render(self, **overrides):
        from django.template.loader import render_to_string
        context = {"registry_id": "registry_1", "repo_mode": "REMOTE_REPOSITORY",
                   "format": "DOCKER", "use_public_repository": True,
                   "repo_mirror_url": "", "repo_username": "u",
                   "repo_password": "p", "use_upstream_credentials": True,
                   "cluster": SimpleNamespace(), "cmek": True,
                   "cmek_location": "europe-west4"}
        context.update(overrides)
        return render_to_string("blueprint/artifact_registry_config.yaml.j2",
                                context)

    def test_the_secret_uses_the_cluster_key_in_the_cluster_region(self):
        import yaml
        module = yaml.safe_load(self._render())[0]
        self.assertEqual(
            module["settings"]["user_managed_replication"],
            [{"location": "europe-west4",
              "kms_key_name": "$(cmek_key_iam.crypto_key_id)"}])

    def test_a_mirror_url_registry_is_encrypted_the_same_way(self):
        import yaml
        module = yaml.safe_load(self._render(
            use_public_repository=False,
            repo_mirror_url="https://registry.k8s.io"))[0]
        self.assertEqual(module["settings"]["repo_mirror_url"],
                         "https://registry.k8s.io")
        self.assertEqual(
            module["settings"]["user_managed_replication"],
            [{"location": "europe-west4",
              "kms_key_name": "$(cmek_key_iam.crypto_key_id)"}])

    def test_no_replication_setting_without_credentials(self):
        self.assertNotIn("user_managed_replication",
                         self._render(use_upstream_credentials=False))

    def test_no_replication_setting_when_not_encrypted(self):
        self.assertNotIn("user_managed_replication",
                         self._render(cmek=False))

    def _through_clusterinfo(self, cmek_on):
        """Render via ClusterInfo._prepare_ghpc_artifact_registry itself."""
        import yaml
        from django.template import engines
        from ..cluster_manager import clusterinfo

        class Registries(list):
            def exists(self):
                return bool(self)

        registry = SimpleNamespace(
            id=1, status="n", cloud_state="", repo_mode="REMOTE_REPOSITORY",
            format="DOCKER", use_public_repository=True, repo_mirror_url="",
            repo_username="u", repo_password="p",
            use_upstream_credentials=True, save=lambda **kwargs: None)
        info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
        info.env = engines["django"]
        info._cmek_in_use = cmek_on
        info.cluster = SimpleNamespace(
            use_containers=True, cloud_region="europe-west4",
            container_registry_relations=SimpleNamespace(
                exclude=lambda **kwargs: Registries([registry])))
        rendered, has_registries = info._prepare_ghpc_artifact_registry()
        self.assertTrue(has_registries)
        return yaml.safe_load(rendered)[0]

    def test_clusterinfo_passes_the_cluster_region_as_the_location(self):
        module = self._through_clusterinfo(cmek_on=True)
        self.assertEqual(
            module["settings"]["user_managed_replication"],
            [{"location": "europe-west4",
              "kms_key_name": "$(cmek_key_iam.crypto_key_id)"}])

    def test_clusterinfo_adds_no_replication_when_cmek_is_off(self):
        module = self._through_clusterinfo(cmek_on=False)
        self.assertNotIn("user_managed_replication", module["settings"])


# A key project typed into a form, written unquoted into the blueprint, would
# close the settings mapping and append a module of the typist's choosing.
INJECTION = "p\n  - id: evil\n    source: git::https://example.com/x.git"


def modules(out):
    """The emitted blocks, as the blueprint's YAML parser will read them."""
    return yaml.safe_load(out)


class BlueprintInjectionTests(SimpleTestCase):
    """Every value OFE writes into a blueprint is data, never YAML syntax."""

    def test_a_key_project_cannot_add_a_module(self):
        got = modules(emit(key_project_id=INJECTION))
        self.assertEqual([m["id"] for m in got],
                         [cmek.KEY_MODULE_ID, cmek.KEY_IAM_MODULE_ID])
        self.assertEqual(got[0]["settings"]["project_id"], INJECTION)

    def test_a_key_ring_with_a_trailing_newline_is_refused(self):
        # `$` also matches before a final newline, so the check must be a
        # full match (the emitter strips what it is given; the settings
        # reader does not)
        with self.assertRaises(cmek.CmekConfigError):
            cmek.CmekSettings.from_config(
                {"cmek": {"key_ring_name": "ofe-cmek\n"}})

    def test_a_key_ring_carrying_a_module_is_refused(self):
        with self.assertRaises(cmek.CmekConfigError):
            emit(key_ring_name=INJECTION)

    def test_the_settings_key_project_is_refused(self):
        with self.assertRaises(cmek.CmekConfigError):
            cmek.CmekSettings.from_config(
                {"cmek": {"key_project_id": INJECTION}})

    def test_the_superuser_settings_form_names_the_key_project(self):
        from ..forms import CmekSettingsForm
        form = CmekSettingsForm({"enforcement": "preferred",
                                 "key_project_id": INJECTION})
        self.assertFalse(form.is_valid())
        self.assertIn("key_project_id", form.errors)

    def test_list_syntax_in_an_account_is_refused(self):
        for char in (",", "[", "]", "#"):
            with self.subTest(char=char):
                with self.assertRaises(cmek.CmekConfigError):
                    cmek.assert_service_account_email(
                        f"a{char}b@p.iam.gserviceaccount.com")

    def test_every_emitted_value_round_trips(self):
        # "null" and "0123" are valid Cloud KMS names that unquoted YAML
        # reads as a null and an integer
        args = {"key_name": "null", "key_ring_name": "0123",
                "key_project_id": "example.com:keys-proj",
                "deletion_policy": "DELETE", "protection_level": "HSM",
                "rotation_period": "7776000s",
                "encryption_service_account": "me@p.iam.gserviceaccount.com",
                "custom_service_accounts": ["you@p.iam.gserviceaccount.com"],
                "extra_service_agents": ["cloudsql"]}
        for ring_exists in (False, True):
            with self.subTest(ring_exists=ring_exists):
                key, iam = modules(emit(ring_exists=ring_exists, **args))
                expected_key = {
                    "project_id": "example.com:keys-proj",
                    "location": "europe-west4",
                    "key_name": "null",
                    "deletion_policy": "DELETE",
                    "protection_level": "HSM",
                    "rotation_period": "7776000s",
                }
                if ring_exists:
                    expected_key["key_ring_id"] = (
                        "projects/example.com:keys-proj/locations/"
                        "europe-west4/keyRings/0123")
                else:
                    expected_key["key_ring_name"] = "0123"
                self.assertEqual(key["settings"], expected_key)
                self.assertEqual(iam["settings"], {
                    "project_id": "workload-proj",
                    "service_agents": ["compute", "storage", "cloudsql"],
                    "custom_service_accounts": [
                        "you@p.iam.gserviceaccount.com",
                        "me@p.iam.gserviceaccount.com"],
                })


class ProjectIdTests(SimpleTestCase):
    def test_a_domain_scoped_project_is_accepted(self):
        self.assertEqual(cmek.assert_project_id("example.com:proj-1"),
                         "example.com:proj-1")
        settings = cmek.CmekSettings.from_config(
            {"cmek": {"key_project_id": "example.com:proj-1"}})
        self.assertEqual(settings.key_project_id, "example.com:proj-1")

    def test_a_malformed_project_is_refused(self):
        for value in ("Bad_Project!", "short", "abcde", "proj-", "1proj-abc",
                      "a" * 31, "My-project", "my_project",
                      "example.com:", "example.com:a-", "example.com:1abc",
                      "example.com:My-proj", "example.com:" + "a" * 31,
                      "example.com-:proj-123"):
            with self.subTest(value=value):
                with self.assertRaises(cmek.CmekConfigError):
                    cmek.assert_project_id(value)

    def test_a_bare_project_is_6_to_30_characters(self):
        for value in ("abcdef", "a" * 30, "proj-123"):
            with self.subTest(value=value):
                self.assertEqual(cmek.assert_project_id(value), value)

    def test_a_domain_scoped_project_may_be_short(self):
        # only the bare form carries the 6-character minimum
        for value in ("example.com:a", "example.com:ab", "google.com:proj-123",
                      "example.com:" + "a" * 30):
            with self.subTest(value=value):
                self.assertEqual(cmek.assert_project_id(value), value)

    def test_blank_means_unset(self):
        self.assertEqual(cmek.assert_project_id(None), "")


class FrozenEncryptionTests(SimpleTestCase):
    """A deployed resource keeps the encryption it was deployed with.

    Enforcement changed after the deploy would otherwise strip the key blocks
    from a reconfigured cluster's blueprint (disabled) or add a key to a
    resource whose disks were created without one (required).
    """

    def resource(self, **kwargs):
        base = {"pk": 1, "cloud_state": "m", "cmek_enabled": True,
                "cmek_key_name": "fs-key", "cmek_key_project_id": "keys-proj",
                "cmek_key_ring_name": "ofe-cmek",
                "cmek_deletion_policy": "ABANDON",
                "cmek_encryption_service_account": ""}
        base.update(kwargs)
        return SimpleNamespace(**base)

    def test_a_deployed_filestore_is_frozen(self):
        from ..models import GCPFilestoreFilesystem
        for state, frozen in (("nm", False), ("cm", True), ("m", True),
                              ("re", True)):
            with self.subTest(state=state):
                fs = GCPFilestoreFilesystem(pk=1, cloud_state=state)
                self.assertEqual(cmek.is_frozen(fs), frozen)

    def test_a_deployed_cluster_is_frozen(self):
        from ..models import Cluster
        for state, status, frozen in (("nm", "n", False), ("nm", "e", False),
                                      ("m", "i", True), ("nm", "r", True),
                                      ("re", "re", True)):
            with self.subTest(state=state, status=status):
                cluster = Cluster(pk=1, cloud_state=state, status=status)
                self.assertEqual(cmek.is_frozen(cluster), frozen)

    def test_a_successful_apply_freezes_whatever_the_state_says(self):
        # a failed start or reconfigure puts the record back to "nm" (and a
        # cluster to "e"), which the state alone reads as never deployed
        from ..models import Cluster, GCPFilestoreFilesystem
        for resource in (
                GCPFilestoreFilesystem(pk=1, cloud_state="nm",
                                       cmek_applied=True),
                Cluster(pk=1, cloud_state="nm", status="e",
                        cmek_applied=True),
                Cluster(pk=1, cloud_state="nm", status="n",
                        cmek_applied=True)):
            with self.subTest(resource=type(resource).__name__,
                              status=getattr(resource, "status", None)):
                self.assertTrue(cmek.is_frozen(resource))

    def test_a_revoked_key_freezes_whatever_the_state_says(self):
        from ..models import Cluster, GCPFilestoreFilesystem, Image
        for resource in (
                GCPFilestoreFilesystem(pk=1, cloud_state="nm",
                                       cmek_revoked=True),
                Cluster(pk=1, cloud_state="nm", status="e", cmek_revoked=True),
                Image(pk=1, cloud_state="nm", status="n", cmek_revoked=True)):
            with self.subTest(resource=type(resource).__name__):
                self.assertTrue(cmek.is_frozen(resource))

    def test_an_applied_image_is_frozen(self):
        from ..models import Image
        self.assertTrue(cmek.is_frozen(
            Image(pk=1, cloud_state="nm", status="n", cmek_applied=True)))
        self.assertTrue(cmek.is_frozen(
            Image(pk=1, cloud_state="m", status="r", cmek_applied=True)))

    def test_the_snapshot_freezes_an_image_too(self):
        from ..models import Image
        image = Image(pk=1, cloud_state="nm", status="n")
        image._cmek_deployed = True  # pylint: disable=protected-access
        self.assertTrue(cmek.is_frozen(image))

    def test_flags_left_false_fall_back_to_the_state(self):
        from ..models import Cluster, GCPFilestoreFilesystem
        for resource, frozen in (
                (GCPFilestoreFilesystem(pk=1, cloud_state="nm"), False),
                (GCPFilestoreFilesystem(pk=1, cloud_state="m"), True),
                (Cluster(pk=1, cloud_state="nm", status="e"), False),
                (Cluster(pk=1, cloud_state="m", status="r"), True)):
            with self.subTest(resource=type(resource).__name__,
                              state=resource.cloud_state):
                self.assertFalse(resource.cmek_applied)
                self.assertFalse(resource.cmek_revoked)
                self.assertEqual(cmek.is_frozen(resource), frozen)

    def test_an_unsaved_resource_is_never_frozen_even_with_the_flags(self):
        from ..models import GCPFilestoreFilesystem, Image
        self.assertFalse(cmek.is_frozen(GCPFilestoreFilesystem(
            cloud_state="nm", cmek_applied=True, cmek_revoked=True)))
        self.assertFalse(cmek.is_frozen(Image(cmek_applied=True)))

    def test_only_a_real_true_freezes(self):
        # a MagicMock answers every attribute with another (truthy) mock,
        # which must not read as an applied or revoked resource
        stand_in = mock.MagicMock(pk=1, cloud_state="nm", status="n")
        self.assertFalse(cmek.is_frozen(stand_in))
        self.assertFalse(cmek.is_frozen(SimpleNamespace(
            pk=1, cloud_state="nm", cmek_applied=1, cmek_revoked="yes")))

    def test_images_and_unsaved_resources_are_not_frozen(self):
        from ..models import GCPFilestoreFilesystem, Image
        self.assertFalse(cmek.is_frozen(
            Image(pk=1, cloud_state="m", status="r")))
        self.assertFalse(cmek.is_frozen(
            GCPFilestoreFilesystem(cloud_state="m")))
        self.assertFalse(cmek.is_frozen(None))

    def test_deployed_encrypted_keeps_its_key_when_enforcement_is_disabled(self):
        got = cmek.resource_settings(self.resource(),
                                     settings_for(enforcement="disabled"))
        self.assertIsNotNone(got)
        self.assertEqual(got["key_name"], "fs-key")
        out = cmek.key_modules_yaml(
            service="filestore", project_id="workload-proj",
            location="europe-west4", settings=settings_for(), **got)
        self.assertEqual(sources(out), [cmek.KEY_MODULE_SOURCE,
                                        cmek.KEY_IAM_MODULE_SOURCE])

    def test_deployed_unencrypted_stays_so_under_required(self):
        self.assertIsNone(cmek.resource_settings(
            self.resource(cmek_enabled=False),
            settings_for(enforcement="required")))

    def test_an_undeployed_resource_follows_enforcement(self):
        new = self.resource(cloud_state="nm", cmek_enabled=False)
        self.assertIsNotNone(cmek.resource_settings(
            new, settings_for(enforcement="required")))
        self.assertIsNone(cmek.resource_settings(
            self.resource(cloud_state="nm"),
            settings_for(enforcement="disabled")))


class RevokedKeyTests(SimpleTestCase):
    """A resource whose key an administrator revoked cannot be used again."""

    def test_a_revoked_resource_is_refused_by_name(self):
        with self.assertRaises(cmek.CmekConfigError) as caught:
            cmek.assert_not_revoked(
                SimpleNamespace(name="fsone", cmek_revoked=True))
        message = str(caught.exception)
        self.assertIn("'fsone'", message)
        self.assertIn("revoked by an administrator", message)
        self.assertIn("Destroy it and create a new one", message)
        self.assertIn("does not clear this", message)

    def test_anything_else_passes(self):
        for resource in (SimpleNamespace(name="fsone", cmek_revoked=False),
                         SimpleNamespace(name="fsone"),
                         mock.MagicMock(),
                         None):
            with self.subTest(resource=resource):
                self.assertIsNone(cmek.assert_not_revoked(resource))

    def test_reading_the_settings_still_works_once_revoked(self):
        # the forms and the detail page read resource_settings, and must
        # still show what a revoked resource was encrypted with, whatever
        # enforcement says now
        revoked = SimpleNamespace(
            pk=1, name="fsone", cloud_state="nm", cmek_revoked=True,
            cmek_enabled=True, cmek_key_name="fs-key",
            cmek_key_project_id="keys-proj", cmek_key_ring_name="ofe-cmek",
            cmek_deletion_policy="ABANDON",
            cmek_encryption_service_account="")
        got = cmek.resource_settings(revoked,
                                     settings_for(enforcement="disabled"))
        self.assertEqual(got["key_name"], "fs-key")


class _Rows(list):
    """Stands in for a queryset: filter and exclude are applied by the test."""

    def filter(self, **kwargs):
        return self

    def exclude(self, **kwargs):
        return self


class ResolvedDuplicateKeyTests(SimpleTestCase):
    """Resources are compared by where their key will live, not by the raw
    columns: an undeployed resource still has a blank project and key ring,
    which the deploy fills in from the deployment settings and credential.
    """

    PATH = ("projects/cred-proj/locations/europe-west4/keyRings/ofe-cmek"
            "/cryptoKeys/dup")

    def scan(self, rows, **kwargs):
        from ..models import Cluster, GCPFilestoreFilesystem, Image
        with mock.patch.object(GCPFilestoreFilesystem, "objects",
                               _Rows(rows)), \
                mock.patch.object(Cluster, "objects", _Rows()), \
                mock.patch.object(Image, "objects", _Rows()):
            return cmek.resources_sharing_key(self.PATH, **kwargs)

    def row(self, **kwargs):
        base = {"pk": 2, "name": "fsone", "cloud_state": "nm",
                "cmek_enabled": True, "cmek_key_name": "dup",
                "cmek_key_ring_name": "", "cmek_key_project_id": "",
                "cloud_zone": "europe-west4-a", "project_id": "cred-proj"}
        base.update(kwargs)
        return SimpleNamespace(**base)

    def test_two_undeployed_resources_with_one_typed_name_collide(self):
        self.assertEqual(
            self.scan([self.row()], settings=settings_for(key_project_id="")),
            ["Filestore 'fsone'"])

    def test_a_deployed_row_is_compared_by_its_stored_path(self):
        deployed = self.row(cloud_state="m", cmek_key_ring_name="ofe-cmek",
                            cmek_key_project_id="cred-proj")
        self.assertEqual(self.scan([deployed]), ["Filestore 'fsone'"])
        self.assertEqual(
            self.scan([deployed], settings=settings_for(key_project_id="")),
            ["Filestore 'fsone'"])

    def test_a_blank_name_never_collides(self):
        # OFE derives a name unique to that resource
        self.assertEqual(
            self.scan([self.row(cmek_key_name="")],
                      settings=settings_for(key_project_id="")), [])
