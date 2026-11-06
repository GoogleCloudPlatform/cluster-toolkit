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
"""What deploy.sh --config does about the project's APIs, and the admin text.

An interactive deployment runs check_apis, which asks before enabling each
missing API. A --config deployment never called it, so on a fresh project it
went straight to Terraform and failed there on whichever API it reached first.
It now enables, without asking, the PRJ_API entries the project is missing
before it deploys. Enabling an API needs serviceusage.services.enable even when
the API is already on, so it lists the enabled APIs first and makes no enable
call when nothing is missing.

`service_account.sh delete` retries a role removal that fails, so a transient
failure does not leave a dangling `deleted:serviceAccount:` binding.
"""
import os
import subprocess
import tempfile
from pathlib import Path

from django.test import SimpleTestCase

from ..forms import FILESTORE_CAPACITY_RULES
from ..models import GCPFilestoreFilesystem

# ofe/website/ghpcfe/tests -> ofe
OFE_ROOT = Path(__file__).resolve().parents[3]


class DeployFromConfigEnablesApisTests(SimpleTestCase):

    def _source(self):
        return (OFE_ROOT / "deploy.sh").read_text()

    def _function(self, name):
        """Body of a shell function in deploy.sh."""
        src = self._source()
        start = src.index(f"{name}() {{")
        return src[start:src.index("\n}\n", start)]

    def test_it_enables_the_missing_apis_before_it_deploys(self):
        body = self._function("deploy_from_config")
        self.assertTrue(body.endswith("\n\tdeploy"))
        self.assertIn('\n\tenable_required_apis "${project_id}"\n', body)
        self.assertLess(body.index("enable_required_apis"),
                        body.rindex("\n\tdeploy"))
        # no unconditional enable of the whole list any more
        self.assertNotIn("gcloud services enable", body)

    def test_interactive_check_lists_the_apis_of_the_project(self):
        # without --project it listed gcloud's default project instead
        self.assertIn('gcloud services list --project="${project}"',
                      self._function("check_apis"))

    def test_the_kms_api_is_in_the_list_it_enables(self):
        self.assertIn("PRJ_API['cloudkms.googleapis.com']", self._source())

    def test_interactive_check_still_asks_for_each_api(self):
        body = self._function("check_apis")
        self.assertIn('read -r -p "         Enable this API? [y/N] " reply',
                      body)
        self.assertIn('gcloud services enable "${id}"', body)
        self.assertNotIn("${!PRJ_API[@]}\"\n", body.split("for id in")[1])


FAKE_GCLOUD = """#!/bin/bash
echo "$*" >>"${GCLOUD_LOG}"
case "$1 $2" in
"services list")
	if [[ ${FAKE_LIST_FAILS} == 1 ]]; then
		exit 1
	fi
	for api in ${FAKE_ENABLED}; do
		echo "${api}"
	done
	;;
"services enable")
	if [[ ${FAKE_ENABLE_FAILS} == 1 ]]; then
		echo "PERMISSION_DENIED" >&2
		exit 1
	fi
	;;
*)
	exit 2
	;;
esac
"""


class EnableRequiredApisTests(SimpleTestCase):
    """Run enable_required_apis from deploy.sh against a fake gcloud."""

    def setUp(self):
        src = (OFE_ROOT / "deploy.sh").read_text()
        start = src.index("declare -A PRJ_API")
        parts = [src[start:src.index("\n\n", start)]]
        for name in ("error", "enable_required_apis"):
            start = src.find(f"\n{name}() {{")
            # a missing function is left out, so calling it fails the test
            if start != -1:
                parts.append(src[start:src.index("\n}\n", start) + 3])
        # PRJ_API lists Pub/Sub twice; as array keys that is one entry
        self.required = sorted({
            line.split("'")[1] for line in parts[0].splitlines()
            if line.startswith("PRJ_API['")})
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        (self.tmp / "functions.sh").write_text("\n".join(parts) + "\n")
        gcloud = self.tmp / "bin" / "gcloud"
        gcloud.parent.mkdir()
        gcloud.write_text(FAKE_GCLOUD)
        gcloud.chmod(0o755)
        self.log = self.tmp / "gcloud.log"

    def _run(self, enabled=(), list_fails=False, enable_fails=False):
        env = dict(os.environ,
                   PATH=f"{self.tmp / 'bin'}:{os.environ['PATH']}",
                   GCLOUD_LOG=str(self.log),
                   FAKE_ENABLED=" ".join(enabled),
                   FAKE_LIST_FAILS="1" if list_fails else "0",
                   FAKE_ENABLE_FAILS="1" if enable_fails else "0")
        try:
            result = subprocess.run(
                ["bash", "-c", f"source {self.tmp / 'functions.sh'}; "
                 "enable_required_apis my-proj"],
                env=env, capture_output=True, text=True, check=False)
        except PermissionError as err:
            self.skipTest(f"cannot start bash in this environment: {err}")
        calls = (self.log.read_text().splitlines()
                 if self.log.exists() else [])
        return result, calls

    def _enabled_apis(self, call):
        args = call.split()
        self.assertEqual(args[:2], ["services", "enable"])
        self.assertEqual(args[-1], "--project=my-proj")
        return sorted(args[2:-1])

    def _announced_apis(self, stdout):
        """The APIs the 'Enabling required APIs:' line names, or None."""
        prefix = "Enabling required APIs: "
        lines = [line for line in stdout.splitlines()
                 if line.startswith(prefix)]
        if not lines:
            return None
        self.assertEqual(len(lines), 1, stdout)
        return sorted(lines[0][len(prefix):].split())

    def test_all_enabled_makes_no_enable_call(self):
        result, calls = self._run(enabled=self.required)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [
            "services list --enabled --project=my-proj "
            "--format=value(config.name)"
        ])
        self.assertIn(
            "All required APIs are already enabled on project my-proj.",
            result.stdout)
        self.assertIsNone(self._announced_apis(result.stdout))

    def test_only_the_missing_api_is_enabled(self):
        enabled = [a for a in self.required if a != "cloudkms.googleapis.com"]
        result, calls = self._run(enabled=enabled)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[1],
                         "services enable cloudkms.googleapis.com "
                         "--project=my-proj")
        self.assertEqual(self._announced_apis(result.stdout),
                         ["cloudkms.googleapis.com"])

    def test_it_names_every_missing_api_before_enabling(self):
        missing = ["cloudkms.googleapis.com", "file.googleapis.com"]
        enabled = [a for a in self.required if a not in missing]
        result, calls = self._run(enabled=enabled)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self._enabled_apis(calls[1]), missing)
        self.assertEqual(self._announced_apis(result.stdout), missing)

    def test_a_refused_enable_names_the_api_and_the_role_and_stops(self):
        enabled = [a for a in self.required if a != "cloudkms.googleapis.com"]
        result, calls = self._run(enabled=enabled, enable_fails=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(calls), 2)
        self.assertIn("my-proj", result.stderr)
        self.assertIn("cloudkms.googleapis.com", result.stderr)
        self.assertIn("serviceusage.services.enable", result.stderr)
        self.assertIn("roles/serviceusage.serviceUsageAdmin", result.stderr)

    def test_a_failed_list_falls_back_to_enabling_every_api(self):
        result, calls = self._run(list_fails=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 2)
        self.assertTrue(calls[0].startswith("services list "))
        self.assertEqual(len(self.required), 17)
        self.assertEqual(self._enabled_apis(calls[1]), self.required)
        self.assertEqual(self._announced_apis(result.stdout), self.required)

    def test_a_failed_list_and_a_refused_enable_stops(self):
        result, calls = self._run(list_fails=True, enable_fails=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self._enabled_apis(calls[1]), self.required)
        self.assertIn("serviceusage.services.enable", result.stderr)


FAKE_SA_GCLOUD = """#!/bin/bash
echo "$*" >>"${GCLOUD_LOG}"
case "$1 $2" in
"projects get-iam-policy")
	echo "roles/compute.admin"
	echo "roles/pubsub.admin"
	;;
"projects remove-iam-policy-binding")
	role=${*: -1}
	role=${role#--role=}
	if [[ ${role} == "${FAKE_FAILING_ROLE}" ]]; then
		count=$(grep -c -- "--role=${role}$" "${GCLOUD_LOG}")
		if [[ ${count} -le ${FAKE_FAILURES} ]]; then
			exit 1
		fi
	fi
	;;
"iam service-accounts")
	;;
*)
	exit 2
	;;
esac
"""


class DeleteServiceAccountTests(SimpleTestCase):
    """Run service_account.sh delete against a fake gcloud."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        bin_dir = self.tmp / "bin"
        bin_dir.mkdir()
        for name, text in (("gcloud", FAKE_SA_GCLOUD),
                           ("sleep", '#!/bin/bash\necho "sleep $*" >>'
                                     '"${GCLOUD_LOG}"\n')):
            (bin_dir / name).write_text(text)
            (bin_dir / name).chmod(0o755)
        self.log = self.tmp / "gcloud.log"

    def _run(self, failing_role="", failures=0):
        env = dict(os.environ,
                   PATH=f"{self.tmp / 'bin'}:{os.environ['PATH']}",
                   GCLOUD_LOG=str(self.log),
                   FAKE_FAILING_ROLE=failing_role,
                   FAKE_FAILURES=str(failures))
        try:
            result = subprocess.run(
                ["bash", str(OFE_ROOT / "script/service_account.sh"),
                 "delete", "my-proj", "ofe-sa"],
                env=env, capture_output=True, text=True, check=False)
        except PermissionError as err:
            self.skipTest(f"cannot start bash in this environment: {err}")
        calls = (self.log.read_text().splitlines()
                 if self.log.exists() else [])
        return result, calls

    def _removals(self, calls, role):
        return [c for c in calls if c.startswith(
            "projects remove-iam-policy-binding ")
            and c.endswith(f"--role={role}")]

    def _deleted(self, calls):
        return ("iam service-accounts delete "
                "ofe-sa@my-proj.iam.gserviceaccount.com --quiet "
                "--project=my-proj") in calls

    def test_every_role_is_removed_and_the_account_deleted(self):
        result, calls = self._run()
        self.assertEqual(result.returncode, 0, result.stderr)
        for role in ("roles/compute.admin", "roles/pubsub.admin"):
            self.assertEqual(len(self._removals(calls, role)), 1)
        self.assertTrue(self._deleted(calls))
        self.assertNotIn("sleep 3", calls)
        self.assertNotIn("Warning", result.stderr)

    def test_a_removal_that_fails_once_is_retried(self):
        result, calls = self._run("roles/pubsub.admin", failures=1)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            len(self._removals(calls, "roles/pubsub.admin")), 2)
        self.assertEqual(
            len(self._removals(calls, "roles/compute.admin")), 1)
        self.assertEqual(calls.count("sleep 3"), 1)
        self.assertNotIn("Warning", result.stderr)
        self.assertTrue(self._deleted(calls))

    def test_a_removal_that_keeps_failing_warns_and_deletes_the_account(self):
        result, calls = self._run("roles/pubsub.admin", failures=99)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            len(self._removals(calls, "roles/pubsub.admin")), 3)
        self.assertEqual(calls.count("sleep 3"), 2)
        self.assertIn("Warning: Failed to remove IAM policy: "
                      "roles/pubsub.admin", result.stderr)
        self.assertEqual(result.stderr.count("Warning"), 1)
        self.assertTrue(self._deleted(calls))


class AdminGuideTests(SimpleTestCase):

    def _guide(self):
        return (OFE_ROOT / "docs/admin_guide.md").read_text()

    def test_the_api_list_includes_cloud_kms(self):
        guide = self._guide()
        start = guide.index("with the following APIs enabled")
        api_list = guide[start:guide.index("```", guide.index("```", start) + 3)]
        self.assertIn("Cloud Key Management Service (KMS) API", api_list)

    def test_it_says_config_deployment_enables_only_missing_apis(self):
        guide = self._guide()
        self.assertNotIn("A deployment with `--config` enables them\n"
                         "without asking", guide)
        self.assertIn("A deployment with `--config` lists the\nenabled APIs "
                      "and enables only those that are missing", guide)
        self.assertIn("no change when all of them are already enabled", guide)
        self.assertIn("`serviceusage.services.list`", guide)
        self.assertIn("`serviceusage.services.enable`", guide)

    def test_the_required_roles_include_service_usage(self):
        guide = self._guide()
        start = guide.index("The\nrequired roles are:")
        roles = guide[start:guide.index("```", guide.index("```", start) + 3)]
        self.assertIn(" Service Usage Viewer\n", roles)
        self.assertIn(" Service Usage Admin (only while an API still has to "
                      "be enabled)\n", roles)

    def _upgrade_section(self):
        guide = self._guide()
        start = guide.index("## Upgrading an existing TKFE")
        return guide[start:guide.index("## Teardown Process")]

    def test_there_is_an_upgrade_section_before_teardown(self):
        section = self._upgrade_section()
        for step in ("pip install --require-hashes -r requirements.txt",
                     "python manage.py makemigrations ghpcfe",
                     "python manage.py migrate",
                     "script/service_account.sh update",
                     "Back up the database"):
            self.assertIn(step, section)
        self.assertNotIn("refresh-server", section)

    def test_upgrade_enables_kms_and_checks_terraform_first(self):
        section = self._upgrade_section()
        kms = section.index(
            "`gcloud services enable cloudkms.googleapis.com "
            "--project=<PROJECT_ID>`")
        version = section.index("`terraform version`")
        update = section.index("Update the TKFE code on the server")
        self.assertLess(kms, update)
        self.assertLess(version, update)
        self.assertIn("separate project that holds encryption keys", section)
        self.assertIn("1.12.2 or later", section)
        self.assertIn("`sudo dnf install -y terraform-1.12.2-1`", section)
        self.assertIn("`sudo dnf upgrade -y terraform`", section)
        self.assertNotIn("refresh-server", section)

    def test_upgrade_restarts_django_through_supervisord(self):
        # startup.sh runs Django as the supervisord program
        # gcluster-uvicorn-background; the gcluster systemd unit is nginx.
        section = self._upgrade_section()
        self.assertIn(
            "`sudo supervisorctl restart 'gcluster-uvicorn-background:*'`",
            section)
        self.assertNotIn("Restart the service with `sudo systemctl restart "
                         "gcluster`", section)

    def test_upgrade_steps_name_the_user_and_directories(self):
        section = self._upgrade_section()
        self.assertIn("`sudo su - gcluster`", section)
        self.assertIn(
            "`cd /opt/gcluster/cluster-toolkit/community/front-end/ofe`",
            section)
        self.assertIn("`cd website`", section)
        # requirements.txt is installed from the TKFE directory, before
        # changing into website/ for the manage.py commands.
        self.assertLess(
            section.index(
                "`cd /opt/gcluster/cluster-toolkit/community/front-end/ofe`"),
            section.index("-r requirements.txt"))
        self.assertLess(section.index("-r requirements.txt"),
                        section.index("`cd website`"))
        self.assertLess(section.index("`cd website`"),
                        section.index("python manage.py makemigrations"))

    def test_upgrade_mentions_django_requirements_when_it_changed(self):
        section = self._upgrade_section()
        self.assertIn(
            "pip install --require-hashes -r "
            "infrastructure_files/gcs_bucket/webserver/django_requirements.txt",
            section)
        self.assertTrue((OFE_ROOT / "infrastructure_files/gcs_bucket/webserver/"
                         "django_requirements.txt").is_file())

    def test_upgrade_explains_the_key_ring_rename_questions(self):
        section = " ".join(self._upgrade_section().split())
        for text in ("`cmek_keychain` -> `cmek_key_ring_name`",
                     "`cmek_key_project` -> `cmek_key_project_id`",
                     "asks six questions",
                     "Answer `y` to all six",
                     "Never use `--noinput`, which silently drops the stored "
                     "values",
                     "closed standard input",
                     "A fresh install, or an upgrade from a release that "
                     "never had these columns, is unaffected"):
            self.assertIn(text, section)
        self.assertLess(section.index("python manage.py makemigrations"),
                        section.index("asks six questions"))
        self.assertLess(section.index("asks six questions"),
                        section.index("Leave the `gcluster` shell"))


class AdvancedAdminServiceUsageTests(SimpleTestCase):

    def _doc(self):
        return (OFE_ROOT / "docs/AdvancedAdmin.md").read_text()

    def test_deployer_permissions_include_service_usage(self):
        doc = self._doc()
        start = doc.index("The complete list of required permissions")
        perms = doc[start:doc.index("```", doc.index("```", start) + 3)]
        self.assertIn(" serviceusage.services.list\n", perms)
        self.assertIn(" serviceusage.services.enable (only needed when an API "
                      "is missing)\n", perms)

    def test_portal_project_apis_include_cloud_kms(self):
        doc = self._doc()
        start = doc.index("A project that administers GCP resources via the")
        apis = doc[start:doc.index("```", doc.index("```", start) + 3)]
        self.assertIn("Cloud Key Management Service (KMS) API", apis)

    def test_revoking_a_key_is_described_as_permanent(self):
        doc = " ".join(self._doc().split())
        for text in ("Revoking a key is permanent in TKFE",
                     "a resource whose key was revoked can only be destroyed",
                     "Destroying a revoked cluster uses a teardown that skips "
                     "the Terraform refresh",
                     "an administrator must first remove the cluster's "
                     "encrypted objects from its Slurm bucket"):
            self.assertIn(text, doc)


class ServerVmTerraformTests(SimpleTestCase):

    def test_server_vm_has_no_compute_access(self):
        try:
            main_tf = (OFE_ROOT / "tf/main.tf").read_text()
        except PermissionError as err:
            self.skipTest(f"cannot read tf/main.tf in this environment: {err}")
        for unused in ("compute.viewer", "compute.readonly",
                       "allow_stopping_for_update"):
            self.assertNotIn(unused, main_tf)


class KmsRoleWordingTests(SimpleTestCase):
    """cloudkms.admin cannot decrypt, but it is not a barrier to decryption."""

    def test_advanced_admin_does_not_promise_tkfe_can_never_decrypt(self):
        doc = (OFE_ROOT / "docs/AdvancedAdmin.md").read_text()
        self.assertNotIn("never able to decrypt", doc)
        self.assertIn("This is not a hard barrier", doc)
        self.assertIn("on that key project", doc)

    def test_service_account_script_comment_says_the_same(self):
        src = (OFE_ROOT / "script/service_account.sh").read_text()
        self.assertNotIn("must not be\n\t# able to decrypt", src)
        self.assertIn("it is not a hard barrier", src)


class FilestoreCapacityHelpTextTests(SimpleTestCase):
    """The capacity help text must quote the limits the form enforces."""

    def test_help_text_matches_the_form_rules(self):
        help_text = GCPFilestoreFilesystem._meta.get_field(
            "capacity").help_text
        tiers = dict(GCPFilestoreFilesystem.FILESTORE_TIER)
        for code, rules in FILESTORE_CAPACITY_RULES.items():
            expected = f"{tiers[code]} min {rules['min']} GiB"
            if rules["step"]:
                expected += f" in {rules['step']} GiB steps"
            self.assertIn(expected, help_text)
