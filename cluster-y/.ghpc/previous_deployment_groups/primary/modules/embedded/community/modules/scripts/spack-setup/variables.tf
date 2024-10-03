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

variable "spack_profile_script_path" {
  description = "Path to the Spack profile.d script. Created by this module"
  type        = string
  default     = "/etc/profile.d/spack.sh"
}
