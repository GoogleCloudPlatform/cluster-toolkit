#! /bin/bash
# Copyright 2025 Google LLC
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
# Contains functions to check and manage OAuth/IAP brands and clients for TKFE.
# IAP brands are limited to 1 per project, so this script helps detect existing
# brands and provides guidance for safe deployment.
#
#   check_brand    - checks if an IAP brand already exists in the project
#   list_brands    - lists existing IAP brands (should be 0 or 1)
#   guidance       - provides setup guidance based on existing brand status
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

#
# Check if an IAP brand exists for the project
# Returns 0 if brand exists, 1 if not, 99 on error
#
check_iap_brand() {
	local project=${1}

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		return 99
	fi

	# List IAP brands for the project
	local brands
	brands=$(gcloud iap oauth-brands list --project="${project}" --format="value(name)" 2>${devnull})
	
	if [[ -n ${brands} ]]; then
		return 0  # Brand exists
	else
		return 1  # No brand exists
	fi
}

#
# Check IAP brand application type (Internal vs External)
# Returns 0 if Internal, 1 if External, 99 on error
#
check_brand_type() {
	local project=${1}

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		return 99
	fi

	# Get brand internal flag - should be true for OAuth clients (Internal brands)
	local org_internal_only
	org_internal_only=$(gcloud iap oauth-brands list --project="${project}" --format="value(orgInternalOnly)" 2>${devnull})
	
	if [[ ${org_internal_only} == "True" ]]; then
		return 0  # Internal - can create OAuth clients
	elif [[ ${org_internal_only} == "False" ]]; then
		return 1  # External - cannot create OAuth clients
	else
		return 99  # Error or unknown type
	fi
}

#
# List existing IAP brands for the project with application type
#
list_iap_brands() {
	local project=${1}

	if [[ -z ${project} ]]; then
		error "Error: Must specify PROJECT_ID"
		return 99
	fi

	echo "Checking for existing IAP OAuth brands in project [${project}]..."
	
	# Get brand info and format it nicely
	local brands_json
	brands_json=$(gcloud iap oauth-brands list --project="${project}" --format="json" 2>${devnull})
	
	if [[ -n ${brands_json} && ${brands_json} != "[]" ]]; then
		echo "Found existing IAP brand(s):"
		echo "NAME                                       APPLICATION_TITLE  TYPE      SUPPORT_EMAIL"
		echo "${brands_json}" | jq -r '.[] | "\(.name) \(.applicationTitle) \(if .orgInternalOnly then "Internal" else "External" end) \(.supportEmail)"' | while read -r line; do
			printf "%-40s %-18s %-9s %s\n" ${line}
		done
		return 0
	else
		echo "No existing IAP brands found."
		return 1
	fi
}

#
# Ask a yes/no question and return 0 for yes, 1 for no
#
ask() {
	local prompt="$1"
	local default="${2:-N}"
	local answer

	read -r -p "${prompt} " answer
	case "${answer:-${default}}" in
		[Yy]*) return 0 ;;
		*) return 1 ;;
	esac
}

#
# Check if a domain is authorized for the IAP brand
# Returns 0 if domain is authorized, 1 if not, 99 on error
#
check_authorized_domain() {
	local project=${1}
	local domain=${2}

	if [[ -z ${project} || -z ${domain} ]]; then
		error "Error: Must specify PROJECT_ID and DOMAIN"
		return 99
	fi

	# Extract top-level domain
	local top_level_domain
	top_level_domain=$(echo "${domain}" | awk -F. '{print $(NF-1)"."$NF}')
	
	# Always show domain authorization instructions
	echo ""
	echo "   Domain Authorization Required"
	echo ""
	echo "    To authorize this domain for OAuth redirects:"
	echo "    1. Go to https://console.cloud.google.com/apis/credentials/consent"
	echo "    2. Select project: ${project}"
	echo "    3. Go to 'Authorized domains' section"
	echo "    4. Click 'ADD DOMAIN'"
	echo "    5. Add the top-level domain: ${top_level_domain}"
	echo "    6. Click 'SAVE'"
	echo ""
	echo "    Note: You only need to add the top-level domain once."
	echo "          All subdomains will be automatically authorized."
	echo ""
	
	# Ask user to confirm domain authorization
	if ask "    Has the domain been authorized? [y/N] "; then
		echo "    Proceeding with deployment..."
		return 0
	else
		echo "    Please authorize the domain and restart deployment."
		return 1
	fi
}

#
# Provide guidance based on IAP brand status
#
oauth_guidance() {
	local main_project=${1}
	local attach_existing=${2:-false}
	local oauth_project=${3:-${main_project}}
	local dns_hostname=${4}

	if [[ -z ${main_project} ]]; then
		error "Error: Must specify MAIN_PROJECT_ID"
		return 99
	fi

	# Check if using cross-project OAuth
	if [[ ${oauth_project} != ${main_project} ]]; then
		echo ""
		echo "Info: Cross-project OAuth configuration detected."
		echo "      Main project: ${main_project}"
		echo "      OAuth project: ${oauth_project}"
		echo ""
		
		# Validate OAuth project access
		if ! gcloud projects describe "${oauth_project}" &>/dev/null; then
			error "Error: Cannot access OAuth project [${oauth_project}]."
			error "       Please ensure:"
			error "       - The project ID is correct"
			error "       - You have IAP Admin role in the OAuth project"
			error "       - The project has the IAP API enabled"
			return 1
		fi
	fi

	if check_iap_brand "${oauth_project}"; then
		# Brand exists - check if it's properly configured
		echo ""
		echo "Info: IAP brand found in project [${oauth_project}]."
		list_iap_brands "${oauth_project}"
		echo ""
		
		# Check brand application type
		if ! check_brand_type "${oauth_project}"; then
			error "Error: IAP brand application type is set to 'External'."
			error ""
			error "       OAuth clients can only be created for 'Internal' application types."
			error "       Please change the brand to 'Internal' using one of these methods:"
			error ""
			error "       Option 1 - Using GCP Console:"
			error "         1. Go to https://console.cloud.google.com/auth/overview"
			error "         2. Select project: ${oauth_project}"
			error "         3. Go to 'Audience' tab"    
			error "         4. Change 'User Type' from 'External' to 'Internal'"
			error "         5. Confirm the changes (you may need to wait for the change to apply)"
			error ""
			error "       Option 2 - Using gcloud CLI:"
			error "         # This feature may not be available in all gcloud versions"
			error "         gcloud alpha iap oauth-brands update BRAND_NAME \\"
			error "         --application_type=INTERNAL \\"
			error "         --project=${oauth_project}"
			error ""
			error "       After changing to 'Internal', run deployment again."
			error ""
			return 1
		fi

		# Check if domain is authorized if hostname provided
		if [[ -n ${dns_hostname} ]]; then
			if ! check_authorized_domain "${oauth_project}" "${dns_hostname}"; then
				return 1
			fi
		fi
		
		if [[ ${attach_existing} != "true" ]]; then
			error ""
			error "Error: An IAP OAuth brand already exists for project [${oauth_project}]."
			error ""
			if [[ ${oauth_project} != ${main_project} ]]; then
				error "       Since you're using cross-project OAuth, you likely want to"
				error "       attach to the existing brand. Add this to your configuration:"
			else
				error "       Google Cloud projects can only have ONE IAP brand."
				error "       To use the existing brand for OAuth authentication,"
				error "       add this to your configuration:"
			fi
			error ""
			error "           oauth_attach_existing: true"
			if [[ ${oauth_project} != ${main_project} ]]; then
				error "           oauth_project_id: ${oauth_project}"
			fi
			error ""
			error "       This prevents accidental conflicts with existing OAuth deployments."
			error ""
			return 1
		else
			echo ""
			if [[ ${oauth_project} != ${main_project} ]]; then
				echo "Info: Using existing Internal IAP brand from OAuth project [${oauth_project}]."
			else
				echo "Info: Using existing Internal IAP brand for OAuth authentication."
			fi
			echo ""
			return 0
		fi
	else
		echo ""
		if [[ ${oauth_project} != ${main_project} ]]; then
			echo "Info: No existing IAP brand found in OAuth project [${oauth_project}]."
			echo "      Will create new Internal brand and client in the OAuth project."
		else
			echo "Info: No existing IAP brand found. Will create new Internal brand and client."
		fi
		echo ""
		return 0
	fi
}

#
# Help function
#
help() {
	cat <<HELP
OAuth Client Management Script

This script helps manage OAuth/IAP configuration for Cluster Toolkit Frontend deployments.

Usage: ./oauth_client.sh COMMAND MAIN_PROJECT_ID [OPTIONS] [OAUTH_PROJECT_ID]

Commands:
    check_brand PROJECT_ID              - Check if IAP brand exists in project
    check_type PROJECT_ID               - Check IAP brand application type (Internal/External)
    list_brands PROJECT_ID              - List existing IAP brands in project  
    guidance MAIN_PROJECT_ID [ATTACH] [OAUTH_PROJECT_ID] - Provide setup guidance
    help                                - Show this help

Arguments:
    MAIN_PROJECT_ID                     - Main deployment project ID
    ATTACH                              - true/false for attaching to existing brand
    OAUTH_PROJECT_ID                    - Optional: Project ID for OAuth (defaults to main project)

Examples:
    ./oauth_client.sh check_brand my-project-id
    ./oauth_client.sh check_type my-project-id
    ./oauth_client.sh guidance my-project-id true
    ./oauth_client.sh guidance my-project-id false shared-oauth-project
    ./oauth_client.sh list_brands shared-oauth-project

Important Notes:
    - OAuth clients can ONLY be created for IAP brands with "Internal" application type
    - "External" brands cannot have OAuth clients - you must change them to "Internal"
    - Use the GCP Console (Security > Identity-Aware Proxy > OAuth consent screen) to change type
    - Each Google Cloud project can only have ONE IAP brand

Cross-Project OAuth:
    When using OAuth from a different project, ensure you have:
    - IAP Admin role in the OAuth project  
    - IAP API enabled in the OAuth project
    - Proper OAuth client redirect URIs configured

HELP
}

#
# Main function dispatcher
#
case "${1}" in
	check_brand)
		check_iap_brand "${2}"
		;;
	check_type)
		if check_brand_type "${2}"; then
			echo "IAP brand application type: Internal (can create OAuth clients)"
			exit 0
		else
			echo "IAP brand application type: External (cannot create OAuth clients)"
			exit 1
		fi
		;;
	list_brands)
		list_iap_brands "${2}"
		;;
	guidance)
		oauth_guidance "${2}" "${3}" "${4}" "${5}"
		;;
	help|--help|-h)
		help
		;;
	*)
		error "Error: Unknown command: ${1}"
		help
		exit 1
		;;
esac 