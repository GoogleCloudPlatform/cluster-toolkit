#!/bin/bash
# Copyright 2024 "Google LLC"
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

set -ex

depl_name="e2e-${BUILD_ID:0:6}"
region="us-central1"
zone="us-central1-a"
vars="project_id=$PROJECT_ID,deployment_name=$depl_name,region=$region,zone=$zone"

# Already in a root of the repo
make
./ghpc deploy tools/cloud-build/daily-tests/blueprints/e2e.yaml --vars="$vars" -l ERROR --auto-approve

# check instance was created
gcloud compute instances describe "${depl_name}-0" --project="$PROJECT_ID" --zone="$zone" >/dev/null

./ghpc destroy "$depl_name" --auto-approve
