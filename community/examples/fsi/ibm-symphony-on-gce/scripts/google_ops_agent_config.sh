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

mkdir -p /etc/google-cloud-ops-agent

cat > /etc/google-cloud-ops-agent/config.yaml << INNER_EOF
logging:
  receivers:
    syslog:
      type: files
      include_paths:
      - /var/log/messages
      - /var/log/syslog
      - ${EGO_TOP}/kernel/log/*.log*
      - ${EGO_TOP}/hostfactory/log/*.log*
  service:
    pipelines:
      default_pipeline:
        receivers: [syslog]
metrics:
  receivers:
    hostmetrics:
      type: hostmetrics
      collection_interval: 60s
  processors:
    metrics_filter:
      type: exclude_metrics
      metrics_pattern: []
  service:
    pipelines:
      default_pipeline:
        receivers: [hostmetrics]
        processors: [metrics_filter]
INNER_EOF

systemctl restart google-cloud-ops-agent || service google-cloud-ops-agent restart || true
