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

# shellcheck source=/dev/null
. "$EGO_TOP/profile.platform"

cat >"$HF_TOP/conf/hostfactoryconf.json" <<EOF
{
  "HF_LOGLEVEL": "LOG_TRACE",
  "HF_LOG_MAX_FILE_SIZE": 10,
  "HF_LOG_MAX_ROTATE": 5,
  "HF_REQUESTOR_POLL_INTERVAL": 30,
  "HF_HOUSEKEEPING_LOOP_INTERVAL": 30,
  "HF_REST_TRANSPORT": "TCPIPv4",
  "HF_REST_LISTEN_PORT": 9080,
  "HF_REQUESTOR_ACTION_TIMEOUT": 240,
  "HF_PROVIDER_ACTION_TIMEOUT": 300,
  "HF_DB_HISTORY_DURATION": 90,
  "HF_REST_RESULT_DEFAULT_PAGESIZE": 2000,
  "HF_REST_RESULT_MAX_PAGESIZE": 10000,
  "HF_DEMAND_BATCH_LIMIT": 100,
  "HF_RETURN_BATCH_LIMIT": 100
}
EOF
