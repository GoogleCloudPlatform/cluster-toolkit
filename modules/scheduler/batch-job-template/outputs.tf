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
  provided_instance_tpl_msg  = "The Batch job template uses the existing VM instance template:"
  generated_instance_tpl_msg = "The Batch job template uses a new VM instance template created matching the provided settings:"
  submit_msg                 = <<-EOT

    The job has been submitted. See job status at:
      https://console.cloud.google.com/batch/jobsDetail/regions/${var.region}/jobs/${local.submit_job_id}?project=${var.project_id}
  EOT
}

output "instructions" {
  description = "Instructions for submitting the Batch job."
  value       = <<-EOT

  A Batch job template file has been created locally at:
    ${abspath(local.job_template_output_path)}

  ${var.instance_template == null ? local.generated_instance_tpl_msg : local.provided_instance_tpl_msg}
    ${local.instance_template}
  ${var.submit ? local.submit_msg : ""}

  Use the following commands to:
  Submit your job${var.submit ? " (Note: job has already been submitted)" : ""}:
    gcloud ${var.gcloud_version} batch jobs submit ${local.submit_job_id} --config=${abspath(local.job_template_output_path)} --location=${var.region} --project=${var.project_id}
  
  Check status:
    gcloud ${var.gcloud_version} batch jobs describe ${local.submit_job_id} --location=${var.region} --project=${var.project_id} | grep state:
  
  Delete job:
    gcloud ${var.gcloud_version} batch jobs delete ${local.submit_job_id} --location=${var.region} --project=${var.project_id}

  List all jobs:
    gcloud ${var.gcloud_version} batch jobs list --project=${var.project_id}
  EOT
}

output "job_data" {
  description = "All data associated with the defined job, typically provided as input to clout-batch-login-node."
  value = {
    template_contents = local.job_template_contents,
    filename          = local.job_filename,
    id                = local.submit_job_id
  }
}

output "instance_template" {
  description = "Instance template used by the Batch job."
  value       = local.instance_template
}

output "network_storage" {
  description = "An array of network attached storage mounts used by the Batch job."
  value       = var.network_storage
}

output "startup_script" {
  description = "Startup script run before Google Cloud Batch job starts."
  value       = var.startup_script
}

output "gcloud_version" {
  description = "The version of gcloud to be used."
  value       = var.gcloud_version
}
