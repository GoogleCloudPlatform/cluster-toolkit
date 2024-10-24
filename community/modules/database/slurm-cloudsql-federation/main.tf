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

locals {
  user_managed_replication = var.user_managed_replication
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


resource "google_sql_database_instance" "instance" {
  project             = var.project_id
  depends_on          = [var.private_vpc_connection_peering]
  name                = local.sql_instance_name
  region              = var.region
  deletion_protection = var.deletion_protection
  database_version    = var.database_version

  settings {
    disk_size       = var.disk_size_gb
    disk_autoresize = var.disk_autoresize
    edition         = var.edition
    tier            = var.tier
    user_labels     = local.labels

    dynamic "data_cache_config" {
      for_each = var.edition == "ENTERPRISE_PLUS" ? [""] : []
      content {
        data_cache_enabled = var.data_cache_enabled
      }
    }
    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.use_psc_connection ? null : var.network_id
      enable_private_path_for_google_cloud_services = true

      dynamic "authorized_networks" {
        for_each = var.use_psc_connection ? [] : var.authorized_networks
        iterator = ip_range

        content {
          value = ip_range.value
        }
      }
      dynamic "psc_config" {
        for_each = var.use_psc_connection ? [""] : []
        content {
          psc_enabled               = true
          allowed_consumer_projects = [var.project_id]
        }
      }
    }

    backup_configuration {
      enabled = var.enable_backups
      # to allow easy switching between ENTERPRISE and ENTERPRISE_PLUS
      transaction_log_retention_days = 7
    }
  }
  lifecycle {
    precondition {
      condition     = var.disk_autoresize && var.disk_size_gb == null || !var.disk_autoresize
      error_message = "If setting disk_size_gb set disk_autorize to false to prevent re-provisioning of the instance after disk auto-expansion."
    }
  }
}



resource "google_compute_address" "psc" {
  count        = var.use_psc_connection ? 1 : 0
  project      = var.project_id
  name         = local.sql_instance_name
  address_type = "INTERNAL"
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  labels       = local.labels
}

resource "google_compute_forwarding_rule" "psc_consumer" {
  count                 = var.use_psc_connection ? 1 : 0
  name                  = local.sql_instance_name
  project               = var.project_id
  region                = var.region
  subnetwork            = var.subnetwork_self_link
  ip_address            = google_compute_address.psc[0].self_link
  load_balancing_scheme = ""
  recreate_closed_psc   = true
  target                = google_sql_database_instance.instance.psc_service_attachment_link
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
