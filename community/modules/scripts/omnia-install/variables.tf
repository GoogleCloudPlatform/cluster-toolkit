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

variable "manager_ips" {
  description = "IPs of the Omnia manager nodes"
  type        = list(string)
}

variable "compute_ips" {
  description = "IPs of the Omnia compute nodes"
  type        = list(string)
}

variable "install_dir" {
  description = <<EOT
Path where omnia will be installed, defaults to omnia user home directory (/home/omnia).
If specifying this path, please make sure it is on a shared file system, accessible by all omnia nodes.
EOT
  default     = ""
  type        = string
}

variable "omnia_username" {
  description = "Name of the user that installs omnia"
  default     = "omnia"
  type        = string
}

variable "slurm_uid" {
  description = "User ID of the slurm user"
  default     = 981
  type        = number
}
