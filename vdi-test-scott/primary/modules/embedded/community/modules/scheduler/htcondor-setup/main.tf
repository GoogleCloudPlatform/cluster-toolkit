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
  labels = merge(var.labels, { ghpc_module = "htcondor-setup", ghpc_role = "scheduler" })
}

locals {
  service_account_iam_email = [
    "serviceAccount:${var.access_point_service_account_email}",
    "serviceAccount:${var.central_manager_service_account_email}",
    "serviceAccount:${var.execute_point_service_account_email}",
  ]
  service_account_email = [
    var.access_point_service_account_email,
    var.central_manager_service_account_email,
    var.execute_point_service_account_email,
  ]
}

module "health_check_firewall_rule" {
  source = "../../../../modules/network/firewall-rules"

  subnetwork_self_link = var.subnetwork_self_link

  ingress_rules = [{
    name        = "allow-health-check-${var.deployment_name}"
    description = "Allow Managed Instance Group Health Checks for HTCondor VMs"
    direction   = "INGRESS"
    source_ranges = [
      "130.211.0.0/22",
      "35.191.0.0/16",
    ]
    target_service_accounts = local.service_account_email
    allow = [{
      protocol = "tcp"
      ports    = ["9618"]
    }]
  }]
}

module "htcondor_bucket" {
  source = "../../../../community/modules/file-system/cloud-storage-bucket"

  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  name_prefix     = "${var.deployment_name}-htcondor-config"
  random_suffix   = true
  labels          = local.labels
  viewers         = local.service_account_iam_email

  use_deployment_name_in_bucket_name = false
}
