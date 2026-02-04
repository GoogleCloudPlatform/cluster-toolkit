/**
 * Copyright 2026 Google LLC
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
  description = "ID of project in which the notebook will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment; used as part of name of the notebook."
  type        = string
  # notebook name can have: lowercase letters, numbers, or hyphens (-) and cannot end with a hyphen
  validation {
    error_message = "The notebook name uses 'deployment_name' -- can only have: lowercase letters, numbers, or hyphens"
    condition     = can(regex("^[a-z0-9]+(?:-[a-z0-9]+)*$", var.deployment_name))
  }
}

variable "zone" {
  description = "The zone to deploy to"
  type        = string
}

variable "machine_type" {
  description = "The machine type to employ"
  type        = string
}

variable "labels" {
  description = "Labels to add to the resource Key-value pairs."
  type        = map(string)
}

variable "instance_image" {
  description = "Instance Image"
  type        = map(string)
  default = {
    project = "deeplearning-platform-release"
    family  = "tf-latest-cpu"
    name    = null
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

variable "gcs_bucket_path" {
  description = "Bucket name, can be provided from the google-cloud-storage module"
  type        = string
  default     = null
}

variable "mount_runner" {
  description = "mount content from the google-cloud-storage module"
  type        = map(string)

  validation {
    condition     = (length(split(" ", var.mount_runner.args)) == 5)
    error_message = "There must be 5 elements in the Mount Runner Arguments: ${var.mount_runner.args} \n "
  }
}

variable "service_account_email" {
  description = "If defined, the instance will use the service account specified instead of the Default Compute Engine Service Account"
  type        = string
  default     = null
}

variable "network_interfaces" {
  type = list(object({
    network  = optional(string)
    subnet   = optional(string)
    nic_type = optional(string)
    access_configs = optional(list(object({
      external_ip = optional(string)
    })))
  }))
  default     = []
  description = <<EOT
A list of network interfaces for the VM instance. Each network interface is represented by an object with the following fields:

- network: (Optional) The name of the Virtual Private Cloud (VPC) network that this VM instance is connected to.

- subnet: (Optional) The name of the subnetwork within the specified VPC that this VM instance is connected to.

- nic_type: (Optional) The type of vNIC to be used on this interface. Possible values are: `VIRTIO_NET`, `GVNIC`.

- access_configs: (Optional) An array of access configurations for this network interface. The access_config object contains:
  * external_ip: (Required) An external IP address associated with this instance. Specify an unused static external IP address available to the project or leave this field undefined to use an IP from a shared ephemeral IP address pool. If you specify a static external IP address, it must live in the same region as the zone of the instance.
EOT
}
