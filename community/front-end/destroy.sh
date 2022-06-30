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
#                 Google HPC Toolkit FrontEnd destroy script                   #
#                                                                              #
################################################################################

#
#
#
tfdestroy() {
    
    cd tf

    # -- If there's no lock file, there shouldn't be any Front End deployed.
    #
    if [[ ! -f .tkfe.lock ]]; then
	echo "Warning: No lock file found"
	echo "         It is likely there is no Front End currently deployed"
	read -r -p "          Proceed anyway? [y/N]: " ready
	case "$ready" in
	    [Yy]*) ;;
	    *)
		echo "exiting."
		exit 0
		;;
	esac
	
    fi

    name=`terraform show | awk '/ghpcfe_id/ {print $3; exit}'`
    
    echo ""
    echo "  This will destroy the running Front End:" ${name}
    echo "  Please ensure all resources deployed by the Front End have been deleted."
    echo ""
    read -r -p "  Proceed? [y/N]: " ready
    case "$ready" in
	[Yy]*) ;;
	*)
	    echo "Exiting."
	    exit 0
	    ;;
    esac
    #
    # TODO - Spawn a shutdown script on FE server, via gcloud, which finds all
    #        running clusters and 'terraform destroy's them.
    #        This will give a totally clean shut down, leaving no GCP resources
    #        attributable to this FE.
    #
    
    #
    # -- Start the deployment using Terraform
    #
    terraform destroy -auto-approve | tee tfdestroy.log
    
    # -- Remove the lock file
    #
    rm -f .tkfe.lock 2> /dev/null
}


################################################################################
#

# -- Exit on any errors
set -e

# -- Splash screen
#
cat <<+

--------------------------------------------------------------------------------

                        Google HPC Toolkit FrontEnd

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

tfdestroy

wait
exit 0

#
# eof

