#!/bin/bash
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
set -o pipefail
set -x

export EGO_TOP=$1
source $EGO_TOP/profile.platform

export KUBECONFIG_PATH=${2:-"$HF_TOP/conf/providers/gcpgkeinst/kubeconfig"}
export CRD_NAMESPACE=${3:-"gcp-symphony"}


mkdir -p $HF_TOP/conf/providers/gcpgkeinst/pod-specs

cat << INNER_EOF > $HF_TOP/conf/providers/gcpgkeinst/gcpgkeinstprov_config.json
{
  "GKE_KUBECONFIG": "$KUBECONFIG_PATH",
  "GKE_CRD_NAMESPACE": "$CRD_NAMESPACE",
  "LOG_LEVEL": "DEBUG"
}
INNER_EOF
