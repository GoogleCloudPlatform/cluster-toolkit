#!/bin/bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

export EGO_TOP=$1
export GCP_PROJECT_ID=$2
export PUBSUB_TOPIC=$3
export PUBSUB_SUBSCRIPTION=$4

# shellcheck source=/dev/null
. "$EGO_TOP/profile.platform"

cat >"$HF_TOP/conf/providers/gcpgceinst/gcpgceinstprov_config.json" <<EOF
{
  "GCP_PROJECT_ID": "${GCP_PROJECT_ID}",
  "LOG_LEVEL":"DEBUG",
  "PUBSUB_TOPIC": "${PUBSUB_TOPIC}",
  "PUBSUB_SUBSCRIPTION": "${PUBSUB_SUBSCRIPTION}",
  "PUBSUB_AUTOLAUNCH": true,
  "PUBSUB_TIMEOUT": 100
}
EOF
