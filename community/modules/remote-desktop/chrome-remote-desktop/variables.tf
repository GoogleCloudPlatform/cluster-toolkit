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
  description = "Project in which Google Cloud resources will be created"
  type        = string
}

variable "deployment_name" {
  description = "HPC Toolkit deployment name. Cloud resource names will include this value."
  type        = string
  #default     = "chrome-remote-desktop"
}

variable "region" {
  description = "Default region for creating resources"
  type        = string
}

variable "zone" {
  description = "Default zone for creating resources"
  type        = string
}

variable "instance_count" {
  description = "Number of instances"
  type        = number
  default     = 1
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

variable "instance_image" {
  description = <<-EOD
    Image used to build chrome remote desktop node. The default image is from
    family= \"debian-11\" and project = \"debian-cloud\". An alternative image is
    from family  = \"ubuntu-2204-lts\" and project = \"ubuntu-os-cloud\".

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "debian-cloud"
    family  = "debian-11"
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
  default     = "pd-balanced"
}

variable "auto_delete_boot_disk" {
  description = "Controls if boot disk should be auto-deleted when instance is deleted."
  type        = bool
  default     = true
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

variable "enable_public_ips" {
  description = "If set to true, instances will have public IPs on the internet."
  type        = bool
  default     = true
}

variable "machine_type" {
  description = "Machine type to use for the instance creation. Must be N1 family if GPU is used."
  type        = string
  default     = "n1-standard-8"
}

variable "labels" {
  description = "Labels to add to the instances. Key-value pairs."
  type        = map(string)
  default     = {}
}

variable "service_account" {
  description = "Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account."
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email = null
    scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

variable "network_self_link" {
  description = "The self link of the network to attach the VM."
  type        = string
  default     = "default"
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
    the `use` field. `bandwidth_tier` and `enable_public_ips` also do not apply
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
  description = "List of the type and count of accelerator cards attached to the instance. Requires virtual workstation accelerator if Nvidia Grid Drivers are required"
  type = list(object({
    type  = string,
    count = number
  }))
  default = [{
    type  = "nvidia-tesla-t4-vws"
    count = 1
  }]
}

variable "threads_per_core" {
  description = "Sets the number of threads per physical core"
  type        = number
  default     = 2
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE`"
  type        = string
  default     = "TERMINATE"
}

variable "bandwidth_tier" {
  description = <<EOT
  Tier 1 bandwidth increases the maximum egress bandwidth for VMs.
  Using the `tier_1_enabled` setting will enable both gVNIC and TIER_1 higher bandwidth networking.
  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER_1.
  Note that TIER_1 only works with specific machine families & shapes and must be using an image th
at supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-v
m-with-high-bandwidth-configuration) for more details.
  EOT
  type        = string
  default     = "not_enabled"
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

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
}

variable "install_nvidia_driver" {
  description = "Installs the nvidia driver (true/false). For details, see https://cloud.google.com/compute/docs/gpus/install-drivers-gpu"
  type        = bool
}
