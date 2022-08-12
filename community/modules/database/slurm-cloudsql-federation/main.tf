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

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "random_password" "password" {
  length  = 12
  special = false
}

locals {
  sql_instance_name = var.sql_instance_name == null ? "${var.deployment_name}-sql-${random_id.resource_name_suffix.hex}" : var.sql_instance_name
  sql_password      = var.sql_password == null ? random_password.password.result : var.sql_password
}

resource "google_sql_database_instance" "instance" {
  project             = var.project_id
  depends_on          = [var.nat_ips]
  name                = local.sql_instance_name
  region              = var.region
  deletion_protection = var.deletion_protection
  database_version    = "MYSQL_5_7"

  settings {
    user_labels = var.labels
    tier        = var.tier
    ip_configuration {

      ipv4_enabled = true

      dynamic "authorized_networks" {
        for_each = var.nat_ips
        iterator = ip

        content {
          name  = ip.value
          value = "${ip.value}/32"
        }
      }
    }
  }
}

resource "google_sql_database" "database" {
  project  = var.project_id
  name     = "slurm_accounting"
  instance = google_sql_database_instance.instance.name
}

resource "google_sql_user" "users" {
  project  = var.project_id
  name     = var.sql_username
  instance = google_sql_database_instance.instance.name
  password = local.sql_password
}

resource "google_bigquery_connection" "connection" {
  provider = google-beta
  project  = var.project_id
  cloud_sql {
    instance_id = google_sql_database_instance.instance.connection_name
    database    = google_sql_database.database.name
    type        = "MYSQL"
    credential {
      username = google_sql_user.users.name
      password = google_sql_user.users.password
    }
  }
}
