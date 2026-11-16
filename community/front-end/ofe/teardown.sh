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

#
#
#
delete_service_account() {

	local project=${1}
	local server_name=${2}
	local service_account="${server_name}-tkfe-sa"
	local ready

	# -- If no detected service account, just return ok
	if ! bash "${SCRIPT_DIR}/script/service_account.sh" check \
		"${project}" "${service_account}"; then
		return 0
	fi

	echo "  Service account found: ${service_account}"
	read -r -p "           Delete [y/N] > " ready
	case "$ready" in
	[Yy]*)
		if ! bash "${SCRIPT_DIR}/script/service_account.sh" delete \
			"${project}" "${service_account}"; then
			echo ""
			echo "  Error: Failed to delete service account"
			echo "         Please manually check and clean up using gcloud"
			echo ""
			return 1
		else
			echo "  Deleted"
			return 0
		fi
		;;
	*)
		return 0
		;;
	esac
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

	if ! bash "${SCRIPT_DIR}/script/service_account.sh" check \
		"${project}" "${service_account}"; then
		return 0
	fi

	echo "  Removing leftover service account: ${service_account}"
	if ! bash "${SCRIPT_DIR}/script/service_account.sh" delete \
		"${project}" "${service_account}"; then
		echo "  Warning: Failed to delete ${service_account}; please check with gcloud"
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
# and tf/'s state can be read and records no FrontEnd. Fails closed, so a
# bucket is never deleted from under a FrontEnd that may still be running.
#
front_end_gone() {

	local shown

	[[ -f tf/.tkfe.lock ]] && return 1
	shown=$(cd tf && terraform show 2>/dev/null) || return 1
	! grep -q ghpcfe_id <<<"${shown}"
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

	# All action happens in tf subdirectory
	(
		cd tf

		# -- Start the deployment using Terraform
		#
		set -o pipefail
		terraform destroy -auto-approve | tee tfdestroy.log
	)
}

################################################################################
#

# -- Exit on any errors
set -e

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
	echo "      Please ensure terraform (version 0.13 or higher) is in your \$PATH"
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

# -- If there's no lock file, there shouldn't be any FrontEnd deployed.
#
if [[ ! -f tf/.tkfe.lock ]]; then
	echo "  Warning: No lock file found"
	echo "           It is likely there is no FrontEnd currently deployed"
	if [[ ${assume_yes} -eq 1 ]]; then
		echo "           -y flag passed. Proceeding anyway."
	else
		read -r -p "           Proceed anyway? [y/N]: " ready
		case "$ready" in
		[Yy]*) ;;
		*)
			echo "Exiting."
			exit 0
			;;
		esac
	fi
	echo ""
fi

# -- Get project and deployment name used used for current TKFE
#    This can be extracted from the terraform files
#
project=$(awk '$1 ~ /project_id/ {v=substr($3,2,length($3)-2); print v}' \
	${tfvars})
dname=$(awk '$1 ~ /deployment_name/ {v=substr($3,2,length($3)-2); print v}' \
	${tfvars})

# -- Check before the confirmation prompts or any change, so a refusal leaves
#    everything as it was: a FrontEnd that tf/ does not show must be provably
#    gone, and the state bucket must be empty.
#
if [[ ${delete_bucket} -eq 1 ]]; then
	if ! (cd tf && terraform show 2>/dev/null) | grep -q ghpcfe_id && ! front_end_gone; then
		echo "  Error: cannot confirm that no FrontEnd is deployed from here"
		echo "         (tf/.tkfe.lock exists, or tf/'s state cannot be read); nothing"
		echo "         has been changed."
		front_end_gone_help
		exit 1
	fi
	prepare_state_bucket_deletion "${dname}" || exit 1
fi

# -- The tf directory can exist, but TKFE deployment was aborted
#    If this happened, the service account may still need removing.
#
delete_service_account "${project}" "${dname}"

# -- Now check TKFE was deployed - get server name from terraform
#
tname=$(
	cd tf
	terraform show |
		awk '/ghpcfe_id/ {v=substr($3,2,length($3)-2); print v; exit}'
)

# -- If no name returned from terraform, there's nothing running
#
if [[ ! ${tname} && ${delete_bucket} -eq 1 ]]; then
	if ! front_end_gone; then
		echo "  Error: cannot confirm that no FrontEnd is deployed from here"
		echo "         (tf/.tkfe.lock exists, or tf/'s state cannot be read), so the"
		echo "         state bucket is kept."
		front_end_gone_help
		exit 1
	fi
	echo "  No FrontEnd is deployed from here; deleting only its state bucket."
	delete_state_bucket || exit 1
	echo "  The Terraform state bucket gs://${state_bucket} has been deleted."
	exit 0
fi
if [[ ! ${tname} ]]; then
	echo "  Error: No terraform deployment found"
	echo "Exiting."
	exit 1
fi

echo "  This will destroy the running FrontEnd: ${dname}"
echo "  Please ensure all resources deployed by the FrontEnd have been deleted."
echo ""
if [[ ${assume_yes} -eq 1 ]]; then
	echo "           -y flag passed. Proceeding anyway."
else
	read -r -p "           Proceed? [y/N]: " ready
	case "$ready" in
	[Yy]*) ;;
	*)
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

tfdestroy

# -- Safety net: terraform destroy should have removed the server service
#    account, but clean up any leftover so a same-name redeploy is not blocked.
#
cleanup_server_service_account "${project}" "${dname}"

# -- Remove the lock file
#
rm -f tf/.tkfe.lock 2>/dev/null

# -- The Terraform state bucket is NOT destroyed here unless
#    --delete-state-bucket was given and it was checked empty above. It holds
#    the state of the clusters, filesystems, networks, images and workbenches
#    this FrontEnd provisioned; destroying it would leave any that are still
#    running impossible to remove with Terraform.
#
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

echo ""
echo "  Completed teardown"
echo ""
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
exit 0

#
# eof
