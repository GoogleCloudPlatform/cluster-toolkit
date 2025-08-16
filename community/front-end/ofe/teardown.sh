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

	# Set the gcloud project first
	if ! gcloud config set project "${project}" &>/dev/null; then
		echo "  Error: Failed to set gcloud project to ${project}"
		return 1
	fi

	echo "  Checking for service account: ${service_account} in project: ${project}"

	# -- If no detected service account, just return ok
	if ! bash "${SCRIPT_DIR}/script/service_account.sh" check \
		"${project}" "${service_account}"; then
		echo "  No service account found"
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

# -- Splash screen
#
cat <<'HEADER'

--------------------------------------------------------------------------------

                       Google Cluster Toolkit Open FrontEnd

--------------------------------------------------------------------------------

HEADER

# -- Check for terraform and gsutil
#
if ! command -v terraform &>/dev/null; then
	echo "  Error:"
	echo "      Please ensure terraform (version 0.13 or higher) is in your \$PATH"
	echo "Exiting."
	exit 1
fi
if ! command -v gsutil &>/dev/null; then
	echo "  Error:"
	echo "      Please ensure gsutil (part of Google Cloud Tools)  is in your \$PATH"
	echo "Exiting."
	exit 1
fi

# TODO: Check default authorisation has been set up
#       'gcloud auth list' or similar could be used

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
	if [[ "$1" == "-y" ]]; then
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
project=$(awk -F'=' '/^project_id/ {gsub(/^[ \t]+|[ \t]+$|^"|"$/, "", $2); print $2}' ${tfvars} | tr -d '"')
dname=$(awk -F'=' '/^deployment_name/ {gsub(/^[ \t]+|[ \t]+$|^"|"$/, "", $2); print $2}' ${tfvars} | tr -d '"')

# -- Validate project and deployment name
#
if [[ -z "${project}" || -z "${dname}" ]]; then
	echo "  Error: Could not extract project_id or deployment_name from ${tfvars}"
	echo "Exiting."
	exit 1
fi

# -- The tf directory can exist, but TKFE deployment was aborted
#    If this happened, the service account may still need removing.
#
delete_service_account "${project}" "${dname}"

# -- Now check TKFE was deployed - get server name from terraform
#
tname=$(
	cd tf 2>/dev/null || {
		echo "Error: tf directory not found"
		exit 1
	}
	if [[ ! -f terraform.tfstate ]]; then
		echo "Error: No terraform state file found"
		exit 1
	fi
	terraform show -json 2>/dev/null | jq -r '.values.root_module.resources[] | select(.address=="google_compute_instance.server_vm") | .values.name' 2>/dev/null || echo ""
)

# -- If no name returned from terraform, there's nothing running
#
if [[ ! ${tname} ]]; then
	echo "  Error: No terraform deployment found"
	echo "         This could mean:"
	echo "         1. The deployment was already destroyed"
	echo "         2. The terraform state is missing"
	echo "         3. The deployment failed before completion"
	echo ""
	echo "         You can safely proceed with cleanup."
	echo ""
	if [[ "$1" == "-y" ]]; then
		echo "           -y flag passed. Proceeding with cleanup."
	else
		read -r -p "           Proceed with cleanup? [y/N]: " ready
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

echo "  This will destroy the running FrontEnd: ${dname}"
echo "  Please ensure all resources deployed by the FrontEnd have been deleted."
echo ""
if [[ "$1" == "-y" ]]; then
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

# -- Remove the lock file
#
rm -f tf/.tkfe.lock 2>/dev/null

echo ""
echo "  Completed teardown"
echo ""

wait
exit 0

#
# eof
