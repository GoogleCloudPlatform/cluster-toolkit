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
  access_point_display_name    = "HTCondor Access Point (${var.deployment_name})"
  access_point_roles           = [for role in var.access_point_roles : "${var.project_id}=>${role}"]
  central_manager_display_name = "HTCondor Central Manager (${var.deployment_name})"
  central_manager_roles        = [for role in var.central_manager_roles : "${var.project_id}=>${role}"]

  pool_password = var.pool_password == null ? random_password.pool.result : var.pool_password

  start_enable_runner = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_start_enable.yml")
    "destination" = "htcondor_start_enable.yml"
  }

  secure_runner = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_secure.yml")
    "destination" = "htcondor_secure.yml"
    "args"        = "-e \"password_id=${google_secret_manager_secret.pool_password.secret_id}\""
  }

  role_runner_cm = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_role.yml")
    "destination" = "htcondor_role.yml"
    "args"        = "-e \"htcondor_role=get_htcondor_central_manager\""
  }

  role_runner_access = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_role.yml")
    "destination" = "htcondor_role.yml"
    "args"        = "-e \"htcondor_role=get_htcondor_submit\""
  }

  central_manager_runners = [
    local.role_runner_cm,
    local.secure_runner,
    local.start_enable_runner,
  ]

  access_point_runners = [
    local.role_runner_access,
    local.secure_runner,
    local.start_enable_runner,
  ]
}

module "access_point_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.1"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["access"]
  display_name  = local.access_point_display_name
  project_roles = local.access_point_roles
}

module "central_manager_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.1"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["cm"]
  display_name  = local.central_manager_display_name
  project_roles = local.central_manager_roles
}

resource "random_password" "pool" {
  length           = 24
  special          = true
  override_special = "_-#=."
}

resource "google_secret_manager_secret" "pool_password" {
  secret_id = "${var.deployment_name}-pool-password"

  labels = {
    label = var.deployment_name
  }

  replication {
    automatic = true
  }
}

resource "google_secret_manager_secret_version" "pool_password" {
  secret      = google_secret_manager_secret.pool_password.id
  secret_data = local.pool_password
}

resource "google_secret_manager_secret_iam_member" "central_manager" {
  secret_id = google_secret_manager_secret.pool_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = module.central_manager_service_account.iam_email
}

resource "google_secret_manager_secret_iam_member" "access_point" {
  secret_id = google_secret_manager_secret.pool_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = module.access_point_service_account.iam_email
}
