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
  description = "The GCP zone where the instance is running"
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "install_dir" {
  description = "Directory to install spack into"
  type        = string
  default     = "/apps/spack"
}

variable "spack_url" {
  description = "URL to clone the spack repo from"
  type        = string
  default     = "https://github.com/spack/spack"
}

variable "spack_ref" {
  description = "Git ref to checkout for spack"
  type        = string
  default     = "develop"
}

variable "spack_cache_url" {
  description = "List of buildcaches for spack"
  type = list(object({
    mirror_name = string
    mirror_url  = string
  }))
  default = null
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
  description = "Defines packages for spack to install (in order)"
  default     = []
  type        = list(string)
}

