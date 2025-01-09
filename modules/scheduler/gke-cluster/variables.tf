/**
 * Copyright 2022 Google LLC
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

variable "name_suffix" {
  description = "Custom cluster name postpended to the `deployment_name`. See `prefix_with_deployment_name`."
  type        = string
  default     = ""
}

variable "deployment_name" {
  description = "Name of the HPC deployment. Used in the GKE cluster name by default and can be configured with `prefix_with_deployment_name`."
  type        = string
}

variable "prefix_with_deployment_name" {
  description = "If true, cluster name will be prefixed by `deployment_name` (ex: <deployment_name>-<name_suffix>)."
  type        = bool
  default     = true
}

variable "region" {
  description = "The region to host the cluster in."
  type        = string
}

variable "zone" {
  description = "Zone for a zonal cluster."
  default     = null
  type        = string
}

variable "network_id" {
  description = "The ID of the GCE VPC network to host the cluster given in the format: `projects/<project_id>/global/networks/<network_name>`."
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to host the cluster in."
  type        = string
}

variable "pods_ip_range_name" {
  description = "The name of the secondary subnet ip range to use for pods."
  type        = string
  default     = "pods"
}

variable "services_ip_range_name" {
  description = "The name of the secondary subnet range to use for services."
  type        = string
  default     = "services"
}

variable "enable_private_ipv6_google_access" {
  description = "The private IPv6 google access type for the VMs in this subnet."
  type        = bool
  default     = true
}

variable "release_channel" {
  description = "The release channel of this cluster. Accepted values are `UNSPECIFIED`, `RAPID`, `REGULAR` and `STABLE`."
  type        = string
  default     = "UNSPECIFIED"
}

variable "min_master_version" {
  description = "The minimum version of the master. If unset, the cluster's version will be set by GKE to the version of the most recent official release."
  type        = string
  default     = null
}

variable "version_prefix" {
  description = "If provided, Terraform will only return versions that match the string prefix. For example, `1.31.` will match all `1.31` series releases. Since this is just a string match, it's recommended that you append a `.` after minor versions to ensure that prefixes such as `1.3` don't match versions like `1.30.1-gke.10` accidentally."
  type        = string
  default     = "1.31."
}

variable "maintenance_start_time" {
  description = "Start time for daily maintenance operations. Specified in GMT with `HH:MM` format."
  type        = string
  default     = "09:00"
}

variable "maintenance_exclusions" {
  description = "List of maintenance exclusions. A cluster can have up to three."
  type = list(object({
    name            = string
    start_time      = string
    end_time        = string
    exclusion_scope = string
  }))
  default = []
  validation {
    condition = alltrue([
      for x in var.maintenance_exclusions :
      contains(["NO_UPGRADES", "NO_MINOR_UPGRADES", "NO_MINOR_OR_NODE_UPGRADES"], x.exclusion_scope)
    ])
    error_message = "`exclusion_scope` must be set to `NO_UPGRADES` OR `NO_MINOR_UPGRADES` OR `NO_MINOR_OR_NODE_UPGRADES`."
  }
}

variable "enable_filestore_csi" {
  description = "The status of the Filestore Container Storage Interface (CSI) driver addon, which allows the usage of filestore instance as volumes."
  type        = bool
  default     = false
}

variable "enable_gcsfuse_csi" {
  description = "The status of the GCSFuse Filestore Container Storage Interface (CSI) driver addon, which allows the usage of a gcs bucket as volumes."
  type        = bool
  default     = false
}

variable "enable_persistent_disk_csi" {
  description = "The status of the Google Compute Engine Persistent Disk Container Storage Interface (CSI) driver addon, which allows the usage of a PD as volumes."
  type        = bool
  default     = true
}

variable "enable_parallelstore_csi" {
  description = "The status of the Google Compute Engine Parallelstore Container Storage Interface (CSI) driver addon, which allows the usage of a parallelstore as volumes."
  type        = bool
  default     = false
}

variable "enable_dcgm_monitoring" {
  description = "Enable GKE to collect DCGM metrics"
  type        = bool
  default     = false
}

variable "enable_node_local_dns_cache" {
  description = "Enable GKE NodeLocal DNSCache addon to improve DNS lookup latency"
  type        = bool
  default     = false
}

variable "system_node_pool_enabled" {
  description = "Create a system node pool."
  type        = bool
  default     = true
}

variable "system_node_pool_name" {
  description = "Name of the system node pool."
  type        = string
  default     = "system"
}

variable "system_node_pool_node_count" {
  description = "The total min and max nodes to be maintained in the system node pool."
  type = object({
    total_min_nodes = number
    total_max_nodes = number
  })
  default = {
    total_min_nodes = 2
    total_max_nodes = 10
  }
}

variable "system_node_pool_machine_type" {
  description = "Machine type for the system node pool."
  type        = string
  default     = "e2-standard-4"
}

variable "system_node_pool_disk_size_gb" {
  description = "Size of disk for each node of the system node pool."
  type        = number
  default     = 100
}

variable "system_node_pool_disk_type" {
  description = "Disk type for each node of the system node pool."
  type        = string
  default     = null
}

variable "system_node_pool_taints" {
  description = "Taints to be applied to the system node pool."
  type = list(object({
    key    = string
    value  = any
    effect = string
  }))
  default = [{
    key    = "components.gke.io/gke-managed-components"
    value  = true
    effect = "NO_SCHEDULE"
  }]
}

variable "system_node_pool_kubernetes_labels" {
  description = <<-EOT
  Kubernetes labels to be applied to each node in the node group. Key-value pairs. 
  (The `kubernetes.io/` and `k8s.io/` prefixes are reserved by Kubernetes Core components and cannot be specified)
  EOT
  type        = map(string)
  default     = null
}
variable "system_node_pool_image_type" {
  description = "The default image type used by NAP once a new node pool is being created. Use either COS_CONTAINERD or UBUNTU_CONTAINERD."
  type        = string
  default     = "COS_CONTAINERD"
}
variable "system_node_pool_enable_secure_boot" {
  description = "Enable secure boot for the nodes.  Keep enabled unless custom kernel modules need to be loaded. See [here](https://cloud.google.com/compute/shielded-vm/docs/shielded-vm#secure-boot) for more info."
  type        = bool
  default     = true
}

variable "enable_private_nodes" {
  description = "(Beta) Whether nodes have internal IP addresses only."
  type        = bool
  default     = true
}

variable "enable_private_endpoint" {
  description = "(Beta) Whether the master's internal IP address is used as the cluster endpoint."
  type        = bool
  default     = true
}

variable "master_ipv4_cidr_block" {
  description = "(Beta) The IP range in CIDR notation to use for the hosted master network."
  type        = string
  default     = "172.16.0.32/28"
}

variable "enable_master_global_access" {
  description = "Whether the cluster master is accessible globally (from any region) or only within the same region as the private endpoint."
  type        = bool
  default     = false
}

variable "gcp_public_cidrs_access_enabled" {
  description = "Whether the cluster master is accessible via all the Google Compute Engine Public IPs. To view this list of IP addresses look here https://cloud.google.com/compute/docs/faq#find_ip_range"
  type        = bool
  default     = false
}

variable "master_authorized_networks" {
  description = "External network that can access Kubernetes master through HTTPS. Must be specified in CIDR notation."
  type = list(object({
    cidr_block   = string
    display_name = string
  }))
  default = []
}

variable "service_account_email" {
  description = "Service account e-mail address to use with the system node pool"
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to to use with the system node pool."
  type        = set(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
}

variable "configure_workload_identity_sa" {
  description = "When true, a kubernetes service account will be created and bound using workload identity to the service account used to create the cluster."
  type        = bool
  default     = false
}

variable "autoscaling_profile" {
  description = "(Beta) Optimize for utilization or availability when deciding to remove nodes. Can be BALANCED or OPTIMIZE_UTILIZATION."
  type        = string
  default     = "OPTIMIZE_UTILIZATION"
}

variable "authenticator_security_group" {
  description = "The name of the RBAC security group for use with Google security groups in Kubernetes RBAC. Group name must be in format gke-security-groups@yourdomain.com"
  type        = string
  default     = null
}

variable "enable_dataplane_v2" {
  description = "Enables [Dataplane v2](https://cloud.google.com/kubernetes-engine/docs/concepts/dataplane-v2). This setting is immutable on clusters. If null, will default to false unless using multi-networking, in which case it will default to true"
  type        = bool
  default     = null
}

variable "labels" {
  description = "GCE resource labels to be applied to resources. Key-value pairs."
  type        = map(string)
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

variable "enable_multi_networking" {
  description = "Enables [multi networking](https://cloud.google.com/kubernetes-engine/docs/how-to/setup-multinetwork-support-for-pods#create-a-gke-cluster) (Requires GKE Enterprise). This setting is immutable on clusters and enables [Dataplane V2](https://cloud.google.com/kubernetes-engine/docs/concepts/dataplane-v2?hl=en). If null, will determine state based on if additional_networks are passed in."
  type        = bool
  default     = null
}

variable "additional_networks" {
  description = "Additional network interface details for GKE, if any. Providing additional networks enables multi networking and creates relevat network objects on the cluster."
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
}

variable "cluster_reference_type" {
  description = "How the google_container_node_pool.system_node_pools refers to the cluster. Possible values are: {SELF_LINK, NAME}"
  default     = "SELF_LINK"
  type        = string
  nullable    = false
  validation {
    condition     = contains(["SELF_LINK", "NAME"], var.cluster_reference_type)
    error_message = "`cluster_reference_type` must be one of {SELF_LINK, NAME}"
  }
}

variable "cluster_availability_type" {
  description = "Type of cluster availability. Possible values are: {REGIONAL, ZONAL}"
  default     = "REGIONAL"
  type        = string
  nullable    = false
  validation {
    condition     = contains(["REGIONAL", "ZONAL"], var.cluster_availability_type)
    error_message = "`cluster_availability_type` must be one of {REGIONAL, ZONAL}"
  }
}

variable "default_max_pods_per_node" {
  description = "The default maximum number of pods per node in this cluster."
  type        = number
  default     = null
}

variable "networking_mode" {
  description = "Determines whether alias IPs or routes will be used for pod IPs in the cluster. Options are VPC_NATIVE or ROUTES. VPC_NATIVE enables IP aliasing. The default is VPC_NATIVE."
  type        = string
  default     = "VPC_NATIVE"
}

variable "deletion_protection" {
  description = <<-EOT
  "Determines if the cluster can be deleted by gcluster commands or not".
  To delete a cluster provisioned with deletion_protection set to true, you must first set it to false and apply the changes.
  Then proceed with deletion as usual.
  EOT
  type        = bool
  default     = false
}

variable "upgrade_settings" {
  description = <<-EOT
  Defines gke cluster upgrade settings. It is highly recommended that you define all max_surge and max_unavailable.
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
