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

variable "rpm_url" {
  description = "Path to PBS Pro RPM file for select PBS host type (server, client, execution)"
  type        = string
}

variable "pbs_exec" {
  description = "Root path in which to install PBS"
  type        = string
  default     = "/opt/pbs"
}

variable "pbs_data_service_user" {
  description = "PBS Data Service POSIX user"
  type        = string
  default     = "pbsdata"
}

variable "pbs_home" {
  description = "PBS working directory"
  type        = string
  default     = "/var/spool/pbs"
}

variable "pbs_license_server" {
  description = "IP address or DNS name of PBS license server (required only for PBS server hosts)"
  type        = string
  default     = "CHANGE_THIS_TO_PBS_PRO_LICENSE_SERVER_HOSTNAME"
}

variable "pbs_license_server_port" {
  description = "Networking port of PBS license server"
  type        = number
  default     = 6200
}

variable "pbs_server" {
  description = "IP address or DNS name of PBS server host (required only for PBS client and execution hosts)"
  type        = string
  default     = "CHANGE_THIS_TO_PBS_PRO_SERVER_HOSTNAME"
}

variable "pbs_role" {
  description = "Type of PBS host to provision: server, client, execution"
  type        = string
  validation {
    condition     = contains(["server", "client", "execution"], var.pbs_role)
    error_message = "Value for var.pbs_role must be one of \"server\", \"client\", or \"execution\"."
  }
}
