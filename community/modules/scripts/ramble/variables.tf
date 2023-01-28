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
  description = "Destination directory of installation of Ramble"
  default     = "/apps/ramble"
  type        = string
}

variable "ramble_url" {
  description = "URL for Ramble repository to clone"
  default     = "https://github.com/GoogleCloudPlatform/ramble"
  type        = string
}

variable "ramble_ref" {
  description = "Git ref to checkout for Ramble"
  default     = "develop"
  type        = string
}

variable "log_file" {
  description = "Log file to write output from ramble steps into"
  default     = "/var/log/ramble.log"
  type        = string
}

variable "chown_owner" {
  description = "Owner to chown the Ramble clone to"
  default     = "root"
  type        = string
}

variable "chgrp_group" {
  description = "Group to chgrp the Ramble clone to"
  default     = "root"
  type        = string
}

variable "chmod_mode" {
  description = "Mode to chmod the Ramble clone to."
  default     = ""
  type        = string
}

variable "spack_path" {
  description = "Path to the spack installation"
  default     = ""
  type        = string
}

variable "commands" {
  description = "Commands to execute within ramble"
  default     = []
  type        = list(string)
}
