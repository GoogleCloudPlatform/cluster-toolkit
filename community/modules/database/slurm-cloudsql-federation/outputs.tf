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

output "cloudsql" {
  description = "Describes the cloudsql instance."
  sensitive   = true
  value = {
    server_ip                = var.use_psc_connection ? google_compute_address.psc[0].address : google_sql_database_instance.instance.ip_address[0].ip_address
    user                     = google_sql_user.users.name
    password                 = google_sql_user.users.password
    db_name                  = google_sql_database.database.name
    user_managed_replication = local.user_managed_replication
  }
}
