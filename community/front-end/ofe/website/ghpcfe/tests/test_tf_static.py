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

"""Static checks of the web server's Terraform (tf/), read as HCL text.

These run without Terraform: they read the .tf files and check the provider
constraints, module pins, the web server identity, and the moved and removed
blocks that let an existing deployment upgrade in place.
"""

import re
from pathlib import Path

from django.test import SimpleTestCase

OFE_DIR = Path(__file__).resolve().parents[3]
REPO_ROOT = OFE_DIR.parents[2]
TF_DIR = OFE_DIR / "tf"
PROFILES_MODULE = REPO_ROOT / "community/modules/iam/service-account-profiles"

GOOGLE_CONSTRAINT = ">= 7.33.0, < 8.0.0"


HEREDOC = re.compile(r"<<-?([A-Za-z_][A-Za-z0-9_]*)[ \t]*\n")


def _scan(text):
    """Yield (index, kind) for each character of HCL text.

    kind is "code", "string" (inside a quoted string or heredoc, outside any
    ${...} interpolation) or "comment". Interpolations are code again, and may
    hold quoted strings of their own.
    """
    stack = ["code"]  # "code", "string", or "interp" (a ${ } in a string)
    depth = [0]  # brace depth per interp level
    i = 0
    while i < len(text):
        state = stack[-1]
        if state == "string":
            if text[i] == "\\":
                yield i, "string"
                yield i + 1, "string"
                i += 2
                continue
            if text.startswith("${", i):
                yield i, "string"
                yield i + 1, "string"
                stack.append("interp")
                depth.append(0)
                i += 2
                continue
            if text[i] == '"':
                stack.pop()
            yield i, "string"
            i += 1
            continue
        # code or interp
        if text[i] == "#" or text.startswith("//", i):
            end = text.find("\n", i)
            end = len(text) if end == -1 else end
            for j in range(i, end):
                yield j, "comment"
            i = end
            continue
        if text.startswith("/*", i):
            end = text.find("*/", i)
            end = len(text) if end == -1 else end + 2
            for j in range(i, end):
                yield j, "comment"
            i = end
            continue
        heredoc = HEREDOC.match(text, i) if state == "code" else None
        if heredoc:
            closing = re.compile(r"^[ \t]*" + heredoc.group(1) + r"[ \t]*$", re.M)
            end_match = closing.search(text, heredoc.end())
            end = len(text) if end_match is None else end_match.end()
            for j in range(i, end):
                yield j, "string"
            i = end
            continue
        if text[i] == '"':
            stack.append("string")
            yield i, "string"
            i += 1
            continue
        if state == "interp":
            if text[i] == "{":
                depth[-1] += 1
            elif text[i] == "}":
                if depth[-1] == 0:
                    stack.pop()
                    depth.pop()
                    yield i, "string"
                    i += 1
                    continue
                depth[-1] -= 1
        yield i, "code"
        i += 1


def _strip_comments(text):
    """Remove # and // line comments and /* */ blocks outside strings."""
    return "".join(text[i] for i, kind in _scan(text) if kind != "comment")


def _block_end(text, start):
    """Index just past the brace that closes the brace at text[start]."""
    depth = 0
    for i, kind in _scan(text[start:]):
        if kind != "code":
            continue
        if text[start + i] == "{":
            depth += 1
        elif text[start + i] == "}":
            depth -= 1
            if depth == 0:
                return start + i + 1
    raise ValueError("unbalanced braces")


def blocks(text, header):
    """Bodies of every top-level block whose header matches the regex."""
    found = []
    for match in re.finditer(r"^" + header + r"\s*\{", text, re.M):
        brace = match.end() - 1
        found.append(text[brace + 1:_block_end(text, brace) - 1])
    return found


def read_tf(path):
    return _strip_comments(path.read_text())


def string_list(body, name):
    """The quoted strings of a `name = [ ... ]` attribute in a block body."""
    match = re.search(r"^\s*" + name + r"\s*=\s*\[(.*?)\]", body, re.M | re.S)
    if not match:
        return None
    return re.findall(r'"([^"]*)"', match.group(1))


def attribute(body, name):
    match = re.search(r"^\s*" + name + r"\s*=\s*(.+)$", body, re.M)
    return match.group(1).strip() if match else None


def reference_list(body, name):
    """The unquoted references of a `name = [ ... ]` attribute in a block body."""
    match = re.search(r"^\s*" + name + r"\s*=\s*\[(.*?)\]", body, re.M | re.S)
    if not match:
        return None
    return [item.strip() for item in match.group(1).split(",") if item.strip()]


def map_attribute(body, name):
    """The text inside the braces of a `name = { ... }` attribute."""
    match = re.search(r"^\s*" + name + r"\s*=\s*\{", body, re.M)
    if not match:
        return None
    brace = match.end() - 1
    return body[brace + 1:_block_end(body, brace) - 1]


def secret_bindings(profile):
    """{key: (secret_id reference, role)} of a profile's secret_bindings."""
    bindings = map_attribute(profile, "secret_bindings")
    if bindings is None:
        return {}
    found = {}
    for match in re.finditer(r"^\s*(\w+)\s*=\s*\{", bindings, re.M):
        brace = match.end() - 1
        body = bindings[brace + 1:_block_end(bindings, brace) - 1]
        found[match.group(1)] = (attribute(body, "secret_id"), attribute(body, "role").strip('"'))
    return found


def tf_files():
    return [path for path in TF_DIR.glob("**/*.tf") if ".terraform" not in path.parts]


TOKEN_CREATOR = (
    r'resource\s+"google_service_account_iam_member"\s+"fe_impersonates_provisioning"'
)


class ProviderConstraintTests(SimpleTestCase):
    """tf/ and tf/network/ require the provider versions the modules need"""

    def _providers(self, path):
        text = read_tf(path)
        found = {}
        for name, body in re.findall(
            r"^\s*([\w-]+)\s*=\s*\{(.*?)\}", text, re.M | re.S
        ):
            version = attribute(body, "version")
            if version:
                found[name] = version.strip('"')
        return found

    def test_root_providers(self):
        providers = self._providers(TF_DIR / "versions.tf")
        self.assertEqual(providers["google"], GOOGLE_CONSTRAINT)
        self.assertEqual(providers["google-beta"], GOOGLE_CONSTRAINT)
        self.assertIn("random", providers)
        self.assertIn("null", providers)

    def test_identity_modules_require_time(self):
        # hashicorp/time comes in through the two identity modules; tf/ itself
        # uses none of its resources, so it does not declare it.
        for module in (REPO_ROOT / "modules/project/service-account", PROFILES_MODULE):
            providers = self._providers(module / "versions.tf")
            self.assertEqual(providers["time"], ">= 0.9", module)

    def test_network_providers(self):
        providers = self._providers(TF_DIR / "network" / "versions.tf")
        self.assertEqual(providers["google"], GOOGLE_CONSTRAINT)


class ModulePinTests(SimpleTestCase):
    """Every module is pinned: registry modules by version, others by path"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.modules = dict(
            (match.group(1), blocks(self.text, r'module\s+"' + match.group(1) + '"')[0])
            for match in re.finditer(r'^module\s+"([\w-]+)"', self.text, re.M)
        )

    def test_every_module_is_pinned(self):
        self.assertTrue(self.modules)
        for name, body in self.modules.items():
            source = attribute(body, "source").strip('"')
            if source.startswith((".", "/")):
                self.assertIsNone(attribute(body, "version"), name)
                self.assertTrue((TF_DIR / source).is_dir(), f"{name}: {source}")
            else:
                version = attribute(body, "version")
                self.assertIsNotNone(version, f"{name} has no version")
                self.assertRegex(version, r'^"~> \d+\.\d+"$', name)

    def test_registry_module_versions(self):
        self.assertEqual(attribute(self.modules["control_storage"], "version"), '"~> 11.1"')
        self.assertEqual(attribute(self.modules["pubsub"], "version"), '"~> 8.3"')

    def test_identity_modules_come_from_this_checkout(self):
        self.assertEqual(
            (TF_DIR / attribute(self.modules["fe_sa"], "source").strip('"')).resolve(),
            (REPO_ROOT / "modules/project/service-account").resolve(),
        )
        self.assertEqual(
            (TF_DIR / attribute(self.modules["fe_profile"], "source").strip('"')).resolve(),
            PROFILES_MODULE.resolve(),
        )

    def test_old_module_calls_are_gone(self):
        # The removed blocks are valid only while nothing declares these.
        self.assertNotIn("service_account", self.modules)
        self.assertNotIn("control_bucket", self.modules)


class WebIdentityTests(SimpleTestCase):
    """The web server runs as the custom profile with resource-scoped grants"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.fe_sa = blocks(self.text, r'module\s+"fe_sa"')[0]
        self.profile = blocks(self.text, r'module\s+"fe_profile"')[0]
        self.bucket = blocks(self.text, r'module\s+"control_storage"')[0]
        self.vm = blocks(self.text, r'resource\s+"google_compute_instance"\s+"server_vm"')[0]

    def _module_denylist(self):
        """The profiles module's own refusal rules, read from its main.tf."""
        module = read_tf(PROFILES_MODULE / "main.tf")
        locals_body = blocks(module, "locals")[0]
        return (
            string_list(locals_body, "denied_permissions"),
            string_list(locals_body, "denied_permission_prefixes"),
            string_list(locals_body, "denied_permission_suffixes"),
            string_list(locals_body, "allowed_despite_suffix"),
        )

    def test_account_keeps_only_logging_and_monitoring_project_roles(self):
        # monitoring.viewer: Grafana's keyless Cloud Monitoring datasource.
        self.assertEqual(attribute(self.fe_sa, "name"), '"fe-sa"')
        self.assertEqual(
            string_list(self.fe_sa, "project_roles"),
            ["logging.logWriter", "monitoring.metricWriter", "monitoring.viewer"],
        )
        self.assertEqual(attribute(self.profile, "grant_baseline_roles"), "false")

    def test_profile_is_custom(self):
        self.assertEqual(attribute(self.profile, "profile"), '"custom"')
        self.assertEqual(attribute(self.profile, "name"), attribute(self.fe_sa, "name"))
        self.assertEqual(
            attribute(self.profile, "service_account_email"),
            "module.fe_sa.service_account_email",
        )

    def test_custom_permissions_are_the_cited_set(self):
        self.assertEqual(
            string_list(self.profile, "custom_permissions"),
            [
                # gcluster create's machine-type lookup runs as this account
                "compute.machineTypes.get",
                "pubsub.subscriptions.create",
                "pubsub.subscriptions.delete",
                "pubsub.subscriptions.get",
                "pubsub.subscriptions.getIamPolicy",
                "pubsub.subscriptions.setIamPolicy",
            ],
        )

    def test_no_refused_permission(self):
        denied, prefixes, suffixes, allowed = self._module_denylist()
        # The module's lists were found, so this check is not vacuous.
        self.assertIn("iam.serviceAccounts.actAs", denied)
        self.assertIn("secretmanager.versions.access", denied)
        self.assertIn(".setIamPolicy", suffixes)
        for permission in string_list(self.profile, "custom_permissions"):
            self.assertNotIn(permission, denied)
            self.assertFalse(permission.startswith(tuple(prefixes)), permission)
            if permission.endswith(tuple(suffixes)):
                self.assertIn(permission, allowed)

    def test_no_refused_token_anywhere_in_tf(self):
        # Never actAs, secret payload access, IAM-admin or basic roles, in the
        # profile or anywhere else in tf/. The one exception is the
        # impersonation grant on the provisioning account, checked by
        # ProvisioningAccountTests and cut out here.
        for path in tf_files():
            text = read_tf(path)
            for body in blocks(text, TOKEN_CREATOR):
                text = text.replace(body, "")
            for token in (
                "iam.serviceAccounts.actAs",
                "secretmanager.versions.access",
                "roles/iam.",
                "roles/resourcemanager.",
                "roles/owner",
                "roles/editor",
                "roles/viewer",
            ):
                self.assertNotIn(token, text, f"{path.name}: {token}")

    def test_resource_bindings(self):
        roles = re.findall(r'role\s*=\s*"([^"]+)"', self.profile)
        self.assertEqual(
            sorted(roles),
            sorted([
                "roles/storage.admin",
                "roles/pubsub.admin",
                "roles/pubsub.subscriber",
                "roles/pubsub.subscriber",
                "roles/secretmanager.secretAccessor",
                "roles/secretmanager.secretVersionAdder",
                # the server configuration secret
                "roles/secretmanager.secretAccessor",
                "roles/secretmanager.secretVersionManager",
            ]),
        )
        self.assertIn("module.control_storage.name", self.profile)
        self.assertIn("module.pubsub.topic", self.profile)
        self.assertIn("google_pubsub_subscription.cloud_build_logs_sub.name", self.profile)
        for unused in (
            "artifact_registry_bindings",
            "subnetwork_bindings",
            "bigquery_dataset_bindings",
        ):
            self.assertNotIn(unused, self.profile)

    def test_bucket_module_grants_nothing(self):
        # The web server's bucket access comes only from the profile's
        # bucket_bindings, which wait for the account.
        for name in ("admins", "storage_admins", "set_admin_roles", "set_storage_admin_roles"):
            self.assertIsNone(attribute(self.bucket, name), name)

    def test_vm_runs_as_profile_with_cloud_platform_scope(self):
        account = blocks(self.vm, r"\s*service_account")[0]
        self.assertEqual(
            attribute(account, "email"), "module.fe_profile.service_account_email"
        )
        self.assertEqual(string_list(account, "scopes"), ["cloud-platform"])
        self.assertEqual(attribute(self.vm, "allow_stopping_for_update"), "true")

    def test_superseded_roles_are_not_granted(self):
        for role in (
            "storage.objectAdmin",
            "artifactregistry.admin",
            "secretmanager.admin",
            "cloudtrace.agent",
        ):
            self.assertNotIn(role, self.text)
        # pubsub.admin only on the command-and-control topic.
        self.assertEqual(self.text.count("pubsub.admin"), self.profile.count("pubsub.admin"))
        self.assertEqual(self.profile.count("pubsub.admin"), 1)


class UpgradeBlockTests(SimpleTestCase):
    """Existing deployments keep their account and bucket and lose no access"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")

    def _moves(self):
        return [
            (attribute(body, "from"), attribute(body, "to"))
            for body in blocks(self.text, "moved")
        ]

    def _removed(self):
        return {
            attribute(body, "from"): body for body in blocks(self.text, "removed")
        }

    def test_moved_blocks(self):
        self.assertCountEqual(
            self._moves(),
            [
                (
                    'module.service_account.google_service_account.service_accounts["fe-sa"]',
                    'module.fe_sa.module.service_account.google_service_account.service_accounts["fe-sa"]',
                ),
                (
                    'module.control_bucket.google_storage_bucket.buckets["storage"]',
                    'module.control_storage.google_storage_bucket.buckets["storage"]',
                ),
                (
                    "module.control_bucket.random_id.bucket_suffix[0]",
                    "module.control_storage.random_id.bucket_suffix[0]",
                ),
            ],
        )

    def test_removed_blocks_forget_without_destroying(self):
        removed = self._removed()
        self.assertCountEqual(
            removed,
            [
                "module.service_account.google_project_iam_member.project-roles",
                "module.control_bucket.google_storage_bucket_iam_binding.admins",
                "module.control_bucket.google_storage_bucket_iam_binding.storage_admins",
            ],
        )
        for address, body in removed.items():
            lifecycle = blocks(body, r"\s*lifecycle")
            self.assertEqual(len(lifecycle), 1, address)
            self.assertEqual(attribute(lifecycle[0], "destroy"), "false", address)


class ControlBucketLocationTests(SimpleTestCase):
    """The bucket stays where it is on upgrade; a new one goes in region"""

    def setUp(self):
        self.bucket = blocks(read_tf(TF_DIR / "main.tf"), r'module\s+"control_storage"')[0]
        self.variables = read_tf(TF_DIR / "variables.tf")

    def test_location_is_the_kept_location_when_set_else_region(self):
        self.assertEqual(
            attribute(self.bucket, "location"),
            'var.control_bucket_location != "" ? var.control_bucket_location : var.region',
        )

    def test_variable_defaults_to_empty(self):
        # A new deployment, and an old server's tfvars, have no such line.
        found = blocks(self.variables, r'variable\s+"control_bucket_location"')
        self.assertEqual(len(found), 1)
        self.assertEqual(attribute(found[0], "type"), "string")
        self.assertEqual(attribute(found[0], "default"), '""')


class ControlBucketAccessTests(SimpleTestCase):
    """The control bucket keeps uniform bucket-level access, which the
    controllers' conditional (prefix-limited) grants need"""

    def setUp(self):
        self.bucket = blocks(read_tf(TF_DIR / "main.tf"), r'module\s+"control_storage"')[0]

    def test_uniform_access_is_pinned_for_the_bucket(self):
        # Keyed by the bucket's entry in names: the module looks it up by
        # that key, and falls back to its default for any other
        (name,) = string_list(self.bucket, "names")
        entries = re.findall(
            r"^\s*(\w+)\s*=\s*(\S+)\s*$",
            map_attribute(self.bucket, "bucket_policy_only") or "",
            re.M,
        )
        self.assertEqual(entries, [(name, "true")])

    def test_public_access_prevention_is_enforced(self):
        # The module's default, "inherited", would let a grant to allUsers
        # or allAuthenticatedUsers make the server's boot files public
        self.assertEqual(
            attribute(self.bucket, "public_access_prevention"), '"enforced"'
        )


class ProfilesModuleConditionLimitsTests(SimpleTestCase):
    """The module refuses a bucket_bindings condition IAM would refuse at
    apply: a title of 1 to 100 characters, a description of at most 256"""

    def setUp(self):
        variables = read_tf(PROFILES_MODULE / "variables.tf")
        (self.variable,) = blocks(variables, r'variable\s+"bucket_bindings"')

    def test_title_and_description_limits(self):
        (validation,) = blocks(self.variable, r"\s*validation")
        condition = re.search(
            r"condition\s*=\s*(.*?)\n\s*error_message", validation, re.S
        ).group(1)
        self.assertIn("length(binding.condition.title) <= 100", condition)
        self.assertRegex(
            condition,
            r"binding\.condition\.description == null \? 0 : "
            r"length\(binding\.condition\.description\)\) <= 256",
        )
        self.assertIn("description of at most 256 characters", validation)

    def test_propagation_wait_is_keyed_on_the_fingerprint(self):
        # jsonencode(local.grants) would show an unset condition as null, so
        # upgrading would replace the wait in every existing deployment and
        # make service_account_email unknown; test_profiles_module_condition
        # checks the fingerprint's value
        module = read_tf(PROFILES_MODULE / "main.tf")
        (wait,) = blocks(module, r'resource\s+"time_sleep"\s+"wait_for_iam"')
        triggers = map_attribute(wait, "triggers")
        self.assertIsNotNone(triggers)
        self.assertEqual(attribute(triggers, "grants"), "local.grants_fingerprint")


class ProvisioningAccountTests(SimpleTestCase):
    """Only the web account may impersonate the provisioning account"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.variables = read_tf(TF_DIR / "variables.tf")
        found = blocks(self.text, TOKEN_CREATOR)
        self.assertEqual(len(found), 1)
        self.grant = found[0]

    def test_variable_defaults_to_empty(self):
        # An old server's tfvars have no such line and must still apply and
        # destroy.
        body = blocks(self.variables, r'variable\s+"provisioning_service_account"')[0]
        self.assertEqual(attribute(body, "type"), "string")
        self.assertEqual(attribute(body, "default"), '""')

    def test_grant_is_token_creator_for_the_web_account_only(self):
        self.assertEqual(attribute(self.grant, "role"), '"roles/iam.serviceAccountTokenCreator"')
        self.assertEqual(
            attribute(self.grant, "member"),
            '"serviceAccount:${module.fe_sa.service_account_email}"',
        )

    def test_grant_is_on_the_provisioning_account_only(self):
        self.assertEqual(
            attribute(self.grant, "service_account_id"),
            '"projects/${var.project_id}/serviceAccounts/${var.provisioning_service_account}"',
        )
        self.assertNotIn("for_each", self.grant)

    def test_grant_only_when_the_variable_is_set(self):
        self.assertEqual(
            attribute(self.grant, "count"),
            'var.provisioning_service_account != "" ? 1 : 0',
        )

    def test_no_other_impersonation_grant(self):
        for path in tf_files():
            text = read_tf(path)
            expected = 1 if path == TF_DIR / "main.tf" else 0
            self.assertEqual(text.count("roles/iam."), expected, path.name)
            self.assertEqual(
                len(re.findall(r'^resource\s+"google_service_account_iam_', text, re.M)),
                expected,
                path.name,
            )

    def test_no_iam_conditions(self):
        # The provisioning account's access is not conditional: IAM conditions
        # cannot limit it to OFE's own resources.
        self.assertNotRegex(self.grant, r"condition")
        for path in tf_files():
            text = read_tf(path)
            # A bucket lifecycle rule's condition (the state bucket's) is not
            # an IAM condition
            for match in reversed(
                list(re.finditer(r"^\s*lifecycle_rule\s*\{", text, re.M))
            ):
                text = text[:match.start()] + text[_block_end(text, match.end() - 1):]
            self.assertNotRegex(text, r"(?m)^\s*condition\s*\{", path.name)


class DatabaseKeySecretTests(SimpleTestCase):
    """The key's secret exists, with no version, and only the web account uses it"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.profile = blocks(self.text, r'module\s+"fe_profile"')[0]

    def test_secret_name(self):
        secret = blocks(self.text, r'resource\s+"google_secret_manager_secret"\s+"db_key"')
        self.assertEqual(len(secret), 1)
        self.assertEqual(attribute(secret[0], "secret_id"), '"${var.deployment_name}-db-key"')

    def test_no_version_in_terraform(self):
        # The key is generated on the server, so it is never in state. The
        # only version Terraform creates is the server configuration's.
        for path in tf_files():
            text = read_tf(path)
            versions = re.findall(
                r'^resource\s+"google_secret_manager_secret_version"\s+"([^"]+)"', text, re.M
            )
            self.assertEqual(
                versions, ["server_config"] if path == TF_DIR / "main.tf" else [], path.name
            )
            for body in blocks(text, r'resource\s+"google_secret_manager_secret_version"\s+"[^"]+"'):
                self.assertNotIn("db_key", body)

    def test_web_account_reads_and_adds_versions_on_this_secret_only(self):
        bindings = secret_bindings(self.profile)
        db_key = "google_secret_manager_secret.db_key.secret_id"
        self.assertEqual(
            sorted(role for secret, role in bindings.values() if secret == db_key),
            ["roles/secretmanager.secretAccessor", "roles/secretmanager.secretVersionAdder"],
        )
        self.assertNotIn("project", map_attribute(self.profile, "secret_bindings"))


class ServerVmTests(SimpleTestCase):
    """What the server reads at boot is in place before it boots"""

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.vm = blocks(self.text, r'resource\s+"google_compute_instance"\s+"server_vm"')[0]

    def test_metadata_keys(self):
        metadata = map_attribute(self.vm, "metadata")
        self.assertEqual(
            attribute(metadata, "ofe-provisioning-service-account"),
            "var.provisioning_service_account",
        )
        self.assertEqual(
            attribute(metadata, "ofe-db-key-secret"),
            "google_secret_manager_secret.db_key.secret_id",
        )
        self.assertEqual(
            attribute(metadata, "ofe-server-config-secret"),
            "google_secret_manager_secret.server_config.secret_id",
        )
        self.assertIn("webserver/startup.sh", attribute(metadata, "startup-script-url"))

    def test_boots_after_its_configuration_and_impersonation_grant(self):
        depends_on = reference_list(self.vm, "depends_on")
        self.assertIsNotNone(depends_on)
        for reference in (
            # the configuration startup.sh reads from Secret Manager
            "google_secret_manager_secret_version.server_config",
            "google_service_account_iam_member.fe_impersonates_provisioning",
        ):
            self.assertIn(reference, depends_on)
        self.assertNotIn("google_storage_bucket_object.config_file", depends_on)

    def test_boots_after_its_grants_have_propagated(self):
        # The profile's email output waits for its grants (the configuration
        # secret's among them) and their propagation.
        account = blocks(self.vm, r"\s*service_account")[0]
        self.assertEqual(attribute(account, "email"), "module.fe_profile.service_account_email")
        outputs = read_tf(PROFILES_MODULE / "outputs.tf")
        email = blocks(outputs, r'output\s+"service_account_email"')[0]
        self.assertEqual(reference_list(email, "depends_on"), ["time_sleep.wait_for_iam"])
        module = read_tf(PROFILES_MODULE / "main.tf")
        wait = blocks(module, r'resource\s+"time_sleep"\s+"wait_for_iam"')[0]
        self.assertIn(
            "google_secret_manager_secret_iam_member.this", reference_list(wait, "depends_on")
        )

    def test_ordering_does_not_replace_the_vm(self):
        lifecycle = blocks(self.vm, r"\s*lifecycle")
        for body in lifecycle:
            self.assertNotIn("replace_triggered_by", body)
        self.assertNotIn("triggers", self.vm)


class ServerConfigSecretTests(SimpleTestCase):
    """The server's configuration is a secret only the web account can read"""

    REFERENCE = "google_secret_manager_secret.server_config.secret_id"

    def setUp(self):
        self.text = read_tf(TF_DIR / "main.tf")
        self.profile = blocks(self.text, r'module\s+"fe_profile"')[0]
        self.vm = blocks(self.text, r'resource\s+"google_compute_instance"\s+"server_vm"')[0]

    def test_not_a_bucket_object(self):
        # Cluster accounts can read the control bucket, so the configuration
        # (which holds the admin password) is never uploaded there. With the
        # resource gone, an upgrade apply deletes an earlier version's copy.
        for path in tf_files():
            text = read_tf(path)
            self.assertNotIn("webserver/config", text, path.name)
            self.assertNotRegex(
                text, r'resource\s+"google_storage_bucket_object"\s+"config_file"', path.name
            )
            for body in blocks(text, r'resource\s+"google_storage_bucket_object"\s+"[^"]+"'):
                self.assertNotIn("server_config_file", body, path.name)

    def test_secret_name(self):
        secret = blocks(self.text, r'resource\s+"google_secret_manager_secret"\s+"server_config"')
        self.assertEqual(len(secret), 1)
        self.assertEqual(
            attribute(secret[0], "secret_id"), '"${var.deployment_name}-server-config"'
        )
        self.assertEqual(attribute(secret[0], "project"), "var.project_id")

    def test_one_version_holding_the_configuration(self):
        versions = blocks(
            self.text, r'resource\s+"google_secret_manager_secret_version"\s+"server_config"'
        )
        self.assertEqual(len(versions), 1)
        version = versions[0]
        self.assertEqual(
            attribute(version, "secret"), "google_secret_manager_secret.server_config.id"
        )
        self.assertEqual(attribute(version, "secret_data"), "local.server_config_file")
        self.assertNotIn("count", version)
        self.assertNotIn("for_each", version)
        # The version is the configuration's only use.
        self.assertEqual(self.text.count("local.server_config_file"), 1)

    def test_only_the_web_account_reads_and_disables_it(self):
        # Secret Version Manager lets the startup script disable the version
        # it read; it grants no payload access.
        self.assertEqual(
            sorted(
                role
                for secret, role in secret_bindings(self.profile).values()
                if secret == self.REFERENCE
            ),
            ["roles/secretmanager.secretAccessor", "roles/secretmanager.secretVersionManager"],
        )
        # The id is used only by those grants and the server's metadata, and
        # nothing else in tf/ grants access to a secret.
        self.assertEqual(self.text.count(self.REFERENCE), 3)
        self.assertEqual(self.profile.count(self.REFERENCE), 2)
        self.assertEqual(self.vm.count(self.REFERENCE), 1)
        for path in tf_files():
            self.assertNotRegex(
                read_tf(path), r'resource\s+"google_secret_manager_secret_iam_', path.name
            )

    def test_version_manager_grant_is_on_this_secret_only(self):
        # Granted through the profile's secret_bindings: one
        # google_secret_manager_secret_iam_member per binding, on the named
        # secret, which the server's boot waits for.
        bindings = secret_bindings(self.profile)
        self.assertEqual(
            [
                secret
                for secret, role in bindings.values()
                if role == "roles/secretmanager.secretVersionManager"
            ],
            [self.REFERENCE],
        )
        self.assertNotIn("project", map_attribute(self.profile, "secret_bindings"))
        module = read_tf(PROFILES_MODULE / "main.tf")
        member = blocks(module, r'resource\s+"google_secret_manager_secret_iam_member"\s+"this"')
        self.assertEqual(len(member), 1)
        self.assertEqual(attribute(member[0], "for_each"), "local.grants.bindings.secret")
        self.assertEqual(attribute(member[0], "secret_id"), "each.value.secret_id")

    def test_a_later_apply_does_not_enable_the_version_again(self):
        version = blocks(
            self.text, r'resource\s+"google_secret_manager_secret_version"\s+"server_config"'
        )[0]
        lifecycle = blocks(version, r"\s*lifecycle")
        self.assertEqual(len(lifecycle), 1)
        self.assertEqual(reference_list(lifecycle[0], "ignore_changes"), ["enabled"])
        # Terraform never sets the state itself.
        self.assertIsNone(attribute(version, "enabled"))

    def test_configuration_holds_no_deployment_key(self):
        # The first-boot settings only: no private key file, which neither
        # startup script used.
        config = re.search(r"server_config_file\s*=\s*<<-EOT\n(.*?)\nEOT", self.text, re.S)
        self.assertIsNotNone(config)
        self.assertEqual(
            re.findall(r"^(\w+):", config.group(1), re.M),
            [
                "django_username",
                "django_password",
                "django_email",
                "git_branch",
                "git_fork",
                "google_client_id",
                "google_client_secret",
            ],
        )
        for path in tf_files():
            text = read_tf(path)
            self.assertNotIn("deploy_key", text, path.name)
            self.assertNotIn("deployment_key", text, path.name)
