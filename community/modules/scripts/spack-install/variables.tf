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
  default     = "/sw/spack"
}

variable "spack_url" {
  description = "URL to clone the spack repo from."
  type        = string
  default     = "https://github.com/spack/spack"
}

variable "spack_ref" {
  description = "Git ref to checkout for spack."
  type        = string
  default     = "v0.20.0"
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
    All configs must specify content, and a
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
      for c in var.configs : contains(keys(c), "content")
    ])
    error_message = "All configs must declare a content."
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

variable "install_flags" {
  description = "Defines the flags to pass into `spack install`"
  default     = ""
  type        = string
}

variable "concretize_flags" {
  description = "Defines the flags to pass into `spack concretize`"
  default     = ""
  type        = string
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
  description = <<EOT
  Defines spack environments to configure, given as a list.
  Each environment must define a name.
  Additional optional attributes are 'content' and 'packages'.
  'content' must be a string, defining the content of the Spack Environment YAML file.
  'packages' must be a list of strings, defining the spack specs to install.
  If both 'content' and 'packages' are defined, 'content' is processed first.

EOT
  default     = []
  type        = any
  validation {
    condition = alltrue([
      for e in var.environments : (contains(keys(e), "name"))
    ])
    error_message = "All environments must have a name."
  }

  validation {
    condition = alltrue([
      for e in var.environments : (contains(keys(e), "packages") ? alltrue(([for p in e["packages"] : alltrue([tostring(p) == p])])) : true)
    ])
    error_message = "The packages attribute within environments is required to be a list of strings."
  }

  validation {
    condition = alltrue([
      for e in var.environments : (contains(keys(e), "content") ? tostring(e["content"]) == e["content"] : true)
    ])
    error_message = "The content attribute within environments is required to be a string."
  }
}

variable "log_file" {
  description = "Defines the logfile that script output will be written to"
  default     = "/var/log/spack.log"
  type        = string
}

variable "spack_virtualenv_path" {
  description = "Virtual environment path in which to install Spack Python interpreter and other dependencies"
  default     = "/usr/local/spack-python"
  type        = string
}
