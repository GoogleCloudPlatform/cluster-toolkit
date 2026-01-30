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
  description = "A static flag that signals to downstream modules that a cluster has been created. Needed by community/modules/scripts/kubernetes-operations."
  type        = bool
  default     = false
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

variable "kueue" {
  description = "Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config_path to be applied right after kueue installation. If a template file provided, its variables can be set to config_template_vars."
  type = object({
    install              = optional(bool, false)
    version              = optional(string, "v0.11.4")
    config_path          = optional(string, null)
    config_template_vars = optional(map(any), null)
  })
  default = {}
}

variable "jobset" {
  description = "Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit."
  type = object({
    install = optional(bool, false)
    version = optional(string, "v0.7.2")
  })
  default = {}
}
