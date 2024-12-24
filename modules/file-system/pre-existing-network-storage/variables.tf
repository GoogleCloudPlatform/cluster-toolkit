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

variable "server_ip" {
  description = "The device name as supplied to fs-tab, excluding remote fs-name(for nfs, that is the server IP, for lustre <MGS NID>[:<MGS NID>]). This can be omitted for gcsfuse."
  type        = string
  default     = ""
}

variable "remote_mount" {
  description = "Remote FS name or export. This is the exported directory for nfs, fs name for lustre, and bucket name (without gs://) for gcsfuse."
  type        = string
}

variable "local_mount" {
  description = "The mount point where the contents of the device may be accessed after mounting."
  type        = string
  default     = "/mnt"
}

variable "fs_type" {
  description = "Type of file system to be mounted (e.g., nfs, lustre)"
  type        = string
  default     = "nfs"
}

variable "mount_options" {
  description = "Options describing various aspects of the file system. Consider adding setting to 'defaults,_netdev,implicit_dirs' when using gcsfuse."
  type        = string
  default     = "defaults,_netdev"
  nullable    = false
}

variable "parallelstore_options" {
  description = "Parallelstore specific options"
  type = object({
    daos_agent_config = optional(string, "")
    dfuse_environment = optional(map(string), {})
  })
  default = {}
}
