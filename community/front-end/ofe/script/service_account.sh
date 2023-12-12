#! /bin/bash
# Copyright 2022 Google LLC
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
#
# Contains functions to manage a service account needed to deploy resources
# within TKFE.  Service accounts can be created for any project the user has
# permission.
#
#   list       - list all service accounts associated with a project (excluding
#                those automatically created by GCP APIs)
#   check      - checks if a service account already exists
#   create     - create a service account with all required roles
#   delete     - deletes a service account
#   credential - create a credential file for existing service account
#                (file contents are then registered within TKFE)
#
################################################################################

# -- Set a var for /dev/null
#    Makes it easier to send stdout/stderr to a log file for debugging
#
devnull=/dev/null

#
#
error() {
	echo "$*" >&2
}

# functions to return service account full name
# - used as function command, so echo is return value
sa_expand() {
	local project=${1}
	local account=${2}
	echo "${account}@${project}.iam.gserviceaccount.com"
}

# Roles needed by service account to deploy resources from TKFE
#
declare -a SA_ROLES
SA_ROLES=('aiplatform.admin'
	'compute.admin'
	'storage.admin'
	'file.editor'
	'iam.serviceAccountAdmin'
	'iam.serviceAccountUser'
	'notebooks.admin'
	'resourcemanager.projectIamAdmin'
	'monitoring.viewer'
	'pubsub.admin'
	'cloudsql.admin'
	'bigquery.admin'
	'secretmanager.admin'
	'servicenetworking.networksAdmin')

#
#
list_service_accounts() {

	local project=${1}

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		help
		return 99
	fi

	# -- Get list of service accounts, but filter for those created explicitly by
	#    the project
	#    (i.e. exclude ones like NUMBER-compute@developer.gserviceaccount.com)
	#
	#    Just output the account name, without any @...
	#
	list=$(gcloud iam service-accounts list \
		--project="${project}" --format="csv(email)" 2>${devnull} |
		awk -v PROJ="@${project}" '$1 ~ PROJ')
	if [[ -n ${list} ]]; then
		for li in ${list}; do
			echo "${li}" | cut -d'@' -f1
		done
		return 0
	else
		return 1
	fi
}

#
#
check_service_account() {

	local project=${1}
	local account=${2}
	local sa_fullname
	local exists

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		help
		return 99
	fi
	if [[ -z ${account} ]]; then
		error "Error: Must specify ACCOUNT"
		help
		return 99
	fi

	sa_fullname=$(sa_expand "${project}" "${account}")

	exists=$(gcloud iam service-accounts list --project="${project}" |
		awk -v SA="${sa_fullname}" '$1 ~ SA')
	if [[ -n "${exists}" ]]; then
		return 0
	else
		return 1
	fi
}

#
#
create_service_account() {

	local project=${1}
	local account=${2}
	local sa_fullname
	local role

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		help
		return 99
	fi
	if [[ -z ${account} ]]; then
		error "Error: Must specify ACCOUNT"
		help
		return 99
	fi

	# Create the service account
	# - may fail if user doesn't have permissions
	#
	set +e
	if ! gcloud iam service-accounts create "${account}" \
		--description="TKFE service account" --project="${project}"; then
		error ""
		error "Error: Failed to create service account."
		error "       Your account may not have permissions to do this."
		error "       Please contact your administrator."
		error ""
		return 1
	fi
	set -e

	# Add all required roles to new service account
	# - can assume we can do this if account creation above works
	#
	sa_fullname=$(sa_expand "${project}" "${account}")
	for role in "${SA_ROLES[@]}"; do
		gcloud projects add-iam-policy-binding "${project}" \
			--member="serviceAccount:${sa_fullname}" \
			--role="roles/${role}" &>${devnull}
	done

	return 0
}

#
#
delete_service_account() {

	local project=${1}
	local account=${2}
	local sa_fullname
	local roles
	local role

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		help
		return 99
	fi
	if [[ -z ${account} ]]; then
		error "Error: Must specify ACCOUNT"
		help
		return 99
	fi

	sa_fullname=$(sa_expand "${project}" "${account}")

	set +e

	#
	# WARNING: we assume this is a valid service account, with only roles as set
	#          by the create_service_account function.
	#
	#          If this is used with a non-service account, or a service account
	#          with higher privileges, the roles will be removed.
	#

	# -- get list of all current roles
	#
	roles=$(gcloud projects get-iam-policy "${project}" \
		--flatten="bindings[].members" \
		--format="table[no-heading](bindings.role)" \
		--filter="bindings.members:${sa_fullname}")

	# -- Remove each role associated with the account
	#    (Deleting an account with associated roles outstanding leaves the roles
	#    hanging in a "deleted:serviceAccount" within IAM admin.)
	#
	for role in ${roles}; do
		if ! gcloud projects remove-iam-policy-binding "${project}" \
			--member="serviceAccount:${sa_fullname}" \
			--role="${role}" &>${devnull}; then
			error ""
			error "    Warning: Failed to remove IAM policy: ${role}"
			error "             Continuing..."
			error ""
		fi
	done

	# -- Delete the service account
	#
	if ! gcloud iam service-accounts delete "${sa_fullname}" \
		--quiet --project="${project}" &>${devnull}; then
		error ""
		error "Error: Failed to delete service account."
		error "       Your account may not have permissions to do this."
		error "       Please contact your administrator."
		error ""
		return 1
	fi

	set -e

	return 0
}

#
#
create_service_account_credential() {

	local project=${1}
	local account=${2}
	local credfile=${3}
	local sa_fullname

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		help
		return 99
	fi
	if [[ -z ${account} ]]; then
		error "Error: Must specify ACCOUNT"
		help
		return 99
	fi
	if [[ -z ${credfile} ]]; then
		error "Error: Must specify PATH_TO_FILE for credential"
		help
		return 99
	fi

	sa_fullname="${account}@${project}.iam.gserviceaccount.com"

	# Create credential for new service account and put into file
	#
	set +e

	if ! gcloud iam service-accounts keys create "${credfile}" \
		--iam-account="${sa_fullname}" >${devnull} 2>&1; then
		return 1
	fi

	set -e

	return 0
}

#
#
help() {
	cat <<HELP

Utility functions to manage service accounts & credential for TKFE
Usage:

- List all service accounts in a project:
      $ ./service_account.sh list PROJECT_ID

- Check if an account already exists:
      $ ./service_account.sh check PROJECT_ID ACCOUNT
      Returns exit code of 0 if account exists, otherwise 1

- Create a new service account - required roles will be added:
      $ ./service_account.sh create PROJECT_ID ACCOUNT

- Delete a service account:
      $ ./service_account.sh delete PROJECT_ID ACCOUNT

- Create a credential file (json format) for an existing service account:
      $ ./service_account.sh credential PROJECT_ID ACCOUNT PATH_TO_FILE


  PROJECT_ID   = GCP Project ID (not name)
  ACCOUNT      = service account name (only name, not @...com)
  PATH_TO_FILE = location to write credential file (will overwrite)

  Exit code of 99 is returned on any error.

HELP
}

#
#
option="${1}"
project="${2}"
account="${3}"
filename="${4}"

case "${option}" in
"-h" | "--help")
	help
	rc=0
	;;
"list")
	list_service_accounts "${project}"
	;;
"check")
	check_service_account "${project}" "${account}"
	rc=$?
	;;
"create")
	create_service_account "${project}" "${account}"
	rc=$?
	;;
"delete")
	delete_service_account "${project}" "${account}"
	rc=$?
	;;
"credential")
	create_service_account_credential "${project}" "${account}" "${filename}"
	rc=$?
	;;
*)
	error "Error: Unrecognised option to service_account.sh"
	help
	exit 99
	;;
esac

exit "$rc"

#
# eof
