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
  display_name = "${var.display_name} (${var.deployment_name})"
  description  = "${var.description} (${var.deployment_name})"
}

module "service_account" {
  source  = "terraform-google-modules/service-accounts/google"
  version = "~> 4.2"

  billing_account_id = var.billing_account_id
  description        = local.description
  display_name       = local.display_name
  generate_keys      = var.generate_keys
  grant_billing_role = var.grant_billing_role
  grant_xpn_roles    = var.grant_xpn_roles
  names              = [var.name]
  org_id             = var.org_id
  prefix             = var.deployment_name
  project_id         = var.project_id
  project_roles      = [for role in var.project_roles : "${var.project_id}=>roles/${role}"]
}
