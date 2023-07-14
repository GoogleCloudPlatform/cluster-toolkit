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
  labels = merge(var.labels, { ghpc_module = "htcondor-base" })
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

  cm_config = templatefile("${path.module}/templates/condor_config.tftpl", {
    htcondor_role       = "get_htcondor_central_manager",
    central_manager_ips = module.address.addresses,
  })

  cm_object = "gs://${module.htcondor_bucket.name}/${google_storage_bucket_object.cm_config.output_name}"
  runner_cm = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure.yml")
    "destination" = "htcondor_configure.yml"
    "args" = join(" ", [
      "-e htcondor_role=get_htcondor_central_manager",
      "-e config_object=${local.cm_object}",
    ])
  }
}

module "htcondor_bucket" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "~> 4.0"

  project_id       = var.project_id
  location         = var.region
  prefix           = var.deployment_name
  names            = ["htcondor-config"]
  randomize_suffix = true
  labels           = local.labels

  bucket_viewers = {
    "htcondor-config" = join(",", [
      module.access_point_service_account.iam_email,
      module.central_manager_service_account.iam_email,
      module.execute_point_service_account.iam_email,
    ])
  }
  set_viewer_roles = true
}

resource "google_storage_bucket_object" "cm_config" {
  name    = "${var.deployment_name}-cm-config-${substr(md5(local.cm_config), 0, 4)}"
  content = local.cm_config
  bucket  = module.htcondor_bucket.name
}

module "access_point_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.2"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["access"]
  display_name  = local.access_point_display_name
  project_roles = local.access_point_roles
}

module "execute_point_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.2"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["execute"]
  display_name  = local.execute_point_display_name
  project_roles = local.execute_point_roles
}

module "central_manager_service_account" {
  source     = "terraform-google-modules/service-accounts/google"
  version    = "~> 4.2"
  project_id = var.project_id
  prefix     = var.deployment_name

  names         = ["cm"]
  display_name  = local.central_manager_display_name
  project_roles = local.central_manager_roles
}

module "address" {
  source     = "terraform-google-modules/address/google"
  version    = "~> 3.0"
  project_id = var.project_id
  region     = var.region
  subnetwork = var.subnetwork_self_link
  names      = local.central_manager_ip_names
}

data "google_compute_subnetwork" "htcondor" {
  self_link = var.subnetwork_self_link
}

module "health_check_firewall_rule" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 6.0"
  project_id   = data.google_compute_subnetwork.htcondor.project
  network_name = data.google_compute_subnetwork.htcondor.network

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
