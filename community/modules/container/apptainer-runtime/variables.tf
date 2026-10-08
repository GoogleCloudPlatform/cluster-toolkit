# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "install_root" {
  description = "Absolute mounted path where shared Apptainer assets will be written. If unset, resolve the path from network_storage."
  type        = string
  default     = null

  validation {
    condition     = var.install_root == null || startswith(var.install_root, "/")
    error_message = "install_root must be null or an absolute path."
  }

  validation {
    condition = var.install_root != null || (
      can(var.network_storage[var.network_storage_index].local_mount) &&
      startswith(var.network_storage[var.network_storage_index].local_mount, "/")
    )
    error_message = "Set install_root to an absolute path or supply network_storage with a selectable absolute local_mount."
  }
}

variable "network_storage" {
  description = <<-EOT
  Optional list of network storage mounts, following the standard Cluster
  Toolkit network_storage contract (e.g. from a storage module used via
  `use:`). When install_root is unset, the module resolves install_root from
  the selected entry's local_mount.
  EOT
  type = list(object({
    server_ip               = string
    remote_mount            = string
    local_mount             = string
    local_mount_owner       = optional(string)
    local_mount_permissions = optional(string)
    fs_type                 = string
    mount_options           = string
    client_install_runner   = optional(map(string))
    mount_runner            = optional(map(string))
  }))
  default = []
}

variable "network_storage_index" {
  description = "Index to select when network_storage is provided as a list and install_root is unset."
  type        = number
  default     = 0

  validation {
    condition     = var.network_storage_index >= 0 && floor(var.network_storage_index) == var.network_storage_index
    error_message = "network_storage_index must be a non-negative integer."
  }
}

variable "sif_subdir" {
  description = "Relative subdirectory under install_root where SIF images are staged."
  type        = string
  default     = "containers"

  validation {
    condition     = !startswith(var.sif_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.sif_subdir)) == 0
    error_message = "sif_subdir must be a relative path and must not contain '.' or '..' path segments."
  }
}

variable "bin_subdir" {
  description = "Relative subdirectory under install_root where wrapper commands are written."
  type        = string
  default     = "bin"

  validation {
    condition     = !startswith(var.bin_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.bin_subdir)) == 0
    error_message = "bin_subdir must be a relative path and must not contain '.' or '..' path segments."
  }
}

variable "modulefile_subdir" {
  description = "Relative subdirectory under install_root where Tcl modulefiles are written."
  type        = string
  default     = "modulefiles"

  validation {
    condition     = !startswith(var.modulefile_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.modulefile_subdir)) == 0
    error_message = "modulefile_subdir must be a relative path and must not contain '.' or '..' path segments."
  }
}

variable "manifest_subdir" {
  description = "Relative subdirectory under install_root where generated app manifests are written."
  type        = string
  default     = "app-manifests"

  validation {
    condition     = !startswith(var.manifest_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.manifest_subdir)) == 0
    error_message = "manifest_subdir must be a relative path and must not contain '.' or '..' path segments."
  }
}

variable "module_init_path" {
  description = "Absolute path of the shell profile snippet that should add the shared modulefile tree to MODULEPATH."
  type        = string
  default     = "/etc/profile.d/shared-modules.sh"

  validation {
    condition     = startswith(var.module_init_path, "/")
    error_message = "module_init_path must be an absolute path."
  }
}
