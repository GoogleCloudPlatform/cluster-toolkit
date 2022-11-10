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
  network_storage_metadata = var.network_storage == null ? {} : { network_storage = jsonencode(var.network_storage) }

  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  enable_oslogin = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }

  metadata = merge(var.metadata, local.network_storage_metadata, local.enable_oslogin)

  configure_autoscaler_role = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure_autoscaler.yml")
    "destination" = "htcondor_configure_autoscaler.yml"
    "args"        = "-e project_id=${var.project_id} -e region=${var.region} -e zone=${var.zone} -e mig_id=${module.mig.instance_group_manager.name} -e max_size=${var.max_size}"
  }

}

module "execute_point_instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "~> 7.8.0"

  name_prefix     = "${var.deployment_name}-xp"
  project_id      = var.project_id
  network         = var.network_self_link
  subnetwork      = var.subnetwork_self_link
  service_account = var.service_account
  labels          = var.labels

  machine_type         = var.machine_type
  startup_script       = var.startup_script
  metadata             = local.metadata
  source_image_family  = var.image.family
  source_image_project = var.image.project
}

module "mig" {
  source            = "terraform-google-modules/vm/google//modules/mig"
  version           = "~> 7.8.0"
  project_id        = var.project_id
  region            = var.region
  target_size       = var.target_size
  hostname          = "${var.deployment_name}-x"
  instance_template = module.execute_point_instance_template.self_link
}
