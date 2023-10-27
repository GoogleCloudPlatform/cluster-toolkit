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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "slurm-cloudsql-federation", ghpc_role = "database" })
}

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

resource "google_compute_global_address" "private_ip_address" {
  provider = google-beta

  name          = "slurm-cloudsql-private-ip-${random_id.resource_name_suffix.hex}"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = var.network_id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  provider = google-beta

  network                 = var.network_id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = google_compute_global_address.private_ip_address[*].name
}

resource "google_sql_database_instance" "instance" {
  project             = var.project_id
  depends_on          = [google_service_networking_connection.private_vpc_connection]
  name                = local.sql_instance_name
  region              = var.region
  deletion_protection = var.deletion_protection
  database_version    = "MYSQL_5_7"

  settings {
    user_labels = local.labels
    tier        = var.tier
    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.network_id
      enable_private_path_for_google_cloud_services = true
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
  provider = google
  project  = var.project_id
  location = var.region
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
