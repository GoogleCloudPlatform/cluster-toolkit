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

# Trailer appended to the generated post-startup script.
# On Vertex AI Workbench Instances the image owns JupyterLab and its
# systemd unit: do NOT relocate the notebook directory or overwrite the
# unit, or the instance never becomes ready. Readiness is detected by
# wait-for-startup polling the instance state, so no completion marker
# is needed here.

echo "OFE workbench post-startup script finished at $(date)" | tee -a /tmp/startup.log
