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
  # This list defines the Kueue Helm chart versions that are officially tested and supported by this toolkit, based on the official changelog.
  # The list should be updated as new versions are tested and approved.
  # Refer https://github.com/kubernetes-sigs/kueue/tree/main/CHANGELOG
  kueue_supported_versions = ["0.13.3", "0.13.2", "0.13.1", "0.13.0"]

  # Officially supported latest helm chart versions of Jobset.
  # For details refer the official change log https://github.com/kubernetes-sigs/jobset/releases
  jobset_supported_versions    = ["0.10.1", "0.10.0", "0.9.1", "0.9.0"]
  gib_supported_versions_x86   = ["v1.0.2", "v1.0.3", "v1.0.5", "v1.0.6", "v1.1.0"]
  gib_supported_versions_arm64 = ["v1.0.7"]
  gib_supported_versions = var.target_architecture == "arm64" ? (
    local.gib_supported_versions_arm64
    ) : (
    local.gib_supported_versions_x86
  )
}

variable "target_architecture" {
  description = "The target architecture for the GKE nodes and gIB plugin (e.g., 'x86_64' or 'arm64')."
  type        = string
  default     = "x86_64"
  validation {
    condition     = contains(["x86_64", "arm64"], var.target_architecture)
    error_message = "The target_architecture must be either 'x86_64' or 'arm64'."
  }
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

resource "terraform_data" "gib_validations" {
  lifecycle {
    precondition {
      condition     = !var.gib.install || contains(local.gib_supported_versions, var.gib.template_vars.version)
      error_message = "Supported version of the NCCL gIB plugin for architecture ${var.target_architecture} are ${join(", ", local.gib_supported_versions)}"
    }
  }
}

resource "terraform_data" "initial_gib_version" {
  input = var.gib.install ? var.gib.template_vars.version : null

  lifecycle {
    ignore_changes = [input]
  }
}

check "gib_version_changes" {
  assert {
    # Skip version checking if gIB was not initially or is not currently installed
    condition     = terraform_data.initial_gib_version.output == null || !var.gib.install || terraform_data.initial_gib_version.output == var.gib.template_vars.version
    error_message = "When changing the gIB NCCL plugin version, confirm full rollout and environment consistency. Replace any NCCL env hard coding/caches with set_nccl_env.sh sourcing."
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
  description = "A list of manifests to apply to GKE cluster using kubectl. For more details see [kubectl module's inputs](kubectl/README.md).\n NOTE: The `enable` input acts as a FF to apply a manifest or not. By default it is always set to `true`. "
  type = list(object({
    enable            = optional(bool, true)
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
    version              = optional(string, "0.13.3")
    config_path          = optional(string, null)
    config_template_vars = optional(map(any), null)
  })
  default = {}
}

variable "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster has been created. Needed by community/modules/scripts/kubernetes-operations."
  type        = bool
  default     = false
}

variable "jobset" {
  description = "Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit."
  type = object({
    install = optional(bool, false)
    version = optional(string, "0.10.1")
  })
  default = {}
}


variable "gpu_operator" {
  description = "Install [GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html) which uses the [Kubernetes operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) to automate the management of all NVIDIA software components needed to provision GPU."
  type = object({
    install = optional(bool, false)
    version = optional(string, "v25.3.0")
  })
  default = {}
}

variable "nvidia_dra_driver" {
  description = "Installs [Nvidia DRA driver](https://github.com/NVIDIA/k8s-dra-driver-gpu) which supports Dynamic Resource Allocation for NVIDIA GPUs in Kubernetes"
  type = object({
    install = optional(bool, false)
    version = optional(string, "v25.3.0")
  })
  default = {}
}

variable "gib" {
  description = "Install the NCCL gIB plugin"
  type = object({
    install = bool
    path    = string
    template_vars = object({
      image   = optional(string, "us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-plugin-gib")
      version = string
      node_affinity = optional(any, {
        requiredDuringSchedulingIgnoredDuringExecution = {
          nodeSelectorTerms = [{
            matchExpressions = [{
              key      = "cloud.google.com/gke-gpu",
              operator = "In",
              values   = ["true"]
            }]
          }]
        }
      })
      accelerator_count = number
      max_unavailable   = optional(string, "50%")
    })
  })
  default = {
    install = false
    path    = ""
    template_vars = {
      version           = ""
      accelerator_count = 0
    }
  }
}
