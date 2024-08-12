# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "deployment_name" {
  description = "Cluster Toolkit deployment name"
  type        = string
}

variable "project_id" {
  description = "Project in which to create VM and image"
  type        = string
}

variable "machine_type" {
  description = "VM machine type on which to build new image"
  type        = string
  default     = "n2-standard-4"
}

variable "disk_size" {
  description = "Size of disk image in GB"
  type        = number
  default     = null
}

variable "disk_type" {
  description = "Type of persistent disk to provision"
  type        = string
  default     = "pd-balanced"
}

variable "zone" {
  description = "Cloud zone in which to provision image building VM"
  type        = string
}

variable "network_project_id" {
  description = "Project ID of Shared VPC network"
  type        = string
  default     = null
}

variable "subnetwork_name" {
  description = "Name of subnetwork in which to provision image building VM"
  type        = string
}

variable "omit_external_ip" {
  description = "Provision the image building VM without a public IP address"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Assign network tags to apply firewall rules to VM instance"
  type        = list(string)
  default     = null
}

variable "image_family" {
  description = "The family name of the image to be built. Defaults to `deployment_name`"
  type        = string
  default     = null
}

variable "image_name" {
  description = "The name of the image to be built. If not supplied, it will be set to image_family-$ISO_TIMESTAMP"
  type        = string
  default     = null
}

variable "source_image_project_id" {
  description = <<EOD
A list of project IDs to search for the source image. Packer will search the
first project ID in the list first, and fall back to the next in the list,
until it finds the source image.
EOD
  type        = list(string)
  default     = null
}

variable "source_image" {
  description = "Source OS image to build from"
  type        = string
  default     = null
}

variable "source_image_family" {
  description = "Alternative to source_image. Specify image family to build from latest image in family"
  type        = string
  default     = "hpc-rocky-linux-8"
}

variable "service_account_email" {
  description = "The service account email to use. If null or 'default', then the default Compute Engine service account will be used."
  type        = string
  default     = null
}

variable "scopes" {
  description = "DEPRECATED: use var.service_account_scopes"
  type        = set(string)
  default     = null

  validation {
    condition     = var.scopes == null
    error_message = "DEPRECATED: var.scopes was renamed to var.service_account_scopes with identical format."
  }
}

variable "service_account_scopes" {
  description = <<EOD
Service account scopes to attach to the instance. See
https://cloud.google.com/compute/docs/access/service-accounts.
EOD
  type        = set(string)
  default = [
    "https://www.googleapis.com/auth/cloud-platform",
  ]
}

variable "use_iap" {
  description = "Use IAP proxy when connecting by SSH"
  type        = bool
  default     = true
}

variable "use_os_login" {
  description = "Use OS Login when connecting by SSH"
  type        = bool
  default     = false
}

variable "ssh_username" {
  description = "Username to use for SSH access to VM"
  type        = string
  default     = "hpc-toolkit-packer"
}

variable "ansible_playbooks" {
  description = "A list of Ansible playbook configurations that will be uploaded to customize the VM image"
  type = list(object({
    playbook_file   = string
    galaxy_file     = string
    extra_arguments = list(string)
  }))
  default = []
}

variable "shell_scripts" {
  description = "A list of paths to local shell scripts which will be uploaded to customize the VM image"
  type        = list(string)
  default     = []
}

variable "windows_startup_ps1" {
  description = "A list of strings containing PowerShell scripts which will customize a Windows VM image (requires WinRM communicator)"
  type        = list(string)
  default     = []
}

variable "startup_script" {
  description = "Startup script (as raw string) used to build the custom Linux VM image (overridden by var.startup_script_file if both are set)"
  type        = string
  default     = null
}

variable "startup_script_file" {
  description = "File path to local shell script that will be used to customize the Linux VM image (overrides var.startup_script)"
  type        = string
  default     = null
}

variable "wrap_startup_script" {
  description = "Wrap startup script with Packer-generated wrapper"
  type        = bool
  default     = true
}

variable "labels" {
  description = "Labels to apply to the short-lived VM"
  type        = map(string)
  default     = null
}

variable "accelerator_type" {
  description = "Type of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2)."
  type        = string
  default     = null
}

variable "accelerator_count" {
  description = "Number of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2)."
  type        = number
  default     = null
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except the use of GPUs requires it to be `TERMINATE`"
  type        = string
  default     = null
  validation {
    condition     = var.on_host_maintenance == null ? true : contains(["MIGRATE", "TERMINATE"], var.on_host_maintenance)
    error_message = "When set, the on_host_maintenance must be set to MIGRATE or TERMINATE."
  }
}

# the plugin default is 5m; we have found it is sometimes hit
variable "state_timeout" {
  description = "The time to wait for instance state changes, including image creation"
  type        = string
  default     = "10m"
}

variable "metadata" {
  description = "Instance metadata for the builder VM (use var.startup_script or var.startup_script_file to set startup-script metadata)"
  type        = map(string)
  default     = {}
}

variable "manifest_file" {
  description = "File to which to write Packer build manifest"
  type        = string
  default     = "packer-manifest.json"
}

variable "communicator" {
  description = "Communicator to use for provisioners that require access to VM (\"ssh\" or \"winrm\")"
  type        = string
  default     = null
  validation {
    condition     = var.communicator == null ? true : contains(["ssh", "winrm"], var.communicator)
    error_message = "Set var.communicator to \"ssh\", \"winrm\", or null."
  }
}

variable "image_storage_locations" {
  description = <<EOD
Storage location, either regional or multi-regional, where snapshot content is to be stored and only accepts 1 value.
See https://developer.hashicorp.com/packer/plugins/builders/googlecompute#image_storage_locations
EOD
  type        = list(string)
  default     = null
}

variable "enable_shielded_vm" {
  type        = bool
  default     = false
  description = "Enable the Shielded VM configuration (var.shielded_instance_config)."
}

variable "shielded_instance_config" {
  description = "Shielded VM configuration for the instance (must set var.enabled_shielded_vm)"
  type = object({
    enable_secure_boot          = bool
    enable_vtpm                 = bool
    enable_integrity_monitoring = bool
  })

  default = {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }
}
