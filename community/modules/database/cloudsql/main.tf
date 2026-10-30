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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "cloudsql", ghpc_role = "database" })
}

# effective_instance_name reads the created resource so that a same-apply
# adopter depends on the instance existing. private_ip_address is used, not
# ip_address[0], because an adopted instance can also have unordered public or
# outgoing addresses. PostgreSQL users are abandoned on destroy because a user
# that owns objects cannot be dropped, and an adopted instance's database is
# abandoned so destroy keeps its data. Deletion protection guards only the
# instance, and Terraform destroys the database and user before it, so they
# are also abandoned while either protection is on.
locals {
  create_instance        = var.existing_instance_name == null
  sql_instance_name      = var.sql_instance_name != null ? var.sql_instance_name : (local.create_instance ? "${var.deployment_name}-sql-${random_id.resource_name_suffix[0].hex}" : null)
  database_engine        = local.create_instance ? var.database_version : data.google_sql_database_instance.existing[0].database_version
  instance_protected     = local.create_instance && (coalesce(var.deletion_protection, true) || coalesce(var.api_deletion_protection, false))
  user_deletion_policy   = var.user_deletion_policy != null ? var.user_deletion_policy : (startswith(local.database_engine, "POSTGRES") || local.instance_protected ? "ABANDON" : null)
  db_deletion_policy     = var.database_deletion_policy != null ? var.database_deletion_policy : (!local.create_instance || local.instance_protected ? "ABANDON" : null)
  point_in_time_recovery = var.enable_backups && var.point_in_time_recovery

  effective_instance_name  = local.create_instance ? google_sql_database_instance.instance[0].name : var.existing_instance_name
  instance_self_link       = local.create_instance ? google_sql_database_instance.instance[0].self_link : data.google_sql_database_instance.existing[0].self_link
  instance_connection_name = local.create_instance ? google_sql_database_instance.instance[0].connection_name : data.google_sql_database_instance.existing[0].connection_name
  instance_ip_address      = local.create_instance ? google_sql_database_instance.instance[0].private_ip_address : data.google_sql_database_instance.existing[0].private_ip_address
  server_ip                = var.use_psc_connection ? google_compute_address.psc[0].address : local.instance_ip_address

  password_secret_accessors = toset(concat(var.password_secret_accessors, var.service_account_iam_email))
}

resource "random_id" "resource_name_suffix" {
  count = local.create_instance ? 1 : 0

  byte_length = 4
}

# Never stored in state or plan. Only the first value reaches the secret, and
# the user is set from the secret, so the two cannot diverge.
ephemeral "random_password" "password" {
  length  = 24
  special = false
}

# Root (MySQL) or postgres (PostgreSQL) password nobody knows, so the
# built-in user cannot be used without one. Never stored in state or output.
ephemeral "random_password" "root" {
  count = local.create_instance && var.set_root_password ? 1 : 0

  length  = 32
  special = false
}

resource "google_sql_database_instance" "instance" {
  count = local.create_instance ? 1 : 0

  project             = var.project_id
  depends_on          = [var.private_vpc_connection_peering]
  name                = local.sql_instance_name
  region              = var.region
  deletion_protection = var.deletion_protection
  database_version    = var.database_version
  encryption_key_name = var.encryption_key_name

  root_password_wo         = one(ephemeral.random_password.root[*].result)
  root_password_wo_version = var.set_root_password ? "1" : null

  settings {
    disk_size                   = var.disk_size_gb
    disk_autoresize             = var.disk_autoresize
    edition                     = var.edition
    tier                        = var.tier
    availability_type           = var.availability_type
    deletion_protection_enabled = var.api_deletion_protection
    retain_backups_on_delete    = var.retain_backups_on_delete
    user_labels                 = local.labels

    dynamic "final_backup_config" {
      for_each = var.final_backup_retention_days == null ? [] : [var.final_backup_retention_days]
      content {
        enabled        = true
        retention_days = final_backup_config.value
      }
    }

    dynamic "data_cache_config" {
      for_each = var.edition == "ENTERPRISE_PLUS" ? [""] : []
      content {
        data_cache_enabled = var.data_cache_enabled
      }
    }

    dynamic "database_flags" {
      for_each = var.database_flags
      content {
        name  = database_flags.key
        value = database_flags.value
      }
    }

    insights_config {
      query_insights_enabled  = var.query_insights.enabled
      query_plans_per_minute  = var.query_insights.query_plans_per_minute
      query_string_length     = var.query_insights.query_string_length
      record_application_tags = var.query_insights.record_application_tags
      record_client_address   = var.query_insights.record_client_address
    }

    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.use_psc_connection ? null : var.network_id
      enable_private_path_for_google_cloud_services = true
      ssl_mode                                      = var.ssl_mode

      dynamic "psc_config" {
        for_each = var.use_psc_connection ? [""] : []
        content {
          psc_enabled               = true
          allowed_consumer_projects = coalesce(var.psc_allowed_consumer_projects, [var.project_id])
        }
      }
    }

    # transaction_log_retention_days = 7 allows easy switching between
    # ENTERPRISE and ENTERPRISE_PLUS.
    backup_configuration {
      enabled                        = var.enable_backups
      start_time                     = var.backup_start_time
      binary_log_enabled             = startswith(var.database_version, "MYSQL") ? local.point_in_time_recovery : null
      point_in_time_recovery_enabled = startswith(var.database_version, "POSTGRES") ? local.point_in_time_recovery : null
      transaction_log_retention_days = 7

      dynamic "backup_retention_settings" {
        for_each = var.retained_backups == null ? [] : [var.retained_backups]
        content {
          retained_backups = backup_retention_settings.value
        }
      }
    }

    dynamic "maintenance_window" {
      for_each = var.maintenance_window != null ? [var.maintenance_window] : []
      content {
        day          = maintenance_window.value.day
        hour         = maintenance_window.value.hour
        update_track = maintenance_window.value.update_track
      }
    }
  }

  # The root password is set only when the instance is created. Any later
  # change to root_password_wo_version, including set_root_password turning
  # false, would make the provider send an empty root password.
  lifecycle {
    ignore_changes = [root_password_wo_version]
  }
}

data "google_sql_database_instance" "existing" {
  count   = local.create_instance ? 0 : 1
  project = var.project_id
  name    = var.existing_instance_name

  # Besides checking the address, the postcondition keeps same-apply adoption
  # working: Terraform does not read a data source with conditions at plan
  # while its dependencies have pending changes, so the read waits for an
  # instance created in the same apply. Without it the read runs at plan,
  # before that instance exists, and fails.
  lifecycle {
    postcondition {
      condition     = self.private_ip_address != null && self.private_ip_address != ""
      error_message = "The Cloud SQL instance named in existing_instance_name has no private IP address. This module only connects over private networking, so the instance must be attached to a VPC network through Private Service Access."
    }
  }
}

resource "google_compute_address" "psc" {
  count = var.use_psc_connection ? 1 : 0

  project      = var.project_id
  name         = local.sql_instance_name
  address_type = "INTERNAL"
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  labels       = local.labels
}

resource "google_compute_forwarding_rule" "psc_consumer" {
  count = var.use_psc_connection ? 1 : 0

  name                  = local.sql_instance_name
  project               = var.project_id
  region                = var.region
  subnetwork            = var.subnetwork_self_link
  ip_address            = google_compute_address.psc[0].self_link
  load_balancing_scheme = ""
  recreate_closed_psc   = true
  target                = google_sql_database_instance.instance[0].psc_service_attachment_link
  labels                = local.labels
}

resource "google_sql_database" "database" {
  project         = var.project_id
  name            = var.database_name
  instance        = local.effective_instance_name
  deletion_policy = local.db_deletion_policy
}

# password_wo_version follows the secret version's creation time, so the user
# is updated whenever the version is replaced (a rotation or a new secret).
resource "google_sql_user" "users" {
  project         = var.project_id
  name            = var.sql_username
  instance        = local.effective_instance_name
  deletion_policy = local.user_deletion_policy

  password_wo         = ephemeral.google_secret_manager_secret_version.password.secret_data
  password_wo_version = parseint(formatdate("YYYYMMDDhhmmss", google_secret_manager_secret_version.password.create_time), 10)

  lifecycle {
    precondition {
      condition     = !startswith(local.database_engine, "MYSQL") || length(var.sql_username) <= (local.database_engine == "MYSQL_5_6" ? 16 : 32)
      error_message = "sql_username must be at most 32 characters for MySQL (16 for MYSQL_5_6)."
    }
  }
}

resource "google_secret_manager_secret" "password" {
  project   = var.project_id
  secret_id = "${local.effective_instance_name}-${var.sql_username}-password"
  labels    = local.labels

  replication {
    dynamic "auto" {
      for_each = length(var.user_managed_replication) == 0 ? [1] : []
      content {}
    }
    dynamic "user_managed" {
      for_each = length(var.user_managed_replication) == 0 ? [] : [1]
      content {
        dynamic "replicas" {
          for_each = var.user_managed_replication
          content {
            location = replicas.value.location
            dynamic "customer_managed_encryption" {
              for_each = compact([replicas.value.kms_key_name])
              content {
                kms_key_name = customer_managed_encryption.value
              }
            }
          }
        }
      }
    }
  }
}

# No create_before_destroy: Terraform would extend it to the secret and the
# instance, whose fixed names would then collide when either is replaced.
# DISABLE keeps a replaced version, so an apply that fails between removing it
# and creating its successor can be recovered by re-enabling it.
resource "google_secret_manager_secret_version" "password" {
  secret                 = google_secret_manager_secret.password.id
  secret_data_wo         = var.sql_password != null ? var.sql_password : ephemeral.random_password.password.result
  secret_data_wo_version = var.password_version
  deletion_policy        = "DISABLE"
}

# Read on every plan and apply; see the README on unreadable secret versions.
ephemeral "google_secret_manager_secret_version" "password" {
  secret  = google_secret_manager_secret_version.password.secret
  version = google_secret_manager_secret_version.password.version
}

# Deletion protection is checked only when the instance itself is deleted,
# after Terraform has removed everything that depends on it. This resource
# depends on all of them, so its destroy runs first and fails while the
# instance is protected, before anything else in the module is removed.
resource "terraform_data" "destroy_guard" {
  count = local.create_instance ? 1 : 0

  input = local.instance_protected
  depends_on = [
    google_sql_database.database,
    google_sql_user.users,
    google_secret_manager_secret_iam_member.password_accessor,
    google_compute_forwarding_rule.psc_consumer,
  ]

  provisioner "local-exec" {
    when    = destroy
    command = self.input ? "echo 'The Cloud SQL instance has deletion_protection or api_deletion_protection on. Set both to false and apply before destroying it.' >&2; exit 1" : "true"
  }
}

# Members are for_each keys, so they must be known at plan time.
resource "google_secret_manager_secret_iam_member" "password_accessor" {
  for_each = local.password_secret_accessors

  secret_id = google_secret_manager_secret.password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = each.value
}
