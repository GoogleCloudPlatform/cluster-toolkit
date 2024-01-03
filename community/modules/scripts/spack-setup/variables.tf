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

variable "configure_for_google" {
  description = "When true, the spack installation will be configured to pull from Google's Spack binary cache."
  type        = bool
  default     = true
}

# tflint-ignore: terraform_unused_declarations
variable "chown_owner" {
  description = "Deprecated: use `system_user_name`."
  default     = null
  type        = string

  validation {
    condition     = var.chown_owner == null
    error_message = "chown_owner is deprecated. Use system_user_name to set the owner of the installation."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "chgrp_group" {
  description = "Deprecated: installation will be owned by group of `system_user_name`. If special group is needed, supply user with group assigned."
  default     = null
  type        = string

  validation {
    condition     = var.chgrp_group == null
    error_message = "chgrp_group is deprecated. Use system_user_name to set owning user and group."
  }
}

variable "chmod_mode" {
  description = <<-EOT
    `chmod` to apply to the Spack installation. Adds group write by default. Set to `""` (empty string) to prevent modification.
    For usage information see:
    https://docs.ansible.com/ansible/latest/collections/ansible/builtin/file_module.html#parameter-mode
    EOT
  default     = "g+w"
  type        = string
  nullable    = false
}

variable "system_user_name" {
  description = "Name of system user that will perform installation of Spack. It will be created if it does not exist."
  default     = "spack"
  type        = string
  nullable    = false
}

variable "system_user_uid" {
  description = "UID used when creating system user. Ignored if `system_user_name` already exists on system. Default of 1104762903 is arbitrary."
  default     = 1104762903
  type        = number
  nullable    = false
}

variable "system_user_gid" {
  description = "GID used when creating system user group. Ignored if `system_user_name` already exists on system. Default of 1104762903 is arbitrary."
  default     = 1104762903
  type        = number
  nullable    = false
}

variable "spack_virtualenv_path" {
  description = "Virtual environment path in which to install Spack Python interpreter and other dependencies"
  default     = "/usr/local/spack-python"
  type        = string
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

# tflint-ignore: terraform_unused_declarations
variable "log_file" {
  description = <<-EOT
  DEPRECATED 
  
  All install logs are printed to stdout/stderr.
  Execution log_file location can be set on spack-execute module.
  EOT
  default     = null
  type        = string
  validation {
    condition     = var.log_file == null
    error_message = "log_file is deprecated. See spack-execute module for similar functionality."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "spack_cache_url" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to add a build cache:

  ```
  spack mirror add --scope site <mirror name> gs://my-build-cache
  spack buildcache keys --install --trust
  ```

  List of build caches for Spack.
  EOT
  type = list(object({
    mirror_name = string
    mirror_url  = string
  }))
  default = null
  validation {
    condition     = var.spack_cache_url == null
    error_message = "spack_cache_url is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "configs" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to add a single config:

  ```
  spack config --scope defaults add config:default:true
  ```

  Alternatively, use `data_files` to transfer a config file and use the `spack config add -f <file>` command to add the config.

  List of configuration options to set within spack.
  EOT
  default     = null
  type        = list(map(any))
  validation {
    condition     = var.configs == null
    error_message = "configs is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "compilers" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to install compilers:

  ```
  spack install gcc@10.3.0 target=x86_64
  spack load gcc@10.3.0 target=x86_64
  spack compiler find --scope site
  spack clean -s
  spack unload gcc@10.3.0
  ```

  Defines compilers for spack to install before installing packages.
  EOT
  type        = list(string)
  default     = null
  validation {
    condition     = var.compilers == null
    error_message = "compilers is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "licenses" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with `data_files` variable to install license files:

  ```
  data_files = [{
    source = "/abs/path/on/deployment/machine/license.lic"
    destination = "/sw/spack/etc/spack/licenses/license.lic"
  }]
  ```

  List of software licenses to install within spack.
  EOT

  default = null
  type = list(object({
    source = string
    dest   = string
  }))
  validation {
    condition     = var.licenses == null
    error_message = "licenses is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "packages" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to install a package:

  ```
  spack install intel-mpi@2018.4.274 %gcc@10.3.0
  ```

  Defines root packages for spack to install.
  EOT
  type        = list(string)
  default     = null
  validation {
    condition     = var.packages == null
    error_message = "packages is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "install_flags" {
  description = "DEPRECATED - spack install is now performed using the [spack-execute](../spack-execute/) module `commands` variable."
  default     = null
  type        = string
  validation {
    condition     = var.install_flags == null
    error_message = "install_flags is deprecated. Add install flags to the relevant line in commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "concretize_flags" {
  description = "DEPRECATED - spack concretize is now performed using the [spack-execute](../spack-execute/) module `commands` variable."
  default     = null
  type        = string
  validation {
    condition     = var.concretize_flags == null
    error_message = "concretize_flags is deprecated. Add concretize flags to the relevant line in commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "gpg_keys" {
  description = <<EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to create a new GPG key:

  ```
  spack gpg init
  spack gpg create <name> <email>
  ```

  Alternatively, `data_files` can be used to transfer an existing GPG key. Then use `spack gpg trust <file>` to add the key to the keyring.

  GPG Keys to trust within spack.
EOT
  default     = null
  type        = list(map(any))
  validation {
    condition     = var.gpg_keys == null
    error_message = "gpg_keys is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "caches_to_populate" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to populate a cache:

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
    error_message = "caches_to_populate is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "environments" {
  description = <<-EOT
  DEPRECATED

  Use [spack-execute](../spack-execute/) module with the following `commands` can be used to configure an environment:

  ```
  if ! spack env list \| grep -q my-env; then
    spack env create my-env
  fi
  spack env activate my-env
  spack add intel-mpi@2018.4.274 %gcc@10.3.0
  spack concretize
  spack install
  ```
  
  Defines spack environments to configure.
  For more information, see: https://spack.readthedocs.io/en/latest/environments.html.

EOT
  default     = null
  type        = any
  validation {
    condition     = var.environments == null
    error_message = "environments is deprecated. Use spack-execute.commands instead. See variable documentation for proposed alternative commands."
  }
}

variable "spack_profile_script_path" {
  description = "Path to the Spack profile.d script. Created by this module"
  type        = string
  default     = "/etc/profile.d/spack.sh"
}
