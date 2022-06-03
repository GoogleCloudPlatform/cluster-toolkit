# Copyright 2022 Google LLC
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

output "access_point_service_account" {
  description = "HTCondor Access Point Service Account (e-mail format)"
  value       = module.access_point_service_account.email
  depends_on = [
    google_secret_manager_secret_iam_member.access_point
  ]
}

output "central_manager_service_account" {
  description = "HTCondor Central Manager Service Account (e-mail format)"
  value       = module.central_manager_service_account.email
  depends_on = [
    google_secret_manager_secret_iam_member.central_manager
  ]
}

output "pool_password_secret_id" {
  description = "Google Cloud Secret Manager ID containing HTCondor Pool Password"
  value       = google_secret_manager_secret.pool_password.secret_id
  sensitive   = true
}

output "central_manager_runners" {
  description = "Toolkit Runner to configure an HTCondor Central Manager"
  value       = local.central_manager_runners
}

output "access_point_runners" {
  description = "Toolkit Runner to configure an HTCondor Access Point"
  value       = local.access_point_runners
}
