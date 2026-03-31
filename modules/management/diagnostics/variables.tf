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

variable "project_id" {
  description = "The project ID that hosts the gke cluster."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the gke cluster resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  type        = string
  nullable    = false
}

variable "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster has been created."
  type        = bool
  default     = false
}

variable "ready" {
  description = "A static flag that signals to downstream modules that upstream dependencies are ready."
  type        = any
  default     = false
}

variable "k8s_service_account_name" {
  description = "Kubernetes service account name used by the gke cluster"
  type        = string
  default     = "workload-identity-k8s-sa"
}

variable "mldiagnostics" {
  description = "Unified settings for mldiagnostics"
  type = object({
    enable                      = optional(bool, false)
    workload_namespace          = optional(string, "default")
    injection_webhook_version   = optional(string, "0.25.0")
    connection_operator_version = optional(string, "0.21.0")
  })
  default = {}
}

# Validate that the workload namespace exists
resource "terraform_data" "validate_namespace" {
  count = var.mldiagnostics.enable ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(data.kubernetes_all_namespaces.all[0].namespaces, var.mldiagnostics.workload_namespace)
      error_message = "The specified workload namespace '${var.mldiagnostics.workload_namespace}' does not exist in the cluster. Please ensure configure_workload_identity_sa is enabled and k8s_service_account_namespace is set to '${var.mldiagnostics.workload_namespace}' in the gke-cluster module."
    }
  }

  depends_on = [var.gke_cluster_exists, var.ready]
}

# Validate that the workload service account exists in workload namespace and is annotated for Workload Identity
resource "terraform_data" "validate_sa" {
  count = var.mldiagnostics.enable ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(keys(try(data.kubernetes_service_account_v1.workload_sa[0].metadata[0].annotations, {})), "iam.gke.io/gcp-service-account")
      error_message = "The Service Account must be annotated for Workload Identity in workload namespace '${var.mldiagnostics.workload_namespace}'. Please ensure configure_workload_identity_sa is enabled and k8s_service_account_namespace is set to '${var.mldiagnostics.workload_namespace}' in the gke-cluster module."
    }
  }

  depends_on = [var.gke_cluster_exists, var.ready]
}

# Validate that the cert-manager namespace exists
resource "terraform_data" "validate_cert_manager" {
  count = var.mldiagnostics.enable ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(data.kubernetes_all_namespaces.all[0].namespaces, "cert-manager")
      error_message = "Cert-Manager was not found in the cluster (namespace 'cert-manager' missing). Please ensure Cert-Manager is set to install: true in kubectl-apply module as it is required by the ML Diagnostics webhook."
    }
  }

  depends_on = [var.gke_cluster_exists, var.ready]
}
