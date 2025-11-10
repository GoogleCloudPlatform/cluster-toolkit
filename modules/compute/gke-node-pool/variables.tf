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
  description = <<-EOD
    The name of the node pool. If not set, automatically populated by machine type and module id (unique blueprint-wide) as suffix.
    If setting manually, ensure a unique value across all gke-node-pools.
    EOD
  type        = string
  default     = null

  validation {
    # Check if the variable is null OR if it matches the GCP resource naming regex.
    condition     = var.name == null || can(regex("^[a-z]([-a-z0-9]{0,34}[a-z0-9])?$", var.name))
    error_message = <<-EOD
    If provided, the node pool name must be between 1 and 36 characters, start with a lowercase letter, end with an alphanumeric, and contain only lowercase letters, numbers, and hyphens.
    Underscores are not allowed. A shorter length is enforced to accommodate a suffix when creating multiple node pools.
    EOD
  }
}

variable "internal_ghpc_module_id" {
  description = "DO NOT SET THIS MANUALLY. Automatically populates with module id (unique blueprint-wide)."
  type        = string
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
  default     = null
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
    type  = optional(string)
    count = optional(number, 0)
    gpu_driver_installation_config = optional(object({
      gpu_driver_version = string
    }), { gpu_driver_version = "DEFAULT" })
    gpu_partition_size = optional(string)
    gpu_sharing_config = optional(object({
      gpu_sharing_strategy       = string
      max_shared_clients_per_gpu = number
    }))
  }))
  default  = []
  nullable = false

  validation {
    condition     = alltrue([for ga in var.guest_accelerator : ga.count != null])
    error_message = "var.guest_accelerator[*].count cannot be null"
  }

  validation {
    condition     = alltrue([for ga in var.guest_accelerator : ga.count >= 0])
    error_message = "var.guest_accelerator[*].count must never be negative"
  }

  validation {
    condition     = alltrue([for ga in var.guest_accelerator : ga.gpu_driver_installation_config != null])
    error_message = "var.guest_accelerator[*].gpu_driver_installation_config must not be null; leave unset to enable GKE to select default GPU driver installation"
  }
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
  When set to null,  default value either is [set based on machine_type](https://cloud.google.com/compute/docs/disks/local-ssd#choose_number_local_ssds) or GKE decides about default value.
  [See above](#local-ssd-storage) for more info.
  EOT 
  type        = number
  default     = null
}

variable "local_ssd_count_nvme_block" {
  description = <<-EOT
  The number of local SSDs to attach to each node to back block storage.  
  Uses NVMe interfaces.  Must be supported by `machine_type`.
  When set to null,  default value either is [set based on machine_type](https://cloud.google.com/compute/docs/disks/local-ssd#choose_number_local_ssds) or GKE decides about default value.
  [See above](#local-ssd-storage) for more info.
  
  EOT 
  type        = number
  default     = null
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

variable "is_reservation_active" {
  description = "Whether the specified reservation is already created."
  type        = bool
  default     = true
}

variable "auto_repair" {
  description = "Whether the nodes will be automatically repaired."
  type        = bool
  default     = true
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

# tflint-ignore: terraform_unused_declarations
variable "compact_placement" {
  description = "DEPRECATED: Use `placement_policy`"
  type        = bool
  default     = null
  validation {
    condition     = var.compact_placement == null
    error_message = "`compact_placement` is deprecated. Use `placement_policy` instead"
  }
}

variable "placement_policy" {
  description = <<-EOT
  Group placement policy to use for the node pool's nodes. `COMPACT` is the only supported value for `type` currently. `name` is the name of the placement policy. `tpu_topology` is the TPU placement topology for pod slice node pool.
  It is assumed that the specified policy exists. To create a placement policy refer to https://cloud.google.com/sdk/gcloud/reference/compute/resource-policies/create/group-placement.
  Note: Placement policies have the [following](https://cloud.google.com/compute/docs/instances/placement-policies-overview#restrictions-compact-policies) restrictions.
  EOT

  type = object({
    type         = string
    name         = optional(string)
    tpu_topology = optional(string)
  })
  default = {
    type         = null
    name         = null
    tpu_topology = null
  }
  validation {
    condition     = var.placement_policy.type == null || try(contains(["COMPACT"], var.placement_policy.type), false)
    error_message = "`COMPACT` is the only supported value for `placement_policy.type`."
  }
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
  default = []
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

variable "additional_networks" {
  description = "Additional network interface details for GKE, if any. Providing additional networks adds additional node networks to the node pool"
  default     = []
  type = list(object({
    network            = string
    subnetwork         = string
    subnetwork_project = string
    network_ip         = string
    nic_type           = string
    stack_type         = string
    queue_count        = number
    access_config = list(object({
      nat_ip       = string
      network_tier = string
    }))
    ipv6_access_config = list(object({
      network_tier = string
    }))
    alias_ip_range = list(object({
      ip_cidr_range         = string
      subnetwork_range_name = string
    }))
  }))
  nullable = false
}

variable "reservation_affinity" {
  description = <<-EOT
  Reservation resource to consume. When targeting SPECIFIC_RESERVATION, specific_reservations needs be specified.
  Even though specific_reservations is a list, only one reservation is allowed by the NodePool API.
  It is assumed that the specified reservation exists and has available capacity.
  For a shared reservation, specify the project_id as well in which it was created.
  To create a reservation refer to https://cloud.google.com/compute/docs/instances/reservations-single-project and https://cloud.google.com/compute/docs/instances/reservations-shared
  EOT
  type = object({
    consume_reservation_type = string
    specific_reservations = optional(list(object({
      name    = string
      project = optional(string)
    })))
  })
  default = {
    consume_reservation_type = "NO_RESERVATION"
    specific_reservations    = []
  }
  validation {
    condition     = contains(["NO_RESERVATION", "ANY_RESERVATION", "SPECIFIC_RESERVATION"], var.reservation_affinity.consume_reservation_type)
    error_message = "Accepted values are: {NO_RESERVATION, ANY_RESERVATION, SPECIFIC_RESERVATION}"
  }
}

variable "host_maintenance_interval" {
  description = "Specifies the frequency of planned maintenance events."
  type        = string
  default     = ""
  nullable    = false
  validation {
    condition     = contains(["", "PERIODIC", "AS_NEEDED"], var.host_maintenance_interval)
    error_message = "Invalid host_maintenance_interval value. Must be PERIODIC, AS_NEEDED or the empty string"
  }
}

variable "initial_node_count" {
  description = "The initial number of nodes for the pool. In regional clusters, this is the number of nodes per zone. Changing this setting after node pool creation will not make any effect. It cannot be set with static_node_count and must be set to a value between autoscaling_total_min_nodes and autoscaling_total_max_nodes."
  type        = number
  default     = null
}

variable "gke_version" {
  description = "GKE version"
  type        = string
}

variable "max_pods_per_node" {
  description = "The maximum number of pods per node in this node pool. This will force replacement."
  type        = number
  default     = null
}

variable "upgrade_settings" {
  description = <<-EOT
  Defines node pool upgrade settings. It is highly recommended that you define all max_surge and max_unavailable.
  If max_surge is not specified, it would be set to a default value of 0.
  If max_unavailable is not specified, it would be set to a default value of 1.  
  EOT
  type = object({
    strategy        = string
    max_surge       = optional(number)
    max_unavailable = optional(number)
  })
  default = {
    strategy        = "SURGE"
    max_surge       = 0
    max_unavailable = 1
  }
}

variable "run_workload_script" {
  description = "Whether execute the script to create a sample workload and inject rxdm sidecar into workload. Currently, implemented for A3-Highgpu and A3-Megagpu only."
  type        = bool
  default     = true
}

variable "enable_queued_provisioning" {
  description = "If true, enables Dynamic Workload Scheduler and adds the cloud.google.com/gke-queued taint to the node pool."
  type        = bool
  default     = false
}

variable "enable_flex_start" {
  description = <<-EOT
  If true, start the node pool with Flex Start provisioning model.
  To learn more about flex-start mode, please refer to
  https://cloud.google.com/kubernetes-engine/docs/how-to/dws-flex-start-training and
  https://cloud.google.com/kubernetes-engine/docs/how-to/provisioningrequest
  EOT
  type        = bool
  default     = false
}

variable "max_run_duration" {
  description = "The duration (in whole seconds) of the instance. Instance will run and be terminated after then."
  type        = number
  default     = null
}

variable "enable_private_nodes" {
  description = "Whether nodes have internal IP addresses only."
  type        = bool
  default     = true
}

variable "num_node_pools" {
  description = "Number of node pools to create. This is same as num_slices."
  type        = number
  default     = 1
}

variable "num_slices" {
  description = "Number of TPUs slices to create. This is same as num_node_pools."
  type        = number
  default     = 1
}

variable "enable_numa_aware_scheduling" {
  description = "Enable [NUMA-aware](https://cloud.google.com/kubernetes-engine/distributed-cloud/bare-metal/docs/vm-runtime/numa) scheduling."
  type        = bool
  default     = false
}
