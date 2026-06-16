/**
  * Copyright 2026 Google LLC
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

  # Note: The apiVersion associated with the kueue resources should be kueue.x-k8s.io/v1beta2 when using v0.15.0 or higher.
  # Refer: https://github.com/kubernetes-sigs/kueue/blob/main/CHANGELOG/CHANGELOG-0.15.md#v0150
  kueue_supported_versions = ["0.17.1", "0.16.0", "0.15.3", "0.15.2", "0.15.1", "0.15.0"]

  # Officially supported latest helm chart versions of Jobset.
  # For details refer the official change log https://github.com/kubernetes-sigs/jobset/releases
  jobset_supported_versions    = ["0.10.1", "0.10.0", "0.9.1", "0.9.0"]
  gib_supported_versions_x86   = ["v1.0.2", "v1.0.3", "v1.0.5", "v1.0.6", "v1.1.0"]
  gib_supported_versions_arm64 = ["v1.1.1", "v1.1.0", "v1.0.7"]
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
    precondition {
      condition     = !var.kueue.install || !(var.enable_pathways_for_tpus || try(var.kueue.enable_pathways_for_tpus, false)) || try(var.kueue.config_path, "") != "" || contains(keys(coalesce(var.kueue.config_template_vars, {})), "accelerator_type")
      error_message = "accelerator_type must be set in kueue.config_template_vars when using the default pathways configuration."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || var.kueue.accelerator_topology_mode == "PROVISION_ONLY"
      error_message = "When enable_dynamic_slicing_for_tpus is true, accelerator_topology_mode must be 'PROVISION_ONLY'."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || (var.kueue.machine_type != null && length(regexall("^(tpu|ct)", var.kueue.machine_type)) > 0)
      error_message = "When enable_dynamic_slicing_for_tpus is true, machine_type must be a TPU machine type."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || coalesce(var.kueue.config_path, "") != "" || (var.kueue.config_template_vars != null && contains(keys(var.kueue.config_template_vars), "accelerator_type"))
      error_message = "accelerator_type must be set in kueue.config_template_vars when using the default dynamic slicing configuration."
    }
    precondition {
      condition     = !(var.kueue.enable_dynamic_slicing_for_tpus && !var.kueue.install)
      error_message = "Slice controller requires Kueue to be installed. Set kueue.install to true when kueue.enable_dynamic_slicing_for_tpus is true."
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
  description = "A list of manifests to apply to the GKE cluster using helm_install. For more details on the underlying deployment mechanism, see the [helm_install module](helm_install/README.md). The `enable` input acts as a FF to apply a manifest or not. By default it is always set to `true`. "
  type = list(object({
    name             = optional(string, null)
    enable           = optional(bool, true)
    content          = optional(string, null)
    source           = optional(string, null)
    template_vars    = optional(map(any), null)
    wait_for_rollout = optional(bool, true)
    namespace        = optional(string, null)
  }))
  default = []

  validation {
    condition     = alltrue([for m in var.apply_manifests : m.name == null || length(m.name) <= 44])
    error_message = "The 'name' attribute in apply_manifests must not exceed 44 characters to ensure the final Helm release name fits within the 53-character limit."
  }
}


variable "kueue" {
  description = "Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config_path to be applied right after kueue installation. If a template file provided, its variables can be set to config_template_vars."
  type = object({
    # ATTENTION: If you update the KUEUE's default version below, please also update the corresponding
    # defaultKueueVersion constant in pkg/orchestrator/gke/infra_manager.go. (note the 'v' prefix there)
    version                         = optional(string, "0.17.1")
    install                         = optional(bool, false)
    config_path                     = optional(string, null)
    config_template_vars            = optional(map(any), null)
    enable_pathways_for_tpus        = optional(bool, false)
    enable_dynamic_slicing_for_tpus = optional(bool, false)
    accelerator_topology_mode       = optional(string, null)
    machine_type                    = optional(string, null)
    controller_cpu                  = optional(string, null)
    controller_memory               = optional(string, null)
    controller_replicas             = optional(number, null)
    slice_controller_cpu_request    = optional(string, "8000m")
    slice_controller_memory_request = optional(string, "16Gi")
    slice_controller_cpu_limit      = optional(string, "12000m")
    slice_controller_memory_limit   = optional(string, "32Gi")
  })
  default = {}
}

variable "enable_pathways_for_tpus" {
  description = "Enable Pathways for TPUs. This is automatically wired from gke-cluster module if used."
  type        = bool
  default     = false
}

variable "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster has been created."
  type        = bool
  default     = false
}

variable "jobset" {
  description = "Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit."
  type = object({
    install           = optional(bool, false)
    version           = optional(string, "0.10.1")
    controller_cpu    = optional(string, null)
    controller_memory = optional(string, null)
  })
  default = {}
}

variable "cert_manager" {
  description = "Install [cert-manager](https://cert-manager.io/docs/) which manages TLS certificates for Kubernetes."
  type = object({
    install = optional(bool, false)
    version = optional(string, "v1.17.2")
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
    install          = optional(bool, false)
    version          = optional(string, "v25.3.0")
    accelerator_type = optional(string, "nvidia-gb200")
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

variable "system_node_pool_id" {
  description = "The ID of the system node pool. Used to ensure the node pool remains active during Kueue uninstallation."
  type        = string
  default     = null
}

variable "asapd_lite" {
  description = "Install the asapd-lite daemonset for A4X-Max Bare Metal."
  type = object({
    install              = optional(bool, false)
    config_path          = optional(string, null)
    config_template_vars = optional(map(any), {})
  })
  default = {}
}

variable "module_id" {
  description = "The ID of the module as defined in the blueprint. Injected by ghpc."
  type        = string
  default     = "kubectl-apply" # Fallback if run manually
}

variable "cluster_endpoint" {
  description = "The endpoint of the GKE cluster."
  type        = string
  default     = null
}

variable "cluster_ca_certificate" {
  description = "The base64 encoded CA certificate of the GKE cluster. Must be base64 encoded; the module internally decodes this value using base64decode(...) before passing it to the providers."
  type        = string
  default     = null
}

variable "access_token" {
  description = "The access token for Kubernetes/Helm providers."
  type        = string
  sensitive   = true
  default     = null
}

variable "service_account_annotations" {
  description = "Optional map of service accounts and workload identity emails to patch natively via HCL."
  type = map(object({
    namespace                 = string
    gcp_service_account_email = string
  }))
  default = {}
}
