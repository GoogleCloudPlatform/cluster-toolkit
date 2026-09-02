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
export TEMPLATE_ID=${2:-"sym-pod-c2-4"}
export MAX_NUMBER=${3:-100}
export CPUS=${4:-4}
export RAM=${5:-16384}
export POD_SPEC=${6:-"pod-specs/pod-spec.yaml"}
export FULL_IMAGE_NAME=$7

source $EGO_TOP/profile.platform

# Get master IP and Hostname
MASTER_IP=$(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip)
MASTER_HOSTNAME=$(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/hostname | cut -d. -f1)

# Replace placeholders in /tmp/Symphony/pod-spec.yaml
sed -i "s|COMPUTE_IMAGE_PLACEHOLDER|${FULL_IMAGE_NAME}|g" /tmp/Symphony/pod-spec.yaml
sed -i "s|MASTER_IP_PLACEHOLDER|${MASTER_IP}|g" /tmp/Symphony/pod-spec.yaml
sed -i "s|MASTER_HOSTNAME_PLACEHOLDER|${MASTER_HOSTNAME}|g" /tmp/Symphony/pod-spec.yaml

echo "Updated /tmp/Symphony/pod-spec.yaml with image and master IP"

mkdir -p $HF_TOP/conf/providers/gcpgkeinst/pod-specs
cp /tmp/Symphony/pod-spec.yaml $HF_TOP/conf/providers/gcpgkeinst/pod-specs/pod-spec.yaml

cat <<INNER_EOF >$HF_TOP/conf/providers/gcpgkeinst/gcpgkeinstprov_templates.json
{
  "templates": [
    {
      "templateId": "$TEMPLATE_ID",
      "maxNumber": $MAX_NUMBER,
      "attributes": {
        "type": [
          "String",
          "X86_64"
        ],
        "ncores": [
          "Numeric",
          "$CPUS"
        ],
        "ncpus": [
          "Numeric",
          "$CPUS"
        ],
        "nram": [
          "Numeric",
          "$RAM"
        ]
      },
      "podSpecYaml": "$POD_SPEC"
    }
  ]
}
INNER_EOF
