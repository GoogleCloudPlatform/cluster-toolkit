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

variable "ramble_path" {
  description = "Path to the ramble installation"
  default     = ""
  type        = string
}

variable "log_file" {
  description = "Log file to write output from ramble execute steps into"
  default     = "/var/log/ramble-execute.log"
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

variable "ramble_runner" {
  description = "Ansible based startup-script runner from a previous ramble step"
  default     = null
  type = object({
    type        = string
    content     = string
    destination = string
  })
}
