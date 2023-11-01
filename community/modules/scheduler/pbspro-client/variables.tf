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
}

variable "region" {
  description = "Default region for creating resources"
  type        = string
}

variable "zone" {
  description = "Default zone for creating resources"
  type        = string
}

variable "pbs_exec" {
  description = "Root path in which to install PBS"
  type        = string
  default     = "/opt/pbs"
}

variable "pbs_home" {
  description = "PBS working directory"
  type        = string
  default     = "/var/spool/pbs"
}

variable "pbs_server" {
  description = "IP address or DNS name of PBS server host"
  type        = string
}

variable "pbs_client_rpm_url" {
  description = "Path to PBS Pro Client Host RPM file"
  type        = string
}

variable "name_prefix" {
  description = "Name prefix for PBS execution hostnames"
  type        = string
  default     = null
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
    Instance Image

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
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

variable "enable_public_ips" {
  description = "If set to true, instances will have public IPs on the internet."
  type        = bool
  default     = true
}

variable "machine_type" {
  description = "Machine type to use for the instance creation"
  type        = string
  default     = "c2-standard-60"
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
    email = null
    scopes = [
      "https://www.googleapis.com/auth/devstorage.read_write",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring.write",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/trace.append",
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
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default = null
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE`"
  type        = string
  default     = null
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

variable "placement_policy" {
  description = "Control where your VM instances are physically located relative to each other within a zone."
  type = object({
    vm_count                  = number,
    availability_domain_count = number,
    collocation               = string,
  })
  default = null
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
}

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
}
