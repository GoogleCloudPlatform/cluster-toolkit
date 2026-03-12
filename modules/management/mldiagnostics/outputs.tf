# Copyright 2026 "Google LLC"
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

output "instructions" {
  description = "GKE ML Diagnostics cluster created"
  value       = <<-EOT
    ML Diagnostics has been successfully installed in the cluster: ${local.cluster_name}.
    - Cert-manager is installed in the 'cert-manager' namespace.
    - ML Diagnostics Webhook and Connection Operator are installed in the 'gke-mldiagnostics' namespace.
  EOT
}
