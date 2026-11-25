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

"""deploy.sh and teardown.sh, run against fake terraform and gcloud commands.

Each test copies the script into a scratch directory with an empty tf/
directory and puts fake `terraform` and `gcloud` executables first on PATH.
The fakes record every call and answer from files and environment variables
the test sets, so nothing reaches Terraform or Google Cloud.
"""

import json
import os
import pty
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

from django.test import SimpleTestCase

OFE_DIR = Path(__file__).resolve().parents[3]

PROJECT = "test-project"
DEPLOYMENT = "testdep"
EMAIL = f"{DEPLOYMENT}-fe-sa@{PROJECT}.iam.gserviceaccount.com"
MEMBER = f"serviceAccount:{EMAIL}"

NEW_ADDRESS = 'module.fe_sa.module.service_account.google_service_account.service_accounts["fe-sa"]'
OLD_ADDRESS = 'module.service_account.google_service_account.service_accounts["fe-sa"]'
WAITS = [
    "module.fe_sa.time_sleep.wait_for_service_account",
    "module.fe_profile.time_sleep.wait_for_iam",
]
REPLACE_FLAGS = [f"-replace={wait}" for wait in WAITS]
SUPERSEDED = [
    "storage.objectAdmin",
    "cloudtrace.agent",
    "pubsub.admin",
    "artifactregistry.admin",
    "secretmanager.admin",
]
OLD_ROLES = SUPERSEDED + ["logging.logWriter", "monitoring.metricWriter"]
OLD_UID = "111"
DELETED_MEMBER = f"deleted:serviceAccount:{EMAIL}?uid={OLD_UID}"
OLD_GRANT_ADDRESSES = [
    "module.service_account.google_project_iam_member.project-roles"
    f'["fe-sa-{PROJECT}=>roles/{role}"]'
    for role in OLD_ROLES
]
NEW_GRANT_ADDRESSES = [
    "module.fe_sa.module.service_account.google_project_iam_member.project-roles"
    f'["fe-sa-{PROJECT}=>roles/{role}"]'
    for role in ("logging.logWriter", "monitoring.metricWriter")
]
BUCKET_ADDRESS = 'module.control_storage.google_storage_bucket.buckets["storage"]'
OLD_BUCKET_ADDRESS = 'module.control_bucket.google_storage_bucket.buckets["storage"]'
OTHER_ADDRESS = "google_storage_bucket_object.deployment_file[0]"


def inconsistent_result_output(address):
    """A failed apply's output with the inconsistent-result error on address.

    As Terraform writes it to a pipe: colour codes, a box down the left of
    the error, and the text wrapped at 78 columns, which puts a long address
    on its own line. A refresh line before the error names the web server's
    account, as every real apply log does.
    """
    esc = "\x1b"
    bar = f"{esc}[31m│{esc}[0m {esc}[0m"
    lines = [
        f"{NEW_ADDRESS}: Refreshing state... [id=projects/{PROJECT}/serviceAccounts/{EMAIL}]",
        f"{esc}[31m╷{esc}[0m{esc}[0m",
        f"{bar}{esc}[1m{esc}[31mError: {esc}[0m{esc}[0m{esc}[1m"
        f"Provider produced inconsistent result after apply{esc}[0m",
        bar,
    ]
    first = f"When applying changes to {address},"
    if len(first) <= 78:
        lines.append(f"{bar}{esc}[0m{first}")
    else:
        lines += [f"{bar}{esc}[0mWhen applying changes to", f"{bar}{address},"]
    lines += [
        f'{bar}provider "provider[\\"registry.terraform.io/hashicorp/google\\"]" produced an',
        f"{bar}unexpected new value: Root object was present, but now absent.",
        bar,
        f"{bar}This is a bug in the provider, which should be reported in the provider's",
        f"{bar}own issue tracker.",
        f"{esc}[31m╵{esc}[0m{esc}[0m",
    ]
    return "".join(f"{line}\n" for line in lines)


# Calls are logged one per line, arguments separated by a unit separator, so
# arguments containing spaces or quotes are recovered exactly.
TERRAFORM_FAKE = r"""#!/bin/bash
# The Terraform state bucket's own root, which deploy.sh applies with -chdir
# before tf/: logged apart, to <FAKE_LOG>.state_bucket, so the tf/ calls the
# tests count are unchanged. It has no local state, so deploy.sh creates the
# bucket, named FAKE_STATE_BUCKET.
if [[ $1 == -chdir=* ]]; then
	(IFS=$'\x1f'; printf 'terraform\x1f%s\n' "$*") >>"${FAKE_LOG}.state_bucket"
	case "$2" in
	console) echo "\"${FAKE_STATE_BUCKET}\"" ;;
	output) echo "${FAKE_STATE_BUCKET}" ;;
	esac
	exit 0
fi
(IFS=$'\x1f'; printf 'terraform\x1f%s\n' "$*") >>"${FAKE_LOG}"
case "$1" in
init)
	echo "fake init output"
	exit "${FAKE_INIT_RC:-0}"
	;;
state)
	case "$2" in
	list)
		# FAKE_STATE_LIST_RC: fail with FAKE_STATE_LIST_ERROR on stderr.
		if [[ -n ${FAKE_STATE_LIST_RC:-} ]]; then
			echo "${FAKE_STATE_LIST_ERROR:-}" >&2
			exit "${FAKE_STATE_LIST_RC}"
		fi
		# FAKE_STATE_LIST_FAIL_FROM=N: the first N-1 calls succeed; from the
		# Nth on, fail with exit 1 and FAKE_STATE_LIST_ERROR on stderr.
		if [[ -n ${FAKE_STATE_LIST_FAIL_FROM:-} ]] &&
			[[ $(grep -c $'^terraform\x1fstate\x1flist' "${FAKE_LOG}") -ge ${FAKE_STATE_LIST_FAIL_FROM} ]]; then
			echo "${FAKE_STATE_LIST_ERROR:-}" >&2
			exit 1
		fi
		cat "${FAKE_STATE_LIST}"
		;;
	pull)
		# FAKE_STATE_PULL_RC: fail with FAKE_STATE_PULL_ERROR on stderr.
		if [[ -n ${FAKE_STATE_PULL_RC:-} ]]; then
			echo "${FAKE_STATE_PULL_ERROR:-}" >&2
			exit "${FAKE_STATE_PULL_RC}"
		fi
		# FAKE_STATE_JSON: a file printed as the state.
		if [[ -n ${FAKE_STATE_JSON:-} ]]; then
			cat "${FAKE_STATE_JSON}"
			exit 0
		fi
		# Otherwise a state that holds the addresses in FAKE_STATE_LIST, the
		# bucket with FAKE_BUCKET_LOCATION (when set) and the account with
		# FAKE_STATE_UID; nothing, as for no state, when the list is empty.
		python3 -I - "${FAKE_STATE_LIST}" <<'PYTHON'
import json, os, re, sys

addresses = [line for line in open(sys.argv[1]).read().splitlines() if line]
if not addresses:
    sys.exit(0)
resources = {}
for address in addresses:
    module, rtype, name, key = re.fullmatch(
        r"((?:module\.[^.\[]+(?:\[[^\]]+\])?\.)*)([^.\[]+)\.([^.\[]+)(?:\[(.+)\])?", address
    ).groups()
    attributes = {"id": "x"}
    if rtype == "google_storage_bucket" and os.environ.get("FAKE_BUCKET_LOCATION"):
        attributes["location"] = os.environ["FAKE_BUCKET_LOCATION"]
    if rtype == "google_service_account":
        attributes["unique_id"] = os.environ.get("FAKE_STATE_UID", "")
    instance = {"schema_version": 0, "attributes": attributes}
    if key is not None:
        instance["index_key"] = json.loads(key)
    resource = {"mode": "managed", "type": rtype, "name": name, "instances": []}
    if module:
        resource["module"] = module.rstrip(".")
    resources.setdefault((module, rtype, name), resource)["instances"].append(instance)
print(json.dumps({"version": 4, "resources": list(resources.values())}, indent=2))
PYTHON
		;;
	show)
		# FAKE_STATE_SHOW_RC: fail as state show did live, on a resource that
		# Google provider 4.x wrote, once terraform init -upgrade had
		# installed 7.x.
		if [[ -n ${FAKE_STATE_SHOW_RC:-} ]]; then
			echo "Error: Failed to marshal state to json: schema version 0 for google_storage_bucket.buckets in state does not match version 4 from the provider" >&2
			exit "${FAKE_STATE_SHOW_RC}"
		fi
		case "${@: -1}" in
		*google_storage_bucket.buckets*)
			# A location line only when FAKE_BUCKET_LOCATION is set.
			printf '# %s:\nresource "google_storage_bucket" "buckets" {\n' "${@: -1}"
			printf '    force_destroy = true\n'
			if [[ -n ${FAKE_BUCKET_LOCATION:-} ]]; then
				printf '    location      = "%s"\n' "${FAKE_BUCKET_LOCATION}"
			fi
			printf '    name          = "x"\n\n    custom_placement_config {\n'
			printf '        data_locations = [\n            "A",\n        ]\n    }\n}\n'
			;;
		*)
			printf '# %s:\nresource "google_service_account" "service_accounts" {\n' "${@: -1}"
			printf '    account_id   = "x"\n    unique_id    = "%s"\n}\n' "${FAKE_STATE_UID}"
			;;
		esac
		;;
	esac
	exit 0
	;;
show)
	# The server's label, unless FAKE_DEPLOYMENT is empty: a state whose
	# resources carry no label, as after a destroy that failed part-way.
	if [[ -n ${FAKE_DEPLOYMENT} ]]; then
		echo '    labels = {'
		echo "        \"ghpcfe_id\" = \"${FAKE_DEPLOYMENT}\""
		echo '    }'
	fi
	exit 0
	;;
apply)
	# The tfvars each apply read, appended to <FAKE_LOG>.tfvars.
	cat terraform.tfvars >>"${FAKE_LOG}.tfvars"
	# FAKE_FIRST_APPLY_INCONSISTENT: a file whose content the first apply
	# prints before it fails; later applies succeed.
	if [[ -n ${FAKE_FIRST_APPLY_INCONSISTENT:-} && ! -e "${FAKE_LOG}.applied" ]]; then
		touch "${FAKE_LOG}.applied"
		cat "${FAKE_FIRST_APPLY_INCONSISTENT}"
		exit 1
	fi
	exit "${FAKE_APPLY_RC:-0}"
	;;
esac
exit 0
"""

GCLOUD_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'gcloud\x1f%s\n' "$*") >>"${FAKE_LOG}"
# FAKE_READ_CMD names one read ("policy" or "keys") that fails, printing
# FAKE_READ_ERROR, on its first FAKE_READ_FAILS calls.
read_fails() {
	[[ ${FAKE_READ_CMD:-} == "$1" ]] || return 1
	local n
	n=$(($(cat "${FAKE_LOG}.reads" 2>/dev/null || echo 0) + 1))
	echo "${n}" >"${FAKE_LOG}.reads"
	[[ ${n} -le ${FAKE_READ_FAILS:-0} ]] || return 1
	echo "${FAKE_READ_ERROR:-}" >&2
}
case "$1 $2 $3" in
"iam service-accounts keys")
	# keys list: prints FAKE_USER_KEYS, one key name per line.
	if read_fails keys; then
		exit 1
	fi
	printf '%s' "${FAKE_USER_KEYS:-}"
	exit 0
	;;
"iam service-accounts describe")
	# FAKE_EXISTING_ACCOUNTS: e-mails ($4) that exist, whatever FAKE_LIVE_UID.
	if [[ " ${FAKE_EXISTING_ACCOUNTS:-} " == *" $4 "* ]]; then
		exit 0
	fi
	if [[ -z ${FAKE_LIVE_UID} ]]; then
		echo "NOT_FOUND" >&2
		exit 1
	fi
	if [[ $* == *uniqueId* ]]; then
		echo "${FAKE_LIVE_UID}"
	fi
	exit 0
	;;
"projects get-iam-policy "*)
	if read_fails policy; then
		exit 1
	fi
	cat "${FAKE_POLICY}"
	exit "${FAKE_POLICY_RC:-0}"
	;;
"config list"*)
	if [[ $* == *core.project* ]]; then
		echo "${FAKE_CONFIG_PROJECT:-}"
	else
		echo "${FAKE_CONFIG_ACCOUNT:-}"
	fi
	exit 0
	;;
"info "*)
	# The Python gcloud runs on.
	echo "${FAKE_GCLOUD_PYTHON:-}"
	exit 0
	;;
"compute instances describe")
	# The server VM ($4). FAKE_SERVER_VM: exists (the default), absent (the
	# instance is not found), zone_absent (the zone is not found) or error.
	zone="" project=""
	for arg in "$@"; do
		case ${arg} in
		--zone=*) zone=${arg#--zone=} ;;
		--project=*) project=${arg#--project=} ;;
		esac
	done
	case ${FAKE_SERVER_VM:-exists} in
	exists)
		echo "$4"
		exit 0
		;;
	absent) resource="projects/${project}/zones/${zone}/instances/$4" ;;
	zone_absent) resource="projects/${project}/zones/${zone}" ;;
	*)
		echo "ERROR: (gcloud.compute.instances.describe) Could not fetch resource:" >&2
		echo " - Required 'compute.instances.get' permission for 'projects/${project}/zones/${zone}/instances/$4'" >&2
		exit 1
		;;
	esac
	echo "ERROR: (gcloud.compute.instances.describe) Could not fetch resource:" >&2
	echo " - The resource '${resource}' was not found" >&2
	exit 1
	;;
"services list"*)
	cat "${FAKE_SERVICES}"
	exit "${FAKE_SERVICES_RC:-0}"
	;;
"services enable"*)
	echo "fake enable output" >&2
	exit "${FAKE_ENABLE_RC:-0}"
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
	exit 0
	;;
esac
exit 0
"""

# sleep, recorded instead of waited for.
SLEEP_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'sleep\x1f%s\n' "$*") >>"${FAKE_LOG}"
"""

# iam_policy_edit's waits before each retry: 8 attempts in all.
RETRY_DELAYS = ["2", "4", "8", "16", "30", "30", "30"]
CONFLICT = (
    f"Resource in projects [{PROJECT}:setIamPolicy] is the subject of a conflict: "
    "There were concurrent policy changes. Please retry the whole read-modify-write "
    "with exponential backoff."
)
DELETED_MEMBER_ERROR = (
    f"INVALID_ARGUMENT: Deleted member '{DEPLOYMENT}-prov-sa@{PROJECT}.iam.gserviceaccount.com"
    "?uid=222' is not deleted."
)
QUOTA_PER_MINUTE = (
    "Quota exceeded for quota metric 'Read requests' and limit 'Read requests per minute' "
    "of service 'cloudresourcemanager.googleapis.com' for consumer 'project_number:1'."
)
DENIED = "PERMISSION_DENIED: Policy update access denied."

# script/service_account.sh, as deploy.sh and teardown.sh call it: records the
# call; create and credential succeed unless FAKE_SA_RC is set; check finds
# only the accounts named in FAKE_SA_EXISTS (space-separated) and cannot check
# those in FAKE_SA_UNCHECKABLE (exit FAKE_SA_CHECK_RC, by default 2, as the
# script does when the accounts cannot be listed), and, as the script does, exits 99
# when the project is empty; delete succeeds unless FAKE_SA_DELETE_RC is set. With FAKE_SA_KEY_ERROR set, credential prints it,
# as the script prints gcloud's error, and fails.
SA_SCRIPT_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'service_account.sh\x1f%s\n' "$*") >>"${FAKE_LOG}"
case "$1" in
create)
	exit "${FAKE_SA_RC:-0}"
	;;
credential)
	if [[ -n ${FAKE_SA_KEY_ERROR:-} ]]; then
		echo "${FAKE_SA_KEY_ERROR}" >&2
		exit 1
	fi
	exit "${FAKE_SA_RC:-0}"
	;;
check)
	if [[ -z $2 ]]; then
		echo "Error: Must specify PROJECT_ID" >&2
		exit 99
	fi
	if [[ " ${FAKE_SA_UNCHECKABLE:-} " == *" $3 "* ]]; then
		echo "ERROR: (gcloud.iam.service-accounts.list) fake list failure" >&2
		exit "${FAKE_SA_CHECK_RC:-2}"
	fi
	[[ " ${FAKE_SA_EXISTS:-} " == *" $3 "* ]]
	exit
	;;
delete)
	exit "${FAKE_SA_DELETE_RC:-0}"
	;;
esac
exit 1
"""

PROV_ACCOUNT = f"{DEPLOYMENT}-prov-sa"
PROV_EMAIL = f"{PROV_ACCOUNT}@{PROJECT}.iam.gserviceaccount.com"
PROV_TFVARS_LINE = f'provisioning_service_account = "{PROV_EMAIL}"'
# How each deploy path calls service_account.sh create: the interactive one
# asks before granting the roles to an existing account with keys.
CONFIG_CREATE = ["create", PROJECT, PROV_ACCOUNT]
INTERACTIVE_CREATE = ["create", PROJECT, PROV_ACCOUNT, "--ask"]
PROV_DELETE_COMMAND = f"script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}"

# Every API deploy.sh requires, read from its PRJ_API list.
REQUIRED_APIS = sorted(
    set(re.findall(r"^PRJ_API\['([^']+)'\]", (OFE_DIR / "deploy.sh").read_text(), re.M))
)

ZONE = "europe-west4-a"
REGION = "europe-west4"
SERVER = f"{DEPLOYMENT}-server"
FE_ACCOUNT = f"{DEPLOYMENT}-fe-sa"
OLD_ACCOUNT = f"{DEPLOYMENT}-tkfe-sa"
# The Terraform state bucket the fake state bucket root reports
STATE_BUCKET = f"{DEPLOYMENT}-tfstate-0a1b2c3d"

DEPLOY_CONFIG = f"""\
deployment_name: {DEPLOYMENT}
project_id: {PROJECT}
zone: europe-west4-a
django_superuser_username: admin
django_superuser_email: admin@example.com
deployment_mode: git
"""


# A server deployed with the current code.
CURRENT_STATE = [NEW_ADDRESS, *NEW_GRANT_ADDRESSES, *WAITS, BUCKET_ADDRESS, OTHER_ADDRESS]


class FakeCommandTestCase(SimpleTestCase):
    """A scratch directory with a copy of the scripts and fake commands"""

    # teardown() runs with an empty state instead of CURRENT_STATE.
    keep_empty_state = False

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-deploy-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        for script in ("deploy.sh", "teardown.sh"):
            shutil.copy2(OFE_DIR / script, self.tmp / script)
        (self.tmp / "tf").mkdir()
        # The state bucket's Terraform files, which deploy.sh links into the
        # deployment's own directory under tf/state_bucket/deployments/
        shutil.copytree(OFE_DIR / "tf" / "state_bucket", self.tmp / "tf" / "state_bucket")
        # deploy.sh creates the provisioning account with this script, and
        # teardown.sh asks it whether the per-server account exists.
        (self.tmp / "script").mkdir()
        self._write_executable(self.tmp / "script" / "service_account.sh", SA_SCRIPT_FAKE)

        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        self._write_executable(self.bin / "terraform", TERRAFORM_FAKE)
        self._write_executable(self.bin / "gcloud", GCLOUD_FAKE)
        self._write_executable(self.bin / "sleep", SLEEP_FAKE)

        self.log = self.tmp / "calls.log"
        self.log.touch()
        self.state_list = self.tmp / "state_list"
        self.state_list.touch()
        self.policy = self.tmp / "policy"
        self.policy.touch()
        # Every required API is enabled unless a test says otherwise.
        self.services = self.tmp / "services"
        self.set_services(REQUIRED_APIS)
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_LOG=str(self.log),
            FAKE_STATE_LIST=str(self.state_list),
            FAKE_POLICY=str(self.policy),
            FAKE_SERVICES=str(self.services),
            FAKE_DEPLOYMENT=DEPLOYMENT,
            FAKE_STATE_UID="",
            FAKE_LIVE_UID="",
            FAKE_STATE_BUCKET=STATE_BUCKET,
            DJANGO_SUPERUSER_PASSWORD="not-a-real-password",
        )

    @staticmethod
    def _write_executable(path, content):
        path.write_text(content)
        path.chmod(0o755)

    def set_state(self, *addresses):
        self.state_list.write_text("".join(f"{address}\n" for address in addresses))

    def set_services(self, apis):
        self.services.write_text("".join(f"{api}\n" for api in apis))

    def set_policy(self, *rows):
        self.policy.write_text("".join(f"{role}\t{member}\n" for role, member in rows))

    def iam_errors(self, *messages):
        """The policy edits fail, in order, with these messages ("" succeeds)."""
        path = self.tmp / "iam_errors"
        path.write_text("".join(f"{message}\n" for message in messages))
        self.env["FAKE_IAM_ERRORS"] = str(path)

    def sleeps(self):
        return [call[0] for call in self.calls("sleep")]

    def removals(self):
        return [
            call for call in self.calls("gcloud", "projects")
            if call[1] == "remove-iam-policy-binding"
        ]

    @staticmethod
    def removal(role, member):
        return [
            "projects", "remove-iam-policy-binding", PROJECT,
            f"--member={member}", f"--role=roles/{role}", "--condition=None",
        ]

    def fail_first_apply(self, output):
        """Make the first apply print output and fail; later applies succeed."""
        path = self.tmp / "first_apply_output"
        path.write_text(output)
        self.env["FAKE_FIRST_APPLY_INCONSISTENT"] = str(path)

    def applied_tfvars(self):
        """The tfvars lines every apply read, in order."""
        path = Path(f"{self.log}.tfvars")
        return path.read_text().splitlines() if path.exists() else []

    def calls(self, program, command=None):
        found = []
        for line in self.log.read_text().splitlines():
            args = line.split("\x1f")
            if args[0] == program and (command is None or args[1:2] == [command]):
                found.append(args[1:])
        return found

    def run_script(self, *args, stdin_text=None):
        return subprocess.run(
            ["bash", *args],
            cwd=self.tmp,
            env=self.env,
            input=stdin_text,
            stdin=subprocess.DEVNULL if stdin_text is None else None,
            capture_output=True,
            text=True,
            check=False,
        )

    def deploy(self, config_text=DEPLOY_CONFIG):
        config = self.tmp / "config.yaml"
        config.write_text(config_text)
        return self.run_script(str(self.tmp / "deploy.sh"), "--config", str(config))

    def interactive_deploy(
        self, names=(DEPLOYMENT,), key_answer="", api_answers=(), proceed="y", grant_answer=None
    ):
        """Answer deploy.sh's questions, in the order it asks them.

        The user owns the project, so the account check asks nothing. The API
        check asks once per API missing from self.services; api_answers are
        its answers. grant_answer, when given, answers service_account.sh's
        question before it grants the roles to an existing account with keys.
        """
        user = "deployer@example.com"
        self.env.update(FAKE_CONFIG_ACCOUNT=user, FAKE_CONFIG_PROJECT=PROJECT)
        self.set_policy(("roles/owner", f"user:{user}"))
        answers = [
            *names,  # deployment name (re-asked while invalid)
            PROJECT,
            *api_answers,  # enable each missing API?
            "",  # zone: default
            "",  # subnet
            "",  # DNS hostname
            "",  # static IP
            "admin",
            "not-a-real-password",
            "not-a-real-password",
            "admin@example.com",
            *(() if grant_answer is None else (grant_answer,)),  # grant despite keys?
            key_answer,  # write a key file?
            "",  # deployment method: default (git)
            "",  # repository owner: default
            "",  # repository branch: default
            proceed,  # proceed to deploy
        ]
        return self.run_script(
            str(self.tmp / "deploy.sh"), stdin_text="".join(f"{a}\n" for a in answers)
        )

    def tfvars(self):
        return (self.tmp / "tf" / "terraform.tfvars").read_text().splitlines()

    def teardown(self, answers=None, extra_tfvars="", lock=True):
        """Run teardown.sh with -y, or, given answers, interactively.

        Unless the test set one, the state is that of a current server.
        Unless extra_tfvars sets a region or a zone, the tfvars hold both, as
        deploy.sh writes them. Given answers, a lock file is written first,
        unless lock is False.
        """
        if "region =" not in extra_tfvars and "zone =" not in extra_tfvars:
            extra_tfvars = f'region = "{REGION}"\nzone = "{ZONE}"\n{extra_tfvars}'
        (self.tmp / "tf" / "terraform.tfvars").write_text(
            f'project_id = "{PROJECT}"\ndeployment_name = "{DEPLOYMENT}"\n{extra_tfvars}'
        )
        if not self.state_list.read_text() and not self.keep_empty_state:
            self.set_state(*CURRENT_STATE)
        if answers is None:
            return self.run_script(str(self.tmp / "teardown.sh"), "-y")
        # With a lock file, the only questions are "Proceed?" and those after it.
        if lock:
            (self.tmp / "tf" / ".tkfe.lock").touch()
        return self.run_script(
            str(self.tmp / "teardown.sh"), stdin_text="".join(f"{a}\n" for a in answers)
        )

    def assert_no_state_show(self):
        """deploy.sh never decodes state with terraform state show"""
        shows = [call for call in self.calls("terraform", "state") if call[1:2] == ["show"]]
        self.assertEqual(shows, [])

    def run_function(self, function, *args, path=None):
        """Source deploy.sh and run one of its functions in tf/.

        path, when given, replaces PATH once deploy.sh is sourced.
        """
        env = dict(self.env)
        script = 'source "$1"; cd tf || exit 1; shift; '
        if path is not None:
            env["FUNCTION_PATH"] = path
            script += 'PATH="${FUNCTION_PATH}"; '
        script += '"$@"'
        return subprocess.run(
            ["bash", "-c", script, "bash", str(self.tmp / "deploy.sh"), function, *args],
            cwd=self.tmp,
            env=env,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            check=False,
        )

    def apply_flags(self):
        applies = self.calls("terraform", "apply")
        self.assertEqual(len(applies), 1, applies)
        return [arg for arg in applies[0] if arg.startswith("-replace=")]


class InitFailureTests(FakeCommandTestCase):
    """A failed terraform init stops each script with a readable message"""

    def setUp(self):
        super().setUp()
        self.env["FAKE_INIT_RC"] = "1"

    def test_deploy_stops(self):
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("terraform init failed", result.stderr)
        self.assertIn(str(self.tmp / "tf" / "tfinit.log"), result.stderr)
        self.assertIn("hashicorp/time", result.stderr)
        self.assertIn("fake init output", result.stderr)
        self.assertIn("Terraform has not changed any resource.", result.stderr)
        self.assertIn(PROV_EMAIL, result.stderr)
        self.assertEqual(self.calls("terraform", "apply"), [])
        self.assertEqual(self.calls("terraform", "import"), [])
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_teardown_stops(self):
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("terraform init failed", result.stderr)
        self.assertIn(str(self.tmp / "tf" / "tfinit.log"), result.stderr)
        self.assertEqual(self.calls("terraform", "state"), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])


class InitTests(FakeCommandTestCase):
    """Both scripts run init, with -upgrade, before anything else in tf/"""

    def test_deploy_runs_init_first(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        terraform_calls = self.calls("terraform")
        self.assertEqual(terraform_calls[0], ["init", "-input=false", "-upgrade"])

    def test_teardown_runs_init_before_reading_state_and_destroy(self):
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        commands = [call[0] for call in self.calls("terraform")]
        # Only read-only "state" commands: whether anything is deployed, the
        # account's old grants, and its unique id (from the raw state).
        # And a destroy plan, which changes nothing, before any grant removal.
        self.assertEqual(commands, ["init", "state", "plan", "state", "state", "destroy"])
        self.assertEqual(self.calls("terraform", "plan"), [["plan", "-destroy", "-input=false"]])
        self.assertEqual(
            self.calls("terraform", "state"),
            [["state", "list"], ["state", "list"], ["state", "pull"]],
        )
        self.assertEqual(self.calls("terraform", "init")[0], ["init", "-input=false", "-upgrade"])


class ReplaceWaitTests(FakeCommandTestCase):
    """-replace is added exactly when the account changed outside Terraform"""

    def test_first_deploy_has_no_replace(self):
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.apply_flags(), [])

    def test_matching_ids_have_no_replace(self):
        self.set_state(NEW_ADDRESS, *WAITS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.apply_flags(), [])
        self.assertNotIn("recreated outside Terraform", result.stderr)

    def test_different_ids_replace_both_waits(self):
        self.set_state(NEW_ADDRESS, *WAITS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="222")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.apply_flags(), REPLACE_FLAGS)
        self.assertIn("recreated outside Terraform", result.stderr)
        self.assertIn(["state", "pull"], self.calls("terraform", "state"))
        self.assert_no_state_show()

    def test_missing_account_replaces_both_waits(self):
        self.set_state(NEW_ADDRESS, *WAITS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="")
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.apply_flags(), REPLACE_FLAGS)

    def test_unreadable_state_id_replaces_both_waits(self):
        self.set_state(NEW_ADDRESS, *WAITS)
        self.env.update(FAKE_STATE_UID="", FAKE_LIVE_UID="222")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.apply_flags(), REPLACE_FLAGS)
        self.assertIn(f"Could not read the unique id of {EMAIL}", result.stderr)
        self.assertIn(f"The state holds {NEW_ADDRESS} with no unique_id.", result.stderr)

    def test_unreadable_state_replaces_both_waits_and_prints_the_error(self):
        self.set_state(NEW_ADDRESS, *WAITS)
        self.env.update(
            FAKE_LIVE_UID="111",
            FAKE_STATE_PULL_RC="1",
            FAKE_STATE_PULL_ERROR="Error: fake pull failure",
        )
        # Not reached: the bucket read stops the run first, so drive the
        # function on its own.
        result = self.run_function("fe_sa_replace_flags", EMAIL, PROJECT)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), REPLACE_FLAGS)
        self.assertIn("'terraform state pull' failed (exit status 1):", result.stderr)
        self.assertIn("Error: fake pull failure", result.stderr)

    def test_only_waits_in_state_are_replaced(self):
        self.set_state(NEW_ADDRESS, WAITS[0])
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="222")
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.apply_flags(), REPLACE_FLAGS[:1])

    def test_upgrade_compares_the_old_address(self):
        # A server deployed before the move: account at the old address, and
        # no waits yet, so they are created (and pause) anyway.
        self.set_state(OLD_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="222")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("recreated outside Terraform", result.stderr)
        self.assert_no_state_show()
        self.assertEqual(self.apply_flags(), [])


class PreImportTests(FakeCommandTestCase):
    """An existing account is imported only when no address holds it"""

    def test_imported_at_new_address_when_absent_from_state(self):
        self.env["FAKE_LIVE_UID"] = "111"
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(
            self.calls("terraform", "import"),
            [[
                "import",
                "-input=false",
                NEW_ADDRESS,
                f"projects/{PROJECT}/serviceAccounts/{EMAIL}",
            ]],
        )

    def test_not_imported_when_at_new_address(self):
        self.set_state(NEW_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111")
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.calls("terraform", "import"), [])

    def test_not_imported_when_at_old_address(self):
        # The moved block takes it from there; an import would fight it.
        self.set_state(OLD_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111")
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.calls("terraform", "import"), [])

    def test_not_imported_when_account_does_not_exist(self):
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.calls("terraform", "import"), [])

    def test_retry_not_imported_when_at_old_address(self):
        # An upgrade apply that fails with "inconsistent result": the retry
        # must not import either while the moved block holds the account.
        self.set_state(OLD_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111")
        # The moved block has moved it, so the error names the new address.
        self.fail_first_apply(inconsistent_result_output(NEW_ADDRESS))
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("terraform", "apply")), 2)
        self.assertEqual(self.calls("terraform", "import"), [])

    def test_retry_imports_when_account_dropped_from_state(self):
        # The case the retry exists for: the account was created, then
        # dropped from state, so it is at neither address.
        self.env["FAKE_LIVE_UID"] = "111"
        self.fail_first_apply(inconsistent_result_output(NEW_ADDRESS))
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("terraform", "apply")), 2)
        self.assertEqual(len(self.calls("terraform", "import")), 2)


class InconsistentResultRetryTests(FakeCommandTestCase):
    """The apply is retried only for the web server account's own error"""

    def setUp(self):
        super().setUp()
        self.set_state(NEW_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111")

    def test_retried_for_the_account_at_its_address(self):
        self.fail_first_apply(inconsistent_result_output(NEW_ADDRESS))
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("terraform", "apply")), 2)
        self.assertIn("Reconciling service account state", result.stdout)

    def test_retried_for_the_account_at_its_old_address(self):
        self.fail_first_apply(inconsistent_result_output(OLD_ADDRESS))
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("terraform", "apply")), 2)

    def test_retried_for_unwrapped_uncoloured_output(self):
        output = (
            "Error: Provider produced inconsistent result after apply\n\n"
            f"When applying changes to {NEW_ADDRESS}, provider produced an\n"
            "unexpected new value: Root object was present, but now absent.\n"
        )
        self.fail_first_apply(output)
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(len(self.calls("terraform", "apply")), 2)

    def test_not_retried_for_another_resource(self):
        # As in a live upgrade: the error is on the tarball object, and the
        # log names the account elsewhere (the refresh line, and the fe_profile
        # grant that failed after it).
        output = inconsistent_result_output(OTHER_ADDRESS) + (
            "Error: Error retrieving IAM policy for storage bucket: notFound\n"
            '  with module.fe_profile.google_storage_bucket_iam_member.this["control"],\n'
            "  on ../../../../community/modules/iam/service-account-profiles/main.tf\n"
        )
        self.fail_first_apply(output)
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)
        self.assertNotIn("Reconciling", result.stdout)
        self.assertIn("Terraform failed", result.stderr)
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_not_retried_for_an_address_that_only_starts_with_the_account(self):
        self.fail_first_apply(inconsistent_result_output(f"{NEW_ADDRESS}.x"))
        self.assertNotEqual(self.deploy().returncode, 0)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)

    def test_not_retried_for_another_error_naming_the_account(self):
        output = (
            "Error: Error creating service account: googleapi: Error 409\n"
            f"  with {NEW_ADDRESS},\n"
        )
        self.fail_first_apply(output)
        self.assertNotEqual(self.deploy().returncode, 0)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)


class ControlBucketLocationTests(FakeCommandTestCase):
    """A re-run keeps the control bucket where it is; a new one goes in region"""

    LINE = "control_bucket_location"

    def location_lines(self):
        return [line for line in self.tfvars() if line.startswith(self.LINE)]

    def test_rerun_keeps_a_us_bucket(self):
        # A server that an earlier version deployed with --config (no region).
        self.set_state(NEW_ADDRESS, BUCKET_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111", FAKE_BUCKET_LOCATION="US")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), ['control_bucket_location = "US"'])
        # The apply read it.
        self.assertIn('control_bucket_location = "US"', self.applied_tfvars())
        self.assertIn(["state", "pull"], self.calls("terraform", "state"))
        self.assert_no_state_show()
        self.assertIn("Keeping the existing control bucket in its location, US.", result.stdout)

    def test_upgrade_reads_the_old_address(self):
        # A server deployed before the bucket module call was renamed.
        self.set_state(OLD_ADDRESS, OLD_BUCKET_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111", FAKE_BUCKET_LOCATION="US")
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), ['control_bucket_location = "US"'])
        self.assert_no_state_show()

    def test_rerun_keeps_a_regional_bucket(self):
        self.set_state(BUCKET_ADDRESS)
        self.env["FAKE_BUCKET_LOCATION"] = "EUROPE-WEST4"
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.location_lines(), ['control_bucket_location = "EUROPE-WEST4"'])

    def test_interactive_rerun_keeps_the_bucket(self):
        self.set_state(BUCKET_ADDRESS)
        self.env["FAKE_BUCKET_LOCATION"] = "US"
        result = self.interactive_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), ['control_bucket_location = "US"'])

    def test_new_deployment_writes_none(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), [])
        self.assertEqual(len(self.calls("terraform", "apply")), 1)
        bucket_shows = [
            call for call in self.calls("terraform", "state") if "google_storage_bucket" in call[-1]
        ]
        self.assertEqual(bucket_shows, [])

    def test_no_state_is_a_new_deployment(self):
        # terraform state pull prints nothing when there is no state.
        self.env["FAKE_STATE_SHOW_RC"] = "1"
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(["state", "pull"], self.calls("terraform", "state"))
        self.assertEqual(self.location_lines(), [])
        self.assertEqual(len(self.calls("terraform", "apply")), 1)
        self.assert_no_state_show()

    def test_state_without_a_bucket_is_a_new_bucket(self):
        # As after a destroy that failed part-way, once it deleted the bucket.
        self.set_state(OTHER_ADDRESS)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), [])
        self.assertEqual(len(self.calls("terraform", "apply")), 1)

    def assert_stopped_before_apply(self, result, message):
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(message, result.stderr)
        self.assertIn("Could not read the control bucket's location", result.stderr)
        self.assertIn("replace the bucket and delete its files.", result.stderr)
        # What the run changed before it stopped.
        self.assertIn("Terraform has not changed any resource.", result.stderr)
        self.assertIn(PROV_EMAIL, result.stderr)
        self.assertIn("running deploy.sh again reuses it.", result.stderr)
        self.assertIn("do not run terraform apply there by hand.", result.stderr)
        self.assertEqual(self.calls("terraform", "apply"), [])
        self.assertEqual(self.calls("terraform", "import"), [])
        self.assertEqual(self.location_lines(), [])
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_unreadable_location_stops_before_apply(self):
        self.set_state(NEW_ADDRESS, BUCKET_ADDRESS)
        self.env.update(FAKE_STATE_UID="111", FAKE_LIVE_UID="111", FAKE_BUCKET_LOCATION="")
        self.assert_stopped_before_apply(
            self.deploy(), f"The state holds {BUCKET_ADDRESS} with no location."
        )

    def test_unexpected_location_text_stops_before_apply(self):
        self.set_state(BUCKET_ADDRESS)
        # Never written to tfvars unless it looks like a location.
        self.env["FAKE_BUCKET_LOCATION"] = "US;x"
        self.assert_stopped_before_apply(
            self.deploy(), "The state holds an unexpected location for the bucket: 'US;x'."
        )

    def test_unreadable_state_stops_before_apply(self):
        self.env.update(
            FAKE_STATE_PULL_RC="1", FAKE_STATE_PULL_ERROR="Failed to load state: fake error"
        )
        result = self.deploy()
        self.assert_stopped_before_apply(
            result, "'terraform state pull' failed (exit status 1):"
        )
        self.assertIn("Failed to load state: fake error", result.stderr)


class OlderProviderStateTests(FakeCommandTestCase):
    """Values are read from the state as stored, not decoded by a provider

    The fixture is the state of a server deployed by an earlier version with
    Google provider 4.x (schema version 0 resources, old addresses), with
    identifying values replaced. After terraform init -upgrade installs the
    7.x provider, terraform state show cannot decode such a resource, so the
    fake's state show fails throughout, as it did live.
    """

    FIXTURE = Path(__file__).parent / "fixtures" / "terraform_state" / "google4_old_addresses.json"
    FIXTURE_UID = "100000000000000000001"
    # Where the current tf/main.tf moves the bucket and the account to.
    MOVES = {
        ("module.control_bucket", "google_storage_bucket"): "module.control_storage",
        ("module.service_account", "google_service_account"): "module.fe_sa.module.service_account",
    }

    def setUp(self):
        super().setUp()
        self.env["FAKE_STATE_SHOW_RC"] = "1"

    def use_state(self, state, *extra_addresses):
        """Make state (a dict) the stored state, and list its addresses."""
        path = self.tmp / "state.json"
        path.write_text(json.dumps(state, indent=2))
        self.env["FAKE_STATE_JSON"] = str(path)
        addresses = []
        for resource in state["resources"]:
            prefix = f"{resource['module']}." if "module" in resource else ""
            for instance in resource["instances"]:
                key = f"[{json.dumps(instance['index_key'])}]" if "index_key" in instance else ""
                addresses.append(f"{prefix}{resource['type']}.{resource['name']}{key}")
        self.set_state(*addresses, *extra_addresses)
        return addresses

    def fixture(self, moved=False):
        state = json.loads(self.FIXTURE.read_text())
        if moved:
            for resource in state["resources"]:
                module = self.MOVES.get((resource.get("module"), resource["type"]))
                if module:
                    resource["module"] = module
        return state

    def location_lines(self):
        return [line for line in self.tfvars() if line.startswith("control_bucket_location")]

    def assert_kept_in_us(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.location_lines(), ['control_bucket_location = "US"'])
        self.assertIn('control_bucket_location = "US"', self.applied_tfvars())
        self.assertIn("Keeping the existing control bucket in its location, US.", result.stdout)
        self.assertIn(["state", "pull"], self.calls("terraform", "state"))
        self.assert_no_state_show()

    def test_location_read_at_the_old_address(self):
        addresses = self.use_state(self.fixture())
        self.assertIn(OLD_BUCKET_ADDRESS, addresses)
        self.assertIn(OLD_ADDRESS, addresses)
        self.env["FAKE_LIVE_UID"] = self.FIXTURE_UID
        self.assert_kept_in_us(self.deploy())

    def test_location_read_at_the_new_address(self):
        addresses = self.use_state(self.fixture(moved=True))
        self.assertIn(BUCKET_ADDRESS, addresses)
        self.assertNotIn(OLD_BUCKET_ADDRESS, addresses)
        self.env["FAKE_LIVE_UID"] = self.FIXTURE_UID
        self.assert_kept_in_us(self.deploy())

    def test_location_read_from_flat_attributes(self):
        # A state that Terraform 0.12 upgraded from a 0.11 one.
        state = self.fixture()
        for resource in state["resources"]:
            if resource["type"] == "google_storage_bucket":
                for instance in resource["instances"]:
                    del instance["attributes"]
                    instance["attributes_flat"] = {"location": "US", "name": "x"}
        self.use_state(state)
        self.env["FAKE_LIVE_UID"] = self.FIXTURE_UID
        self.assert_kept_in_us(self.deploy())

    def test_unique_id_read_at_the_old_address(self):
        self.use_state(self.fixture())
        self.env["FAKE_LIVE_UID"] = self.FIXTURE_UID
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("recreated outside Terraform", result.stderr)
        self.assertNotIn("Could not read the unique id", result.stderr)
        self.assertEqual(self.apply_flags(), [])
        self.assert_no_state_show()

    def test_changed_unique_id_found_at_the_old_address(self):
        self.use_state(self.fixture())
        self.env["FAKE_LIVE_UID"] = "222"
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("recreated outside Terraform", result.stderr)
        self.assertNotIn("Could not read the unique id", result.stderr)

    def test_unique_id_read_at_the_new_address(self):
        self.use_state(self.fixture(moved=True), *WAITS)
        self.env["FAKE_LIVE_UID"] = self.FIXTURE_UID
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("recreated outside Terraform", result.stderr)
        self.assertEqual(self.apply_flags(), [])
        self.assert_no_state_show()

    def test_changed_unique_id_at_the_new_address_replaces_the_waits(self):
        self.use_state(self.fixture(moved=True), *WAITS)
        self.env["FAKE_LIVE_UID"] = "222"
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.apply_flags(), REPLACE_FLAGS)

    def assert_stopped_with(self, result, message):
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Could not read the control bucket's location", result.stderr)
        self.assertIn(message, result.stderr)
        self.assertIn("Terraform has not changed any resource.", result.stderr)
        self.assertIn(PROV_EMAIL, result.stderr)
        self.assertEqual(self.calls("terraform", "apply"), [])
        self.assertEqual(self.calls("terraform", "import"), [])
        self.assertEqual(self.location_lines(), [])

    def test_malformed_state_stops_before_apply(self):
        self.use_state(self.fixture())
        # Cut off part-way, as a damaged file would be.
        path = Path(self.env["FAKE_STATE_JSON"])
        path.write_text(path.read_text()[:500])
        self.assert_stopped_with(self.deploy(), "The state is not valid JSON:")

    def test_state_that_is_not_terraform_state_stops_before_apply(self):
        self.use_state(self.fixture())
        Path(self.env["FAKE_STATE_JSON"]).write_text("[]\n")
        self.assert_stopped_with(self.deploy(), "The state has no list of resources.")

    def test_never_runs_state_show(self):
        commands = [
            line for line in (OFE_DIR / "deploy.sh").read_text().splitlines()
            if not line.lstrip().startswith("#")
        ]
        self.assertEqual([line for line in commands if "state show" in line], [])

    def restricted_path(self):
        """A PATH with the fakes and the few commands the reader runs, but no python3"""
        path = self.tmp / "restricted"
        path.mkdir()
        for name in ("terraform", "gcloud"):
            (path / name).symlink_to(self.bin / name)
        for name in ("mktemp", "rm", "cat"):
            (path / name).symlink_to(shutil.which(name))
        return str(path)

    def test_reads_with_the_python_gcloud_uses_without_python3(self):
        self.use_state(self.fixture())
        self.env["FAKE_GCLOUD_PYTHON"] = sys.executable
        result = self.run_function(
            "tf_state_attribute", "location", BUCKET_ADDRESS, OLD_BUCKET_ADDRESS,
            path=self.restricted_path(),
        )
        self.assertEqual((result.returncode, result.stdout), (0, "US\n"), result.stderr)
        self.assertEqual(
            self.calls("gcloud", "info"), [["info", "--format=value(basic.python_location)"]]
        )

    def test_no_python_is_an_error(self):
        self.use_state(self.fixture())
        result = self.run_function(
            "tf_state_attribute", "location", BUCKET_ADDRESS, path=self.restricted_path()
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("needs python3", result.stderr)

    def test_no_state_needs_no_python(self):
        result = self.run_function(
            "tf_state_attribute", "location", BUCKET_ADDRESS, path=self.restricted_path()
        )
        self.assertEqual((result.returncode, result.stdout, result.stderr), (3, "", ""))


class SupersededRoleTests(FakeCommandTestCase):
    """After a successful apply, only the superseded roles the account holds go"""

    def test_removes_only_held_superseded_roles(self):
        self.set_policy(
            ("roles/logging.logWriter", MEMBER),
            ("roles/monitoring.metricWriter", MEMBER),
            ("roles/pubsub.admin", MEMBER),
            ("roles/storage.objectAdmin", MEMBER),
            ("roles/secretmanager.admin", "serviceAccount:someone-else@example.com"),
            ("roles/artifactregistry.admin", f"{MEMBER}.other"),
        )
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertCountEqual(
            self.removals(),
            [
                [
                    "projects", "remove-iam-policy-binding", PROJECT,
                    f"--member={MEMBER}", f"--role=roles/{role}", "--condition=None",
                ]
                for role in ("pubsub.admin", "storage.objectAdmin")
            ],
        )
        self.assertNotIn("Warning", result.stderr)

    def test_new_server_removes_nothing(self):
        self.set_policy(("roles/logging.logWriter", MEMBER))
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.removals(), [])

    def test_unreadable_policy_warns_with_commands(self):
        self.env["FAKE_POLICY_RC"] = "1"
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.removals(), [])
        self.assertIn("Could not check or remove", result.stderr)
        for role in SUPERSEDED:
            self.assertIn(
                f"gcloud projects remove-iam-policy-binding {PROJECT} "
                f"--member={MEMBER} --role=roles/{role} --condition=None",
                result.stderr,
            )

    def test_not_run_when_apply_fails(self):
        self.env["FAKE_APPLY_RC"] = "1"
        self.set_policy(("roles/pubsub.admin", MEMBER))
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls("gcloud", "projects"), [])

    def test_removal_retried_on_transient_errors(self):
        self.set_policy(("roles/pubsub.admin", MEMBER), ("roles/storage.objectAdmin", MEMBER))
        self.iam_errors(CONFLICT, DELETED_MEMBER_ERROR, "", CONFLICT)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            [call[4] for call in self.removals()],
            ["--role=roles/storage.objectAdmin"] * 3 + ["--role=roles/pubsub.admin"] * 2,
        )
        self.assertEqual(self.sleeps(), ["2", "4", "2"])
        self.assertNotIn("Warning", result.stderr)

    def test_policy_read_retried_on_the_per_minute_quota(self):
        self.set_policy(("roles/pubsub.admin", MEMBER))
        self.env.update(
            FAKE_READ_CMD="policy", FAKE_READ_FAILS="1", FAKE_READ_ERROR=QUOTA_PER_MINUTE
        )
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.sleeps(), ["2"])
        self.assertEqual(
            [call[4] for call in self.removals()], ["--role=roles/pubsub.admin"]
        )

    def test_failed_policy_read_prints_gclouds_error(self):
        self.set_policy(("roles/pubsub.admin", MEMBER))
        self.env.update(FAKE_READ_CMD="policy", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.deploy()
        self.assertEqual(self.removals(), [])
        self.assertEqual(self.sleeps(), [])
        self.assertIn(DENIED, result.stderr)
        self.assertIn(
            f"gcloud projects remove-iam-policy-binding {PROJECT} "
            f"--member={MEMBER} --role=roles/pubsub.admin --condition=None",
            result.stderr,
        )

    def test_removal_retried_on_the_per_minute_quota(self):
        self.set_policy(("roles/pubsub.admin", MEMBER), ("roles/storage.objectAdmin", MEMBER))
        self.iam_errors(QUOTA_PER_MINUTE, "", QUOTA_PER_MINUTE)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            [call[4] for call in self.removals()],
            ["--role=roles/storage.objectAdmin"] * 2 + ["--role=roles/pubsub.admin"] * 2,
        )
        self.assertEqual(self.sleeps(), ["2", "2"])
        self.assertNotIn("Warning", result.stderr)

    def test_retries_are_bounded_and_the_rest_listed(self):
        self.set_policy(("roles/pubsub.admin", MEMBER), ("roles/storage.objectAdmin", MEMBER))
        self.env["FAKE_IAM_ERROR_ALWAYS"] = DELETED_MEMBER_ERROR
        result = self.deploy()
        # The server works without the removal, so the deployment succeeds.
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.removals()), len(RETRY_DELAYS) + 1)
        self.assertEqual(self.sleeps(), RETRY_DELAYS)
        self.assertIn("is not deleted", result.stderr)
        for role in ("pubsub.admin", "storage.objectAdmin"):
            self.assertIn(
                f"gcloud projects remove-iam-policy-binding {PROJECT} "
                f"--member={MEMBER} --role=roles/{role} --condition=None",
                result.stderr,
            )
        self.assertTrue((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_other_errors_not_retried(self):
        self.set_policy(("roles/pubsub.admin", MEMBER))
        self.iam_errors(DENIED)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.removals()), 1)
        self.assertEqual(self.sleeps(), [])
        self.assertIn(DENIED, result.stderr)
        self.assertIn("Could not check or remove", result.stderr)


class TeardownOldGrantTests(FakeCommandTestCase):
    """teardown.sh removes, before the destroy, the old project grants it would leave"""

    def assert_removed_before_destroy(self):
        log = self.log.read_text()
        self.assertLess(log.rindex("remove-iam-policy-binding"), log.index("terraform\x1fdestroy"))

    def test_old_state_removes_only_held_old_roles(self):
        self.set_state(OLD_ADDRESS, *OLD_GRANT_ADDRESSES)
        self.env["FAKE_STATE_UID"] = OLD_UID
        self.set_policy(
            ("roles/pubsub.admin", MEMBER),
            ("roles/logging.logWriter", MEMBER),
            # Left by an earlier, partial teardown of the same account.
            ("roles/storage.objectAdmin", DELETED_MEMBER),
            # Not an old role, and not this account.
            ("roles/compute.viewer", MEMBER),
            ("roles/secretmanager.admin", "serviceAccount:someone-else@example.com"),
            ("roles/cloudtrace.agent", f"deleted:serviceAccount:{EMAIL}?uid=999"),
        )
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertCountEqual(
            self.removals(),
            [
                self.removal("pubsub.admin", MEMBER),
                self.removal("logging.logWriter", MEMBER),
                self.removal("storage.objectAdmin", DELETED_MEMBER),
            ],
        )
        self.assertNotIn("Warning", result.stderr)
        # Removed while the account still exists, before the destroy deletes it.
        self.assert_removed_before_destroy()
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)

    def test_old_grants_without_account_in_state_use_live_unique_id(self):
        self.set_state(*OLD_GRANT_ADDRESSES)
        self.env["FAKE_LIVE_UID"] = OLD_UID
        self.set_policy(("roles/pubsub.admin", DELETED_MEMBER))
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.removals(), [self.removal("pubsub.admin", DELETED_MEMBER)])

    def test_upgraded_server_loses_the_roles_its_upgrade_forgot(self):
        # The m7o case: an upgrade apply forgot the old grants (removed
        # block) and then failed, so deploy.sh never removed the superseded
        # ones; the state holds the account and its two new grants only.
        self.set_state(NEW_ADDRESS, *NEW_GRANT_ADDRESSES, *WAITS, BUCKET_ADDRESS)
        self.env["FAKE_STATE_UID"] = OLD_UID
        self.set_policy(
            *((f"roles/{role}", MEMBER) for role in OLD_ROLES),
            ("roles/pubsub.admin", "serviceAccount:someone-else@example.com"),
        )
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        # The two roles the state manages are left for the destroy.
        self.assertCountEqual(self.removals(), [self.removal(role, MEMBER) for role in SUPERSEDED])
        self.assert_removed_before_destroy()
        # The uid comes from the raw state, never from terraform state show.
        state_calls = self.calls("terraform", "state")
        self.assertIn(["state", "pull"], state_calls)
        self.assertEqual([call for call in state_calls if call[:2] == ["state", "show"]], [])

    def test_unique_id_read_when_state_show_cannot_decode_the_state(self):
        # A state stored by the older provider: terraform state show fails
        # on it (as it did live), so the uid must come from the raw state.
        self.set_state(OLD_ADDRESS, *OLD_GRANT_ADDRESSES)
        self.env.update(FAKE_STATE_UID=OLD_UID, FAKE_STATE_SHOW_RC="1", FAKE_LIVE_UID="")
        self.set_policy(("roles/storage.objectAdmin", DELETED_MEMBER))
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(self.removal("storage.objectAdmin", DELETED_MEMBER), self.removals())

    def test_current_server_without_old_roles_removes_nothing(self):
        self.set_state(*CURRENT_STATE)
        self.set_policy(*((f"roles/{role}", MEMBER) for role in ("logging.logWriter", "monitoring.metricWriter")))
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.removals(), [])
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)

    def test_removal_retried_on_transient_errors(self):
        self.set_state(NEW_ADDRESS, *NEW_GRANT_ADDRESSES)
        self.set_policy(("roles/pubsub.admin", MEMBER), ("roles/storage.objectAdmin", MEMBER))
        self.iam_errors(DELETED_MEMBER_ERROR, CONFLICT, "", DELETED_MEMBER_ERROR)
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            [call[4] for call in self.removals()],
            ["--role=roles/storage.objectAdmin"] * 3 + ["--role=roles/pubsub.admin"] * 2,
        )
        self.assertEqual(self.sleeps(), ["2", "4", "2"])
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)

    def test_failed_removal_stops_before_destroy(self):
        self.set_state(NEW_ADDRESS, *NEW_GRANT_ADDRESSES)
        self.set_policy(("roles/pubsub.admin", MEMBER), ("roles/storage.objectAdmin", MEMBER))
        self.env["FAKE_IAM_ERROR_ALWAYS"] = CONFLICT
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls("terraform", "destroy"), [])
        # Bounded: one role tried through every retry, the other not tried.
        self.assertEqual(len(self.removals()), len(RETRY_DELAYS) + 1)
        self.assertEqual(self.sleeps(), RETRY_DELAYS)
        self.assertIn("nothing was destroyed", result.stdout)
        for role in ("pubsub.admin", "storage.objectAdmin"):
            self.assertIn(
                f"gcloud projects remove-iam-policy-binding {PROJECT} "
                f"--member={MEMBER} --role=roles/{role} --condition=None",
                result.stderr,
            )

    def test_removal_not_retried_for_other_errors(self):
        self.set_state(NEW_ADDRESS, *NEW_GRANT_ADDRESSES)
        self.set_policy(("roles/pubsub.admin", MEMBER))
        self.iam_errors(DENIED)
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.removals()), 1)
        self.assertEqual(self.sleeps(), [])
        self.assertIn(DENIED, result.stderr)
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_destroy_that_cannot_start_keeps_the_old_roles(self):
        # The m7t case: Terraform cannot configure the provider, so the
        # destroy would fail; the old roles must stay on the running server.
        terraform = self.bin / "terraform"
        terraform.write_text(
            terraform.read_text().replace(
                'esac\nexit 0\n', 'plan)\n\techo "region was set to \\`\\`" >&2\n\texit 1\n\t;;\nesac\nexit 0\n'
            )
        )
        self.set_state(OLD_ADDRESS, *OLD_GRANT_ADDRESSES)
        self.env["FAKE_STATE_UID"] = OLD_UID
        self.set_policy(("roles/pubsub.admin", MEMBER))
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.removals(), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertIn("nothing was changed or destroyed", result.stdout)
        self.assertIn("region was set to", result.stderr)
        self.assertNotIn("Completed teardown", result.stdout)

    def test_unreadable_policy_stops_before_destroy(self):
        self.set_state(OLD_ADDRESS, *OLD_GRANT_ADDRESSES)
        self.env.update(FAKE_STATE_UID=OLD_UID, FAKE_POLICY_RC="1")
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.removals(), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertIn("Could not check or remove", result.stderr)
        for role in OLD_ROLES:
            for member in (MEMBER, DELETED_MEMBER):
                self.assertIn(
                    f"gcloud projects remove-iam-policy-binding {PROJECT} "
                    f"--member={member} --role=roles/{role} --condition=None",
                    result.stderr,
                )
        self.assertNotIn("Completed teardown", result.stdout)


class TeardownStateTests(FakeCommandTestCase):
    """teardown.sh destroys whatever the state holds, labelled or not"""

    keep_empty_state = True

    def test_unlabelled_resources_are_still_destroyed(self):
        # What a destroy that failed part-way leaves: no server, so no label.
        self.env["FAKE_DEPLOYMENT"] = ""
        self.set_state(
            "module.network[0].google_compute_network.hosting_vpc",
            "module.network[0].google_compute_subnetwork.hosting_subnetwork",
        )
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("No terraform deployment found", result.stdout)
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertIn("Completed teardown", result.stdout)

    def test_only_the_account_and_its_grants_are_still_destroyed(self):
        # The m7o3 case: state held only the account and its project grants.
        self.env["FAKE_DEPLOYMENT"] = ""
        self.set_state(OLD_ADDRESS, *OLD_GRANT_ADDRESSES)
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)

    def test_empty_state_reports_no_deployment(self):
        # Nothing to destroy, no server and no account left: a finished teardown.
        self.env["FAKE_SERVER_VM"] = "absent"
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_unreadable_state_destroys_nothing(self):
        self.set_state(OTHER_ADDRESS)
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="Failed to load state")
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertEqual(self.removals(), [])


class ProvisioningAccountTests(FakeCommandTestCase):
    """Both deploy paths create the keyless provisioning account the same way"""

    def account_calls(self):
        """service_account.sh calls other than the checks deploy_state_bucket
        makes to choose the state bucket's writers"""
        return [call for call in self.calls("service_account.sh") if call[0] != "check"]

    def assert_created_without_key(self, result, create_call):
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertEqual(self.account_calls(), [create_call])
        self.assertIn(PROV_TFVARS_LINE, self.tfvars())
        self.assertFalse((self.tmp / "credential.json").exists())

    def test_config_path_creates_account_and_writes_tfvars(self):
        self.assert_created_without_key(self.deploy(), CONFIG_CREATE)

    def test_the_account_writes_the_state_bucket_the_server_is_given(self):
        # The account exists once create has run
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn(f'terraform_state_bucket = "{STATE_BUCKET}"', self.tfvars())
        bucket_tfvars = (
            self.tmp / "tf" / "state_bucket" / "deployments" / DEPLOYMENT / "terraform.tfvars"
        ).read_text()
        self.assertIn(f'default_writers = ["serviceAccount:{PROV_EMAIL}"]', bucket_tfvars)
        # The bucket's writers are chosen after the account is created
        log = self.log.read_text()
        self.assertLess(
            log.index("service_account.sh\x1fcreate"), log.index("service_account.sh\x1fcheck")
        )

    def assert_uncheckable_writer_stops_before_the_bucket(self, account, check_rc="2"):
        # A failed check is not an absent account: default_writers is
        # authoritative, so applying without the account would take away its
        # access to the state. Only exit status 1 means absent.
        self.env.update(
            FAKE_SA_EXISTS=PROV_ACCOUNT, FAKE_SA_UNCHECKABLE=account, FAKE_SA_CHECK_RC=check_rc
        )
        result = self.deploy()
        self.assertEqual(result.returncode, 1, result.stderr + result.stdout)
        email = f"{account}@{PROJECT}.iam.gserviceaccount.com"
        self.assertIn(f"could not check whether the service account {email}", result.stdout)
        self.assertIn(f"service_account.sh check exited with status {check_rc}", result.stdout)
        # The check's own error is shown, before the message, not thrown away
        self.assertIn(
            "ERROR: (gcloud.iam.service-accounts.list) fake list failure\n"
            f"Error: could not check whether the service account {email}",
            result.stdout,
        )
        self.assertIn(f"Nothing was changed in the state bucket: gs://{STATE_BUCKET}", result.stdout)
        self.assertIn("run deploy.sh again", result.stdout)
        self.assertNotIn(f"Note: {email} does not exist", result.stdout)
        bucket_log = Path(f"{self.log}.state_bucket").read_text()
        self.assertEqual(
            [line for line in bucket_log.splitlines() if line.split("\x1f")[2:3] == ["apply"]],
            [],
        )
        self.assertFalse(
            (self.tmp / "tf" / "state_bucket" / "deployments" / DEPLOYMENT / "terraform.tfvars")
            .exists()
        )
        self.assertEqual(self.calls("terraform"), [])

    def test_provisioning_account_that_cannot_be_checked_stops_the_deploy(self):
        self.assert_uncheckable_writer_stops_before_the_bucket(PROV_ACCOUNT)

    def test_old_account_that_cannot_be_checked_stops_the_deploy(self):
        self.assert_uncheckable_writer_stops_before_the_bucket(f"{DEPLOYMENT}-tkfe-sa")

    def test_check_that_exits_99_is_not_an_absent_account(self):
        # 99: a missing or invalid argument
        self.assert_uncheckable_writer_stops_before_the_bucket(PROV_ACCOUNT, check_rc="99")

    def test_check_that_exits_127_is_not_an_absent_account(self):
        # 127: a command that could not be found, such as gcloud
        self.assert_uncheckable_writer_stops_before_the_bucket(
            f"{DEPLOYMENT}-tkfe-sa", check_rc="127"
        )

    def test_check_killed_by_a_signal_is_not_an_absent_account(self):
        # 143: killed by SIGTERM
        self.assert_uncheckable_writer_stops_before_the_bucket(PROV_ACCOUNT, check_rc="143")

    def test_interactive_path_creates_account_and_writes_tfvars(self):
        result = self.interactive_deploy()
        self.assert_created_without_key(result, INTERACTIVE_CREATE)
        # The account is created before Terraform runs.
        log = self.log.read_text()
        self.assertLess(log.index("service_account.sh\x1fcreate"), log.index("terraform\x1fapply"))

    def test_both_paths_give_the_same_result(self):
        self.assertEqual(self.deploy().returncode, 0)
        config_calls = self.account_calls()
        config_line = [line for line in self.tfvars() if "provisioning_service_account" in line]

        self.log.write_text("")
        (self.tmp / "tf" / ".tkfe.lock").unlink()
        self.assertEqual(self.interactive_deploy().returncode, 0)
        # The same create, except that only the interactive path can ask
        # before granting the roles to an existing account with keys.
        self.assertEqual(config_calls, [CONFIG_CREATE])
        self.assertEqual(self.account_calls(), [INTERACTIVE_CREATE])
        self.assertEqual(
            [line for line in self.tfvars() if "provisioning_service_account" in line],
            config_line,
        )
        self.assertEqual(config_line, [PROV_TFVARS_LINE])

    def test_interactive_key_only_when_asked(self):
        result = self.interactive_deploy(key_answer="y")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            self.account_calls(),
            [
                INTERACTIVE_CREATE,
                ["credential", PROJECT, PROV_ACCOUNT, str(self.tmp / "credential.json")],
            ],
        )
        self.assertIn("written in plain text", result.stdout)
        # The key changes nothing for the server: it still impersonates.
        self.assertIn(PROV_TFVARS_LINE, self.tfvars())

    def test_interactive_key_declined(self):
        result = self.interactive_deploy(key_answer="n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.account_calls(), [INTERACTIVE_CREATE])

    def test_interactive_rejects_a_name_too_long_for_the_account(self):
        long_name = "a" * 23  # 23 + len("-prov-sa") = 31 > 30
        result = self.interactive_deploy(names=(long_name, DEPLOYMENT))
        self.assertEqual(result.returncode, 0, result.stderr)
        # validate_deployment_name's rules, which hold this limit
        self.assertIn(f"Deployment name '{long_name}' is invalid", result.stderr)
        self.assertIn("Be 3 to 22 characters long", result.stderr)
        self.assertEqual(self.account_calls(), [INTERACTIVE_CREATE])

    def test_failed_create_stops_before_terraform(self):
        self.env["FAKE_SA_RC"] = "1"
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(f"Could not create the provisioning service account {PROV_ACCOUNT}", result.stderr)
        self.assertEqual(self.calls("terraform"), [])
        self.assertFalse((self.tmp / "tf" / "terraform.tfvars").exists())

    def test_failed_key_write_prints_the_error_and_the_delete_command(self):
        # By then the account exists, with its project roles.
        key_refused = (
            "ERROR: (gcloud.iam.service-accounts.keys.create) FAILED_PRECONDITION: "
            "Key creation is not allowed on this service account."
        )
        self.env["FAKE_SA_KEY_ERROR"] = key_refused
        result = self.interactive_deploy(key_answer="y")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(key_refused, result.stderr)
        self.assertIn(f"The account {PROV_ACCOUNT} exists, with its project roles.", result.stderr)
        self.assertIn(f"script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}", result.stderr)
        self.assertIn("Exiting.", result.stdout)
        self.assertEqual(self.calls("terraform"), [])

    def test_interactive_no_at_proceed_names_the_account(self):
        # The account exists by then, with its project roles.
        result = self.interactive_deploy(proceed="n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.account_calls(), [INTERACTIVE_CREATE])
        self.assertIn(f"{PROV_EMAIL} exists, with its project roles.", result.stdout)
        self.assertIn("./teardown.sh", result.stdout)
        self.assertIn(f"script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}", result.stdout)
        self.assertIn("Exiting.", result.stdout)
        self.assertEqual(self.calls("terraform"), [])
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_interactive_no_at_proceed_gives_the_delete_command_first(self):
        # No lock file is written, so teardown.sh first warns that there is
        # likely no FrontEnd: the command that deletes the account comes first.
        result = self.interactive_deploy(proceed="n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(
            "To delete it and its project\n    roles:\n"
            f"      {PROV_DELETE_COMMAND}\n",
            result.stdout,
        )
        self.assertLess(result.stdout.index(PROV_DELETE_COMMAND), result.stdout.index("./teardown.sh"))
        self.assertNotIn("run ./teardown.sh, or:", result.stdout)

    def test_config_name_too_long_is_explained(self):
        long_name = "a" * 23
        self.env["FAKE_SA_RC"] = "99"
        result = self.deploy(DEPLOY_CONFIG.replace(DEPLOYMENT, long_name))
        self.assertNotEqual(result.returncode, 0)
        # validate_deployment_name stops it before any API or account change
        self.assertIn(f"Deployment name '{long_name}' is invalid", result.stderr)
        self.assertIn("Be 3 to 22 characters long", result.stderr)
        self.assertEqual(self.calls("service_account.sh"), [])
        self.assertEqual(self.calls("terraform"), [])


PROV_KEY_IDS = ["0123456789abcdef", "fedcba9876543210"]
PROV_KEY_NAMES = "".join(
    f"projects/{PROJECT}/serviceAccounts/{PROV_EMAIL}/keys/{key_id}\n" for key_id in PROV_KEY_IDS
)
GRANT_QUESTION = "Continue and grant the roles to this account? [y/N]"


class ProvisioningAccountKeyTests(FakeCommandTestCase):
    """Reusing a provisioning account that has user-managed keys is never
    silent: the interactive path asks, the --config path warns and goes on.

    These run the real script/service_account.sh against the fake gcloud.
    """

    def setUp(self):
        super().setUp()
        shutil.copy2(
            OFE_DIR / "script" / "service_account.sh", self.tmp / "script" / "service_account.sh"
        )
        self.env.update(FAKE_EXISTING_ACCOUNTS=PROV_EMAIL, FAKE_USER_KEYS=PROV_KEY_NAMES)

    def grants(self):
        return [
            call for call in self.calls("gcloud", "projects")
            if call[1] == "add-iam-policy-binding"
        ]

    def assert_keys_warned(self, result):
        output = " ".join(result.stderr.split())
        self.assertIn(f"Warning: {PROV_EMAIL} already has user-managed keys:", output)
        for key_id in PROV_KEY_IDS:
            self.assertIn(
                f"gcloud iam service-accounts keys delete {key_id} --iam-account={PROV_EMAIL}",
                result.stderr,
            )

    def test_interactive_reuse_with_keys_asks_and_no_grants_nothing(self):
        result = self.interactive_deploy(grant_answer="n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_keys_warned(result)
        self.assertIn(GRANT_QUESTION, result.stdout)
        self.assertIn(f"No role was granted to {PROV_EMAIL}.", result.stdout)
        self.assertIn(
            f"The existing account {PROV_ACCOUNT} was not given its\n    roles by this run.",
            result.stdout,
        )
        self.assertEqual(self.grants(), [])
        self.assertEqual(self.calls("terraform"), [])
        self.assertFalse((self.tmp / "tf" / "terraform.tfvars").exists())
        self.assertFalse((self.tmp / "credential.json").exists())

    def test_interactive_reuse_with_keys_yes_grants_and_deploys(self):
        result = self.interactive_deploy(grant_answer="y")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_keys_warned(result)
        self.assertIn(GRANT_QUESTION, result.stdout)
        self.assertEqual(len(self.grants()), 17)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)
        self.assertIn(PROV_TFVARS_LINE, self.tfvars())

    def test_interactive_unreadable_keys_asks(self):
        self.env.update(FAKE_READ_CMD="keys", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.interactive_deploy(grant_answer="n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(DENIED, result.stderr)
        self.assertIn(f"Could not list the user-managed keys of {PROV_EMAIL}", result.stderr)
        self.assertIn(GRANT_QUESTION, result.stdout)
        self.assertEqual(self.grants(), [])
        self.assertEqual(self.calls("terraform"), [])

    def test_interactive_reuse_without_keys_asks_nothing(self):
        self.env["FAKE_USER_KEYS"] = ""
        result = self.interactive_deploy()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn(GRANT_QUESTION, result.stdout)
        self.assertNotIn("user-managed keys", result.stderr)
        self.assertEqual(len(self.grants()), 17)

    def test_config_reuse_with_keys_warns_and_continues(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_keys_warned(result)
        self.assertNotIn(GRANT_QUESTION, result.stdout)
        self.assertEqual(len(self.grants()), 17)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)
        # The keys are listed before any role is granted.
        log = self.log.read_text()
        self.assertLess(
            log.index("gcloud\x1fiam\x1fservice-accounts\x1fkeys\x1flist"),
            log.index("gcloud\x1fprojects\x1fadd-iam-policy-binding"),
        )

    def test_config_unreadable_keys_warns_and_continues(self):
        self.env.update(FAKE_READ_CMD="keys", FAKE_READ_FAILS="99", FAKE_READ_ERROR=DENIED)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(f"Could not list the user-managed keys of {PROV_EMAIL}", result.stderr)
        self.assertEqual(len(self.grants()), 17)
        self.assertEqual(len(self.calls("terraform", "apply")), 1)


class TerraformVersionMessageTests(SimpleTestCase):
    """The 'terraform not found' messages name the version tf/ requires"""

    def test_messages_name_the_required_version(self):
        versions = (OFE_DIR / "tf" / "versions.tf").read_text()
        required = re.search(r'required_version\s*=\s*">=\s*([0-9.]+)"', versions).group(1)
        self.assertEqual(required, "1.12.2")
        for script in ("deploy.sh", "teardown.sh"):
            text = (OFE_DIR / script).read_text()
            self.assertIn(
                f"Please ensure terraform (version {required} or later) is in your", text, script
            )
            self.assertNotIn("0.13", text, script)


class ConfigRegionTests(FakeCommandTestCase):
    """The --config path derives the region from the zone, as the interactive one does"""

    def test_config_tfvars_have_the_zones_region(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        # DEPLOY_CONFIG's zone is europe-west4-a.
        self.assertIn('region = "europe-west4"', self.tfvars())
        self.assertNotIn('region = ""', self.tfvars())

    def test_both_paths_write_the_same_region(self):
        self.assertEqual(self.deploy().returncode, 0)
        config_region = [line for line in self.tfvars() if line.startswith("region =")]
        (self.tmp / "tf" / ".tkfe.lock").unlink()
        self.assertEqual(self.interactive_deploy().returncode, 0)
        self.assertEqual(
            [line for line in self.tfvars() if line.startswith("region =")], config_region
        )


class TeardownProvisioningAccountTests(FakeCommandTestCase):
    """teardown.sh deletes the provisioning account, with its roles, after the destroy"""

    def account_calls(self, command):
        return [call for call in self.calls("service_account.sh") if call[0] == command]

    def deletes(self):
        return self.account_calls("delete")

    def print_on_destroy(self):
        """The fake terraform destroy prints a line, which shows on stdout."""
        terraform = self.bin / "terraform"
        terraform.write_text(
            terraform.read_text().replace(
                "esac\nexit 0\n", 'destroy)\n\techo "fake destroy output"\n\t;;\nesac\nexit 0\n'
            )
        )

    def test_yes_flag_deletes_after_destroy(self):
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.deletes(), [["delete", PROJECT, PROV_ACCOUNT]])
        log = self.log.read_text()
        self.assertLess(
            log.index("terraform\x1fdestroy"),
            log.index(f"service_account.sh\x1fdelete\x1f{PROJECT}\x1f{PROV_ACCOUNT}"),
        )
        self.assertIn("Completed teardown", result.stdout)

    def test_prompt_yes_deletes(self):
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown(answers=["y", "y"])  # proceed, delete
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.deletes(), [["delete", PROJECT, PROV_ACCOUNT]])

    def test_prompt_no_keeps_the_account(self):
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown(answers=["y", "n"])
        # Kept by choice, it still holds IAM-admin roles: listed, exit 1
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertIn(f"Kept {PROV_ACCOUNT}, which may still hold its project roles.", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{PROV_ACCOUNT}: script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}",
            result.stdout,
        )

    def test_prompt_with_no_input_keeps_the_account_and_finishes(self):
        # End of input at the question must not stop the script (set -e).
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown(answers=["y"])
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{PROV_ACCOUNT}: script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}",
            result.stdout,
        )
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_old_server_account_is_deleted_without_asking_under_y(self):
        # With -y, a pre-existing <name>-tkfe-sa account is deleted as the
        # answer "y" would delete it, only once the destroy has succeeded and
        # before the provisioning account; no answer is read.
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env["FAKE_SA_EXISTS"] = old_account
        self.print_on_destroy()
        for stdin_text in (None, "n\n"):
            with self.subTest(stdin_text=stdin_text):
                self.log.write_text("")
                (self.tmp / "tf" / ".tkfe.lock").touch()
                if stdin_text is None:
                    result = self.teardown()  # stdin is /dev/null
                else:
                    # A "no" waiting on stdin: were an answer read, it would
                    # keep the account.
                    (self.tmp / "tf" / "terraform.tfvars").write_text(
                        f'project_id = "{PROJECT}"\ndeployment_name = "{DEPLOYMENT}"\n'
                        f'region = "{REGION}"\nzone = "{ZONE}"\n'
                    )
                    self.set_state(*CURRENT_STATE)
                    result = self.run_script(str(self.tmp / "teardown.sh"), "-y", stdin_text=stdin_text)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.deletes(), [["delete", PROJECT, old_account]])
                self.assertNotIn("Kept", result.stdout)
                # Only after the teardown goes ahead and the destroy succeeds.
                self.assertLess(
                    result.stdout.index(f"This will destroy the running FrontEnd: {DEPLOYMENT}"),
                    result.stdout.index("-y flag passed. Proceeding anyway."),
                )
                self.assertLess(
                    result.stdout.index("-y flag passed. Proceeding anyway."),
                    result.stdout.index("fake destroy output"),
                )
                self.assertLess(
                    result.stdout.index("fake destroy output"),
                    result.stdout.index(f"Service account found: {old_account}"),
                )
                log = self.log.read_text()
                old_check = log.index(f"service_account.sh\x1fcheck\x1f{PROJECT}\x1f{old_account}")
                old_delete = log.index(f"service_account.sh\x1fdelete\x1f{PROJECT}\x1f{old_account}")
                self.assertLess(log.index("terraform\x1fdestroy"), old_check)
                self.assertLess(old_check, old_delete)
                self.assertLess(
                    old_delete,
                    log.index(f"service_account.sh\x1fcheck\x1f{PROJECT}\x1f{PROV_ACCOUNT}"),
                )
                self.assertIn("  Completed teardown\n", result.stdout)
                self.assertNotIn("Completed teardown, except:", result.stdout)
                self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_old_server_account_under_y_asks_nothing_on_a_terminal(self):
        # read -p prints its prompt only when stdin is a terminal, so a pipe
        # cannot show whether -y asks. With stdin on a pseudo-terminal holding
        # "n", -y must neither print the question nor act on an answer, nor
        # read it: the "n" line is still waiting on the terminal afterwards.
        # Each Ctrl-D after it gives a further read end of input, so a script
        # that reads more fails here instead of waiting for input forever.
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env["FAKE_SA_EXISTS"] = old_account
        self.print_on_destroy()
        (self.tmp / "tf" / "terraform.tfvars").write_text(
            f'project_id = "{PROJECT}"\ndeployment_name = "{DEPLOYMENT}"\n'
            f'region = "{REGION}"\nzone = "{ZONE}"\n'
        )
        self.set_state(*CURRENT_STATE)
        (self.tmp / "tf" / ".tkfe.lock").touch()
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        self.addCleanup(os.close, slave)
        os.write(master, b"n\n\x04\x04\x04")
        result = subprocess.run(
            ["bash", str(self.tmp / "teardown.sh"), "-y"],
            cwd=self.tmp,
            env=self.env,
            stdin=slave,
            capture_output=True,
            text=True,
            check=False,
        )
        os.set_blocking(slave, False)
        self.assertEqual(os.read(slave, 1024), b"n\n", "-y read the answer from the terminal")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("Delete [y/N]", result.stdout + result.stderr)
        self.assertNotIn("Kept", result.stdout)
        self.assertEqual(self.deletes(), [["delete", PROJECT, old_account]])
        self.assertLess(
            result.stdout.index("-y flag passed. Proceeding anyway."),
            result.stdout.index("fake destroy output"),
        )
        self.assertLess(
            result.stdout.index("fake destroy output"),
            result.stdout.index(f"Service account found: {old_account}"),
        )
        log = self.log.read_text()
        self.assertLess(
            log.index("terraform\x1fdestroy"),
            log.index(f"service_account.sh\x1fdelete\x1f{PROJECT}\x1f{old_account}"),
        )
        self.assertIn("  Completed teardown\n", result.stdout)

    def test_old_server_account_question_without_y_is_asked_on_a_terminal(self):
        # The control for the test above: without -y, on a terminal, the
        # question is printed once and the "n" read from it keeps the account.
        # The Ctrl-Ds give any further read end of input, so a script that
        # reads more fails here instead of waiting for input forever.
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env["FAKE_SA_EXISTS"] = old_account
        (self.tmp / "tf" / "terraform.tfvars").write_text(
            f'project_id = "{PROJECT}"\ndeployment_name = "{DEPLOYMENT}"\n'
            f'region = "{REGION}"\nzone = "{ZONE}"\n'
        )
        self.set_state(*CURRENT_STATE)
        (self.tmp / "tf" / ".tkfe.lock").touch()
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        try:
            os.write(master, b"n\nn\n\x04\x04\x04")  # keep it; do not proceed
            result = subprocess.run(
                ["bash", str(self.tmp / "teardown.sh")],
                cwd=self.tmp,
                env=self.env,
                stdin=slave,
                capture_output=True,
                text=True,
                check=False,
            )
        finally:
            os.close(slave)
        self.assertEqual(result.stderr.count("Delete [y/N]"), 1, result.stdout + result.stderr)
        self.assertIn(f"Kept {old_account}, which may still hold its project roles.", result.stdout)
        self.assertEqual(self.deletes(), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_failed_old_server_account_delete_under_y_is_listed_and_fails(self):
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env.update(FAKE_SA_EXISTS=old_account, FAKE_SA_DELETE_RC="1")
        result = self.teardown()
        # Its IAM-admin roles may remain: listed with its command, exit 1
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(self.deletes(), [["delete", PROJECT, old_account]])
        self.assertIn(f"Error: Failed to delete {old_account} (the error is above).", result.stdout)
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{old_account}: script/service_account.sh delete {PROJECT} {old_account}",
            result.stdout,
        )
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_old_server_account_question_without_y_keeps_on_no(self):
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env["FAKE_SA_EXISTS"] = old_account
        # Keep it, proceed. A third answer waits on stdin: without -y the
        # question is asked once, before init, so nothing may read it.
        result = self.teardown(answers=["n", "y", "y"])
        # Kept, with its IAM-admin roles: listed with its command, exit 1
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(
            [call for call in self.calls("service_account.sh") if call[1:3] == [PROJECT, old_account]],
            [["check", PROJECT, old_account]],
        )
        self.assertEqual(self.deletes(), [])
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertEqual(
            result.stdout.count(f"Kept {old_account}, which may still hold its project roles."),
            1,
            result.stdout,
        )
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertEqual(
            result.stdout.count(
                f"      {old_account}: script/service_account.sh delete {PROJECT} {old_account}\n"
            ),
            1,
            result.stdout,
        )
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_old_server_account_question_with_no_input_does_not_stop_teardown(self):
        # Without -y, end of input at the question about a pre-existing
        # <name>-tkfe-sa account keeps it and must not stop the script (set -e).
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env["FAKE_SA_EXISTS"] = old_account
        result = self.teardown(answers=[])
        # No answer at "Proceed?" either, which is N: nothing is planned or
        # destroyed; kept, listed with its command, exit 1
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertIn(f"Kept {old_account}, which may still hold its project roles.", result.stdout)
        self.assertIn(
            f"{old_account}: script/service_account.sh delete {PROJECT} {old_account}",
            result.stdout,
        )
        self.assertEqual(self.calls("terraform", "plan"), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertTrue((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_failed_old_server_account_delete_does_not_stop_teardown(self):
        old_account = f"{DEPLOYMENT}-tkfe-sa"
        self.env.update(FAKE_SA_EXISTS=old_account, FAKE_SA_DELETE_RC="1")
        result = self.teardown(answers=["y", "y"])  # delete it, proceed
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.deletes(), [["delete", PROJECT, old_account]])
        self.assertIn(f"Failed to delete {old_account}", result.stdout)
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{old_account}: script/service_account.sh delete {PROJECT} {old_account}",
            result.stdout,
        )
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_absent_account_is_not_deleted(self):
        result = self.teardown()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(["check", PROJECT, PROV_ACCOUNT], self.account_calls("check"))
        self.assertEqual(self.deletes(), [])

    def test_failed_delete_finishes_but_names_the_account_and_fails(self):
        # The account holds IAM-admin roles: never a plain "Completed"
        self.env.update(FAKE_SA_EXISTS=PROV_ACCOUNT, FAKE_SA_DELETE_RC="1")
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn(f"Failed to delete {PROV_ACCOUNT}", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{PROV_ACCOUNT}: script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}",
            result.stdout,
        )
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_account_that_cannot_be_checked_is_named_and_fails(self):
        # A failed list is not an absent account
        self.env["FAKE_SA_UNCHECKABLE"] = f"{PROV_ACCOUNT} {DEPLOYMENT}-fe-sa"
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertIn(f"Could not check whether {PROV_ACCOUNT} still exists", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        for account in (PROV_ACCOUNT, f"{DEPLOYMENT}-fe-sa"):
            self.assertIn(
                f"{account}: script/service_account.sh delete {PROJECT} {account}",
                result.stdout,
            )
        self.assertNotIn("  Completed teardown\n", result.stdout)
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_failed_leftover_web_account_delete_is_listed_and_fails(self):
        # The destroy normally deletes the web server's account; a leftover
        # one whose delete fails may still hold its project roles.
        self.env.update(FAKE_SA_EXISTS=FE_ACCOUNT, FAKE_SA_DELETE_RC="1")
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(self.deletes(), [["delete", PROJECT, FE_ACCOUNT]])
        self.assertIn(f"Removing leftover service account: {FE_ACCOUNT}", result.stdout)
        self.assertIn(f"Failed to delete {FE_ACCOUNT}", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"      {FE_ACCOUNT}: script/service_account.sh delete {PROJECT} {FE_ACCOUNT}\n",
            result.stdout,
        )
        self.assertNotIn("  Completed teardown\n", result.stdout)
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_failed_destroy_deletes_nothing(self):
        terraform = self.bin / "terraform"
        terraform.write_text(
            terraform.read_text().replace('esac\nexit 0\n', 'destroy)\n\texit 1\n\t;;\nesac\nexit 0\n')
        )
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.calls("terraform", "destroy")), 1)
        self.assertNotIn(["check", PROJECT, PROV_ACCOUNT], self.account_calls("check"))
        self.assertEqual(self.deletes(), [])


class AbortedDeployTeardownTests(FakeCommandTestCase):
    """After a deploy that stopped before Terraform created anything, so
    with no server, teardown.sh deletes the provisioning account it created
    when the user answers yes, and otherwise lists it; with -y it only lists
    it, as a deploy running elsewhere under the same name may use it"""

    keep_empty_state = True

    def setUp(self):
        super().setUp()
        self.env.update(FAKE_SA_EXISTS=PROV_ACCOUNT, FAKE_SERVER_VM="absent")
        self.lock = self.tmp / "tf" / ".tkfe.lock"
        self.lock.touch()

    def account_calls(self, command):
        return [call for call in self.calls("service_account.sh") if call[0] == command]

    def assert_listed(self, result):
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"{PROV_ACCOUNT}: script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}",
            result.stdout,
        )
        self.assertFalse(self.lock.exists())

    def test_yes_flag_lists_the_account(self):
        # With no state, a deploy running elsewhere under the same name may
        # use the account: -y never deletes it, only lists it.
        result = self.teardown()
        self.assertEqual(self.account_calls("delete"), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)
        self.assertIn(f"Provisioning service account found: {PROV_ACCOUNT}", result.stdout)
        self.assertIn("It is not deleted with -y", result.stdout)
        self.assert_listed(result)
        # The web server's account is checked too.
        self.assertIn(["check", PROJECT, f"{DEPLOYMENT}-fe-sa"], self.account_calls("check"))

    def test_answer_yes_deletes_the_account(self):
        result = self.teardown(answers=["y"])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.account_calls("delete"), [["delete", PROJECT, PROV_ACCOUNT]])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertIn("  Completed teardown\n", result.stdout)
        self.assertFalse(self.lock.exists())

    def test_no_state_file_deletes_the_account(self):
        # Before the first apply there is no state: "state list" fails and
        # "state pull" prints nothing.
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="No state file was found!")
        result = self.teardown(answers=["y"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.account_calls("delete"), [["delete", PROJECT, PROV_ACCOUNT]])
        self.assertNotIn("No state file was found!", result.stderr)
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)

    def test_failed_delete_is_listed(self):
        self.env["FAKE_SA_DELETE_RC"] = "1"
        result = self.teardown(answers=["y"])
        self.assertEqual(self.account_calls("delete"), [["delete", PROJECT, PROV_ACCOUNT]])
        self.assertIn(f"Failed to delete {PROV_ACCOUNT}", result.stdout)
        self.assert_listed(result)

    def test_answer_no_keeps_and_lists_the_account(self):
        result = self.teardown(answers=["n"])
        self.assertEqual(self.account_calls("delete"), [])
        self.assertIn(f"Kept {PROV_ACCOUNT}, which may still hold its project roles.", result.stdout)
        self.assert_listed(result)

    def test_unreadable_state_touches_no_account(self):
        # A state that cannot be read may hold a running server, which acts
        # as the account.
        self.set_state(OTHER_ADDRESS)
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="Failed to load state")
        result = self.teardown()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.account_calls("delete"), [])
        self.assertNotIn(["check", PROJECT, PROV_ACCOUNT], self.account_calls("check"))
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertIn("Failed to load state", result.stderr)
        self.assertIn("Could not read the Terraform state", result.stdout)
        self.assertTrue(self.lock.exists())


class TeardownEarlyExitSummaryTests(FakeCommandTestCase):
    """Every early exit of teardown.sh lists the accounts it may have left,
    with their commands, exits non-zero, and leaves the lock file"""

    def setUp(self):
        super().setUp()
        # An old account, kept by "n"; with -y, an early exit must not
        # touch it, and were it deleted, the delete would fail.
        self.env.update(FAKE_SA_EXISTS=OLD_ACCOUNT, FAKE_SA_DELETE_RC="1")
        self.lock = self.tmp / "tf" / ".tkfe.lock"
        self.lock.touch()

    def fail_terraform(self, command, message=""):
        terraform = self.bin / "terraform"
        terraform.write_text(
            terraform.read_text().replace(
                "esac\nexit 0\n", f'{command})\n\techo "{message}" >&2\n\texit 1\n\t;;\nesac\nexit 0\n'
            )
        )

    def assert_listed_early(self, result):
        """The old account was kept by "n" (-y never touches it before an early exit)"""
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(f"Kept {OLD_ACCOUNT}, which may still hold its project roles.", result.stdout)
        self.assertIn("The teardown stopped before it finished", result.stdout)
        self.assertIn(
            f"      {OLD_ACCOUNT}: script/service_account.sh delete {PROJECT} {OLD_ACCOUNT}\n",
            result.stdout,
        )
        # The list comes last.
        self.assertGreater(
            result.stdout.index(f"{OLD_ACCOUNT}: script/service_account.sh"),
            result.stdout.index("Exiting."),
        )
        self.assertNotIn("Completed teardown", result.stdout)
        self.assertTrue(self.lock.exists())

    def test_no_at_proceed(self):
        result = self.teardown(answers=["n", "n"])  # keep the old account, do not proceed
        self.assert_listed_early(result)
        self.assertEqual(self.calls("terraform", "plan"), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_no_at_proceed_without_a_kept_account_still_exits_0(self):
        self.env["FAKE_SA_EXISTS"] = ""
        result = self.teardown(answers=["n"])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("may also still exist", result.stdout)

    def test_no_answer_at_proceed_is_no(self):
        # End of input at "Proceed?" is N: it stops cleanly, destroying
        # nothing, rather than set -e ending the script at the read.
        self.env["FAKE_SA_EXISTS"] = ""
        result = self.teardown(answers=[])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Exiting.", result.stdout)
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def assert_old_account_untouched_under_y(self, result, destroy_runs=0):
        """-y stopped before the destroy succeeded: the old account was
        neither checked nor deleted, so nothing is listed, and it exits 1.
        destroy_runs: how many times terraform destroy ran (and failed)."""
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(
            [call for call in self.calls("service_account.sh") if call[1:3] == [PROJECT, OLD_ACCOUNT]],
            [],
        )
        self.assertNotIn(OLD_ACCOUNT, result.stdout)
        self.assertNotIn("The teardown stopped before it finished", result.stdout)
        self.assertIn("Exiting.", result.stdout)
        self.assertNotIn("Completed teardown", result.stdout)
        self.assertEqual(len(self.calls("terraform", "destroy")), destroy_runs)
        self.assertTrue(self.lock.exists())

    def test_init_fails(self):
        self.env["FAKE_INIT_RC"] = "1"
        result = self.teardown(answers=["n"])  # keep the old account
        self.assertIn("terraform init failed", result.stderr)
        self.assert_listed_early(result)

    def test_init_fails_under_y_does_not_touch_the_old_account(self):
        self.env["FAKE_INIT_RC"] = "1"
        result = self.teardown()
        self.assertIn("terraform init failed", result.stderr)
        self.assert_old_account_untouched_under_y(result)

    def test_unreadable_state(self):
        self.set_state(OTHER_ADDRESS)
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="Failed to load state")
        result = self.teardown(answers=["n"])  # keep the old account
        self.assertIn("Could not read the Terraform state", result.stdout)
        self.assert_listed_early(result)

    def test_unreadable_state_under_y_does_not_touch_the_old_account(self):
        self.set_state(OTHER_ADDRESS)
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="Failed to load state")
        result = self.teardown()
        self.assertIn("Could not read the Terraform state", result.stdout)
        self.assert_old_account_untouched_under_y(result)

    def test_second_state_read_fails_under_y_does_not_touch_the_old_account(self):
        # The first state read succeeds and the destroy check runs; the read
        # after it fails, so nothing is destroyed. Were the old account
        # deleted here, the delete would succeed with nothing listed.
        del self.env["FAKE_SA_DELETE_RC"]
        self.env.update(FAKE_STATE_LIST_FAIL_FROM="2", FAKE_STATE_LIST_ERROR="fake second read failure")
        result = self.teardown()
        self.assertEqual(
            [call for call in self.calls("terraform", "state") if call[1:2] == ["list"]],
            [["state", "list"], ["state", "list"]],
        )
        self.assertEqual(len(self.calls("terraform", "plan")), 1)
        self.assertIn("fake second read failure", result.stderr)
        self.assertIn(
            "Error: Could not read the Terraform state in tf/; nothing was destroyed.", result.stdout
        )
        self.assert_old_account_untouched_under_y(result)

    def test_destroy_check_fails(self):
        self.fail_terraform("plan", "fake plan failure")
        result = self.teardown(answers=["n", "y"])  # keep the old account, proceed
        self.assertIn("Terraform cannot plan the destroy", result.stdout)
        self.assert_listed_early(result)
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_destroy_check_fails_under_y_does_not_touch_the_old_account(self):
        self.fail_terraform("plan", "fake plan failure")
        result = self.teardown()
        self.assertIn("Terraform cannot plan the destroy", result.stdout)
        self.assert_old_account_untouched_under_y(result)

    def fail_old_role_removal(self):
        self.set_state(NEW_ADDRESS, *NEW_GRANT_ADDRESSES)
        self.set_policy(("roles/pubsub.admin", MEMBER))
        self.iam_errors(DENIED)

    def test_old_role_removal_fails(self):
        self.fail_old_role_removal()
        result = self.teardown(answers=["n", "y"])  # keep the old account, proceed
        self.assertIn("nothing was destroyed", result.stdout)
        self.assert_listed_early(result)
        self.assertEqual(self.calls("terraform", "destroy"), [])

    def test_old_role_removal_fails_under_y_does_not_touch_the_old_account(self):
        self.fail_old_role_removal()
        result = self.teardown()
        self.assertIn("nothing was destroyed", result.stdout)
        self.assert_old_account_untouched_under_y(result)

    def assert_destroy_failed(self, result):
        self.assertIn("fake destroy failure", result.stdout + result.stderr)
        self.assertIn(
            "Error: Terraform destroy failed (the error is above); the deployment may\n"
            "         be partly destroyed. Fix the cause and run teardown.sh again.",
            result.stdout,
        )
        # The server may still act as the provisioning account: not touched.
        self.assertNotIn(["check", PROJECT, PROV_ACCOUNT], self.calls("service_account.sh"))

    def test_destroy_fails(self):
        self.fail_terraform("destroy", "fake destroy failure")
        self.env["FAKE_SA_EXISTS"] = f"{OLD_ACCOUNT} {PROV_ACCOUNT}"
        result = self.teardown(answers=["n", "y"])  # keep the old account, proceed
        self.assert_destroy_failed(result)
        self.assert_listed_early(result)

    def test_destroy_fails_after_a_failed_old_account_delete(self):
        # Without -y, "y" deletes the old account before init; that delete
        # fails (setUp), then the destroy fails: the account is listed under
        # the early-stop summary with its command, after the delete error.
        self.fail_terraform("destroy", "fake destroy failure")
        result = self.teardown(answers=["y", "y"])  # delete the old account, proceed
        self.assert_destroy_failed(result)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(
            [call for call in self.calls("service_account.sh") if call[1:3] == [PROJECT, OLD_ACCOUNT]],
            [["check", PROJECT, OLD_ACCOUNT], ["delete", PROJECT, OLD_ACCOUNT]],
        )
        self.assertIn(f"Error: Failed to delete {OLD_ACCOUNT} (the error is above).", result.stdout)
        self.assertNotIn(f"Kept {OLD_ACCOUNT}", result.stdout)
        self.assertIn("The teardown stopped before it finished", result.stdout)
        listing = f"      {OLD_ACCOUNT}: script/service_account.sh delete {PROJECT} {OLD_ACCOUNT}\n"
        self.assertEqual(result.stdout.count(listing), 1, result.stdout)
        # The list comes last.
        self.assertGreater(result.stdout.index(listing), result.stdout.index("Exiting."))
        self.assertNotIn("Completed teardown", result.stdout)
        self.assertTrue(self.lock.exists())

    def test_destroy_fails_under_y_does_not_touch_the_old_account(self):
        self.fail_terraform("destroy", "fake destroy failure")
        self.env["FAKE_SA_EXISTS"] = f"{OLD_ACCOUNT} {PROV_ACCOUNT}"
        result = self.teardown()
        self.assert_destroy_failed(result)
        self.assert_old_account_untouched_under_y(result, destroy_runs=1)

    def test_destroy_fails_under_y_keeps_an_old_account_that_could_be_deleted(self):
        # The case a delete before the destroy would get wrong: the delete
        # would succeed, leaving the deployment in place and the account
        # gone, with nothing listed to show it.
        del self.env["FAKE_SA_DELETE_RC"]
        self.fail_terraform("destroy", "fake destroy failure")
        result = self.teardown()
        self.assert_destroy_failed(result)
        self.assertEqual(
            [call for call in self.calls("service_account.sh") if call[0] == "delete"], []
        )
        self.assert_old_account_untouched_under_y(result, destroy_runs=1)


class TeardownNoStateServerTests(FakeCommandTestCase):
    """With no Terraform state, teardown.sh touches the provisioning and web
    server accounts only when the server VM is known to be gone"""

    keep_empty_state = True

    def setUp(self):
        super().setUp()
        self.env["FAKE_SA_EXISTS"] = f"{PROV_ACCOUNT} {FE_ACCOUNT}"
        self.lock = self.tmp / "tf" / ".tkfe.lock"
        self.lock.touch()

    def account_calls(self):
        """service_account.sh calls naming the provisioning or web server account"""
        return [
            call for call in self.calls("service_account.sh")
            if call[2:3] in ([PROV_ACCOUNT], [FE_ACCOUNT])
        ]

    def describes(self):
        return [call for call in self.calls("gcloud", "compute") if call[1:3] == ["instances", "describe"]]

    def assert_refused(self, result, message):
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(message, result.stdout)
        self.assertIn("Nothing was", result.stdout)
        self.assertIn("Teardown Process", result.stdout)
        self.assertEqual(self.account_calls(), [])
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertNotIn("No terraform deployment found", result.stdout)
        self.assertTrue(self.lock.exists())

    def test_server_exists_touches_no_account(self):
        # The state was lost while the server still runs, and acts as both.
        self.env["FAKE_SERVER_VM"] = "exists"
        result = self.teardown()
        self.assert_refused(
            result, f"Error: The server {SERVER} still exists but tf/ has no Terraform"
        )
        self.assertEqual(
            self.describes(),
            [[
                "compute", "instances", "describe", SERVER, f"--zone={ZONE}",
                f"--project={PROJECT}", "--format=value(name)",
            ]],
        )

    def test_server_exists_without_a_state_file_touches_no_account(self):
        self.env.update(FAKE_STATE_LIST_RC="1", FAKE_STATE_LIST_ERROR="No state file was found!")
        result = self.teardown()
        self.assert_refused(result, f"The server {SERVER} still exists")

    def test_server_that_cannot_be_checked_touches_no_account(self):
        self.env["FAKE_SERVER_VM"] = "error"
        result = self.teardown()
        self.assert_refused(result, f"Error: Could not check whether the server {SERVER} exists")
        self.assertIn("Required 'compute.instances.get' permission", result.stderr)

    def test_zone_not_found_is_not_an_absent_server(self):
        self.env["FAKE_SERVER_VM"] = "zone_absent"
        result = self.teardown()
        self.assert_refused(result, f"Error: Could not check whether the server {SERVER} exists")
        self.assertIn(f"projects/{PROJECT}/zones/{ZONE}' was not found", result.stderr)

    def test_no_zone_is_not_an_absent_server(self):
        result = self.teardown(extra_tfvars=f'region = "{REGION}"\n')
        self.assert_refused(result, f"Error: Could not check whether the server {SERVER} exists")
        self.assertIn("has no zone", result.stdout)
        self.assertEqual(self.describes(), [])

    def assert_web_account_listed(self, result):
        self.assertIn(f"Web server service account found: {FE_ACCOUNT}", result.stdout)
        self.assertIn("It is not deleted: with no Terraform state, this folder did not", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"      {FE_ACCOUNT}: script/service_account.sh delete {PROJECT} {FE_ACCOUNT}\n",
            result.stdout,
        )
        self.assertNotIn(f"Kept {FE_ACCOUNT}", result.stdout)

    def test_absent_server_yes_flag_lists_both_accounts(self):
        # With no state, Terraform did not create the web server's account
        # from this folder, and a deploy running elsewhere under the same
        # name may use either account: -y deletes neither, and lists both.
        self.env["FAKE_SERVER_VM"] = "absent"
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual([call for call in self.account_calls() if call[0] == "delete"], [])
        self.assertIn(["check", PROJECT, PROV_ACCOUNT], self.account_calls())
        self.assertIn(["check", PROJECT, FE_ACCOUNT], self.account_calls())
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)
        self.assertIn(f"Provisioning service account found: {PROV_ACCOUNT}", result.stdout)
        self.assertIn("It is not deleted with -y", result.stdout)
        self.assert_web_account_listed(result)
        self.assertIn(f"      {PROV_ACCOUNT}: {PROV_DELETE_COMMAND}\n", result.stdout)
        self.assertNotIn(f"Kept {PROV_ACCOUNT}", result.stdout)
        self.assertFalse(self.lock.exists())

    def test_absent_server_answer_yes_does_not_delete_the_web_account(self):
        self.env["FAKE_SERVER_VM"] = "absent"
        result = self.teardown(answers=["y", "y"])
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(
            [call for call in self.account_calls() if call[0] == "delete"],
            [["delete", PROJECT, PROV_ACCOUNT]],
        )
        self.assert_web_account_listed(result)

    def test_absent_server_without_the_web_account_completes(self):
        self.env.update(FAKE_SERVER_VM="absent", FAKE_SA_EXISTS=PROV_ACCOUNT)
        result = self.teardown(answers=["y"])  # delete the provisioning account
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(["check", PROJECT, FE_ACCOUNT], self.account_calls())
        self.assertNotIn("Web server service account found", result.stdout)
        self.assertIn("  Completed teardown\n", result.stdout)

    def test_absent_server_answer_no_lists_both_accounts(self):
        self.env["FAKE_SERVER_VM"] = "absent"
        result = self.teardown(answers=["n"])  # keep the provisioning account
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual([call for call in self.account_calls() if call[0] == "delete"], [])
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(f"Kept {PROV_ACCOUNT}, which may still hold its project roles.", result.stdout)
        for account in (PROV_ACCOUNT, FE_ACCOUNT):
            self.assertIn(
                f"{account}: script/service_account.sh delete {PROJECT} {account}", result.stdout
            )
        self.assert_web_account_listed(result)
        self.assertFalse(self.lock.exists())

    def test_refusal_under_y_does_not_touch_the_old_account(self):
        # The server may still use an old <name>-tkfe-sa account: a -y
        # teardown that refuses neither checks nor deletes it.
        self.env["FAKE_SA_EXISTS"] = f"{PROV_ACCOUNT} {FE_ACCOUNT} {OLD_ACCOUNT}"
        for server_vm, message in (
            ("exists", f"Error: The server {SERVER} still exists but tf/ has no Terraform"),
            ("error", f"Error: Could not check whether the server {SERVER} exists"),
        ):
            with self.subTest(server_vm=server_vm):
                self.log.write_text("")
                self.env["FAKE_SERVER_VM"] = server_vm
                result = self.teardown()
                self.assert_refused(result, message)
                self.assertEqual(
                    [call for call in self.calls("service_account.sh") if call[2:3] == [OLD_ACCOUNT]],
                    [],
                )
                self.assertNotIn(OLD_ACCOUNT, result.stdout)
                self.assertNotIn("The teardown stopped before it finished", result.stdout)

    def test_absent_server_yes_flag_lists_the_old_account(self):
        # With no state, a deploy running elsewhere under the same name may
        # use an old <name>-tkfe-sa account: -y lists it, never deletes it.
        self.env.update(
            FAKE_SERVER_VM="absent", FAKE_SA_EXISTS=f"{PROV_ACCOUNT} {FE_ACCOUNT} {OLD_ACCOUNT}"
        )
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual([call for call in self.calls("service_account.sh") if call[0] == "delete"], [])
        self.assertIn(["check", PROJECT, OLD_ACCOUNT], self.calls("service_account.sh"))
        self.assertIn(f"Service account found: {OLD_ACCOUNT}", result.stdout)
        self.assertIn(
            f"Service account found: {OLD_ACCOUNT}\n"
            "  It is not deleted with -y: with no Terraform state, this teardown\n",
            result.stdout,
        )
        self.assertNotIn(f"Kept {OLD_ACCOUNT}", result.stdout)
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertIn(
            f"      {OLD_ACCOUNT}: script/service_account.sh delete {PROJECT} {OLD_ACCOUNT}\n",
            result.stdout,
        )
        self.assertIn(f"      {PROV_ACCOUNT}: {PROV_DELETE_COMMAND}\n", result.stdout)
        self.assert_web_account_listed(result)
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertFalse(self.lock.exists())

    def test_absent_server_yes_flag_lists_an_old_account_that_cannot_be_checked(self):
        # An old <name>-tkfe-sa account whose check fails may still exist and
        # hold its project roles: -y lists it once, and the teardown fails.
        self.env.update(FAKE_SERVER_VM="absent", FAKE_SA_UNCHECKABLE=OLD_ACCOUNT)
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual([call for call in self.calls("service_account.sh") if call[0] == "delete"], [])
        self.assertIn(["check", PROJECT, OLD_ACCOUNT], self.calls("service_account.sh"))
        self.assertIn(f"Could not check whether {OLD_ACCOUNT} still exists", result.stdout)
        self.assertNotIn(f"Service account found: {OLD_ACCOUNT}", result.stdout)
        self.assertIn("No terraform deployment found; nothing else to destroy.", result.stdout)
        self.assertIn("Completed teardown, except:", result.stdout)
        self.assertEqual(
            result.stdout.count(
                f"      {OLD_ACCOUNT}: script/service_account.sh delete {PROJECT} {OLD_ACCOUNT}\n"
            ),
            1,
            result.stdout,
        )
        self.assertEqual(self.calls("terraform", "destroy"), [])
        self.assertFalse(self.lock.exists())

    def test_refusal_points_to_the_lost_state_steps(self):
        self.env["FAKE_SERVER_VM"] = "exists"
        result = self.teardown()
        self.assertIn('"If tf/ state is lost while the server runs"', result.stdout)
        guide = (OFE_DIR / "docs" / "admin_guide.md").read_text()
        self.assertIn("\n### If tf/ state is lost while the server runs\n", guide)


class TeardownNoLockTests(FakeCommandTestCase):
    """With no lock file, as after a deploy answered N at "Proceed to
    deploy?", a teardown answered N lists the provisioning account, if it
    exists, and deletes nothing"""

    keep_empty_state = True

    def run_no_lock(self, answer="n"):
        return self.teardown(answers=[answer], lock=False)

    def test_answer_no_lists_an_existing_provisioning_account(self):
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.run_no_lock()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("It is likely there is no FrontEnd currently deployed", result.stdout)
        self.assertIn(f"The provisioning service account {PROV_ACCOUNT} exists,", result.stdout)
        self.assertIn("The teardown stopped before it finished", result.stdout)
        self.assertIn(f"      {PROV_ACCOUNT}: {PROV_DELETE_COMMAND}\n", result.stdout)
        self.assertEqual(self.calls("service_account.sh"), [["check", PROJECT, PROV_ACCOUNT]])
        self.assertEqual(self.calls("terraform"), [])
        self.assertFalse((self.tmp / "tf" / ".tkfe.lock").exists())

    def test_answer_no_without_the_account_lists_nothing(self):
        result = self.run_no_lock()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.calls("service_account.sh"), [["check", PROJECT, PROV_ACCOUNT]])
        self.assertNotIn(PROV_DELETE_COMMAND, result.stdout)
        self.assertNotIn("stopped before it finished", result.stdout)
        self.assertIn("Exiting.", result.stdout)
        self.assertEqual(self.calls("terraform"), [])

    def test_answer_no_when_the_account_cannot_be_checked_lists_it(self):
        self.env["FAKE_SA_UNCHECKABLE"] = PROV_ACCOUNT
        result = self.run_no_lock()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(f"Could not check whether {PROV_ACCOUNT} still exists", result.stdout)
        self.assertIn(f"      {PROV_ACCOUNT}: {PROV_DELETE_COMMAND}\n", result.stdout)
        self.assertEqual(self.calls("terraform"), [])

    def test_no_answer_is_no_and_still_lists_the_account(self):
        # End of input at "Proceed anyway?" (no -y, nothing typed) is N:
        # the account is still listed rather than set -e stopping the script.
        self.env["FAKE_SA_EXISTS"] = PROV_ACCOUNT
        result = self.teardown(answers=[], lock=False)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(f"The provisioning service account {PROV_ACCOUNT} exists,", result.stdout)
        self.assertIn(f"      {PROV_ACCOUNT}: {PROV_DELETE_COMMAND}\n", result.stdout)
        self.assertEqual(self.calls("terraform"), [])

    def test_answer_yes_goes_on_to_the_no_state_path(self):
        # After a deploy answered N there is no state and no server: the
        # provisioning account is then offered for deletion.
        self.env.update(FAKE_SA_EXISTS=PROV_ACCOUNT, FAKE_SERVER_VM="absent")
        # Proceed anyway; delete the provisioning account.
        result = self.teardown(answers=["y", "y"], lock=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            [call for call in self.calls("service_account.sh") if call[0] == "delete"],
            [["delete", PROJECT, PROV_ACCOUNT]],
        )


class TeardownInitFailureAccountTests(FakeCommandTestCase):
    """When terraform init fails, teardown.sh lists the provisioning account,
    deleting nothing, only when the server VM is known to be gone"""

    def setUp(self):
        super().setUp()
        self.env.update(FAKE_INIT_RC="1", FAKE_SA_EXISTS=PROV_ACCOUNT)

    def deletes(self):
        return [call for call in self.calls("service_account.sh") if call[0] == "delete"]

    def test_absent_server_lists_the_account(self):
        self.env["FAKE_SERVER_VM"] = "absent"
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertIn("If this deployment stopped", result.stdout)
        self.assertIn(
            f"      {PROV_ACCOUNT}: script/service_account.sh delete {PROJECT} {PROV_ACCOUNT}\n",
            result.stdout,
        )
        self.assertEqual(self.calls("terraform", "state"), [])

    def test_absent_server_and_no_account_lists_nothing(self):
        self.env.update(FAKE_SERVER_VM="absent", FAKE_SA_EXISTS="")
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertNotIn(PROV_ACCOUNT, result.stdout)

    def test_running_server_lists_nothing(self):
        self.env["FAKE_SERVER_VM"] = "exists"
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertEqual(self.deletes(), [])
        self.assertNotIn(PROV_ACCOUNT, result.stdout)
        self.assertNotIn(["check", PROJECT, PROV_ACCOUNT], self.calls("service_account.sh"))

    def test_server_that_cannot_be_checked_lists_nothing(self):
        self.env["FAKE_SERVER_VM"] = "error"
        result = self.teardown()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertNotIn(PROV_ACCOUNT, result.stdout)
        self.assertIn(f"Could not check whether the server {SERVER} exists", result.stdout)


class TeardownEmptyProjectTests(FakeCommandTestCase):
    """teardown.sh refuses, changing nothing, when tf/terraform.tfvars names
    no project: an account check without one says nothing"""

    def test_empty_or_missing_project_is_refused(self):
        self.env["FAKE_SA_EXISTS"] = f"{OLD_ACCOUNT} {PROV_ACCOUNT} {FE_ACCOUNT}"
        lock = self.tmp / "tf" / ".tkfe.lock"
        for project_line in ('project_id = ""\n', ""):
            with self.subTest(project_line=project_line):
                self.log.write_text("")
                lock.touch()
                self.set_state(*CURRENT_STATE)
                (self.tmp / "tf" / "terraform.tfvars").write_text(
                    f'{project_line}deployment_name = "{DEPLOYMENT}"\n'
                    f'region = "{REGION}"\nzone = "{ZONE}"\n'
                )
                result = self.run_script(str(self.tmp / "teardown.sh"), "-y")
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(
                    "Error: tf/terraform.tfvars has no project_id (or it is empty)",
                    result.stdout,
                )
                self.assertIn("Nothing was changed.", result.stdout)
                self.assertEqual(self.calls("service_account.sh"), [])
                self.assertEqual(self.calls("terraform"), [])
                self.assertEqual(
                    [call for call in self.calls("gcloud") if call[:1] != ["storage"]], []
                )
                self.assertTrue(lock.exists())


class TeardownRegionTests(FakeCommandTestCase):
    """teardown.sh gives Terraform a region when an older --config deploy left it empty"""

    def test_empty_region_uses_the_zones_region(self):
        result = self.teardown(extra_tfvars='region = ""\nzone = "europe-west4-a"\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        expected = ["-var", "region=europe-west4"]
        self.assertEqual(self.calls("terraform", "plan"), [["plan", "-destroy", "-input=false", *expected]])
        self.assertEqual(self.calls("terraform", "destroy"), [["destroy", "-auto-approve", *expected]])
        self.assertIn("empty region; using europe-west4", result.stdout)

    def test_set_region_is_left_to_tfvars(self):
        result = self.teardown(extra_tfvars='region = "us-central1"\nzone = "europe-west4-a"\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls("terraform", "destroy"), [["destroy", "-auto-approve"]])
        self.assertNotIn("empty region", result.stdout)


class ApiCheckTests(FakeCommandTestCase):
    """Both deploy paths enable every API the server needs, the same way"""

    MISSING = "iamcredentials.googleapis.com"

    def enables(self):
        return [call for call in self.calls("gcloud", "services") if call[1] == "enable"]

    def test_impersonation_api_is_required(self):
        # The server acts as the keyless provisioning account through the IAM
        # Service Account Credentials API (generateAccessToken).
        self.assertIn(self.MISSING, REQUIRED_APIS)

    def test_config_path_enables_a_missing_api_without_asking(self):
        self.set_services([api for api in REQUIRED_APIS if api != self.MISSING])
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertEqual(
            self.enables(), [["services", "enable", self.MISSING, f"--project={PROJECT}"]]
        )
        # Before anything uses it: the account is created and Terraform runs
        # only after the API is enabled.
        log = self.log.read_text()
        enable = log.index("gcloud\x1fservices\x1fenable")
        self.assertLess(enable, log.index("service_account.sh\x1fcreate"))
        self.assertLess(enable, log.index("terraform\x1fapply"))

    def test_apis_are_listed_for_the_deployment_project(self):
        self.env["FAKE_CONFIG_PROJECT"] = "some-other-project"
        self.assertEqual(self.deploy().returncode, 0)
        lists = [call for call in self.calls("gcloud", "services") if call[1] == "list"]
        self.assertEqual(len(lists), 1)
        self.assertIn(f"--project={PROJECT}", lists[0])

    def test_config_path_enables_nothing_when_all_are_enabled(self):
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(self.enables(), [])

    def test_a_name_that_only_contains_the_api_does_not_count(self):
        # bigquerystorage.googleapis.com contains storage.googleapis.com.
        self.set_services(
            [api for api in REQUIRED_APIS if api != "storage.googleapis.com"]
            + ["bigquerystorage.googleapis.com"]
        )
        self.assertEqual(self.deploy().returncode, 0)
        self.assertEqual(
            self.enables(),
            [["services", "enable", "storage.googleapis.com", f"--project={PROJECT}"]],
        )

    def test_config_path_stops_when_enabling_fails(self):
        self.set_services([api for api in REQUIRED_APIS if api != self.MISSING])
        self.env["FAKE_ENABLE_RC"] = "1"
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(f"Unable to enable IAM Service Account Credentials API ({self.MISSING})", result.stderr)
        self.assertIn(f"in project {PROJECT}", result.stderr)
        self.assertIn("fake enable output", result.stderr)
        self.assertEqual(self.calls("service_account.sh"), [])
        self.assertEqual(self.calls("terraform"), [])

    def test_config_path_stops_when_apis_cannot_be_listed(self):
        self.env["FAKE_SERVICES_RC"] = "1"
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(f"Unable to list the APIs enabled in project {PROJECT}", result.stderr)
        self.assertEqual(self.enables(), [])
        self.assertEqual(self.calls("terraform"), [])

    def test_both_paths_give_the_same_result(self):
        missing = [self.MISSING, "secretmanager.googleapis.com"]
        enabled = [api for api in REQUIRED_APIS if api not in missing]
        self.set_services(enabled)
        self.assertEqual(self.deploy().returncode, 0)
        config_enables = self.enables()
        self.assertCountEqual(
            config_enables,
            [["services", "enable", api, f"--project={PROJECT}"] for api in missing],
        )

        self.log.write_text("")
        (self.tmp / "tf" / ".tkfe.lock").unlink()
        result = self.interactive_deploy(api_answers=["y"] * len(missing))
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertEqual(self.enables(), config_enables)

    def test_interactive_decline_stops_before_anything_is_created(self):
        self.set_services([api for api in REQUIRED_APIS if api != self.MISSING])
        result = self.interactive_deploy(api_answers=["n"])
        self.assertEqual(result.returncode, 0)
        self.assertIn("Exiting.", result.stdout)
        self.assertEqual(self.enables(), [])
        self.assertEqual(self.calls("service_account.sh"), [])
        self.assertEqual(self.calls("terraform"), [])

    def test_both_paths_call_the_one_check(self):
        script = (OFE_DIR / "deploy.sh").read_text()

        def body(function):
            match = re.search(rf"^{function}\(\) \{{\n(.*?)^\}}$", script, re.M | re.S)
            self.assertIsNotNone(match, function)
            return match.group(1)

        self.assertEqual(len(re.findall(r"^check_apis\(\)", script, re.M)), 1)
        self.assertIn('check_apis "${project_id}"\n', body("setup"))
        self.assertIn('check_apis "${project_id}" --yes\n', body("deploy_from_config"))


TARBALL_CONFIG = DEPLOY_CONFIG.replace("deployment_mode: git", "deployment_mode: tarball")


class TarballPackingTests(FakeCommandTestCase):
    """A tarball deployment packs only the files git would commit.

    The scratch directory is a real git checkout here. Packing must leave out
    ignored files (the Django secret key, a built binary, a virtual
    environment) but keep uncommitted changes and new files that are not
    ignored.
    """

    TRACKED = {
        "README.md": "readme\n",
        "website/settings.py": "committed settings\n",
        "website/old.py": "deleted before deploying\n",
        "website/.gitignore": ".secret_key\n",
        "tf/main.tf": "# tf is left out of the archive\n",
    }
    IGNORED = ["website/.secret_key", "gcluster", ".venv/lib/site.py"]

    def setUp(self):
        super().setUp()
        # Only this checkout's own ignore rules: no user or system git config.
        self.env.update(
            GIT_CONFIG_GLOBAL=os.devnull,
            GIT_CONFIG_NOSYSTEM="1",
            GIT_CEILING_DIRECTORIES=str(self.tmp.parent),
            TMPDIR=str(self.make_dir("staging")),
        )
        self.archive = self.tmp / "tf" / "deployment.tar.gz"

    def make_dir(self, name):
        path = self.tmp / name
        path.mkdir()
        return path

    def git(self, *args, cwd=None):
        subprocess.run(
            ["git", "-c", "user.name=Test", "-c", "user.email=test@example.com", *args],
            cwd=cwd or self.tmp,
            env=self.env,
            check=True,
            capture_output=True,
        )

    def write(self, name, content):
        path = self.tmp / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def make_checkout(self):
        """Commit the scripts and TRACKED, then change the working tree."""
        self.write(
            ".gitignore",
            "gcluster\n.venv/\nbin/\ncalls.log*\nstate_list\npolicy\nservices\n"
            "config.yaml\nstaging/\n**/deployment.tar.gz\n",
        )
        for name, content in self.TRACKED.items():
            self.write(name, content)
        (self.tmp / "link").symlink_to("README.md")
        self.git("init", "-q")
        self.git("add", "-A")
        self.git("commit", "-q", "-m", "initial")

        self.write("website/settings.py", "uncommitted settings\n")
        (self.tmp / "website" / "old.py").unlink()
        self.write("website/new_page.html", "new, not ignored\n")
        # An unusual mode, to show modes are kept exactly.
        (self.tmp / "deploy.sh").chmod(0o750)
        for name in self.IGNORED:
            self.write(name, "must not be shipped\n")

    def members(self):
        with tarfile.open(self.archive) as archive:
            return {member.name: member for member in archive.getmembers()}

    def read_member(self, name):
        with tarfile.open(self.archive) as archive:
            return archive.extractfile(f"./cluster-toolkit/{name}").read().decode()

    def test_ships_committable_files_only(self):
        self.make_checkout()
        result = self.deploy(TARBALL_CONFIG)
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)

        members = self.members()
        names = set(members)
        for name in self.IGNORED:
            self.assertNotIn(f"./cluster-toolkit/{name}", names)
        self.assertFalse([name for name in names if "/.venv" in name or "/.git/" in name], names)
        self.assertFalse([name for name in names if name.endswith("/.git")], names)
        self.assertNotIn("./cluster-toolkit/website/old.py", names)
        self.assertNotIn("./cluster-toolkit/tf", names)
        self.assertFalse([name for name in names if "/tf/" in name], names)

        self.assertEqual(self.read_member("website/settings.py"), "uncommitted settings\n")
        self.assertEqual(self.read_member("website/new_page.html"), "new, not ignored\n")
        self.assertEqual(self.read_member("website/.gitignore"), ".secret_key\n")
        self.assertTrue(members["./cluster-toolkit/link"].issym())
        self.assertEqual(members["./cluster-toolkit/link"].linkname, "README.md")
        self.assertEqual(members["./cluster-toolkit/deploy.sh"].mode & 0o777, 0o750)

    def test_archive_top_folder_is_cluster_toolkit(self):
        self.make_checkout()
        self.assertEqual(self.deploy(TARBALL_CONFIG).returncode, 0)
        tops = {name.split("/")[1] for name in self.members() if name.startswith("./")}
        self.assertEqual(tops, {"cluster-toolkit"})
        self.assertFalse([name for name in self.members() if not name.startswith("./")])

    def test_staging_directory_is_removed(self):
        self.make_checkout()
        self.assertEqual(self.deploy(TARBALL_CONFIG).returncode, 0)
        self.assertEqual(list(Path(self.env["TMPDIR"]).iterdir()), [])

    def test_nested_repository_is_not_shipped(self):
        self.make_checkout()
        nested = self.write("vendor/thing/file.txt", "another repository\n").parent
        self.git("init", "-q", cwd=nested)
        result = self.deploy(TARBALL_CONFIG)
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertFalse([name for name in self.members() if "vendor" in name])
        self.assertIn("Not packing vendor/thing/", result.stderr)

    def test_not_a_git_checkout_stops_before_terraform(self):
        result = self.deploy(TARBALL_CONFIG)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("is not in one", result.stderr)
        self.assertFalse(self.archive.exists())
        self.assertEqual(self.calls("terraform"), [])
