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

# Most variables have been sourced and modified from the SchedMD/slurm-gcp
# github repository: https://github.com/SchedMD/slurm-gcp/tree/v5.1.0

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters)."
  default     = null
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "region" {
  description = "The default region for Cloud resources."
  type        = string
}

variable "zone" {
  description = "Zone in which to create all compute VMs. If `zone_policy_deny` or `zone_policy_allow` are set, the `zone` variable will be ignored."
  type        = string
  default     = null
}

variable "zone_policy_allow" {
  description = <<-EOD
    Partition nodes will prefer to be created in the listed zones. If a zone appears
    in both zone_policy_allow and zone_policy_deny, then zone_policy_deny will take
    priority for that zone.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zone_policy_allow : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A provided zone in zone_policy_allow is not a valid zone (Regexp: '^[a-z]+-[a-z]+[0-9]-[a-z]$')."
  }
}

variable "zone_policy_deny" {
  description = <<-EOD
    Partition nodes will not be created in the listed zones. If a zone appears in
    both zone_policy_allow and zone_policy_deny, then zone_policy_deny will take
    priority for that zone.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zone_policy_deny : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A provided zone in zone_policy_deny is not a valid zone (Regexp '^[a-z]+-[a-z]+[0-9]-[a-z]$')."
  }
}

variable "partition_name" {
  description = "The name of the slurm partition."
  type        = string
}

variable "partition_conf" {
  description = <<-EOD
    Slurm partition configuration as a map.
    See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION
    EOD
  type        = map(string)
  default     = {}
}

variable "is_default" {
  description = <<-EOD
    Sets this partition as the default partition by updating the partition_conf.
    If "Default" is already set in partition_conf, this variable will have no effect.
    EOD
  type        = bool
  default     = false
}

variable "machine_type" {
  description = "Compute Platform machine type to use for this partition compute nodes."
  type        = string
  default     = "c2-standard-60"
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
  default     = {}
}

variable "node_count_static" {
  description = "Number of nodes to be statically created."
  type        = number
  default     = 0
}

variable "node_conf" {
  description = "Map of Slurm node line configuration."
  type        = map(any)
  default     = {}
}

variable "node_count_dynamic_max" {
  description = "Maximum number of nodes allowed in this partition."
  type        = number
  default     = 10
}

variable "source_image_project" {
  type        = string
  description = <<-EOD
    Project path where the source image comes from. If not provided, this value
    will default to the project hosting the slurm-gcp public images. More
    information can be found in the slurm-gcp docs:
    https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image.
    EOD
  default     = null
}

variable "source_image_family" {
  type        = string
  description = <<-EOD
    Source image family. If not provided, the default image family name for the
    hpc-centos-7 version of the slurm-gcp public images will be used. More
    information can be found in the slurm-gcp docs:
    https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image
    EOD
  default     = null
}

variable "source_image" {
  type        = string
  description = <<-EOD
    Source disk image. By default, the image used will be the hpc-centos7
    version of the slurm-gcp public images. More information can be found in the
    slurm-gcp docs:
    https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image
    EOD
  default     = null
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "disk_type" {
  description = "Boot disk type, can be either pd-ssd, local-ssd, or pd-standard."
  type        = string
  default     = "pd-standard"

  validation {
    condition     = contains(["pd-ssd", "local-ssd", "pd-standard"], var.disk_type)
    error_message = "Variable disk_type must be one of pd-ssd, local-ssd, or pd-standard."
  }
}

variable "disk_size_gb" {
  description = "Size of boot disk to create for the partition compute nodes."
  type        = number
  default     = 50
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "additional_disks" {
  description = "Configurations of additional disks to be included on the partition nodes."
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_size_gb = number
    disk_type    = string
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  default = []
}

variable "enable_confidential_vm" {
  type        = bool
  description = "Enable the Confidential VM configuration. Note: the instance image must support option."
  default     = false
}

variable "enable_shielded_vm" {
  type        = bool
  description = "Enable the Shielded VM configuration. Note: the instance image must support option."
  default     = false
}

variable "enable_oslogin" {
  type        = bool
  description = <<-EOD
    Enables Google Cloud os-login for user login and authentication for VMs.
    See https://cloud.google.com/compute/docs/oslogin
    EOD
  default     = true
}

variable "can_ip_forward" {
  description = "Enable IP forwarding, for NAT instances for example."
  type        = bool
  default     = false
}

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = true
}

variable "labels" {
  description = "Labels to add to partition compute instances. List of key key, value pairs."
  type        = any
  default     = {}
}

variable "min_cpu_platform" {
  description = "The name of the minimum CPU platform that you want the instance to use."
  type        = string
  default     = null
}

variable "on_host_maintenance" {
  type        = string
  description = <<-EOD
    Instance availability Policy.

    Note: Placement groups are not supported when on_host_maintenance is set to
    "MIGRATE" and will be deactivated regardless of the value of
    enable_placement. To support enable_placement, ensure on_host_maintenance is
    set to "TERMINATE".
    EOD
  default     = "TERMINATE"
}

variable "gpu" {
  description = "Definition of requested GPU resources."
  type = object({
    count = number,
    type  = string
  })
  default = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on the partition compute nodes."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "preemptible" {
  description = "Should use preemptibles to burst."
  type        = string
  default     = false
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<-EOD
    Service account to attach to the compute instances. If not set, the
    default compute service account for the given project will be used with the
    "https://www.googleapis.com/auth/cloud-platform" scope.
    EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<-EOD
    Shielded VM configuration for the instance. Note: not used unless
    enable_shielded_vm is 'true'.
    * enable_integrity_monitoring : Compare the most recent boot measurements to the
      integrity policy baseline and return a pair of pass/fail results depending on
      whether they match or not.
    * enable_secure_boot : Verify the digital signature of all boot components, and
      halt the boot process if signature verification fails.
    * enable_vtpm : Use a virtualized trusted platform module, which is a
      specialized computer chip you can use to encrypt objects like keys and
      certificates.
    EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
  default     = null
}

variable "exclusive" {
  description = "Exclusive job access to nodes."
  type        = bool
  default     = true
}

variable "enable_placement" {
  description = "Enable placement groups."
  type        = bool
  default     = true
}

variable "enable_spot_vm" {
  description = "Enable the partition to use spot VMs (https://cloud.google.com/spot-vms)."
  type        = bool
  default     = false
}

variable "spot_instance_config" {
  description = "Configuration for spot VMs."
  type = object({
    termination_action = string
  })
  default = null
}

variable "bandwidth_tier" {
  description = <<EOT
  Configures the network interface card and the maximum egress bandwidth for VMs.
  - Setting `platform_default` respects the Google Cloud Platform API default values for networking.
  - Setting `virtio_enabled` explicitly selects the VirtioNet network adapter.
  - Setting `gvnic_enabled` selects the gVNIC network adapter (without Tier 1 high bandwidth).
  - Setting `tier_1_enabled` selects both the gVNIC adapter and Tier 1 high bandwidth networking.
  - Note: both gVNIC and Tier 1 networking require a VM image with gVNIC support as well as specific VM families and shapes.
  - See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details.
  EOT
  type        = string
  default     = "platform_default"

  validation {
    condition     = contains(["platform_default", "virtio_enabled", "gvnic_enabled", "tier_1_enabled"], var.bandwidth_tier)
    error_message = "Allowed values for bandwidth_tier are 'platform_default', 'virtio_enabled', 'gvnic_enabled', or 'tier_1_enabled'."
  }
}

variable "node_groups" {
  description = <<-EOT
    **Preview: This variable is still in development** A list of node groups
    associated with this partition.
    The default node group will be prepended to this list based on other input
    variables to this module.
    EOT
  type = list(object({
    node_count_static      = number
    node_count_dynamic_max = number
    group_name             = string
    node_conf              = map(string)
    additional_disks = list(object({
      disk_name    = string
      device_name  = string
      disk_size_gb = number
      disk_type    = string
      disk_labels  = map(string)
      auto_delete  = bool
      boot         = bool
    }))
    bandwidth_tier         = string
    can_ip_forward         = bool
    disable_smt            = bool
    disk_auto_delete       = bool
    disk_labels            = map(string)
    disk_size_gb           = number
    disk_type              = string
    enable_confidential_vm = bool
    enable_oslogin         = bool
    enable_shielded_vm     = bool
    enable_spot_vm         = bool
    gpu = object({
      count = number
      type  = string
    })
    instance_template   = string
    labels              = map(string)
    machine_type        = string
    metadata            = map(string)
    min_cpu_platform    = string
    on_host_maintenance = string
    preemptible         = bool
    service_account = object({
      email  = string
      scopes = list(string)
    })
    shielded_instance_config = object({
      enable_integrity_monitoring = bool
      enable_secure_boot          = bool
      enable_vtpm                 = bool
    })
    spot_instance_config = object({
      termination_action = string
    })
    source_image_family  = string
    source_image_project = string
    source_image         = string
    tags                 = list(string)
  }))
  default = []
}
