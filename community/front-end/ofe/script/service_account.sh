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
#
# Contains functions to manage a service account needed to deploy resources
# within TKFE.  Service accounts can be created for any project the user has
# permission.
#
#   list       - list all service accounts associated with a project (excluding
#                those automatically created by GCP APIs)
#   check      - checks if a service account already exists
#   create     - create a service account with all required roles
#   update     - add any missing required roles to an existing service account
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
	'serviceusage.serviceUsageConsumer'
	'servicenetworking.networksAdmin'
	'artifactregistry.admin'
	'cloudbuild.builds.builder'
	# Customer-managed encryption keys: OFE creates a CryptoKey per encrypted
	# resource, grants the service agents on it through kms-key-iam, and can
	# schedule its versions for destruction when an admin revokes it. Without
	# this every encrypted resource fails at apply. cloudkms.admin has no
	# encrypt/decrypt permission, so OFE does not decrypt data in normal
	# operation, but it is not a hard barrier: it can change key IAM policies,
	# and this account also holds resourcemanager.projectIamAdmin. Enforce that
	# guarantee with organisation policy or IAM conditions if it is needed.
	'cloudkms.admin'
)

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
# Grant every role in SA_ROLES to an account. Safe to repeat: a role the
# account already has is left as it is, and no role is ever removed.
#
grant_service_account_roles() {

	local project=${1}
	local account=${2}
	local sa_fullname
	local role

	sa_fullname=$(sa_expand "${project}" "${account}")

	# ---- Configure number of retries ----
	local max_retries=3
	local attempt
	local success

	for role in "${SA_ROLES[@]}"; do
		success=false
		attempt=1
		while [[ ${attempt} -le ${max_retries} ]]; do
			if gcloud projects add-iam-policy-binding "${project}" \
				--member="serviceAccount:${sa_fullname}" \
				--role="roles/${role}" >/dev/null 2>&1; then
				success=true
				break
			else
				error "Failed to assign role [roles/${role}] on attempt ${attempt}."
				if [[ ${attempt} -lt ${max_retries} ]]; then
					error "Retrying in a few seconds..."
					sleep 3
				fi
			fi
			((attempt++))
		done

		if [[ ${success} != true ]]; then
			error "All $max_retries attempts to assign [roles/${role}] failed. Exiting."
			return 1
		fi
	done

	return 0
}

#
#
create_service_account() {

	local project=${1}
	local account=${2}

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
	echo "Creating service account [${account}] in project [${project}] ..."
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
	# - retries in case of delay in SA creation propagating to gcloud
	#
	grant_service_account_roles "${project}" "${account}" || return 1
	return 0
}

#
# Exact lookup by email. The default table output of gcloud puts the display
# name in the first column, so an account with a display name is not found by
# check_service_account; this asks gcloud for the email column only and
# matches the whole line, so a name that is a prefix or suffix of another
# account, or contains a dot, cannot match the wrong one.
#
service_account_exists() {

	local project=${1}
	local account=${2}
	local sa_fullname

	sa_fullname=$(sa_expand "${project}" "${account}")
	gcloud iam service-accounts list --project="${project}" \
		--format='value(email)' 2>"${devnull}" | grep -Fxq "${sa_fullname}"
}

#
# Bring an existing service account up to date with the roles TKFE needs now.
# An account made by an earlier version can lack roles added since (for
# example cloudkms.admin, needed for customer-managed encryption keys).
# Only adds missing roles; never removes one.
#
update_service_account_roles() {

	local project=${1}
	local account=${2}

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

	if ! service_account_exists "${project}" "${account}"; then
		error "Error: Service account [${account}] not found in project [${project}]."
		return 1
	fi

	echo "Ensuring service account [${account}] has all required roles in project [${project}] ..."
	grant_service_account_roles "${project}" "${account}" || return 1
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
	#    A removal can fail transiently, so each one is retried.
	#
	local max_retries=3
	local attempt
	local success
	for role in ${roles}; do
		success=false
		attempt=1
		while [[ ${attempt} -le ${max_retries} ]]; do
			if gcloud projects remove-iam-policy-binding "${project}" \
				--member="serviceAccount:${sa_fullname}" \
				--role="${role}" &>${devnull}; then
				success=true
				break
			fi
			if [[ ${attempt} -lt ${max_retries} ]]; then
				sleep 3
			fi
			((attempt++))
		done

		if [[ ${success} != true ]]; then
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

- Add any missing required roles to an existing service account (never removes
  a role):
      $ ./service_account.sh update PROJECT_ID ACCOUNT

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
"update")
	update_service_account_roles "${project}" "${account}"
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
