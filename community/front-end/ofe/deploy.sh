#! /bin/bash
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

################################################################################
#                                                                              #
#                    Cluster Toolkit FrontEnd deployment script                    #
#                                                                              #
################################################################################
#
#   Deployment requires a GCP project and authenticated user to run the FrontEnd
#   on GCP resources.
#
#   The FrontEnd also requires a provisioning service account, with the
#   permissions to create clusters and other resources. This script creates it
#   (<deployment_name>-prov-sa) with no key: only the FrontEnd web server's
#   account may act as it, and the server uses it without a key. A key file is
#   written only if the interactive user asks for one.
#

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

# Default GCP zone to use for FrontEnd server
#
DEFAULT_ZONE='europe-west4-a'

# GCP APIs needed by GCP project
# - using a bash associative array to hold key-value of API and description,
#   similar to Python
#
declare -A PRJ_API
PRJ_API['monitoring.googleapis.com']='Cloud Monitoring API'
PRJ_API['logging.googleapis.com']='Cloud Logging API'
PRJ_API['compute.googleapis.com']='Compute Engine API'
PRJ_API['pubsub.googleapis.com']='Cloud Pub/Sub API'
PRJ_API['file.googleapis.com']='Cloud Filestore API'
PRJ_API['cloudresourcemanager.googleapis.com']='Cloud Resource Manager API'
PRJ_API['pubsub.googleapis.com']='Cloud Pub/Sub API'
PRJ_API['iam.googleapis.com']='Identity and Access Management (IAM) API'
# The web server acts as the keyless provisioning account through this API.
PRJ_API['iamcredentials.googleapis.com']='IAM Service Account Credentials API'
PRJ_API['oslogin.googleapis.com']='Cloud OS Login API'
PRJ_API['cloudbilling.googleapis.com']='Cloud Billing API'
PRJ_API['aiplatform.googleapis.com']='Vertex AI API'
PRJ_API['bigqueryconnection.googleapis.com']='BigQuery Connection API'
PRJ_API['sqladmin.googleapis.com']='Cloud SQL Admin API'
PRJ_API['servicenetworking.googleapis.com']='Service Networking API'
PRJ_API['secretmanager.googleapis.com']='Secret Manager API'
PRJ_API['serviceusage.googleapis.com']='Service Usage API'
PRJ_API['storage.googleapis.com']='Cloud Storage API'

# Location for output credential file = pwd/credential.json
#
CREDENTIAL_FILE="${SCRIPT_DIR}/credential.json"

# help function
#   - write out purpose and preprequisites
#
help() {
	cat <<HELP1
  This script will ask a short series of questions to configure the FrontEnd
  web application before deployment.

  The Google Cloud CLI and terraform must be installed before use.
  These can be downloaded from:
      https://cloud.google.com/cli
      https://www.terraform.io/downloads

  A valid GCP project must already be created and cloud credentials must be
  associated with your GCP account.  If credentials haven't been authorised
  already, this command can be used:
      $ gcloud auth application-default login --project=<PROJECT_ID>

  The GCP project must have a number of APIs enabled in order to deploy and host
  the FrontEnd.  These are:
HELP1
	local aa
	for aa in "${!PRJ_API[@]}"; do
		echo "      - ${PRJ_API[${aa}]}"
	done

	cat <<HELP2

  If these are not enabled, you will be asked to confirm they can be
  enabled by this script.  Not confirming will cause the script to exit, so you
  can rectify the project manually, or start again with a different project.
  With --config, the script enables any that are missing without asking.
  If you do not have permission to modify APIs on the project, these will need
  to be added by an administrator (with Owner or Editor privileges).
  Please see the Administrator's Guide for details on the APIs.
  
  This deployment creates GCP resources, so a number of roles/permissions are
  also required on the account.  If the account is the Owner or Editor of the
  GCP host project, this is sufficient and it can deploy without any problem.
  Otherwise, a workable set of GCP roles for successful deployment need to be
  need to be added to the account, which includes:
      - Compute Admin
      - Storage Admin
      - Pub/Sub Admin
      - Create Service Accounts, Delete Service Accounts, Service Account User
        (or Service Account Admin)
      - Project IAM Admin

  Usage: ./deploy.sh [--config <path-to-config-file>] [--help]
  
    --config <path-to-config-file> : path to YAML configuration file containing deployment variables.
                                     If not specified, script will prompt user for input.
    --help                         : display this help message
  
  If --config option is used, all variables required for deployment must be specified in the YAML
  file. The script will not prompt for any input in this case.
  
  The following deployment variables are required:
  
    deployment_name:             Name of the deployment (3-${DEPLOYMENT_NAME_MAX_LENGTH} chars: lowercase letters, digits, '-')
    project_id:                  ID of the Google Cloud project
    zone:                        Zone where the deployment will be created
    django_superuser_username:   Username for the Django superuser
    django_superuser_password:   Password for the Django superuser (optional if DJANGO_SUPERUSER_PASSWORD is set)
    django_superuser_email:      Email for the Django superuser
  
  The following deployment variables are optional:
  
    subnet_name:                 Name of the subnet to use for the deployment
    dns_hostname:                Hostname to assign to the deployment's IP address
    ip_address:                  Static IP address to use for the deployment
	deployment_mode:             The mode used to deploy the FrontEnd, which must be either 'git' or 'tarball'
	repo_fork:					 The GitHub owner of the forked repo that is used for the deployment, if the 'git' deployment mode is used
	repo_branch:				 The git branch of the forked repo that is used for the deployment, if the 'git' deployment mode is used
  
  To set the Django superuser password securely, you can set the DJANGO_SUPERUSER_PASSWORD
  environment variable with the password you want to use, like this:
  
    export DJANGO_SUPERUSER_PASSWORD=my_password
  
  Replace 'my_password' with the actual password you want to use. The script will automatically
  read the password from the DJANGO_SUPERUSER_PASSWORD environment variable if it is set, and
  will fall back to the YAML file if it is not set.
  
  If you are running the script as a different user, you may need to use 'sudo -E' to preserve
  the environment variable when running the script, like this:
  
    sudo -E ./deploy.sh --config my-config.yaml
  
  This will run the script with elevated privileges (sudo) and preserve the environment variable (-E)
  so that it can be used by the script.

  Example YAML file:
  deployment_name: my-deployment
  project_id: my-project-id
  zone: us-west1-a
  subnet_name: my-subnet (optional)
  dns_hostname: myhostname.com (optional)
  ip_address: 1.2.3.4 (optional)
  django_superuser_username: sysadm
  django_superuser_password: Passw0rd! (optional if DJANGO_SUPERUSER_PASSWORD is passed)
  django_superuser_email: sysadmin@example.com
  deployment_mode: git (optional)
  repo_fork: GoogleCloudPlatform (optional)
  repo_branch: develop (optional)

HELP2
}

#
# Simple functions to direct verbose output (if on) and stderr
#
verbose() {
	if false; then
		echo ">>> $*"
	fi
}

error() {
	echo "$*" >&2
}

# ask
#
# Capture user entry.
#  - Has an option to hide the response, which is useful for passwords.
#  - Accepts a default that is used when no user entry.
#  - Note: this function is used in command substitution, i.e. foo=$(ask "bar")
#          so no echo commands can be used
#
# Usage:
#    ask [--hidden] PROMPT [DEFAULT]
#
# Example:
#    reply=$(ask "Enter string> " "foo")
#
ask() {

	# set if we are hiding the typing
	local hidden
	unset hidden
	if [[ ${1} == '--hidden' ]]; then
		hidden=1
		shift
	fi

	# get the question string
	local question="${1}"
	shift

	# get any default value
	local default
	unset default
	if [[ ${1} ]]; then
		default=${1}
		printf "%s [default: %s]" "${question}" "${default}" >&2
	else
		printf "%s" "${question}" >&2
	fi

	local charcount='0'
	local prompt='> '
	local reply=''
	local IFS
	if [[ ${hidden} ]]; then
		while IFS='' read -n '1' -p "${prompt}" -r -s 'char'; do
			case "${char}" in
			# Handle NULL
			$'\000')
				break
				;;
				# Handle BACKSPACE and DELETE
			$'\010' | $'\177')
				if ((charcount > 0)); then
					prompt=$'\b \b'
					reply="${reply%?}"
					((charcount--))
				else
					prompt=''
				fi
				;;
			*)
				prompt='*'
				reply+="${char}"
				((charcount++))
				;;
			esac
		done
		printf '\n' >&2
	else
		IFS='' read -p "${prompt}" -r 'reply'
	fi
	if [[ ! ${reply} && ${default} ]]; then
		reply=${default}
	fi

	# return string
	printf '%s\n' "${reply}"
}

# Set and check a lock file.
#   - Ensures only one instance of FE is deployed from this location.
#
setlock() {
	touch "${SCRIPT_DIR}"/tf/.tkfe.lock
}

checklock() {

	# -- If lock exists, there is a FrontEnd already deployed, so abort as
	#    deploying another will overwrite the terraform files, leaving the
	#    current deployment impossible to destroy, so then dangling and in
	#    need of manual clean-up via Google console.
	#
	if [[ -f ${SCRIPT_DIR}/tf/.tkfe.lock ]]; then
		error ""
		error "Error:  A lock file has been found."
		error ""
		error "    A FrontEnd has already been deployed from this location."
		error ""
		error "    Either destroy existing FrontEnd by deleting all resources via web"
		error "    application, then use ./teardown.sh"
		error "    Or ensure all resources have been removed via Google Console and delete the"
		error "    lock file: tf/.tkfe.lock"
		error ""
		exit 1
	fi
}

#
# Check the deployment name can be used in every resource name derived from it.
#   - The tightest limit is the service account ID "<name>-prov-sa"
#     (PROV_SA_SUFFIX), which GCP caps at 30 characters, leaving 22 for the
#     name.
#   - Service account IDs, VM names and label values do not allow '.'.
#   - Prints the reason to stderr and returns 1 if the name is invalid.
#
DEPLOYMENT_NAME_MAX_LENGTH=22

validate_deployment_name() {
	local name=$1
	local validname='^[a-z][a-z0-9-]*[a-z0-9]$'

	if [[ ${#name} -lt 3 || ${#name} -gt ${DEPLOYMENT_NAME_MAX_LENGTH} || ! ${name} =~ ${validname} ]]; then
		error "    Error: Deployment name '${name}' is invalid. It must:"
		error "              - Be 3 to ${DEPLOYMENT_NAME_MAX_LENGTH} characters long, so that the service"
		error "                account names made from it are valid"
		error "              - Only contain characters a-z (lowercase), 0-9 and '-'"
		error "              - Start with a letter and end with a letter or digit"
		return 1
	fi
	return 0
}

#
# Check that account if all good
#
check_account() {

	local project=$1
	local account

	account=$(gcloud config list --format 'value(core.account)' 2>/dev/null)

	if [[ ! ${account} ]]; then
		error ""
		error "Error: No authorized GCP account was found."
		error ""
		error "   Please ensure your account has been authorized with this command:"
		error "   $ gcloud auth application-default login --project=<PROJECT_ID>"
		error ""
		exit 1
	fi

	# Basic check that user is Owner or Editor of the project.
	# - issue warning if not - user can set permissions to do everything that
	#   is needed to deploy FrontEnd, but Owner/Editor is good by default.
	#
	local roles
	roles=$(gcloud projects get-iam-policy "${project}" \
		--flatten="bindings[].members" \
		--format="table[no-heading](bindings.role)" \
		--filter="bindings.members:${account}" | grep '^roles')
	set +e
	echo "${roles}" | grep -q 'roles/owner'
	is_owner=$?
	echo "${roles}" | grep -q 'roles/editor'
	is_editor=$?
	set -e

	if [[ ${is_owner} -ne 0 && ${is_editor} -ne 0 ]]; then
		echo ""
		echo "Warning: account is not Owner or Editor of project"
		echo "         Please ensure account has correct permissions before proceeding."
		echo "         See Cluster Toolkit FrontEnd Administrator's Guide for details."
		echo ""
		case $(ask "         Proceed [y/N] ") in
		[Yy]*) ;;
		*)
			echo "Exiting."
			exit 0
			;;
		esac
	fi

	# TODO: perform more extensive check the account has all required roles.
	#       - these could change over, depending back-end GCP / Cluster Toolkit
	#         requirements, so would require maintaining.
}

#
# Check that all required APIs are enabled for the project
# - If some are missing, ask if they would like them enabled and do so.
# - Abort if declined, as cannot deploy without them.
# - With --yes (the --config path, which cannot ask), enable them without
#   asking. Both paths call this, so they give the same result.
# - A failure to list or enable the APIs stops the script.
#
# Usage:
#    check_apis PROJECT [--yes]
#
check_apis() {

	local project=$1
	local assume_yes=${2:-}

	# Get all currently enabled APIs for this project, one name per line
	#
	local enabled_apis
	if ! enabled_apis=$(gcloud services list --enabled \
		--project="${project}" --format='value(config.name)' 2>&1); then
		error ""
		error "  Error: Unable to list the APIs enabled in project ${project}:"
		error "${enabled_apis}"
		error ""
		error "Exiting."
		exit 1
	fi

	# Check all required APIs are present, prompt and enable for any missing
	#
	local id output
	for id in "${!PRJ_API[@]}"; do
		local desc=${PRJ_API[$id]}

		verbose "checking API: ${desc}"
		if grep -qxF "${id}" <<<"${enabled_apis}"; then
			continue
		fi

		echo ""
		echo "Warning: Required API is not enabled: ${desc} (${id})"
		if [[ ${assume_yes} != '--yes' ]]; then
			read -r -p "         Enable this API? [y/N] " reply
			case "${reply}" in
			[Yy]*) ;;
			*)
				echo "Exiting."
				exit 0
				;;
			esac
		fi

		echo ""
		echo "  Enabling: ${desc}"
		echo "  This may take a few seconds..."
		# Enabling may fail if user doesn't have privileges to
		# modify the project.
		if ! output=$(gcloud services enable "${id}" \
			--project="${project}" 2>&1); then
			error ""
			error "  Error: Unable to enable ${desc} (${id}) in project ${project}:"
			error "${output}"
			error "         Please ask a project administrator to enable it."
			error ""
			error "Exiting."
			exit 1
		fi
	done
}

# The provisioning service account is <deployment_name>-prov-sa. Service
# account names are at most 30 characters, so the deployment name can be at
# most 30 - 8 = 22.
PROV_SA_SUFFIX='-prov-sa'
PROV_SA_MAX_DEPLOYMENT_NAME=$((30 - ${#PROV_SA_SUFFIX}))

#
# Create the provisioning service account, with the roles the FrontEnd needs
# to create clusters and other resources, and no key. One that already exists
# under this name (a redeploy) is reused and given the same roles.
# - Only the web server's account may act as it: tf/main.tf grants that, for
#   the e-mail written to terraform.tfvars, and each use is in the project's
#   audit logs.
# - Both the interactive and the --config paths call this, so they give the
#   same result.
# - An existing account with user-managed keys, whose holders would get the
#   roles, is reused with a warning. With --ask (the interactive path), the
#   user is asked first, and a no exits here with nothing granted. The
#   --config path cannot ask, and a key written earlier by this script at its
#   user's request is no reason to stop a redeploy, so it goes on.
# - Sets provisioning_email, which deploy() writes to terraform.tfvars.
#
# Usage:
#    create_provisioning_account PROJECT DEPLOYMENT [--ask]
#
create_provisioning_account() {

	local project=$1
	local deployment=$2
	local account="${deployment}${PROV_SA_SUFFIX}"
	local options=()
	local rc=0

	if [[ ${3:-} == "--ask" ]]; then
		options=(--ask)
	fi

	echo ""
	echo "    Creating the provisioning service account ${account}, with no key."
	echo "    This may take a few seconds..."
	bash "${SCRIPT_DIR}/script/service_account.sh" create \
		"${project}" "${account}" "${options[@]}" || rc=$?
	if [[ ${rc} -eq 3 && ${#options[@]} -gt 0 ]]; then
		echo ""
		echo "    Not deploying. The existing account ${account} was not given its"
		echo "    roles by this run. Running ./deploy.sh again checks its keys again."
		echo "Exiting."
		exit 0
	fi
	if [[ ${rc} -ne 0 ]]; then
		error ""
		error "Error:  Could not create the provisioning service account ${account}."
		if [[ ${#deployment} -gt ${PROV_SA_MAX_DEPLOYMENT_NAME} ]]; then
			error "        The deployment name must be at most ${PROV_SA_MAX_DEPLOYMENT_NAME} characters."
		fi
		error ""
		echo "Exiting."
		exit 1
	fi
	provisioning_email="${account}@${project}.iam.gserviceaccount.com"
}

#
# Write a key for the provisioning service account, only when the interactive
# user asks for one. The server does not need it.
#
write_provisioning_key() {

	local project=$1
	local deployment=$2
	local credfile=$3
	local account="${deployment}${PROV_SA_SUFFIX}"

	verbose "creating credential file: ${credfile}"
	if ! bash "${SCRIPT_DIR}/script/service_account.sh" credential \
		"${project}" "${account}" "${credfile}"; then
		error ""
		error "Error:  Could not write a key for ${account} (the error is above)."
		error "        The account ${account} exists, with its project roles."
		error "        Running this script again reuses it. To delete it and its"
		error "        project roles instead:"
		error "          script/service_account.sh delete ${project} ${account}"
		error ""
		echo "Exiting."
		exit 1
	fi
	echo ""
	echo "    Warning: A key for ${account} was written in plain text to:"
	echo "      ${credfile}"
	echo "    Anyone who holds this file can act as the account, which can"
	echo "    administer IAM in project ${project}. Keeping it safe is up to you:"
	echo "    delete the file once you no longer need it, and delete the key with"
	echo "    'gcloud iam service-accounts keys delete' when it is no longer used."
}

# Terraform addresses of the web server's service account: current, and as
# written by servers deployed before the move to the Cluster Toolkit modules
# (tf/main.tf moves it from the old address on the next apply).
FE_SA_ADDRESS='module.fe_sa.module.service_account.google_service_account.service_accounts["fe-sa"]'
FE_SA_OLD_ADDRESS='module.service_account.google_service_account.service_accounts["fe-sa"]'

# Terraform addresses of the control bucket: current, and as written by
# servers deployed before the bucket module call was renamed (tf/main.tf moves
# it from the old address on the next apply).
CONTROL_BUCKET_ADDRESS='module.control_storage.google_storage_bucket.buckets["storage"]'
CONTROL_BUCKET_OLD_ADDRESS='module.control_bucket.google_storage_bucket.buckets["storage"]'

# The waits that let IAM catch up with the account and with its grants.
FE_SA_WAITS=(
	'module.fe_sa.time_sleep.wait_for_service_account'
	'module.fe_profile.time_sleep.wait_for_iam'
)

# Project roles that servers deployed before the move to the profile modules
# gave their account and that resource-scoped grants now replace. tf/main.tf
# forgets the old grants without deleting them (a removed block), so access
# never lapses; remove_project_role_grants removes these after the upgrade
# apply. logging.logWriter and monitoring.metricWriter are kept, and
# module.fe_sa now manages them. teardown.sh has the full old list.
FE_SA_SUPERSEDED_ROLES=(
	storage.objectAdmin
	cloudtrace.agent
	pubsub.admin
	artifactregistry.admin
	secretmanager.admin
)

# tf_init
#
# Run terraform init in the current directory and report a failure clearly.
#  - -upgrade lets a lock file written for older provider versions move to the
#    versions tf/versions.tf now requires.
#  - A provider mirror configured through TF_CLI_CONFIG_FILE is used as it is.
#  - teardown.sh uses this function too (it sources this file).
#
tf_init() {
	local log="tfinit.log"

	if terraform init -input=false -upgrade >"${log}" 2>&1; then
		return 0
	fi
	error ""
	error "Error:  terraform init failed. Full output: $(pwd)/${log}"
	error "        The usual cause is a provider or module that cannot be downloaded."
	error "        Check access to registry.terraform.io, or, if you use a provider"
	error "        mirror, that it holds hashicorp/google and hashicorp/google-beta"
	error "        >= 7.33.0, hashicorp/time, hashicorp/random and hashicorp/null."
	error ""
	tail -n 20 "${log}" >&2
	return 1
}

# The Python that tf_state_attribute runs on the state: argv[1] is the
# attribute, the rest are resource addresses, and stdin is the state. Prints
# the attribute's value for the first address the state holds, as a string,
# and exits 0; exits 3 when the state holds none of the addresses; exits 1,
# with the reason on stderr, otherwise.
#  - Only managed resources, at addresses of the form
#    [module.NAME[KEY].]...TYPE.NAME[KEY], and their current (not deposed)
#    object.
#  - A state upgraded from Terraform 0.11 holds attributes_flat instead of
#    attributes; a top-level string attribute is read from either.
TF_STATE_ATTRIBUTE_PY=$(
	cat <<'PYTHON'
import json
import re
import sys

ADDRESS = re.compile(
    r"((?:module\.[A-Za-z0-9_-]+(?:\[[^\]]+\])?\.)*)"
    r"([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)(?:\[(.+)\])?"
)


def parse(address):
    match = ADDRESS.fullmatch(address)
    if not match:
        sys.exit("Not a resource address: " + address)
    module, rtype, name, key = match.groups()
    return module.rstrip("."), rtype, name, None if key is None else json.loads(key)


def instance(resources, address):
    module, rtype, name, key = parse(address)
    for resource in resources:
        if (
            isinstance(resource, dict)
            and resource.get("mode") == "managed"
            and resource.get("module", "") == module
            and resource.get("type") == rtype
            and resource.get("name") == name
        ):
            for item in resource.get("instances") or []:
                if (
                    isinstance(item, dict)
                    and "deposed" not in item
                    and item.get("index_key") == key
                ):
                    return item
    return None


attribute, addresses = sys.argv[1], sys.argv[2:]
try:
    state = json.load(sys.stdin)
except ValueError as err:
    sys.exit("The state is not valid JSON: " + str(err))
resources = state.get("resources") if isinstance(state, dict) else None
if not isinstance(resources, list):
    sys.exit("The state has no list of resources.")
for address in addresses:
    item = instance(resources, address)
    if item is None:
        continue
    attributes = item.get("attributes")
    if attributes is None:
        attributes = item.get("attributes_flat")
    value = attributes.get(attribute) if isinstance(attributes, dict) else None
    if not isinstance(value, str) or not value:
        sys.exit("The state holds " + address + " with no " + attribute + ".")
    print(value)
    sys.exit(0)
sys.exit(3)
PYTHON
)

# tf_state_attribute ATTRIBUTE ADDRESS...
#
# Run in tf/. Print ATTRIBUTE of the first ADDRESS, in order, that the
# Terraform state holds, and return 0. Return 3, printing nothing, when the
# state holds none of them or there is no state yet (a new deployment).
# Otherwise return 1, with the reason on stderr.
#  - It reads the state as stored (terraform state pull), which needs no
#    provider and works for a local or a remote backend. terraform state show
#    instead decodes a resource with the schema of the provider installed
#    now, and fails for one that an older major version of the Google
#    provider wrote, until an apply upgrades it ("schema version 0 for
#    google_storage_bucket.buckets in state does not match version 4 from
#    the provider", from Google provider 4.x to 7.x). deploy() runs
#    terraform init -upgrade first, so on the upgrade of an old deployment
#    state show cannot read its bucket or account.
#  - Only terraform's error output from state pull goes to a file (made with
#    mktemp and deleted straight after); the state passes through a pipe.
#  - It parses the state with python3 or, if there is none, with the Python
#    that gcloud runs on (the gcloud CLI needs one, and its archive install
#    bundles it). It needs neither when there is no state.
#
tf_state_attribute() {
	local attribute=${1}
	shift
	local errors state rc python

	errors=$(mktemp) || return 1
	state=$(terraform state pull 2>"${errors}")
	rc=$?
	if [[ ${rc} -ne 0 ]]; then
		echo "'terraform state pull' failed (exit status ${rc}):" >&2
		cat "${errors}" >&2
		rm -f "${errors}"
		return 1
	fi
	rm -f "${errors}"
	if [[ ! ${state} =~ [^[:space:]] ]]; then
		return 3
	fi

	python=$(command -v python3) ||
		python=$(gcloud info --format='value(basic.python_location)' 2>/dev/null)
	if [[ -z ${python} || ! -x ${python} ]]; then
		echo "Reading the Terraform state needs python3, or the Python that gcloud" >&2
		echo "runs on, and neither was found." >&2
		return 1
	fi
	# -I: ignore PYTHON* variables and the current directory's modules.
	printf '%s' "${state}" |
		"${python}" -I -c "${TF_STATE_ATTRIBUTE_PY}" "${attribute}" "$@"
}

# fe_sa_replace_flags EMAIL PROJECT
#
# Print the -replace flags, one per line, that the next apply needs.
#  - The waits after the account is created and after its grants are made run
#    only when they are created. If the account in state was deleted outside
#    Terraform, or deleted and created again under the same name (a new unique
#    id), apply recreates the account or its grants without pausing, and the
#    grants or the server can then use the account before IAM knows it.
#  - So, when the unique id in state differs from the live one, or the live
#    account is missing, the waits in state are replaced and pause again.
#  - Prints nothing for a first deploy (no account in state; the waits are
#    created anyway) or when the ids match.
#
fe_sa_replace_flags() {
	local email=${1}
	local project=${2}
	local state_list state_id live_id address in_state wait

	state_list=$(terraform state list 2>/dev/null)
	in_state=""
	for address in "${FE_SA_ADDRESS}" "${FE_SA_OLD_ADDRESS}"; do
		if grep -qxF "${address}" <<<"${state_list}"; then
			in_state=${address}
			break
		fi
	done
	if [[ -z ${in_state} ]]; then
		return 0
	fi

	# An id that cannot be read counts as changed: the only cost of replacing
	# the waits is the pause.
	if ! state_id=$(tf_state_attribute unique_id "${in_state}" 2>&1); then
		error "Could not read the unique id of ${email} from the Terraform state:"
		error "${state_id:-The state pulled does not hold ${in_state}.}"
		error "The IAM propagation waits will run again before the account is used."
	else
		live_id=$(gcloud iam service-accounts describe "${email}" \
			--project="${project}" --format='value(uniqueId)' 2>/dev/null)
		if [[ ${state_id} == "${live_id}" ]]; then
			return 0
		fi
		error "Service account ${email} is missing or was recreated outside Terraform;"
		error "the IAM propagation waits will run again before it is used."
	fi
	for wait in "${FE_SA_WAITS[@]}"; do
		if grep -qxF "${wait}" <<<"${state_list}"; then
			printf -- '-replace=%s\n' "${wait}"
		fi
	done
}

# control_bucket_location
#
# Print the location of the control bucket recorded in Terraform state, or
# nothing when the state holds no bucket (a new deployment).
#  - A bucket cannot move: Terraform replaces a bucket whose location changes,
#    and the replacement deletes every file in it (force_destroy), including
#    the files existing clusters read at boot. Servers deployed with --config
#    by earlier versions had no region set, so their bucket is in the US
#    multi-region rather than in the deployment's region. deploy() passes the
#    location printed here to Terraform as control_bucket_location, so the
#    apply keeps the bucket where it is.
#  - Reads the location with tf_state_attribute, so it works on the state of
#    a server that an older major version of the Google provider deployed.
#  - Returns non-zero, with a message that includes the underlying error, when
#    the state cannot be read or holds the bucket without a location that
#    looks like one, so that deploy() stops before the apply rather than risk
#    replacing the bucket.
#
control_bucket_location() {
	local location rc

	location=$(tf_state_attribute location \
		"${CONTROL_BUCKET_ADDRESS}" "${CONTROL_BUCKET_OLD_ADDRESS}" 2>&1)
	rc=$?
	if [[ ${rc} -eq 3 ]]; then
		# No state, or no bucket in it: a new deployment.
		return 0
	fi
	if [[ ${rc} -eq 0 && ${location} =~ ^[A-Za-z0-9-]+$ ]]; then
		printf '%s\n' "${location}"
		return 0
	fi
	if [[ ${rc} -eq 0 ]]; then
		location="The state holds an unexpected location for the bucket: '${location}'."
	fi
	error ""
	error "Error:  Could not read the control bucket's location from the"
	error "        Terraform state in $(pwd):"
	error "${location}"
	error "        Stopping before the apply: applying without the location could"
	error "        replace the bucket and delete its files."
	return 1
}

# fe_sa_inconsistent_result LOG
#
# Succeed when the apply log LOG holds the Google provider's "Provider
# produced inconsistent result after apply" error for the web server's
# service account itself, at its current or its old address. The same error
# on any other resource does not count, even when the log names the account
# elsewhere.
#  - Terraform draws a box and colour codes around each error and wraps its
#    text, so those are removed and the lines joined before matching. It
#    wraps only at spaces, so an address is never split.
#
fe_sa_inconsistent_result() {
	local log=${1}
	local text address

	text=$(sed -e $'s/\033\\[[0-9;]*m//g' -e 's/│/ /g' "${log}" | tr -s '[:space:]' ' ')
	for address in "${FE_SA_ADDRESS}" "${FE_SA_OLD_ADDRESS}"; do
		if grep -qF "Provider produced inconsistent result after apply When applying changes to ${address}," <<<"${text}"; then
			return 0
		fi
	done
	return 1
}

# -- IAM policy edits that Google refuses for a while, and their retry.
#    script/service_account.sh holds an identical copy, from IAM_RETRY_DELAYS
#    to the end of iam_policy_edit; a test checks that the two match.
#
#    A project policy edit (add- or remove-iam-policy-binding) is a
#    read-modify-write of the whole policy. Google refuses it, changing
#    nothing, with these errors, which clear on their own:
#    - "There were concurrent policy changes" (409, ABORTED): another edit
#      landed between the read and the write. gcloud does not retry it.
#    - "Deleted member '<email>?uid=<id>' is not deleted": the policy holds
#      grants of an account deleted moments ago. Until that deletion has
#      reached all of IAM, any edit of the policy, for any member, can be
#      refused; seen live for several minutes after an account was deleted.
#    - "Quota exceeded for quota metric '...' and limit '... per minute'"
#      (429, RESOURCE_EXHAUSTED): the project's per-minute Resource Manager
#      quota, shared by everything that uses the project's API, is used up.
#      It refills each minute; seen live during a teardown while other
#      deployments used the same project. Limits over longer windows, such
#      as per day, are not retried.
#    Remove the retry for an error once gcloud retries that error itself.
#
#    Seconds to wait before each retry: 8 attempts over about 2 minutes.
IAM_RETRY_DELAYS=(2 4 8 16 30 30 30)
IAM_CONFLICT_ERROR='There were concurrent policy changes'
IAM_DELETED_MEMBER_ERROR="Deleted member '[^']*' is not deleted"
IAM_QUOTA_ERROR="Quota exceeded for quota metric '[^']*' and limit '[^']*per minute[^']*'"

# iam_read COMMAND...
#
# Run COMMAND, a gcloud read of IAM data (a project policy, the list of
# service accounts), and print its output. Retry it after each of the
# IAM_RETRY_DELAYS while it fails with IAM_QUOTA_ERROR, the only error above
# that a read can get; the live 429 was on getIamPolicy. Any other error is
# final at once. On failure, print gcloud's last error to stderr and return
# its exit code.
#
iam_read() {
	local errors output rc delay reason

	errors=$(mktemp) || return 1
	for delay in "${IAM_RETRY_DELAYS[@]}" last; do
		if output=$("$@" 2>"${errors}"); then
			rm -f "${errors}"
			if [[ -n ${output} ]]; then
				printf '%s\n' "${output}"
			fi
			return 0
		else
			rc=$?
		fi
		reason=$(grep -m1 -oE -- "${IAM_QUOTA_ERROR}" "${errors}") || reason=""
		if [[ -z ${reason} || ${delay} == last ]]; then
			cat "${errors}" >&2
			rm -f "${errors}"
			return "${rc}"
		fi
		echo "    Google refused the read (${reason}); retrying in ${delay}s ..." >&2
		sleep "${delay}"
	done
}

# iam_policy_edit EXTRA_ERROR COMMAND...
#
# Run COMMAND, a gcloud IAM policy edit, quietly. Retry it after each of the
# IAM_RETRY_DELAYS while it fails with one of the errors above, or with
# EXTRA_ERROR (an extended regular expression; empty for none). Any other
# error is final at once. On failure, print gcloud's last error to stderr and
# return its exit code.
#
iam_policy_edit() {
	local extra=$1
	shift
	local retryable="${IAM_CONFLICT_ERROR}|${IAM_DELETED_MEMBER_ERROR}|${IAM_QUOTA_ERROR}"
	local output rc delay reason

	if [[ -n ${extra} ]]; then
		retryable+="|${extra}"
	fi
	for delay in "${IAM_RETRY_DELAYS[@]}" last; do
		if output=$("$@" 2>&1); then
			return 0
		else
			rc=$?
		fi
		reason=$(grep -m1 -oE -- "${retryable}" <<<"${output}") || reason=""
		if [[ -z ${reason} || ${delay} == last ]]; then
			printf '%s\n' "${output}" >&2
			return "${rc}"
		fi
		echo "    IAM refused the change (${reason}); retrying in ${delay}s ..." >&2
		sleep "${delay}"
	done
}

# remove_project_role_grants PROJECT "MEMBER..." ROLE...
#
# Remove the given project roles (without "roles/") from each member in the
# space-separated MEMBER list, for grants Terraform no longer manages. A role
# a member does not hold is skipped, so this does nothing when there is
# nothing to remove. A deleted account's grants are listed under
# "deleted:serviceAccount:<email>?uid=<unique id>", so pass that form too when
# the account may already be gone. Each removal is retried on the transient
# IAM errors (iam_policy_edit); after one fails, the rest are not tried, so
# the wait stays bounded. Returns non-zero, after printing the commands to run
# by hand, when the policy could not be read or a removal failed.
#
remove_project_role_grants() {
	local project=${1}
	local members
	read -r -a members <<<"${2}"
	shift 2
	local roles=("$@")
	local policy held role member entry
	local failed=()

	if ! policy=$(iam_read gcloud projects get-iam-policy "${project}" \
		--flatten='bindings[].members' \
		--format='value(bindings.role,bindings.members)'); then
		for role in "${roles[@]}"; do
			for member in "${members[@]}"; do
				failed+=("roles/${role} ${member}")
			done
		done
		policy=""
	fi
	# One "<role> <member>" line per grant in the policy.
	held=$(awk 'NF >= 2 { print $1 " " $2 }' <<<"${policy}")

	for role in "${roles[@]}"; do
		for member in "${members[@]}"; do
			if ! grep -qxF "roles/${role} ${member}" <<<"${held}"; then
				continue
			fi
			if [[ ${#failed[@]} -eq 0 ]]; then
				echo "Removing project role roles/${role} from ${member}"
				if iam_policy_edit "" gcloud projects remove-iam-policy-binding \
					"${project}" --member="${member}" --role="roles/${role}" \
					--condition=None; then
					continue
				fi
			fi
			failed+=("roles/${role} ${member}")
		done
	done

	if [[ ${#failed[@]} -eq 0 ]]; then
		return 0
	fi
	error ""
	error "Warning:  Could not check or remove these project roles. Remove them by hand:"
	for entry in "${failed[@]}"; do
		error "    gcloud projects remove-iam-policy-binding ${project} --member=${entry#* } --role=${entry%% *} --condition=None"
	done
	error ""
	return 1
}

#
# Pack the git checkout that holds this script for a tarball deployment, which
# uploads the archive to the control bucket.
# - Only the files git would commit go in: tracked files, with any uncommitted
#   changes, and untracked files that are not ignored. Ignored files (such as
#   website/.secret_key, key files, virtual environments and build output)
#   and the .git directory stay out. Tracked files deleted from the working
#   tree are skipped, and so are nested repositories, with a warning.
# - The archive's top directory must be named 'cluster-toolkit', as the
#   server's install depends on it, so the files are copied under that name
#   into a new staging directory (mktemp -d), which is removed afterwards.
# - Returns non-zero, with a message, if this is not a git checkout or
#   packing fails.
#
# Usage:
#    pack_deployment_tarball OUTPUT
#
pack_deployment_tarball() (
	set -o pipefail

	local output=$1
	local basedir path count=0
	# Not local: the EXIT trap that removes it runs after the function returns.
	# The function runs in a subshell, so it does not reach the caller.
	stage=''

	if ! basedir=$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel 2>&1); then
		error ""
		error "  Error: A tarball deployment packs the git checkout that holds"
		error "         this script, but ${SCRIPT_DIR} is not in one:"
		error "${basedir}"
		error "         Deploy from a git checkout, or use the git deployment mode."
		error ""
		return 1
	fi

	if ! stage=$(mktemp -d); then
		error "  Error: Unable to create a staging directory to pack the deployment."
		return 1
	fi
	trap 'rm -rf "${stage}"' EXIT

	if ! git -C "${basedir}" ls-files -z --cached --others --exclude-standard \
		>"${stage}/listed"; then
		error "  Error: Unable to list the files of the git checkout ${basedir}."
		return 1
	fi
	while IFS= read -r -d '' path; do
		if [[ -L ${basedir}/${path} || -f ${basedir}/${path} ]]; then
			printf '%s\0' "${path}"
			count=$((count + 1))
		elif [[ -d ${basedir}/${path} ]]; then
			# A submodule, or an untracked directory that holds its own
			# repository: its files are not this checkout's to pack.
			error "Warning: Not packing ${path}: it is a separate git repository."
		fi
	done <"${stage}/listed" >"${stage}/files"

	echo "Packing ${count} files from ${basedir} into ${output}"
	# Copy the listed entries only (no recursion), keeping their modes.
	if ! mkdir "${stage}/cluster-toolkit" ||
		! tar -C "${basedir}" --null --no-recursion -T "${stage}/files" -cf - |
		tar -C "${stage}/cluster-toolkit" -xpf -; then
		error "  Error: Unable to copy the files of ${basedir} to pack them."
		return 1
	fi

	if ! tar -zcf "${output}" \
		--exclude=.terraform \
		--exclude=.terraform.lock.hcl \
		--exclude=tf \
		--directory="${stage}" \
		./cluster-toolkit; then
		error "  Error: Unable to write the deployment archive ${output}."
		return 1
	fi
)

# stopped_before_apply
#
# Say what this run has already changed when deploy() stops before its
# apply. Terraform has changed no resource yet, but both setup() and
# deploy_from_config() create the provisioning account, or give an existing
# one its roles again, before deploy() runs.
#
stopped_before_apply() {
	error "        Terraform has not changed any resource. This run has already"
	error "        created the provisioning service account ${provisioning_email}"
	error "        and granted its roles (or granted them again, if it existed);"
	error "        running deploy.sh again reuses it."
}

#
# Check the state bucket name is a valid Cloud Storage bucket name.
#  - A safety net: tf/state_bucket/main.tf derives and shortens the name from
#    the deployment name; this stops before anything is created or imported
#    if that ever yields an invalid name.
#  - '.' is not allowed, as such names need domain verification.
#  - Prints the reason to stderr and returns 1 if the name is invalid.
#
BUCKET_NAME_MAX_LENGTH=63

validate_bucket_name() {

	local name=${1}
	local validname='^[a-z0-9][a-z0-9_-]*[a-z0-9]$'

	if [[ ${#name} -lt 3 || ${#name} -gt ${BUCKET_NAME_MAX_LENGTH} || ! ${name} =~ ${validname} ]]; then
		error "    Error: State bucket name '${name}' is invalid. It must:"
		error "              - Be 3 to ${BUCKET_NAME_MAX_LENGTH} characters long"
		error "              - Only contain characters a-z (lowercase), 0-9, '-' and '_'"
		error "              - Start and end with a letter or digit"
		return 1
	fi
}

#
# Prepare this deployment's state bucket working directory
#  - Each deployment name has its own tf/state_bucket/deployments/<name>/
#    holding its local state, terraform.tfvars and admin.auto.tfvars, so
#    deployments from one checkout never collide or need files moved by hand.
#  - Symlinks the tracked .tf files rather than copying them, so it never
#    runs a stale copy.
#  - Moves files an earlier release left in tf/state_bucket/ (which held a
#    single deployment) into the owning deployment's directory.
#  - Sets ${state_bucket_dir}.
#
STATE_BUCKET_ROOT=tf/state_bucket

prepare_state_bucket_dir() {

	if [[ -z ${deployment_name} || ${deployment_name} == . ||
		${deployment_name} == .. || ${deployment_name} == */* ]]; then
		echo "Error: invalid deployment name '${deployment_name}'."
		return 1
	fi

	# -- The owner is the deployment named in the terraform.tfvars deploy.sh
	#    wrote. If it is unknown, or its directory already holds any of these
	#    files, stop rather than guess: a wrong move could make a later deploy
	#    create a second bucket and orphan the state in the first.
	local -a legacy_files=(terraform.tfstate terraform.tfstate.backup terraform.tfvars admin.auto.tfvars)
	local f found=0
	for f in "${legacy_files[@]}"; do
		[[ -e ${STATE_BUCKET_ROOT}/${f} ]] && found=1
	done
	if [[ ${found} -eq 1 ]]; then
		local owner="" owner_dir
		if [[ -f ${STATE_BUCKET_ROOT}/terraform.tfvars ]]; then
			owner=$(sed -n 's|^deployment_name = "\([^"/]*\)"$|\1|p' \
				"${STATE_BUCKET_ROOT}/terraform.tfvars")
		fi
		# The bucket in the state must carry the same deployment label: an
		# earlier release rewrote terraform.tfvars before applying, so after a
		# failed deploy under another name it can name the wrong deployment.
		if [[ -n ${owner} && -f ${STATE_BUCKET_ROOT}/terraform.tfstate ]]; then
			local label
			label=$(python3 -c '
import json, sys
for r in json.load(open(sys.argv[1])).get("resources", []):
    if r.get("type") == "google_storage_bucket" and r.get("name") == "tfstate":
        for i in r.get("instances", []):
            print(i["attributes"]["labels"]["ghpcfe_id"])
' "${STATE_BUCKET_ROOT}/terraform.tfstate" 2>/dev/null) || label="?"
			[[ -n ${label} && ${label} != "${owner}" ]] && owner=""
		fi
		owner_dir=${STATE_BUCKET_ROOT}/deployments/${owner}
		for f in "${legacy_files[@]}"; do
			[[ -n ${owner} && -e ${owner_dir}/${f} ]] && owner=""
		done
		if [[ -z ${owner} ]]; then
			echo "Error: ${STATE_BUCKET_ROOT}/ holds state bucket files from an earlier"
			echo "       release, and deploy.sh cannot tell which deployment they belong to"
			echo "       or that deployment's directory already has its own. Move"
			echo "       terraform.tfstate*, terraform.tfvars and admin.auto.tfvars from"
			echo "       ${STATE_BUCKET_ROOT}/ into ${STATE_BUCKET_ROOT}/deployments/<deployment name>/"
			echo "       for the deployment they belong to (or aside, if none), and rerun."
			return 1
		fi
		echo "    Moving deployment ${owner}'s state bucket files to ${owner_dir}/"
		mkdir -p "${owner_dir}" || return 1
		for f in "${legacy_files[@]}"; do
			if [[ -e ${STATE_BUCKET_ROOT}/${f} ]]; then
				mv "${STATE_BUCKET_ROOT}/${f}" "${owner_dir}/${f}" || return 1
			fi
		done
	fi

	state_bucket_dir=${STATE_BUCKET_ROOT}/deployments/${deployment_name}
	mkdir -p "${state_bucket_dir}" || return 1
	# Relinked on every deploy, so a .tf file added or removed upstream is
	# picked up.
	find "${state_bucket_dir}" -maxdepth 1 -name '*.tf' -type l -delete || return 1
	local tf_file
	for tf_file in "${STATE_BUCKET_ROOT}"/*.tf; do
		ln -s "../../${tf_file##*/}" "${state_bucket_dir}/" || return 1
	done
}

#
# Deploy the Terraform state bucket
#  - A separate Terraform root from tf/, applied first. It holds the state of
#    every cluster, filesystem, network, image and workbench the FrontEnd
#    provisions; if that state were lost, Terraform could no longer destroy
#    them and they would keep running and billing. So teardown.sh keeps it
#    unless given --delete-state-bucket, and then only if it is empty.
#  - Works in this deployment's own directory (prepare_state_bucket_dir).
#  - Sets ${state_bucket_name} for the main deployment to consume.
#
deploy_state_bucket() {

	# The accounts deploy.sh makes writers: the provisioning account, which
	# the server acts as to run Terraform, and the <name>-tkfe-sa account
	# that earlier versions created, while it exists, because resources
	# created with its registered key keep running Terraform as it.
	local -a default_accounts=(
		"${deployment_name}${PROV_SA_SUFFIX}"
		"${deployment_name}-tkfe-sa"
	)
	local account
	local -a tfvar_args=(
		-var="project_id=${project_id}"
		-var="region=${region}"
		-var="deployment_name=${deployment_name}"
	)

	echo ""
	echo "Creating the Terraform state bucket..."

	prepare_state_bucket_dir || return 1

	# The writer list is split by owner so deploy.sh never rebuilds an
	# authoritative IAM member list from a file an administrator edits:
	# terraform.tfvars is deploy.sh's and is regenerated on every deploy
	# (default_writers); admin.auto.tfvars is the administrator's
	# (state_bucket_writers) and is created once and never read or rewritten.
	local admin_file=${state_bucket_dir}/admin.auto.tfvars
	if [[ ! -f ${admin_file} && -f ${state_bucket_dir}/terraform.tfvars ]] &&
		grep -q '^[[:space:]]*state_bucket_writers' "${state_bucket_dir}/terraform.tfvars"; then
		echo "Error: state_bucket_writers is set in ${state_bucket_dir}/terraform.tfvars, which"
		echo "       deploy.sh regenerates. Move that line into ${admin_file},"
		echo "       which deploy.sh never overwrites, and rerun."
		return 1
	fi
	local -a seed_members=()

	# -- Adopt an existing bucket. Its name is a pure function of project and
	#    deployment name (tf/state_bucket/main.tf), so a deploy from a machine
	#    without this root's local state must import it, not create a second,
	#    empty one and orphan the state the first one holds.
	#    -upgrade: init rejects a lock file from an earlier provider pin unless
	#    it is re-resolved within versions.tf's range, as on a new machine.
	terraform -chdir="${state_bucket_dir}" init -upgrade -input=false || return 1
	local bucket
	bucket=$(echo 'local.bucket_name' |
		terraform -chdir="${state_bucket_dir}" console "${tfvar_args[@]}") || return 1
	bucket=${bucket//\"/}
	if [[ -z ${bucket} ]]; then
		echo "Error: could not compute the Terraform state bucket name."
		return 1
	fi
	if ! validate_bucket_name "${bucket}"; then
		return 1
	fi

	# -- No local state is normal on a new machine. State that exists but
	#    cannot be read must stop the deploy with Terraform's own error, not be
	#    mistaken for "no state" and lead to a confusing failed import.
	local state_file=${state_bucket_dir}/terraform.tfstate
	local resources=""
	# 1 once the bucket is in this machine's state (already there, or imported
	# below); the apply-failure diagnosis depends on it.
	local managed=0
	if [[ -f ${state_file} ]] &&
		! resources=$(terraform -chdir="${state_bucket_dir}" state list 2>&1); then
		echo "Error: could not read ${state_file}:"
		echo "  ${resources//$'\n'/$'\n'  }"
		echo "       It records only the state bucket itself, not any provisioned"
		echo "       resource. Fix it, or move it aside and rerun to adopt the bucket:"
		echo "         mv ${state_file} ${state_file}.unreadable"
		return 1
	fi

	if grep -qx 'google_storage_bucket.tfstate' <<<"${resources}"; then
		# Local state from a different deployment name or project, or from a
		# bucket named before the deterministic naming, would make Terraform
		# try to replace a prevent_destroy bucket. Stop with a remedy instead.
		# An adoption whose apply failed leaves the bucket in state but no
		# outputs, so the name is then read from the local state file itself,
		# as stored: decoding it with the provider installed now (terraform
		# state show) fails for state an older provider major version wrote.
		local local_bucket
		local_bucket=$(terraform -chdir="${state_bucket_dir}" output -raw bucket_name 2>/dev/null) ||
			local_bucket=$(python3 -c '
import json, sys
for r in json.load(open(sys.argv[1])).get("resources", []):
    if r.get("mode") == "managed" and r.get("type") == "google_storage_bucket" and r.get("name") == "tfstate":
        for i in r.get("instances", []):
            if "deposed" not in i:
                print(i["attributes"]["name"])
' "${state_file}" 2>/dev/null | head -n 1)
		if [[ ${local_bucket} != "${bucket}" ]]; then
			echo "Error: ${state_bucket_dir}/terraform.tfstate on this machine belongs to"
			echo "       gs://${local_bucket:-<unknown bucket>}, but this deployment's state bucket"
			echo "       is gs://${bucket}. Move the old local files aside and rerun:"
			echo "         mv ${state_bucket_dir}/terraform.tfstate ${state_bucket_dir}/terraform.tfstate.old"
			echo "         mv ${state_bucket_dir}/terraform.tfvars ${state_bucket_dir}/terraform.tfvars.old"
			echo "         mv ${state_bucket_dir}/admin.auto.tfvars ${state_bucket_dir}/admin.auto.tfvars.old"
			echo "       The old bucket and the state it holds are not touched."
			return 1
		fi
		managed=1
	else
		# -- Is the bucket already in this project? Decided from the exit
		#    status and the listed names, never from gcloud's error wording:
		#    a failed listing (auth, permission, network) stops the deploy,
		#    a successful listing without the bucket means it is absent.
		#    Bucket names are global; if another project owns the name, the
		#    create below fails with a 409, reported before the server is built.
		local listed
		if ! listed=$(gcloud storage buckets list --project="${project_id}" \
			--filter="name=${bucket}" --format='value(name)' 2>&1); then
			echo "Error: could not determine whether gs://${bucket} already exists:"
			echo "  ${listed}"
			return 1
		fi
		if grep -qxF "${bucket}" <<<"${listed}"; then
			echo "    Found existing state bucket gs://${bucket}; adopting it."
			terraform -chdir="${state_bucket_dir}" import -input=false "${tfvar_args[@]}" \
				google_storage_bucket.tfstate "${project_id}/${bucket}" || return 1
			managed=1

			# The writer binding is authoritative, and this machine has no
			# admin.auto.tfvars recording who was granted access before. Seed
			# it from the current holders (used only if that file is absent).
			# Buckets created before writers moved to objectUser hold objectAdmin.
			local policy live_members member default_member
			policy=$(gcloud storage buckets get-iam-policy "gs://${bucket}" \
				--format=json) || return 1
			live_members=$(python3 -c '
import json, sys
seen = set()
for b in json.load(sys.stdin).get("bindings", []):
    if b.get("role") in ("roles/storage.objectUser", "roles/storage.objectAdmin"):
        for m in b.get("members", []):
            if not m.startswith("deleted:") and m not in seen:
                seen.add(m)
                print(m)
' <<<"${policy}") || return 1
			while IFS= read -r member; do
				[[ -z ${member} ]] && continue
				# deploy.sh's own writers are regenerated below, not seeded
				default_member=0
				for account in "${default_accounts[@]}"; do
					if [[ ${member} == "serviceAccount:${account}@${project_id}.iam.gserviceaccount.com" ]]; then
						default_member=1
					fi
				done
				[[ ${default_member} -eq 1 ]] && continue
				seed_members+=("${member}")
			done <<<"${live_members}"
		fi
	fi

	if [[ ! -f ${admin_file} ]]; then
		{
			echo "# Administrator-maintained settings for the state bucket. deploy.sh creates"
			echo "# this file once and never reads or overwrites it. Add the service account of"
			echo "# any provisioning credential registered from another project here, then"
			echo "# run: terraform -chdir=${state_bucket_dir} apply"
			echo "state_bucket_writers = ["
			local m
			for m in "${seed_members[@]}"; do echo "  \"${m}\","; done
			echo "]"
		} >"${admin_file}" || return 1
	fi

	# GCS rejects an IAM binding that names a nonexistent service account, so
	# each of these is granted only if it exists. The provisioning account
	# exists by now (setup() and deploy_from_config() create it first); an
	# old -tkfe-sa account normally does not. Only exit status 1 from
	# service_account.sh check means the account is absent. Any other failure
	# (2 when the accounts could not be listed, 99, a missing script or bash,
	# a signal) is not taken to be absent: default_writers is authoritative,
	# so applying without an account that exists would take away its access
	# to the state it writes.
	local default_writers="[]" sa check_rc check_error
	local -a writers=()
	for account in "${default_accounts[@]}"; do
		sa="${account}@${project_id}.iam.gserviceaccount.com"
		check_rc=0
		check_error=$(bash "${SCRIPT_DIR}/script/service_account.sh" check \
			"${project_id}" "${account}" 2>&1 >/dev/null) || check_rc=$?
		if [[ ${check_rc} -eq 0 ]]; then
			writers+=("\"serviceAccount:${sa}\"")
		elif [[ ${check_rc} -ne 1 ]]; then
			if [[ -n ${check_error} ]]; then
				printf '%s\n' "${check_error}"
			fi
			echo "Error: could not check whether the service account ${sa}"
			echo "       exists (service_account.sh check exited with status ${check_rc}),"
			echo "       so the state bucket's writers could not be chosen."
			echo "       Nothing was changed in the state bucket: gs://${bucket} was"
			echo "       not created or updated. Check that you can list the project's"
			echo "       service accounts, then run deploy.sh again:"
			echo "         gcloud iam service-accounts list --project=${project_id}"
			return 1
		elif [[ ${account} == "${deployment_name}${PROV_SA_SUFFIX}" ]]; then
			echo "    Note: ${sa} does not exist, so the state bucket grants it no"
			echo "          writer access. Once it exists, rerun deploy.sh, or add it to"
			echo "          state_bucket_writers in ${admin_file} and run:"
			echo "            terraform -chdir=${state_bucket_dir} apply"
		fi
	done
	if [[ ${#writers[@]} -gt 0 ]]; then
		default_writers="[$(
			IFS=,
			echo "${writers[*]}"
		)]"
	fi

	cat >"${state_bucket_dir}/terraform.tfvars" <<TFVARS || return 1
project_id = "${project_id}"
region = "${region}"

deployment_name = "${deployment_name}"

# Regenerated by deploy.sh on every deploy; do not edit. Add writers and any
# other overrides to admin.auto.tfvars instead.
default_writers = ${default_writers}

extra_labels = {
    creator = "${USER}"
}
TFVARS

	if ! terraform -chdir="${state_bucket_dir}" apply -auto-approve; then
		echo "Error: failed to create or update the Terraform state bucket gs://${bucket}."
		# -- If the bucket was already in this machine's state, this was an
		#    update and the name cannot be the problem. Otherwise a 409 means
		#    the name is taken: say by whom, from the exit status and the
		#    owning project number, not error wording.
		local owner this_project
		if [[ ${managed} -eq 1 ]]; then
			echo "       The bucket is already managed by this deployment; see Terraform's"
			echo "       error above (for example missing IAM permission or quota)."
		elif owner=$(gcloud storage buckets describe "gs://${bucket}" --raw \
			--format='value(projectNumber)' 2>/dev/null) && [[ -n ${owner} ]]; then
			this_project=$(gcloud projects describe "${project_id}" \
				--format='value(projectNumber)' 2>/dev/null) || this_project=""
			if [[ ${owner} == "${this_project}" ]]; then
				echo "       gs://${bucket} already exists in ${project_id}; the lookup"
				echo "       above did not list it. Rerun deploy.sh to adopt it."
			else
				echo "       gs://${bucket} already exists in another project (number"
				echo "       ${owner}). Bucket names are global, so this deployment name"
				echo "       cannot be used in ${project_id}; choose another."
			fi
		else
			echo "       If the error above is a 409, gs://${bucket} exists in a project"
			echo "       this account cannot see. Choose another deployment name."
		fi
		echo "       The FrontEnd will not be deployed without it."
		return 1
	fi

	state_bucket_name=$(terraform -chdir="${state_bucket_dir}" output -raw bucket_name)
	if [[ -z ${state_bucket_name} ]]; then
		echo "Error: the state bucket was applied but reported no name."
		return 1
	fi

	echo "    Terraform state bucket: ${state_bucket_name}"
	echo "    Note: this bucket is versioned and is kept by teardown.sh unless"
	echo "          --delete-state-bucket is given."
	echo "          Provisioning credentials for other projects must be added to"
	echo "          state_bucket_writers in ${admin_file}."
	echo ""
}

#
# Deploy the FrontEnd
#  - All deployments parameters have been obtained, so can now construct the
#    terraform recipes and initiate the deployment.
#
deploy() {

	# -- Confirm before the state bucket is created: it is protected from
	#    deletion, so declining after it exists would leave it behind.
	#
	if [ -z "$config_file" ]; then
		case $(ask "    Proceed to deploy? [y/N] ") in
		[Yy]*) ;;
		*)
			# setup() has already created the provisioning account.
			echo ""
			echo "    Not deploying. The provisioning service account"
			echo "    ${provisioning_email} exists, with its project roles."
			echo "    Running ./deploy.sh again reuses it. To delete it and its project"
			echo "    roles:"
			echo "      script/service_account.sh delete ${project_id} ${deployment_name}${PROV_SA_SUFFIX}"
			echo "    (./teardown.sh also offers to delete it, or lists it.)"
			echo "Exiting."
			exit 0
			;;
		esac
	fi

	# -- The state bucket must exist before the server is created, because the
	#    server's configuration carries its name. It is created after the
	#    provisioning account, which is one of its writers.
	#
	if ! deploy_state_bucket; then
		error ""
		error "Error:  The FrontEnd was not deployed: the Terraform state bucket could"
		error "        not be created or updated (the error is above). This run has"
		error "        already created the provisioning service account"
		error "        ${provisioning_email} and granted its roles (or granted them"
		error "        again, if it existed); running deploy.sh again reuses it."
		error ""
		exit 1
	fi

	# -- Collect deployment files
	#
	#    A tarball deployment ships the files of this git checkout, with its
	#    uncommitted changes, but none that git ignores.
	#
	if [ "${deployment_mode}" == "tarball" ]; then
		if ! pack_deployment_tarball "${SCRIPT_DIR}/tf/deployment.tar.gz"; then
			error "Exiting."
			exit 1
		fi
	fi

	# -- All Terraform operations to be done in tf subdir
	#
	(
		cd tf

		# -- Create Terraform setup
		#
		cat >terraform.tfvars <<TFVARS
project_id = "${project_id}"
region = "${region}"
zone = "${zone}"

deployment_name = "${deployment_name}"

django_su_username = "${django_superuser_username}"
django_su_password = "${django_superuser_password}"
django_su_email    = "${django_superuser_email}"

deployment_mode = "${deployment_mode}"
subnet = "${subnet_name}"

provisioning_service_account = "${provisioning_email}"
terraform_state_bucket = "${state_bucket_name}"

extra_labels = {
    creator = "${USER}"
}
TFVARS
		if [[ ${dns_hostname} ]]; then
			echo "webserver_hostname = \"${dns_hostname}\"" >>terraform.tfvars
		fi
		if [[ ${ip_address} ]]; then
			echo "static_ip = \"${ip_address}\"" >>terraform.tfvars
		fi

		if [ "${deployment_mode}" == "git" ]; then
			echo "Will clone cluster-toolkit from github.com/${repo_fork}/cluster-toolkit.git ${repo_branch} branch."

			cat <<-END >>terraform.tfvars
				repo_fork = "${repo_fork}"
				repo_branch = "${repo_branch}"
			END
		fi

		echo ""
		#    echo "terraform.tfvars file has been created in the 'tf' directory."
		#    echo "If you wish additional customization, please edit that file before continuing."
		#    echo ""

		# TODO - upload terraform files to the FE server, so that they can be recovered if ever
		#        needed

		# -- Start the deployment using Terraform.
		#    Note: Extract $? for terraform using PIPESTATUS, as $? below is
		#          from tee
		#          Also toggle exit on error.
		#
		set +e
		if ! tf_init; then
			error ""
			stopped_before_apply
			error ""
			exit 2
		fi

		# -- Keep an existing control bucket where it is (see
		#    control_bucket_location). Nothing is added for a new deployment,
		#    whose bucket is created in its region.
		if ! bucket_location=$(control_bucket_location); then
			stopped_before_apply
			error "        terraform.tfvars there was rewritten for this run without the"
			error "        bucket's location: do not run terraform apply there by hand."
			error ""
			exit 2
		fi
		if [[ -n ${bucket_location} ]]; then
			echo "Keeping the existing control bucket in its location, ${bucket_location}."
			echo "control_bucket_location = \"${bucket_location}\"" >>terraform.tfvars
		fi

		# -- Adopt a pre-existing server service account so a redeploy with the
		#    same deployment name does not fail trying to recreate it. terraform
		#    destroy normally removes it, but an aborted/partial teardown can
		#    leave it behind; importing lets apply reconcile the existing
		#    identity instead of hitting an "already exists" error.
		#    Only when the account is at neither address: a server deployed
		#    before the move to the profile modules has it at the old address,
		#    and the moved block in tf/main.tf takes it from there.
		fe_sa_email="${deployment_name}-fe-sa@${project_id}.iam.gserviceaccount.com"
		if gcloud iam service-accounts describe "${fe_sa_email}" \
			--project="${project_id}" >/dev/null 2>&1; then
			tf_state=$(terraform state list 2>/dev/null)
			if ! grep -qxF "${FE_SA_ADDRESS}" <<<"${tf_state}" &&
				! grep -qxF "${FE_SA_OLD_ADDRESS}" <<<"${tf_state}"; then
				echo "Found existing service account ${fe_sa_email}; importing into Terraform state."
				terraform import -input=false "${FE_SA_ADDRESS}" \
					"projects/${project_id}/serviceAccounts/${fe_sa_email}" || true
			fi
		fi

		# -- Make the IAM waits run again if the account changed outside
		#    Terraform (see fe_sa_replace_flags).
		mapfile -t replace_flags < <(fe_sa_replace_flags "${fe_sa_email}" "${project_id}")

		# -- Merge stderr into the piped/logged stream: terraform writes its
		#    "Error: ..." diagnostics to stderr, not stdout, so the retry
		#    detection below would never see them if only stdout were teed.
		terraform apply -auto-approve "${replace_flags[@]}" 2>&1 | tee tfapply.log
		tf_rc=${PIPESTATUS[0]}

		# -- The Google provider can return "Provider produced inconsistent
		#    result after apply" on the service account when its create races
		#    IAM's eventual consistency (e.g. reusing a recently-freed name).
		#    The account is created regardless, so a single re-apply reconciles
		#    state. Retry only when that error names the account itself (see
		#    fe_sa_inconsistent_result), not for the same error on any other
		#    resource.
		if [[ ${tf_rc} -ne 0 ]] && fe_sa_inconsistent_result tfapply.log; then
			echo "Reconciling service account state and retrying terraform apply..."
			# The failed apply actually created the account but, because of the
			# inconsistent-result error, dropped it from state. A bare re-apply
			# would then hit "409 already exists", so adopt the existing account
			# back into state before retrying.
			# As for the pre-import: only when the account is at neither
			# address, or the import would fight the moved block.
			tf_state=$(terraform state list 2>/dev/null)
			if ! grep -qxF "${FE_SA_ADDRESS}" <<<"${tf_state}" &&
				! grep -qxF "${FE_SA_OLD_ADDRESS}" <<<"${tf_state}"; then
				terraform import -input=false "${FE_SA_ADDRESS}" \
					"projects/${project_id}/serviceAccounts/${fe_sa_email}" || true
			fi
			mapfile -t replace_flags < <(fe_sa_replace_flags "${fe_sa_email}" "${project_id}")
			terraform apply -auto-approve "${replace_flags[@]}" 2>&1 | tee tfapply.log
			tf_rc=${PIPESTATUS[0]}
		fi

		if [[ ${tf_rc} -ne 0 ]]; then
			error ""
			error "Error:  Terraform failed."
			error "        Please check parameters and/or seek assistance."
			error ""
			exit 2
		fi

		# -- The new grants exist, so the project roles they replace can go.
		#    The server works with or without them, so a failure (reported
		#    with the commands to run by hand) does not fail the deployment;
		#    teardown.sh removes any that are left before deleting the account.
		remove_project_role_grants "${project_id}" "serviceAccount:${fe_sa_email}" \
			"${FE_SA_SUPERSEDED_ROLES[@]}" || true
		set -e

		echo ""
		echo "Deployment in progress."
		echo ""
		echo "  Started at: $(date)"
		echo "  Initialization should take about 15 minutes."
		echo "  After that time, please point your web browser to the above server_ip."
		if [[ ${dns_hostname} ]]; then
			echo "  Also update your DNS record to point '${dns_hostname}' to that IP."
		fi

		# TODO - Put in an optional wait, to confirm back to the user when the
		#        FE is ready
		#        A ping every 30s may work?   Need to capture IP address and
		#        then:
		#           until $?==0 do: ping -q -c 1 -w 5 IPADDRESS >/dev/null

		#
		#
		echo ""
		echo "To terminate this deployment, please make sure all resources created"
		echo "within the FrontEnd have been deleted, then run ./teardown.sh"
		echo ""

		# -- Set FE lock file, to ensure only one deployment is performed.
		#
		setlock
	)
}

#
# setup
#
# - uses question/answer to obtain all required parameters for deployment
# - checks are performed on pre-requisites/account/project
#
# - Note: take care not to declare variables local if they are used in the
#   deploy() function
#
setup() {

	# -- Ensure that there is no pre-deployed FE from this location.
	# -- Echo help to instruct user
	#
	checklock
	help
	cat <<LINE
--------------------------------------------------------------------------------

LINE

	# -- Check for terraform and the gcloud storage CLI
	#
	if ! command -v terraform &>/dev/null; then
		error "Error:"
		error "    Please ensure terraform (version 1.12.2 or later) is in your \$PATH"
		exit 1
	fi
	if ! command -v gcloud &>/dev/null || ! gcloud storage --help &>/dev/null; then
		error "Error:"
		error "    Please ensure the gcloud CLI (with the 'gcloud storage' command) is in your \$PATH"
		exit 1
	fi

	cat <<FUNDAMENTALS

* GCP deployment name, project and location

    The webserver will need a name and will be run within a specified
    project and zone.  The project must have authorization and quota to use
    resources in the zone.
    
    The deployment name must be 3 to ${DEPLOYMENT_NAME_MAX_LENGTH} characters long, contain only
    lowercase letters, numbers and '-', start with a letter and end with a
    letter or number.

FUNDAMENTALS

	# -- Name to use for this FrontEnd deployment
	#    This prefixes the server VM, service accounts, bucket and network
	while [ -z "${deployment_name}" ]; do
		deployment_name=$(ask "    Deployment name")
		echo
		if [ -z "${deployment_name}" ]; then
			echo "    Error: This cannot be left blank"
		elif ! validate_deployment_name "${deployment_name}"; then
			deployment_name=""
		fi
	done

	# -- GCP project to deploy into
	#    Offer default project from config
	#    If there is no default in config, this will check it is not blank
	#
	local default_project
	default_project=$(gcloud config list --format 'value(core.project)' 2>/dev/null)

	while [ -z "${project_id}" ]; do
		project_id=$(ask "    GCP Project ID" "${default_project}")
		if [ -z "${project_id}" ]; then
			echo "    Error: This cannot be left blank"
			echo ""
		else
			# Check project_id is valid.
			# Toggle error exit so check works.
			set +e
			if ! gcloud projects describe "${project_id}" &>/dev/null; then
				echo "    Error: Invalid project ID"
				echo "           Please check you are using the project ID and not the project name"
				echo "           and that you have access to the project"
				echo ""
				project_id=""
			fi
			set -e
		fi
	done

	# -- Check/set project APIs are all good
	#
	#echo "    (... checking project and account...)"
	verbose "checking APIs..."
	check_apis "${project_id}"

	# -- Check account is good
	#
	verbose "checking account..."
	check_account "${project_id}"

	# -- GCP zone/region to use
	#    + clip datacenter from zone to get the region
	#
	zone=$(ask "    GCP zone" "${DEFAULT_ZONE}")
	region=${zone%-*}

	cat <<SUBNET

* GCP subnet

    Enter existing subnet in which to place the webserver,
    or leave empty to have one created.

SUBNET
	subnet_name=$(ask "    GCP subnet name (or just press Enter)")

	cat <<DNSHOST
    
* DNS hostname
    
    If you specify a DNS hostname for the new webserver, we will attempt to
    acquire a TLS Certificate from LetsEncrypt.
    
    If this is left blank with no hostname specified, the server will use
    standard, unencrypted HTTP.  This is not recommended.
    
DNSHOST
	dns_hostname=$(ask "    DNS hostname (or just press Enter)")

	cat <<IPADDRESS
    
* IP address
    
    If you specify a static IP address it will be assigned to the server.
    If not specified, an ephemeral public IP address will be created.
    You can create and get a static IP address with the commands:

    $ gcloud compute addresses create <NAME> \\
           --project ${project_id} \\
           --region=${region}
    $ gcloud compute addresses describe <NAME> \\
           --project ${project_id} \\
           --region=${region} | grep 'address:'

    If you have set a DNS hostname above, it is recommended that you use a
    static IP address and set the DNS record for the hostname to point at it.
   
IPADDRESS
	ip_address=$(ask "    Static IP address (or just press Enter)")

	#
	# -- Collect Django admin user information
	#
	cat <<ADMIN

* Admin user creation

    Please give details of admin user for the web application.

ADMIN
	django_superuser_username=$(ask "    Username")
	django_superuser_password=$(ask --hidden "    Password")
	local password_check
	password_check=$(ask --hidden "    Re-enter password")

	if [[ ${django_superuser_password} != "${password_check}" ]]; then
		error "Error:"
		error "   Passwords do not match - exiting"
		exit 1
	fi

	django_superuser_email=$(ask "    Admin user email address")

	#
	# -- Create the provisioning service account; a key only if asked for
	#
	cat <<SERVICEACC

* Provisioning service account

    The FrontEnd creates clusters and other resources in this project as the
    service account ${deployment_name}${PROV_SA_SUFFIX}, which this script creates now.
    It has no key: only the FrontEnd web server's account may act as it, and
    every use is recorded in the project's audit logs.

    It can administer IAM in this project, because clusters need accounts,
    roles and grants of their own; whoever controls the FrontEnd server
    therefore controls it. Running the FrontEnd in a project of its own is
    recommended.

    Credentials for other projects can be registered with the FrontEnd once it
    is running.  Please see the Administrator's Guide for more details.

SERVICEACC
	create_provisioning_account "${project_id}" "${deployment_name}" --ask

	cat <<SERVICEKEY

    The FrontEnd does not need a key for this account. If you need one for
    another purpose, this script can write it to a file in plain text, which
    you are then responsible for keeping safe. This is not recommended.

SERVICEKEY
	case $(ask "    Write a key file for this account? [y/N]: ") in
	[Yy]*)
		write_provisioning_key "${project_id}" \
			"${deployment_name}" \
			"${CREDENTIAL_FILE}"
		;;
	*) ;;
	esac

	echo ""
	echo "Please select deployment method of server software:"
	echo "  1) Clone the git repo when server deploys"
	echo "  2) Use a copy of the code from this computer"
	deploy_choice=$(ask "  Please choose one of the above options", "1")
	if [ "${deploy_choice}" == "1" ]; then
		deployment_mode="git"
		repo_fork=$(ask "Please specify the forked repo owner (or just press Enter)", "GoogleCloudPlatform")
		repo_branch=$(ask "Please specify the forked repo branch (or just press Enter)", "main")
	elif [ "${deploy_choice}" == "2" ]; then
		deployment_mode="tarball"
	else
		error "Invalid selection"
		exit 1
	fi

	# -- Summarise entered parameters back to user
	#
	echo ""
	echo "***  Deployment summary:  ***"
	echo ""
	echo "    Deploymnet mode:  ${deployment_mode}"
	echo "    Deployment name:  ${deployment_name}"
	echo "    GCP project ID:   ${project_id}"
	echo "    GCP zone:         ${zone}"
	if [[ ${subnet_name} ]]; then
		echo "    GCP subnet:       ${subnet_name}"
	else
		echo "    GCP subnet:       Automatically created"
	fi
	if [[ ${dns_hostname} ]]; then
		echo "    DNS hostname:     ${dns_hostname}"
	else
		echo "    DNS hostname:     None - will use standard HTTP"
	fi
	if [[ ${ip_address} ]]; then
		echo "    IP address:       ${ip_address}"
	else
		echo "    IP address:       Automatically created"
	fi
	echo ""
	echo "    Admin username:   ${django_superuser_username}"
	echo "    Admin email:      ${django_superuser_email}"
	echo ""
}

deploy_from_config() {
	# Read the YAML file as an associative array
	declare -A yaml_array
	while IFS='=: ' read -r key value; do
		[[ -n $key ]] && yaml_array[$key]=$value
	done < <(grep -vE '^#|^\s*$' "$config_file" | sed -n '/:/p')

	local required_params=("deployment_name" "project_id" "zone" "django_superuser_username" "django_superuser_email")
	# Check that all required parameters are present in the YAML file
	for param in "${required_params[@]}"; do
		if [[ -z ${yaml_array[$param]} ]]; then
			echo "Required parameter '$param' not found in YAML file"
			exit 1
		fi
	done

	# Set variables based on keys in the array
	deployment_name=${yaml_array[deployment_name]}
	if ! validate_deployment_name "${deployment_name}"; then
		exit 1
	fi
	project_id=${yaml_array[project_id]}
	zone=${yaml_array[zone]}
	region=${zone%-*}
	subnet_name=${yaml_array[subnet_name]}
	dns_hostname=${yaml_array[dns_hostname]}
	ip_address=${yaml_array[ip_address]}
	django_superuser_username=${yaml_array[django_superuser_username]}
	django_superuser_email=${yaml_array[django_superuser_email]}
	deployment_mode=${yaml_array[deployment_mode]:-tarball}
	repo_fork=${yaml_array[repo_fork]:-GoogleCloudPlatform}
	repo_branch=${yaml_array[repo_branch]:-main}

	# Set password from environment variable if it exists, otherwise from YAML file
	if [[ -n ${DJANGO_SUPERUSER_PASSWORD+x} ]]; then
		echo "Django superuser password environment variable detected"
		django_superuser_password=${DJANGO_SUPERUSER_PASSWORD}
	else
		if [[ -z ${yaml_array[django_superuser_password]} ]]; then
			echo "Required parameter 'django_superuser_password' not found in YAML file"
			echo "DJANGO_SUPERUSER_PASSWORD environment variable is not set"
			exit 1
		else
			django_superuser_password=${yaml_array[django_superuser_password]}
		fi
	fi

	# Print deployment summary
	echo ""
	echo "***  Deployment summary:  ***"
	echo ""
	echo "    Deploymnet mode:  ${deployment_mode}"
	echo "    Deployment name:  ${deployment_name}"
	echo "    GCP project ID:   ${project_id}"
	echo "    GCP zone:         ${zone}"
	if [[ ${subnet_name} ]]; then
		echo "    GCP subnet:       ${subnet_name}"
	else
		echo "    GCP subnet:       Automatically created"
	fi
	if [[ ${dns_hostname} ]]; then
		echo "    DNS hostname:     ${dns_hostname}"
	else
		echo "    DNS hostname:     None - will use standard HTTP"
	fi
	if [[ ${ip_address} ]]; then
		echo "    IP address:       ${ip_address}"
	else
		echo "    IP address:       Automatically created"
	fi
	echo ""
	echo "    Admin username:   ${django_superuser_username}"
	echo "    Admin email:      ${django_superuser_email}"
	echo ""

	# The same API check the interactive path makes, enabling any missing API
	# without asking (this path cannot ask). An existing deployment is upgraded
	# through this path, so this also enables APIs a newer version needs.
	check_apis "${project_id}" --yes

	# The same keyless provisioning account the interactive path creates. This
	# path cannot ask: an existing account with user-managed keys is given its
	# roles, with a warning naming each key.
	create_provisioning_account "${project_id}" "${deployment_name}"

	deploy
}

################################################################################
#

# -- When sourced (teardown.sh does, for the shared functions above), stop
#    here: everything below runs a deployment.
if [[ ${BASH_SOURCE[0]} != "${0}" ]]; then
	return 0
fi

# -- Exit on any errors by default (will toggle on/off throughout this script)
set -e

cat <<HEADER

--------------------------------------------------------------------------------

                               Cluster Toolkit FrontEnd

--------------------------------------------------------------------------------

HEADER

while [[ ${#} -gt 0 ]]; do
	case "${1}" in
	-h | --help)
		help
		exit 0
		;;
	--config)
		if [[ ${2} == "" ]]; then
			echo "Error: --config option requires an argument - path to deployment config. "
			exit 1
		fi
		if [[ ! -e ${2} ]]; then
			echo "Error: ${2} is not a valid Unix path"
			exit 1
		fi
		echo "Deploying OFE from the config file."
		config_file=${2}
		deploy_from_config
		exit 0
		;;
	*)
		echo "Error: Unknown option: ${1}"
		exit 1
		;;
	esac
done

setup
deploy

wait
exit 0

#
# eof
