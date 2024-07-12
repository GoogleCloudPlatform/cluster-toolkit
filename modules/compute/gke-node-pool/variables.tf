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

variable "zones" {
  description = "A list of zones to be used. Zones must be in region of cluster. If null, cluster zones will be inherited. Note `zones` not `zone`; does not work with `zone` deployment variable."
  type        = list(string)
  default     = null
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

variable "disk_size_gb" {
  description = "Size of disk for each node."
  type        = number
  default     = 100
}

variable "disk_type" {
  description = "Disk type for each node."
  type        = string
  default     = "pd-standard"
}

variable "enable_gcfs" {
  description = "Enable the Google Container Filesystem (GCFS). See [restrictions](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster#gcfs_config)."
  type        = bool
  default     = false
}

variable "enable_secure_boot" {
  description = "Enable secure boot for the nodes.  Keep enabled unless custom kernel modules need to be loaded. See [here](https://cloud.google.com/compute/shielded-vm/docs/shielded-vm#secure-boot) for more info."
  type        = bool
  default     = true
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string
    count = number
    gpu_driver_installation_config = list(object({
      gpu_driver_version = string
    }))
    gpu_partition_size = string
    gpu_sharing_config = list(object({
      gpu_sharing_strategy       = string
      max_shared_clients_per_gpu = number
    }))
  }))
  default = null
}

variable "image_type" {
  description = "The default image type used by NAP once a new node pool is being created. Use either COS_CONTAINERD or UBUNTU_CONTAINERD."
  type        = string
  default     = "COS_CONTAINERD"
}

variable "local_ssd_count_ephemeral_storage" {
  description = <<-EOT
  The number of local SSDs to attach to each node to back ephemeral storage.  
  Uses NVMe interfaces.  Must be supported by `machine_type`.
  [See above](#local-ssd-storage) for more info.
  EOT 
  type        = number
  default     = 0
}

variable "local_ssd_count_nvme_block" {
  description = <<-EOT
  The number of local SSDs to attach to each node to back block storage.  
  Uses NVMe interfaces.  Must be supported by `machine_type`.
  [See above](#local-ssd-storage) for more info.
  
  EOT 
  type        = number
  default     = 0
}


variable "autoscaling_total_min_nodes" {
  description = "Total minimum number of nodes in the NodePool."
  type        = number
  default     = 0
}

variable "autoscaling_total_max_nodes" {
  description = "Total maximum number of nodes in the NodePool."
  type        = number
  default     = 1000
}

variable "static_node_count" {
  description = "The static number of nodes in the node pool. If set, autoscaling will be disabled."
  type        = number
  default     = null
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

variable "service_account_email" {
  description = "Service account e-mail address to use with the node pool"
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to to use with the node pool."
  type        = set(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
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

variable "kubernetes_labels" {
  description = <<-EOT
  Kubernetes labels to be applied to each node in the node group. Key-value pairs. 
  (The `kubernetes.io/` and `k8s.io/` prefixes are reserved by Kubernetes Core components and cannot be specified)
  EOT
  type        = map(string)
  default     = null
}

variable "timeout_create" {
  description = "Timeout for creating a node pool"
  type        = string
  default     = null
}

variable "timeout_update" {
  description = "Timeout for updating a node pool"
  type        = string
  default     = null
}

# Deprecated

# tflint-ignore: terraform_unused_declarations
variable "total_min_nodes" {
  description = "DEPRECATED: Use autoscaling_total_min_nodes."
  type        = number
  default     = null
  validation {
    condition     = var.total_min_nodes == null
    error_message = "total_min_nodes was renamed to autoscaling_total_min_nodes and is deprecated; use autoscaling_total_min_nodes"
  }
}

# tflint-ignore: terraform_unused_declarations
variable "total_max_nodes" {
  description = "DEPRECATED: Use autoscaling_total_max_nodes."
  type        = number
  default     = null
  validation {
    condition     = var.total_max_nodes == null
    error_message = "total_max_nodes was renamed to autoscaling_total_max_nodes and is deprecated; use autoscaling_total_max_nodes"
  }
}

# tflint-ignore: terraform_unused_declarations
variable "service_account" {
  description = "DEPRECATED: use service_account_email and scopes."
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = null
  validation {
    condition     = var.service_account == null
    error_message = "service_account is deprecated and replaced with service_account_email and scopes."
  }
}
