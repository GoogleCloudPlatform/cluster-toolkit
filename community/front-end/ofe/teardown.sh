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
#                 Google Cluster Toolkit FrontEnd teardown script                  #
#                                                                              #
################################################################################

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

# -- Functions shared with deploy.sh: tf_init, error, remove_project_role_grants,
#    the FE_SA_* names and PROV_SA_SUFFIX. Sourcing defines them and runs
#    nothing.
#
# shellcheck source=deploy.sh
source "${SCRIPT_DIR}/deploy.sh"

# -- The project roles that servers deployed before the move to the Cluster
#    Toolkit service-account modules gave their account. tf/main.tf forgets
#    these grants without deleting them (a removed block), so the destroy
#    leaves them on the account, except logging.logWriter and
#    monitoring.metricWriter once module.fe_sa manages them again (at this
#    address). deploy.sh removes the superseded ones after a successful
#    upgrade apply, but not after a failed one, which has already forgotten
#    them; teardown.sh removes whatever is left before the destroy.
FE_SA_ROLES_ADDRESS='module.fe_sa.module.service_account.google_project_iam_member.project-roles'
FE_SA_OLD_ROLES=(
	storage.objectAdmin
	logging.logWriter
	monitoring.metricWriter
	cloudtrace.agent
	pubsub.admin
	artifactregistry.admin
	secretmanager.admin
)

# fe_sa_unmanaged_roles STATE_LIST
#
# Print, one per line, the FE_SA_OLD_ROLES that the destroy would leave on
# the web server's account: each one not granted at FE_SA_ROLES_ADDRESS in
# STATE_LIST (the output of terraform state list).
#
fe_sa_unmanaged_roles() {
	local state_list=${1}
	local role

	for role in "${FE_SA_OLD_ROLES[@]}"; do
		if ! awk -v prefix="${FE_SA_ROLES_ADDRESS}[" -v suffix="=>roles/${role}\"]" '
			index($0, prefix) == 1 &&
			substr($0, length($0) - length(suffix) + 1) == suffix { found = 1 }
			END { exit !found }' <<<"${state_list}"; then
			printf '%s\n' "${role}"
		fi
	done
}

# fe_sa_grant_members PROJECT DEPLOYMENT STATE_LIST
#
# Run in tf/. Print the members the web server's account's grants may be
# listed under: the account itself and, when its unique id is known (from
# the state, at either address, or from the live account), its "deleted:"
# form, which is how the policy lists them once the account is deleted.
#
fe_sa_grant_members() {
	local project=${1}
	local deployment=${2}
	local state_list=${3}
	local email="${deployment}-fe-sa@${project}.iam.gserviceaccount.com"
	local address unique_id="" members

	members="serviceAccount:${email}"
	for address in "${FE_SA_ADDRESS}" "${FE_SA_OLD_ADDRESS}"; do
		if grep -qxF "${address}" <<<"${state_list}"; then
			# From the raw state (deploy.sh tf_state_attribute): terraform
			# state show cannot read a resource stored by an older provider.
			unique_id=$(tf_state_attribute unique_id "${address}" 2>/dev/null)
			break
		fi
	done
	if [[ -z ${unique_id} ]]; then
		unique_id=$(gcloud iam service-accounts describe "${email}" \
			--project="${project}" --format='value(uniqueId)' 2>/dev/null)
	fi
	if [[ -n ${unique_id} ]]; then
		members+=" deleted:serviceAccount:${email}?uid=${unique_id}"
	fi
	printf '%s\n' "${members}"
}

# deployment_in_state
#
# 0 when tf/'s Terraform state holds any resource, even without the server,
# whose label once decided this: a destroy that failed part-way leaves no
# server, and what it left still needs destroying. 1 when there is no state,
# or it holds nothing: a deploy that stopped before Terraform created
# anything, or a finished teardown. 2 when the state cannot be read
# (Terraform's error is printed).
#
deployment_in_state() {

	local addrs errors state

	errors=$(mktemp) || return 2
	if addrs=$(cd tf && terraform state list 2>"${errors}"); then
		rm -f "${errors}"
		[[ -n ${addrs} ]]
		return
	fi
	# Before the first apply there is no state at all: "state list" then
	# fails, and "state pull" succeeds and prints nothing.
	if state=$(cd tf && terraform state pull 2>/dev/null) &&
		[[ ! ${state} =~ [^[:space:]] ]]; then
		rm -f "${errors}"
		return 1
	fi
	cat "${errors}" >&2
	rm -f "${errors}"
	return 2
}

# server_vm_state PROJECT DEPLOYMENT ZONE
#
# Whether the deployment's server VM, <DEPLOYMENT>-server in ZONE, exists: 0
# if it does, 1 if Compute Engine says it is not found, 2 if that cannot be
# told (gcloud's error, or the reason, is printed). It only reads. Without
# Terraform state, this is what tells a deploy that stopped before Terraform
# created anything (no server) from a running server whose state is missing.
#
server_vm_state() {
	local project=${1}
	local server="${2}-server"
	local zone=${3}
	local output

	if [[ -z ${zone} ]]; then
		echo "  tf/terraform.tfvars has no zone, so whether the server ${server}"
		echo "  exists cannot be checked."
		return 2
	fi
	if output=$(gcloud compute instances describe "${server}" --zone="${zone}" \
		--project="${project}" --format='value(name)' 2>&1); then
		return 0
	fi
	# Only the instance itself not found counts: a zone or project that is
	# not found, or any other error, says nothing about the server.
	if grep -qF "/zones/${zone}/instances/${server}' was not found" <<<"${output}"; then
		return 1
	fi
	printf '%s\n' "${output}" >&2
	return 2
}

# Accounts this teardown could not confirm are gone, each with the command
# that deletes it and its project roles. The summary at the end lists them,
# as does every earlier exit (report_left_accounts). Running teardown.sh
# again checks each one again: once the deployment is destroyed, its state
# is empty and the server gone, so it offers to delete the provisioning
# account (with -y, lists it) and lists the web server's account (see the
# no-state path below).
left_accounts=()

# left_account PROJECT ACCOUNT
#
# Record ACCOUNT as possibly left, and say how to delete it.
#
left_account() {
	local command="script/service_account.sh delete ${1} ${2}"

	left_accounts+=("${2}: ${command}")
	echo "  To delete it and its project roles: ${command}"
}

# account_exists PROJECT ACCOUNT
#
# 0 if ACCOUNT exists, 1 if it does not. 2 if it could not be checked
# (service_account.sh has printed gcloud's error): then ACCOUNT is recorded
# as possibly left. service_account.sh's 99, a missing or invalid argument
# such as an empty PROJECT, says nothing about the account, so it is
# treated as 2, never as an absent account.
#
account_exists() {
	local rc=0

	bash "${SCRIPT_DIR}/script/service_account.sh" check "${1}" "${2}" || rc=$?
	if [[ ${rc} -eq 99 ]]; then
		rc=2
	fi
	if [[ ${rc} -eq 2 ]]; then
		echo "  Could not check whether ${2} still exists (the error is above);"
		echo "  if it does, it may still hold its project roles."
		left_account "${1}" "${2}"
	fi
	return "${rc}"
}

#
# delete_service_account PROJECT DNAME ASSUME_YES
#
# Delete the account <dname>-tkfe-sa that earlier versions created, after
# asking unless ASSUME_YES is -y: -y deletes it as the answer "y" would, so
# the teardown calls it with -y only once the destroy has succeeded.
# Returns 0 whatever happens, so the teardown goes on: an account that is
# kept, or whose delete fails, is recorded through left_account and listed
# in the summary at the end.
#
delete_service_account() {

	local project=${1}
	local server_name=${2}
	local assume_yes=${3}
	local service_account="${server_name}-tkfe-sa"
	local ready

	# -- If no detected service account, just return ok
	if ! account_exists "${project}" "${service_account}"; then
		return 0
	fi

	echo "  Service account found: ${service_account}"
	if [[ ${assume_yes} == "-y" ]]; then
		ready=y
	else
		read -r -p "           Delete [y/N] > " ready || ready=""
	fi
	case "$ready" in
	[Yy]*)
		if ! bash "${SCRIPT_DIR}/script/service_account.sh" delete \
			"${project}" "${service_account}"; then
			echo ""
			echo "  Error: Failed to delete ${service_account} (the error is above)."
			left_account "${project}" "${service_account}"
			echo ""
			return 0
		else
			echo "  Deleted"
			return 0
		fi
		;;
	*)
		echo "  Kept ${service_account}, which may still hold its project roles."
		left_account "${project}" "${service_account}"
		return 0
		;;
	esac
}

# delete_account_if_agreed PROJECT ACCOUNT ASSUME_YES DESCRIPTION
#
# If ACCOUNT exists, delete it and its project roles, after asking unless
# ASSUME_YES is -y. service_account.sh delete removes the account's project
# role grants before deleting it, so none are left behind under a "deleted:"
# member; if it cannot remove them all, it keeps the account and prints the
# commands to finish by hand. An account that is kept, or whose delete fails,
# is recorded through left_account. DESCRIPTION names the account's role in
# the messages.
#
delete_account_if_agreed() {

	local project=${1}
	local service_account=${2}
	local assume_yes=${3}
	local description=${4}
	local ready

	if ! account_exists "${project}" "${service_account}"; then
		return 0
	fi

	echo "  ${description} found: ${service_account}"
	if [[ ${assume_yes} == "-y" ]]; then
		ready=y
	else
		read -r -p "           Delete it and its project roles [y/N] > " ready || ready=""
	fi
	case "${ready}" in
	[Yy]*)
		if bash "${SCRIPT_DIR}/script/service_account.sh" delete \
			"${project}" "${service_account}"; then
			echo "  Deleted"
		else
			echo "  Warning: Failed to delete ${service_account} (the error is above)."
			left_account "${project}" "${service_account}"
		fi
		;;
	*)
		echo "  Kept ${service_account}, which may still hold its project roles."
		left_account "${project}" "${service_account}"
		;;
	esac
	return 0
}

#
# Delete the provisioning service account (<dname>-prov-sa) that deploy.sh
# created, asking first unless -y was given. Called after the destroy, which
# removes the server's permission to act as it, or when there is no
# deployment to destroy and no server (an aborted deploy).
#
delete_provisioning_account() {
	delete_account_if_agreed "${1}" "${2}${PROV_SA_SUFFIX}" "${3}" \
		"Provisioning service account"
}

#
# Safety-net removal of the terraform-managed server service account
# (<dname>-fe-sa). terraform destroy normally deletes it; this catches the
# case where a partial or aborted destroy leaves it behind, which would
# otherwise block a later redeploy that reuses the same deployment name.
#
cleanup_server_service_account() {

	local project=${1}
	local server_name=${2}
	local service_account="${server_name}-fe-sa"

	if ! account_exists "${project}" "${service_account}"; then
		return 0
	fi

	echo "  Removing leftover service account: ${service_account}"
	if ! bash "${SCRIPT_DIR}/script/service_account.sh" delete \
		"${project}" "${service_account}"; then
		echo "  Warning: Failed to delete ${service_account} (the error is above)."
		left_account "${project}" "${service_account}"
	fi
	return 0
}

#
# Print the state bucket working directory of deployment ${1}: its own
# directory under tf/state_bucket/deployments/, or tf/state_bucket/ itself if
# an earlier release left that deployment's files there and deploy.sh has not
# moved them yet. Prints nothing if there is neither.
#
state_bucket_dir() {

	local name=${1}

	[[ ${name} == "" || ${name} == . || ${name} == .. || ${name} == */* ]] && return 0
	if [[ -f tf/state_bucket/deployments/${name}/terraform.tfstate ]]; then
		echo "tf/state_bucket/deployments/${name}"
	elif [[ -f tf/state_bucket/terraform.tfstate ]] &&
		grep -qxF "deployment_name = \"${name}\"" tf/state_bucket/terraform.tfvars 2>/dev/null; then
		echo "tf/state_bucket"
	fi
}

#
# Return 0 only if no FrontEnd is provably deployed from tf/: no lock file,
# and tf/'s state can be read and holds nothing (deployment_in_state returns
# 1). A state that holds anything, even without the server (a destroy that
# failed part-way), still records a FrontEnd. Fails closed, so a bucket is
# never deleted from under a FrontEnd that may still be running.
#
front_end_gone() {

	local rc=0

	[[ -f tf/.tkfe.lock ]] && return 1
	deployment_in_state 2>/dev/null || rc=$?
	[[ ${rc} -eq 1 ]]
}

#
# How to get past a front_end_gone() refusal.
#
front_end_gone_help() {

	echo "         If the FrontEnd is still running, tear it down and delete its"
	echo "         bucket with: ./teardown.sh --delete-state-bucket"
	echo "         If it is already gone (for example after a failed teardown), run"
	echo "         'terraform -chdir=tf init' if needed, check that"
	echo "         'terraform -chdir=tf state list' lists nothing, then remove the"
	echo "         leftover lock file tf/.tkfe.lock and rerun."
}

#
# Check deployment ${1}'s state bucket can be deleted, and confirm.
#  - Sets ${sb_dir} and ${state_bucket}.
#  - Refuses unless every state in the bucket is provably empty, before
#    anything is destroyed, so a refusal leaves everything as it was.
#
prepare_state_bucket_deletion() {

	local name=${1}
	local reply

	sb_dir=$(state_bucket_dir "${name}")
	if [[ -z ${sb_dir} ]]; then
		echo "  Error: no state bucket for deployment '${name}' is recorded in this"
		echo "         directory (tf/state_bucket/deployments/${name}/)."
		return 1
	fi
	state_bucket=$(terraform -chdir="${sb_dir}" output -raw bucket_name 2>/dev/null) ||
		state_bucket=""
	if [[ -z ${state_bucket} ]]; then
		echo "  Error: ${sb_dir} records no state bucket; it may already be deleted."
		return 1
	fi

	check_state_bucket_empty "${state_bucket}" || return 1

	echo "  This will permanently delete the Terraform state bucket"
	echo "      gs://${state_bucket}"
	echo "  and every object in it, including earlier versions of state files."
	if [[ ${assume_yes} -eq 1 ]]; then
		echo "           -y flag passed. Proceeding."
	else
		read -r -p "           Type the bucket name to confirm: " reply || reply=""
		if [[ ${reply} != "${state_bucket}" ]]; then
			echo "  Not confirmed; nothing has been changed."
			echo "Exiting."
			exit 0
		fi
	fi
	echo ""
}

#
# Check a state bucket holds no resources.
#  - Reads every Terraform state in the bucket, under every namespace and
#    prefix. Lists each that records resources or cannot be read, and each
#    lock (an operation may still be running).
#  - Returns 1 unless every state is provably empty.
#
check_state_bucket_empty() {

	local bucket=${1}
	local objects object noncurrent count
	local -a busy=()

	# The bucket is versioned and the listing includes noncurrent versions,
	# which every released lock and superseded state leaves; only live
	# objects count.
	if ! objects=$(gcloud storage objects list "gs://${bucket}/**" \
		--format='value(name,noncurrent_time)' 2>&1); then
		echo "  Error: could not list gs://${bucket}:"
		echo "    ${objects}"
		return 1
	fi
	while IFS=$'\t' read -r object noncurrent; do
		[[ -z ${object} || -n ${noncurrent} ]] && continue
		case ${object} in
		*.tflock)
			busy+=("${object} (locked: an operation may be running)")
			;;
		*.tfstate)
			if ! count=$(gcloud storage cat "gs://${bucket}/${object}" 2>/dev/null |
				python3 -c 'import json, sys; print(len(json.load(sys.stdin)["resources"]))' 2>/dev/null); then
				busy+=("${object} (could not be read)")
			elif [[ ${count} -ne 0 ]]; then
				busy+=("${object} (${count} resources)")
			fi
			;;
		esac
	done <<<"${objects}"

	if [[ ${#busy[@]} -gt 0 ]]; then
		echo "  Error: gs://${bucket} is not provably empty, so it will not be"
		echo "         deleted:"
		printf '           %s\n' "${busy[@]}"
		echo "         Delete those resources in TKFE first, or destroy them from"
		echo "         this state (see 'Terraform State' in docs/admin_guide.md)."
		return 1
	fi
}

#
# Delete the state bucket in ${sb_dir}.
#  - The bucket is protected by prevent_destroy and force_destroy = false in
#    tf/state_bucket/main.tf. Those are lifted in a temporary copy, so the
#    tracked files are never edited. force_destroy is read from state, so it
#    is applied to the bucket before the destroy.
#  - Refuses if Terraform would change the bucket in any other way, for
#    example replace it after a change of region.
#  - The copy's state, which records what is left, replaces ${sb_dir}'s
#    whether or not the destroy succeeds, and also on an interrupt.
#
delete_state_bucket() {

	local tmp rc=0 started=0 actions

	tmp=$(mktemp -d) || return 1
	# Before Terraform starts nothing has changed, so the copy is not needed.
	trap 'if [[ ${started} -eq 1 ]]; then
		cp "${tmp}/terraform.tfstate" "${sb_dir}/terraform.tfstate" || true
	fi
	rm -rf "${tmp:?}"
	echo "  Interrupted. To finish, rerun teardown.sh with --delete-state-bucket."
	exit 130' INT TERM
	cp tf/state_bucket/*.tf "${sb_dir}"/terraform.tfstate "${sb_dir}"/*.tfvars "${tmp}/" ||
		rc=1
	sed -i -e '/^  lifecycle {$/,/^  }$/d' \
		-e 's/^  force_destroy = false$/  force_destroy = true/' "${tmp}/main.tf" || rc=1
	if grep -q '^ *prevent_destroy *= *true' "${tmp}/main.tf" ||
		! grep -q '^  force_destroy = true$' "${tmp}/main.tf"; then
		echo "  Error: could not lift the bucket's deletion protection in a copy of"
		echo "         tf/state_bucket/main.tf."
		rc=1
	fi

	# Checked again immediately before the destroy, which deletes every
	# object, in case anything was provisioned since the first check.
	# The protection change is planned first and exactly that plan applied,
	# so nothing but an in-place update of the bucket is ever applied.
	if [[ ${rc} -eq 0 ]]; then
		started=1
		if ! terraform -chdir="${tmp}" init -input=false ||
			! terraform -chdir="${tmp}" plan -input=false \
				-target=google_storage_bucket.tfstate -out=target.tfplan ||
			! actions=$(terraform -chdir="${tmp}" show -json target.tfplan |
				python3 -c '
import json, sys
for c in json.load(sys.stdin).get("resource_changes", []):
    if c["address"] == "google_storage_bucket.tfstate":
        print(",".join(c["change"]["actions"]))
'); then
			rc=1
		elif [[ ${actions} != update && ${actions} != no-op ]]; then
			echo "  Error: Terraform would change gs://${state_bucket} (${actions:-not in state}),"
			echo "         not only lift its protection, probably because a setting such"
			echo "         as the region differs from the bucket's. Nothing was deleted."
			rc=1
		elif ! terraform -chdir="${tmp}" apply -input=false target.tfplan; then
			rc=1
		elif ! check_state_bucket_empty "${state_bucket}"; then
			# The bucket stays, so restore force_destroy = false from the
			# tracked main.tf (prevent_destroy does not block an update).
			cp tf/state_bucket/main.tf "${tmp}/main.tf" &&
				terraform -chdir="${tmp}" apply -input=false -auto-approve \
					-target=google_storage_bucket.tfstate ||
				echo "  Warning: could not restore force_destroy = false; the next deploy.sh does."
			rc=1
		elif ! terraform -chdir="${tmp}" destroy -input=false -auto-approve; then
			rc=1
		fi
		trap - INT TERM
		if ! cp "${tmp}/terraform.tfstate" "${sb_dir}/terraform.tfstate"; then
			echo "  Error: could not copy the state back to ${sb_dir}; it is kept in"
			echo "         ${tmp}/terraform.tfstate"
			return 1
		fi
	fi
	trap - INT TERM
	rm -rf "${tmp:?}"
	return ${rc}
}

#
#
tfdestroy() {

	# All action happens in tf subdirectory. The caller tests the result, so
	# set -e does not apply here.
	(
		cd tf || exit 1

		# -- Start the deployment using Terraform
		#
		set -o pipefail
		terraform destroy -auto-approve "$@" | tee tfdestroy.log
	)
}

# tfdestroy_check [VAR_ARGS...]
#
# Plan the destroy, changing nothing, so that a destroy that cannot start (a
# provider that refuses its configuration, an unreadable state) stops the
# teardown before it removes any grant. Prints the end of the log on failure.
#
tfdestroy_check() {
	(
		cd tf
		echo "  Checking that Terraform can destroy the deployment (tfdestroy-check.log)..."
		if ! terraform plan -destroy -input=false "$@" >tfdestroy-check.log 2>&1; then
			tail -n 20 tfdestroy-check.log >&2
			echo "  Error: Terraform cannot plan the destroy (see tf/tfdestroy-check.log);"
			echo "         nothing was changed or destroyed."
			return 1
		fi
	)
}

# Set by finish_teardown: the teardown reached its end.
teardown_finished=""

# report_left_accounts
#
# The EXIT trap, so it runs on every exit, early or at the end, whatever the
# cause (an explicit exit, or a command failing under set -e). When an
# account may still exist (left_accounts), list each one with the command
# that deletes it, and make the exit status non-zero. It changes nothing
# else: an early exit leaves the lock file as it is.
#
report_left_accounts() {
	local rc=$?
	local entry

	if [[ ${#left_accounts[@]} -gt 0 ]]; then
		echo ""
		if [[ -n ${teardown_finished} ]]; then
			echo "  Completed teardown, except: these service accounts may still exist,"
			echo "  with their project roles. Delete each one, from this folder:"
		else
			echo "  The teardown stopped before it finished (see above). These service"
			echo "  accounts may also still exist, with their project roles. Running"
			echo "  teardown.sh again checks each one again; to delete one yourself,"
			echo "  from this folder:"
		fi
		for entry in "${left_accounts[@]}"; do
			echo "      ${entry}"
		done
		echo ""
		if [[ ${rc} -eq 0 ]]; then
			rc=1
		fi
	fi
	exit "${rc}"
}

# settle_state_bucket
#
# With --delete-state-bucket, delete the state bucket, which was checked
# empty before anything changed; exit 1, with how to finish, if that fails.
# Otherwise find the bucket that is kept. Sets ${state_bucket}. The lock
# file is removed first: the FrontEnd is gone either way.
#
settle_state_bucket() {

	rm -f tf/.tkfe.lock 2>/dev/null

	if [[ ${delete_bucket} -eq 1 ]]; then
		echo ""
		if ! delete_state_bucket; then
			echo "  Error: the FrontEnd was torn down, but deleting its state bucket"
			echo "         gs://${state_bucket} failed (see above). Rerun:"
			echo "           ./teardown.sh --delete-state-bucket ${dname}"
			exit 1
		fi
	else
		state_bucket=""
		sb_dir=$(state_bucket_dir "${dname}")
		if [[ ${sb_dir} ]]; then
			state_bucket=$(terraform -chdir="${sb_dir}" output -raw bucket_name 2>/dev/null || true)
		fi
	fi
}

# finish_teardown
#
# Settle the state bucket (settle_state_bucket), remove the lock file, say
# what became of the bucket, and end the teardown through
# report_left_accounts: exit 0, or, when an account may still exist
# (left_accounts), list each one with the command that deletes it and exit 1.
#
finish_teardown() {

	settle_state_bucket
	teardown_finished=1

	if [[ ${#left_accounts[@]} -eq 0 ]]; then
		echo ""
		echo "  Completed teardown"
		echo ""
	fi

	# -- The Terraform state bucket is NOT destroyed unless
	#    --delete-state-bucket was given and it was checked empty before
	#    anything changed. It holds the state of the clusters, filesystems,
	#    networks, images and workbenches this FrontEnd provisioned;
	#    destroying it would leave any that are still running impossible to
	#    remove with Terraform.
	#
	if [[ ${delete_bucket} -eq 1 ]]; then
		echo "  The Terraform state bucket gs://${state_bucket} has been deleted."
		echo ""
	elif [[ ${state_bucket} ]]; then
		echo "  The Terraform state bucket has been left in place:"
		echo "      gs://${state_bucket}"
		echo "  It is versioned and protected against accidental deletion. Once you"
		echo "  are certain every resource this FrontEnd provisioned is gone, remove"
		echo "  it with:"
		echo "      ./teardown.sh --delete-state-bucket ${dname}"
		echo ""
	fi

	wait
	# The normal end reports directly; the trap is for every other exit
	trap - EXIT
	report_left_accounts
}

# refuse_without_state REASON...
#
# Stop the teardown, changing nothing more, when tf/ has no Terraform state
# but the server may still run: its provisioning and web server accounts may
# be in use, and the state that says what to destroy is missing. Each REASON
# is printed as a line of the error.
#
refuse_without_state() {
	local line

	echo ""
	for line in "$@"; do
		echo "  ${line}"
	done
	echo "  If this deployment was made from another folder, run teardown.sh there."
	echo "  See \"Teardown Process\" in docs/admin_guide.md; if the state is lost,"
	echo "  its \"If tf/ state is lost while the server runs\" gives the manual steps."
	echo "Exiting."
	exit 1
}

################################################################################
#

# -- Exit on any errors
set -e

# -- Whatever the exit, list any account that may be left
trap report_left_accounts EXIT

# -- Options
#
assume_yes=0
delete_bucket=0
bucket_deployment=""
while [[ ${#} -gt 0 ]]; do
	case "${1}" in
	-y)
		assume_yes=1
		;;
	--delete-state-bucket)
		delete_bucket=1
		if [[ ${2} && ${2} != -* ]]; then
			bucket_deployment=${2}
			shift
		fi
		;;
	*)
		echo "  Error: Unknown option: ${1}"
		echo "  Usage: ${0} [-y] [--delete-state-bucket [<deployment name>]]"
		exit 1
		;;
	esac
	shift
done
# The form delete_provisioning_account takes
yes_flag=""
if [[ ${assume_yes} -eq 1 ]]; then
	yes_flag=-y
fi

# -- Splash screen
#
cat <<'HEADER'

--------------------------------------------------------------------------------

                       Google Cluster Toolkit Open FrontEnd

--------------------------------------------------------------------------------

HEADER

# -- Check for terraform and the gcloud storage CLI
#
if ! command -v terraform &>/dev/null; then
	echo "  Error:"
	echo "      Please ensure terraform (version 1.12.2 or later) is in your \$PATH"
	echo "Exiting."
	exit 1
fi
if ! command -v gcloud &>/dev/null || ! gcloud storage --help &>/dev/null; then
	echo "  Error:"
	echo "      Please ensure the gcloud CLI (with the 'gcloud storage' command) is in your \$PATH"
	echo "Exiting."
	exit 1
fi

# TODO: Check default authorisation has been set up
#       'gcloud auth list' or similar could be used

# -- A state bucket named explicitly: only the bucket is touched, so a
#    FrontEnd that may still be deployed from here under that name is refused
#    rather than torn down. tf/'s deployment name is read leniently; if
#    tf/terraform.tfvars is missing or names none, it is treated as possibly
#    this one.
#
if [[ ${bucket_deployment} ]]; then
	tf_name=$(sed -n 's/^[[:space:]]*deployment_name[[:space:]]*=[[:space:]]*"\([^"]*\)".*$/\1/p' \
		tf/terraform.tfvars 2>/dev/null | head -n 1)
	if [[ ! -f tf/terraform.tfvars || -z ${tf_name} || ${tf_name} == "${bucket_deployment}" ]] &&
		! front_end_gone; then
		echo "  Error: the FrontEnd '${bucket_deployment}' may still be deployed from here"
		echo "         (tf/.tkfe.lock exists, or tf/'s state shows it or cannot be read),"
		echo "         and --delete-state-bucket <name> deletes only a bucket."
		front_end_gone_help
		exit 1
	fi
	prepare_state_bucket_deletion "${bucket_deployment}" || exit 1
	delete_state_bucket || exit 1
	echo "  The Terraform state bucket gs://${state_bucket} has been deleted."
	exit 0
fi

# -- Check terraform file for a TKFE deployment exits
#

tfvars="tf/terraform.tfvars"

if [[ ! -f ${tfvars} ]]; then
	echo "  Error: No TKFE deployment directory/file found: ${tfvars}"
	echo "Exiting."
	exit 1
fi

# -- Get project and deployment name used used for current TKFE
#    This can be extracted from the terraform files
#
project=$(awk '$1 ~ /project_id/ {v=substr($3,2,length($3)-2); print v}' \
	${tfvars})
dname=$(awk '$1 ~ /deployment_name/ {v=substr($3,2,length($3)-2); print v}' \
	${tfvars})

# -- Every check and delete below names the project: without one, an
#    account check would say nothing, so stop before anything is changed.
if [[ -z ${project} ]]; then
	echo "  Error: ${tfvars} has no project_id (or it is empty), so this teardown"
	echo "         cannot tell which project to work in. Nothing was changed."
	echo "         Set project_id = \"<PROJECT_ID>\" in ${tfvars}, the project"
	echo "         this FrontEnd was deployed to, and run teardown.sh again."
	echo "Exiting."
	exit 1
fi

# -- If there's no lock file, there shouldn't be any FrontEnd deployed.
#    deploy.sh writes the lock only after a successful apply, so a deploy
#    answered N at "Proceed to deploy?", or stopped before its apply, leaves
#    none, but has already created the provisioning account. On N here,
#    list that account, if it exists, with the command that deletes it
#    (nothing is deleted): the exit then lists it and exits 1.
#
if [[ ! -f tf/.tkfe.lock ]]; then
	echo "  Warning: No lock file found"
	echo "           It is likely there is no FrontEnd currently deployed"
	if [[ ${assume_yes} -eq 1 ]]; then
		echo "           -y flag passed. Proceeding anyway."
	else
		read -r -p "           Proceed anyway? [y/N]: " ready || ready=""
		case "$ready" in
		[Yy]*) ;;
		*)
			if account_exists "${project}" "${dname}${PROV_SA_SUFFIX}"; then
				echo "  The provisioning service account ${dname}${PROV_SA_SUFFIX} exists,"
				echo "  with its project roles. If no FrontEnd server uses it, delete it."
				left_account "${project}" "${dname}${PROV_SA_SUFFIX}"
			fi
			echo "Exiting."
			exit 0
			;;
		esac
	fi
	echo ""
fi

# -- deploy.sh --config before the move to the Cluster Toolkit
#    service-account modules wrote an empty region. The Google provider
#    those versions used accepted it; the one tf/ needs now refuses it, so
#    give Terraform the zone's region, as deploy.sh does today.
region=$(awk '$1 == "region" {v=substr($3,2,length($3)-2); print v}' ${tfvars})
zone=$(awk '$1 == "zone" {v=substr($3,2,length($3)-2); print v}' ${tfvars})
tf_var_args=()
if [[ -z ${region} && -n ${zone} ]]; then
	tf_var_args=(-var "region=${zone%-*}")
	echo "  ${tfvars} has an empty region; using ${zone%-*}, the region of ${zone}."
fi

# -- With --delete-state-bucket, check before the confirmation prompts or any
#    change, so a refusal leaves everything as it was: a FrontEnd that tf/'s
#    state does not hold must be provably gone (front_end_gone), and the
#    state bucket must be empty.
#
if [[ ${delete_bucket} -eq 1 ]]; then
	in_state_rc=0
	deployment_in_state 2>/dev/null || in_state_rc=$?
	if [[ ${in_state_rc} -ne 0 ]] && ! front_end_gone; then
		echo "  Error: cannot confirm that no FrontEnd is deployed from here"
		echo "         (tf/.tkfe.lock exists, or tf/'s state cannot be read); nothing"
		echo "         has been changed."
		front_end_gone_help
		exit 1
	fi
	prepare_state_bucket_deletion "${dname}" || exit 1
fi

# -- An account <dname>-tkfe-sa from an earlier version may still exist.
#    Without -y, ask now. One that is kept, or whose delete fails, is listed
#    at the end, and the teardown goes on. With -y it is not touched here:
#    it is deleted only once the destroy has succeeded (below), so a -y
#    teardown that stops before then leaves it as it was.
#
if [[ ${assume_yes} -ne 1 ]]; then
	delete_service_account "${project}" "${dname}" ""
fi

# -- Install the providers and modules the current tf/ needs before reading
#    or destroying the deployment.
#
if ! (cd tf && tf_init); then
	# -- Without init, the state cannot be read. A deploy that stopped at its
	#    own init has already created the provisioning account; when there is
	#    no server, list it with its delete command (nothing is deleted).
	vm_rc=0
	server_vm_state "${project}" "${dname}" "${zone}" || vm_rc=$?
	if [[ ${vm_rc} -eq 1 ]] &&
		account_exists "${project}" "${dname}${PROV_SA_SUFFIX}"; then
		echo ""
		echo "  The server ${dname}-server does not exist. If this deployment stopped"
		echo "  before Terraform created anything, delete its provisioning account,"
		echo "  ${dname}${PROV_SA_SUFFIX}, which may still hold its project roles."
		left_account "${project}" "${dname}${PROV_SA_SUFFIX}"
	elif [[ ${vm_rc} -eq 2 ]]; then
		echo "  Could not check whether the server ${dname}-server exists (see above)."
	fi
	echo "Exiting."
	exit 1
fi

# -- Now check TKFE was deployed: anything left in terraform's state, even
#    after a destroy that failed part-way, still needs destroying. A state
#    that cannot be read stops the teardown before any Terraform change and
#    before the provisioning or web server account is touched; only an old
#    -tkfe-sa account may already have been deleted, and only without -y,
#    when the user answered yes to the question above. With -y it is left
#    as it was.
#
state_rc=0
deployment_in_state || state_rc=$?
if [[ ${state_rc} -eq 2 ]]; then
	echo "  Error: Could not read the Terraform state in tf/ (the error is above);"
	echo "         nothing was destroyed."
	echo "Exiting."
	exit 1
fi
if [[ ${state_rc} -ne 0 ]]; then
	# -- No state: either a deploy that stopped before Terraform created
	#    anything (declined at the confirmation, or stopped by a failure
	#    before the apply, such as terraform init), which has already
	#    created the provisioning account, or a running server whose state
	#    is missing (deployed from another folder, or the state was lost).
	#    Only the server VM tells them apart: touch no account unless it is
	#    known to be gone.
	vm_rc=0
	server_vm_state "${project}" "${dname}" "${zone}" || vm_rc=$?
	if [[ ${vm_rc} -eq 0 ]]; then
		refuse_without_state \
			"Error: The server ${dname}-server still exists but tf/ has no Terraform" \
			"       state, so this teardown cannot tell what to destroy. Nothing was" \
			"       destroyed, and no provisioning or web server account was changed."
	fi
	if [[ ${vm_rc} -ne 1 ]]; then
		refuse_without_state \
			"Error: Could not check whether the server ${dname}-server exists (see" \
			"       above), and tf/ has no Terraform state, so this teardown cannot" \
			"       tell what to destroy. Nothing was destroyed, and no provisioning" \
			"       or web server account was changed."
	fi
	# -- No server. With no state, this folder's Terraform made nothing that
	#    uses either account, and a deploy running elsewhere under the same
	#    name may be using them, before its server exists. The provisioning
	#    account is deleted only when the user answers yes; with -y it is
	#    only listed. The web server's account is created by Terraform, so it
	#    is normally absent; if it exists, it is only listed, never deleted,
	#    even with -y. With -y, an old -tkfe-sa account, not touched above,
	#    is only listed too.
	if [[ ${assume_yes} -eq 1 ]]; then
		if account_exists "${project}" "${dname}-tkfe-sa"; then
			echo "  Service account found: ${dname}-tkfe-sa"
			echo "  It is not deleted with -y: with no Terraform state, this teardown"
			echo "  cannot tell whether a deploy running elsewhere under this name uses"
			echo "  it. If none does, delete it, or run teardown.sh without -y to be asked."
			left_account "${project}" "${dname}-tkfe-sa"
		fi
		if account_exists "${project}" "${dname}${PROV_SA_SUFFIX}"; then
			echo "  Provisioning service account found: ${dname}${PROV_SA_SUFFIX}"
			echo "  It is not deleted with -y: with no Terraform state, this teardown"
			echo "  cannot tell whether a deploy running elsewhere under this name uses"
			echo "  it. If none does, delete it, or run teardown.sh without -y to be asked."
			left_account "${project}" "${dname}${PROV_SA_SUFFIX}"
		fi
	else
		delete_provisioning_account "${project}" "${dname}" ""
	fi
	if account_exists "${project}" "${dname}-fe-sa"; then
		echo "  Web server service account found: ${dname}-fe-sa"
		echo "  It is not deleted: with no Terraform state, this folder did not"
		echo "  create it, and a deploy running elsewhere may be using it. If none is,"
		echo "  delete it."
		left_account "${project}" "${dname}-fe-sa"
	fi
	echo "  No terraform deployment found; nothing else to destroy."
	if [[ ${delete_bucket} -eq 1 ]]; then
		# Checked before anything changed: no lock file, and an empty state
		echo "  No FrontEnd is deployed from here; deleting only its state bucket."
	fi
	finish_teardown
fi

echo "  This will destroy the running FrontEnd: ${dname}"
echo "  Please ensure all resources deployed by the FrontEnd have been deleted."
echo ""
if [[ ${assume_yes} -eq 1 ]]; then
	echo "           -y flag passed. Proceeding anyway."
	# -- An old -tkfe-sa account, not touched above under -y, is deleted
	#    only after the destroy has succeeded (below).
else
	read -r -p "           Proceed? [y/N]: " ready || ready=""
	case "$ready" in
	[Yy]*) ;;
	*)
		# Exit status 1, with the list, if an old -tkfe-sa account was kept.
		echo "Exiting."
		exit 0
		;;
	esac
fi
echo ""

# TODO: Spawn a shutdown script on FE server, via gcloud, which finds
#       all running clusters and 'terraform destroy's them.
#       This will give a totally clean shut down, leaving no GCP
#       resources attributable to this FE.
#       This might be needed to remove the PubSub subscriptions.

# TODO: Remove PubSub subscriptions?

# -- The web server's account may hold old project roles that the destroy
#    would leave behind (see FE_SA_OLD_ROLES): those of a server deployed
#    with code older than the move to the Cluster Toolkit service-account
#    modules, upgraded or not. Remove them now, while the account exists:
#    once the destroy deletes it, they would be left on its "deleted:" form,
#    and IAM can refuse edits naming it for minutes. If they cannot be
#    removed, stop before destroying anything. First check that the destroy
#    can run at all, so a teardown that cannot finish leaves them in place.
if ! tfdestroy_check "${tf_var_args[@]}"; then
	echo "Exiting."
	exit 1
fi
if ! fe_sa_state=$(cd tf && terraform state list); then
	echo "  Error: Could not read the Terraform state in tf/; nothing was destroyed."
	echo "Exiting."
	exit 1
fi
mapfile -t fe_sa_leftover_roles < <(fe_sa_unmanaged_roles "${fe_sa_state}")
if [[ ${#fe_sa_leftover_roles[@]} -gt 0 ]]; then
	fe_sa_members=$(cd tf && fe_sa_grant_members "${project}" "${dname}" "${fe_sa_state}")
	if ! remove_project_role_grants "${project}" "${fe_sa_members}" \
		"${fe_sa_leftover_roles[@]}"; then
		echo "  Error: The web server's account still holds the project roles above,"
		echo "         so nothing was destroyed. Remove them, or wait a few minutes,"
		echo "         and run teardown.sh again."
		echo "Exiting."
		exit 1
	fi
fi

if ! tfdestroy "${tf_var_args[@]}"; then
	echo ""
	echo "  Error: Terraform destroy failed (the error is above); the deployment may"
	echo "         be partly destroyed. Fix the cause and run teardown.sh again."
	echo "Exiting."
	exit 1
fi

# -- With -y, an old -tkfe-sa account, not touched above, is deleted now that
#    the destroy has succeeded (or listed if the delete fails). Without -y,
#    the user was asked before init.
if [[ ${assume_yes} -eq 1 ]]; then
	delete_service_account "${project}" "${dname}" "-y"
fi

# -- The provisioning account and its project roles. The destroy has removed
#    the server's permission to act as it; reaching here means it succeeded.
delete_provisioning_account "${project}" "${dname}" "${yes_flag}"

# -- Safety net: terraform destroy should have removed the server service
#    account, but clean up any leftover so a same-name redeploy is not blocked.
#
cleanup_server_service_account "${project}" "${dname}"

# -- Remove the lock file, delete or report the state bucket
#    (settle_state_bucket), and list any account that may be left
#
finish_teardown

#
# eof
