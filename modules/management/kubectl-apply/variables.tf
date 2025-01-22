/**
  * Copyright 2024 Google LLC
  *
  * Licensed under the Apache License, Version 2.0 (the "License");
  * you may not use this file except in compliance with the License.
  * You may obtain a copy of the License at
  *
  *      http://www.apache.org/licenses/LICENSE-2.0
  *
  * Unless required by applicable law or agreed to in writing, software
  * distributed under the License is distributed on an "AS IS" BASIS,
  * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  * See the License for the specific language governing permissions and
  * limitations under the License.
  */

locals {
  kueue_supported_versions  = ["v0.10.0", "v0.9.1", "v0.9.0", "v0.8.1"]
  jobset_supported_versions = ["v0.7.2", "v0.5.2"]
}

resource "terraform_data" "kueue_validations" {
  lifecycle {
    precondition {
      condition     = !var.kueue.install || contains(local.kueue_supported_versions, var.kueue.version)
      error_message = "Supported version of Kueue are ${join(", ", local.kueue_supported_versions)}"
    }
  }
}

resource "terraform_data" "jobset_validations" {
  lifecycle {
    precondition {
      condition     = !var.jobset.install || contains(local.jobset_supported_versions, var.jobset.version)
      error_message = "Supported version of Jobset are ${join(", ", local.jobset_supported_versions)}"
    }
  }
}

variable "project_id" {
  description = "The project ID that hosts the gke cluster."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the gke cluster resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  type        = string
  nullable    = false
}

variable "apply_manifests" {
  description = "A list of manifests to apply to GKE cluster using kubectl. For more details see [kubectl module's inputs](kubectl/README.md)."
  type = list(object({
    content           = optional(string, null)
    source            = optional(string, null)
    template_vars     = optional(map(any), null)
    server_side_apply = optional(bool, false)
    wait_for_rollout  = optional(bool, true)
  }))
  default = []
}


variable "kueue" {
  description = "Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config_path to be applied right after kueue installation. If a template file provided, its variables can be set to config_template_vars."
  type = object({
    install              = optional(bool, false)
    version              = optional(string, "v0.8.1")
    config_path          = optional(string, null)
    config_template_vars = optional(map(any), null)
  })
  default = {}
}

variable "jobset" {
  description = "Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit."
  type = object({
    install = optional(bool, false)
    version = optional(string, "v0.5.2")
  })
  default = {}
}
