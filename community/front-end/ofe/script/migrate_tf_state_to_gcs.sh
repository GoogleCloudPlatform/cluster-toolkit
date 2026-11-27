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
# One-off migration of Terraform state from local disk into the TKFE state
# bucket.
#
# Resources TKFE provisioned before state moved to a versioned GCS bucket
# still have a terraform.tfstate in their run-time directory on the service
# machine. This script moves that state into the bucket without recreating or
# interrupting the resource.
#
# Run it ON THE SERVICE MACHINE, as the user that owns the run-time
# directories, after upgrading TKFE:
#
#   ./script/migrate_tf_state_to_gcs.sh --dry-run
#   ./script/migrate_tf_state_to_gcs.sh --yes
#
# Safety:
#   - The state is copied to terraform.tfstate.premigration BEFORE migrating,
#     because 'terraform init -migrate-state' empties the original. The copy
#     is never deleted or overwritten.
#   - The copy is kept in the resource's run-time directory (e.g.
#     clusters/cluster_<id>/), not in the Terraform working directory: for
#     clusters, filesystems and image builders that is a Cluster Toolkit
#     deployment group, which the Toolkit moves into .ghpc/ and later deletes
#     on every reconfigure.
#   - Terraform runs as each resource's credential stored in the TKFE
#     database (manage.py ofe_terraform_as_credential), so no key file on
#     disk is needed. A resource whose credential cannot be used is reported
#     and left unchanged.
#   - Nothing is changed without --yes.
#   - The bucket is versioned, so a bad upload can be rolled back.
#   - Success is reported only once 'terraform plan' shows the migrated state
#     still matches reality.
#
# Recovery from a half-finished directory: copy terraform.tfstate.premigration
# from the resource's run-time directory to terraform.tfstate in the Terraform
# working directory, delete .terraform/, remove backend.tf if this script added
# it, delete terraform.tfstate.migration-incomplete if present, and run
# 'terraform init'.
#
################################################################################

set -o pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
BASE_DIR=$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)
# The application's own interpreter, with Django and OFE's dependencies.
# Every call that imports OFE code must use it.
OFE_PYTHON=${OFE_PYTHON:-/opt/gcluster/django-env/bin/python3}

dry_run=0
confirmed=0
failures=0
migrated=0
skipped=0

error() {
	echo "$*" >&2
}

usage() {
	cat <<'USAGE'
Usage: migrate_tf_state_to_gcs.sh [--dry-run | --yes] [--dir <path>]

  --dry-run   Report what would be migrated; change nothing.
  --yes       Perform the migration.
  --dir       Migrate only this one working directory (repeatable).

Exactly one of --dry-run or --yes is required.
USAGE
}

#
# Print the state bucket from the application configuration, the same source
# the application uses.
#
get_state_bucket() {
	PYTHONPATH="${BASE_DIR}/website" "${OFE_PYTHON}" -c '
import sys
from ghpcfe.cluster_manager import utils
try:
    print(utils.get_terraform_state_bucket())
except (OSError, RuntimeError) as err:
    sys.exit(f"Error: {err}")
'
}

#
# Print this server's state namespace, read from its database.
#
get_state_namespace() {

	local ns
	ns=$(cd "${BASE_DIR}/website" && "${OFE_PYTHON}" manage.py tfstate_namespace | tail -n 1) || return 1
	if [[ ! ${ns} =~ ^[0-9]{8}-[0-9a-f]{8}$ ]]; then
		error "Error: could not read the Terraform state namespace (got '${ns}')"
		return 1
	fi
	echo "${ns}"
}

#
# Print the resource kind and id of working directory ${1}, as "<kind> <id>".
#  - The kinds are those manage.py ofe_terraform_as_credential and
#    utils.terraform_state_prefix() take.
#
kind_id_for_dir() {

	local dir=${1}
	local rel=${dir#"${BASE_DIR}/"}

	local kind id
	case ${rel} in
	clusters/cluster_*) kind=cluster id=${rel#clusters/cluster_} ;;
	fs/fs_*) kind=filesystem id=${rel#fs/fs_} ;;
	vpcs/vpc_*) kind=vpc id=${rel#vpcs/vpc_} ;;
	workbenches/workbench_*) kind=workbench id=${rel#workbenches/workbench_} ;;
	images/image_*) kind=image id=${rel#images/image_} ;;
	*) return 1 ;;
	esac
	id=${id%%/*}

	echo "${kind} ${id}"
}

#
# Print the state prefix for working directory ${1}'s resource.
#  - Only the directory-to-resource-kind mapping lives here; the prefix itself
#    comes from utils.terraform_state_prefix(), as in the application, so the
#    two cannot drift apart.
#
prefix_for_dir() {

	local kind_id kind id
	kind_id=$(kind_id_for_dir "${1}") || return 1
	read -r kind id <<<"${kind_id}"

	PYTHONPATH="${BASE_DIR}/website" "${OFE_PYTHON}" -c '
import sys
from ghpcfe.cluster_manager import utils
print(utils.terraform_state_prefix(sys.argv[1], sys.argv[2], namespace=sys.argv[3]))
' "${kind}" "${id}" "${state_namespace}"
}

#
# Run terraform with arguments ${3}... as the stored credential of resource
# ${1} (kind) ${2} (id), and return Terraform's exit code.
#  - manage.py runs in the website directory, so a -chdir argument must be an
#    absolute path.
#  - The command refuses with exit code 1, and a message naming the
#    resource, when the resource or its credential cannot be used.
#
terraform_as_credential() {

	local kind=${1} id=${2}
	shift 2

	(cd "${BASE_DIR}/website" &&
		"${OFE_PYTHON}" manage.py ofe_terraform_as_credential "${kind}" "${id}" -- "$@")
}

#
# Print the path of the pre-migration copy of working directory ${1}'s state.
#  - It goes in the resource's run-time directory, <kind dir>/<kind>_<id>.
#    For clusters, filesystems and image builders the working directory is a
#    Cluster Toolkit deployment group below that; every 'gcluster create -w'
#    moves the groups into .ghpc/ and deletes the previous set, so a copy kept
#    there would not survive two reconfigures. For networks the two
#    directories are the same.
#
backup_for_dir() {

	local dir=${1}
	local rel=${dir#"${BASE_DIR}/"}

	case ${rel} in
	clusters/cluster_* | fs/fs_* | vpcs/vpc_* | workbenches/workbench_* | images/image_*) ;;
	*) return 1 ;;
	esac
	local kind_dir=${rel%%/*}
	local resource=${rel#*/}
	resource=${resource%%/*}

	echo "${BASE_DIR}/${kind_dir}/${resource}/terraform.tfstate.premigration"
}

#
# Migrate Terraform working directory ${1}.
#
migrate_dir() {

	local dir=${1}
	local state="${dir}/terraform.tfstate"
	local incomplete="${state}.migration-incomplete"
	local prefix kind id bucket backup

	if [[ -f ${incomplete} ]]; then
		backup=$(backup_for_dir "${dir}") || backup="not known"
		error "  FAIL: an earlier migration of ${dir} did not finish: its state is already"
		error "        in the bucket, but terraform plan did not come back clean. Investigate,"
		error "        then delete ${incomplete} (the original is ${backup})."
		((failures += 1))
		return 1
	fi

	if [[ ! -f ${state} ]]; then
		echo "  skip: no local state in ${dir}"
		((skipped += 1))
		return 0
	fi

	if ! python3 -c "import json,sys; json.load(open(sys.argv[1]))" "${state}" 2>/dev/null; then
		error "  FAIL: ${state} is not valid JSON; refusing to migrate"
		((failures += 1))
		return 1
	fi

	# An empty state has nothing to move and would silently "succeed".
	local nresources
	nresources=$(python3 -c \
		"import json,sys; print(len(json.load(open(sys.argv[1])).get('resources', [])))" \
		"${state}")
	if [[ ${nresources} -eq 0 ]]; then
		echo "  skip: ${dir} has no resources in state"
		((skipped += 1))
		return 0
	fi

	if ! prefix=$(prefix_for_dir "${dir}"); then
		error "  FAIL: cannot derive a state prefix for ${dir}"
		((failures += 1))
		return 1
	fi

	if ! backup=$(backup_for_dir "${dir}"); then
		error "  FAIL: cannot derive a backup location for ${dir}"
		((failures += 1))
		return 1
	fi
	# Never overwrite an existing copy: it may be the only record of another
	# working directory's state. A copy identical to the current state is one
	# this script took on an earlier run that stopped before migrating.
	if [[ -e ${backup} ]] && ! cmp -s "${state}" "${backup}"; then
		error "  FAIL: ${backup} already exists and differs from"
		error "        ${state}; refusing to overwrite it. Find out which"
		error "        state it holds, move it elsewhere if it is no longer needed, and rerun."
		((failures += 1))
		return 1
	fi

	bucket=${state_bucket}

	# Check, before anything is changed, that Terraform can run as the
	# resource's stored credential: 'terraform version' needs no cloud
	# access, so it fails only when the command refuses. It changes
	# nothing, so a dry run makes it too and reports what a migration would.
	read -r kind id <<<"$(kind_id_for_dir "${dir}")"
	if ! terraform_as_credential "${kind}" "${id}" version >/dev/null; then
		error "  FAIL: ${dir}: Terraform cannot run as the stored credential of"
		error "        ${kind} ${id} (see the message above); nothing was changed"
		((failures += 1))
		return 1
	fi

	if [[ ${dry_run} -eq 1 ]]; then
		echo "  would migrate: ${dir}"
		echo "                 ${nresources} resources -> gs://${bucket}/${prefix}/"
		echo "                 original to be kept as ${backup}"
		return 0
	fi

	# Blueprint-generated directories get their backend from the regenerated
	# blueprint; raw-template directories need one written here.
	if ! grep -rqs 'backend "gcs"' "${dir}"/*.tf; then
		echo "  writing backend.tf for ${dir}"
		PYTHONPATH="${BASE_DIR}/website" "${OFE_PYTHON}" -c '
import sys
from ghpcfe.cluster_manager import utils
utils.write_backend_tf(sys.argv[1], sys.argv[2])
' "${dir}" "${prefix}" || {
			error "  FAIL: could not write backend.tf for ${dir}"
			((failures += 1))
			return 1
		}
	fi

	echo "  migrating ${dir} -> gs://${bucket}/${prefix}/"

	# Copy BEFORE migrating: 'terraform init -migrate-state' empties the local
	# state file. An existing copy was checked above to be identical, so it
	# is kept as it is.
	if [[ ! -e ${backup} ]] && ! cp -p "${state}" "${backup}"; then
		error "  FAIL: could not back up ${state} to ${backup}; not migrating"
		((failures += 1))
		return 1
	fi
	if [[ ! -s ${backup} ]]; then
		error "  FAIL: backup ${backup} is empty; not migrating"
		((failures += 1))
		return 1
	fi

	if ! terraform_as_credential "${kind}" "${id}" \
		-chdir="${dir}" init -migrate-state -force-copy -no-color; then
		error "  FAIL: terraform init -migrate-state failed for ${dir}"
		((failures += 1))
		return 1
	fi

	# Prove the migrated state still matches reality before touching the
	# local copy. An empty plan is the only acceptable outcome.
	local rc=0
	terraform_as_credential "${kind}" "${id}" \
		-chdir="${dir}" plan -detailed-exitcode -no-color >/dev/null || rc=$?
	if [[ ${rc} -ne 0 ]]; then
		# init -migrate-state has already moved the state and emptied the local
		# file. Rename it so a rerun reports this directory rather than
		# skipping it as "no resources".
		mv -f "${state}" "${incomplete}"
		if [[ ${rc} -eq 2 ]]; then
			error "  FAIL: ${dir}: state moved to gs://${bucket}/${prefix}/, but terraform plan"
			error "        reports changes. The original is ${backup}."
		else
			error "  FAIL: ${dir}: state moved to gs://${bucket}/${prefix}/, but terraform plan"
			error "        errored. The original is ${backup}."
		fi
		((failures += 1))
		return 1
	fi

	# Remove the state file the migration emptied and Terraform's own backup;
	# only the copy taken above is kept.
	rm -f "${state}" "${state}.backup"

	echo "  OK: ${dir} (pre-migration state kept as ${backup})"
	((migrated += 1))
	return 0
}

################################################################################
#
declare -a explicit_dirs

while [[ ${#} -gt 0 ]]; do
	case ${1} in
	--dry-run)
		dry_run=1
		shift
		;;
	--yes)
		confirmed=1
		shift
		;;
	--dir)
		if ! d=$(cd -- "${2}" 2>/dev/null && pwd); then
			error "Error: --dir: no such directory: ${2}"
			exit 1
		fi
		explicit_dirs+=("${d}")
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		error "Unknown argument: ${1}"
		usage
		exit 1
		;;
	esac
done

if [[ ${dry_run} -eq 0 && ${confirmed} -eq 0 ]]; then
	error "Refusing to run: pass --dry-run to preview, or --yes to migrate."
	usage
	exit 1
fi
if [[ ${dry_run} -eq 1 && ${confirmed} -eq 1 ]]; then
	error "Pass only one of --dry-run and --yes."
	exit 1
fi

if ! command -v terraform &>/dev/null; then
	error "Error: terraform is not in \$PATH"
	exit 1
fi

if ! state_bucket=$(get_state_bucket); then
	exit 1
fi
if ! state_namespace=$(get_state_namespace); then
	exit 1
fi

echo ""
echo "Terraform state bucket: gs://${state_bucket}"
echo "State namespace:        ${state_namespace}"
if [[ ${dry_run} -eq 1 ]]; then
	echo "Mode: dry run (nothing will be changed)"
else
	echo "Mode: MIGRATE"
fi
echo ""

# -- Collect the Terraform working directories to consider. A working directory
#    is one holding a local terraform.tfstate.
#
declare -a dirs
if [[ ${#explicit_dirs[@]} -gt 0 ]]; then
	dirs=("${explicit_dirs[@]}")
else
	# Prune .ghpc/ (the Toolkit's backups of earlier deployment groups,
	# whose stale state would map to the live prefix) and .terraform/
	# (Terraform's backend settings, not a state file). Also collect
	# directories a previous run left half-migrated, so they are reported.
	while IFS= read -r state_file; do
		d=$(dirname "${state_file}")
		[[ " ${dirs[*]} " == *" ${d} "* ]] || dirs+=("${d}")
	done < <(find "${BASE_DIR}"/clusters "${BASE_DIR}"/fs "${BASE_DIR}"/vpcs \
		"${BASE_DIR}"/images "${BASE_DIR}"/workbenches \
		\( -name .ghpc -o -name .terraform \) -prune -o \
		-type f \( -name terraform.tfstate -o -name terraform.tfstate.migration-incomplete \) \
		-print 2>/dev/null | sort -u)
fi

if [[ ${#dirs[@]} -eq 0 ]]; then
	echo "No local Terraform state found. Nothing to migrate."
	exit 0
fi

for dir in "${dirs[@]}"; do
	migrate_dir "${dir}" || true
done

echo ""
echo "Migrated: ${migrated}   Skipped: ${skipped}   Failed: ${failures}"
echo ""

if [[ ${failures} -gt 0 ]]; then
	exit 1
fi
exit 0

#
# eof
