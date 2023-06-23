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

# spack-setup variables

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

variable "spack_virtualenv_path" {
  description = "Virtual environment path in which to install Spack Python interpreter and other dependencies"
  default     = "/usr/local/spack-python"
  type        = string
}

# spack-build variables

variable "log_file" {
  description = "Defines the logfile that script output will be written to"
  default     = "/var/log/spack.log"
  type        = string
}

variable "data_files" {
  description = <<-EOT
    A list of files to be transferred prior to running commands. 
    It must specify one of 'source' (absolute local file path) or 'content' (string).
    It must specify a 'destination' with absolute path where file should be placed.
  EOT
  type        = list(map(string))
  default     = []
  validation {
    condition     = alltrue([for r in var.data_files : substr(r["destination"], 0, 1) == "/"])
    error_message = "All destinations must be absolute paths and start with '/'."
  }
  validation {
    condition = alltrue([
      for r in var.data_files :
      can(r["content"]) != can(r["source"])
    ])
    error_message = "A data_file must specify either 'content' or 'source', but never both."
  }
  validation {
    condition = alltrue([
      for r in var.data_files :
      lookup(r, "content", lookup(r, "source", null)) != null
    ])
    error_message = "A data_file must specify a non-null 'content' or 'source'."
  }
}

variable "commands" {
  description = "String of commands to run within this module"
  type        = string
  default     = null
}

variable "deployment_name" {
  description = "Name of deployment, used to name bucket containing startup script."
  type        = string
}

variable "region" {
  description = "Region to place bucket containing startup script."
  type        = string
}

variable "labels" {
  description = "Key-value pairs of labels to be added to created resources."
  type        = map(string)
}

# variables to be deprecated

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
  description = <<-EOT
  DEPRECATED

  The following `commands` can be used to populate a cache:

  ```
  MIRROR_URL=gs://my-bucket
  spack buildcache create --mirror-url $MIRROR_URL -af \$(spack find --format /{hash});
  spack gpg publish --mirror-url $MIRROR_URL;
  spack buildcache update-index --mirror-url $MIRROR_URL --keys;
  ```

  Defines caches which will be populated with the installed packages.

  NOTE: GPG Keys should be installed before trying to populate a cache
  with packages.

  NOTE: The gpg_keys variable can be used to install existing GPG keys
  and create new GPG keys, both of which are acceptable for populating a
  cache.
EOT
  default     = null
  type        = list(map(any))
  validation {
    condition     = var.caches_to_populate == null
    error_message = "caches_to_populate is deprecated. Use commands instead. See variable documentation for proposed alternative commands."
  }
}

variable "environments" {
  description = <<-EOT
  DEPRECATED

  The following `commands` can be used to configure an environment:

  ```
  if ! spack env list | grep -q my-env; then
    spack env create my-env
  fi
  spack env activate my-env
  spack add intel-mpi@2018.4.274 %gcc@10.3.0
  spack concretize
  ```
  
  Defines spack environments to configure.
  For more information, see: https://spack.readthedocs.io/en/latest/environments.html.

EOT
  default     = null
  type        = any
  validation {
    condition     = var.environments == null
    error_message = "environments is deprecated. Use commands instead. See variable documentation for proposed alternative commands."
  }
}
