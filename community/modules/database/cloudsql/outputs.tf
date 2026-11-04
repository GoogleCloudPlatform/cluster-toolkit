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

output "instance_name" {
  description = "The name of the Cloud SQL instance, whether created or adopted."
  value       = local.effective_instance_name
}

output "connection_name" {
  description = "The connection name of the Cloud SQL instance, used by the Cloud SQL Auth Proxy and BigQuery federation."
  value       = local.instance_connection_name
}

output "self_link" {
  description = "The URI of the Cloud SQL instance."
  value       = local.instance_self_link
}

# depends_on is per output. It holds each output back until what a consumer
# needs to use it exists: the PSC endpoint's forwarding rule for the address,
# and the password's secret version and accessor grants for the secret.

output "private_ip_address" {
  description = "The private IP address of the Cloud SQL instance, or its PSC endpoint address when use_psc_connection is true."
  value       = local.server_ip

  depends_on = [google_compute_forwarding_rule.psc_consumer]
}

output "database_name" {
  description = "The name of the database created on the instance."
  value       = google_sql_database.database.name
}

output "username" {
  description = "The username of the database user."
  value       = google_sql_user.users.name
}

output "password_secret_id" {
  description = "The Secret Manager secret ID holding the database password."
  value       = google_secret_manager_secret.password.secret_id

  depends_on = [
    google_secret_manager_secret_version.password,
    google_secret_manager_secret_iam_member.password_accessor,
  ]
}

output "password_secret_version" {
  description = "The Secret Manager secret version resource name holding the database password."
  value       = google_secret_manager_secret_version.password.name

  depends_on = [
    google_secret_manager_secret_version.password,
    google_secret_manager_secret_iam_member.password_accessor,
  ]
}
