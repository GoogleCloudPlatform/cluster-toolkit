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

variable "project_id" {
  description = "Project ID associated with the deployment."
  type        = string
}

variable "deployment_name" {
  description = "Deployment name associated with the generated artifacts."
  type        = string
}

variable "region" {
  description = "Region associated with the deployment."
  type        = string
}

variable "app_id" {
  description = "Stable machine-readable identifier used for generated filenames."
  type        = string

  validation {
    condition     = length(regexall("^[A-Za-z0-9][A-Za-z0-9._-]*$", var.app_id)) > 0
    error_message = "app_id must start with an alphanumeric character and contain only letters, numbers, dot, underscore, or dash."
  }
}

variable "display_name" {
  description = "Human-readable name for the generated wrapper, modulefile, and manifest."
  type        = string

  validation {
    condition     = trimspace(var.display_name) != ""
    error_message = "display_name must not be empty."
  }

  validation {
    condition     = !strcontains(var.display_name, "{") && !strcontains(var.display_name, "}")
    error_message = "display_name must not contain '{' or '}'; it is embedded in Tcl-brace-quoted modulefile content."
  }
}

variable "image_ref" {
  description = "Artifact Registry OCI image reference without the docker:// prefix."
  type        = string

  validation {
    condition     = !startswith(var.image_ref, "docker://")
    error_message = "image_ref must not include the docker:// prefix."
  }

  validation {
    condition     = length(regexall("^[a-z0-9-]+-docker\\.pkg\\.dev/.+", var.image_ref)) > 0
    error_message = "image_ref must point to a Docker-format Google Artifact Registry host such as us-central1-docker.pkg.dev/PROJECT/REPO/IMAGE[:TAG|@DIGEST]."
  }
}

variable "install_root" {
  description = "Absolute mounted path where the SIF, wrapper, modulefile, and manifest will be written. If unset, resolve the path from network_storage."
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

  validation {
    condition     = var.install_root == null || (!strcontains(var.install_root, "{") && !strcontains(var.install_root, "}"))
    error_message = "install_root must not contain '{' or '}'; it is embedded in Tcl-brace-quoted modulefile content."
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
    condition     = !startswith(var.sif_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.sif_subdir)) == 0 && !strcontains(var.sif_subdir, "{") && !strcontains(var.sif_subdir, "}")
    error_message = "sif_subdir must be a relative path, must not contain '.' or '..' path segments, and must not contain '{' or '}'."
  }
}

variable "bin_subdir" {
  description = "Relative subdirectory under install_root where wrapper commands are written."
  type        = string
  default     = "bin"

  validation {
    condition     = !startswith(var.bin_subdir, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.bin_subdir)) == 0 && !strcontains(var.bin_subdir, "{") && !strcontains(var.bin_subdir, "}")
    error_message = "bin_subdir must be a relative path, must not contain '.' or '..' path segments, and must not contain '{' or '}'."
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

variable "module_name" {
  description = "Optional modulefile name. Defaults to app_id."
  type        = string
  default     = null

  validation {
    condition     = var.module_name == null || (!startswith(var.module_name, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.module_name)) == 0 && trimspace(var.module_name) != "")
    error_message = "module_name must be null or a non-empty relative path without '.' or '..' path segments."
  }
}

variable "module_version" {
  description = "Optional modulefile version. If set, the modulefile path becomes <module_name>/<module_version>."
  type        = string
  default     = null

  validation {
    condition     = var.module_version == null || (!startswith(var.module_version, "/") && length(regexall("(^|/)\\.\\.?($|/)", var.module_version)) == 0 && trimspace(var.module_version) != "")
    error_message = "module_version must be null or a non-empty relative path segment without '.' or '..'."
  }
}

variable "entrypoint" {
  description = "Optional command to execute inside the container before any caller-provided arguments."
  type        = string
  default     = ""
}

variable "entrypoint_args" {
  description = "Default arguments passed after entrypoint and before user-supplied wrapper arguments."
  type        = list(string)
  default     = []
}

variable "bind_paths" {
  description = "List of bind mounts to pass as --bind arguments to apptainer exec."
  type        = list(string)
  default     = []
}

variable "env" {
  description = "Environment variables exported by the generated wrapper before execution."
  type        = map(string)
  default     = {}

  validation {
    condition = alltrue([
      for key in keys(var.env) : length(regexall("^[A-Za-z_][A-Za-z0-9_]*$", key)) > 0
    ])
    error_message = "All env keys must be valid shell environment variable names."
  }
}

variable "run_args" {
  description = "Default arguments passed directly to apptainer exec before the image path."
  type        = list(string)
  default     = []
}

variable "install_apptainer" {
  description = "If true, generate an install runner that attempts to install Apptainer when it is absent."
  type        = bool
  default     = false
}

variable "apptainer_package" {
  description = "Package name used by the optional install runner."
  type        = string
  default     = "apptainer"
}

variable "pull_policy" {
  description = "Container pull behavior. if_missing skips staging when the target SIF already exists; always refreshes it."
  type        = string
  default     = "if_missing"

  validation {
    condition     = contains(["if_missing", "always"], var.pull_policy)
    error_message = "pull_policy must be one of if_missing or always."
  }
}
