# Copyright 2021 Google LLC
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
  description = "HPC Toolkit deployment name"
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
  default     = null
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
  description = "The family name of the image to be built. Image name will also be derived from this value. Defaults to `deployment_name`"
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
  default = [
    "cloud-hpc-image-public"
  ]
}

variable "source_image" {
  description = "Source OS image to build from"
  type        = string
  default     = null
}

variable "source_image_family" {
  description = "Alternative to source_image. Specify image family to build from latest image in family"
  type        = string
  default     = "hpc-centos-7"
}

variable "service_account_email" {
  description = "The service account email to use. If null or 'default', then the default Compute Engine service account will be used."
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = <<EOD
Service account scopes to attach to the instance. See
https://cloud.google.com/compute/docs/access/service-accounts.
EOD
  type        = list(string)
  default     = null
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
  default     = "packer"
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

variable "startup_script" {
  description = "Startup script (as raw string) used to build the custom VM image (overridden by var.startup_script_file if both are supplied)"
  type        = string
  default     = null
}

variable "startup_script_file" {
  description = "Path to local shell script that will be uploaded as a startup script to customize the VM image"
  type        = string
  default     = null
}

variable "wrap_startup_script" {
  description = "Wrap startup script with Packer-generated wrapper"
  type        = bool
  default     = true
}
