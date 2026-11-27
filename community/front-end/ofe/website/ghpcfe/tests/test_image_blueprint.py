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

"""The image build blueprint that ImageBackend writes for gcluster"""

import json
import os
import re
import tempfile
from pathlib import Path
from unittest import mock

import yaml
from django.conf import settings
from django.test import TestCase

from ghpcfe.cluster_manager import utils
from ghpcfe.cluster_manager.image import ImageBackend
from ghpcfe.models import Credential, Image, StartupScript, User

PROJECT = "example-project-1"
CLOUD_PLATFORM_SCOPE = "https://www.googleapis.com/auth/cloud-platform"

# The checks modules/project/service-account and
# community/modules/iam/service-account-profiles apply to their
# deployment_name and name settings (their metadata.yaml validators)
ACCOUNT_ID_PATTERN = re.compile(r"^[a-z][-a-z0-9]{4,28}[a-z0-9]$")
ROLE_ID_PARTS_PATTERN = re.compile(
    r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?[.][a-z0-9]([-a-z0-9]*[a-z0-9])?$"
)


class ImageBlueprintTestBase(TestCase):
    """Creates an image with one startup script and renders its blueprint"""

    def setUp(self):
        # Image ids repeat between tests, so each test writes below its own
        # base directory
        base_dir = tempfile.TemporaryDirectory(prefix="ofe-image-tests-")
        self.addCleanup(base_dir.cleanup)
        config_patch = mock.patch.dict(
            utils.load_config(), {"baseDir": Path(base_dir.name)}
        )
        config_patch.start()
        self.addCleanup(config_patch.stop)
        self.owner = User.objects.create_user(
            username="image-owner", password="image-owner-pass"
        )
        self.script = StartupScript.objects.create(
            name="hello.sh",
            type="shell",
            content="startup-scripts/hello.sh",
            owner=self.owner,
        )

    def make_image(self, identity_model, project=PROJECT, name="base"):
        credential = Credential.objects.create(
            name=f"credential-{name}",
            owner=self.owner,
            detail=json.dumps({"type": "service_account", "project_id": project}),
        )
        image = Image.objects.create(
            name=name,
            family=f"{name}-family",
            source_image_project="example-source-project",
            source_image_family="example-source-family",
            owner=self.owner,
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )
        image.startup_script.add(self.script)
        image.identity_model = identity_model
        return image

    @staticmethod
    def backend_for(image):
        backend = ImageBackend(image)
        # _create_image_dir also writes the credential file, which these
        # tests do not need
        backend.image_dir.mkdir(parents=True)
        return backend

    def render(self, image):
        # The first two steps of ImageBackend.prepare(), with no cloud work
        # pylint: disable=protected-access
        backend = self.backend_for(image)
        backend._refresh_role_id_suffix()
        backend._create_blueprint()
        return (backend.image_dir / "image.yaml").read_text()

    @staticmethod
    def modules(blueprint, group):
        for deployment_group in blueprint["deployment_groups"]:
            if deployment_group["group"] == group:
                return {m["id"]: m for m in deployment_group["modules"]}
        raise AssertionError(f"no deployment group {group}")


class LegacyImageBlueprintTests(ImageBlueprintTestBase):
    """Legacy images render exactly as before service-account profiles"""

    def test_legacy_blueprint_is_unchanged(self):
        image = self.make_image("legacy")
        script_path = os.path.join(settings.MEDIA_ROOT, "startup-scripts/hello.sh")
        # The packer module takes the builder network through `use`, as
        # gcluster rejects a blueprint with an unused module
        expected = "\n".join([
            f"blueprint_name: image_{image.id}",
            "",
            # The builder's Terraform state is kept in the state bucket
            "terraform_backend_defaults:",
            "  type: gcs",
            "  configuration:",
            f"    bucket: {utils.get_terraform_state_bucket()}",
            f"    prefix: {utils.terraform_state_prefix('image', image.id)}",
            "",
            "vars:",
            f"  project_id: {PROJECT}",
            f"  deployment_name: image_{image.id}",
            "  region: us-central1",
            "  zone: us-central1-a",
            f"  network_name: image-{image.id}-network",
            f"  subnetwork_name: image{image.id}-subnetwork",
            "  image_name: image-base",
            "  image_family: image-base-family",
            "",
            "deployment_groups:",
            "- group: builder-env",
            "  modules:",
            "  - id: network1",
            "    source: modules/network/vpc",
            "    settings:",
            "      network_name: $(vars.network_name)",
            "",
            "  - id: scripts_for_image",
            "    source: modules/scripts/startup-script",
            "    settings:",
            "      runners:",
            "      - type: shell",
            "        destination: hello.sh",
            f"        source: {script_path}",
            "    outputs: [startup_script]",
            "",
            "- group: packer-image",
            "  modules:",
            "  - id: custom-image",
            "    source: modules/packer/custom-image",
            "    kind: packer",
            "    use:",
            "    - network1",
            "    - scripts_for_image",
            "    settings:",
            "      source_image_project_id: [example-source-project]",
            "      source_image_family: example-source-family",
            "      disk_size: 50",
            "      image_family: $(vars.image_family)",
            "      state_timeout: 30m",
            "      zone: $(vars.zone)",
            '      image_storage_locations: ["us-central1"]',
            "      metadata:",
            "        enable-oslogin: TRUE",
            "        block-project-ssh-keys: TRUE",
            "",
        ])
        self.assertEqual(self.render(image), expected)

    def test_legacy_blueprint_has_no_builder_identity(self):
        rendered = self.render(self.make_image("legacy"))
        self.assertNotIn("service-account", rendered)
        self.assertNotIn("service_account_email", rendered)
        self.assertNotIn("bucket_viewers", rendered)
        self.assertNotIn("role_id_suffix", rendered)


class ProfilesImageBlueprintTests(ImageBlueprintTestBase):
    """Profiles images build as their own image-builder account"""

    def setUp(self):
        super().setUp()
        self.image = self.make_image("profiles")
        self.blueprint = yaml.safe_load(self.render(self.image))
        self.builder_env = self.modules(self.blueprint, "builder-env")
        self.packer_image = self.modules(self.blueprint, "packer-image")

    def test_builder_account_block(self):
        account = self.builder_env["builder_sa"]
        self.assertEqual(account["source"], "modules/project/service-account")
        self.assertEqual(
            account["settings"],
            {
                "deployment_name": f"img-{self.image.id}",
                "name": "bld",
                "project_roles": [],
            },
        )

    def test_builder_profile_block(self):
        profile = self.builder_env["builder_profile"]
        self.assertEqual(
            profile["source"], "community/modules/iam/service-account-profiles"
        )
        self.assertEqual(profile["use"], ["builder_sa"])
        self.assertEqual(
            profile["settings"],
            {
                "deployment_name": f"img-{self.image.id}",
                "name": "bld",
                "profile": "image-builder",
                "role_id_suffix": self.image.role_id_suffix,
            },
        )

    def test_names_pass_the_module_checks(self):
        for block in ("builder_sa", "builder_profile"):
            settings_ = self.builder_env[block]["settings"]
            account_id = f"{settings_['deployment_name']}-{settings_['name']}"
            role_id_parts = f"{settings_['deployment_name']}.{settings_['name']}"
            self.assertRegex(account_id, ACCOUNT_ID_PATTERN)
            self.assertRegex(role_id_parts, ROLE_ID_PARTS_PATTERN)

    def test_packer_runs_as_the_builder_account(self):
        account = self.builder_env["builder_sa"]["settings"]
        expected_email = (
            f"{account['deployment_name']}-{account['name']}"
            f"@{PROJECT}.iam.gserviceaccount.com"
        )
        custom_image = self.packer_image["custom-image"]["settings"]
        self.assertEqual(custom_image["service_account_email"], expected_email)
        self.assertEqual(
            custom_image["service_account_scopes"], [CLOUD_PLATFORM_SCOPE]
        )

    def test_builder_reads_the_startup_script_bucket(self):
        scripts = self.builder_env["scripts_for_image"]["settings"]
        self.assertEqual(
            scripts["bucket_viewers"],
            ["serviceAccount:$(builder_profile.service_account_email)"],
        )

    def test_build_vm_name_prefix_is_unchanged(self):
        # The profile's scoped setMetadata grant covers instances named packer-*,
        # which is the custom-image module's own default
        custom_image = self.packer_image["custom-image"]["settings"]
        self.assertNotIn("instance_name", custom_image)
        self.assertNotIn(
            "instance_name_prefixes", self.builder_env["builder_profile"]["settings"]
        )

    def test_deployment_image_name_and_family_are_unchanged(self):
        variables = self.blueprint["vars"]
        self.assertEqual(self.blueprint["blueprint_name"], f"image_{self.image.id}")
        self.assertEqual(variables["deployment_name"], f"image_{self.image.id}")
        self.assertEqual(variables["image_name"], "image-base")
        self.assertEqual(variables["image_family"], "image-base-family")

    def test_email_project_comes_from_the_credential(self):
        image = self.make_image("profiles", project="example-project-2", name="other")
        blueprint = yaml.safe_load(self.render(image))
        custom_image = self.modules(blueprint, "packer-image")["custom-image"]
        self.assertEqual(
            custom_image["settings"]["service_account_email"],
            f"img-{image.id}-bld@example-project-2.iam.gserviceaccount.com",
        )
        self.assertEqual(blueprint["vars"]["project_id"], "example-project-2")


class UnknownIdentityModelTests(ImageBlueprintTestBase):
    """An unrecognised identity model fails the image instead of guessing"""

    def test_unknown_identity_model_marks_the_image_failed(self):
        image = self.make_image("unknown")
        backend = self.backend_for(image)
        # The error is raised again, so the build stops there
        with self.assertLogs(
            "ghpcfe.cluster_manager.image", level="ERROR"
        ), self.assertRaisesMessage(ValueError, "Unknown identity model 'unknown'"):
            backend._create_blueprint()  # pylint: disable=protected-access
        self.assertFalse((backend.image_dir / "image.yaml").exists())
        image.refresh_from_db()
        self.assertEqual(image.status, "e")


class RoleIdSuffixTests(ImageBlueprintTestBase):
    """Each image build gets custom role ids Google is not still holding"""

    # role_id_suffix validation in
    # community/modules/iam/service-account-profiles/variables.tf
    MODULE_SUFFIX_PATTERN = re.compile(r"^[a-z0-9]{1,8}$")
    GENERATED_SUFFIX_PATTERN = re.compile(r"^[0-9a-f]{6}$")

    @staticmethod
    def start_build(image):
        """The suffix step that ImageBackend.prepare() runs first"""
        # pylint: disable=protected-access
        ImageBackend(Image.objects.get(pk=image.pk))._refresh_role_id_suffix()
        return Image.objects.get(pk=image.pk).role_id_suffix

    def make_saved_image(self, identity_model, **kwargs):
        image = self.make_image(identity_model, **kwargs)
        image.save()
        return image

    def assert_valid_suffix(self, suffix):
        self.assertIsInstance(suffix, str)
        self.assertRegex(suffix, self.GENERATED_SUFFIX_PATTERN)
        self.assertRegex(suffix, self.MODULE_SUFFIX_PATTERN)
        self.assertNotEqual(suffix, "scoped")

    def test_unexpected_build_error_fails_the_image_and_renews_suffix(self):
        # The builder env, and its custom role, is destroyed after any build
        # error, so the image must not stay "n" and keep the deleted role id
        image = self.make_saved_image("profiles")
        first = self.start_build(image)
        backend = ImageBackend(Image.objects.get(pk=image.pk))
        with mock.patch.object(
            utils, "run_packer", side_effect=RuntimeError("unexpected")
        ):
            backend._create_image()  # pylint: disable=protected-access
        self.assertEqual(Image.objects.get(pk=image.pk).status, "e")
        second = self.start_build(image)
        self.assert_valid_suffix(second)
        self.assertNotEqual(second, first)

    def test_suffix_is_rendered_and_matches_the_module_pattern(self):
        image = self.make_saved_image("profiles")
        blueprint = yaml.safe_load(self.render(image))
        profile = self.modules(blueprint, "builder-env")["builder_profile"]
        suffix = profile["settings"]["role_id_suffix"]
        self.assert_valid_suffix(suffix)
        image.refresh_from_db()
        self.assertEqual(image.role_id_suffix, suffix)

    def test_first_build_generates_a_suffix(self):
        image = self.make_saved_image("profiles")
        self.assertEqual(image.role_id_suffix, "")
        self.assert_valid_suffix(self.start_build(image))

    def test_suffix_is_kept_when_an_unfinished_build_is_started_again(self):
        image = self.make_saved_image("profiles")
        first = self.start_build(image)
        self.assertEqual(Image.objects.get(pk=image.pk).status, "n")
        self.assertEqual(self.start_build(image), first)

    def test_suffix_is_regenerated_when_a_failed_image_is_rebuilt(self):
        image = self.make_saved_image("profiles")
        first = self.start_build(image)
        Image.objects.filter(pk=image.pk).update(status="e")
        second = self.start_build(image)
        self.assert_valid_suffix(second)
        self.assertNotEqual(second, first)

    def test_suffix_is_regenerated_when_a_built_image_is_rebuilt(self):
        image = self.make_saved_image("profiles")
        first = self.start_build(image)
        Image.objects.filter(pk=image.pk).update(status="r")
        second = self.start_build(image)
        self.assert_valid_suffix(second)
        self.assertNotEqual(second, first)

    def test_image_made_after_a_delete_gets_its_own_suffix(self):
        # The suffix lives on the row, so a deleted image's suffix goes with it
        # and the image made in its place starts without one
        image = self.make_saved_image("profiles")
        self.start_build(image)
        image.delete()
        replacement = self.make_saved_image("profiles")
        self.assertEqual(replacement.role_id_suffix, "")
        self.assert_valid_suffix(self.start_build(replacement))

    def test_legacy_image_gets_no_suffix(self):
        image = self.make_saved_image("legacy")
        self.assertEqual(self.start_build(image), "")
        Image.objects.filter(pk=image.pk).update(status="e")
        self.assertEqual(self.start_build(image), "")

    def test_number_like_suffix_is_rendered_as_a_string(self):
        # Unquoted, YAML would read these as an int (losing the zero) and a float
        for name, suffix in (("digits", "012345"), ("exponent", "12e456")):
            image = self.make_saved_image("profiles", name=name)
            Image.objects.filter(pk=image.pk).update(role_id_suffix=suffix)
            image.refresh_from_db()
            blueprint = yaml.safe_load(self.render(image))
            profile = self.modules(blueprint, "builder-env")["builder_profile"]
            self.assertEqual(profile["settings"]["role_id_suffix"], suffix)

    def test_profiles_image_without_a_suffix_is_not_rendered(self):
        image = self.make_saved_image("profiles")
        backend = self.backend_for(image)
        # The error is raised again, so the build stops there
        with self.assertLogs(
            "ghpcfe.cluster_manager.image", level="ERROR"
        ), self.assertRaisesMessage(ValueError, "has no role_id_suffix"):
            backend._create_blueprint()  # pylint: disable=protected-access
        self.assertFalse((backend.image_dir / "image.yaml").exists())
        image.refresh_from_db()
        self.assertEqual(image.status, "e")
