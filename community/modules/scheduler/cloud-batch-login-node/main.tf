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

data "google_compute_instance_template" "batch_instance_template" {
  name = var.instance_template
}

locals {
  instance_template_metadata = data.google_compute_instance_template.batch_instance_template.metadata
  batch_startup_script       = local.instance_template_metadata["startup-script"]
  startup_metadata           = { startup-script = module.login_startup_script.startup_script }

  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  oslogin_metadata = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }

  login_metadata = merge(local.instance_template_metadata, local.startup_metadata, local.oslogin_metadata)
}

module "login_startup_script" {
  source          = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=v1.3.0"
  labels          = var.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners = [
    {
      content     = local.batch_startup_script
      destination = "/tmp/startup-scripts/batch_startup_script.sh"
      type        = "shell"
    },
    {
      content     = var.job_template_contents
      destination = "${var.batch_job_directory}/${var.job_filename}"
      type        = "data"
    }
  ]
}

resource "google_compute_instance_from_template" "batch_login" {
  name                     = "${var.deployment_name}-batch-login"
  source_instance_template = var.instance_template
  project                  = var.project_id
  metadata                 = local.login_metadata

  service_account {
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }
}
