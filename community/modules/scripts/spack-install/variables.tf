/**
 * Copyright 2021 Google LLC
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

variable "zone" {
  description = "The GCP zone where the instance is running."
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "install_dir" {
  description = "Directory to install spack into."
  type        = string
  default     = "/apps/spack"
}

variable "spack_url" {
  description = "URL to clone the spack repo from."
  type        = string
  default     = "https://github.com/spack/spack"
}

variable "spack_ref" {
  description = "Git ref to checkout for spack."
  type        = string
  default     = "develop"
}

variable "spack_cache_url" {
  description = "List of buildcaches for spack."
  type = list(object({
    mirror_name = string
    mirror_url  = string
  }))
  default = null
}

variable "configs" {
  description = <<EOT
    List of configuration options to set within spack.
    Configs can be of type 'single-config' or 'file'.
    All configs must specify a value, and a
    a scope.
EOT
  default     = []
  type        = list(map(any))
  validation {
    condition = alltrue([
      for c in var.configs : contains(keys(c), "type")
    ])
    error_message = "All configs must declare a type."
  }
  validation {
    condition = alltrue([
      for c in var.configs : contains(keys(c), "scope")
    ])
    error_message = "All configs must declare a scope."
  }
  validation {
    condition = alltrue([
      for c in var.configs : contains(keys(c), "value")
    ])
    error_message = "All configs must declare a value."
  }
  validation {
    condition = alltrue([
      for c in var.configs : (c["type"] == "single-config" || c["type"] == "file")
    ])
    error_message = "The 'type' must be 'single-config' or 'file'."
  }
}

variable "compilers" {
  description = "Defines compilers for spack to install before installing packages."
  default     = []
  type        = list(string)
}

variable "licenses" {
  description = "List of software licenses to install within spack."
  default     = null
  type = list(object({
    source = string
    dest   = string
  }))
}

variable "packages" {
  description = "Defines root packages for spack to install (in order)."
  default     = []
  type        = list(string)
}

variable "gpg_keys" {
  description = <<EOT
  GPG Keys to trust within spack.
  Each key must define a type. Valid types are 'file' and 'new'.
  Keys of type 'file' must define a path to the key that
  should be trusted.
  Keys of type 'new' must define a 'name' and 'email' to create
  the key with.
EOT
  default     = []
  type        = list(map(any))
  validation {
    condition = alltrue([
      for k in var.gpg_keys : contains(keys(k), "type")
    ])
    error_message = "Each gpg_key must define a type."
  }
  validation {
    condition = alltrue([
      for k in var.gpg_keys : (k["type"] == "file" || k["type"] == "new")
    ])
    error_message = "Valid types for gpg_keys are 'file' and 'new'."
  }
  validation {
    condition = alltrue([
      for k in var.gpg_keys : ((k["type"] == "file" && contains(keys(k), "path")) || (k["type"] == "new"))
    ])
    error_message = "Each gpg_key of type file must define a path."
  }
  validation {
    condition = alltrue([
      for k in var.gpg_keys : (k["type"] == "file" || ((k["type"] == "new") && contains(keys(k), "name") && contains(keys(k), "email")))
    ])
    error_message = "Each gpg_key of type new must define a name and email."
  }
}

variable "caches_to_populate" {
  description = <<EOT
  Defines caches which will be populated with the installed packages.
  Each cache must specify a type (either directory, or mirror).
  Each cache must also specify a path. For directory caches, this path
  must be on a local file system (i.e. file:///path/to/cache). For
  mirror paths, this can be any valid URL that spack accepts.

  NOTE: GPG Keys should be installed before trying to populate a cache
  with packages.

  NOTE: The gpg_keys variable can be used to install existing GPG keys
  and create new GPG keys, both of which are acceptable for populating a
  cache.
EOT
  default     = []
  type        = list(map(any))
  validation {
    condition = alltrue([
      for c in var.caches_to_populate : (contains(keys(c), "type") && contains(keys(c), "path"))
    ])
    error_message = "Each cache_to_populate must have define both 'type' and 'path'."
  }
  validation {
    condition = alltrue([
      for c in var.caches_to_populate : (c["type"] == "directory" || c["type"] == "mirror")
    ])

    error_message = "Cache_to_populate type must be either 'directory' or 'mirror'."
  }
}

variable "environments" {
  description = "Defines a spack environment to configure."
  default     = null
  type = list(object({
    name     = string
    packages = list(string)
  }))
}

variable "log_file" {
  description = "Defines the logfile that script output will be written to"
  default     = "/dev/null"
  type        = string
}
