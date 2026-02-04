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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the NFS instance if no name is specified."
  type        = string
}

variable "name" {
  description = "The resource name of the instance."
  type        = string
  default     = null
}

variable "zone" {
  description = "The zone name where the NFS instance located in."
  type        = string
}

variable "boot_disk_size" {
  description = "Storage size in GB for the boot disk"
  type        = number
  default     = null
}

variable "boot_disk_type" {
  description = "Storage type for the boot disk"
  type        = string
  default     = null
}

variable "create_boot_snapshot_before_destroy" {
  description = "Whether to create a snapshot before destroying the boot disk"
  type        = bool
  default     = false
}

variable "disk_size" {
  description = "Storage size in GB for the NFS data disk"
  type        = number
  default     = "100"
}

variable "type" {
  description = "Storage type for the NFS data disk"
  type        = string
  default     = "pd-ssd"
}

variable "create_snapshot_before_destroy" {
  description = "Whether to create a snapshot before destroying the NFS data disk"
  type        = bool
  default     = false
}

variable "provisioned_iops" {
  description = "Provisioned IOPS for the NFS data disk if using Extreme PD or Hyperdisk Balanced/ML/Throughput"
  type        = number
  default     = null
}

variable "provisioned_throughput" {
  description = "Provisioned throughput for the NFS data disk if using Hyperdisk Balanced/Extreme"
  type        = number
  default     = null
}

# Deprecated, replaced by instance_image
# tflint-ignore: terraform_unused_declarations
variable "image" {
  description = "DEPRECATED: The VM image used by the NFS server"
  type        = string
  default     = null

  validation {
    condition     = var.image == null
    error_message = "The 'var.image' setting is deprecated, please use 'var.instance_image' with the fields 'project' and 'family' or 'name'."
  }
}

variable "instance_image" {
  description = <<-EOD
    The VM image used by the NFS server.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "cloud-hpc-image-public"
    family  = "hpc-rocky-linux-8"
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

# Deprecated, replaced by create_snapshot_before_destroy and create_boot_snapshot_before_destroy
# tflint-ignore: terraform_unused_declarations
variable "auto_delete_disk" {
  description = "DEPRECATED: Whether or not the NFS disk should be auto-deleted"
  type        = string
  default     = null

  validation {
    condition     = var.auto_delete_disk == null
    error_message = "The 'var.auto_delete_disk' setting is broken in Cluster Toolkit versions >1.25.0 and deprecated in versions >1.48.0, please use 'var.create_snapshot_before_destroy' and 'var.create_boot_snapshot_before_destroy' instead."
  }
}

variable "network_self_link" {
  description = "The self link of the network to attach the NFS VM."
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to attach the NFS VM."
  type        = string
  default     = null
}

variable "machine_type" {
  description = "Type of the VM instance to use"
  type        = string
  default     = "n2d-standard-2"
}

variable "labels" {
  description = "Labels to add to the NFS instance. Key-value pairs."
  type        = map(string)
}

variable "metadata" {
  description = "Metadata, provided as a map"
  type        = map(string)
  default     = {}
}

variable "service_account" {
  description = "Service Account for the NFS server"
  type        = string
  default     = null
}

variable "scopes" {
  description = "Scopes to apply to the controller"
  type        = list(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
}

variable "local_mounts" {
  description = "Mountpoint for this NFS compute instance"
  type        = list(string)
  default     = ["/data"]

  validation {
    condition = alltrue([
      for m in var.local_mounts : substr(m, 0, 1) == "/"
    ])
    error_message = "Local mountpoints have to start with '/'."
  }
  validation {
    condition     = length(var.local_mounts) > 0
    error_message = "At least one local mount must be specified in var.local_mounts."
  }
}
