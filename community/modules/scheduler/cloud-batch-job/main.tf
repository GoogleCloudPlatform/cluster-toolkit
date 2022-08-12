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
  instance_template = var.instance_template != null ? var.instance_template : module.instance_template[0].self_link

  job_template_contents = templatefile(
    "${path.module}/templates/batch-job-base.json.tftpl",
    {
      runnable          = var.runnable
      log_policy        = var.log_policy
      instance_template = local.instance_template
    }
  )

  job_id                   = var.job_id != null ? var.job_id : var.deployment_name
  job_filename             = var.job_filename != null ? var.job_filename : "cloud-batch-${local.job_id}.json"
  job_template_output_path = "${path.root}/${local.job_filename}"
}

module "instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "> 7.6.0"
  count   = var.instance_template == null ? 1 : 0

  name_prefix     = "${local.job_id}-instance-template"
  project_id      = var.project_id
  network         = var.network_self_link
  subnetwork      = var.subnetwork_self_link
  service_account = var.service_account
  access_config   = [{ nat_ip = null, network_tier = null }]
  labels          = var.labels

  machine_type         = var.machine_type
  startup_script       = var.startup_script
  metadata             = var.network_storage != null ? ({ network_storage = jsonencode(var.network_storage) }) : {}
  source_image_family  = var.image.family
  source_image_project = var.image.project
}

resource "local_file" "job_template" {
  content  = local.job_template_contents
  filename = local.job_template_output_path
}
