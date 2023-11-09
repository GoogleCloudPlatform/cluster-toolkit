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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "instance_count" {
  description = "Number of instances"
  type        = number
  default     = 1
}

variable "instance_image" {
  description = "Instance Image"
  type        = map(string)
  default = {
    project = "cloud-hpc-image-public"
    family  = "hpc-centos-7"
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "disk_size_gb" {
  description = "Size of disk for instances."
  type        = number
  default     = 200
}

variable "disk_type" {
  description = "Disk type for instances."
  type        = string
  default     = "pd-standard"
}

variable "auto_delete_boot_disk" {
  description = "Controls if boot disk should be auto-deleted when instance is deleted."
  type        = bool
  default     = true
}

variable "local_ssd_count" {
  description = "The number of local SSDs to attach to each VM. See https://cloud.google.com/compute/docs/disks/local-ssd."
  type        = number
  default     = 0
}

variable "local_ssd_interface" {
  description = "Interface to be used with local SSDs. Can be either 'NVME' or 'SCSI'. No effect unless `local_ssd_count` is also set."
  type        = string
  default     = "NVME"
}

variable "name_prefix" {
  description = <<-EOT
    An optional name for all VM and disk resources. 
    If not supplied, `deployment_name` will be used. 
    When `name_prefix` is supplied, and `add_deployment_name_before_prefix` is set, 
    then resources are named by "<`deployment_name`>-<`name_prefix`>-<#>".
    EOT
  type        = string
  default     = null
}

variable "add_deployment_name_before_prefix" {
  description = <<-EOT
    If true, the names of VMs and disks will always be prefixed with `deployment_name` to enable uniqueness across deployments.
    See `name_prefix` for further details on resource naming behavior.
    EOT
  type        = bool
  default     = false
}

variable "disable_public_ips" {
  description = "If set to true, instances will not have public IPs"
  type        = bool
  default     = false
}

variable "machine_type" {
  description = "Machine type to use for the instance creation"
  type        = string
  default     = "c2-standard-60"
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured."
  type = list(object({
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
  }))
  default = []
}

variable "deployment_name" {
  description = "Name of the deployment, will optionally be used name resources according to `name_prefix`"
  type        = string
}

variable "labels" {
  description = "Labels to add to the instances. Key-value pairs."
  type        = map(string)
}

variable "service_account" {
  description = "Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account."
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email  = null
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }
}

variable "network_self_link" {
  description = "The self link of the network to attach the VM. Can use \"default\" for the default network."
  type        = string
  default     = null
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to attach the VM."
  type        = string
  default     = null
}

variable "network_interfaces" {
  description = <<-EOT
    A list of network interfaces. The options match that of the terraform
    network_interface block of google_compute_instance. For descriptions of the
    subfields or more information see the documentation:
    https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance#nested_network_interface

    **_NOTE:_** If `network_interfaces` are set, `network_self_link` and
    `subnetwork_self_link` will be ignored, even if they are provided through
    the `use` field. `bandwidth_tier` and `disable_public_ips` also do not apply
    to network interfaces defined in this variable.

    Subfields:
    network            (string, required if subnetwork is not supplied)
    subnetwork         (string, required if network is not supplied)
    subnetwork_project (string, optional)
    network_ip         (string, optional)
    nic_type           (string, optional, choose from ["GVNIC", "VIRTIO_NET"])
    stack_type         (string, optional, choose from ["IPV4_ONLY", "IPV4_IPV6"])
    queue_count        (number, optional)
    access_config      (object, optional)
    ipv6_access_config (object, optional)
    alias_ip_range     (list(object), optional)
    EOT
  type = list(object({
    network            = string,
    subnetwork         = string,
    subnetwork_project = string,
    network_ip         = string,
    nic_type           = string,
    stack_type         = string,
    queue_count        = number,
    access_config = list(object({
      nat_ip                 = string,
      public_ptr_domain_name = string,
      network_tier           = string
    })),
    ipv6_access_config = list(object({
      public_ptr_domain_name = string,
      network_tier           = string
    })),
    alias_ip_range = list(object({
      ip_cidr_range         = string,
      subnetwork_range_name = string
    }))
  }))
  default = []
  validation {
    condition = alltrue([
      for ni in var.network_interfaces : (ni.network == null) != (ni.subnetwork == null)
    ])
    error_message = "All additional network interfaces must define exactly one of \"network\" or \"subnetwork\"."
  }
  validation {
    condition = alltrue([
      for ni in var.network_interfaces : ni.nic_type == "GVNIC" || ni.nic_type == "VIRTIO_NET" || ni.nic_type == null
    ])
    error_message = "In the variable network_interfaces, field \"nic_type\" must be either \"GVNIC\", \"VIRTIO_NET\" or null."
  }
  validation {
    condition = alltrue([
      for ni in var.network_interfaces : ni.stack_type == "IPV4_ONLY" || ni.stack_type == "IPV4_IPV6" || ni.stack_type == null
    ])
    error_message = "In the variable network_interfaces, field \"stack_type\" must be either \"IPV4_ONLY\", \"IPV4_IPV6\" or null."
  }
}

variable "region" {
  description = "The region to deploy to"
  type        = string
}

variable "zone" {
  description = "Compute Platform zone"
  type        = string
}

variable "metadata" {
  description = "Metadata, provided as a map"
  type        = map(string)
  default     = {}
}

variable "startup_script" {
  description = "Startup script used on the instance"
  type        = string
  default     = null
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default  = []
  nullable = false
}

variable "automatic_restart" {
  description = "Specifies if the instance should be restarted if it was terminated by Compute Engine (not a user)."
  type        = bool
  default     = null
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE`"
  type        = string
  default     = null
  validation {
    condition     = var.on_host_maintenance == null ? true : contains(["MIGRATE", "TERMINATE"], var.on_host_maintenance)
    error_message = "When set, the on_host_maintenance must be set to MIGRATE or TERMINATE."
  }
}

variable "bandwidth_tier" {
  description = <<EOT
  Tier 1 bandwidth increases the maximum egress bandwidth for VMs.
  Using the `tier_1_enabled` setting will enable both gVNIC and TIER_1 higher bandwidth networking.
  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER_1.
  Note that TIER_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details.
  EOT
  type        = string
  default     = "not_enabled"

  validation {
    condition     = contains(["not_enabled", "gvnic_enabled", "tier_1_enabled"], var.bandwidth_tier)
    error_message = "Allowed values for bandwidth_tier are 'not_enabled', 'gvnic_enabled', or  'tier_1_enabled'."
  }
}

variable "placement_policy" {
  description = <<-EOT
  Control where your VM instances are physically located relative to each other within a zone.
  See https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_resource_policy#nested_group_placement_policy
  EOT

  type    = any # It's a workaround of lack of `optional` in Terraform 1.2
  default = null
  validation {
    condition     = var.placement_policy == null ? true : try(keys(var.placement_policy), null) != null
    error_message = <<-EOT
    The var.placement_policy should be either unset/null or be a map/object with 
    fields: vm_count (number), availability_domain_count (number), collocation (string), max_distance (number).
    EOT
  }

  validation {
    condition = alltrue([
      for k in try(keys(var.placement_policy), []) : contains([
      "vm_count", "availability_domain_count", "collocation", "max_distance"], k)
    ])
    error_message = <<-EOT
    The supported fields for var.placement_policy are:
    vm_count (number), availability_domain_count (number), collocation (string), max_distance (number).
    EOT
  }
}

variable "spot" {
  description = "Provision VMs using discounted Spot pricing, allowing for preemption"
  type        = bool
  default     = false
}

variable "tags" {
  description = "Network tags, provided as a list"
  type        = list(string)
  default     = []
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

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
  validation {
    condition     = var.enable_oslogin == null ? false : contains(["ENABLE", "DISABLE", "INHERIT"], var.enable_oslogin)
    error_message = "Allowed string values for var.enable_oslogin are \"ENABLE\", \"DISABLE\", or \"INHERIT\"."
  }
}
