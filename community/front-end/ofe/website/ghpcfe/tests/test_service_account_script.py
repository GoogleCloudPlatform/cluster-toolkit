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

"""script/service_account.sh, run against a fake gcloud command.

The fake records every call and answers from environment variables the test
sets, so nothing reaches Google Cloud.
"""

import os
import shutil
import subprocess
import tempfile
from pathlib import Path

from django.test import SimpleTestCase

OFE_DIR = Path(__file__).resolve().parents[3]
SCRIPT = OFE_DIR / "script" / "service_account.sh"

PROJECT = "test-project"
ACCOUNT = "testdep-prov-sa"
EMAIL = f"{ACCOUNT}@{PROJECT}.iam.gserviceaccount.com"

# The reduced role list. Each role is cited in the script to the Terraform or
# API call that needs it; a change here must come with its citation there.
EXPECTED_ROLES = [
    "compute.admin",
    "storage.admin",
    "file.editor",
    "notebooks.admin",
    "servicenetworking.networksAdmin",
    "cloudsql.admin",
    "bigquery.connectionAdmin",
    "secretmanager.admin",
    "artifactregistry.admin",
    "cloudbuild.builds.editor",
    "pubsub.admin",
    "monitoring.viewer",
    "serviceusage.serviceUsageConsumer",
    "iam.serviceAccountAdmin",
    "iam.serviceAccountUser",
    "iam.roleAdmin",
    "resourcemanager.projectIamAdmin",
]

# Calls are logged one per line, arguments separated by a unit separator.
GCLOUD_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf '%s\n' "$*") >>"${FAKE_LOG}"
# FAKE_READ_CMD names one read ("list", "policy" or "keys") that fails,
# printing FAKE_READ_ERROR, on its first FAKE_READ_FAILS calls.
read_fails() {
	[[ ${FAKE_READ_CMD:-} == "$1" ]] || return 1
	local n
	n=$(($(cat "${FAKE_LOG}.reads" 2>/dev/null || echo 0) + 1))
	echo "${n}" >"${FAKE_LOG}.reads"
	[[ ${n} -le ${FAKE_READ_FAILS:-0} ]] || return 1
	echo "${FAKE_READ_ERROR:-}" >&2
}
case "$1 $2 $3" in
"iam service-accounts describe")
	# Fails with FAKE_DESCRIBE_RC (by default 1, the account does not exist),
	# printing FAKE_DESCRIBE_ERROR, by default gcloud's NOT_FOUND error.
	if [[ ${FAKE_DESCRIBE_RC:-1} != 0 ]]; then
		echo "${FAKE_DESCRIBE_ERROR:-ERROR: (gcloud.iam.service-accounts.describe) NOT_FOUND: Unknown service account}" >&2
	fi
	exit "${FAKE_DESCRIBE_RC:-1}"
	;;
"iam service-accounts create")
	exit "${FAKE_CREATE_RC:-0}"
	;;
"iam service-accounts delete")
	if [[ ${FAKE_DELETE_RC:-0} != 0 ]]; then
		echo "${FAKE_DELETE_ERROR:-}" >&2
	fi
	exit "${FAKE_DELETE_RC:-0}"
	;;
"iam service-accounts keys")
	# keys list: prints FAKE_USER_KEYS, one key name per line.
	if [[ $4 == list ]]; then
		if read_fails keys; then
			exit 1
		fi
		printf '%s' "${FAKE_USER_KEYS:-}"
		exit 0
	fi
	# keys create: fails with FAKE_KEY_ERROR when FAKE_KEY_RC is set.
	if [[ ${FAKE_KEY_RC:-0} != 0 ]]; then
		echo "${FAKE_KEY_ERROR:-}" >&2
		exit "${FAKE_KEY_RC}"
	fi
	echo "created key [0] of type [json] as [$5]" >&2
	exit 0
	;;
"projects get-iam-policy "*)
	# FAKE_POLICY_ROWS: "<role>\t<member>" lines, as the flattened value()
	# format prints the policy.
	if read_fails policy; then
		exit 1
	fi
	printf '%s' "${FAKE_POLICY_ROWS:-}"
	exit "${FAKE_POLICY_RC:-0}"
	;;
"projects add-iam-policy-binding "* | "projects remove-iam-policy-binding "*)
	# Policy edits, counted across calls. FAKE_IAM_ERRORS: a file with one
	# line per edit, in order: an empty line succeeds, any other is printed
	# as gcloud's error and fails. Edits past its end succeed, unless
	# FAKE_IAM_ERROR_ALWAYS is set: then they print it and fail.
	n=$(($(cat "${FAKE_LOG}.edits" 2>/dev/null || echo 0) + 1))
	echo "${n}" >"${FAKE_LOG}.edits"
	message=""
	if [[ -n ${FAKE_IAM_ERRORS:-} ]]; then
		message=$(sed -n "${n}p" "${FAKE_IAM_ERRORS}")
		if [[ ${n} -gt $(wc -l <"${FAKE_IAM_ERRORS}") ]]; then
			message=${FAKE_IAM_ERROR_ALWAYS:-}
		fi
	else
		message=${FAKE_IAM_ERROR_ALWAYS:-}
	fi
	if [[ -n ${message} ]]; then
		echo "ERROR: (gcloud.projects.$2) ${message}" >&2
		exit 1
	fi
	echo "Updated IAM policy for project [$3]." >&2
	echo "bindings: []"
	exit 0
	;;
"iam service-accounts list")
	if read_fails list; then
		exit 1
	fi
	# FAKE_ACCOUNTS: one "<display name>|<email>" per line. With the email
	# column only, print the emails; otherwise the default table, whose first
	# column is the display name.
	if [[ $* == *--format=value\(email\)* ]]; then
		cut -d'|' -f2 <<<"${FAKE_ACCOUNTS:-}"
	else
		echo "DISPLAY NAME  EMAIL  DISABLED"
		awk -F'|' '{ print $1 "  " $2 "  False" }' <<<"${FAKE_ACCOUNTS:-}"
	fi
	exit 0
	;;
esac
exit 0
"""


# sleep, recorded instead of waited for: one line per call with its seconds.
SLEEP_FAKE = r"""#!/bin/bash
echo "$*" >>"${FAKE_SLEEP_LOG}"
"""

# iam_policy_edit's waits before each retry: 8 attempts in all.
RETRY_DELAYS = ["2", "4", "8", "16", "30", "30", "30"]
CONFLICT = (
    "Resource in projects [test-project:setIamPolicy] is the subject of a conflict: "
    "There were concurrent policy changes. Please retry the whole read-modify-write "
    "with exponential backoff."
)
DELETED_MEMBER = (
    "INVALID_ARGUMENT: Deleted member "
    "'gone-sa@test-project.iam.gserviceaccount.com?uid=123' is not deleted."
)
NOT_YET_VISIBLE = f"INVALID_ARGUMENT: Service account {EMAIL} does not exist."
DENIED = "PERMISSION_DENIED: Policy update access denied."
QUOTA_PER_MINUTE = (
    "ERROR: (gcloud.projects.remove-iam-policy-binding) HttpError accessing "
    "<https://cloudresourcemanager.googleapis.com/v1/projects/test:getIamPolicy?alt=json>: "
    "response: <{'status': 429}>, content <{ \"error\": { \"code\": 429, \"message\": "
    "\"Quota exceeded for quota metric 'Read requests' and limit 'Read requests per minute' "
    "of service 'cloudresourcemanager.googleapis.com' for consumer 'project_number:1'.\", "
    "\"status\": \"RESOURCE_EXHAUSTED\" } }>"
)
# A quota over a longer window does not refill within the retries.
QUOTA_PER_DAY = QUOTA_PER_MINUTE.replace("per minute", "per day")
KEY_REFUSED = (
    "ERROR: (gcloud.iam.service-accounts.keys.create) FAILED_PRECONDITION: "
    "Key creation is not allowed on this service account."
)


class ServiceAccountScriptTestCase(SimpleTestCase):
    """A scratch directory with fake gcloud and sleep commands first on PATH"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-sa-script-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        bin_dir = self.tmp / "bin"
        bin_dir.mkdir()
        gcloud = bin_dir / "gcloud"
        gcloud.write_text(GCLOUD_FAKE)
        gcloud.chmod(0o755)
        sleep = bin_dir / "sleep"
        sleep.write_text(SLEEP_FAKE)
        sleep.chmod(0o755)
        self.log = self.tmp / "calls.log"
        self.log.touch()
        self.sleep_log = self.tmp / "sleeps.log"
        self.sleep_log.touch()
        self.env = dict(
            os.environ,
            PATH=f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
            FAKE_LOG=str(self.log),
            FAKE_SLEEP_LOG=str(self.sleep_log),
        )

    def iam_errors(self, *messages):
        """The policy edits fail, in order, with these messages ("" succeeds)."""
        path = self.tmp / "iam_errors"
        path.write_text("".join(f"{message}\n" for message in messages))
        self.env["FAKE_IAM_ERRORS"] = str(path)

    def sleeps(self):
        return self.sleep_log.read_text().split()

    def policy_rows(self, *rows):
        self.env["FAKE_POLICY_ROWS"] = "".join(f"{role}\t{member}\n" for role, member in rows)

    def grant_calls(self):
        return [c for c in self.calls() if c[:2] == ["projects", "add-iam-policy-binding"]]

    def removal_calls(self):
        return [c for c in self.calls() if c[:2] == ["projects", "remove-iam-policy-binding"]]

    def delete_calls(self):
        return [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "delete"]]

    def run_script(self, *args, stdin_text=None):
        return subprocess.run(
            ["bash", str(SCRIPT), *args],
            cwd=self.tmp,
            env=self.env,
            input=stdin_text,
            stdin=subprocess.DEVNULL if stdin_text is None else None,
            capture_output=True,
            text=True,
            check=False,
        )

    def calls(self):
        return [line.split("\x1f") for line in self.log.read_text().splitlines()]

    def granted_roles(self):
        roles = []
        for call in self.calls():
            if call[:2] == ["projects", "add-iam-policy-binding"]:
                self.assertEqual(call[2], PROJECT)
                self.assertIn(f"--member=serviceAccount:{EMAIL}", call)
                # Unconditioned, and stated so: gcloud refuses a grant with no
                # --condition, when not interactive, on a policy that already
                # holds conditional bindings.
                self.assertEqual(
                    [arg for arg in call if arg.startswith("--condition")],
                    ["--condition=None"],
                    call,
                )
                roles.extend(arg[len("--role=roles/"):] for arg in call if arg.startswith("--role="))
        return roles

    def assert_no_key(self):
        # No key is created; create only lists an existing account's keys.
        for call in self.calls():
            self.assertNotEqual(call[:4], ["iam", "service-accounts", "keys", "create"], call)


class ServiceAccountScriptTests(ServiceAccountScriptTestCase):
    """create makes a keyless account with the cited roles; credential keeps working"""

    def test_create_makes_account_without_key(self):
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        creates = [call for call in self.calls() if call[:3] == ["iam", "service-accounts", "create"]]
        self.assertEqual(len(creates), 1)
        self.assertEqual(creates[0][3], ACCOUNT)
        self.assertIn(f"--project={PROJECT}", creates[0])
        self.assert_no_key()

    def test_create_grants_exactly_the_cited_roles(self):
        self.assertEqual(self.run_script("create", PROJECT, ACCOUNT).returncode, 0)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_roles_dropped_from_the_old_list(self):
        self.assertEqual(self.run_script("create", PROJECT, ACCOUNT).returncode, 0)
        roles = self.granted_roles()
        for dropped in (
            "aiplatform.admin",
            "bigquery.admin",
            "cloudbuild.builds.builder",
            # Added by another change, in its own hunk, not by this list.
            "cloudkms.admin",
        ):
            self.assertNotIn(dropped, roles)

    def test_existing_account_is_reused_and_given_the_roles(self):
        self.env["FAKE_DESCRIBE_RC"] = "0"
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("reusing it", result.stdout)
        self.assertFalse(
            [call for call in self.calls() if call[:3] == ["iam", "service-accounts", "create"]]
        )
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)
        self.assert_no_key()

    def describe_calls(self):
        return [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "describe"]]

    def create_calls(self):
        return [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "create"]]

    def test_account_not_found_is_created(self):
        # The default describe fails with gcloud's NOT_FOUND error
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.describe_calls(),
            [["iam", "service-accounts", "describe", EMAIL, f"--project={PROJECT}"]],
        )
        self.assertEqual(len(self.create_calls()), 1)
        calls = self.calls()
        self.assertLess(calls.index(self.describe_calls()[0]), calls.index(self.create_calls()[0]))
        self.assertIn(f"Creating service account [{ACCOUNT}]", result.stdout)
        self.assertNotIn("NOT_FOUND", result.stderr)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)
        # NOT_FOUND alone decides: the account list is not read.
        self.assertEqual(self.list_calls(), [])

    def list_calls(self):
        return [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "list"]]

    # gcloud's answer, seen live, to describe of an account deleted moments ago.
    DESCRIBE_DENIED = (
        "ERROR: (gcloud.iam.service-accounts.describe) PERMISSION_DENIED: "
        "Permission 'iam.serviceAccounts.get' denied on resource (or it may not exist)."
    )
    OTHER_DESCRIBE_ERRORS = (
        DESCRIBE_DENIED,
        "ERROR: (gcloud.iam.service-accounts.describe) There was a problem refreshing "
        "your current auth tokens: Unable to find the server at oauth2.googleapis.com",
        QUOTA_PER_MINUTE,
    )

    def test_describe_denied_and_absent_from_the_list_is_created(self):
        # Any failure other than NOT_FOUND falls back to the account list:
        # an account it does not show is absent.
        for error in self.OTHER_DESCRIBE_ERRORS:
            with self.subTest(error=error[:60]):
                self.log.write_text("")
                self.env.update(
                    FAKE_DESCRIBE_RC="1",
                    FAKE_DESCRIBE_ERROR=error,
                    FAKE_ACCOUNTS=f"Other|other-sa@{PROJECT}.iam.gserviceaccount.com",
                )
                result = self.run_script("create", PROJECT, ACCOUNT)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(self.describe_calls()), 1)
                self.assertEqual(
                    self.list_calls(),
                    [["iam", "service-accounts", "list", f"--project={PROJECT}",
                      "--format=value(email)"]],
                )
                self.assertEqual(len(self.create_calls()), 1)
                calls = self.calls()
                self.assertLess(calls.index(self.list_calls()[0]), calls.index(self.create_calls()[0]))
                self.assertIn(f"Creating service account [{ACCOUNT}]", result.stdout)
                self.assertNotIn("reusing it", result.stdout)
                self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_describe_denied_and_in_the_list_is_reused(self):
        self.env.update(
            FAKE_DESCRIBE_RC="1",
            FAKE_DESCRIBE_ERROR=self.DESCRIBE_DENIED,
            FAKE_ACCOUNTS=f"Prov|{EMAIL}",
        )
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.list_calls()), 1)
        self.assertIn("reusing it", result.stdout)
        self.assertEqual(self.create_calls(), [])
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)
        self.assert_no_key()

    def test_describe_and_list_that_both_fail_create_and_grant_nothing(self):
        # Neither read says whether the account exists.
        for error in self.OTHER_DESCRIBE_ERRORS:
            with self.subTest(error=error[:60]):
                self.log.write_text("")
                self.env.update(
                    FAKE_DESCRIBE_RC="1",
                    FAKE_DESCRIBE_ERROR=error,
                    FAKE_READ_CMD="list",
                    FAKE_READ_FAILS="99",
                    FAKE_READ_ERROR=DENIED,
                )
                result = self.run_script("create", PROJECT, ACCOUNT)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn(error, result.stderr)
                self.assertIn(DENIED, result.stderr)
                output = " ".join(result.stderr.split())
                self.assertIn(
                    f"Error: Could not check whether service account {EMAIL} exists "
                    "(errors above: neither reading it nor listing the project's service "
                    "accounts worked), so it was neither created nor given any role.",
                    output,
                )
                self.assertEqual(len(self.describe_calls()), 1)
                self.assertEqual(len(self.list_calls()), 1)
                self.assertEqual(self.create_calls(), [])
                self.assertEqual(self.grant_calls(), [])
                self.assertNotIn("reusing it", result.stdout)
                self.assertNotIn("Creating service account", result.stdout)

    def test_describe_failure_with_another_exit_status_falls_back_to_the_list(self):
        self.env.update(FAKE_DESCRIBE_RC="2", FAKE_DESCRIBE_ERROR="gcloud crashed")
        with self.subTest("list fails too"):
            self.env.update(FAKE_READ_CMD="list", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
            result = self.run_script("create", PROJECT, ACCOUNT)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn("gcloud crashed", result.stderr)
            self.assertEqual(self.create_calls(), [])
            self.assertEqual(self.grant_calls(), [])
        with self.subTest("list without the account"):
            self.log.write_text("")
            self.env.update(FAKE_READ_FAILS="0")
            result = self.run_script("create", PROJECT, ACCOUNT)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(len(self.create_calls()), 1)

    def test_failed_create_grants_nothing(self):
        self.env["FAKE_CREATE_RC"] = "1"
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertIn("Failed to create service account", result.stderr)
        self.assertEqual(self.granted_roles(), [])

    def test_name_length_limits(self):
        for name in ("a" * 30, "abcdef"):
            self.log.write_text("")
            self.assertEqual(self.run_script("create", PROJECT, name).returncode, 0, name)
        for name in ("a" * 31, "abcde"):
            self.log.write_text("")
            result = self.run_script("create", PROJECT, name)
            self.assertEqual(result.returncode, 99, name)
            self.assertIn(f"Invalid service account name [{name}] ({len(name)} characters)", result.stderr)
            self.assertIn("6 to 30 characters", result.stderr)
            # Refused before anything is created or granted.
            self.assertEqual(self.calls(), [], name)

    def test_invalid_characters_refused(self):
        for name in ("Testdep-prov-sa", "test.dep-prov-sa", "1testdep-prov", "testdep-prov-"):
            result = self.run_script("create", PROJECT, name)
            self.assertEqual(result.returncode, 99, name)
        self.assertEqual(self.calls(), [])

    def test_every_grant_is_explicitly_unconditioned(self):
        self.assertEqual(self.run_script("create", PROJECT, ACCOUNT).returncode, 0)
        grants = [c for c in self.calls() if c[:2] == ["projects", "add-iam-policy-binding"]]
        self.assertEqual(len(grants), len(EXPECTED_ROLES))
        for call in grants:
            self.assertIn("--condition=None", call)

    def test_check_finds_an_account_with_a_display_name(self):
        # The default table puts the display name first; check must still
        # find the account by its e-mail address.
        self.env["FAKE_ACCOUNTS"] = f"Open Front End web server|{EMAIL}"
        self.assertEqual(self.run_script("check", PROJECT, ACCOUNT).returncode, 0)
        lists = [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "list"]]
        self.assertEqual(
            lists,
            [["iam", "service-accounts", "list", f"--project={PROJECT}", "--format=value(email)"]],
        )

    def test_check_matches_the_whole_address_only(self):
        self.env["FAKE_ACCOUNTS"] = "\n".join([
            f"|x{EMAIL}",
            f"|{ACCOUNT}x@{PROJECT}.iam.gserviceaccount.com",
        ])
        self.assertEqual(self.run_script("check", PROJECT, ACCOUNT).returncode, 1)

    def test_check_without_the_account(self):
        self.assertEqual(self.run_script("check", PROJECT, ACCOUNT).returncode, 1)

    def test_delete_removes_each_role_unconditioned_then_the_account(self):
        held = ["roles/compute.admin", "roles/iam.roleAdmin"]
        self.policy_rows(*((role, f"serviceAccount:{EMAIL}") for role in held))
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        policy_read = [c for c in calls if c[:2] == ["projects", "get-iam-policy"]]
        self.assertEqual(len(policy_read), 1)
        self.assertIn(f"--filter=bindings.members:{EMAIL}", policy_read[0])
        removals = [c for c in calls if c[:2] == ["projects", "remove-iam-policy-binding"]]
        self.assertEqual(
            removals,
            [
                [
                    "projects", "remove-iam-policy-binding", PROJECT,
                    f"--member=serviceAccount:{EMAIL}", f"--role={role}", "--condition=None",
                ]
                for role in held
            ],
        )
        deletes = [c for c in calls if c[:3] == ["iam", "service-accounts", "delete"]]
        self.assertEqual(
            deletes, [["iam", "service-accounts", "delete", EMAIL, "--quiet", f"--project={PROJECT}"]]
        )
        # Every role goes before the account, so none is left under a
        # "deleted:" member.
        self.assertLess(max(calls.index(c) for c in removals), calls.index(deletes[0]))

    def test_delete_of_a_missing_account_fails_cleanly(self):
        # No roles to remove, and the account delete itself fails.
        self.env["FAKE_DELETE_RC"] = "1"
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertIn("Failed to delete service account", result.stderr)
        self.assertFalse(
            [c for c in self.calls() if c[:2] == ["projects", "remove-iam-policy-binding"]]
        )

    def test_credential_still_writes_a_key(self):
        keyfile = self.tmp / "key.json"
        result = self.run_script("credential", PROJECT, ACCOUNT, str(keyfile))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.calls(),
            [["iam", "service-accounts", "keys", "create", str(keyfile), f"--iam-account={EMAIL}"]],
        )

    def test_refused_credential_prints_gclouds_error(self):
        self.env.update(FAKE_KEY_RC="1", FAKE_KEY_ERROR=KEY_REFUSED)
        result = self.run_script("credential", PROJECT, ACCOUNT, str(self.tmp / "key.json"))
        self.assertEqual(result.returncode, 1)
        self.assertIn(KEY_REFUSED, result.stderr)
        self.assertIn(f"Failed to create a key for {EMAIL}", result.stderr)

    def test_list_prints_the_projects_own_accounts(self):
        self.env["FAKE_ACCOUNTS"] = "\n".join([
            f"Provisioning|{EMAIL}",
            f"|other-sa@{PROJECT}.iam.gserviceaccount.com",
            # Made by Google APIs, not by the project
            "|123-compute@developer.gserviceaccount.com",
            f"|x-sa@{PROJECT}-other.iam.gserviceaccount.com",
        ])
        result = self.run_script("list", PROJECT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.split(), [ACCOUNT, "other-sa"])

    def test_list_of_a_project_without_accounts_succeeds(self):
        result = self.run_script("list", PROJECT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "")

    def test_list_that_fails_returns_1_with_gclouds_error(self):
        self.env.update(FAKE_READ_CMD="list", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.run_script("list", PROJECT)
        self.assertEqual(result.returncode, 1)
        self.assertIn(DENIED, result.stderr)
        self.assertEqual(result.stdout.strip(), "")

    def test_help_states_the_exit_codes(self):
        result = self.run_script("--help")
        self.assertIn(
            "Exit codes: 0 on success; 1 on failure (gcloud's error is printed); for "
            "check, 0 if the account exists, 1 if it does not and 2 if the accounts "
            "could not be listed; and 99 for a missing or invalid argument.",
            " ".join(result.stdout.split()),
        )


KEY_IDS = ["0123456789abcdef", "fedcba9876543210"]
KEY_NAMES = "".join(
    f"projects/{PROJECT}/serviceAccounts/{EMAIL}/keys/{key_id}\n" for key_id in KEY_IDS
)
KEYS_LIST_CALL = [
    "iam", "service-accounts", "keys", "list",
    f"--iam-account={EMAIL}", "--managed-by=user", "--format=value(name)",
]


class ExistingKeyTests(ServiceAccountScriptTestCase):
    """create warns about the user-managed keys of an account it reuses,
    whose holders would get its roles, before granting them"""

    def setUp(self):
        super().setUp()
        self.env["FAKE_DESCRIBE_RC"] = "0"  # the account exists

    def key_lists(self):
        return [c for c in self.calls() if c[:4] == ["iam", "service-accounts", "keys", "list"]]

    def assert_keys_warned(self, result):
        output = " ".join(result.stderr.split())
        self.assertIn(f"Warning: {EMAIL} already has user-managed keys:", output)
        self.assertIn("Anyone who holds one of these keys gets the roles granted here", output)
        self.assertIn(f"including IAM administration in project {PROJECT}", output)
        for key_id in KEY_IDS:
            self.assertIn(
                f"gcloud iam service-accounts keys delete {key_id} --iam-account={EMAIL}",
                result.stderr,
            )

    def test_keys_are_listed_before_any_grant(self):
        self.env["FAKE_USER_KEYS"] = KEY_NAMES
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.key_lists(), [KEYS_LIST_CALL])
        calls = self.calls()
        self.assertLess(calls.index(KEYS_LIST_CALL), calls.index(self.grant_calls()[0]))

    def test_keys_warned_with_their_ids_and_delete_commands_then_granted(self):
        self.env["FAKE_USER_KEYS"] = KEY_NAMES
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_keys_warned(result)
        for key_id in KEY_IDS:
            self.assertIn(f"\n    {key_id}\n", result.stderr)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)
        self.assert_no_key()

    def test_no_keys_no_warning(self):
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.key_lists(), [KEYS_LIST_CALL])
        self.assertNotIn("Warning", result.stderr)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_unreadable_key_list_warns_and_grants(self):
        self.env.update(FAKE_READ_CMD="keys", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        output = " ".join(result.stderr.split())
        self.assertIn(DENIED, result.stderr)
        self.assertIn(f"Could not list the user-managed keys of {EMAIL}", output)
        self.assertIn("was not checked", output)
        self.assertIn(
            f"gcloud iam service-accounts keys list --iam-account={EMAIL} --managed-by=user",
            result.stderr,
        )
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_key_list_retried_on_the_per_minute_quota(self):
        self.env.update(
            FAKE_USER_KEYS=KEY_NAMES,
            FAKE_READ_CMD="keys",
            FAKE_READ_FAILS="1",
            FAKE_READ_ERROR=QUOTA_PER_MINUTE,
        )
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.sleeps(), ["2"])
        self.assert_keys_warned(result)

    def test_new_account_lists_no_keys(self):
        self.env.update(FAKE_DESCRIBE_RC="1", FAKE_USER_KEYS=KEY_NAMES)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.key_lists(), [])
        self.assertNotIn("Warning", result.stderr)

    def test_ask_no_grants_nothing(self):
        self.env["FAKE_USER_KEYS"] = KEY_NAMES
        for answer in ("n\n", "\n", ""):
            self.log.write_text("")
            result = self.run_script("create", PROJECT, ACCOUNT, "--ask", stdin_text=answer)
            self.assertEqual(result.returncode, 3, repr(answer))
            self.assert_keys_warned(result)
            self.assertIn("Continue and grant the roles to this account? [y/N]", result.stdout)
            self.assertIn(f"No role was granted to {EMAIL}.", result.stdout)
            self.assertEqual(self.grant_calls(), [], repr(answer))

    def test_ask_yes_grants(self):
        self.env["FAKE_USER_KEYS"] = KEY_NAMES
        result = self.run_script("create", PROJECT, ACCOUNT, "--ask", stdin_text="y\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Continue and grant the roles to this account? [y/N]", result.stdout)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_ask_when_the_keys_cannot_be_listed(self):
        self.env.update(FAKE_READ_CMD="keys", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.run_script("create", PROJECT, ACCOUNT, "--ask", stdin_text="n\n")
        self.assertEqual(result.returncode, 3)
        self.assertIn("Could not list the user-managed keys", result.stderr)
        self.assertEqual(self.grant_calls(), [])

    def test_ask_without_keys_asks_nothing(self):
        result = self.run_script("create", PROJECT, ACCOUNT, "--ask")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("Continue and grant", result.stdout)
        self.assertEqual(self.granted_roles(), EXPECTED_ROLES)

    def test_unknown_create_option_is_refused(self):
        result = self.run_script("create", PROJECT, ACCOUNT, "--yes")
        self.assertEqual(result.returncode, 99)
        self.assertIn("Unrecognised option to create: --yes", result.stderr)
        self.assertEqual(self.calls(), [])

    def test_help_states_the_unchecked_create_exit_code(self):
        result = self.run_script("--help")
        self.assertIn(
            "For create, 2 if whether the account exists could not be checked "
            "(gcloud's error is printed): nothing was created or granted.",
            " ".join(result.stdout.split()),
        )

    def test_help_states_the_ask_exit_code(self):
        result = self.run_script("--help")
        self.assertIn(
            "For create with --ask, 3 when the answer is no: no role was granted.",
            " ".join(result.stdout.split()),
        )


class IamRetryTests(ServiceAccountScriptTestCase):
    """Policy edits are retried, within a bound, on the transient IAM errors only"""

    def test_grant_retried_while_the_new_account_is_not_visible(self):
        self.iam_errors(NOT_YET_VISIBLE, NOT_YET_VISIBLE)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        # The first role is asked for three times, then every other role once.
        self.assertEqual(self.granted_roles(), [EXPECTED_ROLES[0]] * 2 + EXPECTED_ROLES)
        self.assertEqual(self.sleeps(), RETRY_DELAYS[:2])
        self.assertIn("does not exist", result.stderr)

    def test_grant_gives_up_after_the_bound(self):
        self.env["FAKE_IAM_ERROR_ALWAYS"] = NOT_YET_VISIBLE
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.granted_roles(), [EXPECTED_ROLES[0]] * (len(RETRY_DELAYS) + 1))
        self.assertEqual(self.sleeps(), RETRY_DELAYS)
        self.assertIn(f"Could not grant [roles/{EXPECTED_ROLES[0]}] to {EMAIL}", result.stderr)
        # gcloud's own last error is shown.
        self.assertIn(NOT_YET_VISIBLE, result.stderr)

    def test_grant_not_retried_for_another_account_not_found(self):
        other = f"INVALID_ARGUMENT: Service account other-sa@{PROJECT}.iam.gserviceaccount.com does not exist."
        self.iam_errors(other)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.grant_calls()), 1)
        self.assertEqual(self.sleeps(), [])

    def test_grant_not_retried_for_other_errors(self):
        self.iam_errors(DENIED)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.grant_calls()), 1)
        self.assertEqual(self.sleeps(), [])
        self.assertIn(DENIED, result.stderr)

    def test_grant_retried_on_conflict_and_deleted_member(self):
        self.iam_errors(CONFLICT, DELETED_MEMBER, "", CONFLICT)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.granted_roles(),
            [EXPECTED_ROLES[0]] * 2 + EXPECTED_ROLES[:2] + EXPECTED_ROLES[1:],
        )
        self.assertEqual(self.sleeps(), ["2", "4", "2"])

    def test_removal_retried_on_conflict_and_deleted_member(self):
        held = ["roles/compute.admin", "roles/iam.roleAdmin"]
        self.policy_rows(*((role, f"serviceAccount:{EMAIL}") for role in held))
        self.iam_errors(DELETED_MEMBER, DELETED_MEMBER, "", CONFLICT)
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        removed = [c[4] for c in self.removal_calls()]
        self.assertEqual(removed, ["--role=roles/compute.admin"] * 3 + ["--role=roles/iam.roleAdmin"] * 2)
        self.assertEqual(self.sleeps(), ["2", "4", "2"])
        self.assertEqual(len(self.delete_calls()), 1)

    def test_removal_retried_on_the_per_minute_quota(self):
        held = ["roles/compute.admin", "roles/iam.roleAdmin"]
        self.policy_rows(*((role, f"serviceAccount:{EMAIL}") for role in held))
        self.iam_errors(QUOTA_PER_MINUTE, QUOTA_PER_MINUTE, "", QUOTA_PER_MINUTE)
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        removed = [c[4] for c in self.removal_calls()]
        self.assertEqual(removed, ["--role=roles/compute.admin"] * 3 + ["--role=roles/iam.roleAdmin"] * 2)
        self.assertEqual(self.sleeps(), ["2", "4", "2"])
        self.assertEqual(len(self.delete_calls()), 1)

    def test_grant_retried_on_the_per_minute_quota(self):
        self.iam_errors(QUOTA_PER_MINUTE, "", QUOTA_PER_MINUTE)
        result = self.run_script("create", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.granted_roles(),
            [EXPECTED_ROLES[0]] * 2 + [EXPECTED_ROLES[1]] * 2 + EXPECTED_ROLES[2:],
        )
        self.assertEqual(self.sleeps(), ["2", "2"])

    def test_removal_not_retried_on_a_per_day_quota(self):
        self.policy_rows(("roles/compute.admin", f"serviceAccount:{EMAIL}"))
        self.iam_errors(QUOTA_PER_DAY)
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.removal_calls()), 1)
        self.assertEqual(self.sleeps(), [])
        self.assertEqual(self.delete_calls(), [])
        self.assertIn("per day", result.stderr)
        self.assertIn("so the account was not deleted", result.stderr)

    def test_failed_removal_keeps_the_account_and_prints_the_commands(self):
        held = ["roles/compute.admin", "roles/iam.roleAdmin", "roles/pubsub.admin"]
        self.policy_rows(*((role, f"serviceAccount:{EMAIL}") for role in held))
        self.iam_errors("", *[CONFLICT] * (len(RETRY_DELAYS) + 1))
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.delete_calls(), [])
        # The first role went; the second failed after every retry; the third
        # was not tried.
        removed = [c[4] for c in self.removal_calls()]
        self.assertEqual(
            removed,
            ["--role=roles/compute.admin"] + ["--role=roles/iam.roleAdmin"] * (len(RETRY_DELAYS) + 1),
        )
        self.assertEqual(self.sleeps(), RETRY_DELAYS)
        self.assertIn("so the account was not deleted", result.stderr)
        for role in held[1:]:
            self.assertIn(
                f"gcloud projects remove-iam-policy-binding {PROJECT} "
                f"--member=serviceAccount:{EMAIL} --role={role} --condition=None",
                result.stderr,
            )
        self.assertIn(f"gcloud iam service-accounts delete {EMAIL} --project={PROJECT}", result.stderr)
        self.assertIn("There were concurrent policy changes", result.stderr)

    def test_removal_not_retried_for_other_errors(self):
        self.policy_rows(("roles/compute.admin", f"serviceAccount:{EMAIL}"))
        self.iam_errors(DENIED)
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.removal_calls()), 1)
        self.assertEqual(self.sleeps(), [])
        self.assertEqual(self.delete_calls(), [])
        self.assertIn(DENIED, result.stderr)

    def test_delete_removes_only_the_accounts_own_grants(self):
        # The filter also matches the "deleted:" form of an earlier account
        # with the same name; that grant is not this account's to remove.
        self.policy_rows(
            ("roles/compute.admin", f"serviceAccount:{EMAIL}"),
            ("roles/iam.roleAdmin", f"deleted:serviceAccount:{EMAIL}?uid=1"),
        )
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([c[4] for c in self.removal_calls()], ["--role=roles/compute.admin"])
        self.assertEqual(len(self.delete_calls()), 1)

    def test_failed_account_delete_prints_gclouds_error(self):
        self.env.update(FAKE_DELETE_RC="1", FAKE_DELETE_ERROR=DENIED)
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.delete_calls()), 1)
        self.assertIn(DENIED, result.stderr)
        self.assertIn(f"gcloud iam service-accounts delete {EMAIL} --project={PROJECT}", result.stderr)

    def test_unreadable_policy_keeps_the_account(self):
        self.env["FAKE_POLICY_RC"] = "1"
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.removal_calls(), [])
        self.assertEqual(self.delete_calls(), [])
        self.assertIn("was not deleted", result.stderr)

    def test_check_that_cannot_list_returns_2_with_the_error(self):
        # A failed list is not an absent account
        self.env.update(
            FAKE_ACCOUNTS=f"x|{EMAIL}",
            FAKE_READ_CMD="list",
            FAKE_READ_FAILS="99",
            FAKE_READ_ERROR=DENIED,
        )
        result = self.run_script("check", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 2)
        self.assertIn(DENIED, result.stderr)
        self.assertEqual(self.sleeps(), [])

    def test_check_list_retried_on_the_per_minute_quota(self):
        self.env.update(
            FAKE_ACCOUNTS=f"x|{EMAIL}",
            FAKE_READ_CMD="list",
            FAKE_READ_FAILS="2",
            FAKE_READ_ERROR=QUOTA_PER_MINUTE,
        )
        result = self.run_script("check", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.sleeps(), ["2", "4"])

    def test_read_gives_up_after_the_bound(self):
        self.env.update(
            FAKE_ACCOUNTS=f"x|{EMAIL}",
            FAKE_READ_CMD="list",
            FAKE_READ_FAILS="99",
            FAKE_READ_ERROR=QUOTA_PER_MINUTE,
        )
        result = self.run_script("check", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 2)
        lists = [c for c in self.calls() if c[:3] == ["iam", "service-accounts", "list"]]
        self.assertEqual(len(lists), len(RETRY_DELAYS) + 1)
        self.assertEqual(self.sleeps(), RETRY_DELAYS)
        # gcloud's last error is shown.
        self.assertIn(QUOTA_PER_MINUTE, result.stderr)

    def test_delete_policy_read_retried_on_the_per_minute_quota(self):
        self.policy_rows(("roles/compute.admin", f"serviceAccount:{EMAIL}"))
        self.env.update(
            FAKE_READ_CMD="policy", FAKE_READ_FAILS="1", FAKE_READ_ERROR=QUOTA_PER_MINUTE
        )
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.sleeps(), ["2"])
        self.assertEqual([c[4] for c in self.removal_calls()], ["--role=roles/compute.admin"])
        self.assertEqual(len(self.delete_calls()), 1)

    def test_delete_policy_read_not_retried_on_a_per_day_quota(self):
        self.policy_rows(("roles/compute.admin", f"serviceAccount:{EMAIL}"))
        self.env.update(
            FAKE_READ_CMD="policy", FAKE_READ_FAILS="99", FAKE_READ_ERROR=QUOTA_PER_DAY
        )
        result = self.run_script("delete", PROJECT, ACCOUNT)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.sleeps(), [])
        self.assertEqual(self.removal_calls(), [])
        self.assertEqual(self.delete_calls(), [])
        self.assertIn("per day", result.stderr)
        self.assertIn("was not deleted", result.stderr)

    def test_deploy_sh_has_the_same_retry(self):
        # deploy.sh and teardown.sh edit policies with their own copy.
        def block(path):
            text = path.read_text()
            start = text.index("IAM_RETRY_DELAYS=(")
            end = text.index("\n}\n", text.index("iam_policy_edit() {")) + 3
            return text[start:end]

        self.assertEqual(block(OFE_DIR / "deploy.sh"), block(SCRIPT))
