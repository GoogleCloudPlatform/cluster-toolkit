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
#               Google HPC Toolkit FrontEnd deployment script                  #
#                                                                              #
################################################################################


SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )


# Default GCP zone to use
default_zone='europe-west4-a'


# help function
#   - write out purpose and preprequisites
#
help() {
    cat <<+
  This script will ask a short series of questions to configure the FrontEnd
  web application before deployment.

  The Google Cloud CLI and terraform must be installed before use.
  These can be downloaded from:
      https://cloud.google.com/cli
      https://www.terraform.io/downloads

  A valid GCP project must already be created and cloud credentials must be
  associated with your GCP account.  If credentials haven't been authorised
  already, this command can be used:
      $ gcloud auth application-default login

  The GCP project must have the following APIs enabled:
      - Compute Engine API
      - Cloud Monitoring API
      - Cloud Logging API
      - Cloud Pub/Sub API
      - Cloud Resource Manager
      - Identity and Access Management (IAM) API
      - Cloud OS Login API
      - Cloud Filestore API
      - Cloud Billing API
      - Vertex AI API

  As this deployment creates many GCP resources, a number of permissions are
  required.  An Owner or Editor of the hosting GCP project can deploy without
  any problem. Otherwise, a workable set of GCP roles for successful deployment
  need to be added to the account, which includes:

      - Compute Admin
      - Storage Admin
      - Pub/Sub Admin
      - Create Service Accounts, Delete Service Accounts, Service Account User
        (or Service Account Admin)
      - Project IAM Admin

+
#'
}


# Function for capturing user entry.
#  - Has an option to hide the response, which is useful for passwords.
#  - Accepts a default that is used when no user entry.
#
# Usage:
#   ask [--hidden] PROMPT [DEFAULT]
#
ask() {
        
    # set if we are hiding the typing
    unset hidden
    if [[ "${1}" == '--hidden' ]]; then
	hidden=1;
	shift
    fi
    
    # get the question string
    question="${1}"
    shift
    
    # get any default value
    unset default
    if [[ ${1} ]]; then
	default=${1}
	printf "${question} [default: ${default}]" >&2
    else
	printf "${question}" >&2
    fi
    
    charcount='0'
    prompt='> '
    reply=''
    if [[ ${hidden} ]]; then
	while IFS='' read -n '1' -p "${prompt}" -r -s 'char'
	do
            case "${char}" in
		# Handle NULL
		( $'\000' )
		    break
		    ;;
		# Handle BACKSPACE and DELETE
		( $'\010' | $'\177' )
		    if (( charcount > 0 )); then
			prompt=$'\b \b'
			reply="${reply%?}"
			(( charcount-- ))
		    else
			prompt=''
		    fi
		    ;;
		( * )
		    prompt='*'
		    reply+="${char}"
		    (( charcount++ ))
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
    touch ${SCRIPT_DIR}/tf/.tkfe.lock
}

checklock() {
    
    # -- If lock exists, there is a FrontEnd already deployed, so abort as
    #    deploying another will overwrite the terraform files, leaving the
    #    current deployment impossible to destroy, so then dangling and in
    #    need of manual clean-up via Google console.
    #
    if [[ -f ${SCRIPT_DIR}/tf/.tkfe.lock ]]; then
	echo ""
	echo "Error:  A lock file has been found."
	echo ""
	echo "    A FrontEnd has already been deployed from this location."
	echo ""
	echo "    Either destroy existing FrontEnd by deleting all resources via web"
	echo "    application, then use ./teardown.sh"
	echo "    Or ensure all resources have been removed via Google Console and delete the"
	echo "    lock file: tf/.tkfe.lock"
	echo ""
	exit 0
    fi
}

    
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
    if [ "${deployment}" == "tarball" ]; then
	sdir=${PWD}
	tdir=/tmp/hpc-toolkit
	cp -R ../.. ${tdir}/
	cd ${tdir}
	tar -zcf ${sdir}/tf/deployment.tar.gz --exclude=tf ../hpc-toolkit 2>/dev/null
	cd ${sdir}
	rm -rf ${tdir}
    fi


    cd tf

    # -- Create Terraform setup
    #
    cat >terraform.tfvars <<+
project_id = "${project_id}"
region = "${region}"
zone = "${zone}"

deployment_name = "${deployment_name}"

django_su_username = "${django_superuser_username}"
django_su_password = "${django_superuser_password}"
django_su_email    = "${django_superuser_email}"

deployment_mode = "${deployment}"
subnet = "${subnet_name}"

extra_labels = {
    creator = "${USER}"
}
+
    if [ -n "${dns_hostname}" ]; then
	echo "webserver_hostname = \"${dns_hostname}\"" >>terraform.tfvars
    fi
    if [ -n "${ip_address}" ]; then
	echo "static_ip = \"${ip_address}\"" >>terraform.tfvars
    fi

    if [ "${deployment}" == "git" ]; then
	echo "Will clone hpc-toolkit from github.com/${REPO_FORK:-GoogleCloudPlatform}.git branch ${REPO_BRANCH:-main}."
	echo "Set REPO_BRANCH and REPO_FORK environment variables to override"

	cat >>terraform.tfvars <<+
    repo_branch = "${REPO_BRANCH:-main}"
    repo_fork = "${REPO_FORK:-GoogleCloudPlatform}"
    deployment_key = "${deploy_key}"
+
    fi

    echo ""
#    echo "terraform.tfvars file has been created in the 'tf' directory."
#    echo "If you wish additional customization, please edit that file before continuing."
#    echo ""

    # TODO - upload terraform files to the FE server, so that they can be recovered if ever
    #        needed
    
    ready=$(ask '  Proceed to deploy? [y/N]: ')
    case "$ready" in
	[Yy]*) ;;
	*)
	    echo "Exiting."
	    exit 0
	    ;;
    esac

    # -- Start the deployment using Terraform.
    #    Note: Extract $? for terraform using PIPESTATUS, as $? below is just from tee
    #          Also toggle exit on error.
    #
    set +e
    terraform init
    terraform apply -auto-approve | tee tfapply.log
    if [[ ${PIPESTATUS[0]} -ne 0 ]]; then
	echo "Error:  Terraform failed."
	echo "        Please check parameters and/or seek assistance."
	echo ""
	exit 2
    fi
    set -e
    
    echo ""
    echo "Deployment in progress."
    echo ""
    echo "  Started at:" `date`
    echo "  Initialization should take about 15 minutes."
    echo "  After that time, please point your web browser to the above server_ip."
    if [ -n "${dns_hostname}" ]; then
	echo "  Also update your DNS record to point '${dns_hostname}' to that IP."
    fi

    # TODO - Put in an optional wait, to confirm back to the user when the FE is ready
    #        A ping every 30s may work?   Need to capture IP address and then:
    #           until $?==0 do: ping -q -c 1 -w 5 IPADDRESS >/dev/null

    # 
    #
    echo ""
    echo "To terminate this deployment, please make sure any resources created"
    echo "within the FrontEnd have been deleted, then run ./teardown.sh"
    echo ""

    # -- Set FE lock file, to ensure only one deployment is performed.
    #
    setlock
}


#
#
#
standard_setup() {

    # -- Ensure that there is no pre-deployed FE from this location.
    #
    checklock
        
    help
    cat <<+
--------------------------------------------------------------------------------

+
	
    # -- Check for terraform and gsutil
    #
    if ! command -v terraform &>/dev/null; then
	echo "Error:"
	echo "   Please ensure terraform (version 0.13 or higher) is in your \$PATH"
	exit 1
    fi
    if ! command -v gsutil &>/dev/null; then
	echo "Error:"
	echo "   Please ensure gsutil (part of Google Cloud Tools)  is in your \$PATH"
	exit 1
    fi

    # TODO - Check default authorisation has been set up
    #        'gcloud auth list' could be used


    cat <<+

* GCP deployment name, project and location

    The webserver will need a name within GCP and will be run within a specified
    project and zone.  The project must have authorization and quota to use
    resources in the zone.
    
    The deployment name must contain only lower-case letters and numbers (no
    spaces)

+
    # -- Name to use for this FrontEnd deployment
    #    This will be the name of the server VM
    #
    deployment_name=$(ask '    Deployment name')
    
    # -- GCP project to deploy into
    #
    while [ -z "${project_id}" ]; do
	project_id=$(ask '    GCP Project ID')
	if [ -z "${project_id}" ]; then
	    echo "    Error: This cannot be left blank"
	    echo ""
	else
	    # Check project_id is valid.
	    # Toggle error exit so check works.
	    set +e
	    gcloud projects describe ${project_id} >/dev/null 2>&1
	    if [[ $? -ne 0 ]]; then
		echo "    Error: Invalid project ID"
		echo "           Please check you are using the project ID and not the project name"
		echo "           and that you have access to the project"
		echo ""
		project_id=""
	    fi
	    set -e
	fi
    done
    
    # -- GCP zone/region to use
    #    + clip datacenter from zone to get the region
    #
    zone=$(ask '    GCP zone' ${default_zone})
    region=${zone%-*}
    
    cat <<+

* GCP subnet

    Enter existing subnet in which to place the webserver,
    or leave empty to have one created.

+
    subnet_name=$(ask '    GCP subnet name (or just press Enter)')

    cat <<+ 

* DNS hostname
    
    If you specify a DNS hostname for the new webserver, we will attempt to
    acquire a TLS Certificate from LetsEncrypt.
    
    If this is left blank with no hostname specified, the server will use
    standard, unencrypted HTTP.  This is not recommended.
    
+
    dns_hostname=$(ask '    DNS hostname (or just press Enter)')

    cat <<+

* IP address
    
    If you specify a static IP address it will be assigned to the server.
    If not specified, an ephemeral public IP address will be created.
    You can create and get a static IP address with the commands:

    $ gcloud compute addresses create <NAME> --project ${project_id} \\
           --region=${region}
    $ gcloud compute addresses describe <NAME> --project ${project_id} \\
           --region=${region} | grep 'address:'

    If you have set a hostname above, it is recommended that you use a static IP
    address and set the DNS record for the hostname to point at this.
   
+
    ip_address=$(ask '    Static IP address (or just press Enter)')

    #
    # -- Collect Django admin user information
    #
    cat <<+

* Admin user creation

    Please give details of admin user for the web application.

+
    django_superuser_username=$(ask '    Username')
    django_superuser_password=$(ask --hidden '    Password')
    password_check=$(ask --hidden '    Re-enter password')

    if [[ ${django_superuser_password} != ${password_check} ]]; then
	echo "Error:"
	echo "   Passwords do not match - exiting"
	exit 1
    fi
    
    django_superuser_email=$(ask '    Admin user email address')


    #
    # Have restricted deployment to only be via tarball
    #
    # TODO - Reinstate option to deploy from git, once close to formal release
    #        and location and access to open repository is available.
    #
    #        Will need to make sure that names, etc., are correct and don't break
    #        deployment and startup scripts (e.g. 'root' directory name must be
    #        "hpc-toolkit"
    #
    #echo ""
    #echo "Please select deployment method of server software:"
    #echo "  1) Use a copy of the code from this computer"
    #echo "  2) Clone the git repo when server deploys"
    #deploy_choice=$(ask '  Please choose one of the above options', '1')
    #if [ "${deploy_choice}" == "1" ]; then
    #	deployment="tarball"
    #elif [ "${deploy_choice}" == "2" ]; then
    #	deployment="git"
    #	deploy_key=$(ask 'For Git clones, please specify the path to the deployment key')
    #	if [ ! -r "${deploy_key}" ]; then
    #		echo "Deployment key file cannot be read."
    #		exit 1
    #	fi
    #	deploy_key=$(realpath "${deploy_key}")
    #else
    #	echo "Invalid selection"
    #	exit 1
    #fi
    deployment="tarball"


    # -- check user is ok to proceed with entered parameters:
    #
    echo ""
    echo "***  Deployment summary:  ***"
    echo ""
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
#    ready=$(ask '  Are details correct? [y/N]: ')
#    case "$ready" in
#	[Yy]*) ;;
#	*)
#	    echo "Exiting."
#	    exit 0
#	    ;;
#    esac
}


#
#
#
quick_setup() {
    #
    # -- Shortened setup, for expert user
    #
    checklock
    deployment_name=$(ask '  Deployment name')
    project_id=$(ask '  GCP project ID')
    zone=$(ask '  GCP zone' ${default_zone})
    region=${zone%-*}
    subnet_name=$(ask '  GCP subnet   [blank to create]')
    dns_hostname=$(ask '  DNS hostname [blank to create]')
    ip_address=$(ask '  IP address   [blank to create]')
    django_superuser_username=$(ask '  Username')
    django_superuser_password=$(ask --hidden '  Password')
    password_check=$(ask --hidden '  Re-enter password')
    if [[ ${django_superuser_password} != ${password_check} ]]; then
	echo "Error:"
	echo "   Passwords do not match - Exiting"
	exit 1
    fi
    django_superuser_email=$(ask '  Admin user email address')
    deployment="tarball"
}


################################################################################
#

# -- Exit on any errors by default (will toggle on/off throughout this script)
set -e

cat <<+

--------------------------------------------------------------------------------

                        Google HPC Toolkit FrontEnd

--------------------------------------------------------------------------------

+

while [[ $# -gt 0 ]]; do
    case "$1" in
	-q)
	    quick=1
	    ;;
	-h|--help)
	    help
	    exit 0
	    ;;
    esac
    shift
done

if [[ $quick ]]; then
    quick_setup
else
    standard_setup
fi

deploy

wait
exit 0

#
# eof
