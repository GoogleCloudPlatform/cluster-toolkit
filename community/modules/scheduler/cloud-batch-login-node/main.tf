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

  # Handle directly created job data (deprecated). All of job_id, job_template_contents and job_filename must be set.
  default_job_data = var.job_template_contents == null || var.job_id == null || var.job_filename == null ? [] : [{
    id                = var.job_id
    filename          = var.job_filename
    template_contents = var.job_template_contents
  }]

  job_data = concat(local.default_job_data, var.job_data)

  job_template_runners = [for job in local.job_data : {
    content     = job.template_contents
    destination = "${var.batch_job_directory}/${job.filename}"
    type        = "data"
  }]

  instance_template_metadata = data.google_compute_instance_template.batch_instance_template.metadata
  startup_metadata           = { startup-script = module.login_startup_script.startup_script }

  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  oslogin_metadata = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }

  login_metadata = merge(local.instance_template_metadata, local.startup_metadata, local.oslogin_metadata)

  batch_command_instructions = join("\n", [for job in local.job_data : <<-EOT
  ## For job: ${job.id} ##

  Submit your job from login node:
    gcloud ${var.gcloud_version} batch jobs submit ${job.id} --config=${var.batch_job_directory}/${job.filename} --location=${var.region} --project=${var.project_id}
  
  Check status:
    gcloud ${var.gcloud_version} batch jobs describe ${job.id} --location=${var.region} --project=${var.project_id} | grep state:
  
  Delete job:
    gcloud ${var.gcloud_version} batch jobs delete ${job.id} --location=${var.region} --project=${var.project_id}

  EOT
  ])

  readme_contents = <<-EOT
  # Batch Job Templates

  This folder contains Batch job templates created by the Cloud HPC Toolkit.
  These templates can be edited before submitting to Batch to capture more
  complex workloads.

  Use the following commands to:
  ${local.batch_command_instructions}
  EOT

  # Construct startup script for network storage
  storage_client_install_runners = [
    for ns in var.network_storage :
    ns.client_install_runner if ns.client_install_runner != null
  ]
  mount_runners = [
    for ns in var.network_storage :
    ns.mount_runner if ns.mount_runner != null
  ]

  startup_script_runner = {
    content     = var.startup_script != null ? var.startup_script : "echo 'Batch job template had no startup script'"
    destination = "passed_startup_script.sh"
    type        = "shell"
  }
}

module "login_startup_script" {
  source          = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=v1.7.0"
  labels          = var.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners = concat(
    local.storage_client_install_runners,
    local.mount_runners,
    [local.startup_script_runner],
    local.job_template_runners,
    [
      {
        content     = local.readme_contents
        destination = "${var.batch_job_directory}/README.md"
        type        = "data"
      },
      {
        content     = var.job_template_contents
        destination = local.job_template_destination
        type        = "data"
      }
    ]
  )
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
