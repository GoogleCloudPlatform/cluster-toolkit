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
#   create     - create a service account, with no key, and grant it all
#                required roles (an existing account is reused, with a
#                warning about its user-managed keys)
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

# -- IAM policy edits that Google refuses for a while, and their retry.
#    deploy.sh holds an identical copy, from IAM_RETRY_DELAYS to the end of
#    iam_policy_edit; a test checks that the two match.
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

# functions to return service account full name
# - used as function command, so echo is return value
sa_expand() {
	local project=${1}
	local account=${2}
	echo "${account}@${project}.iam.gserviceaccount.com"
}

# Roles needed by service account to deploy resources from TKFE
#
# Each role is there for the Terraform, Packer or API calls named in its
# comment: the cluster blueprints (website/ghpcfe/templates/blueprint and
# cluster_manager/clusterinfo.py), images (cluster_manager/image.py),
# filesystems (cluster_manager/filesystem.py), VPCs
# (infrastructure_files/vpc_tf) and workbenches
# (infrastructure_files/workbench_tf), and the Cluster Toolkit modules they use.
#
# The IAM administration roles at the end are granted without IAM conditions:
# a condition cannot limit them to the accounts, custom roles and grants that
# OFE creates (it cannot match a role-id or account-name prefix). Only the
# FrontEnd web server's account can act as this account, and only without a
# key; run the FrontEnd in a project of its own to limit what it can reach.
#
declare -a SA_ROLES
SA_ROLES=(
	# VMs, templates, disks, addresses, firewalls, health checks: slurm-gcp-v6 controller, login, nodesets, cleanup; vpc_tf; modules/network/vpc; Packer image builds; workbench OS Login binding
	'compute.admin'
	# Buckets, objects and bucket IAM: slurm-gcp-v6 controller bucket, startup-script configs bucket, profiles bucket_bindings, workbench startup-script object
	'storage.admin'
	# google_filestore_instance: modules/file-system/filestore (filesystem.py), and listing Filestore instances (cloud_info.py)
	'file.editor'
	# google_notebooks_instance and listing its locations: workbench_tf, cloud_info.py
	'notebooks.admin'
	# google_service_networking_connection: vpc_tf
	'servicenetworking.networksAdmin'
	# google_sql_database_instance, google_sql_database, google_sql_user: slurm-cloudsql-federation
	'cloudsql.admin'
	# google_bigquery_connection: slurm-cloudsql-federation
	'bigquery.connectionAdmin'
	# Secrets, versions and secret IAM: artifact-registry repository password, slurm-gcp-v6 controller Cloud SQL secret
	'secretmanager.admin'
	# Repositories and repository IAM: artifact-registry module, profiles artifact_registry_bindings; listing and deleting images (views/containers.py)
	'artifactregistry.admin'
	# Cloud Build builds that copy images into a registry: views/containers.py
	'cloudbuild.builds.editor'
	# Topic and subscription IAM on the FrontEnd's command-and-control topic and each cluster's subscription: profiles pubsub_*_bindings
	'pubsub.admin'
	# Grafana's Cloud Monitoring datasource, when this account's key is registered (grafana.py)
	'monitoring.viewer'
	# Enabled-API checks and quota project: gcluster validators, service-enablement, workbench_tf google_project_service
	'serviceusage.serviceUsageConsumer'
	# google_service_account and service-account IAM: modules/project/service-account, profiles act_as_service_accounts, workbench_tf
	'iam.serviceAccountAdmin'
	# actAs: VMs and templates run as the cluster, image-builder, workbench and Cloud Build accounts
	'iam.serviceAccountUser'
	# google_project_iam_custom_role: service-account-profiles (one custom role per cluster and image account)
	'iam.roleAdmin'
	# google_project_iam_member: modules/project/service-account project_roles, service-account-profiles role grants, workbench_tf
	'resourcemanager.projectIamAdmin'
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
	#    A list that fails is an error, with gcloud's error printed, not an
	#    empty project; a project with no such account prints nothing.
	#
	local listed

	listed=$(iam_read gcloud iam service-accounts list --project="${project}" \
		--format='value(email)') || return 1
	awk -F'@' -v domain="${project}.iam.gserviceaccount.com" \
		'$2 == domain { print $1 }' <<<"${listed}"
}

#
# Exact lookup by email. The default table output of gcloud puts the display
# name in the first column, so a first-column match misses an account with a
# display name; this asks gcloud for the email column only and matches the
# whole line, so a name that is a prefix or suffix of another account, or
# contains a dot, cannot match the wrong one.
#
service_account_exists() {

	local project=${1}
	local account=${2}
	local sa_fullname

	local listed

	sa_fullname=$(sa_expand "${project}" "${account}")
	# A list that fails is not an absent account: 2, with gcloud's error
	listed=$(iam_read gcloud iam service-accounts list --project="${project}" \
		--format='value(email)') || return 2
	grep -Fxq "${sa_fullname}" <<<"${listed}"
}

#
#
check_service_account() {

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

	service_account_exists "${project}" "${account}"
}

# warn_user_managed_keys PROJECT EMAIL
#
# Before an existing account is given the roles, list its user-managed keys
# (made with 'gcloud iam service-accounts keys create', for example by
# deploy.sh when its user asks for a key file). Anyone who holds one gets the
# roles, IAM administration among them. Return 0 when it has none; 1, after a
# warning naming each key with the command that deletes it, when it has some;
# 2, after a warning that the keys were not checked, when they cannot be
# listed (gcloud's error is printed).
#
warn_user_managed_keys() {
	local project=${1}
	local sa_fullname=${2}
	local listed name

	if ! listed=$(iam_read gcloud iam service-accounts keys list \
		--iam-account="${sa_fullname}" --managed-by=user \
		--format='value(name)'); then
		error ""
		error "Warning: Could not list the user-managed keys of ${sa_fullname}"
		error "         (the error is above), so whether anyone holds a key to it was"
		error "         not checked. Anyone who holds one gets the roles granted here,"
		error "         including IAM administration in project ${project}. To check:"
		error "    gcloud iam service-accounts keys list --iam-account=${sa_fullname} --managed-by=user"
		error ""
		return 2
	fi
	if [[ -z ${listed} ]]; then
		return 0
	fi
	error ""
	error "Warning: ${sa_fullname} already has user-managed keys:"
	while IFS= read -r name; do
		error "    ${name##*/}"
	done <<<"${listed}"
	error "         Anyone who holds one of these keys gets the roles granted here,"
	error "         including IAM administration in project ${project}. Delete each"
	error "         key that is no longer needed:"
	while IFS= read -r name; do
		error "    gcloud iam service-accounts keys delete ${name##*/} --iam-account=${sa_fullname}"
	done <<<"${listed}"
	error ""
	return 1
}

#
# create_service_account PROJECT ACCOUNT [--ask]
#
# With --ask, when ACCOUNT already exists and has user-managed keys, or its
# keys cannot be listed, ask before granting the roles; return 3, granting
# nothing, unless the answer is yes. Without it, warn and grant. Return 2,
# creating and granting nothing, when whether ACCOUNT exists cannot be
# checked.
#
create_service_account() {

	local project=${1}
	local account=${2}
	local ask=${3}
	local sa_fullname
	local role
	local reply

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
	if [[ -n ${ask} && ${ask} != "--ask" ]]; then
		error "Error: Unrecognised option to create: ${ask}"
		help
		return 99
	fi

	# Google's rule for service account names, checked before any change is
	# made so that an invalid name is reported clearly.
	if [[ ! ${account} =~ ^[a-z][-a-z0-9]{4,28}[a-z0-9]$ ]]; then
		error ""
		error "Error: Invalid service account name [${account}] (${#account} characters)."
		error "       A name must be 6 to 30 characters long, start with a lowercase"
		error "       letter, contain only lowercase letters, digits and dashes, and end"
		error "       with a letter or digit."
		error ""
		return 99
	fi

	sa_fullname=$(sa_expand "${project}" "${account}")

	# Create the service account, with no key: whoever uses it either acts as
	# it (impersonation) or creates a key separately (the credential option).
	# An account that already exists, for example from an earlier deployment
	# with the same name, is reused and given the roles below, after a warning
	# about any user-managed key it has (warn_user_managed_keys). Such a key
	# is not refused: deploy.sh writes one to a file when its user asks.
	# Only NOT_FOUND, the status gcloud names in its error when the account
	# does not exist, means at once that it is absent. Any other failure says
	# nothing on its own: describe of an account deleted moments ago fails
	# with PERMISSION_DENIED "... (or it may not exist)" (seen live), and no
	# permission to read it, a network error or a quota say nothing either.
	# Then the project's account list decides, read as check reads it
	# (service_account_exists): listed, the account is reused; not listed, it
	# is created. If the list fails too, stop with 2, as check does, and print
	# gcloud's errors, rather than try to create an account that may exist.
	set +e
	local describe_error describe_rc=0 exists_rc=0
	describe_error=$(gcloud iam service-accounts describe "${sa_fullname}" \
		--project="${project}" 2>&1 >${devnull}) || describe_rc=$?
	if [[ ${describe_rc} -eq 0 ]]; then
		exists_rc=0
	elif grep -qF 'NOT_FOUND' <<<"${describe_error}"; then
		exists_rc=1
	else
		service_account_exists "${project}" "${account}" || exists_rc=$?
	fi
	if [[ ${exists_rc} -ne 0 && ${exists_rc} -ne 1 ]]; then
		error "${describe_error}"
		error ""
		error "Error: Could not check whether service account ${sa_fullname}"
		error "       exists (errors above: neither reading it nor listing the"
		error "       project's service accounts worked), so it was neither created"
		error "       nor given any role. Check that you can read the project's"
		error "       service accounts, then run this command again."
		error ""
		return 2
	fi
	if [[ ${exists_rc} -eq 0 ]]; then
		echo "Service account [${account}] already exists in project [${project}]; reusing it ..."
		if ! warn_user_managed_keys "${project}" "${sa_fullname}" &&
			[[ -n ${ask} ]]; then
			printf '%s' "    Continue and grant the roles to this account? [y/N] "
			read -r reply || reply=""
			echo ""
			case "${reply}" in
			[Yy]*) ;;
			*)
				echo "    No role was granted to ${sa_fullname}."
				return 3
				;;
			esac
		fi
	else
		echo "Creating service account [${account}] in project [${project}] ..."
		if ! gcloud iam service-accounts create "${account}" \
			--description="TKFE service account" --project="${project}"; then
			error ""
			error "Error: Failed to create service account."
			error "       Your account may not have permissions to do this."
			error "       Please contact your administrator."
			error ""
			return 1
		fi
	fi
	set -e

	# Add all required roles to the service account.
	# - A grant made seconds after the account was created can be refused
	#   with INVALID_ARGUMENT "Service account <email> does not exist" until
	#   the new account has reached all of IAM (seen live in a project other
	#   than the deployer's). iam_policy_edit retries that error, for this
	#   account only, besides the transient policy errors above. Remove this
	#   retry if 'gcloud iam service-accounts create' starts returning only
	#   once the account can be granted roles.
	# - --condition=None: an unconditioned grant. Without it gcloud refuses,
	#   when not interactive, to add a binding to a project policy that
	#   already holds conditional bindings.
	#
	local not_yet_visible="Service account ${sa_fullname//./\\.} does not exist"

	for role in "${SA_ROLES[@]}"; do
		if ! iam_policy_edit "${not_yet_visible}" \
			gcloud projects add-iam-policy-binding "${project}" \
			--member="serviceAccount:${sa_fullname}" \
			--role="roles/${role}" --condition=None; then
			error ""
			error "Error: Could not grant [roles/${role}] to ${sa_fullname} (error above)."
			error "       A later create with the same name reuses the account and"
			error "       grants every role."
			error ""
			return 1
		fi
	done

	return 0
}

#
#
delete_service_account() {

	local project=${1}
	local account=${2}
	local sa_fullname
	local member
	local policy
	local roles
	local role
	local failed=()

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
	member="serviceAccount:${sa_fullname}"

	set +e

	#
	# WARNING: we assume this is a valid service account, with only roles as set
	#          by the create_service_account function.
	#
	#          If this is used with a non-service account, or a service account
	#          with higher privileges, the roles will be removed.
	#

	# -- get list of all current roles: those granted to the account itself,
	#    not to a "deleted:" member that the filter also matches
	#
	if ! policy=$(iam_read gcloud projects get-iam-policy "${project}" \
		--flatten="bindings[].members" \
		--format="value(bindings.role,bindings.members)" \
		--filter="bindings.members:${sa_fullname}"); then
		error ""
		error "Error: Could not read the IAM policy of project ${project}, so"
		error "       ${sa_fullname} was not deleted: its project roles must be"
		error "       removed first. Run this command again later."
		error ""
		return 1
	fi
	roles=$(awk -v member="${member}" '$2 == member && !seen[$1]++ { print $1 }' \
		<<<"${policy}")

	# -- Remove each role associated with the account
	#    (Deleting an account with associated roles outstanding leaves the roles
	#    hanging in a "deleted:serviceAccount" within IAM admin.)
	#    --condition=None: the unconditioned grant, which is what create makes;
	#    without it gcloud refuses, when not interactive, on a project policy
	#    that holds conditional bindings.
	#    After a removal fails, even after iam_policy_edit's retries, the rest
	#    are not tried: the account is kept anyway, so nothing would be gained
	#    by waiting on each of them.
	#
	for role in ${roles}; do
		if [[ ${#failed[@]} -eq 0 ]] &&
			iam_policy_edit "" gcloud projects remove-iam-policy-binding "${project}" \
				--member="${member}" --role="${role}" --condition=None; then
			continue
		fi
		failed+=("${role}")
	done

	# -- Keep the account while it holds any of its roles: deleting it would
	#    leave them in the policy under its "deleted:serviceAccount" form.
	#
	if [[ ${#failed[@]} -gt 0 ]]; then
		error ""
		error "Error: Could not remove every project role of ${sa_fullname} (error"
		error "       above), so the account was not deleted. Run this command again"
		error "       later, or run these by hand:"
		for role in "${failed[@]}"; do
			error "    gcloud projects remove-iam-policy-binding ${project} --member=${member} --role=${role} --condition=None"
		done
		error "    gcloud iam service-accounts delete ${sa_fullname} --project=${project}"
		error ""
		return 1
	fi

	# -- Delete the service account
	#
	local delete_error
	if ! delete_error=$(gcloud iam service-accounts delete "${sa_fullname}" \
		--quiet --project="${project}" 2>&1 >"${devnull}"); then
		error "${delete_error}"
		error ""
		error "Error: Failed to delete service account ${sa_fullname}."
		error "       Your account may not have permission to do this, or the"
		error "       error above may clear on a retry. Its project roles are"
		error "       already removed. To retry:"
		error "    gcloud iam service-accounts delete ${sa_fullname} --project=${project}"
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

	local key_error
	if ! key_error=$(gcloud iam service-accounts keys create "${credfile}" \
		--iam-account="${sa_fullname}" 2>&1 >"${devnull}"); then
		error "${key_error}"
		error ""
		error "Error: Failed to create a key for ${sa_fullname} (error above)."
		error "       Your account may not have permission to do this, or the"
		error "       project's organization policy may not allow keys."
		error ""
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
      Returns exit code 0 if the account exists, 1 if it does not, and 2 if
      the accounts could not be listed (gcloud's error is printed)

- Create a new service account, with no key - required roles will be added
  (an existing account with this name is reused and given the roles, after a
  warning if it has user-managed keys, whose holders would get the roles):
      $ ./service_account.sh create PROJECT_ID ACCOUNT [--ask]
  With --ask, a reused account with user-managed keys, or whose keys cannot
  be listed, is given the roles only if you answer yes. If whether the
  account exists cannot be checked, nothing is created or granted.

- Delete a service account:
      $ ./service_account.sh delete PROJECT_ID ACCOUNT

- Create a credential file (json format) for an existing service account:
      $ ./service_account.sh credential PROJECT_ID ACCOUNT PATH_TO_FILE


  PROJECT_ID   = GCP Project ID (not name)
  ACCOUNT      = service account name (only name, not @...com), 6 to 30
                 characters
  PATH_TO_FILE = location to write credential file (will overwrite)

  Exit codes: 0 on success; 1 on failure (gcloud's error is printed); for
  check, 0 if the account exists, 1 if it does not and 2 if the accounts
  could not be listed; and 99 for a missing or invalid argument. For create,
  2 if whether the account exists could not be checked (gcloud's error is
  printed): nothing was created or granted. For create with --ask, 3 when the
  answer is no: no role was granted.

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
	rc=$?
	;;
"check")
	check_service_account "${project}" "${account}"
	rc=$?
	;;
"create")
	# The fourth argument is create's option, --ask
	create_service_account "${project}" "${account}" "${4}"
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
