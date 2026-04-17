/**
 * Copyright 2025 Google LLC
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

variable "local_mount" {
  description = "The mount point where the contents of the device may be accessed after mounting."
  type        = string
  default     = "/mnt"
}

variable "mount_options" {
  description = "Mount options for filesystem shared by all clients."
  type        = string
  default     = ""
  nullable    = false
}

variable "remote_mount" {
  description = "Weka filesystem name."
  type        = string
}

variable "server_ip" {
  description = "Weka backend IP address used for bootstrapping."
  type        = string
  default     = ""
}
