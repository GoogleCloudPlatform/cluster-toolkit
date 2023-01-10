/**
 * Copyright 2023 Google LLC
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

variable "install_dir" {
  description = "Destination directory of installation of Spack"
  default     = "/apps/ramble"
  type        = string
}

variable "spack_url" {
  description = "URL for Spack repository to clone"
  default     = "https://github.com/spack/spack"
  type        = string
}

variable "spack_ref" {
  description = "Git ref to checkout for Spack"
  default     = "v0.19.0"
  type        = string
}

variable "log_file" {
  description = "Log file to write output from spack steps into"
  default     = "/var/log/spack.log"
  type        = string
}

variable "chown_owner" {
  description = "Owner to chown the Spack clone to"
  default     = "root"
  type        = string
}

variable "chgrp_group" {
  description = "Group to chgrp the Spack clone to"
  default     = "root"
  type        = string
}

variable "chmod_mode" {
  description = "Mode to chmod the Spack clone to."
  default     = ""
  type        = string
}

variable "commands" {
  description = "Commands to execute within spack"
  default     = []
  type        = list(string)
}

variable "compilers" {
  description = "Defines compilers for spack to install before installing packages."
  default     = []
  type        = list(string)
}


variable "packages" {
  description = "Defines root packages for spack to install (in order)."
  default     = []
  type        = list(string)
}

/* Deprecated Functionality */

variable "spack_cache_url" {
  description = "DEPRECATED"
  type        = any
  default     = null
  nullable    = true
  validation {
    condition     = var.spack_cache_url == null
    error_message = <<EOT
    The spack_cache_url setting is deprecated.
    Please add the cache using the `spack mirror add` and `spack buildcache keys` commands directly.
    For more information, see: https://spack.readthedocs.io/en/latest/binary_caches.html
EOT
  }
}

variable "configs" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.configs == null
    error_message = <<EOT
      The configs setting is deprecated.
      You can replicate its functionality by writing files with data runners,
      and using a command to `config add -f <file>` instead.
EOT
  }
}

variable "licenses" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.licenses == null
    error_message = <<EOT
      The licenses setting is deprecated.
      Please use a data runner to copy your license.
      For more information on configuring spack to use license files,
      see: https://spack.readthedocs.io/en/latest/config_yaml.html#spack-settings-config-yaml
      and for an intel specific example: https://spack.readthedocs.io/en/latest/build_systems/intelpackage.html#configuring-spack-to-use-intel-licenses
EOT
  }
}

variable "install_flags" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.install_flags == null
    error_message = <<EOT
      The install_flags setting is deprecated.
      Instead, set flags on `install` commands directly.
EOT
  }
}

variable "concretize_flags" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.concretize_flags == null
    error_message = <<EOT
      The concretize_flags setting is deprecated.
      Instead, set flags on `concretize` commands directly.
EOT
  }
}

variable "gpg_keys" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.gpg_keys == null
    error_message = <<EOT
      The gpg_keys setting is deprecated.
      Instead, please use a data runner to transfer your key.
      Then, `gpg` commands can be used to add your key directly.
      For example: `spack gpg init && spack gpg trust <path_to_key>`
EOT
  }
}

variable "caches_to_populate" {
  description = "DEPRECATED"
  default     = null
  type        = any
  validation {
    condition     = var.caches_to_populate == null
    error_message = <<EOT
      The caches_to_populate setting is deprecated.
      Please add cache creation commands directly to the commands setting.
EOT
  }
}

variable "environments" {
  description = "DEPRECATED"
  default     = null
  type        = any
  nullable    = true
  validation {
    condition     = var.environments == null
    error_message = <<EOT
      The environments setting is deprecated.
      Please use a data runner to transfer an environment file,
      and use spack commands to create an environment from it directly.
      For more information, see: https://spack.readthedocs.io/en/latest/environments.html
EOT
  }
}
