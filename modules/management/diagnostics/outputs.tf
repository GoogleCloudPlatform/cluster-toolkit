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
  value = var.mldiagnostics.enable ? (<<-EOT
    ML Diagnostics has been successfully installed in the cluster: ${local.cluster_name}.
    - Validated that Cert-Manager is installed (namespace 'cert-manager' found).
    - Validated that the workload service account '${var.k8s_service_account_name}' exists and is annotated for Workload Identity.
    - ML Diagnostics Webhook and Connection Operator are installed in the '${local.mldiagnostics_namespace}' namespace.

    IMPORTANT:
    - Workloads must be deployed in the '${var.namespace}' namespace.
    - That namespace has been labeled 'managed-mldiagnostics-gke: "true"' to enable webhook injection.
    - Ensure your workload pods use the Kubernetes Service Account configured with Workload Identity - '${var.k8s_service_account_name}'.
  EOT
  ) : null
}
