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

output "access_point_service_account" {
  description = "HTCondor Access Point Service Account (e-mail format)"
  value       = module.access_point_service_account.email
  depends_on = [
    google_secret_manager_secret_iam_member.access_point,
    module.access_point_service_account
  ]
}

output "central_manager_service_account" {
  description = "HTCondor Central Manager Service Account (e-mail format)"
  value       = module.central_manager_service_account.email
  depends_on = [
    google_secret_manager_secret_iam_member.central_manager,
    module.central_manager_service_account
  ]
}

output "execute_point_service_account" {
  description = "HTCondor Execute Point Service Account (e-mail format)"
  value       = module.execute_point_service_account.email
  depends_on = [
    google_secret_manager_secret_iam_member.execute_point,
    module.execute_point_service_account
  ]
}

output "pool_password_secret_id" {
  description = "Google Cloud Secret Manager ID containing HTCondor Pool Password"
  value       = google_secret_manager_secret.pool_password.secret_id
  sensitive   = true
}

output "central_manager_runner" {
  description = "Toolkit Runner to configure an HTCondor Central Manager"
  value       = local.runner_cm
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "access_point_runner" {
  description = "Toolkit Runner to configure an HTCondor Access Point"
  value       = local.runner_access
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "execute_point_runner" {
  description = "Toolkit Runner to configure an HTCondor Execute Point"
  value       = local.runner_execute
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "central_manager_internal_ip" {
  description = "Reserved internal IP address for use by Central Manager"
  value       = try(module.address.addresses[0], null)
}

output "central_manager_secondary_internal_ip" {
  description = "Reserved internal IP address for use by failover Central Manager"
  value       = try(module.address.addresses[1], null)
}

output "windows_startup_ps1" {
  description = "Windows PowerShell script to update HTCondor configuration file"
  value       = local.windows_startup_ps1
}
