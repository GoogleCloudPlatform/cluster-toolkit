# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "deployment_name" {
  description = "The name of the deployment."
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

variable "vnc_flavor" {
  description = "The VNC server flavor to use (tigervnc currently supported)"
  type        = string
  default     = "tigervnc"
}

variable "vdi_tool" {
  type        = string
  description = "VDI tool to deploy (guacamole currently supported)."
  default     = "guacamole"
}

variable "user_provision" {
  type        = string
  description = "User type to create (local_users supported. os-login to do."
  default     = "local_users"
}

variable "vdi_user_group" {
  type        = string
  description = "Unix group to create/use for VDI users."
  default     = "vdiusers"
}

variable "vdi_resolution" {
  type        = string
  description = "Desktop resolution for VNC sessions (e.g. 1920x1080)."
  default     = "1920x1080"
}

variable "vdi_webapp_port" {
  type        = string
  description = "Port to serve the Webapp interface from if applicable"
  default     = "8080"
}

variable "vdi_users" {
  description = "List of VDI users to configure. Passwords are handled securely by the Ansible roles: if secret_name is provided, the password is fetched from Secret Manager; if neither password nor secret_name is provided, a random password is generated and stored in Secret Manager. If secret_project is provided, it specifies the GCP project where the secret is stored (defaults to the deployment project)."
  type = list(object({
    username       = string
    port           = number
    secret_name    = optional(string)
    secret_project = optional(string)
  }))
  default = []
}

variable "vnc_port_min" {
  type        = number
  default     = 5901
  description = "Minimum valid VNC port."
}
variable "vnc_port_max" {
  type        = number
  default     = 5999
  description = "Maximum valid VNC port."
}

variable "vdi_instance_ip" {
  description = "The IP address of the VDI instance"
  type        = string
  default     = null
}

variable "vdi_instance_name" {
  description = "The name of the VDI instance"
  type        = string
  default     = null
}
