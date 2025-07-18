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

output "startup_script" {
  description = "Combined startup script that installs VDI (VNC, Guacamole, users)."
  value       = module.startup_script.startup_script
}

output "vdi_runner" {
  description = "Shell runner wrapping Ansible playbook + roles (for custom-image or direct use)."
  value       = local.combined_runner
}

output "guacamole_admin_username" {
  description = "The admin username for Guacamole"
  value       = "guacadmin"
}

output "guacamole_admin_password_secret" {
  description = "The name of the Secret Manager secret containing the Guacamole admin password"
  value       = "webapp-server-password-${var.deployment_name}"
}

output "vdi_user_credentials" {
  description = "Map of VDI user credentials stored in Secret Manager"
  value = {
    for user in var.vdi_users : user.username => {
      username    = user.username
      port        = user.port
      secret_name = user.secret_name != null ? user.secret_name : "vdi-user-password-${user.username}-${var.deployment_name}"
    }
  }
}
