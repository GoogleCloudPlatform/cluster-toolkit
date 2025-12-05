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

variable "zone" {
  description = "Zone in which the VDI instances are created."
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

variable "vdi_resolution_locked" {
  type        = bool
  description = "Disable resize of remote display in Guacamole connections. When true, VDI displays at native resolution without browser scaling."
  default     = true
}

variable "vdi_webapp_port" {
  type        = string
  description = "Port to serve the Webapp interface from if applicable (note: containers will be recreated if changed)"
  default     = "8080"
}

variable "vdi_users" {
  description = "List of VDI users to configure. Passwords are handled securely by the Ansible roles: if secret_name is provided, the password is fetched from Secret Manager; if neither password nor secret_name is provided, a random password is generated and stored in Secret Manager. If secret_project is provided, it specifies the GCP project where the secret is stored (defaults to the deployment project). Set reset_password to true to trigger password regeneration for auto-generated passwords."
  type = list(object({
    username       = string
    port           = number
    secret_name    = optional(string)
    secret_project = optional(string)
    reset_password = optional(bool)
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

variable "debug" {
  type        = bool
  default     = false
  description = "Enable debug mode for verbose logging during VDI setup."
}

variable "reset_webapp_admin_password" {
  type        = bool
  default     = false
  description = "Force reset of the webapp admin password during reconfiguration. If true, a new password will be generated and stored in Secret Manager, even if an existing password exists."
}

variable "force_rerun" {
  type        = bool
  default     = false
  description = "Force complete container recreation and database re-initialization, bypassing all idempotency checks. Use only when troubleshooting or when the system is in a broken state."
}
