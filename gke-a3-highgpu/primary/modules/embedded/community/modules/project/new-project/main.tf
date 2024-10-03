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
  labels = merge(var.labels, { ghpc_module = "new-project", ghpc_role = "project" })
}

locals {
  name = var.name != null ? var.name : var.project_id
}

module "project_factory" {
  source  = "terraform-google-modules/project-factory/google"
  version = "~> 11.3"

  random_project_id                       = var.random_project_id
  org_id                                  = var.org_id
  domain                                  = var.domain
  name                                    = local.name
  project_id                              = var.project_id
  svpc_host_project_id                    = var.svpc_host_project_id
  enable_shared_vpc_host_project          = var.enable_shared_vpc_host_project
  billing_account                         = var.billing_account
  folder_id                               = var.folder_id
  group_name                              = var.group_name
  group_role                              = var.group_role
  create_project_sa                       = var.create_project_sa
  project_sa_name                         = var.project_sa_name
  sa_role                                 = var.sa_role
  activate_apis                           = var.activate_apis
  activate_api_identities                 = var.activate_api_identities
  usage_bucket_name                       = var.usage_bucket_name
  usage_bucket_prefix                     = var.usage_bucket_prefix
  shared_vpc_subnets                      = var.shared_vpc_subnets
  labels                                  = local.labels
  bucket_project                          = var.bucket_project
  bucket_name                             = var.bucket_name
  bucket_location                         = var.bucket_location
  bucket_versioning                       = var.bucket_versioning
  bucket_labels                           = var.bucket_labels
  bucket_force_destroy                    = var.bucket_force_destroy
  bucket_ula                              = var.bucket_ula
  auto_create_network                     = var.auto_create_network
  lien                                    = var.lien
  disable_services_on_destroy             = var.disable_services_on_destroy
  default_service_account                 = var.default_service_account
  disable_dependent_services              = var.disable_dependent_services
  budget_amount                           = var.budget_amount
  budget_display_name                     = var.budget_display_name
  budget_alert_pubsub_topic               = var.budget_alert_pubsub_topic
  budget_monitoring_notification_channels = var.budget_monitoring_notification_channels
  budget_alert_spent_percents             = var.budget_alert_spent_percents
  vpc_service_control_attach_enabled      = var.vpc_service_control_attach_enabled
  vpc_service_control_perimeter_name      = var.vpc_service_control_perimeter_name
  grant_services_security_admin_role      = var.grant_services_security_admin_role
  grant_services_network_role             = var.grant_services_network_role
  consumer_quotas                         = var.consumer_quotas
  default_network_tier                    = var.default_network_tier

}
