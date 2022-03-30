#!/bin/bash
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

# Exit on any errors
set -e

cat <<+
Welcome to the deployment of the Google HPC Frontend.

This script will ask a short series of questions to configure the frontend
before deployment.

As this deployment creates many GCP resources, plenty of permissions are
required. Owner or Editor of the hosting GCP project can deploy without
any problem. A workable set of GCP roles for successful deployment includes:

* Compute Admin
* Storage Admin
* Pub/Sub Admin
* Create Service Accounts, Delete Service Accounts, Service Account User

+

# Check for terraform
if ! command -v terraform &> /dev/null
then
    echo "Please install or make sure is in your \$PATH, terraform, version 0.13 or higher"
    exit 1
fi

if ! command -v gsutil &> /dev/null
then
    echo "Please install or make sure is in your \$PATH, gsutil, part of the Google Cloud Tools"
    exit 1
fi


# GCP project to deploy
while [ -z "${project_id}" ] ;
do
    read -p "GCP Project Name: " project_id
    if [ -z "${project_id}" ];
    then
        echo "This cannot be left blank"
        echo ""
    fi
done

read -p "  Enter a cloud zone [europe-west4-a]: " zone
zone=${zone:-europe-west4-a}
region=${zone%-*}
read -p "Deployment name [lower-case letters, numbers, no spaces]: " deployment_name

echo ""
echo "Please enter an existing GCP Subnet in which to place the webserver"
echo "or leave blank (hit enter) to have one created for you"
read -p "  GCP Subnet Name: " subnet_name

echo ""
echo "Please select deployment method of server software:"
echo "  1) Use a copy of the code from this computer"
echo "  2) Clone the git repo when server deploys"
read -p "  Please choose one of the above options:  " deploy_choice
if [ "${deploy_choice}" == "1" ];
then
    deployment="tarball"
elif [ "${deploy_choice}" == "2" ];
then
    deployment="git"
    read -p "For Git clones, please specify the path to the deployment key: " deploy_key
    if [ ! -r "${deploy_key}" ];
    then
        echo "Deployment key file cannot be read."
        exit 1
    fi
    deploy_key=$(realpath "${deploy_key}")
else
    echo "Invalid selection"
    exit 1
fi

echo ""
echo "If you specify a DNS hostname, the server will attempt to acquire a TLS"
echo "Certificate from LetsEncrypt.  If no hostname is specified, the server"
echo "will use standard, unencrypted HTTP.  This is NOT RECOMMENDED."
read -p "  Please enter the DNS hostname of the new webserver, or just hit enter for none: " hostname

echo ""
echo "Do you have a static IP address to assign to the webserver?"
echo "If so, we will assign this IP address, otherwise, we will automatically"
echo "create an ephemeral public IP address."
echo "You can create a static IP address with the command:"
echo "  $ gcloud compute addresses create <NAME> --project ${project_id} --region=${region}"
echo "  $ gcloud compute addresses describe <NAME> --project ${project_id} --region=${region} | grep 'address:'"
echo ""
echo "If you have set a hostname above, it is recommended that you use a static"
echo "IP addresses, and set the DNS record for the hostname to point at this"
echo "static address."
read -p "  Enter static IP address, or just press [Enter] for none:  " ip_address

# Collect Django admin user information
echo ""
echo "To create an admin user for this web application:"
read -p "  choose a username: " django_superuser_username
read -p "  set an initial password for this user: " django_superuser_password
read -p "  supply this user's email address: " django_superuser_email

# Collect deployment files
if [ "${deployment}" == "tarball" ] ;
then
    tar -cz -f tf/deployment.tar.gz --exclude=tf ../../hpc-toolkit 2>/dev/null
fi

cd tf

# Create Terraform setup
cat > terraform.tfvars <<+
project_id = "${project_id}"
region = "${region}"
zone = "${zone}"

deployment_name = "${deployment_name}"

django_su_username = "${django_superuser_username}"
django_su_email    = "${django_superuser_email}"
django_su_password = "${django_superuser_password}"

deployment_mode = "${deployment}"
subnet = "${subnet_name}"

extra_labels = {
    creator = "${USER}"
}
+

if [ -n "${hostname}" ]
then
    echo "webserver_hostname = \"${hostname}\"" >> terraform.tfvars
fi
if [ -n "${ip_address}" ]
then
    echo "static_ip = \"${ip_address}\"" >> terraform.tfvars
fi

if [ "${deployment}" == "git" ]; then
    echo "Will clone Hpc-Toolkit from github.com/${REPO_FORK:-GoogleCloudPlatform}.git branch ${REPO_BRANCH:-main}."
    echo "Set REPO_BRANCH and REPO_FORK environment variables to override"

    echo "repo_branch = \"${REPO_BRANCH:-main}\"" >> terraform.tfvars
    echo "repo_fork = \"${REPO_FORK:-GoogleCloudPlatform}\"" >> terraform.tfvars
    echo "deployment_key = \"${deploy_key}\"" >> terraform.tfvars
fi

echo ""
echo ""
echo "terraform.tfvars file has been created in the 'tf' directory."
echo "If you wish additional customization, please edit that file before continuing"
echo ""
read -r -p "  Are you ready to deploy? [y/N]: " ready
case "$ready" in
    [Yy]* ) break;;
    * ) echo "exiting." ; exit;;
esac

terraform init
terraform apply -auto-approve
echo ""
echo "Deployment in process.  Initialization should take about 15 minutes."
echo "After that time, please point your web browser to the above server_ip."
if [ -n "${hostname}" ]
then
    echo "and update your DNS record to point '${hostname}' to that IP."
fi

echo ""
echo "To terminate this deployment, please make sure any resources created"
echo "by the FrontEnd have been destroyed, then run 'terraform destroy' from the"
echo "'tf' directory"
echo ""

