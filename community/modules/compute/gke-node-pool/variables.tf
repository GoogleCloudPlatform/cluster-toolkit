/**
 * Copyright 2023 Google LLC
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
  description = "The project ID to host the cluster in."
  type        = string
}

variable "cluster_id" {
  description = "projects/{{project}}/locations/{{location}}/clusters/{{cluster}}"
  type        = string
}

variable "name" {
  description = "The name of the node pool. If left blank, will default to the machine type."
  type        = string
  default     = null
}

variable "machine_type" {
  description = "The name of a Google Compute Engine machine type."
  type        = string
  default     = "c2-standard-60"
}

variable "image_type" {
  description = "The default image type used by NAP once a new node pool is being created. Use either COS_CONTAINERD or UBUNTU_CONTAINERD."
  type        = string
  default     = "COS_CONTAINERD"
}

# TODO
variable "total_min_nodes" {
  description = "Total minimum number of nodes in the NodePool."
  type        = number
  default     = 0
}

variable "total_max_nodes" {
  description = "Total maximum number of nodes in the NodePool."
  type        = number
  default     = 1000
}

variable "auto_upgrade" {
  description = "Whether the nodes will be automatically upgraded."
  type        = bool
  default     = false
}

variable "threads_per_core" {
  description = <<-EOT
  Sets the number of threads per physical core. By setting threads_per_core
  to 2, Simultaneous Multithreading (SMT) is enabled extending the total number
  of virtual cores. For example, a machine of type c2-standard-60 will have 60
  virtual cores with threads_per_core equal to 2. With threads_per_core equal
  to 1 (SMT turned off), only the 30 physical cores will be available on the VM.

  The default value of \"0\" will turn off SMT for supported machine types, and
  will fall back to GCE defaults for unsupported machine types (t2d, shared-core
  instances, or instances with less than 2 vCPU).

  Disabling SMT can be more performant in many HPC workloads, therefore it is
  disabled by default where compatible.

  null = SMT configuration will use the GCE defaults for the machine type
  0 = SMT will be disabled where compatible (default)
  1 = SMT will always be disabled (will fail on incompatible machine types)
  2 = SMT will always be enabled (will fail on incompatible machine types)
  EOT
  type        = number
  default     = 0

  validation {
    condition     = var.threads_per_core == null || try(var.threads_per_core >= 0, false) && try(var.threads_per_core <= 2, false)
    error_message = "Allowed values for threads_per_core are \"null\", \"0\", \"1\", \"2\"."
  }
}

variable "spot" {
  description = "Provision VMs using discounted Spot pricing, allowing for preemption"
  type        = bool
  default     = false
}

variable "compact_placement" {
  description = "Places node pool's nodes in a closer physical proximity in order to reduce network latency between nodes."
  type        = bool
  default     = false
}

variable "service_account" {
  description = "Service account to use with the system node pool"
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email  = null
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }
}

variable "taints" {
  description = "Taints to be applied to the system node pool."
  type = list(object({
    key    = string
    value  = any
    effect = string
  }))
  default = [{
    key    = "user-workload"
    value  = true
    effect = "NO_SCHEDULE"
  }]
}

variable "labels" {
  description = "GCE resource labels to be applied to resources. Key-value pairs."
  type        = map(string)
}
