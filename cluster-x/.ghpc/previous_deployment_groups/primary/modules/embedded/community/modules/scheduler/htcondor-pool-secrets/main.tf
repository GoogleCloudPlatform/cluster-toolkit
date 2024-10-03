/**
 * Copyright 2023 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "htcondor-pool-secrets", ghpc_role = "scheduler" })
}

locals {
  pool_password                             = coalesce(var.pool_password, random_password.pool.result)
  auto                                      = length(var.user_managed_replication) == 0 ? "" : "-user"
  access_point_service_account_iam_email    = "serviceAccount:${var.access_point_service_account_email}"
  central_manager_service_account_iam_email = "serviceAccount:${var.central_manager_service_account_email}"
  execute_point_service_account_iam_email   = "serviceAccount:${var.execute_point_service_account_email}"

  trust_domain = coalesce(var.trust_domain, "c.${var.project_id}.internal")

  runner_cm = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_secrets.yml")
    "destination" = "htcondor_secrets.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_central_manager",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e xp_idtoken_secret_id=${google_secret_manager_secret.execute_point_idtoken.secret_id}",
      "-e trust_domain=${local.trust_domain}",
    ])
  }

  runner_access = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_secrets.yml")
    "destination" = "htcondor_secrets.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_submit",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e trust_domain=${local.trust_domain}",
    ])
  }

  runner_execute = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_secrets.yml")
    "destination" = "htcondor_secrets.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_execute",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e xp_idtoken_secret_id=${google_secret_manager_secret.execute_point_idtoken.secret_id}",
      "-e trust_domain=${local.trust_domain}",
    ])
  }
  windows_startup_ps1 = templatefile(
    "${path.module}/templates/fetch-idtoken.ps1.tftpl",
    {
      trust_domain         = local.trust_domain,
      xp_idtoken_secret_id = google_secret_manager_secret.execute_point_idtoken.secret_id,
    }
  )
}

resource "random_password" "pool" {
  length           = 24
  special          = true
  override_special = "_-#=."
}

resource "google_secret_manager_secret" "pool_password" {
  secret_id = "${var.deployment_name}-pool-password${local.auto}"

  labels = local.labels

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

resource "google_secret_manager_secret_version" "pool_password" {
  secret      = google_secret_manager_secret.pool_password.id
  secret_data = local.pool_password
}

# this secret will be populated by the Central Manager
resource "google_secret_manager_secret" "execute_point_idtoken" {
  secret_id = "${var.deployment_name}-execute-point-idtoken${local.auto}"

  labels = local.labels

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

resource "google_secret_manager_secret_iam_member" "central_manager_password" {
  secret_id = google_secret_manager_secret.pool_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = local.central_manager_service_account_iam_email
}

resource "google_secret_manager_secret_iam_member" "central_manager_idtoken" {
  secret_id = google_secret_manager_secret.execute_point_idtoken.id
  role      = "roles/secretmanager.secretVersionManager"
  member    = local.central_manager_service_account_iam_email
}

resource "google_secret_manager_secret_iam_member" "access_point" {
  secret_id = google_secret_manager_secret.pool_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = local.access_point_service_account_iam_email
}

resource "google_secret_manager_secret_iam_member" "execute_point" {
  secret_id = google_secret_manager_secret.execute_point_idtoken.id
  role      = "roles/secretmanager.secretAccessor"
  member    = local.execute_point_service_account_iam_email
}
