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
  description = "The project ID that hosts the GKE cluster."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the GKE cluster resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  type        = string
  nullable    = false
}

variable "node_pool_names" {
  description = "Names of node pools, for use in node affinities (Slinky system components)."
  type        = list(string)
  default     = null
}

variable "cert_manager_chart_version" {
  description = "Version of the Cert Manager chart to install."
  type        = string
  default     = "v1.18.2"
}

variable "cert_manager_values" {
  description = "Value overrides for the Cert Manager release"
  type        = any
  default = {
    crds = {
      enabled = true
    }
  }
}

variable "slurm_operator_chart_version" {
  description = "Version of the Slurm Operator chart to install."
  type        = string
  default     = "0.3.1"
}

variable "slurm_operator_values" {
  description = "Value overrides for the Slinky release"
  type        = any
  default     = {}
}

variable "slurm_chart_version" {
  description = "Version of the Slurm chart to install."
  type        = string
  default     = "0.3.1"
}

variable "slurm_values" {
  description = "Value overrides for the Slurm release"
  type        = any
  default     = {}
}

variable "install_kube_prometheus_stack" {
  # Components detailed at https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack
  description = "Install the Kube Prometheus Stack."
  type        = bool
  default     = false
}

variable "prometheus_chart_version" {
  description = "Version of the Kube Prometheus Stack chart to install."
  type        = string
  default     = "77.0.1"
}

variable "prometheus_values" {
  description = "Value overrides for the Prometheus release"
  type        = any
  default = {
    installCRDs = true
  }
}

variable "slurm_namespace" {
  description = "slurm namespace for charts"
  type        = string
  default     = "slurm"
}

variable "slurm_operator_namespace" {
  description = "slurm namespace for charts"
  type        = string
  default     = "slinky"
}

variable "install_slurm_chart" {
  description = "Install slurm-operator chart."
  type        = bool
  default     = true
}

variable "install_slurm_operator_chart" {
  description = "Install slurm-operator chart."
  type        = bool
  default     = true
}

variable "slurm_repository" {
  description = "Value overrides for the Slinky release"
  type        = string
  default     = "oci://ghcr.io/slinkyproject/charts"
}

variable "slurm_operator_repository" {
  description = "Value overrides for the Slinky release"
  type        = string
  default     = "oci://ghcr.io/slinkyproject/charts"
}
