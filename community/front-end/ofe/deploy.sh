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
#                    HPC Toolkit FrontEnd deployment script                    #
#                                                                              #
################################################################################
#
#   Deployment requires a GCP project and authenticated user to run the FrontEnd
#   on GCP resources.
#
#   The FrontEnd also requires a service account with permissions and
#   credential, which needs to be created and registered with the FrontEnd once
#   it is running.
#
#   By default this script will create the account and supply the credential
#   file, but this can be skipped if the admin user already has an account they
#   prefer to use.
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
PRJ_API['oslogin.googleapis.com']='Cloud OS Login API'
PRJ_API['cloudbilling.googleapis.com']='Cloud Billing API'
PRJ_API['aiplatform.googleapis.com']='Vertex AI API'
PRJ_API['bigqueryconnection.googleapis.com']='BigQuery Connection API'
PRJ_API['sqladmin.googleapis.com']='Cloud SQL Admin API'

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
  
    deployment_name:             Name of the deployment
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
  deployment_name: MyDeployment
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
		echo "         See HPC Toolkit FrontEnd Administrator's Guide for details."
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
	#       - these could change over, depending back-end GCP / HPC Toolkit
	#         requirements, so would require maintaining.
}

#
# Check that all required APIs are enabled for the project
# - If some are missing, ask if they would like them enabled and do so.
# - Abort if declined, as cannot deploy without them.
#
check_apis() {

	local project=$1

	# Get all currently enabled APIs for this project
	#
	local enabled_apis
	enabled_apis=$(gcloud services list)

	# Check all required APIs are present, prompt and enable for any missing
	#
	local id
	for id in "${!PRJ_API[@]}"; do
		local desc=${PRJ_API[$id]}

		verbose "checking API: ${desc}"
		set +e
		echo "${enabled_apis}" | grep -q "${id}"
		local ok=$?
		set -e

		if [[ ${ok} -ne 0 ]]; then
			echo ""
			echo "Warning: Required API is not enabled: ${desc}"
			read -r -p "         Enable this API? [y/N] " reply
			case "${reply}" in
			[Yy]*)
				echo ""
				echo "  Enabling: ${desc}"
				echo "  This may take a few seconds..."
				# Enabling may fail if user doesn't have privileges to
				# modify the project.
				# Could use --async option on this command, to prevent wait,
				# but would need to check later if did this.
				set +e
				if ! gcloud services enable "${id}" \
					--project="${project}" &>/dev/null; then
					error ""
					error "  Error: Unable to modify project APIs"
					error "         Please contact to administrator"
					error ""
					error "Exiting."
					exit 1
				fi
				set -e
				;;
			*)
				echo "Exiting."
				exit 0
				;;
			esac

		fi
	done
}

#
# Create a service account with required roles to run TKFE operations
#
create_service_account() {

	local project=$1
	local server_name=$2
	local credfile=$3
	local service_account="${server_name}-tkfe-sa"

	# Check for existing service account with this name
	# - if there is an account, it can be used and a new credential generated
	#   if required.
	#
	verbose "checking for any previous existing account"

	local create=0
	local getcred=0

	if bash "${SCRIPT_DIR}/script/service_account.sh" check \
		"${project}" "${service_account}"; then

		echo ""
		echo "    Warning: Service account already exists (it is likely the deployment name"
		echo "             is being reused or a previous deployment was aborted after the"
		echo "             service account had already been created)."
		echo ""
		echo "             If the credential still exists this can be reused (assuming it"
		echo "             has the correct roles)."
		echo "             Or the account and credential can be deleted and recreated."
		echo ""
		case $(ask "    Delete and recreate? [y/N] ") in
		[Yy]*)
			if ! bash "${SCRIPT_DIR}/script/service_account.sh" delete \
				"${project}" "${service_account}"; then
				echo "Exiting."
				exit 1
			fi
			create=1
			getcred=1
			;;
		*)
			verbose "assuming reuse of account"
			echo ""
			echo "    Using existing service account: ${service_account}"
			case $(ask "    Do you want to regenerate a credential? [y/N] ") in
			[Yy]*)
				getcred=1
				;;
			*)
				echo "    Please register existing credential with FrontEnd once it is running."
				;;
			esac
			;;
		esac
	else
		create=1
		getcred=1
	fi

	if [[ ${create} -ne 0 ]]; then
		verbose "creating service account: ${account}"
		echo "    This may take a few seconds..."
		if ! bash "${SCRIPT_DIR}/script/service_account.sh" create \
			"${project}" "${service_account}"; then
			echo "Exiting."
			exit 1
		fi
	fi

	if [[ ${getcred} -ne 0 ]]; then
		verbose "creating credential file: ${credfile}"
		if ! bash "${SCRIPT_DIR}/script/service_account.sh" credential \
			"${project}" "${service_account}" "${credfile}"; then
			echo "Exiting."
			exit 1
		fi
		echo "    Credential written to:"
		echo "      ${credfile}"
		echo ""
		echo "    You will need to register the contents of this file as a credential to the"
		echo "    FrontEnd once it is running."
	fi
}

#
# Deploy the FrontEnd
#  - All deployments parameters have been obtained, so can now construct the
#    terraform recipes and initiate the deployment.
#
deploy() {

	# -- Collect deployment files
	#
	#    For a tarball deployment, it is important that the 'root' directory is
	#    named 'hpc-toolkit' as most of the install depends on it.
	#
	#    Simplest way to ensure this is to build from a temporary copy that
	#    definitely is named correctly.
	#
	if [ "${deployment_mode}" == "tarball" ]; then

		basedir=$(git rev-parse --show-toplevel)
		tdir=/tmp/hpc-toolkit

		cp -R "${basedir}" ${tdir}/
		(
			cd ${tdir}

			tar -zcf "${SCRIPT_DIR}"/tf/deployment.tar.gz \
				--exclude=.terraform \
				--exclude=.terraform.lock.hcl \
				--exclude=tf \
				--directory=/tmp \
				./hpc-toolkit 2>/dev/null
		)

		rm -rf ${tdir}
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
			echo "Will clone hpc-toolkit from github.com/${repo_fork}/hpc-toolkit.git ${repo_branch} branch."

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

		if [ -z "$config_file" ]; then
			# Ask for user confirmation before deploying
			case $(ask "    Proceed to deploy? [y/N] ") in
			[Yy]*) ;;
			*)
				echo "Exiting."
				exit 0
				;;
			esac
		fi

		# -- Start the deployment using Terraform.
		#    Note: Extract $? for terraform using PIPESTATUS, as $? below is
		#          from tee
		#          Also toggle exit on error.
		#
		set +e
		terraform init
		terraform apply -auto-approve | tee tfapply.log
		if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
			error ""
			error "Error:  Terraform failed."
			error "        Please check parameters and/or seek assistance."
			error ""
			exit 2
		fi
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

	# -- Check for terraform and gsutil
	#
	if ! command -v terraform &>/dev/null; then
		error "Error:"
		error "    Please ensure terraform (version 0.13 or higher) is in your \$PATH"
		exit 1
	fi
	if ! command -v gsutil &>/dev/null; then
		error "Error:"
		error "    Please ensure gsutil (part of Google Cloud Tools)  is in your \$PATH"
		exit 1
	fi

	cat <<FUNDAMENTALS

* GCP deployment name, project and location

    The webserver will need a name and will be run within a specified
    project and zone.  The project must have authorization and quota to use
    resources in the zone.
    
    The deployment name must contain only lowercase letters and numbers
    (no spaces)

FUNDAMENTALS

	# -- Name to use for this FrontEnd deployment
	#    This will be the name of the server VM
	#    TODO: check name is valid - i.e. right length, lowercase,...
	while [ -z "${deployment_name}" ]; do
		deployment_name=$(ask "    Deployment name")
		if [ -z "${deployment_name}" ]; then
			echo "    Error: This cannot be left blank"
		fi
		echo
		validname='^[a-z][\.a-z0-9\-]+[a-z0-9]+$'
		if [[ ! "${deployment_name}" =~ ${validname} ]]; then
			echo "    Error: Name is invalid"
			echo "              Name must have a minimum 3 characters in length"
			echo "              Only contain characters a-z (lowercase), 0-9, '-' and '.'"
			echo "              End with an alphanumeric."
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
	# -- Create service account and credential if required
	#
	cat <<SERVICEACC

* Service account & credential

    A service account with a credential to use this project can be created
    by this script.  This credential allows clusters and other resources to be
    deployed by the FrontEnd within this project.

    If requested, a service account will be created and the new credential is
    written to a file, which needs to be registered to the FrontEnd once
    running.

    Alternatively, service accounts and credentials can be created manually
    via the GCP Console or gcloud CLI.  Multiple credentials can then be
    registered to the FrontEnd for different projects, allowing the
    FrontEnd to deploy resources against these, and/or manage access at a
    finer level.  Existing service accounts with the necessary
    roles/permissions can also be used.  Please see Administrator's Guide for
    more details.

    The single service account and credential is sufficient for most use cases.

SERVICEACC
	case $(ask "    Create a service account and credential? [y/N]: ") in
	[Yy]*)
		create_service_account "${project_id}" \
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
	project_id=${yaml_array[project_id]}
	zone=${yaml_array[zone]}
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

	deploy
}

################################################################################
#

# -- Exit on any errors by default (will toggle on/off throughout this script)
set -e

cat <<HEADER

--------------------------------------------------------------------------------

                               HPC Toolkit FrontEnd

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
