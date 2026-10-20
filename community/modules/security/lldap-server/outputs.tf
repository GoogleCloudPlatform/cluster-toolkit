/**
 * Copyright 2026 Google LLC
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

output "runners" {
  description = "A list with one shell runner that configures an LDAP client (SSSD). Use it with the startup-script module."
  value = [{
    type        = "shell"
    destination = "lldap-client-setup.sh"
    content     = local.client_script
  }]
}

output "startup_script" {
  description = "The LDAP client setup script as a string, for the `startup_script` input of the Slurm v6 login and nodeset modules."
  value       = local.client_script
}

output "controller_startup_script" {
  description = "The LDAP client setup script as a string, for the `controller_startup_script` input of the Slurm v6 controller module."
  value       = local.client_script
}

output "ldap_uri" {
  description = "LDAPS URI of the LLDAP server (static internal IP, port 6360)."
  value       = local.ldap_uri
}

output "base_dn" {
  description = "LDAP base DN."
  value       = var.base_dn
}

output "bind_dn" {
  description = "DN of the read-only bind user used by SSSD."
  value       = local.bind_dn
}

output "ca_cert_secret_id" {
  description = "Secret Manager secret holding the PEM CA certificate of the LDAPS server."
  value       = google_secret_manager_secret.ca_cert.id
}

output "bind_password_secret_id" {
  description = "Secret Manager secret holding the bind user's password."
  value       = google_secret_manager_secret.bind_password.id
}

output "admin_password_secret_id" {
  description = "Secret Manager secret holding the LLDAP admin password (user `admin`)."
  value       = google_secret_manager_secret.admin_password.id
}

output "user_password_secret_ids" {
  description = "Map of username to the Secret Manager secret holding that user's initial password."
  value       = { for name, s in google_secret_manager_secret.user_password : name => s.id }
}

output "lldap_backup_bucket" {
  description = "Name of the GCS bucket holding database backups, or null when backups are disabled."
  value       = var.enable_backups ? google_storage_bucket.backup[0].name : null
}

output "lldap_instance_name" {
  description = "Name of the LLDAP VM."
  value       = google_compute_instance.lldap.name
}

output "lldap_service_account_email" {
  description = "Email of the service account created for the LLDAP VM."
  value       = google_service_account.lldap.email
}
