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
  labels = merge(var.labels, { ghpc_module = "htcondor-configure" })
}

locals {
  execute_point_display_name   = "HTCondor Execute Point (${var.deployment_name})"
  execute_point_roles          = [for role in var.execute_point_roles : "${var.project_id}=>${role}"]
  access_point_display_name    = "HTCondor Access Point (${var.deployment_name})"
  access_point_roles           = [for role in var.access_point_roles : "${var.project_id}=>${role}"]
  central_manager_display_name = "HTCondor Central Manager (${var.deployment_name})"
  central_manager_roles        = [for role in var.central_manager_roles : "${var.project_id}=>${role}"]

  central_manager_count    = var.central_manager_high_availability ? 2 : 1
  central_manager_ip_names = [for i in range(local.central_manager_count) : "${var.deployment_name}-cm-ip-${i}"]

  pool_password = var.pool_password == null ? random_password.pool.result : var.pool_password

  runner_cm_role = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure.yml")
    "destination" = "htcondor_configure.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_central_manager",
      "-e htcondor_central_manager_ips=${join(",", module.address.addresses)}",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e project_id=${var.project_id}",
    ])
  }

  runner_access_role = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure.yml")
    "destination" = "htcondor_configure.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_submit",
      "-e htcondor_central_manager_ips=${join(",", module.address.addresses)}",
      "-e job_queue_ha=${var.job_queue_high_availability}",
      "-e spool_dir=${var.spool_parent_dir}/spool",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e project_id=${var.project_id}",
    ])
  }

  runner_execute_role = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure.yml")
    "destination" = "htcondor_configure.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_execute",
      "-e htcondor_central_manager_ips=${join(",", module.address.addresses)}",
      "-e password_id=${google_secret_manager_secret.pool_password.secret_id}",
      "-e project_id=${var.project_id}",
    ])
  }
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

module "execute_point_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.1"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["execute"]
  display_name  = local.execute_point_display_name
  project_roles = local.execute_point_roles
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

  labels = local.labels

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

resource "google_secret_manager_secret_iam_member" "execute_point" {
  secret_id = google_secret_manager_secret.pool_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = module.execute_point_service_account.iam_email
}

module "address" {
  source     = "terraform-google-modules/address/google"
  version    = "~> 3.0"
  project_id = var.project_id
  region     = var.region
  subnetwork = var.network.primary_subnet.self_link
  names      = local.central_manager_ip_names
}

module "health_check_firewall_rule" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 6.0"
  project_id   = var.network.project
  network_name = var.network.name

  rules = [{
    name        = "allow-health-check-${var.deployment_name}"
    description = "Allow Managed Instance Group Health Checks for HTCondor VMs"
    direction   = "INGRESS"
    priority    = null
    ranges = [
      "130.211.0.0/22",
      "35.191.0.0/16",
    ]
    source_tags             = null
    source_service_accounts = null
    target_tags             = null
    target_service_accounts = [
      module.access_point_service_account.email,
      module.central_manager_service_account.email,
      module.execute_point_service_account.email,
    ]
    allow = [{
      protocol = "tcp"
      ports    = ["9618"]
    }]
    deny = []
    log_config = {
      metadata = "INCLUDE_ALL_METADATA"
    }
  }]
}
